package threads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
)

// StartExistingTurn is the follow-up fork (plan P2): the composer's
// start_turn reaches this. It mints a fresh actionId as the turn's id,
// records it against the thread, and runs the same instrumented Prepare the
// capability_request path uses, so the same question/failure/success entry
// handling applies. The preview cannot ride the start_turn reply, so a
// successful preview goes to the preview sink for the runtime to forward as
// a capability_preview frame (store_test.go:293-298).
func (store *Store) StartExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	ownerID, err := store.ownerOf(ctx, taskID)
	if err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}

	turnID, err := newRandomID("turn-")
	if err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO turn_threads (turn_id, thread_id) VALUES (?, ?)`, turnID, taskID); err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}

	preview, err := store.prepare(ctx, ownerID, turnID, prompt)
	if err != nil {
		var question *capabilityflow.QuestionError
		if errors.As(err, &question) {
			// A router question is a conversation beat, not a failure: it
			// already persisted as an agent entry inside prepare(). The turn
			// itself succeeds, with no preview to deliver.
			return taskadapter.ExistingTaskResult{ThreadID: taskID, TurnID: turnID}, nil
		}
		return taskadapter.ExistingTaskResult{}, err
	}
	store.deliverPreview(taskID, preview)
	return taskadapter.ExistingTaskResult{ThreadID: taskID, TurnID: turnID}, nil
}

func (store *Store) ownerOf(ctx context.Context, taskID string) (string, error) {
	var ownerID string
	err := store.db.QueryRowContext(ctx, `SELECT owner_id FROM threads WHERE id = ?`, taskID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", ErrUnknownThread, taskID)
	}
	return ownerID, err
}

// RedirectExistingTurn: a capability thread has no long-running turn to
// redirect (store_test.go:132-134's CanRedirect: false), so this always
// answers with the sentinel the handler's own code mapping expects
// (handler.go:1475).
func (store *Store) RedirectExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{}, taskadapter.ErrRedirectUnsupported
}

// InterruptExistingTurn: a capability thread never has a turn in flight to
// stop -- Prepare/Confirm both run to completion synchronously -- so this
// reports "not busy", the same sentinel a desktop task uses when asked to
// interrupt an idle turn (handler.go:1473).
func (store *Store) InterruptExistingTurn(context.Context, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{}, taskadapter.ErrTaskNotBusy
}
