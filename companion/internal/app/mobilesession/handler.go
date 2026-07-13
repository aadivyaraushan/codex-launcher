package mobilesession

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskoptions"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

type TaskSource interface {
	ListRecent(context.Context, int) ([]taskstate.Task, error)
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
	ctx              context.Context
	computerName     string
	projects         *projects.Service
	journal          *eventjournal.Journal
	logger           *slog.Logger
	now              func() time.Time
	taskCapable      bool
	taskSource       TaskSource
	transcriptSource TaskTranscriptSource
	managementSource TaskManagementSource
	optionSource     NewTaskOptionsSource
	newTaskSource    NewTaskSource
	promptQueue      *promptqueue.Queue
	nextID           atomic.Uint64
	publishMu        sync.Mutex
	mu               sync.Mutex
	active           map[string]transport.MessageSender
	activeView       atomic.Value
	snapshotGen      atomic.Uint64
	broadcasts       chan outboundBroadcast
	sendTimeout      time.Duration
	taskRefreshWait  func(context.Context, int) bool
}

const (
	broadcastQueueSize     = 128
	defaultSendTimeout     = 2 * time.Second
	maxTaskRefreshAttempts = 3
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
	if projectService == nil || journal == nil || computerName == "" {
		return nil, ErrMissingDependency
	}
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	tasks, err := loadSnapshotTasks(ctx, taskSource)
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
	handler.transcriptSource, _ = taskSource.(TaskTranscriptSource)
	handler.managementSource, _ = taskSource.(TaskManagementSource)
	handler.optionSource, _ = taskSource.(NewTaskOptionsSource)
	handler.newTaskSource, _ = taskSource.(NewTaskSource)
	handler.promptQueue = promptQueue
	handler.activeView.Store([]transport.MessageSender{})
	handler.snapshotGen.Store(1)
	go handler.deliverBroadcasts()
	return handler, nil
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
		return nil
	}
	handler.mu.Lock()
	current := handler.active[sender.DeviceID()]
	handler.mu.Unlock()
	if current == nil || current.ConnectionID() != sender.ConnectionID() {
		return ErrSessionSuperseded
	}
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
	default:
		return ErrUnsupportedMessage
	}
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
	if err := handler.send(ctx, sender, "welcome", nil, welcomeBody(sender.SessionID(), handler.taskCapable, handler.transcriptSource != nil, handler.managementSource != nil, optionsCapable, options)); err != nil {
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
	if body.Resume.Mode == "warm" && body.Resume.LastAck != nil {
		events, err := handler.journal.ReplayAfter(ctx, *body.Resume.LastAck)
		if err == nil {
			for _, event := range events {
				sequence := event.Sequence
				if err := handler.send(ctx, sender, event.Name, &sequence, event.Body); err != nil {
					return err
				}
			}
			return nil
		}
		if !errors.Is(err, eventjournal.ErrCursorCompacted) {
			return err
		}
	}
	snapshot, err := handler.refreshTaskSnapshot(ctx)
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

func (handler *Handler) refreshTaskSnapshot(ctx context.Context) (eventjournal.Snapshot, error) {
	if handler.taskSource == nil {
		return handler.journal.Snapshot(handler.now())
	}
	tasks, err := loadSnapshotTasks(ctx, handler.taskSource)
	if err != nil {
		handler.logger.Error("[mobile-session] task refresh failed", "branch_reason", "catalog_unavailable", "error_class", fmt.Sprintf("%T", err))
		return eventjournal.Snapshot{}, err
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
		ActionID         string `json:"actionId"`
		Kind             string `json:"kind"`
		ProjectID        string `json:"projectId"`
		TaskID           string `json:"taskId"`
		Title            string `json:"title"`
		Text             string `json:"text"`
		ModelID          string `json:"modelId"`
		ReasoningID      string `json:"reasoningId"`
		PermissionModeID string `json:"permissionModeId"`
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
	result := map[string]any{"actionId": action.ActionID, "state": "confirmed"}
	refreshTasks := false
	switch action.Kind {
	case "start_turn":
		switch handler.startNewTask(ctx, action.ActionID, action.ProjectID, action.Text, action.ModelID, action.ReasoningID, action.PermissionModeID) {
		case newTaskConfirmed:
			refreshTasks = true
		case newTaskOutcomeUnknown:
			setActionOutcomeUnknown(result)
		case newTaskFailed:
			setActionFailure(result, "invalid_action", false)
		}
	case "set_project":
		if _, err := handler.projects.Resolve(action.ProjectID); err != nil {
			setActionFailure(result, "invalid_action", false)
		}
	case "rename_task", "archive_task", "fork_task":
		if handler.managementSource == nil {
			setActionFailure(result, "invalid_action", false)
			break
		}
		var err error
		switch action.Kind {
		case "rename_task":
			err = handler.managementSource.Rename(ctx, action.TaskID, action.Title)
		case "archive_task":
			err = handler.managementSource.Archive(ctx, action.TaskID)
		case "fork_task":
			_, err = handler.managementSource.ForkToAppServer(ctx, action.TaskID)
		}
		if err != nil {
			handler.logger.Error("[mobile-session] task management action failed", "device_id", sender.DeviceID(), "task_id", action.TaskID, "action_kind", action.Kind, "error_class", fmt.Sprintf("%T", err))
			var outcomeUnknown *appserver.OutcomeUnknownError
			if errors.As(err, &outcomeUnknown) {
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
		snapshot, refreshErr := handler.refreshTaskSnapshot(ctx)
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

type newTaskOutcome uint8

const (
	newTaskFailed newTaskOutcome = iota
	newTaskConfirmed
	newTaskOutcomeUnknown
)

func (handler *Handler) startNewTask(ctx context.Context, actionID, projectID, prompt, modelID, reasoningID, permissionModeID string) newTaskOutcome {
	if handler.newTaskSource == nil || handler.optionSource == nil || handler.promptQueue == nil {
		return newTaskFailed
	}
	requestHash := newTaskRequestHash(projectID, prompt, modelID, reasoningID, permissionModeID)
	var prepared *promptqueue.Entry
	stored, storedErr := handler.promptQueue.Entry(ctx, actionID)
	switch {
	case storedErr == nil:
		if stored.RequestHash != requestHash {
			handler.logger.Warn("[mobile-session] duplicate new task rejected", "action_id", actionID, "branch_reason", "request_hash_mismatch")
			return newTaskFailed
		}
		switch stored.State {
		case promptqueue.StateConfirmed:
			return newTaskConfirmed
		case promptqueue.StateFailed, promptqueue.StateCanceled:
			return newTaskFailed
		case promptqueue.StateSentUnknown:
			return newTaskOutcomeUnknown
		case promptqueue.StatePrepared:
			prepared = &stored
		default:
			return newTaskFailed
		}
	case !errors.Is(storedErr, promptqueue.ErrActionNotFound):
		handler.logger.Error("[mobile-session] new task lookup failed", "action_id", actionID, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", storedErr))
		return newTaskFailed
	}
	projectPath, err := handler.projects.Resolve(projectID)
	if err != nil {
		return newTaskFailed
	}
	catalog, err := handler.optionSource.NewTaskOptions(ctx)
	if err != nil {
		handler.logger.Error("[mobile-session] new task option reload failed", "action_id", actionID, "branch_reason", "catalog_unavailable", "error_class", fmt.Sprintf("%T", err))
		return newTaskFailed
	}
	model, _, ok := resolveNewTaskOptions(catalog, modelID, reasoningID, permissionModeID)
	if !ok {
		handler.logger.Info("[mobile-session] new task rejected", "action_id", actionID, "branch_reason", "stale_or_unknown_option")
		return newTaskFailed
	}
	queueKey := "new:" + actionID
	entry := promptqueue.Entry{
		ActionID: actionID, QueueKey: queueKey, ProjectID: projectID, Prompt: prompt, Model: model.WireName,
		Effort: reasoningID, PermissionMode: permissionModeID,
		RequestHash: requestHash, CreatedAt: handler.now(),
	}
	if prepared != nil {
		if prepared.Model != entry.Model {
			_ = handler.promptQueue.CancelThread(ctx, queueKey, "stale_model_mapping", handler.now())
			return newTaskFailed
		}
		entry = *prepared
	} else {
		if err := handler.promptQueue.Enqueue(ctx, entry); err != nil {
			handler.logger.Error("[mobile-session] new task preparation failed", "action_id", actionID, "branch_reason", "durable_queue_unavailable", "error_class", fmt.Sprintf("%T", err))
			return newTaskFailed
		}
	}
	_, err = handler.promptQueue.DispatchNext(ctx, queueKey, func(ctx context.Context, queued promptqueue.Entry) (promptqueue.Result, error) {
		sandbox, mapped := permissionSandbox(queued.PermissionMode)
		if !mapped {
			return promptqueue.Result{}, promptqueue.ErrSendNotSent
		}
		started, startErr := handler.newTaskSource.StartNewTask(ctx, taskadapter.NewTaskRequest{
			ProjectPath: projectPath, Prompt: queued.Prompt, Model: queued.Model, Effort: queued.Effort,
			Sandbox: sandbox, ApprovalPolicy: json.RawMessage(`"on-request"`),
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
		handler.logger.Error("[mobile-session] new task dispatch failed", "action_id", actionID, "project_id", projectID, "error_class", fmt.Sprintf("%T", err))
		if errors.Is(err, promptqueue.ErrOutcomeUnknown) {
			return newTaskOutcomeUnknown
		}
		return newTaskFailed
	}
	return newTaskConfirmed
}

func newTaskRequestHash(values ...string) string {
	encoded, _ := json.Marshal(values)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
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
	return nil
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

func welcomeBody(sessionID string, taskCapable, transcriptCapable, managementCapable, optionsCapable bool, options taskoptions.Catalog) json.RawMessage {
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

func loadSnapshotTasks(ctx context.Context, source TaskSource) ([]snapshotTask, error) {
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
		projected = append(projected, snapshotTask{
			TaskID: task.ID, Title: task.Title, ProjectLabel: task.ProjectLabel, State: string(task.State),
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
