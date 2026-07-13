package mobilesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

type TaskSource interface {
	ListRecent(context.Context, int) ([]taskstate.Task, error)
}

var (
	ErrMissingDependency  = errors.New("mobile session dependency is missing")
	ErrUnsupportedMessage = errors.New("mobile session message is unsupported")
	ErrSessionSuperseded  = errors.New("mobile session was replaced")
)

type Handler struct {
	computerName string
	projects     *projects.Service
	journal      *eventjournal.Journal
	logger       *slog.Logger
	now          func() time.Time
	taskCapable  bool
	taskSource   TaskSource
	nextID       atomic.Uint64
	mu           sync.Mutex
	active       map[string]transport.MessageSender
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
	return &Handler{
		computerName: computerName, projects: projectService, journal: journal, logger: logger, now: now, taskCapable: taskSource != nil, taskSource: taskSource,
		active: make(map[string]transport.MessageSender),
	}, nil
}

func (handler *Handler) Handle(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	if handler == nil || sender == nil || sender.ConnectionID() == 0 {
		return ErrMissingDependency
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if message.Type == "hello" {
		previous := handler.active[sender.DeviceID()]
		handler.active[sender.DeviceID()] = sender
		if previous != nil && previous.ConnectionID() != sender.ConnectionID() {
			previous.Close()
			handler.logger.Info("[mobile-session] older device session replaced", "device_id", sender.DeviceID(), "session_id", sender.SessionID(), "decision", "single_active_session")
		}
	} else if current := handler.active[sender.DeviceID()]; current == nil || current.ConnectionID() != sender.ConnectionID() {
		return ErrSessionSuperseded
	}
	handler.logger.Info("[mobile-session] message received", "device_id", sender.DeviceID(), "message_type", message.Type, "input_shape", "validated_protocol_message")
	switch message.Type {
	case "hello":
		return handler.handleHello(ctx, sender, message)
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
	default:
		return ErrUnsupportedMessage
	}
}

func (handler *Handler) handleHello(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	if err := handler.refreshTaskSnapshot(ctx); err != nil {
		return err
	}
	if err := handler.send(ctx, sender, "welcome", nil, welcomeBody(sender.SessionID(), handler.taskCapable)); err != nil {
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
	snapshot, err := handler.journal.Snapshot(handler.now())
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

func (handler *Handler) refreshTaskSnapshot(ctx context.Context) error {
	if handler.taskSource == nil {
		return nil
	}
	tasks, err := loadSnapshotTasks(ctx, handler.taskSource)
	if err != nil {
		handler.logger.Error("[mobile-session] task refresh failed", "branch_reason", "catalog_unavailable", "error_class", fmt.Sprintf("%T", err))
		return err
	}
	state := snapshotState{ComputerName: handler.computerName, Projects: handler.projects.List(), Tasks: tasks}
	if _, err := validatedSnapshotBody(1, state); err != nil {
		handler.logger.Error("[mobile-session] task refresh rejected", "branch_reason", "invalid_safe_projection", "error_class", fmt.Sprintf("%T", err))
		return err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	snapshot, err := handler.journal.ReplaceSnapshot(ctx, encoded, handler.now())
	if err != nil {
		return err
	}
	handler.logger.Info("[mobile-session] task snapshot refreshed", "task_count", len(tasks), "base_sequence", snapshot.BaseSequence, "output_shape", "safe_task_summaries")
	return nil
}

func (handler *Handler) handleAction(ctx context.Context, sender transport.MessageSender, message contract.Message) error {
	var action struct {
		ActionID  string `json:"actionId"`
		Kind      string `json:"kind"`
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal(message.Body, &action); err != nil {
		return err
	}
	if action.Kind != "set_project" {
		return ErrUnsupportedMessage
	}
	result := map[string]any{"actionId": action.ActionID, "state": "confirmed"}
	if _, err := handler.projects.Resolve(action.ProjectID); err != nil {
		result["state"] = "failed"
		result["error"] = map[string]any{"code": "invalid_action", "retryable": false}
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
	handler.logger.Info("[mobile-session] project action resolved", "device_id", sender.DeviceID(), "project_id", action.ProjectID, "result_state", result["state"])
	return handler.send(ctx, sender, "action_result", &sequence, body)
}

func (handler *Handler) send(ctx context.Context, sender transport.MessageSender, messageType string, sequence *uint64, body json.RawMessage) error {
	message := contract.Message{
		Version:   contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor},
		MessageID: fmt.Sprintf("companion-%d", handler.nextID.Add(1)),
		Sender:    "companion", Type: messageType, Sequence: sequence, Body: body,
	}
	if err := sender.Send(ctx, message); err != nil {
		handler.logger.Error("[mobile-session] message send failed", "device_id", sender.DeviceID(), "message_type", messageType, "error_class", fmt.Sprintf("%T", err))
		return err
	}
	handler.logger.Debug("[mobile-session] message sent", "device_id", sender.DeviceID(), "message_type", messageType, "sequence", sequenceValue(sequence))
	return nil
}

func welcomeBody(sessionID string, taskCapable bool) json.RawMessage {
	capabilities := []string{"set_project"}
	if taskCapable {
		capabilities = append(capabilities, "desktop_tasks")
	}
	body, _ := json.Marshal(struct {
		SessionID    string   `json:"sessionId"`
		Capabilities []string `json:"capabilities"`
		Limits       any      `json:"limits"`
	}{
		SessionID:    sessionID,
		Capabilities: capabilities,
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
	tasks, err := source.ListRecent(ctx, contract.MaxSnapshotTasks)
	if err != nil {
		return nil, fmt.Errorf("list recent Codex tasks: %w", err)
	}
	if len(tasks) > contract.MaxSnapshotTasks {
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
