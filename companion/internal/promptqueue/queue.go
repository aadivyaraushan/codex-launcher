package promptqueue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrActionNotFound     = errors.New("queued action was not found")
	ErrDuplicateAction    = errors.New("queued action ID already exists")
	ErrInvalidEntry       = errors.New("queued prompt is invalid")
	ErrNoPreparedAction   = errors.New("no queued prompt is ready")
	ErrOutcomeUnknown     = errors.New("queued prompt outcome is unknown")
	ErrResultUnavailable  = errors.New("queued prompt result is unavailable")
	ErrStateConflict      = errors.New("queued prompt state changed concurrently")
	ErrDispatchInProgress = errors.New("queued prompt is already being dispatched")
	ErrSendNotSent        = errors.New("prompt was not sent")
	ErrSendOutcomeUnknown = errors.New("prompt send outcome is unknown")
)

type State string

const (
	StatePrepared    State = "PREPARED"
	StateSentUnknown State = "SENT_UNKNOWN"
	StateConfirmed   State = "CONFIRMED"
	StateCanceled    State = "CANCELED"
)

type Entry struct {
	ActionID  string
	ThreadID  string
	ProjectID string
	Prompt    string
	State     State
	Result    Result
	ErrorCode string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Result struct {
	Code     string
	ThreadID string
	TurnID   string
}

type Sender func(context.Context, Entry) (Result, error)
type Reconciler func(context.Context, Entry) (Result, bool, error)

type Queue struct {
	store  Store
	logger *slog.Logger
}

func New(store Store, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	return &Queue{store: store, logger: logger}
}

func (queue *Queue) Enqueue(ctx context.Context, entry Entry) error {
	if queue == nil || queue.store == nil || !validEntry(entry) {
		return ErrInvalidEntry
	}
	entry.State = StatePrepared
	entry.UpdatedAt = entry.CreatedAt
	if err := queue.store.Create(ctx, entry); err != nil {
		return fmt.Errorf("store prepared prompt: %w", err)
	}
	queue.logger.Info("[prompt-queue] prompt prepared", "action_id", entry.ActionID, "thread_id", entry.ThreadID, "project_id", entry.ProjectID)
	return nil
}

func (queue *Queue) DispatchNext(ctx context.Context, threadID string, sender Sender, reconcile Reconciler, now time.Time) (Result, error) {
	if queue == nil || queue.store == nil || !validID(threadID) {
		return Result{}, ErrInvalidEntry
	}
	entry, err := queue.store.NextPending(ctx, threadID)
	if err != nil {
		return Result{}, err
	}
	if entry.State == StateSentUnknown {
		return queue.reconcile(ctx, entry, reconcile, now)
	}
	entry.State = StateSentUnknown
	entry.UpdatedAt = now
	if err := queue.store.CompareAndSwap(ctx, StatePrepared, entry); err != nil {
		queue.logger.Error("[prompt-queue] pre-send state failed", "action_id", entry.ActionID, "thread_id", threadID, "error", err)
		if errors.Is(err, ErrStateConflict) {
			return Result{}, ErrDispatchInProgress
		}
		return Result{}, fmt.Errorf("store sent-unknown before send: %w", err)
	}
	if sender == nil {
		return Result{}, ErrOutcomeUnknown
	}
	queue.logger.Info("[prompt-queue] sending prompt", "action_id", entry.ActionID, "thread_id", threadID, "branch_reason", "prepared_state_committed")
	result, sendErr := sender(ctx, entry)
	if sendErr != nil {
		if errors.Is(sendErr, ErrSendNotSent) {
			entry.State = StatePrepared
			entry.UpdatedAt = now
			if saveErr := queue.store.CompareAndSwap(ctx, StateSentUnknown, entry); saveErr != nil {
				return Result{}, fmt.Errorf("restore safe prepared state: %w", saveErr)
			}
			return Result{}, sendErr
		}
		queue.logger.Warn("[prompt-queue] send outcome unknown", "action_id", entry.ActionID, "thread_id", threadID, "error_class", callbackErrorClass(sendErr))
		return Result{}, ErrOutcomeUnknown
	}
	if !validResult(result, entry.ThreadID) {
		return Result{}, ErrOutcomeUnknown
	}
	entry.State = StateConfirmed
	entry.Result = result
	entry.Prompt = ""
	entry.UpdatedAt = now
	if err := queue.store.CompareAndSwap(ctx, StateSentUnknown, entry); err != nil {
		queue.logger.Error("[prompt-queue] confirmed result commit failed", "action_id", entry.ActionID, "thread_id", threadID, "error", err)
		return Result{}, ErrOutcomeUnknown
	}
	queue.logger.Info("[prompt-queue] prompt confirmed", "action_id", entry.ActionID, "thread_id", threadID, "turn_id", result.TurnID)
	return result, nil
}

func (queue *Queue) reconcile(ctx context.Context, entry Entry, reconcile Reconciler, now time.Time) (Result, error) {
	if reconcile == nil {
		return Result{}, ErrOutcomeUnknown
	}
	result, confirmed, err := reconcile(ctx, entry)
	if err != nil || !confirmed || !validResult(result, entry.ThreadID) {
		queue.logger.Warn("[prompt-queue] outcome remains unknown", "action_id", entry.ActionID, "thread_id", entry.ThreadID, "branch_reason", "reconciliation_unproven", "error_class", callbackErrorClass(err))
		return Result{}, ErrOutcomeUnknown
	}
	entry.State = StateConfirmed
	entry.Result = result
	entry.Prompt = ""
	entry.UpdatedAt = now
	if err := queue.store.CompareAndSwap(ctx, StateSentUnknown, entry); err != nil {
		return Result{}, ErrOutcomeUnknown
	}
	queue.logger.Info("[prompt-queue] outcome reconciled", "action_id", entry.ActionID, "thread_id", entry.ThreadID, "turn_id", result.TurnID)
	return result, nil
}

func (queue *Queue) Result(ctx context.Context, actionID string) (Result, error) {
	entry, err := queue.store.Entry(ctx, actionID)
	if err != nil {
		return Result{}, err
	}
	if entry.State != StateConfirmed {
		return Result{}, ErrResultUnavailable
	}
	return entry.Result, nil
}

func (queue *Queue) CancelThread(ctx context.Context, threadID, errorCode string, now time.Time) error {
	if !validID(threadID) || !validID(errorCode) {
		return ErrInvalidEntry
	}
	entries, err := queue.store.ThreadEntries(ctx, threadID)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.State != StatePrepared {
			continue
		}
		entry.State = StateCanceled
		entry.ErrorCode = errorCode
		entry.Prompt = ""
		entry.UpdatedAt = now
		if err := queue.store.CompareAndSwap(ctx, StatePrepared, entry); err != nil {
			if errors.Is(err, ErrStateConflict) {
				continue
			}
			return fmt.Errorf("cancel queued prompt %s: %w", entry.ActionID, err)
		}
	}
	queue.logger.Info("[prompt-queue] thread queue canceled", "thread_id", threadID, "error_code", errorCode)
	return nil
}

func validEntry(entry Entry) bool {
	return validID(entry.ActionID) && validID(entry.ThreadID) && validID(entry.ProjectID) && strings.TrimSpace(entry.Prompt) != "" && len(entry.Prompt) <= 128*1024 && !entry.CreatedAt.IsZero()
}

func validResult(result Result, threadID string) bool {
	return validID(result.Code) && result.ThreadID == threadID && validID(result.TurnID)
}

func validID(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 256
}

func callbackErrorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, ErrSendNotSent):
		return "not_sent"
	case errors.Is(err, ErrSendOutcomeUnknown):
		return "outcome_unknown"
	default:
		return "external_error"
	}
}
