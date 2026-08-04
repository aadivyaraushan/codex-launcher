package mobilesession

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/codex-launcher/codex-launcher/companion/internal/attachments"
	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/devicework"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskoptions"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

type TaskSource interface {
	ListRecent(context.Context, int) ([]taskstate.Task, error)
}

type CapabilityFlow interface {
	Prepare(context.Context, string, string, string) (capabilityflow.Preview, error)
	Confirm(context.Context, string, string, string) (capabilityadapter.Outcome, error)
	Cancel(string, string, string) error
	Disconnect(context.Context, string) error
}

type TaskTranscriptSource interface {
	ReadTranscript(context.Context, string, tasktranscript.PageOptions) (tasktranscript.Page, error)
}

type NewTaskOptionsSource interface {
	NewTaskOptions(context.Context) (taskoptions.Catalog, error)
}

type NewTaskSource interface {
	StartNewTask(context.Context, taskadapter.NewTaskRequest) (taskadapter.NewTaskResult, error)
}

type ExistingTaskSource interface {
	CurrentTask(context.Context, string) (taskstate.Task, error)
	StartExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error)
	RedirectExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error)
	InterruptExistingTurn(context.Context, string) (taskadapter.ExistingTaskResult, error)
}

type ExistingTaskRecoverySource interface {
	CurrentTaskFromSource(context.Context, string, taskstate.Source) (taskstate.Task, error)
}

type ExistingTaskRecoveryControlSource interface {
	StartExistingTurnFromSource(context.Context, string, string, taskstate.Source) (taskadapter.ExistingTaskResult, error)
}

type ExistingTaskAttachmentSource interface {
	StartExistingTurnWithAttachments(context.Context, string, string, []taskadapter.AttachmentInput) (taskadapter.ExistingTaskResult, error)
	RedirectExistingTurnWithAttachments(context.Context, string, string, []taskadapter.AttachmentInput) (taskadapter.ExistingTaskResult, error)
}

type ExistingTaskRecoveryAttachmentSource interface {
	StartExistingTurnFromSourceWithAttachments(context.Context, string, string, taskstate.Source, []taskadapter.AttachmentInput) (taskadapter.ExistingTaskResult, error)
}

type TaskManagementSource interface {
	Rename(context.Context, string, string) error

	Archive(context.Context, string) error

	ForkToAppServer(context.Context, string) (taskstate.Task, error)
}

var (
	ErrMissingDependency             = errors.New("mobile session dependency is missing")
	ErrUnsupportedMessage            = errors.New("mobile session message is unsupported")
	ErrSessionSuperseded             = errors.New("mobile session was replaced")
	ErrInvalidTaskEvent              = errors.New("mobile task event is invalid")
	ErrUnknownTaskEvent              = errors.New("mobile task event references an unknown task")
	ErrTaskEventAuthorizationRevoked = errors.New("Desktop task event authorization was revoked")
)

type Handler struct {
	ctx                context.Context
	computerName       string
	projects           *projects.Service
	journal            *eventjournal.Journal
	logger             *slog.Logger
	now                func() time.Time
	taskCapable        bool
	attachmentCapable  bool
	attachmentStore    *attachments.Store
	taskSource         TaskSource
	transcriptSource   TaskTranscriptSource
	managementSource   TaskManagementSource
	optionSource       NewTaskOptionsSource
	newTaskSource      NewTaskSource
	existingTaskSource ExistingTaskSource
	promptQueue        *promptqueue.Queue
	decisionRouter     *decisions.Router
	capabilityFlow     CapabilityFlow
	deviceWork         *devicework.Ledger
	nextID             atomic.Uint64
	publishMu          sync.Mutex
	mu                 sync.Mutex
	active             map[string]transport.MessageSender
	activeView         atomic.Value
	snapshotGen        atomic.Uint64
	broadcasts         chan outboundBroadcast
	sendTimeout        time.Duration
	taskRefreshWait    func(context.Context, int) bool
}

const (
	broadcastQueueSize     = 128
	defaultSendTimeout     = 2 * time.Second
	maxTaskRefreshAttempts = 3

	// DeviceWorkTimeout is how long the Mac waits for a phone to answer a
	// device_action before giving up on it. A notification reply either lands
	// within seconds or it is not going to land at all, and the cost of
	// waiting longer than that is not patience — it is the user's next
	// prompt sitting blocked behind a reply that was never coming.
	DeviceWorkTimeout = 60 * time.Second
)

type outboundBroadcast struct {
	messageType string
	sequence    uint64
	body        json.RawMessage
	recipients  []transport.MessageSender
}

type snapshotState struct {
	ComputerName string            `json:"computerName"`
	Projects     []projects.Choice `json:"projects"`
	Tasks        []snapshotTask    `json:"tasks"`
}

type snapshotTask struct {
	TaskID         string `json:"taskId"`
	Title          string `json:"title"`
	ProjectLabel   string `json:"projectLabel"`
	State          string `json:"state"`
	ActiveTurnID   string `json:"activeTurnId,omitempty"`
	CanRedirect    bool   `json:"canRedirect"`
	QueueState     string `json:"queueState"`
	LastActivityAt string `json:"lastActivityAt"`
}

func New(ctx context.Context, computerName string, projectService *projects.Service, journal *eventjournal.Journal, now func() time.Time) (*Handler, error) {
	return NewWithLogger(ctx, computerName, projectService, journal, nil, now)
}

func NewWithLogger(ctx context.Context, computerName string, projectService *projects.Service, journal *eventjournal.Journal, logger *slog.Logger, now func() time.Time) (*Handler, error) {
	return NewWithTaskSource(ctx, computerName, projectService, journal, nil, logger, now)
}

func NewWithTaskSource(ctx context.Context, computerName string, projectService *projects.Service, journal *eventjournal.Journal, taskSource TaskSource, logger *slog.Logger, now func() time.Time) (*Handler, error) {
	return NewWithTaskSourceAndQueue(ctx, computerName, projectService, journal, taskSource, nil, logger, now)
}

func NewWithTaskSourceAndQueue(ctx context.Context, computerName string, projectService *projects.Service, journal *eventjournal.Journal, taskSource TaskSource, promptQueue *promptqueue.Queue, logger *slog.Logger, now func() time.Time) (*Handler, error) {
	return NewWithTaskSourceQueueAndAttachments(ctx, computerName, projectService, journal, taskSource, promptQueue, nil, logger, now)
}

func NewWithTaskSourceQueueAndAttachments(
	ctx context.Context,
	computerName string,
	projectService *projects.Service,
	journal *eventjournal.Journal,
	taskSource TaskSource,
	promptQueue *promptqueue.Queue,
	attachmentStore *attachments.Store,
	logger *slog.Logger,
	now func() time.Time,
) (*Handler, error) {
	if projectService == nil || journal == nil || computerName == "" {
		return nil, ErrMissingDependency
	}
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	tasks, err := loadSnapshotTasks(ctx, taskSource, promptQueue)
	if err != nil {
		logger.Error("[mobile-session] task snapshot unavailable", "branch_reason", "unsafe_or_unavailable_catalog", "error_class", fmt.Sprintf("%T", err))
		return nil, err
	}
	initialState := snapshotState{ComputerName: computerName, Projects: projectService.List(), Tasks: tasks}
	if _, err := validatedSnapshotBody(1, initialState); err != nil {
		logger.Error("[mobile-session] initial snapshot rejected", "branch_reason", "invalid_safe_projection", "error_class", fmt.Sprintf("%T", err))
		return nil, err
	}
	state, err := json.Marshal(initialState)
	if err != nil {
		return nil, err
	}
	if _, err := journal.InitializeSnapshot(ctx, state, now()); err != nil {
		return nil, fmt.Errorf("initialize mobile snapshot: %w", err)
	}
	logger.Info("[mobile-session] initial task snapshot ready", "task_count", len(tasks), "output_shape", "safe_task_summaries")
	handler := &Handler{
		ctx:          ctx,
		computerName: computerName, projects: projectService, journal: journal, logger: logger, now: now, taskCapable: taskSource != nil, taskSource: taskSource,
		active: make(map[string]transport.MessageSender), broadcasts: make(chan outboundBroadcast, broadcastQueueSize), sendTimeout: defaultSendTimeout,
		taskRefreshWait: waitForTaskRefreshRetry,
	}
	handler.deviceWork = devicework.NewLedger(DeviceWorkTimeout, handler.now)
	handler.transcriptSource, _ = taskSource.(TaskTranscriptSource)
	handler.managementSource, _ = taskSource.(TaskManagementSource)
	handler.optionSource, _ = taskSource.(NewTaskOptionsSource)
	handler.newTaskSource, _ = taskSource.(NewTaskSource)
	handler.existingTaskSource, _ = taskSource.(ExistingTaskSource)
	handler.promptQueue = promptQueue
	if attachmentStore != nil {
		claims := map[string]attachments.Claim{}
		if promptQueue != nil {
			active, claimErr := promptQueue.AttachmentClaims(ctx)
			if claimErr != nil {
				return nil, fmt.Errorf("load durable attachment claims: %w", claimErr)
			}
			for actionID, claim := range active {
				claims[actionID] = attachments.Claim{DeviceID: claim.DeviceID, UploadIDs: claim.AttachmentIDs}
			}
		}
		if err := attachmentStore.ReconcileClaims(claims); err != nil {
			return nil, fmt.Errorf("reconcile durable attachment claims: %w", err)
		}
		handler.attachmentCapable = true
		handler.attachmentStore = attachmentStore
	}
	handler.activeView.Store([]transport.MessageSender{})
	handler.snapshotGen.Store(1)
	go handler.deliverBroadcasts()
	go handler.recoverQueuedPrompts()
	return handler, nil
}

func (handler *Handler) recoverQueuedPrompts() {
	if handler.promptQueue == nil || handler.existingTaskSource == nil {
		return
	}
	pendingThreads, err := handler.promptQueue.PendingThreads(handler.ctx)
	if err != nil {
		handler.logger.Error("[mobile-session] queued prompt recovery scan failed", "decision", "retain_durable_queue", "error_class", fmt.Sprintf("%T", err))
		return
	}
	for _, pending := range pendingThreads {
		if handler.ctx.Err() != nil {
			return
		}
		var task taskstate.Task
		var taskErr error
		if recoverySource, okay := handler.existingTaskSource.(ExistingTaskRecoverySource); okay && pending.OwnerSource != "" {
			task, taskErr = recoverySource.CurrentTaskFromSource(handler.ctx, pending.ID, taskstate.Source(pending.OwnerSource))
		} else {
			task, taskErr = handler.existingTaskSource.CurrentTask(handler.ctx, pending.ID)
		}
		if errors.Is(taskErr, taskadapter.ErrTaskUnavailable) {
			entries, _ := handler.promptQueue.Entries(handler.ctx, pending.ID)
			if cancelErr := handler.promptQueue.CancelUnavailableThread(handler.ctx, pending.ID, "task_unavailable", handler.now()); cancelErr != nil {
				handler.logger.Error("[mobile-session] unavailable task queue cleanup failed", "thread_id", pending.ID, "decision", "retain_durable_queue", "error_class", fmt.Sprintf("%T", cancelErr))
			} else {
				handler.releaseCanceledAttachments(entries, true)
				handler.logger.Info("[mobile-session] unavailable task queue cancelled", "thread_id", pending.ID, "decision", "clear_prompt_content")
			}
			continue
		}
		if taskErr != nil || task.ID != pending.ID {
			handler.logger.Warn("[mobile-session] queued task recovery deferred", "thread_id", pending.ID, "decision", "retain_durable_queue", "error_class", fmt.Sprintf("%T", taskErr))
			continue
		}
		switch task.State {
		case taskstate.IdleAfterReply, taskstate.Interrupted, taskstate.Failed:
			handler.publishMu.Lock()
			handler.dispatchNextQueuedPrompt(handler.ctx, pending.ID)
			handler.publishMu.Unlock()
		}
	}
}

func (handler *Handler) Handle(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	if handler == nil || sender == nil || sender.ConnectionID() == 0 {
		return ErrMissingDependency
	}
	if message.Type == "hello" {
		handler.publishMu.Lock()
		defer handler.publishMu.Unlock()
		handler.logger.Info("[mobile-session] message received", "device_id", sender.DeviceID(), "message_type", message.Type, "input_shape", "validated_protocol_message")
		if err := handler.handleHello(ctx, sender, message); err != nil {
			return err
		}
		handler.mu.Lock()
		previous := handler.active[sender.DeviceID()]
		handler.active[sender.DeviceID()] = sender
		handler.storeActiveViewLocked()
		handler.mu.Unlock()
		if previous != nil && previous.ConnectionID() != sender.ConnectionID() {
			previous.Close()
			handler.logger.Info("[mobile-session] older device session replaced", "device_id", sender.DeviceID(), "session_id", sender.SessionID(), "decision", "single_active_session")
		}
		// device_action is never replayed from the journal, so a phone
		// starting a fresh session has no memory of any ask it left
		// outstanding and will never answer it. Whether it sent the reply
		// before it disconnected is exactly what nobody can find out, so
		// every one of those requests is given up on now rather than left
		// to sit until its own timeout. This runs after handler.mu is
		// released — publishCapabilityActionResult can fall back to
		// disconnecting a recipient, which needs that lock itself.
		for _, abandoned := range handler.deviceWork.DeviceGone(sender.DeviceID()) {
			if err := handler.publishCapabilityActionResult(ctx, sender, abandoned.ActionID, "outcome_unknown"); err != nil {
				handler.logger.Error("[mobile-session] device action give-up failed", "device_id", sender.DeviceID(), "request_id", abandoned.RequestID, "action_id", abandoned.ActionID, "error_class", fmt.Sprintf("%T", err))
			}
		}
		return nil
	}
	handler.mu.Lock()
	current := handler.active[sender.DeviceID()]
	handler.mu.Unlock()
	if current == nil || current.ConnectionID() != sender.ConnectionID() {
		return ErrSessionSuperseded
	}
	handler.SweepDeviceWork(ctx)
	handler.logger.Info("[mobile-session] message received", "device_id", sender.DeviceID(), "message_type", message.Type, "input_shape", "validated_protocol_message")
	switch message.Type {
	case "ack":
		var body struct {
			ThroughSequence uint64 `json:"throughSeq"`
		}
		if err := json.Unmarshal(message.Body, &body); err != nil {
			return err
		}
		return handler.journal.Acknowledge(ctx, sender.DeviceID(), body.ThroughSequence)
	case "action":
		return handler.handleAction(ctx, sender, message)
	case "task_read":
		return handler.handleTaskRead(ctx, sender, message)
	case "decision_read":
		return handler.handleDecisionRead(ctx, sender, message)
	case "device_action_result":
		return handler.handleDeviceActionResult(ctx, sender, message)
	default:
		return ErrUnsupportedMessage
	}
}

func (handler *Handler) EnableDecisions(router *decisions.Router) {
	if handler != nil && router != nil {
		handler.decisionRouter = router
	}
}

func (handler *Handler) EnableCapabilities(flow CapabilityFlow) {
	if handler != nil && flow != nil {
		handler.capabilityFlow = flow
	}
}

type decisionPage struct {
	RequestID string            `json:"requestId"`
	TaskID    string            `json:"taskId"`
	Requests  []decisionRequest `json:"requests"`
}

type decisionRequest struct {
	RequestID             string               `json:"requestId"`
	TurnID                string               `json:"turnId"`
	ItemID                string               `json:"itemId"`
	Kind                  decisions.Kind       `json:"kind"`
	ComputerName          string               `json:"computerName"`
	ProjectLabel          string               `json:"projectLabel"`
	WorkingDirectory      string               `json:"workingDirectory,omitempty"`
	Reason                string               `json:"reason,omitempty"`
	Access                string               `json:"access,omitempty"`
	Command               string               `json:"command,omitempty"`
	CommandUnderstandable *bool                `json:"commandUnderstandable,omitempty"`
	AffectedPaths         []string             `json:"affectedPaths,omitempty"`
	AllowedDecisions      []decisions.Decision `json:"allowedDecisions,omitempty"`
	Questions             []decisionQuestion   `json:"questions,omitempty"`
	ExpiresAt             string               `json:"expiresAt"`
}

type decisionQuestion struct {
	ID      string   `json:"id"`
	Header  string   `json:"header"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
	Secret  bool     `json:"secret"`
}

func (handler *Handler) handleDecisionRead(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	if handler.decisionRouter == nil {
		return ErrUnsupportedMessage
	}
	var body struct {
		RequestID string `json:"requestId"`
		TaskID    string `json:"taskId"`
	}
	if err := json.Unmarshal(message.Body, &body); err != nil {
		return err
	}
	handler.decisionRouter.Expire(handler.now())
	pending := handler.decisionRouter.Pending(body.TaskID)
	requests := make([]decisionRequest, 0, len(pending))
	for _, request := range pending {
		projected := decisionRequest{
			RequestID: request.ID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, ComputerName: request.ComputerName,
			ProjectLabel: request.ProjectLabel, WorkingDirectory: request.WorkingDirectory, Reason: request.Reason, Access: request.Access,
			Command: request.Command, AffectedPaths: append([]string(nil), request.AffectedPaths...), AllowedDecisions: append([]decisions.Decision(nil), request.AllowedDecisions...),
			ExpiresAt: request.ExpiresAt.UTC().Format(time.RFC3339),
		}
		if request.Kind == decisions.KindCommand {
			understandable := request.CommandUnderstandable
			projected.CommandUnderstandable = &understandable
		}
		for _, question := range request.Questions {
			projected.Questions = append(projected.Questions, decisionQuestion{ID: question.ID, Header: question.Header, Prompt: question.Prompt, Options: append([]string{}, question.Options...), Secret: question.Secret})
		}
		requests = append(requests, projected)
	}
	encoded, err := json.Marshal(decisionPage{RequestID: body.RequestID, TaskID: body.TaskID, Requests: requests})
	if err != nil {
		return err
	}
	handler.logger.Info("[mobile-session] decision page requested", "device_id", sender.DeviceID(), "task_id", body.TaskID, "request_count", len(requests), "output_shape", "live_unsequenced_safe_decisions")
	return handler.send(ctx, sender, "decision_page", nil, encoded)
}

func (handler *Handler) EnableAttachments(store *attachments.Store) {
	if handler != nil && store != nil {
		claims := map[string]attachments.Claim{}
		if handler.promptQueue != nil {
			active, err := handler.promptQueue.AttachmentClaims(handler.ctx)
			if err != nil {
				handler.logger.Error("[mobile-session] attachment claim lookup failed", "decision", "disable_attachments", "error_class", fmt.Sprintf("%T", err))
				return
			}
			for actionID, claim := range active {
				claims[actionID] = attachments.Claim{DeviceID: claim.DeviceID, UploadIDs: claim.AttachmentIDs}
			}
		}
		if err := store.ReconcileClaims(claims); err != nil {
			handler.logger.Error("[mobile-session] attachment claim reconciliation failed", "decision", "disable_attachments", "error_class", fmt.Sprintf("%T", err))
			return
		}
		handler.attachmentCapable = true
		handler.attachmentStore = store
	}
}

func (handler *Handler) PublishAttachmentAck(ctx context.Context, sender transport.MessageSender, ack transport.AttachmentEvent) error {
	if handler == nil || sender == nil || !handler.attachmentCapable {
		return ErrMissingDependency
	}
	handler.mu.Lock()
	current := handler.active[sender.DeviceID()]
	handler.mu.Unlock()
	if current == nil || current.ConnectionID() != sender.ConnectionID() {
		return ErrSessionSuperseded
	}
	body, err := json.Marshal(ack)
	if err != nil {
		return err
	}
	validationSequence := uint64(1)
	if _, err := contract.EncodeText(contract.Message{
		Version: contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor}, MessageID: "attachment-validation",
		Sender: "companion", Type: "attachment_ack", Sequence: &validationSequence, Body: body,
	}); err != nil {
		return err
	}
	handler.publishMu.Lock()
	defer handler.publishMu.Unlock()
	event, err := handler.journal.Apply(ctx, "attachment_ack", body, handler.now(), func(current json.RawMessage, _ eventjournal.Event) (json.RawMessage, error) {
		return current, nil
	})
	if err != nil {
		return err
	}
	handler.queueDelivery("attachment_ack", event.Sequence, body, []transport.MessageSender{sender})
	handler.logger.Info("[mobile-session] attachment acknowledged", "device_id", sender.DeviceID(), "upload_id", ack.UploadID, "state", ack.State, "received_bytes", ack.ReceivedBytes, "next_chunk", ack.NextChunk)
	return nil
}

func (handler *Handler) handleHello(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	options := taskoptions.Catalog{Models: []taskoptions.Model{}, PermissionModes: []taskoptions.PermissionMode{}}
	optionsCapable := false
	if handler.optionSource != nil {
		loaded, err := handler.optionSource.NewTaskOptions(ctx)
		if err != nil {
			handler.logger.Error("[mobile-session] new task options unavailable", "branch_reason", "unsafe_or_unavailable_catalog", "error_class", fmt.Sprintf("%T", err))
		} else {
			options = loaded
			optionsCapable = true
			handler.logger.Info("[mobile-session] new task options ready", "model_count", len(options.Models), "permission_mode_count", len(options.PermissionModes), "output_shape", "safe_option_catalog")
		}
	}
	if err := handler.send(ctx, sender, "welcome", nil, welcomeBody(sender.SessionID(), handler.taskCapable, handler.transcriptSource != nil, handler.managementSource != nil, optionsCapable, handler.attachmentCapable, handler.decisionRouter != nil, handler.capabilityFlow != nil, options)); err != nil {
		return err
	}
	var body struct {
		Resume struct {
			Mode    string  `json:"mode"`
			LastAck *uint64 `json:"lastAck"`
		} `json:"resume"`
	}
	if err := json.Unmarshal(message.Body, &body); err != nil {
		return err
	}
	replayedWarm := false
	if body.Resume.Mode == "warm" && body.Resume.LastAck != nil {
		events, err := handler.journal.ReplayAfter(ctx, *body.Resume.LastAck)
		if err == nil {
			replayedWarm = true
			for _, event := range events {
				sequence := event.Sequence
				if err := handler.send(ctx, sender, event.Name, &sequence, event.Body); err != nil {
					return err
				}
			}
			handler.logger.Info("[mobile-session] warm events replayed", "device_id", sender.DeviceID(), "last_ack", *body.Resume.LastAck, "replayed_count", len(events), "decision", "follow_with_fresh_snapshot")
		}
		if err != nil && !errors.Is(err, eventjournal.ErrCursorCompacted) {
			return err
		}
	}
	var snapshot eventjournal.Snapshot
	var err error
	if replayedWarm && handler.taskSource == nil {
		current, currentErr := handler.journal.Snapshot(handler.now())
		if currentErr != nil {
			return currentErr
		}
		snapshot, err = handler.journal.ReplaceSnapshot(ctx, current.Body, handler.now())
	} else {
		snapshot, err = handler.refreshTaskSnapshot(ctx)
	}
	if err != nil {
		return err
	}
	var state snapshotState
	if err := json.Unmarshal(snapshot.Body, &state); err != nil {
		return err
	}
	bodyBytes, err := validatedSnapshotBody(snapshot.BaseSequence, state)
	if err != nil {
		return err
	}
	sequence := snapshot.BaseSequence
	return handler.send(ctx, sender, "snapshot", &sequence, bodyBytes)
}

func (handler *Handler) refreshTaskSnapshot(ctx context.Context, provisionalTasks ...snapshotTask) (eventjournal.Snapshot, error) {
	if handler.taskSource == nil {
		return handler.journal.Snapshot(handler.now())
	}
	tasks, err := loadSnapshotTasks(ctx, handler.taskSource, handler.promptQueue)
	if err != nil {
		handler.logger.Error("[mobile-session] task refresh failed", "branch_reason", "catalog_unavailable", "error_class", fmt.Sprintf("%T", err))
		return eventjournal.Snapshot{}, err
	}
	for _, provisional := range provisionalTasks {
		tasks = mergeProvisionalTask(tasks, provisional)
	}
	state := snapshotState{ComputerName: handler.computerName, Projects: handler.projects.List(), Tasks: tasks}
	if _, err := validatedSnapshotBody(1, state); err != nil {
		handler.logger.Error("[mobile-session] task refresh rejected", "branch_reason", "invalid_safe_projection", "error_class", fmt.Sprintf("%T", err))
		return eventjournal.Snapshot{}, err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return eventjournal.Snapshot{}, err
	}
	snapshot, err := handler.journal.ReplaceSnapshot(ctx, encoded, handler.now())
	if err != nil {
		return eventjournal.Snapshot{}, err
	}
	handler.snapshotGen.Add(1)
	handler.logger.Info("[mobile-session] task snapshot refreshed", "task_count", len(tasks), "base_sequence", snapshot.BaseSequence, "output_shape", "safe_task_summaries")
	return snapshot, nil
}

func mergeProvisionalTask(tasks []snapshotTask, provisional snapshotTask) []snapshotTask {
	for index := range tasks {
		if tasks[index].TaskID == provisional.TaskID {
			tasks[index].State = provisional.State
			tasks[index].ActiveTurnID = provisional.ActiveTurnID
			tasks[index].CanRedirect = provisional.CanRedirect
			tasks[index].QueueState = provisional.QueueState
			tasks[index].LastActivityAt = provisional.LastActivityAt
			return tasks
		}
	}
	merged := make([]snapshotTask, 0, min(len(tasks)+1, taskstate.MaxHomeTasks))
	merged = append(merged, provisional)
	remaining := min(len(tasks), taskstate.MaxHomeTasks-1)
	return append(merged, tasks[:remaining]...)
}

func provisionalTaskTitle(prompt string) string {
	prompt = strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return ' '
		}
		return value
	}, prompt)
	prompt = strings.Join(strings.Fields(prompt), " ")
	runes := []rune(prompt)
	if len(runes) > 256 {
		prompt = string(runes[:256])
	}
	if prompt == "" {
		return "Codex task"
	}
	return prompt
}

func (handler *Handler) projectDisplayName(projectID string) string {
	for _, choice := range handler.projects.List() {
		if choice.ID == projectID {
			return choice.DisplayName
		}
	}
	return "Project"
}

func (handler *Handler) RefreshTaskSnapshot(ctx context.Context) error {
	if handler == nil {
		return ErrMissingDependency
	}
	handler.publishMu.Lock()
	defer handler.publishMu.Unlock()
	snapshot, err := handler.refreshTaskSnapshot(ctx)
	if err != nil {
		return err
	}
	var state snapshotState
	if json.Unmarshal(snapshot.Body, &state) != nil {
		return ErrInvalidTaskEvent
	}
	body, err := validatedSnapshotBody(snapshot.BaseSequence, state)
	if err != nil {
		return err
	}
	recipients := handler.queueBroadcast("snapshot", snapshot.BaseSequence, body)
	handler.logger.Info("[mobile-session] refreshed task snapshot queued", "base_sequence", snapshot.BaseSequence, "recipient_count", recipients, "output_shape", "safe_task_summaries")
	return nil
}

func (handler *Handler) TaskSnapshotGeneration() uint64 {
	if handler == nil {
		return 0
	}
	return handler.snapshotGen.Load()
}

func (handler *Handler) handleAction(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	var action struct {
		ActionID         string              `json:"actionId"`
		Kind             string              `json:"kind"`
		ProjectID        string              `json:"projectId"`
		TaskID           string              `json:"taskId"`
		Title            string              `json:"title"`
		Text             string              `json:"text"`
		ModelID          string              `json:"modelId"`
		ReasoningID      string              `json:"reasoningId"`
		PermissionModeID string              `json:"permissionModeId"`
		TargetActionID   string              `json:"targetActionId"`
		AttachmentIDs    []string            `json:"attachmentIds"`
		RequestID        string              `json:"requestId"`
		RequestKind      decisions.Kind      `json:"requestKind"`
		Decision         decisions.Decision  `json:"decision"`
		Answers          map[string][]string `json:"answers"`
		Utterance        string              `json:"utterance"`
		Fingerprint      string              `json:"fingerprint"`
		AdapterID        string              `json:"adapterId"`
	}
	if err := json.Unmarshal(message.Body, &action); err != nil {
		return err
	}
	handler.publishMu.Lock()
	defer handler.publishMu.Unlock()
	handler.mu.Lock()
	current := handler.active[sender.DeviceID()]
	handler.mu.Unlock()
	if current == nil || current.ConnectionID() != sender.ConnectionID() {
		return ErrSessionSuperseded
	}
	if action.Kind == "capability_request" || action.Kind == "capability_confirm" || action.Kind == "capability_disconnect" {
		return handler.handleCapabilityAction(ctx, sender, capabilityActionParams{
			ActionID:    action.ActionID,
			Kind:        action.Kind,
			RequestID:   action.RequestID,
			Utterance:   action.Utterance,
			Fingerprint: action.Fingerprint,
			Decision:    string(action.Decision),
			AdapterID:   action.AdapterID,
		})
	}
	result := map[string]any{"actionId": action.ActionID, "state": "confirmed"}
	refreshTasks := false
	var provisionalTask *snapshotTask
	switch action.Kind {
	case "start_turn":
		if action.TaskID != "" {
			outcome, code, resultCode := handler.startExistingTask(ctx, sender.DeviceID(), action.ActionID, action.TaskID, action.Text, action.AttachmentIDs, false)
			applyExistingTaskOutcome(result, outcome, code, resultCode)
			refreshTasks = outcome == existingTaskAccepted && resultCode != "queued"
			break
		}
		outcome, code := handler.startNewTask(ctx, sender.DeviceID(), action.ActionID, action.ProjectID, action.Text, action.ModelID, action.ReasoningID, action.PermissionModeID, action.AttachmentIDs)
		switch outcome {
		case newTaskConfirmed:
			refreshTasks = true
			if confirmed, confirmedErr := handler.promptQueue.Entry(ctx, action.ActionID); confirmedErr == nil && confirmed.Result.ThreadID != "" && confirmed.Result.TurnID != "" {
				task := snapshotTask{
					TaskID: confirmed.Result.ThreadID, Title: provisionalTaskTitle(action.Text), ProjectLabel: handler.projectDisplayName(action.ProjectID),
					State: string(taskstate.Working), ActiveTurnID: confirmed.Result.TurnID, CanRedirect: true,
					QueueState: string(promptqueue.QueueEmpty), LastActivityAt: handler.now().UTC().Format(time.RFC3339),
				}
				provisionalTask = &task
				handler.logger.Info("[mobile-session] provisional new task prepared", "task_id", task.TaskID, "turn_id", task.ActiveTurnID, "branch_reason", "shared_catalog_pending")
			}
		case newTaskOutcomeUnknown:
			setActionOutcomeUnknown(result)
		case newTaskFailed:
			setActionFailure(result, string(code), code == failureInternal)
		}
	case "steer_turn":
		outcome, code, resultCode := handler.startExistingTask(ctx, sender.DeviceID(), action.ActionID, action.TaskID, action.Text, action.AttachmentIDs, true)
		applyExistingTaskOutcome(result, outcome, code, resultCode)
		refreshTasks = outcome == existingTaskAccepted && resultCode == "redirected"
	case "interrupt_turn":
		outcome, code, resultCode := handler.stopExistingTask(ctx, action.ActionID, action.TaskID)
		applyExistingTaskOutcome(result, outcome, code, resultCode)
		refreshTasks = outcome == existingTaskAccepted
	case "dismiss_unknown_control":
		if handler.promptQueue == nil {
			handler.logger.Info("[mobile-session] dismiss rejected", "device_id", sender.DeviceID(), "action_id", action.ActionID, "branch_reason", "no_prompt_queue", "error_code", string(failureDesktopIncompatible))
			setActionFailure(result, string(failureDesktopIncompatible), false)
			break
		}
		dismissed, dismissErr := handler.promptQueue.DismissUnknown(ctx, action.TargetActionID, action.TaskID, sender.DeviceID(), handler.now())
		if dismissErr != nil {
			// The target control does not exist, belongs to another device, or
			// is not in a dismissible state - that is the request being wrong,
			// not a broken component.
			handler.logger.Info("[mobile-session] dismiss rejected", "device_id", sender.DeviceID(), "action_id", action.ActionID, "target_action_id", action.TargetActionID, "branch_reason", "dismiss_target_invalid", "error_code", string(failureInvalidAction))
			setActionFailure(result, string(failureInvalidAction), false)
		} else {
			handler.releaseActionAttachments(dismissed)
			result["resultCode"] = "accepted"
			refreshTasks = true
		}
	case "set_project":
		if _, err := handler.projects.Resolve(action.ProjectID); err != nil {
			handler.logger.Info("[mobile-session] set_project rejected", "device_id", sender.DeviceID(), "project_id", action.ProjectID, "branch_reason", "project_not_resolved", "error_code", string(failureInvalidAction))
			setActionFailure(result, string(failureInvalidAction), false)
		}
	case "approval", "question_response":
		if handler.decisionRouter == nil {
			handler.logger.Info("[mobile-session] decision rejected", "device_id", sender.DeviceID(), "action_id", action.ActionID, "action_kind", action.Kind, "branch_reason", "no_decision_router", "error_code", string(failureDesktopIncompatible))
			setActionFailure(result, string(failureDesktopIncompatible), false)
			break
		}
		pending, okay := handler.pendingDecision(action.TaskID, action.RequestID)
		if !okay || action.Kind == "approval" && pending.Kind != action.RequestKind || action.Kind == "question_response" && pending.Kind != decisions.KindQuestion {
			handler.logger.Info("[mobile-session] decision rejected", "device_id", sender.DeviceID(), "action_id", action.ActionID, "action_kind", action.Kind, "branch_reason", "pending_decision_not_found_or_kind_mismatch", "error_code", string(failureInvalidAction))
			setActionFailure(result, string(failureInvalidAction), false)
			break
		}
		response := decisions.Response{RequestID: pending.ID, ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: pending.ItemID, Kind: pending.Kind}
		if action.Kind == "approval" {
			response.Decision = action.Decision
		} else {
			response.Answers = action.Answers
		}
		if err := handler.decisionRouter.Respond(ctx, response, handler.now()); err != nil {
			switch {
			case errors.Is(err, decisions.ErrResponseOutcomeUnknown):
				setActionOutcomeUnknown(result)
			case errors.Is(err, decisions.ErrOwnerUnavailable):
				setActionFailure(result, "owner_unavailable", true)
			default:
				// We do not know why the router refused this response, so we
				// admit that rather than blaming the request.
				handler.logger.Error("[mobile-session] decision response failed", "device_id", sender.DeviceID(), "action_id", action.ActionID, "action_kind", action.Kind, "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
				setActionFailure(result, string(failureInternal), true)
			}
		} else {
			result["resultCode"] = "accepted"
		}
	case "rename_task", "archive_task", "fork_task":
		if handler.managementSource == nil {
			handler.logger.Info("[mobile-session] task management rejected", "device_id", sender.DeviceID(), "task_id", action.TaskID, "action_kind", action.Kind, "branch_reason", "no_management_source", "error_code", string(failureDesktopIncompatible))
			setActionFailure(result, string(failureDesktopIncompatible), false)
			break
		}
		if action.Kind == "archive_task" && handler.promptQueue != nil {
			queueStatus, statusErr := handler.promptQueue.Status(ctx, action.TaskID)
			if statusErr != nil {
				handler.logger.Error("[mobile-session] archive queue status lookup failed", "device_id", sender.DeviceID(), "task_id", action.TaskID, "error_class", fmt.Sprintf("%T", statusErr), "error_code", string(failureInternal))
				setActionFailure(result, "internal", true)
				break
			}
			if queueStatus == promptqueue.QueueOutcomeUnknown {
				// Deliberately invalid_action, not internal: the archive is
				// refused because a control still needs explicit review, which
				// is a fact about this request, not an unexplained break.
				handler.logger.Info("[mobile-session] archive blocked by unknown task control", "device_id", sender.DeviceID(), "task_id", action.TaskID, "decision", "require_explicit_review", "error_code", string(failureInvalidAction))
				setActionFailure(result, string(failureInvalidAction), false)
				break
			}
		}
		var err error
		switch action.Kind {
		case "rename_task":
			err = handler.managementSource.Rename(ctx, action.TaskID, action.Title)
		case "archive_task":
			err = handler.managementSource.Archive(ctx, action.TaskID)
			if err == nil && handler.promptQueue != nil {
				entries, entriesErr := handler.promptQueue.Entries(ctx, action.TaskID)
				if entriesErr != nil {
					err = fmt.Errorf("archive completed but queued prompt attachment lookup failed: %w", entriesErr)
					break
				}
				if cancelErr := handler.promptQueue.CancelThread(ctx, action.TaskID, "task_archived", handler.now()); cancelErr != nil {
					err = fmt.Errorf("archive completed but queued prompt cleanup is uncertain: %w", cancelErr)
				} else {
					handler.releaseCanceledAttachments(entries, false)
				}
			}
		case "fork_task":
			var forked taskstate.Task
			forked, err = handler.managementSource.ForkToAppServer(ctx, action.TaskID)
			if err == nil {
				result["forkTaskId"] = forked.ID
			}
		}
		if err != nil {
			handler.logger.Error("[mobile-session] task management action failed", "device_id", sender.DeviceID(), "task_id", action.TaskID, "action_kind", action.Kind, "error_class", fmt.Sprintf("%T", err))
			if writeOutcomeUnknown(err) {
				setActionOutcomeUnknown(result)
			} else {
				setActionFailure(result, "internal", true)
			}
		} else {
			refreshTasks = true
		}
	default:
		return ErrUnsupportedMessage
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	event, err := handler.journal.Apply(ctx, "action_result", body, handler.now(), func(current json.RawMessage, _ eventjournal.Event) (json.RawMessage, error) {
		return current, nil
	})
	if err != nil {
		return err
	}
	sequence := event.Sequence
	handler.logger.Info("[mobile-session] action resolved", "device_id", sender.DeviceID(), "action_kind", action.Kind, "task_id", action.TaskID, "project_id", action.ProjectID, "result_state", result["state"])
	handler.queueDelivery("action_result", sequence, body, []transport.MessageSender{sender})
	if refreshTasks {
		var snapshot eventjournal.Snapshot
		var refreshErr error
		if provisionalTask == nil {
			snapshot, refreshErr = handler.refreshTaskSnapshot(ctx)
		} else {
			snapshot, refreshErr = handler.refreshTaskSnapshot(ctx, *provisionalTask)
		}
		if refreshErr != nil {
			handler.logger.Error("[mobile-session] confirmed task action snapshot refresh failed", "device_id", sender.DeviceID(), "action_kind", action.Kind, "task_id", action.TaskID, "decision", "schedule_bounded_refresh", "error_class", fmt.Sprintf("%T", refreshErr))
			handler.scheduleTaskSnapshotRefresh()
			return nil
		}
		var state snapshotState
		if json.Unmarshal(snapshot.Body, &state) != nil {
			return ErrInvalidTaskEvent
		}
		snapshotBody, validationErr := validatedSnapshotBody(snapshot.BaseSequence, state)
		if validationErr != nil {
			return validationErr
		}
		handler.queueBroadcast("snapshot", snapshot.BaseSequence, snapshotBody)
	}
	return nil
}

// capabilityActionParams carries the fields handleCapabilityAction needs out
// of the wire action. It exists so a new capability action kind (like
// capability_disconnect) can add one field without every call site growing
// another positional string parameter.
type capabilityActionParams struct {
	ActionID    string
	Kind        string
	RequestID   string
	Utterance   string
	Fingerprint string
	Decision    string
	AdapterID   string
}

func (handler *Handler) handleCapabilityAction(ctx context.Context, sender transport.MessageSender, params capabilityActionParams) error {
	actionID, kind, requestID, utterance, fingerprint, decision, adapterID :=
		params.ActionID, params.Kind, params.RequestID, params.Utterance, params.Fingerprint, params.Decision, params.AdapterID
	if handler.capabilityFlow == nil {
		// There is no error to map here — this build was simply never given a
		// capability flow, which happens on a desktop that has not wired one
		// up. "desktop_incompatible" says exactly that.
		return handler.publishCapabilityFailure(ctx, sender, actionID, failureDesktopIncompatible)
	}
	switch kind {
	case "capability_request":
		ownerID := capabilityOwner(sender)
		preview, err := handler.capabilityFlow.Prepare(ctx, ownerID, actionID, utterance)
		if err != nil {
			// A question is not a failure. The flow returns QuestionError when
			// it understood the request perfectly well and needs one more word
			// before it can act — most often which of several people named
			// "Maya" was meant. Nothing broke and nothing was refused, so
			// "failed" with a hardcoded "invalid_action" tells the user their
			// request was wrong when it was not.
			//
			// "cancelled" is the honest word the shipped phone will accept.
			// Its decoder holds a closed set of error codes
			// (ProtocolCodec.kt:618) and drops the whole envelope on an
			// unknown one, so a new code like "needs_disambiguation" would
			// mean the user is told nothing at all. "cancelled" is already a
			// valid action state there (:601) and carries no error object
			// (:216 permits one only on failed and outcome_unknown), which is
			// exactly right: we stopped, we did not act, and nothing is wrong.
			//
			// Delivering the question text and routing an answer back is a
			// wire change on both sides and is not done here.
			var question *capabilityflow.QuestionError
			if errors.As(err, &question) {
				handler.logger.Info("[mobile-session] capability needs an answer before it can act", "device_id", sender.DeviceID(), "request_id", actionID, "question_length", len(question.Question))
				return handler.publishCapabilityActionResult(ctx, sender, actionID, "cancelled", question.Question)
			}
			code := capabilityFailureCode(err)
			handler.logger.Error("[mobile-session] capability prepare failed", "device_id", sender.DeviceID(), "request_id", actionID, "error_class", fmt.Sprintf("%T", err), "code", code)
			return handler.publishCapabilityFailure(ctx, sender, actionID, code)
		}
		body, err := json.Marshal(struct {
			RequestID    string   `json:"requestId"`
			AdapterID    string   `json:"adapterId"`
			Verb         string   `json:"verb"`
			Headline     string   `json:"headline"`
			Lines        []string `json:"lines"`
			ConfirmLabel string   `json:"confirmLabel"`
			Fingerprint  string   `json:"fingerprint"`
		}{preview.RequestID, preview.AdapterID, string(preview.Verb), preview.Headline, preview.Lines, preview.Confirm, preview.Fingerprint})
		if err != nil {
			return err
		}
		handler.logger.Info("[mobile-session] capability preview ready", "device_id", sender.DeviceID(), "request_id", actionID, "adapter_id", preview.AdapterID, "verb", preview.Verb, "line_count", len(preview.Lines))
		return handler.send(ctx, sender, "capability_preview", nil, body)
	case "capability_confirm":
		ownerID := capabilityOwner(sender)
		if decision == "cancel" {
			if err := handler.capabilityFlow.Cancel(ownerID, requestID, fingerprint); err != nil {
				code := capabilityFailureCode(err)
				handler.logger.Error("[mobile-session] capability cancel failed", "device_id", sender.DeviceID(), "request_id", requestID, "error_class", fmt.Sprintf("%T", err), "code", code)
				return handler.publishCapabilityFailure(ctx, sender, actionID, code)
			}
			return handler.publishCapabilityActionResult(ctx, sender, actionID, "cancelled")
		}
		outcome, err := handler.capabilityFlow.Confirm(ctx, ownerID, requestID, fingerprint)
		if err != nil {
			// Device work is checked first because it is a different
			// situation from the other two branches below, not because
			// ordering could make one shadow another — the three error
			// types are distinct, so any order would type-switch correctly.
			var deviceWork *capabilityadapter.DeviceWorkError
			if errors.As(err, &deviceWork) {
				return handler.handOffToDevice(ctx, sender, actionID, requestID, deviceWork)
			}
			var unknown *capabilityadapter.OutcomeUnknownError
			if errors.As(err, &unknown) {
				handler.logger.Error("[mobile-session] capability execute outcome unknown", "device_id", sender.DeviceID(), "request_id", requestID, "adapter_id", unknown.AdapterID, "verb", unknown.Verb)
				return handler.publishCapabilityActionResult(ctx, sender, actionID, "outcome_unknown")
			}
			code := capabilityFailureCode(err)
			handler.logger.Error("[mobile-session] capability execute failed", "device_id", sender.DeviceID(), "request_id", requestID, "error_class", fmt.Sprintf("%T", err), "code", code)
			return handler.publishCapabilityFailure(ctx, sender, actionID, code)
		}
		body, err := json.Marshal(struct {
			RequestID   string `json:"requestId"`
			Ceiling     string `json:"ceiling"`
			Done        bool   `json:"done"`
			Detail      string `json:"detail"`
			HandedOffTo string `json:"handedOffTo"`
		}{requestID, string(outcome.Reached), outcome.Done, outcome.Detail, outcome.HandedOffTo})
		if err != nil {
			return err
		}
		event, err := handler.journal.Apply(ctx, "capability_result", body, handler.now(), func(current json.RawMessage, _ eventjournal.Event) (json.RawMessage, error) { return current, nil })
		if err != nil {
			return err
		}
		handler.queueDelivery("capability_result", event.Sequence, body, []transport.MessageSender{sender})
		handler.logger.Info("[mobile-session] capability result queued", "device_id", sender.DeviceID(), "request_id", requestID, "ceiling", outcome.Reached, "done", outcome.Done)
		return nil
	case "capability_disconnect":
		if err := handler.capabilityFlow.Disconnect(ctx, adapterID); err != nil {
			// The same rule as the confirm branch above, for the same reason.
			// A revoke request that went out and lost its reply may already
			// have killed the token; Google answers a second revoke of a dead
			// token with a 4xx, so "failed" here would repeat forever while
			// the access is in fact already gone.
			var unknown *capabilityadapter.OutcomeUnknownError
			if errors.As(err, &unknown) {
				handler.logger.Error("[mobile-session] capability disconnect outcome unknown", "device_id", sender.DeviceID(), "adapter_id", adapterID, "verb", unknown.Verb)
				return handler.publishCapabilityActionResult(ctx, sender, actionID, "outcome_unknown")
			}
			code := capabilityFailureCode(err)
			handler.logger.Error("[mobile-session] capability disconnect failed", "device_id", sender.DeviceID(), "adapter_id", adapterID, "error_class", fmt.Sprintf("%T", err), "code", code)
			return handler.publishCapabilityFailure(ctx, sender, actionID, code)
		}
		handler.logger.Info("[mobile-session] capability disconnected", "device_id", sender.DeviceID(), "adapter_id", adapterID)
		return handler.publishCapabilityActionResult(ctx, sender, actionID, "confirmed")
	default:
		return ErrUnsupportedMessage
	}
}

func capabilityOwner(sender transport.MessageSender) string {
	return fmt.Sprintf("%s/%s/%d", sender.DeviceID(), sender.SessionID(), sender.ConnectionID())
}

// handOffToDevice records that requestID is now waiting on sender's phone
// and asks it to carry out deviceWork's instruction. The ask goes out with
// send, not through the journal: device_action is never replayed, because a
// request the phone missed while offline has to expire rather than fire
// late into a conversation that has moved on. No result frame is sent here
// — until the phone answers, the honest thing to say is nothing at all.
func (handler *Handler) handOffToDevice(ctx context.Context, sender transport.MessageSender, actionID, requestID string, deviceWork *capabilityadapter.DeviceWorkError) error {
	// Resolve now, once, so nothing downstream has to ask what a blank
	// ceiling means. An adapter that named nothing gets read as hands_off —
	// the most modest claim, not the strongest.
	ceiling := deviceWork.Ceiling.OrHandsOff()
	record := devicework.Record{RequestID: requestID, DeviceID: sender.DeviceID(), Kind: deviceWork.Kind, ActionID: actionID, AdapterID: deviceWork.AdapterID, Ceiling: ceiling}
	if !handler.deviceWork.Wait(record) {
		// The request is already on the books. Handing it to the phone a
		// second time would put the same message in front of a real person
		// twice, so the honest response to a duplicate confirm is silence.
		handler.logger.Info("[mobile-session] device action already outstanding", "device_id", sender.DeviceID(), "request_id", requestID, "adapter_id", deviceWork.AdapterID, "kind", deviceWork.Kind, "ceiling", string(ceiling))
		return nil
	}
	body, err := json.Marshal(struct {
		RequestID string `json:"requestId"`
		Kind      string `json:"kind"`
		Handle    string `json:"handle"`
		Text      string `json:"text"`
	}{requestID, deviceWork.Kind, deviceWork.Handle, deviceWork.Text})
	if err != nil {
		return err
	}
	// Handle and Text are what a real person wrote — never in the log line.
	// ceiling is the cap that will be applied to whatever the phone reports
	// back, so it belongs in this log line rather than a new one.
	handler.logger.Info("[mobile-session] device action handed to phone", "device_id", sender.DeviceID(), "request_id", requestID, "adapter_id", deviceWork.AdapterID, "kind", deviceWork.Kind, "ceiling", string(ceiling))
	return handler.send(ctx, sender, "device_action", nil, body)
}

// handleDeviceActionResult is the phone answering a device_action. Only the
// first answer for a request is accepted — the ledger enforces that — so a
// replay, a confused phone, or an answer that arrives after the request was
// already given up on all land here as silence rather than as a second
// capability_result.
func (handler *Handler) handleDeviceActionResult(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	var body struct {
		RequestID string `json:"requestId"`
		Outcome   string `json:"outcome"`
	}
	if err := json.Unmarshal(message.Body, &body); err != nil {
		return err
	}
	handler.publishMu.Lock()
	defer handler.publishMu.Unlock()
	record, settled := handler.deviceWork.Settle(body.RequestID)
	if !settled {
		handler.logger.Info("[mobile-session] device action result for a request nobody is waiting on", "device_id", sender.DeviceID(), "request_id", body.RequestID)
		return nil
	}
	var done bool
	var detail string
	switch body.Outcome {
	case "handed_to_the_app":
		if record.Kind == "youtube_play" {
			done, detail = false, "Opened the selected video in YouTube; playback was not verified."
		} else {
			done, detail = true, "Handed to the app — we can't see whether it reached them."
		}
	case "notification_gone":
		done, detail = false, "The notification is gone, so there was no reply box left to use."
	case "refused":
		done, detail = false, "Nothing was sent — this one has to be replied to in its own app."
	case "failed":
		done, detail = false, "The reply didn't go through."
	default:
		// The contract validator rejects any outcome word it does not know,
		// so this cannot actually be reached. It stays an explicit switch
		// with its own default instead of falling through to "failed" —
		// this codebase has been bitten before by a default that silently
		// inherited the wrong branch.
		handler.logger.Error("[mobile-session] device action reported an outcome word nobody defined", "device_id", sender.DeviceID(), "request_id", body.RequestID, "outcome", body.Outcome)
		return nil
	}
	// The ceiling was fixed at hand-off time (handOffToDevice), from what the
	// adapter actually declared it could do — not from what the phone says
	// happened here. A refusal or failure still carries that same ceiling;
	// only "done" changes with the outcome. A record from before this field
	// existed would carry a blank Ceiling, so resolve it the same way
	// hand-off does rather than trust it is always already set.
	ceiling := record.Ceiling.OrHandsOff()
	// A hands_off adapter is never allowed to claim it finished — that is
	// the one claim its declared ceiling rules out. Device work never names
	// a follow-up app (there is nothing here like "opened Spotify, go
	// finish it there"), so hands_off can only ever mean "not done", the
	// same rule the ordinary path enforces with its own clamp
	// (execution/runner.go:215-230). Without this, a hands_off adapter whose
	// reply the phone says it sent would still be reported as finished —
	// the exact bug this file exists to close, just moved from the ceiling
	// word to the done flag next to it.
	if ceiling == manifest.HandsOff {
		done = false
	}
	resultBody, err := json.Marshal(struct {
		RequestID   string `json:"requestId"`
		Ceiling     string `json:"ceiling"`
		Done        bool   `json:"done"`
		Detail      string `json:"detail"`
		HandedOffTo string `json:"handedOffTo"`
	}{record.RequestID, string(ceiling), done, detail, ""})
	if err != nil {
		return err
	}
	event, err := handler.journal.Apply(ctx, "capability_result", resultBody, handler.now(), func(current json.RawMessage, _ eventjournal.Event) (json.RawMessage, error) { return current, nil })
	if err != nil {
		return err
	}
	handler.queueDelivery("capability_result", event.Sequence, resultBody, []transport.MessageSender{sender})
	handler.logger.Info("[mobile-session] device action result closed the request", "device_id", sender.DeviceID(), "request_id", record.RequestID, "adapter_id", record.AdapterID, "outcome", body.Outcome, "ceiling", string(ceiling), "done", done)
	return nil
}

// SweepDeviceWork reports outcome_unknown for every device_action whose wait
// has run past DeviceWorkTimeout. It is safe to call with nothing
// outstanding, and it has to be — the caller drives it opportunistically on
// every message a live session receives, and there is no goroutine or
// ticker inside the handler, because a background sweeper would make the
// exact deadline a test observes unpredictable.
func (handler *Handler) SweepDeviceWork(ctx context.Context) {
	if handler == nil || handler.deviceWork == nil {
		return
	}
	expired := handler.deviceWork.Expired()
	if len(expired) == 0 {
		return
	}
	handler.publishMu.Lock()
	defer handler.publishMu.Unlock()
	for _, record := range expired {
		handler.mu.Lock()
		sender := handler.active[record.DeviceID]
		handler.mu.Unlock()
		if err := handler.publishCapabilityActionResult(ctx, sender, record.ActionID, "outcome_unknown"); err != nil {
			handler.logger.Error("[mobile-session] device action timeout report failed", "device_id", record.DeviceID, "request_id", record.RequestID, "action_id", record.ActionID, "error_class", fmt.Sprintf("%T", err))
			continue
		}
		handler.logger.Info("[mobile-session] device action timed out", "device_id", record.DeviceID, "request_id", record.RequestID, "action_id", record.ActionID)
	}
}

// actionFailureCode is one of the eleven words the phone's decoder accepts
// (ProtocolCodec.kt:618). Anything outside that set makes the phone drop the
// whole message, so the user is told nothing at all — worse than the wrong
// word.
type actionFailureCode string

const (
	failureUnauthorized        actionFailureCode = "unauthorized"
	failureInvalidAction       actionFailureCode = "invalid_action"
	failureDesktopIncompatible actionFailureCode = "desktop_incompatible"
	failureInternal            actionFailureCode = "internal"
)

// capabilityFailureCode turns a Go error into the one word out of the
// phone's fixed vocabulary that best tells the user what actually happened.
//
// "internal" is the default for anything not explicitly recognized below,
// and that default is deliberate, not lazy: "internal" admits we cannot
// explain what went wrong, while "invalid_action" claims to know the user
// did something wrong. Only the errors listed below have actually
// established that the request itself was the problem; every other error
// gets the honest, unassuming answer.
func capabilityFailureCode(err error) actionFailureCode {
	switch {
	case errors.Is(err, consent.ErrNotGranted), errors.Is(err, manifest.ErrGateNotCleared):
		// The request was fine. The one thing missing is something the user
		// can fix themselves — connect the app, or clear the checkpoint —
		// and "unauthorized" is the only word in the set that says so.
		return failureUnauthorized
	case errors.Is(err, consent.ErrNeverShipped),
		errors.Is(err, execution.ErrVerbNotOffered),
		errors.Is(err, execution.ErrPreviewRequired),
		errors.Is(err, capabilityflow.ErrUnknownRequest),
		errors.Is(err, capabilityflow.ErrRequestExists),
		errors.Is(err, capabilityflow.ErrFingerprintMismatch):
		// These really are the user (or a stale client) asking for something
		// that does not exist or does not match what was previewed.
		// "invalid_action" is the truth here.
		return failureInvalidAction
	default:
		return failureInternal
	}
}

// publishCapabilityActionResult reports a non-failure outcome: confirmed,
// cancelled, or outcome_unknown. A "failed" result always needs a caller to
// say which of the eleven words explains it, so that state is not accepted
// here — see publishCapabilityFailure.
// question is variadic so every existing call site is untouched -- an
// ordinary result must not grow the field just because this function learned
// a new capability. Only the "cancelled" result that follows a QuestionError
// passes one.
func (handler *Handler) publishCapabilityActionResult(ctx context.Context, sender transport.MessageSender, actionID, state string, question ...string) error {
	result := map[string]any{"actionId": actionID, "state": state}
	if state == "outcome_unknown" {
		result["error"] = map[string]any{"code": "outcome_unknown", "retryable": false}
	}
	if len(question) > 0 && question[0] != "" {
		result["question"] = question[0]
	}
	return handler.publishCapabilityResult(ctx, sender, result)
}

// publishCapabilityFailure reports a "failed" result, always naming the
// error code that explains it — there is no path that publishes "failed"
// without one.
func (handler *Handler) publishCapabilityFailure(ctx context.Context, sender transport.MessageSender, actionID string, code actionFailureCode) error {
	result := map[string]any{"actionId": actionID, "state": "failed", "error": map[string]any{"code": string(code), "retryable": false}}
	return handler.publishCapabilityResult(ctx, sender, result)
}

// publishCapabilityResult marshals an already-built action_result body,
// records it in the journal, and delivers it to sender if one is connected.
// It is the shared tail of publishCapabilityActionResult and
// publishCapabilityFailure.
func (handler *Handler) publishCapabilityResult(ctx context.Context, sender transport.MessageSender, result map[string]any) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	event, err := handler.journal.Apply(ctx, "action_result", body, handler.now(), func(current json.RawMessage, _ eventjournal.Event) (json.RawMessage, error) { return current, nil })
	if err != nil {
		return err
	}
	// sender is nil when the device that owed this answer is not currently
	// connected — SweepDeviceWork hits this when a request times out after
	// its phone has gone away. The journal has already recorded the event so
	// the phone learns the outcome on its next reconnect; there is simply no
	// live connection to push it to right now.
	if sender != nil {
		handler.queueDelivery("action_result", event.Sequence, body, []transport.MessageSender{sender})
	}
	return nil
}

func (handler *Handler) pendingDecision(taskID, requestID string) (decisions.Request, bool) {
	if handler == nil || handler.decisionRouter == nil {
		return decisions.Request{}, false
	}
	for _, request := range handler.decisionRouter.Pending(taskID) {
		if request.ID == requestID {
			return request, true
		}
	}
	return decisions.Request{}, false
}

type existingTaskOutcome uint8

const (
	existingTaskFailed existingTaskOutcome = iota
	existingTaskAccepted
	existingTaskOutcomeUnknown
)

// applyExistingTaskOutcome reports the code startExistingTask or
// stopExistingTask chose for a failure, rather than always saying
// "invalid_action" - the same defect the new-task path had, on the path
// used by steer_turn, interrupt_turn, and start_turn against an existing
// task.
func applyExistingTaskOutcome(result map[string]any, outcome existingTaskOutcome, code actionFailureCode, resultCode string) {
	switch outcome {
	case existingTaskAccepted:
		result["resultCode"] = resultCode
	case existingTaskOutcomeUnknown:
		setActionOutcomeUnknown(result)
	default:
		setActionFailure(result, string(code), code == failureInternal)
	}
}

// startExistingTask reports which of the eleven phone-accepted codes
// explains a failure, the same way startNewTask does; the code is
// meaningless on the existingTaskAccepted and existingTaskOutcomeUnknown
// outcomes.
func (handler *Handler) startExistingTask(ctx context.Context, deviceID, actionID, taskID, prompt string, attachmentIDs []string, redirect bool) (existingTaskOutcome, actionFailureCode, string) {
	kind := "start_turn"
	if redirect {
		kind = "steer_turn"
	}
	if handler.existingTaskSource == nil || handler.promptQueue == nil {
		handler.logger.Info("[mobile-session] existing task rejected", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "no_task_management_component", "error_code", string(failureDesktopIncompatible))
		return existingTaskFailed, failureDesktopIncompatible, ""
	}
	requestHash := requestHashWithAttachments([]string{kind, taskID, prompt}, deviceID, attachmentIDs)
	stored, err := handler.promptQueue.Entry(ctx, actionID)
	switch {
	case err == nil:
		if stored.RequestHash != requestHash || stored.ThreadID != taskID || stored.DeviceID != attachmentDeviceID(deviceID, attachmentIDs) || !slices.Equal(stored.AttachmentIDs, attachmentIDs) {
			handler.logger.Warn("[mobile-session] duplicate existing task action rejected", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "request_hash_mismatch", "error_code", string(failureInvalidAction))
			return existingTaskFailed, failureInvalidAction, ""
		}
		switch stored.State {
		case promptqueue.StatePrepared:
			return existingTaskAccepted, "", "queued"
		case promptqueue.StateConfirmed:
			return existingTaskAccepted, "", stored.Result.Code
		case promptqueue.StateSentUnknown:
			return existingTaskOutcomeUnknown, "", ""
		default:
			handler.logger.Info("[mobile-session] existing task rejected", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "prepared_entry_unusable_state", "error_code", string(failureInternal))
			return existingTaskFailed, failureInternal, ""
		}
	case !errors.Is(err, promptqueue.ErrActionNotFound):
		handler.logger.Error("[mobile-session] existing task lookup failed", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	task, err := handler.existingTaskSource.CurrentTask(ctx, taskID)
	if err != nil {
		handler.logger.Error("[mobile-session] existing task preflight failed", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", existingTaskErrorCode(err, task.ID == taskID), "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	if task.ID != taskID {
		// The source answered without error but the task it returned is not
		// the one the phone asked about - that is the request naming
		// something we do not have, not a break in the source.
		handler.logger.Info("[mobile-session] existing task rejected", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "task_id_mismatch", "error_code", string(failureInvalidAction))
		return existingTaskFailed, failureInvalidAction, ""
	}
	if err := handler.claimActionAttachments(deviceID, attachmentIDs, actionID); err != nil {
		handler.logger.Error("[mobile-session] existing task attachment claim failed", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "attachment_claim_failed", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	entry := promptqueue.Entry{
		ActionID: actionID, QueueKey: taskID, ActionKind: kind, OwnerSource: string(task.Source), ThreadID: taskID, Prompt: prompt,
		RequestHash: requestHash, DeviceID: attachmentDeviceID(deviceID, attachmentIDs), AttachmentIDs: append([]string(nil), attachmentIDs...), CreatedAt: handler.now(),
	}
	if err := handler.promptQueue.Enqueue(ctx, entry); err != nil {
		handler.releaseActionAttachments(entry)
		handler.logger.Error("[mobile-session] existing task preparation failed", "task_id", taskID, "action_id", actionID, "action_kind", kind, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	busy := task.State == taskstate.Working || task.State == taskstate.WaitingForApproval || task.State == taskstate.WaitingForAnswer
	if busy && !redirect {
		return existingTaskAccepted, "", "queued"
	}
	if redirect && (!task.CanRedirect || task.State != taskstate.Working) {
		return existingTaskAccepted, "", "queued"
	}
	result, dispatchErr := handler.promptQueue.DispatchNext(ctx, taskID, func(ctx context.Context, queued promptqueue.Entry) (promptqueue.Result, error) {
		var controlled taskadapter.ExistingTaskResult
		var controlErr error
		code := "accepted"
		if redirect {
			code = "redirected"
			controlled, controlErr = handler.redirectQueuedExistingTurn(ctx, queued)
		} else {
			controlled, controlErr = handler.startQueuedExistingTurn(ctx, queued)
		}
		if errors.Is(controlErr, taskadapter.ErrTaskBusy) || errors.Is(controlErr, taskadapter.ErrTaskNotBusy) || errors.Is(controlErr, taskadapter.ErrRedirectUnsupported) || errors.Is(controlErr, taskadapter.ErrTaskLookupTransient) {
			handler.logger.Info("[mobile-session] existing task control deferred", "task_id", queued.ThreadID, "action_id", actionID, "action_kind", kind, "branch_reason", existingTaskErrorCode(controlErr, true))
			return promptqueue.Result{}, promptqueue.ErrSendDeferred
		}
		if writeOutcomeUnknown(controlErr) {
			handler.logger.Warn("[mobile-session] existing task control outcome unknown", "task_id", queued.ThreadID, "action_id", actionID, "action_kind", kind, "error_class", fmt.Sprintf("%T", controlErr))
			return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
		}
		if controlErr != nil {
			handler.logger.Error("[mobile-session] existing task control failed", "task_id", queued.ThreadID, "action_id", actionID, "action_kind", kind, "branch_reason", existingTaskErrorCode(controlErr, true), "error_class", fmt.Sprintf("%T", controlErr))
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		handler.logger.Info("[mobile-session] existing task control confirmed", "task_id", controlled.ThreadID, "turn_id", controlled.TurnID, "action_id", actionID, "action_kind", kind, "result_code", code)
		return promptqueue.Result{Code: code, ThreadID: controlled.ThreadID, TurnID: controlled.TurnID}, nil
	}, nil, handler.now())
	if errors.Is(dispatchErr, promptqueue.ErrSendDeferred) {
		return existingTaskAccepted, "", "queued"
	}
	if errors.Is(dispatchErr, promptqueue.ErrOutcomeUnknown) {
		return existingTaskOutcomeUnknown, "", ""
	}
	if dispatchErr != nil {
		handler.releaseActionAttachments(entry)
		handler.logger.Error("[mobile-session] existing task dispatch failed", "task_id", taskID, "action_id", actionID, "action_kind", kind, "error_class", fmt.Sprintf("%T", dispatchErr), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	handler.releaseActionAttachments(entry)
	return existingTaskAccepted, "", result.Code
}

func existingTaskErrorCode(err error, idMatches bool) string {
	if err == nil {
		if !idMatches {
			return "task_id_mismatch"
		}
		return "none"
	}
	switch {
	case errors.Is(err, taskadapter.ErrTaskBusy):
		return "task_busy"
	case errors.Is(err, taskadapter.ErrTaskNotBusy):
		return "task_not_busy"
	case errors.Is(err, taskadapter.ErrRedirectUnsupported):
		return "redirect_unsupported"
	case errors.Is(err, taskadapter.ErrTaskUnavailable):
		return "task_unavailable"
	case errors.Is(err, taskadapter.ErrTaskLookupTransient):
		return "task_lookup_transient"
	case errors.Is(err, taskadapter.ErrInvalidTaskControl):
		return "invalid_task_control"
	case errors.Is(err, taskadapter.ErrPartialNewTask):
		return "partial_turn_result"
	case errors.Is(err, appserver.ErrInvalidInput):
		return "app_server_invalid_input"
	default:
		return "unclassified"
	}
}

// stopExistingTask reports which of the eleven phone-accepted codes explains
// a failure, the same way startExistingTask does; the code is meaningless on
// the existingTaskAccepted and existingTaskOutcomeUnknown outcomes.
func (handler *Handler) stopExistingTask(ctx context.Context, actionID string, taskID string) (existingTaskOutcome, actionFailureCode, string) {
	if handler.existingTaskSource == nil || handler.promptQueue == nil {
		handler.logger.Info("[mobile-session] interrupt rejected", "task_id", taskID, "action_id", actionID, "branch_reason", "no_task_management_component", "error_code", string(failureDesktopIncompatible))
		return existingTaskFailed, failureDesktopIncompatible, ""
	}
	requestHash := newTaskRequestHash("interrupt_turn", taskID)
	prepared := false
	stored, err := handler.promptQueue.Entry(ctx, actionID)
	switch {
	case err == nil:
		if stored.RequestHash != requestHash || stored.ThreadID != taskID || stored.ActionKind != "interrupt_turn" {
			handler.logger.Warn("[mobile-session] duplicate interrupt action rejected", "task_id", taskID, "action_id", actionID, "branch_reason", "request_hash_mismatch", "error_code", string(failureInvalidAction))
			return existingTaskFailed, failureInvalidAction, ""
		}
		switch stored.State {
		case promptqueue.StateConfirmed:
			return existingTaskAccepted, "", stored.Result.Code
		case promptqueue.StateSentUnknown:
			return existingTaskOutcomeUnknown, "", ""
		case promptqueue.StatePrepared:
			prepared = true
		case promptqueue.StateFailed, promptqueue.StateCanceled:
			handler.logger.Info("[mobile-session] interrupt rejected", "task_id", taskID, "action_id", actionID, "branch_reason", "prepared_entry_unusable_state", "error_code", string(failureInternal))
			return existingTaskFailed, failureInternal, ""
		}
	case !errors.Is(err, promptqueue.ErrActionNotFound):
		handler.logger.Error("[mobile-session] interrupt lookup failed", "task_id", taskID, "action_id", actionID, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	queueKey := "action:" + actionID
	entry := promptqueue.Entry{
		ActionID: actionID, QueueKey: queueKey, ActionKind: "interrupt_turn", ThreadID: taskID,
		RequestHash: requestHash, CreatedAt: handler.now(),
	}
	if !prepared {
		if err := handler.promptQueue.Enqueue(ctx, entry); err != nil {
			handler.logger.Error("[mobile-session] interrupt preparation failed", "task_id", taskID, "action_id", actionID, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
			return existingTaskFailed, failureInternal, ""
		}
	}
	result, err := handler.promptQueue.DispatchNext(ctx, queueKey, func(ctx context.Context, queued promptqueue.Entry) (promptqueue.Result, error) {
		controlled, interruptErr := handler.existingTaskSource.InterruptExistingTurn(ctx, queued.ThreadID)
		if writeOutcomeUnknown(interruptErr) {
			handler.logger.Warn("[mobile-session] task interrupt outcome unknown", "task_id", queued.ThreadID, "action_id", actionID, "error_class", fmt.Sprintf("%T", interruptErr))
			return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
		}
		if interruptErr != nil {
			handler.logger.Error("[mobile-session] task interrupt failed", "task_id", queued.ThreadID, "action_id", actionID, "branch_reason", existingTaskErrorCode(interruptErr, true), "error", interruptErr, "error_class", fmt.Sprintf("%T", interruptErr))
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		handler.logger.Info("[mobile-session] task interrupt confirmed", "task_id", controlled.ThreadID, "turn_id", controlled.TurnID, "action_id", actionID)
		return promptqueue.Result{Code: "interrupted", ThreadID: controlled.ThreadID, TurnID: controlled.TurnID}, nil
	}, nil, handler.now())
	if errors.Is(err, promptqueue.ErrOutcomeUnknown) {
		return existingTaskOutcomeUnknown, "", ""
	}
	if err != nil {
		handler.logger.Error("[mobile-session] interrupt dispatch failed", "task_id", taskID, "action_id", actionID, "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return existingTaskFailed, failureInternal, ""
	}
	return existingTaskAccepted, "", result.Code
}

type newTaskOutcome uint8

const (
	newTaskFailed newTaskOutcome = iota
	newTaskConfirmed
	newTaskOutcomeUnknown
)

// startNewTask reports which of the eleven phone-accepted codes explains a
// failure, rather than collapsing every ending into "invalid_action". Each
// return carries the actionFailureCode the caller should use; that value is
// meaningless on the newTaskConfirmed and newTaskOutcomeUnknown outcomes.
func (handler *Handler) startNewTask(ctx context.Context, deviceID, actionID, projectID, prompt, modelID, reasoningID, permissionModeID string, attachmentIDs []string) (newTaskOutcome, actionFailureCode) {
	if handler.newTaskSource == nil || handler.optionSource == nil || handler.promptQueue == nil {
		handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "branch_reason", "no_task_management_component", "error_code", string(failureDesktopIncompatible))
		return newTaskFailed, failureDesktopIncompatible
	}
	requestHash := requestHashWithAttachments([]string{projectID, prompt, modelID, reasoningID, permissionModeID}, deviceID, attachmentIDs)
	var prepared *promptqueue.Entry
	stored, storedErr := handler.promptQueue.Entry(ctx, actionID)
	switch {
	case storedErr == nil:
		if stored.RequestHash != requestHash || stored.DeviceID != attachmentDeviceID(deviceID, attachmentIDs) || !slices.Equal(stored.AttachmentIDs, attachmentIDs) {
			handler.logger.Warn("[mobile-session] duplicate new task rejected", "action_id", actionID, "branch_reason", "request_hash_mismatch", "error_code", string(failureInvalidAction))
			return newTaskFailed, failureInvalidAction
		}
		switch stored.State {
		case promptqueue.StateConfirmed:
			return newTaskConfirmed, ""
		case promptqueue.StateFailed, promptqueue.StateCanceled:
			handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "branch_reason", "prepared_entry_unusable_state", "error_code", string(failureInternal))
			return newTaskFailed, failureInternal
		case promptqueue.StateSentUnknown:
			return newTaskOutcomeUnknown, ""
		case promptqueue.StatePrepared:
			prepared = &stored
		default:
			handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "branch_reason", "prepared_entry_unusable_state", "error_code", string(failureInternal))
			return newTaskFailed, failureInternal
		}
	case !errors.Is(storedErr, promptqueue.ErrActionNotFound):
		handler.logger.Error("[mobile-session] new task lookup failed", "action_id", actionID, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", storedErr), "error_code", string(failureInternal))
		return newTaskFailed, failureInternal
	}
	projectPath, err := handler.projects.Resolve(projectID)
	if err != nil {
		handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "project_id", projectID, "branch_reason", "project_not_resolved", "error_code", string(failureInvalidAction))
		return newTaskFailed, failureInvalidAction
	}
	catalog, err := handler.optionSource.NewTaskOptions(ctx)
	if err != nil {
		handler.logger.Error("[mobile-session] new task option reload failed", "action_id", actionID, "branch_reason", "catalog_unavailable", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		return newTaskFailed, failureInternal
	}
	model, _, ok := resolveNewTaskOptions(catalog, modelID, reasoningID, permissionModeID)
	if !ok {
		handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "branch_reason", "stale_or_unknown_option", "error_code", string(failureInvalidAction))
		return newTaskFailed, failureInvalidAction
	}
	queueKey := "new:" + actionID
	entry := promptqueue.Entry{
		ActionID: actionID, QueueKey: queueKey, ProjectID: projectID, Prompt: prompt, Model: model.WireName,
		Effort: reasoningID, PermissionMode: permissionModeID,
		RequestHash: requestHash, DeviceID: attachmentDeviceID(deviceID, attachmentIDs), AttachmentIDs: append([]string(nil), attachmentIDs...), CreatedAt: handler.now(),
	}
	if prepared != nil {
		if prepared.Model != entry.Model {
			_ = handler.promptQueue.CancelThread(ctx, queueKey, "stale_model_mapping", handler.now())
			handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "branch_reason", "stale_model_mapping", "error_code", string(failureInvalidAction))
			return newTaskFailed, failureInvalidAction
		}
		entry = *prepared
	} else {
		if err := handler.claimActionAttachments(deviceID, attachmentIDs, actionID); err != nil {
			handler.logger.Error("[mobile-session] new task attachment claim failed", "action_id", actionID, "branch_reason", "attachment_claim_failed", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
			return newTaskFailed, failureInternal
		}
		if err := handler.promptQueue.Enqueue(ctx, entry); err != nil {
			handler.releaseActionAttachments(entry)
			handler.logger.Error("[mobile-session] new task preparation failed", "action_id", actionID, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
			return newTaskFailed, failureInternal
		}
	}
	_, err = handler.promptQueue.DispatchNext(ctx, queueKey, func(ctx context.Context, queued promptqueue.Entry) (promptqueue.Result, error) {
		sandbox, mapped := permissionSandbox(queued.PermissionMode)
		if !mapped {
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		attachmentInputs, attachmentErr := handler.resolveActionAttachments(queued)
		if attachmentErr != nil {
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		started, startErr := handler.newTaskSource.StartNewTask(ctx, taskadapter.NewTaskRequest{
			ProjectPath: projectPath, Prompt: queued.Prompt, Model: queued.Model, Effort: queued.Effort,
			Sandbox: sandbox, ApprovalPolicy: json.RawMessage(`"on-request"`), Attachments: attachmentInputs,
		})
		if startErr != nil {
			var unknown *appserver.OutcomeUnknownError
			if errors.As(startErr, &unknown) || errors.Is(startErr, taskadapter.ErrPartialNewTask) {
				return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
			}
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		return promptqueue.Result{Code: "accepted", ThreadID: started.ThreadID, TurnID: started.TurnID}, nil
	}, nil, handler.now())
	if err != nil {
		if errors.Is(err, promptqueue.ErrOutcomeUnknown) {
			handler.logger.Error("[mobile-session] new task dispatch failed", "action_id", actionID, "project_id", projectID, "error_class", fmt.Sprintf("%T", err), "error_code", "outcome_unknown")
			return newTaskOutcomeUnknown, ""
		}
		handler.logger.Error("[mobile-session] new task dispatch failed", "action_id", actionID, "project_id", projectID, "error_class", fmt.Sprintf("%T", err), "error_code", string(failureInternal))
		handler.releaseActionAttachments(entry)
		return newTaskFailed, failureInternal
	}
	handler.releaseActionAttachments(entry)
	return newTaskConfirmed, ""
}

func newTaskRequestHash(values ...string) string {
	encoded, _ := json.Marshal(values)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func requestHashWithAttachments(values []string, deviceID string, attachmentIDs []string) string {
	if len(attachmentIDs) == 0 {
		return newTaskRequestHash(values...)
	}
	withAttachments := append(append([]string(nil), values...), deviceID)
	withAttachments = append(withAttachments, attachmentIDs...)
	return newTaskRequestHash(withAttachments...)
}

func attachmentDeviceID(deviceID string, attachmentIDs []string) string {
	if len(attachmentIDs) == 0 {
		return ""
	}
	return deviceID
}

func (handler *Handler) claimActionAttachments(deviceID string, attachmentIDs []string, actionID string) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	if handler.attachmentStore == nil {
		return attachments.ErrAttachmentUnavailable
	}
	return handler.attachmentStore.Claim(deviceID, attachmentIDs, actionID, handler.now())
}

func (handler *Handler) resolveActionAttachments(entry promptqueue.Entry) ([]taskadapter.AttachmentInput, error) {
	if len(entry.AttachmentIDs) == 0 {
		return nil, nil
	}
	if handler.attachmentStore == nil {
		return nil, attachments.ErrAttachmentUnavailable
	}
	resolved, err := handler.attachmentStore.ResolveClaimed(entry.DeviceID, entry.AttachmentIDs, entry.ActionID, handler.now())
	if err != nil {
		return nil, err
	}
	result := make([]taskadapter.AttachmentInput, len(resolved))
	for index, attachment := range resolved {
		result[index] = taskadapter.AttachmentInput{ID: attachment.ID, Path: attachment.Path, MediaType: attachment.MediaType}
	}
	return result, nil
}

func (handler *Handler) releaseActionAttachments(entry promptqueue.Entry) {
	if len(entry.AttachmentIDs) == 0 || handler.attachmentStore == nil {
		return
	}
	if err := handler.attachmentStore.ReleaseClaimed(entry.DeviceID, entry.AttachmentIDs, entry.ActionID); err != nil && !errors.Is(err, attachments.ErrAttachmentUnavailable) {
		handler.logger.Error("[mobile-session] attachment cleanup failed", "action_id", entry.ActionID, "device_id", entry.DeviceID, "attachment_count", len(entry.AttachmentIDs), "decision", "retain_bounded_claim", "error_class", fmt.Sprintf("%T", err))
	}
}

func (handler *Handler) releaseCanceledAttachments(entries []promptqueue.Entry, includeUnknown bool) {
	for _, entry := range entries {
		if entry.State == promptqueue.StatePrepared || includeUnknown && entry.State == promptqueue.StateSentUnknown {
			handler.releaseActionAttachments(entry)
		}
	}
}

func resolveNewTaskOptions(catalog taskoptions.Catalog, modelID, reasoningID, permissionModeID string) (taskoptions.Model, taskoptions.PermissionMode, bool) {
	var model taskoptions.Model
	for _, candidate := range catalog.Models {
		if candidate.ID == modelID {
			model = candidate
			break
		}
	}
	if model.ID == "" {
		return taskoptions.Model{}, taskoptions.PermissionMode{}, false
	}
	reasoningFound := false
	for _, reasoning := range model.Reasoning {
		reasoningFound = reasoningFound || reasoning.ID == reasoningID
	}
	var permission taskoptions.PermissionMode
	for _, candidate := range catalog.PermissionModes {
		if candidate.ID == permissionModeID {
			permission = candidate
			break
		}
	}
	_, permissionMapped := permissionSandbox(permission.ID)
	return model, permission, reasoningFound && permission.ID != "" && permissionMapped && model.WireName != ""
}

func permissionSandbox(permissionModeID string) (appserver.SandboxMode, bool) {
	switch permissionModeID {
	case "read-only":
		return appserver.SandboxReadOnly, true
	case "workspace-write":
		return appserver.SandboxWorkspaceWrite, true
	case "danger-full-access":
		return appserver.SandboxDangerFullAccess, true
	default:
		return "", false
	}
}

func setActionFailure(result map[string]any, code string, retryable bool) {
	result["state"] = "failed"
	result["error"] = map[string]any{"code": code, "retryable": retryable}
}

func setActionOutcomeUnknown(result map[string]any) {
	result["state"] = "outcome_unknown"
	result["error"] = map[string]any{"code": "outcome_unknown", "retryable": false}
}

func (handler *Handler) scheduleTaskSnapshotRefresh() {
	go func() {
		for attempt := 1; attempt <= maxTaskRefreshAttempts; attempt++ {
			if !handler.taskRefreshWait(handler.ctx, attempt) {
				return
			}
			handler.publishMu.Lock()
			snapshot, err := handler.refreshTaskSnapshot(handler.ctx)
			if err == nil {
				var state snapshotState
				if json.Unmarshal(snapshot.Body, &state) != nil {
					err = ErrInvalidTaskEvent
				} else {
					var body json.RawMessage
					body, err = validatedSnapshotBody(snapshot.BaseSequence, state)
					if err == nil {
						handler.queueBroadcast("snapshot", snapshot.BaseSequence, body)
					}
				}
			}
			handler.publishMu.Unlock()
			if err == nil {
				handler.logger.Info("[mobile-session] task snapshot refresh recovered", "attempt", attempt, "base_sequence", snapshot.BaseSequence)
				return
			}
			handler.logger.Error("[mobile-session] task snapshot refresh retry failed", "attempt", attempt, "decision", "retry_or_reconnect", "error_class", fmt.Sprintf("%T", err))
		}
		recipients, _ := handler.activeView.Load().([]transport.MessageSender)
		for _, recipient := range recipients {
			recipient.Close()
		}
		handler.logger.Error("[mobile-session] task snapshot refresh retries exhausted", "recipient_count", len(recipients), "decision", "force_fresh_reconnect")
	}()
}

func waitForTaskRefreshRetry(ctx context.Context, attempt int) bool {
	delay := time.Duration(100*(1<<(attempt-1))) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type taskPageBody struct {
	RequestID     string                 `json:"requestId"`
	TaskID        string                 `json:"taskId"`
	Entries       []tasktranscript.Entry `json:"entries"`
	EarlierCursor string                 `json:"earlierCursor,omitempty"`
	Truncated     bool                   `json:"truncated"`
	Error         *taskPageError         `json:"error,omitempty"`
}

type taskPageError struct {
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
}

func (handler *Handler) handleTaskRead(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	if handler.transcriptSource == nil {
		return ErrUnsupportedMessage
	}
	var request struct {
		RequestID     string `json:"requestId"`
		TaskID        string `json:"taskId"`
		Limit         int    `json:"limit"`
		BeforeEntryID string `json:"beforeEntryId"`
	}
	if err := json.Unmarshal(message.Body, &request); err != nil {
		return err
	}
	handler.logger.Info("[mobile-session] transcript read requested", "device_id", sender.DeviceID(), "task_id", request.TaskID, "input_limit", request.Limit, "has_cursor", request.BeforeEntryID != "")
	page, readErr := handler.transcriptSource.ReadTranscript(ctx, request.TaskID, tasktranscript.PageOptions{
		TaskID: request.TaskID, BeforeEntryID: request.BeforeEntryID, Limit: request.Limit,
	})
	if readErr == nil && page.TaskID != request.TaskID {
		readErr = tasktranscript.ErrTaskMismatch
	}
	response := taskPageBody{RequestID: request.RequestID, TaskID: request.TaskID, Entries: []tasktranscript.Entry{}}
	if readErr != nil {
		response.Error = transcriptReadError(readErr)
		handler.logger.Error("[mobile-session] transcript read failed", "device_id", sender.DeviceID(), "task_id", request.TaskID, "error_class", fmt.Sprintf("%T", readErr), "error_code", response.Error.Code)
	} else {
		response.Entries = page.Entries
		response.EarlierCursor = page.EarlierCursor
		response.Truncated = page.Truncated
		handler.logger.Info("[mobile-session] transcript read ready", "device_id", sender.DeviceID(), "task_id", request.TaskID, "output_count", len(page.Entries), "has_earlier", page.EarlierCursor != "", "truncated", page.Truncated)
	}
	body, err := validatedTaskPageBody(response)
	if err != nil {
		handler.logger.Error("[mobile-session] transcript page rejected", "device_id", sender.DeviceID(), "task_id", request.TaskID, "branch_reason", "invalid_safe_projection", "error_class", fmt.Sprintf("%T", err))
		response = taskPageBody{
			RequestID: request.RequestID, TaskID: request.TaskID, Entries: []tasktranscript.Entry{},
			Error: &taskPageError{Code: "internal", Retryable: true},
		}
		body, err = validatedTaskPageBody(response)
		if err != nil {
			return err
		}
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	current := handler.active[sender.DeviceID()]
	if current == nil || current.ConnectionID() != sender.ConnectionID() {
		return ErrSessionSuperseded
	}
	return handler.send(ctx, sender, "task_page", nil, body)
}

func validatedTaskPageBody(response taskPageBody) (json.RawMessage, error) {
	body, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	message := contract.Message{
		Version: contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor}, MessageID: "task-page-validation",
		Sender: "companion", Type: "task_page", Body: body,
	}
	if _, err := contract.EncodeText(message); err != nil {
		return nil, err
	}
	return body, nil
}

func transcriptReadError(err error) *taskPageError {
	switch {
	case errors.Is(err, tasktranscript.ErrTaskMismatch), errors.Is(err, tasktranscript.ErrUnknownCursor), errors.Is(err, tasktranscript.ErrInvalidTranscript):
		return &taskPageError{Code: "invalid_action", Retryable: false}
	case errors.Is(err, taskstate.ErrUnresolvedTaskSource), errors.Is(err, taskstate.ErrUnknownTaskSource):
		return &taskPageError{Code: "owner_unavailable", Retryable: false}
	default:
		return &taskPageError{Code: "internal", Retryable: true}
	}
}

func (handler *Handler) PublishTaskEvent(ctx context.Context, taskEvent taskstate.MobileEvent) error {
	if handler == nil {
		return ErrMissingDependency
	}
	body, err := json.Marshal(struct {
		TaskID  string `json:"taskId"`
		Event   string `json:"event"`
		State   string `json:"state"`
		Summary string `json:"summary"`
	}{taskEvent.TaskID, taskEvent.Kind, string(taskEvent.State), taskEvent.Summary})
	if err != nil || !validTaskEventBody(body) {
		return ErrInvalidTaskEvent
	}
	handler.publishMu.Lock()
	defer handler.publishMu.Unlock()
	createdAt := handler.now()
	var journalEvent eventjournal.Event
	authorized, err := taskEvent.Authorization.RunIfValid(func() error {
		var applyErr error
		journalEvent, applyErr = handler.journal.Apply(ctx, "event", body, createdAt, func(current json.RawMessage, _ eventjournal.Event) (json.RawMessage, error) {
			var state snapshotState
			if json.Unmarshal(current, &state) != nil {
				return nil, ErrInvalidTaskEvent
			}
			found := false
			for index := range state.Tasks {
				if state.Tasks[index].TaskID != taskEvent.TaskID {
					continue
				}
				state.Tasks[index].State = string(taskEvent.State)
				state.Tasks[index].CanRedirect = taskEvent.State == taskstate.Working
				if taskEvent.State != taskstate.Working && taskEvent.State != taskstate.WaitingForApproval && taskEvent.State != taskstate.WaitingForAnswer {
					state.Tasks[index].ActiveTurnID = ""
				}
				state.Tasks[index].LastActivityAt = createdAt.UTC().Format(time.RFC3339)
				found = true
				break
			}
			if !found {
				return nil, ErrUnknownTaskEvent
			}
			if _, validationErr := validatedSnapshotBody(1, state); validationErr != nil {
				return nil, ErrInvalidTaskEvent
			}
			return json.Marshal(state)
		})
		return applyErr
	})
	if !authorized {
		return ErrTaskEventAuthorizationRevoked
	}
	if err != nil {
		return err
	}
	sequence := journalEvent.Sequence
	recipients := handler.queueBroadcast("event", sequence, body)
	handler.logger.Info("[mobile-session] live task event committed", "thread_id", taskEvent.TaskID, "event_kind", taskEvent.Kind, "task_state", taskEvent.State, "sequence", sequence, "recipient_count", recipients)
	if taskEvent.State == taskstate.IdleAfterReply || taskEvent.State == taskstate.Interrupted || taskEvent.State == taskstate.Failed {
		handler.dispatchNextQueuedPrompt(ctx, taskEvent.TaskID)
	}
	return nil
}

func (handler *Handler) dispatchNextQueuedPrompt(ctx context.Context, taskID string) {
	if handler.promptQueue == nil || handler.existingTaskSource == nil {
		return
	}
	var dispatched promptqueue.Entry
	result, err := handler.promptQueue.DispatchNext(ctx, taskID, func(ctx context.Context, queued promptqueue.Entry) (promptqueue.Result, error) {
		dispatched = queued
		started, startErr := handler.startQueuedExistingTurn(ctx, queued)
		if errors.Is(startErr, taskadapter.ErrTaskBusy) || errors.Is(startErr, taskadapter.ErrTaskLookupTransient) {
			return promptqueue.Result{}, promptqueue.ErrSendDeferred
		}
		if writeOutcomeUnknown(startErr) {
			return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
		}
		if startErr != nil {
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		return promptqueue.Result{Code: "accepted", ThreadID: started.ThreadID, TurnID: started.TurnID}, nil
	}, nil, handler.now())
	if errors.Is(err, promptqueue.ErrNoPreparedAction) || errors.Is(err, promptqueue.ErrSendDeferred) {
		return
	}
	if err != nil {
		if !errors.Is(err, promptqueue.ErrOutcomeUnknown) {
			handler.releaseActionAttachments(dispatched)
		}
		handler.logger.Error("[mobile-session] queued follow-up dispatch failed", "thread_id", taskID, "decision", "retain_durable_queue_state", "error_class", fmt.Sprintf("%T", err))
		handler.publishCurrentSnapshot(ctx, taskID, "queued_follow_up_state_changed")
		return
	}
	handler.releaseActionAttachments(dispatched)
	handler.logger.Info("[mobile-session] queued follow-up started", "thread_id", taskID, "turn_id", result.TurnID, "decision", "oldest_prepared_first")
	snapshot, refreshErr := handler.refreshTaskSnapshot(ctx)
	if refreshErr != nil {
		handler.logger.Error("[mobile-session] queued follow-up snapshot refresh failed", "thread_id", taskID, "decision", "schedule_bounded_refresh", "error_class", fmt.Sprintf("%T", refreshErr))
		handler.scheduleTaskSnapshotRefresh()
		return
	}
	var state snapshotState
	if json.Unmarshal(snapshot.Body, &state) != nil {
		return
	}
	body, validationErr := validatedSnapshotBody(snapshot.BaseSequence, state)
	if validationErr == nil {
		handler.queueBroadcast("snapshot", snapshot.BaseSequence, body)
	}
}

func (handler *Handler) startQueuedExistingTurn(ctx context.Context, queued promptqueue.Entry) (taskadapter.ExistingTaskResult, error) {
	attachmentInputs, err := handler.resolveActionAttachments(queued)
	if err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	if len(attachmentInputs) != 0 {
		if source, okay := handler.existingTaskSource.(ExistingTaskRecoveryAttachmentSource); okay && queued.OwnerSource != "" {
			return source.StartExistingTurnFromSourceWithAttachments(ctx, queued.ThreadID, queued.Prompt, taskstate.Source(queued.OwnerSource), attachmentInputs)
		}
		if source, okay := handler.existingTaskSource.(ExistingTaskAttachmentSource); okay {
			return source.StartExistingTurnWithAttachments(ctx, queued.ThreadID, queued.Prompt, attachmentInputs)
		}
		return taskadapter.ExistingTaskResult{}, attachments.ErrAttachmentUnavailable
	}
	if source, okay := handler.existingTaskSource.(ExistingTaskRecoveryControlSource); okay && queued.OwnerSource != "" {
		return source.StartExistingTurnFromSource(ctx, queued.ThreadID, queued.Prompt, taskstate.Source(queued.OwnerSource))
	}
	return handler.existingTaskSource.StartExistingTurn(ctx, queued.ThreadID, queued.Prompt)
}

func (handler *Handler) redirectQueuedExistingTurn(ctx context.Context, queued promptqueue.Entry) (taskadapter.ExistingTaskResult, error) {
	attachmentInputs, err := handler.resolveActionAttachments(queued)
	if err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	if len(attachmentInputs) != 0 {
		if source, okay := handler.existingTaskSource.(ExistingTaskAttachmentSource); okay {
			return source.RedirectExistingTurnWithAttachments(ctx, queued.ThreadID, queued.Prompt, attachmentInputs)
		}
		return taskadapter.ExistingTaskResult{}, attachments.ErrAttachmentUnavailable
	}
	return handler.existingTaskSource.RedirectExistingTurn(ctx, queued.ThreadID, queued.Prompt)
}

func writeOutcomeUnknown(err error) bool {
	var appServerUnknown *appserver.OutcomeUnknownError
	return errors.As(err, &appServerUnknown) || errors.Is(err, desktopipc.ErrWriteOutcomeUnknown)
}

func (handler *Handler) publishCurrentSnapshot(ctx context.Context, taskID, reason string) {
	snapshot, err := handler.refreshTaskSnapshot(ctx)
	if err != nil {
		handler.logger.Error("[mobile-session] queue-state snapshot refresh failed", "thread_id", taskID, "branch_reason", reason, "error_class", fmt.Sprintf("%T", err))
		return
	}
	var state snapshotState
	if json.Unmarshal(snapshot.Body, &state) != nil {
		return
	}
	body, err := validatedSnapshotBody(snapshot.BaseSequence, state)
	if err == nil {
		handler.queueBroadcast("snapshot", snapshot.BaseSequence, body)
	}
}

func (handler *Handler) queueBroadcast(messageType string, sequence uint64, body json.RawMessage) int {
	recipients, _ := handler.activeView.Load().([]transport.MessageSender)
	return handler.queueDelivery(messageType, sequence, body, recipients)
}

func (handler *Handler) queueDelivery(messageType string, sequence uint64, body json.RawMessage, recipients []transport.MessageSender) int {
	if len(recipients) == 0 {
		return 0
	}
	broadcast := outboundBroadcast{
		messageType: messageType, sequence: sequence, body: append(json.RawMessage(nil), body...),
		recipients: append([]transport.MessageSender(nil), recipients...),
	}
	select {
	case handler.broadcasts <- broadcast:
		return len(recipients)
	default:
		handler.logger.Error("[mobile-session] broadcast queue full", "message_type", messageType, "recipient_count", len(recipients), "decision", "disconnect_for_replay")
		handler.disconnectRecipients(recipients)
		return len(recipients)
	}
}

func (handler *Handler) deliverBroadcasts() {
	for {
		select {
		case <-handler.ctx.Done():
			return
		case broadcast := <-handler.broadcasts:
			var wait sync.WaitGroup
			for _, sender := range broadcast.recipients {
				wait.Add(1)
				go func(sender transport.MessageSender) {
					defer wait.Done()
					sequence := broadcast.sequence
					if err := handler.send(handler.ctx, sender, broadcast.messageType, &sequence, broadcast.body); err != nil {
						handler.disconnectRecipients([]transport.MessageSender{sender})
					}
				}(sender)
			}
			wait.Wait()
		}
	}
}

func (handler *Handler) disconnectRecipients(recipients []transport.MessageSender) {
	handler.mu.Lock()
	for _, sender := range recipients {
		current := handler.active[sender.DeviceID()]
		if current != nil && current.ConnectionID() == sender.ConnectionID() {
			delete(handler.active, sender.DeviceID())
		}
		sender.Close()
	}
	handler.storeActiveViewLocked()
	handler.mu.Unlock()
}

func (handler *Handler) storeActiveViewLocked() {
	recipients := make([]transport.MessageSender, 0, len(handler.active))
	for _, sender := range handler.active {
		recipients = append(recipients, sender)
	}
	handler.activeView.Store(recipients)
}

func validTaskEventBody(body json.RawMessage) bool {
	sequence := uint64(1)
	message := contract.Message{
		Version: contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor}, MessageID: "event-validation",
		Sender: "companion", Type: "event", Sequence: &sequence, Body: body,
	}
	_, err := contract.EncodeText(message)
	return err == nil
}

func (handler *Handler) send(ctx context.Context, sender transport.MessageSender, messageType string, sequence *uint64, body json.RawMessage) error {
	message := contract.Message{
		Version:   contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor},
		MessageID: fmt.Sprintf("companion-%d", handler.nextID.Add(1)),
		Sender:    "companion", Type: messageType, Sequence: sequence, Body: body,
	}
	sendContext, cancel := context.WithTimeout(ctx, handler.sendTimeout)
	defer cancel()
	if err := sender.Send(sendContext, message); err != nil {
		handler.logger.Error("[mobile-session] message send failed", "device_id", sender.DeviceID(), "message_type", messageType, "error_class", fmt.Sprintf("%T", err))
		return err
	}
	handler.logger.Debug("[mobile-session] message sent", "device_id", sender.DeviceID(), "message_type", messageType, "sequence", sequenceValue(sequence))
	return nil
}

func welcomeBody(sessionID string, taskCapable, transcriptCapable, managementCapable, optionsCapable, attachmentCapable, decisionCapable, capabilityCapable bool, options taskoptions.Catalog) json.RawMessage {
	capabilities := []string{"set_project"}
	if taskCapable {
		capabilities = append(capabilities, "desktop_tasks")
	}
	if transcriptCapable {
		capabilities = append(capabilities, "task_transcripts")
	}
	if managementCapable {
		capabilities = append(capabilities, "task_management")
	}
	if optionsCapable {
		capabilities = append(capabilities, "new_task_options")
	}
	if attachmentCapable {
		capabilities = append(capabilities, "attachments")
	}
	if decisionCapable {
		capabilities = append(capabilities, "decisions")
	}
	if capabilityCapable {
		capabilities = append(capabilities, "capability_actions")
	}
	body, _ := json.Marshal(struct {
		SessionID      string              `json:"sessionId"`
		Capabilities   []string            `json:"capabilities"`
		Limits         any                 `json:"limits"`
		NewTaskOptions taskoptions.Catalog `json:"newTaskOptions"`
	}{
		SessionID:      sessionID,
		Capabilities:   capabilities,
		NewTaskOptions: options,
		Limits: struct {
			MaxJSONBytes       int `json:"maxJsonBytes"`
			MaxAttachmentBytes int `json:"maxAttachmentBytes"`
			MaxDeviceUploads   int `json:"maxDeviceUploads"`
			MaxGlobalUploads   int `json:"maxGlobalUploads"`
			MaxTemporaryBytes  int `json:"maxTemporaryBytes"`
			UploadExpiry       int `json:"uploadExpirySeconds"`
		}{contract.MaxJSONFrameBytes, contract.MaxAttachmentBytes, contract.MaxDeviceUploads, contract.MaxGlobalUploads, contract.MaxTemporaryBytes, contract.UploadExpirySeconds},
	})
	return body
}

func loadSnapshotTasks(ctx context.Context, source TaskSource, queue *promptqueue.Queue) ([]snapshotTask, error) {
	if source == nil {
		return []snapshotTask{}, nil
	}
	tasks, err := source.ListRecent(ctx, taskstate.MaxHomeTasks)
	if err != nil {
		return nil, fmt.Errorf("list recent Codex tasks: %w", err)
	}
	if len(tasks) > taskstate.MaxHomeTasks {
		return nil, errors.New("Codex task catalog exceeded its requested limit")
	}
	projected := make([]snapshotTask, 0, len(tasks))
	for _, task := range tasks {
		if task.UpdatedAtUnix <= 0 {
			return nil, errors.New("Codex task has no valid activity time")
		}
		queueState := string(promptqueue.QueueEmpty)
		if queue != nil {
			status, statusErr := queue.Status(ctx, task.ID)
			if statusErr != nil {
				return nil, fmt.Errorf("read task prompt queue status: %w", statusErr)
			}
			queueState = string(status)
		}
		projected = append(projected, snapshotTask{
			TaskID: task.ID, Title: task.Title, ProjectLabel: task.ProjectLabel, State: string(task.State),
			ActiveTurnID: task.ActiveTurnID, CanRedirect: task.CanRedirect,
			QueueState:     queueState,
			LastActivityAt: time.Unix(task.UpdatedAtUnix, 0).UTC().Format(time.RFC3339),
		})
	}
	return projected, nil
}

func validatedSnapshotBody(baseSequence uint64, state snapshotState) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		BaseSequence uint64            `json:"baseSeq"`
		ComputerName string            `json:"computerName"`
		Projects     []projects.Choice `json:"projects"`
		Tasks        []snapshotTask    `json:"tasks"`
	}{baseSequence, state.ComputerName, state.Projects, state.Tasks})
	if err != nil {
		return nil, err
	}
	sequence := baseSequence
	message := contract.Message{
		Version: contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor}, MessageID: "snapshot-validation",
		Sender: "companion", Type: "snapshot", Sequence: &sequence, Body: body,
	}
	if _, err := contract.EncodeText(message); err != nil {
		return nil, err
	}
	return body, nil
}

func sequenceValue(sequence *uint64) uint64 {
	if sequence == nil {
		return 0
	}
	return *sequence
}

var _ transport.MessageHandler = (*Handler)(nil).Handle
