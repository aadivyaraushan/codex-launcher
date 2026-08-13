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
	ErrSendDeferred       = errors.New("prompt remains queued because the task is busy")
	ErrSendOutcomeUnknown = errors.New("prompt send outcome is unknown")
)

type State string

const (
	StatePrepared    State = "PREPARED"
	StateSentUnknown State = "SENT_UNKNOWN"
	StateConfirmed   State = "CONFIRMED"
	StateFailed      State = "FAILED"
	StateCanceled    State = "CANCELED"
)

type Entry struct {
	ActionID       string
	QueueKey       string
	ActionKind     string
	OwnerSource    string
	ThreadID       string
	ProjectID      string
	Prompt         string
	Model          string
	Effort         string
	PermissionMode string
	RequestHash    string
	DeviceID       string
	AttachmentIDs  []string
	State          State
	Result         Result
	ErrorCode      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
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

type QueueStatus string

type PendingThread struct {
	ID          string
	OwnerSource string
}

type AttachmentClaim struct {
	DeviceID      string
	AttachmentIDs []string
}

const (
	QueueEmpty          QueueStatus = "none"
	QueueWaiting        QueueStatus = "queued"
	QueueOutcomeUnknown QueueStatus = "outcome_unknown"
)

func New(store Store, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	return &Queue{store: store, logger: logger}
}

func (queue *Queue) Enqueue(ctx context.Context, entry Entry) error {
	if entry.QueueKey == "" {
		entry.QueueKey = entry.ThreadID
	}
	if entry.ActionKind == "" {
		entry.ActionKind = "start_turn"
	}
	if queue == nil || queue.store == nil || !validEntry(entry) {
		return ErrInvalidEntry
	}
	entry.State = StatePrepared
	entry.UpdatedAt = entry.CreatedAt
	if err := queue.store.Create(ctx, entry); err != nil {
		return fmt.Errorf("store prepared prompt: %w", err)
	}
	queue.logger.Info("[prompt-queue] prompt prepared", "action_id", entry.ActionID, "queue_key", entry.QueueKey, "thread_id", entry.ThreadID, "project_id", entry.ProjectID)
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
		if errors.Is(sendErr, ErrSendDeferred) {
			entry.State = StatePrepared
			entry.UpdatedAt = now
			if saveErr := queue.store.CompareAndSwap(ctx, StateSentUnknown, entry); saveErr != nil {
				return Result{}, errors.Join(ErrOutcomeUnknown, fmt.Errorf("restore deferred prompt: %w", saveErr))
			}
			queue.logger.Info("[prompt-queue] prompt remains queued", "action_id", entry.ActionID, "thread_id", threadID, "branch_reason", "task_busy")
			return Result{}, ErrSendDeferred
		}
		if errors.Is(sendErr, ErrSendNotSent) {
			entry.State = StateFailed
			entry.ErrorCode = "send_not_sent"
			entry.Prompt = ""
			entry.UpdatedAt = now
			if saveErr := queue.store.CompareAndSwap(ctx, StateSentUnknown, entry); saveErr != nil {
				return Result{}, errors.Join(ErrOutcomeUnknown, fmt.Errorf("store definite send failure: %w", saveErr))
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

func (queue *Queue) Entry(ctx context.Context, actionID string) (Entry, error) {
	if queue == nil || queue.store == nil || !validID(actionID) {
		return Entry{}, ErrInvalidEntry
	}
	return queue.store.Entry(ctx, actionID)
}

func (queue *Queue) Entries(ctx context.Context, queueKey string) ([]Entry, error) {
	if queue == nil || queue.store == nil || !validID(queueKey) {
		return nil, ErrInvalidEntry
	}
	return queue.store.ThreadEntries(ctx, queueKey)
}

func (queue *Queue) Status(ctx context.Context, queueKey string) (QueueStatus, error) {
	if queue == nil || queue.store == nil || !validID(queueKey) {
		return QueueEmpty, ErrInvalidEntry
	}
	entries, err := queue.store.ThreadEntries(ctx, queueKey)
	if err != nil {
		return QueueEmpty, err
	}
	status := QueueEmpty
	for _, entry := range entries {
		switch entry.State {
		case StateSentUnknown:
			return QueueOutcomeUnknown, nil
		case StatePrepared:
			status = QueueWaiting
		}
	}
	return status, nil
}

func (queue *Queue) PendingThreadIDs(ctx context.Context) ([]string, error) {
	if queue == nil || queue.store == nil {
		return nil, ErrInvalidEntry
	}
	ids, err := queue.store.PendingThreadIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !validID(id) {
			return nil, ErrInvalidEntry
		}
	}
	return ids, nil
}

func (queue *Queue) AttachmentClaims(ctx context.Context) (map[string]AttachmentClaim, error) {
	if queue == nil || queue.store == nil {
		return nil, ErrInvalidEntry
	}
	entries, err := queue.store.PendingEntries(ctx)
	if err != nil {
		return nil, err
	}
	claims := make(map[string]AttachmentClaim)
	for _, entry := range entries {
		if len(entry.AttachmentIDs) == 0 {
			continue
		}
		if !validAttachmentOwnership(entry.ActionKind, entry.DeviceID, entry.AttachmentIDs) {
			return nil, ErrInvalidEntry
		}
		claims[entry.ActionID] = AttachmentClaim{DeviceID: entry.DeviceID, AttachmentIDs: append([]string(nil), entry.AttachmentIDs...)}
	}
	return claims, nil
}

func (queue *Queue) PendingThreads(ctx context.Context) ([]PendingThread, error) {
	ids, err := queue.PendingThreadIDs(ctx)
	if err != nil {
		return nil, err
	}
	threads := make([]PendingThread, 0, len(ids))
	for _, id := range ids {
		entries, readErr := queue.store.ThreadEntries(ctx, id)
		if readErr != nil {
			return nil, readErr
		}
		source := ""
		for _, entry := range entries {
			if entry.State != StatePrepared && entry.State != StateSentUnknown {
				continue
			}
			if source != "" && entry.OwnerSource != "" && source != entry.OwnerSource {
				return nil, ErrInvalidEntry
			}
			if entry.OwnerSource != "" {
				source = entry.OwnerSource
			}
		}
		threads = append(threads, PendingThread{ID: id, OwnerSource: source})
	}
	return threads, nil
}

func (queue *Queue) DismissUnknown(ctx context.Context, actionID, threadID, deviceID string, now time.Time) (Entry, error) {
	if queue == nil || queue.store == nil || !validID(actionID) || threadID != "" && !validID(threadID) || !validID(deviceID) || now.IsZero() {
		return Entry{}, ErrInvalidEntry
	}
	entry, err := queue.store.Entry(ctx, actionID)
	if errors.Is(err, ErrActionNotFound) {
		return Entry{}, nil
	}
	if err != nil {
		return Entry{}, err
	}
	validOwner := entry.ThreadID == threadID
	if threadID == "" {
		validOwner = entry.ThreadID == "" && entry.QueueKey == "new:"+entry.ActionID
	}
	if !validOwner || entry.DeviceID != "" && entry.DeviceID != deviceID || entry.ActionKind != "start_turn" && entry.ActionKind != "steer_turn" && entry.ActionKind != "interrupt_turn" {
		return Entry{}, ErrInvalidEntry
	}
	switch entry.State {
	case StatePrepared, StateSentUnknown:
		previous := entry.State
		entry.State = StateCanceled
		entry.Prompt = ""
		entry.ErrorCode = "user_reviewed"
		entry.UpdatedAt = now
		if err := queue.store.CompareAndSwap(ctx, previous, entry); err != nil {
			return Entry{}, err
		}
		queue.logger.Info("[prompt-queue] unknown action review cleared", "action_id", actionID, "thread_id", threadID, "decision", "cancel_without_retry")
		return entry, nil
	case StateConfirmed, StateFailed, StateCanceled:
		return entry, nil
	default:
		return Entry{}, ErrInvalidEntry
	}
}

func (queue *Queue) CancelThread(ctx context.Context, threadID, errorCode string, now time.Time) error {
	return queue.cancelThread(ctx, threadID, errorCode, now, false)
}

func (queue *Queue) CancelUnavailableThread(ctx context.Context, threadID, errorCode string, now time.Time) error {
	return queue.cancelThread(ctx, threadID, errorCode, now, true)
}

func (queue *Queue) cancelThread(ctx context.Context, threadID, errorCode string, now time.Time, includeUnknown bool) error {
	if !validID(threadID) || !validID(errorCode) {
		return ErrInvalidEntry
	}
	entries, err := queue.store.ThreadEntries(ctx, threadID)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.State != StatePrepared && (!includeUnknown || entry.State != StateSentUnknown) {
			continue
		}
		previous := entry.State
		entry.State = StateCanceled
		entry.ErrorCode = errorCode
		entry.Prompt = ""
		entry.UpdatedAt = now
		if err := queue.store.CompareAndSwap(ctx, previous, entry); err != nil {
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
	return validID(entry.ActionID) && validID(entry.QueueKey) && (entry.ThreadID == "" || validID(entry.ThreadID)) &&
		((entry.ThreadID == "" && validID(entry.ProjectID)) || (entry.ThreadID != "" && validOptionalID(entry.ProjectID))) &&
		validOptionalID(entry.Model) && validOptionalID(entry.Effort) && validOptionalID(entry.PermissionMode) &&
		validOptionalID(entry.RequestHash) && validOwnerSource(entry.OwnerSource) &&
		validAttachmentOwnership(entry.ActionKind, entry.DeviceID, entry.AttachmentIDs) &&
		validActionPayload(entry.ActionKind, entry.Prompt) && !entry.CreatedAt.IsZero()
}

func validAttachmentOwnership(kind, deviceID string, attachmentIDs []string) bool {
	if len(attachmentIDs) == 0 {
		return deviceID == ""
	}
	if kind == "interrupt_turn" || !validID(deviceID) || len(attachmentIDs) > 16 {
		return false
	}
	seen := make(map[string]struct{}, len(attachmentIDs))
	for _, id := range attachmentIDs {
		if !validID(id) {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validOwnerSource(source string) bool {
	return source == "" || source == "app_server" || source == "desktop"
}

func validActionPayload(kind, prompt string) bool {
	switch kind {
	case "start_turn", "steer_turn":
		return strings.TrimSpace(prompt) != "" && len(prompt) <= 128*1024
	case "interrupt_turn":
		return prompt == ""
	default:
		return false
	}
}

func validResult(result Result, _ string) bool {
	// Thread id on the result may differ from the queued row: new tasks learn
	// the created id here, and Home compose starts against a virtual inbox
	// then returns the new chat id.
	return validID(result.Code) && validID(result.ThreadID) && validID(result.TurnID)
}

func validOptionalID(value string) bool { return value == "" || validID(value) }

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
	case errors.Is(err, ErrSendDeferred):
		return "deferred"
	case errors.Is(err, ErrSendOutcomeUnknown):
		return "outcome_unknown"
	default:
		return "external_error"
	}
}
