package threads

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
)

// Flow is the capability flow this package wraps. It matches
// mobilesession.CapabilityFlow's method set structurally (handler.go:51-56)
// so this package never has to import mobilesession, per store_test.go's
// package doc.
type Flow interface {
	Prepare(context.Context, string, string, string) (capabilityflow.Preview, error)
	Confirm(context.Context, string, string, string) (capabilityadapter.Outcome, error)
	Cancel(string, string, string) error
	Disconnect(context.Context, string) error
}

// wrappedFlow is what WrapFlow hands back: a CapabilityFlow-shaped adapter
// that funnels every call back through the store so entries land in the
// right thread. It carries no state of its own -- store.flow is what
// StartExistingTurn also reuses for the follow-up fork.
type wrappedFlow struct {
	store *Store
}

// WrapFlow observes Prepare/Confirm on inner and appends the resulting
// transcript entries. The store keeps inner so StartExistingTurn's
// follow-up fork can drive the exact same instrumented path.
func (store *Store) WrapFlow(inner Flow) *wrappedFlow {
	store.mu.Lock()
	store.flow = inner
	store.mu.Unlock()
	return &wrappedFlow{store: store}
}

func (store *Store) flowRef() Flow {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.flow
}

func (w *wrappedFlow) Prepare(ctx context.Context, ownerID, requestID, utterance string) (capabilityflow.Preview, error) {
	return w.store.prepare(ctx, ownerID, requestID, utterance)
}

func (w *wrappedFlow) Confirm(ctx context.Context, ownerID, requestID, fingerprint string) (capabilityadapter.Outcome, error) {
	return w.store.confirm(ctx, ownerID, requestID, fingerprint)
}

func (w *wrappedFlow) Cancel(ownerID, requestID, reason string) error {
	return w.store.flowRef().Cancel(ownerID, requestID, reason)
}

func (w *wrappedFlow) Disconnect(ctx context.Context, ownerID string) error {
	return w.store.flowRef().Disconnect(ctx, ownerID)
}

// SetPreviewSink registers where a follow-up turn's preview goes once it's
// ready. StartExistingTurn's reply cannot ride the start_turn response
// itself, so the store hands it here for the runtime to forward as a
// capability_preview frame (store_test.go:293-298).
func (store *Store) SetPreviewSink(sink func(threadID string, preview capabilityflow.Preview)) {
	store.mu.Lock()
	store.previewSink = sink
	store.mu.Unlock()
}

func (store *Store) deliverPreview(threadID string, preview capabilityflow.Preview) {
	store.mu.Lock()
	sink := store.previewSink
	store.mu.Unlock()
	if sink != nil {
		sink(threadID, preview)
	}
}

// prepare is the instrumented Prepare both the wrapped flow and
// StartExistingTurn's follow-up fork run through. requestID resolves to an
// existing thread when StartExistingTurn minted it as a follow-up turn;
// otherwise requestID IS the thread id (the base case: the phone's own
// actionId becomes the thread's id, per the plan's taskId linkage).
func (store *Store) prepare(ctx context.Context, ownerID, requestID, utterance string) (capabilityflow.Preview, error) {
	threadID, err := store.resolveThreadID(ctx, requestID)
	if err != nil {
		return capabilityflow.Preview{}, err
	}
	if err := store.ensureThread(ctx, threadID, ownerID, utterance); err != nil {
		return capabilityflow.Preview{}, err
	}
	if err := store.appendEntry(threadID, requestID, tasktranscript.KindUser, utterance, "", time.Time{}); err != nil {
		return capabilityflow.Preview{}, err
	}

	preview, err := store.flowRef().Prepare(ctx, ownerID, requestID, utterance)
	if err != nil {
		var question *capabilityflow.QuestionError
		if errors.As(err, &question) {
			// A router question is a conversation beat, not a failure: the
			// app reads it from the transcript (plan protocol point 4).
			_ = store.appendEntry(threadID, requestID, tasktranscript.KindAgent, question.Question, "", time.Time{})
		} else {
			// The real cause (stage1 socket errors, adapter internals) never
			// reaches the user-visible transcript; the caller still gets the
			// original error for its own code mapping.
			store.logger.Error("[threads] prepare failed", "thread_id", threadID, "request_id", requestID, "error", err)
			_ = store.appendEntry(threadID, requestID, tasktranscript.KindAgent, "I couldn't handle that request. Please try again.", "", time.Time{})
		}
		store.touch(threadID)
		return preview, err
	}
	store.touch(threadID)
	return preview, nil
}

// confirm is the instrumented Confirm both the wrapped flow and
// StartExistingTurn's follow-up fork (via its own Confirm call) run through.
func (store *Store) confirm(ctx context.Context, ownerID, requestID, fingerprint string) (capabilityadapter.Outcome, error) {
	threadID, err := store.resolveThreadID(ctx, requestID)
	if err != nil {
		return capabilityadapter.Outcome{}, err
	}
	outcome, err := store.flowRef().Confirm(ctx, ownerID, requestID, fingerprint)
	if err != nil {
		store.logger.Error("[threads] confirm failed", "thread_id", threadID, "request_id", requestID, "error", err)
		_ = store.appendEntry(threadID, requestID, tasktranscript.KindAgent, "That request didn't complete. Please try again.", "", time.Time{})
		store.touch(threadID)
		return outcome, err
	}

	// Message rows first (raw received DMs), then the human detail sentence,
	// matching what a texting app shows: the messages themselves, then the
	// summary line (store_test.go:216-249). A message whose timestamp
	// couldn't be parsed can never become a wire-valid row
	// (contract/validation.go:887 requires a valid sentAt), so it rides only
	// in the detail sentence.
	for _, message := range outcome.Messages {
		if message.SentAt.IsZero() {
			continue
		}
		sender := sanitizeLine(message.Sender, 256)
		if sender == "" {
			continue
		}
		_ = store.appendEntry(threadID, requestID, tasktranscript.KindMessage, message.Text, sender, message.SentAt)
	}
	if strings.TrimSpace(outcome.Detail) != "" {
		_ = store.appendEntry(threadID, requestID, tasktranscript.KindAgent, outcome.Detail, "", time.Time{})
	}
	store.touch(threadID)
	return outcome, nil
}

func (store *Store) ensureThread(ctx context.Context, threadID, ownerID, utterance string) error {
	_, err := store.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO threads (id, owner_id, title, updated_at_unix) VALUES (?, ?, ?, ?)`,
		threadID, ownerID, deriveTitle(utterance), store.now().Unix())
	return err
}

func (store *Store) touch(threadID string) {
	if _, err := store.db.Exec(`UPDATE threads SET updated_at_unix = ? WHERE id = ?`, store.now().Unix(), threadID); err != nil {
		store.logger.Error("[threads] touch failed", "thread_id", threadID, "error", err)
	}
}

// resolveThreadID maps a request id to the thread it belongs to. A request
// id registered by StartExistingTurn (turn_threads) belongs to that thread;
// any other request id is itself the thread's id -- the base case, where the
// phone's own actionId becomes the thread id.
func (store *Store) resolveThreadID(ctx context.Context, requestID string) (string, error) {
	var threadID string
	err := store.db.QueryRowContext(ctx, `SELECT thread_id FROM turn_threads WHERE turn_id = ?`, requestID).Scan(&threadID)
	switch {
	case err == nil:
		return threadID, nil
	case errors.Is(err, sql.ErrNoRows):
		return requestID, nil
	default:
		return "", err
	}
}

// appendEntry stores one transcript row. A blank text would fail the wire's
// boundedString check (contract/validation.go:879,886, length >= 1), so it
// is silently dropped rather than persisted as an invalid entry.
func (store *Store) appendEntry(threadID, turnID string, kind tasktranscript.Kind, text, sender string, sentAt time.Time) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	id, err := newRandomID("entry-")
	if err != nil {
		return err
	}
	sentAtText := ""
	if !sentAt.IsZero() {
		sentAtText = sentAt.UTC().Format(time.RFC3339)
	}
	_, err = store.db.Exec(
		`INSERT INTO thread_entries (entry_id, thread_id, turn_id, kind, text, sender, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, threadID, turnID, string(kind), text, sender, sentAtText)
	return err
}
