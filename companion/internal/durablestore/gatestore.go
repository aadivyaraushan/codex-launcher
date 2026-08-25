package durablestore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// MarkRecipientMessaged records that adapter has successfully messaged
// recipient at least once. Idempotent: marking an already-known recipient
// again is not an error.
func (store *Store) MarkRecipientMessaged(ctx context.Context, adapter, recipient string) error {
	_, err := store.db.ExecContext(ctx, `
INSERT OR IGNORE INTO gate_known_recipients(adapter, recipient, first_messaged_at) VALUES (?, ?, ?)`,
		adapter, recipient, encodeTime(time.Now()))
	return err
}

// KnownRecipient reports whether adapter has ever successfully messaged
// recipient.
func (store *Store) KnownRecipient(ctx context.Context, adapter, recipient string) (bool, error) {
	var exists int
	err := store.db.QueryRowContext(ctx, `SELECT 1 FROM gate_known_recipients WHERE adapter = ? AND recipient = ?`, adapter, recipient).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RecordGateDenial durably records that gateID was denied, so a denied
// gate can never release even across restarts. Idempotent.
func (store *Store) RecordGateDenial(ctx context.Context, gateID string) error {
	_, err := store.db.ExecContext(ctx, `
INSERT OR IGNORE INTO gate_denials(gate_id, denied_at) VALUES (?, ?)`,
		gateID, encodeTime(time.Now()))
	return err
}

// GateDenied reports whether gateID was ever denied.
func (store *Store) GateDenied(ctx context.Context, gateID string) (bool, error) {
	var exists int
	err := store.db.QueryRowContext(ctx, `SELECT 1 FROM gate_denials WHERE gate_id = ?`, gateID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
