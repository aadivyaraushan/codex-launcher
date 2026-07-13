package durablestore

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

const hostIdentityKey = "host_identity"

func (store *Store) HostIdentity(ctx context.Context) (ed25519.PrivateKey, error) {
	var identity []byte
	err := store.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = ?`, hostIdentityKey).Scan(&identity)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, pairing.ErrIdentityNotFound
	}
	return ed25519.PrivateKey(identity), err
}

func (store *Store) SaveHostIdentity(ctx context.Context, identity ed25519.PrivateKey) error {
	_, err := store.db.ExecContext(ctx, `INSERT INTO metadata(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, hostIdentityKey, []byte(identity))
	return err
}

func (store *Store) Devices(ctx context.Context) ([]pairing.DeviceRecord, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, name, pairing_generation, current_public_key, pending_public_key, paired_at FROM paired_devices ORDER BY paired_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := make([]pairing.DeviceRecord, 0)
	for rows.Next() {
		device, scanErr := scanDevice(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (store *Store) Device(ctx context.Context, deviceID string) (pairing.DeviceRecord, error) {
	device, err := scanDevice(store.db.QueryRowContext(ctx, `SELECT id, name, pairing_generation, current_public_key, pending_public_key, paired_at FROM paired_devices WHERE id = ?`, deviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return pairing.DeviceRecord{}, pairing.ErrDeviceNotFound
	}
	return device, err
}

func (store *Store) SaveDevice(ctx context.Context, device pairing.DeviceRecord) error {
	_, err := store.db.ExecContext(ctx, `
INSERT INTO paired_devices(id, name, pairing_generation, current_public_key, pending_public_key, paired_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET name = excluded.name, pairing_generation = excluded.pairing_generation,
current_public_key = excluded.current_public_key, pending_public_key = excluded.pending_public_key, paired_at = excluded.paired_at`,
		device.ID, device.Name, device.PairingGeneration, device.CurrentPublicKey, append([]byte{}, device.PendingPublicKey...), encodeTime(device.PairedAt))
	return err
}

func (store *Store) DeleteDevice(ctx context.Context, deviceID string) error {
	result, err := store.db.ExecContext(ctx, `DELETE FROM paired_devices WHERE id = ?`, deviceID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return pairing.ErrDeviceNotFound
	}
	return nil
}

func (store *Store) ClearDevices(ctx context.Context) error {
	_, err := store.db.ExecContext(ctx, `DELETE FROM paired_devices`)
	return err
}

func (store *Store) CreatePairingOffer(ctx context.Context, offer pairing.PairingOfferRecord) error {
	_, err := store.db.ExecContext(ctx, `INSERT INTO pairing_offers(secret_hash, host, port, protocol, expires_at, state) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(secret_hash) DO UPDATE SET host = excluded.host, port = excluded.port, protocol = excluded.protocol, expires_at = excluded.expires_at, state = excluded.state`,
		offer.SecretHash, offer.Target.Host, offer.Target.Port, offer.Target.Protocol, encodeTime(offer.ExpiresAt), pairing.PairingOfferPending)
	return err
}

func (store *Store) ClaimPairingOffer(ctx context.Context, secretHash []byte, now time.Time) (pairing.PairingOfferRecord, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return pairing.PairingOfferRecord{}, err
	}
	defer tx.Rollback()
	var offer pairing.PairingOfferRecord
	var expiresAt string
	err = tx.QueryRowContext(ctx, `SELECT secret_hash, host, port, protocol, expires_at, state FROM pairing_offers WHERE secret_hash = ?`, secretHash).
		Scan(&offer.SecretHash, &offer.Target.Host, &offer.Target.Port, &offer.Target.Protocol, &expiresAt, &offer.State)
	if errors.Is(err, sql.ErrNoRows) {
		return pairing.PairingOfferRecord{}, pairing.ErrPairingCodeExpired
	}
	if err != nil {
		return pairing.PairingOfferRecord{}, err
	}
	offer.ExpiresAt, err = decodeTime(expiresAt)
	if err != nil {
		return pairing.PairingOfferRecord{}, err
	}
	if !now.Before(offer.ExpiresAt) {
		_, _ = tx.ExecContext(ctx, `DELETE FROM pairing_offers WHERE secret_hash = ?`, secretHash)
		_ = tx.Commit()
		return pairing.PairingOfferRecord{}, pairing.ErrPairingCodeExpired
	}
	if offer.State != pairing.PairingOfferPending {
		return pairing.PairingOfferRecord{}, pairing.ErrPairingCodeUsed
	}
	result, err := tx.ExecContext(ctx, `UPDATE pairing_offers SET state = ? WHERE secret_hash = ? AND state = ?`, pairing.PairingOfferClaimed, secretHash, pairing.PairingOfferPending)
	if err != nil {
		return pairing.PairingOfferRecord{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return pairing.PairingOfferRecord{}, err
	} else if affected != 1 {
		return pairing.PairingOfferRecord{}, pairing.ErrPairingCodeUsed
	}
	if err := tx.Commit(); err != nil {
		return pairing.PairingOfferRecord{}, err
	}
	offer.State = pairing.PairingOfferClaimed
	return offer, nil
}

func (store *Store) ReleasePairingOffer(ctx context.Context, secretHash []byte) error {
	_, err := store.db.ExecContext(ctx, `UPDATE pairing_offers SET state = ? WHERE secret_hash = ? AND state = ?`, pairing.PairingOfferPending, secretHash, pairing.PairingOfferClaimed)
	return err
}

func (store *Store) ConsumePairingOffer(ctx context.Context, secretHash []byte) error {
	result, err := store.db.ExecContext(ctx, `UPDATE pairing_offers SET state = ? WHERE secret_hash = ? AND state = ?`, pairing.PairingOfferConsumed, secretHash, pairing.PairingOfferClaimed)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return pairing.ErrPairingCodeUsed
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanDevice(row rowScanner) (pairing.DeviceRecord, error) {
	var device pairing.DeviceRecord
	var pairedAt string
	err := row.Scan(&device.ID, &device.Name, &device.PairingGeneration, &device.CurrentPublicKey, &device.PendingPublicKey, &pairedAt)
	if err != nil {
		return pairing.DeviceRecord{}, err
	}
	device.PairedAt, err = decodeTime(pairedAt)
	return device, err
}

func (store *Store) Create(ctx context.Context, entry promptqueue.Entry) error {
	result, err := store.db.ExecContext(ctx, `
INSERT OR IGNORE INTO prompt_entries(action_id, queue_key, action_kind, owner_source, thread_id, project_id, prompt, model, effort, permission_mode, request_hash, state,
result_code, result_thread_id, result_turn_id, error_code, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, promptArguments(entry)...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return promptqueue.ErrDuplicateAction
	}
	return nil
}

func (store *Store) CompareAndSwap(ctx context.Context, expected promptqueue.State, entry promptqueue.Entry) error {
	arguments := promptArguments(entry)
	arguments = append(arguments, entry.ActionID, string(expected))
	result, err := store.db.ExecContext(ctx, `
UPDATE prompt_entries SET queue_key = ?, action_kind = ?, owner_source = ?, thread_id = ?, project_id = ?, prompt = ?, model = ?, effort = ?, permission_mode = ?, request_hash = ?, state = ?,
result_code = ?, result_thread_id = ?, result_turn_id = ?, error_code = ?, created_at = ?, updated_at = ?
WHERE action_id = ? AND state = ?`, arguments[1:]...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 0 {
		return nil
	}
	var exists int
	err = store.db.QueryRowContext(ctx, `SELECT 1 FROM prompt_entries WHERE action_id = ?`, entry.ActionID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return promptqueue.ErrActionNotFound
	}
	if err != nil {
		return err
	}
	return promptqueue.ErrStateConflict
}

func (store *Store) Entry(ctx context.Context, actionID string) (promptqueue.Entry, error) {
	entry, err := scanEntry(store.db.QueryRowContext(ctx, promptSelect+` WHERE action_id = ?`, actionID))
	if errors.Is(err, sql.ErrNoRows) {
		return promptqueue.Entry{}, promptqueue.ErrActionNotFound
	}
	return entry, err
}

func (store *Store) NextPending(ctx context.Context, queueKey string) (promptqueue.Entry, error) {
	return store.nextPrompt(ctx, queueKey, `state IN (?, ?)`, promptqueue.StatePrepared, promptqueue.StateSentUnknown)
}

func (store *Store) NextPrepared(ctx context.Context, queueKey string) (promptqueue.Entry, error) {
	return store.nextPrompt(ctx, queueKey, `state = ?`, promptqueue.StatePrepared)
}

func (store *Store) nextPrompt(ctx context.Context, queueKey, condition string, states ...promptqueue.State) (promptqueue.Entry, error) {
	arguments := []any{queueKey}
	for _, state := range states {
		arguments = append(arguments, state)
	}
	entry, err := scanEntry(store.db.QueryRowContext(ctx, promptSelect+` WHERE queue_key = ? AND `+condition+` ORDER BY created_at, action_id LIMIT 1`, arguments...))
	if errors.Is(err, sql.ErrNoRows) {
		return promptqueue.Entry{}, promptqueue.ErrNoPreparedAction
	}
	return entry, err
}

func (store *Store) ThreadEntries(ctx context.Context, queueKey string) ([]promptqueue.Entry, error) {
	rows, err := store.db.QueryContext(ctx, promptSelect+` WHERE queue_key = ? ORDER BY created_at, action_id`, queueKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]promptqueue.Entry, 0)
	for rows.Next() {
		entry, scanErr := scanEntry(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (store *Store) PendingThreadIDs(ctx context.Context) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT DISTINCT thread_id FROM prompt_entries WHERE queue_key = thread_id AND thread_id <> '' AND state IN (?, ?) ORDER BY thread_id`, promptqueue.StatePrepared, promptqueue.StateSentUnknown)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

const promptSelect = `SELECT action_id, queue_key, action_kind, owner_source, thread_id, project_id, prompt, model, effort, permission_mode, request_hash, state,
result_code, result_thread_id, result_turn_id, error_code, created_at, updated_at FROM prompt_entries`

func promptArguments(entry promptqueue.Entry) []any {
	return []any{entry.ActionID, entry.QueueKey, entry.ActionKind, entry.OwnerSource, entry.ThreadID, entry.ProjectID, entry.Prompt, entry.Model, entry.Effort, entry.PermissionMode, entry.RequestHash,
		entry.State, entry.Result.Code, entry.Result.ThreadID, entry.Result.TurnID, entry.ErrorCode, encodeTime(entry.CreatedAt), encodeTime(entry.UpdatedAt)}
}

func scanEntry(row rowScanner) (promptqueue.Entry, error) {
	var entry promptqueue.Entry
	var createdAt, updatedAt string
	err := row.Scan(&entry.ActionID, &entry.QueueKey, &entry.ActionKind, &entry.OwnerSource, &entry.ThreadID, &entry.ProjectID, &entry.Prompt, &entry.Model, &entry.Effort,
		&entry.PermissionMode, &entry.RequestHash, &entry.State, &entry.Result.Code, &entry.Result.ThreadID, &entry.Result.TurnID, &entry.ErrorCode, &createdAt, &updatedAt)
	if err != nil {
		return promptqueue.Entry{}, err
	}
	if entry.CreatedAt, err = decodeTime(createdAt); err != nil {
		return promptqueue.Entry{}, err
	}
	entry.UpdatedAt, err = decodeTime(updatedAt)
	return entry, err
}

func (store *Store) Append(ctx context.Context, event eventjournal.Event) (eventjournal.Event, error) {
	size := len(event.Name) + len(event.Body) + 32
	if size > store.limits.MaxBytes {
		return eventjournal.Event{}, eventjournal.ErrInvalidEvent
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return eventjournal.Event{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `UPDATE event_state SET next_sequence = next_sequence + 1 WHERE singleton = 1 RETURNING next_sequence`).Scan(&event.Sequence); err != nil {
		return eventjournal.Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal_events(sequence, name, body, created_at, retained_bytes) VALUES (?, ?, ?, ?, ?)`, event.Sequence, event.Name, []byte(event.Body), encodeTime(event.CreatedAt), size); err != nil {
		return eventjournal.Event{}, err
	}
	for {
		var count, retained int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(retained_bytes), 0) FROM journal_events`).Scan(&count, &retained); err != nil {
			return eventjournal.Event{}, err
		}
		if count <= store.limits.MaxEvents && retained <= store.limits.MaxBytes {
			break
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM journal_events WHERE sequence = (SELECT MIN(sequence) FROM journal_events)`); err != nil {
			return eventjournal.Event{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return eventjournal.Event{}, err
	}
	event.Body = append([]byte(nil), event.Body...)
	return event, nil
}

func (store *Store) ReplayAfter(ctx context.Context, sequence uint64) ([]eventjournal.Event, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT sequence, name, body, created_at FROM journal_events WHERE sequence > ? ORDER BY sequence`, sequence)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]eventjournal.Event, 0)
	for rows.Next() {
		var event eventjournal.Event
		var createdAt string
		if err := rows.Scan(&event.Sequence, &event.Name, &event.Body, &createdAt); err != nil {
			return nil, err
		}
		if event.CreatedAt, err = decodeTime(createdAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (store *Store) Bounds(ctx context.Context) (eventjournal.Bounds, error) {
	var bounds eventjournal.Bounds
	err := store.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(sequence), 0), (SELECT next_sequence FROM event_state WHERE singleton = 1), COUNT(*) FROM journal_events`).Scan(&bounds.Earliest, &bounds.Latest, &bounds.Count)
	return bounds, err
}

func (store *Store) EnsureBase(ctx context.Context) (uint64, error) {
	_, err := store.db.ExecContext(ctx, `UPDATE event_state SET next_sequence = 1 WHERE singleton = 1 AND next_sequence = 0`)
	if err != nil {
		return 0, err
	}
	var sequence uint64
	err = store.db.QueryRowContext(ctx, `SELECT next_sequence FROM event_state WHERE singleton = 1`).Scan(&sequence)
	return sequence, err
}

func (store *Store) ReplaceBase(ctx context.Context) (uint64, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var sequence uint64
	if err := tx.QueryRowContext(ctx, `UPDATE event_state SET next_sequence = next_sequence + 1 WHERE singleton = 1 RETURNING next_sequence`).Scan(&sequence); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM journal_events`); err != nil {
		return 0, err
	}
	return sequence, tx.Commit()
}

func (store *Store) Acknowledge(ctx context.Context, deviceID string, sequence uint64) error {
	_, err := store.db.ExecContext(ctx, `INSERT INTO event_acks(device_id, sequence) VALUES (?, ?) ON CONFLICT(device_id) DO UPDATE SET sequence = excluded.sequence`, deviceID, sequence)
	return err
}

func (store *Store) Acknowledged(ctx context.Context, deviceID string) (uint64, error) {
	var sequence uint64
	err := store.db.QueryRowContext(ctx, `SELECT sequence FROM event_acks WHERE device_id = ?`, deviceID).Scan(&sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return sequence, err
}

func encodeTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func decodeTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

var _ pairing.Store = (*Store)(nil)
var _ promptqueue.Store = (*Store)(nil)
var _ eventjournal.Store = (*Store)(nil)
