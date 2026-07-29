package taskadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskoptions"
	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/agent/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
)

type Set struct {
	router                              *taskstate.AdapterRouter
	desktop                             *desktopipc.Client
	app                                 *appserver.Client
	catalog                             *Catalog
	startThread                         func(context.Context, appserver.ThreadOptions) (json.RawMessage, error)
	startTurn                           func(context.Context, appserver.TurnOptions) (json.RawMessage, error)
	currentTask                         func(context.Context, string) (taskstate.Task, error)
	startExistingTurn                   func(context.Context, taskstate.Task, string) (string, error)
	redirectExistingTurn                func(context.Context, taskstate.Task, string) (string, error)
	startExistingTurnWithAttachments    func(context.Context, taskstate.Task, string, []AttachmentInput) (string, error)
	redirectExistingTurnWithAttachments func(context.Context, taskstate.Task, string, []AttachmentInput) (string, error)
	interruptExistingTurn               func(context.Context, taskstate.Task) (string, error)
	readAppTask                         func(context.Context, string) (json.RawMessage, error)
	readDesktopTask                     func(context.Context, string) (json.RawMessage, error)
}

var (
	ErrDesktopUnavailable  = errors.New("desktop adapter is unavailable")
	ErrInvalidNewTask      = errors.New("new task request or result is invalid")
	ErrPartialNewTask      = errors.New("new task thread exists but its first turn is unconfirmed")
	ErrTaskBusy            = errors.New("task is busy")
	ErrTaskNotBusy         = errors.New("task is not busy")
	ErrRedirectUnsupported = errors.New("task redirect is unavailable")
	ErrInvalidTaskControl  = errors.New("existing task control is invalid")
	ErrTaskUnavailable     = errors.New("task is definitively unavailable")
	ErrTaskLookupTransient = errors.New("task lookup is temporarily unavailable")
)

type NewTaskRequest struct {
	ProjectPath    string
	Prompt         string
	Model          string
	Effort         string
	Sandbox        appserver.SandboxMode
	ApprovalPolicy json.RawMessage
	Attachments    []AttachmentInput
}

type AttachmentInput struct {
	ID        string
	Path      string
	MediaType string
}

type NewTaskResult struct {
	ThreadID string
	TurnID   string
}

type ExistingTaskResult struct {
	ThreadID string
	TurnID   string
}

func New(desktop *desktopipc.Client, appServer *appserver.Client) (Set, error) {
	if desktop == nil || appServer == nil {
		return Set{}, errors.New("desktop and app-server adapters are required")
	}
	router, err := taskstate.NewAdapterRouter(desktop, appServer)
	if err != nil {
		return Set{}, err
	}
	catalog, err := NewCatalog(desktop, appServer)
	if err != nil {
		return Set{}, err
	}
	set := Set{router: &router, desktop: desktop, app: appServer, catalog: catalog, startThread: appServer.StartThread, startTurn: appServer.StartTurn}
	set.configureExistingTaskControls()
	return set, nil
}

func NewAppServerOnly(appServer *appserver.Client) (Set, error) {
	if appServer == nil {
		return Set{}, errors.New("app-server adapter is required")
	}
	catalog, err := NewAppServerCatalog(appServer)
	if err != nil {
		return Set{}, err
	}
	set := Set{app: appServer, catalog: catalog, startThread: appServer.StartThread, startTurn: appServer.StartTurn}
	set.configureExistingTaskControls()
	return set, nil
}

func (set *Set) configureExistingTaskControls() {
	if set.app != nil {
		set.readAppTask = func(ctx context.Context, taskID string) (json.RawMessage, error) {
			return set.app.ReadThread(ctx, taskID, true)
		}
	}
	if set.desktop != nil {
		set.readDesktopTask = func(ctx context.Context, taskID string) (json.RawMessage, error) {
			if _, err := set.desktop.LoadCompleteHistory(ctx, taskID); err != nil {
				set.desktop.RevokeMobileEvents(taskID)
				return nil, err
			}
			materialized := set.desktop.Stream(taskID).State().Materialized
			if len(materialized) == 0 {
				set.desktop.RevokeMobileEvents(taskID)
				return nil, errors.New("desktop owner returned no task state")
			}
			if err := set.desktop.AuthorizeMobileEvents(taskID); err != nil {
				set.desktop.RevokeMobileEvents(taskID)
				return nil, err
			}
			return append(json.RawMessage(nil), materialized...), nil
		}
	}
	set.startExistingTurnWithAttachments = func(ctx context.Context, task taskstate.Task, text string, attachments []AttachmentInput) (string, error) {
		var raw json.RawMessage
		var err error
		switch task.Source {
		case taskstate.SourceDesktop:
			raw, err = set.desktop.StartTurnWithAttachments(ctx, task.ID, text, desktopAttachments(attachments))
			if err == nil {
				return nestedNestedResultID(raw, "result", "turn"), nil
			}
		case taskstate.SourceAppServer:
			raw, err = set.app.StartTurn(ctx, appserver.TurnOptions{ThreadID: task.ID, Text: text, Attachments: appServerAttachments(attachments)})
			if err == nil {
				return nestedResultID(raw, "turn"), nil
			}
		default:
			return "", taskstate.ErrUnresolvedTaskSource
		}
		return "", err
	}
	set.startExistingTurn = func(ctx context.Context, task taskstate.Task, text string) (string, error) {
		return set.startExistingTurnWithAttachments(ctx, task, text, nil)
	}
	set.redirectExistingTurnWithAttachments = func(ctx context.Context, task taskstate.Task, text string, attachments []AttachmentInput) (string, error) {
		var raw json.RawMessage
		var err error
		switch task.Source {
		case taskstate.SourceDesktop:
			raw, err = set.desktop.SteerTurnWithAttachments(ctx, task.ID, text, desktopAttachments(attachments))
			if err == nil {
				return nestedNestedString(raw, "result", "turnId"), nil
			}
		case taskstate.SourceAppServer:
			raw, err = set.app.SteerTurnWithAttachments(ctx, task.ID, task.ActiveTurnID, text, appServerAttachments(attachments))
			if err == nil {
				return topLevelString(raw, "turnId"), nil
			}
		default:
			return "", taskstate.ErrUnresolvedTaskSource
		}
		return "", err
	}
	set.redirectExistingTurn = func(ctx context.Context, task taskstate.Task, text string) (string, error) {
		return set.redirectExistingTurnWithAttachments(ctx, task, text, nil)
	}
	set.interruptExistingTurn = func(ctx context.Context, task taskstate.Task) (string, error) {
		switch task.Source {
		case taskstate.SourceDesktop:
			_, err := set.desktop.InterruptTurn(ctx, task.ID)
			return task.ActiveTurnID, err
		case taskstate.SourceAppServer:
			_, err := set.app.InterruptTurn(ctx, task.ID, task.ActiveTurnID)
			return task.ActiveTurnID, err
		default:
			return "", taskstate.ErrUnresolvedTaskSource
		}
	}
	set.currentTask = set.reloadTask
}

func (set Set) CurrentTaskFromSource(ctx context.Context, taskID string, source taskstate.Source) (taskstate.Task, error) {
	if strings.TrimSpace(taskID) != taskID || taskID == "" {
		return taskstate.Task{}, ErrInvalidTaskControl
	}
	var raw json.RawMessage
	var err error
	switch source {
	case taskstate.SourceAppServer:
		if set.readAppTask == nil {
			return taskstate.Task{}, ErrDesktopUnavailable
		}
		raw, err = set.readAppTask(ctx, taskID)
		if appserver.IsThreadNotLoaded(err, taskID) {
			return taskstate.Task{}, ErrTaskUnavailable
		}
		if err == nil {
			raw, err = extractThread(raw)
		}
		if err == nil {
			var task taskstate.Task
			task, err = taskstate.MapAppServerThread(raw)
			if err == nil && task.ID == taskID {
				return task, nil
			}
		}
	case taskstate.SourceDesktop:
		if set.readDesktopTask == nil {
			return taskstate.Task{}, ErrDesktopUnavailable
		}
		raw, err = set.readDesktopTask(ctx, taskID)
		if err == nil {
			var task taskstate.Task
			task, err = taskstate.MapDesktopConversationState(raw)
			if err == nil && task.ID == taskID {
				return task, nil
			}
		}
	default:
		return taskstate.Task{}, taskstate.ErrUnknownTaskSource
	}
	if err == nil {
		err = ErrUnknownCatalogTask
	}
	return taskstate.Task{}, err
}

// CurrentTask resolves a task from the latest catalog before a decision is sent back to Codex.
func (set Set) CurrentTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	if set.currentTask == nil {
		return taskstate.Task{}, ErrTaskLookupTransient
	}
	return set.currentTask(ctx, taskID)
}

func (set Set) reloadTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	tasks, err := set.ListRecentCandidates(ctx, MaxRecentCatalogTasks)
	if err != nil {
		return taskstate.Task{}, err
	}
	for _, task := range tasks {
		if task.ID != taskID {
			continue
		}
		if task.Source == taskstate.SourceCatalog {
			if set.desktop == nil {
				return set.ResumeWithAppServer(ctx, taskID)
			}
			resolved, resolveErr := set.ResolveDesktopOwner(ctx, taskID)
			if resolveErr == nil {
				return resolved, nil
			}
			if errors.Is(resolveErr, desktopipc.ErrOwnerUnavailable) {
				return set.ResumeWithAppServer(ctx, taskID)
			}
			return taskstate.Task{}, resolveErr
		}
		return set.CurrentTaskFromSource(ctx, taskID, task.Source)
	}
	if set.desktop == nil && set.readAppTask != nil {
		slog.Default().Info(
			"[codex-adapter] older task missing from bounded catalog; reading directly",
			"task_id", taskID,
			"branch_reason", "app_server_only_catalog_miss",
		)
		return set.CurrentTaskFromSource(ctx, taskID, taskstate.SourceAppServer)
	}
	return taskstate.Task{}, ErrUnknownCatalogTask
}

func (set Set) StartExistingTurn(ctx context.Context, taskID, text string) (ExistingTaskResult, error) {
	if set.currentTask == nil || set.startExistingTurn == nil || strings.TrimSpace(taskID) != taskID || taskID == "" || strings.TrimSpace(text) == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.currentTask(ctx, taskID)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	return set.startExistingTurnForTask(ctx, task, text)
}

func (set Set) StartExistingTurnWithAttachments(ctx context.Context, taskID, text string, attachments []AttachmentInput) (ExistingTaskResult, error) {
	if set.currentTask == nil || set.startExistingTurnWithAttachments == nil || strings.TrimSpace(taskID) != taskID || taskID == "" || strings.TrimSpace(text) == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.currentTask(ctx, taskID)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	return set.startExistingTurnForTaskWithAttachments(ctx, task, text, attachments)
}

func (set Set) StartExistingTurnFromSource(ctx context.Context, taskID, text string, source taskstate.Source) (ExistingTaskResult, error) {
	if set.startExistingTurn == nil || strings.TrimSpace(taskID) != taskID || taskID == "" || strings.TrimSpace(text) == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.CurrentTaskFromSource(ctx, taskID, source)
	if err != nil {
		if errors.Is(err, ErrTaskUnavailable) {
			return ExistingTaskResult{}, err
		}
		return ExistingTaskResult{}, fmt.Errorf("%w: %T", ErrTaskLookupTransient, err)
	}
	return set.startExistingTurnForTask(ctx, task, text)
}

func (set Set) StartExistingTurnFromSourceWithAttachments(ctx context.Context, taskID, text string, source taskstate.Source, attachments []AttachmentInput) (ExistingTaskResult, error) {
	if set.startExistingTurnWithAttachments == nil || strings.TrimSpace(taskID) != taskID || taskID == "" || strings.TrimSpace(text) == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.CurrentTaskFromSource(ctx, taskID, source)
	if err != nil {
		if errors.Is(err, ErrTaskUnavailable) {
			return ExistingTaskResult{}, err
		}
		return ExistingTaskResult{}, fmt.Errorf("%w: %T", ErrTaskLookupTransient, err)
	}
	return set.startExistingTurnForTaskWithAttachments(ctx, task, text, attachments)
}

func (set Set) startExistingTurnForTask(ctx context.Context, task taskstate.Task, text string) (ExistingTaskResult, error) {
	if task.State == taskstate.Working || task.State == taskstate.WaitingForApproval || task.State == taskstate.WaitingForAnswer {
		return ExistingTaskResult{}, ErrTaskBusy
	}
	turnID, err := set.startExistingTurn(ctx, task, text)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	if strings.TrimSpace(turnID) == "" {
		return ExistingTaskResult{}, ErrPartialNewTask
	}
	return ExistingTaskResult{ThreadID: task.ID, TurnID: turnID}, nil
}

func (set Set) startExistingTurnForTaskWithAttachments(ctx context.Context, task taskstate.Task, text string, attachments []AttachmentInput) (ExistingTaskResult, error) {
	if task.State == taskstate.Working || task.State == taskstate.WaitingForApproval || task.State == taskstate.WaitingForAnswer {
		return ExistingTaskResult{}, ErrTaskBusy
	}
	turnID, err := set.startExistingTurnWithAttachments(ctx, task, text, attachments)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	if strings.TrimSpace(turnID) == "" {
		return ExistingTaskResult{}, ErrPartialNewTask
	}
	return ExistingTaskResult{ThreadID: task.ID, TurnID: turnID}, nil
}

func (set Set) RedirectExistingTurn(ctx context.Context, taskID, text string) (ExistingTaskResult, error) {
	if set.currentTask == nil || set.redirectExistingTurn == nil || strings.TrimSpace(taskID) != taskID || taskID == "" || strings.TrimSpace(text) == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.currentTask(ctx, taskID)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	if task.State != taskstate.Working {
		return ExistingTaskResult{}, ErrTaskNotBusy
	}
	if !task.CanRedirect || task.ActiveTurnID == "" {
		return ExistingTaskResult{}, ErrRedirectUnsupported
	}
	turnID, err := set.redirectExistingTurn(ctx, task, text)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	if strings.TrimSpace(turnID) == "" {
		return ExistingTaskResult{}, ErrPartialNewTask
	}
	return ExistingTaskResult{ThreadID: taskID, TurnID: turnID}, nil
}

func (set Set) RedirectExistingTurnWithAttachments(ctx context.Context, taskID, text string, attachments []AttachmentInput) (ExistingTaskResult, error) {
	if set.currentTask == nil || set.redirectExistingTurnWithAttachments == nil || strings.TrimSpace(taskID) != taskID || taskID == "" || strings.TrimSpace(text) == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.currentTask(ctx, taskID)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	if task.State != taskstate.Working {
		return ExistingTaskResult{}, ErrTaskNotBusy
	}
	if !task.CanRedirect || task.ActiveTurnID == "" {
		return ExistingTaskResult{}, ErrRedirectUnsupported
	}
	turnID, err := set.redirectExistingTurnWithAttachments(ctx, task, text, attachments)
	if err != nil {
		return ExistingTaskResult{}, err
	}
	if strings.TrimSpace(turnID) == "" {
		return ExistingTaskResult{}, ErrPartialNewTask
	}
	return ExistingTaskResult{ThreadID: taskID, TurnID: turnID}, nil
}

func (set Set) InterruptExistingTurn(ctx context.Context, taskID string) (ExistingTaskResult, error) {
	if set.currentTask == nil || set.interruptExistingTurn == nil || strings.TrimSpace(taskID) != taskID || taskID == "" {
		return ExistingTaskResult{}, ErrInvalidTaskControl
	}
	task, err := set.currentTask(ctx, taskID)
	if err != nil {
		slog.Default().Error("[codex-adapter] interrupt task reload failed", "task_id", taskID, "branch_reason", "reload_failed", "error", err, "error_class", fmt.Sprintf("%T", err))
		return ExistingTaskResult{}, err
	}
	slog.Default().Info(
		"[codex-adapter] interrupt task reloaded",
		"task_id", taskID,
		"source", task.Source,
		"state", task.State,
		"active_turn_id_present", task.ActiveTurnID != "",
	)
	if !taskHasActiveTurn(task) {
		slog.Default().Info("[codex-adapter] interrupt task rejected", "task_id", taskID, "branch_reason", "no_active_turn")
		return ExistingTaskResult{}, ErrTaskNotBusy
	}
	turnID, err := set.interruptExistingTurn(ctx, task)
	if err != nil {
		slog.Default().Error("[codex-adapter] interrupt write failed", "task_id", taskID, "source", task.Source, "error", err, "error_class", fmt.Sprintf("%T", err))
		return ExistingTaskResult{}, err
	}
	if turnID == "" {
		return ExistingTaskResult{}, ErrPartialNewTask
	}
	return ExistingTaskResult{ThreadID: taskID, TurnID: turnID}, nil
}

func taskHasActiveTurn(task taskstate.Task) bool {
	return task.ActiveTurnID != "" && (task.State == taskstate.Working || task.State == taskstate.WaitingForApproval || task.State == taskstate.WaitingForAnswer)
}

func (set Set) StartNewTask(ctx context.Context, request NewTaskRequest) (NewTaskResult, error) {
	if set.startThread == nil || set.startTurn == nil || strings.TrimSpace(request.ProjectPath) == "" || strings.TrimSpace(request.Prompt) == "" || strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Effort) == "" {
		return NewTaskResult{}, ErrInvalidNewTask
	}
	threadRaw, err := set.startThread(ctx, appserver.ThreadOptions{
		CWD: request.ProjectPath, Model: request.Model, ApprovalPolicy: request.ApprovalPolicy, Sandbox: request.Sandbox,
	})
	if err != nil {
		return NewTaskResult{}, err
	}
	threadID := nestedResultID(threadRaw, "thread")
	if threadID == "" {
		return NewTaskResult{}, ErrPartialNewTask
	}
	turnRaw, err := set.startTurn(ctx, appserver.TurnOptions{ThreadID: threadID, Text: request.Prompt, Effort: request.Effort, Attachments: appServerAttachments(request.Attachments)})
	if err != nil {
		return NewTaskResult{}, fmt.Errorf("%w: %v", ErrPartialNewTask, err)
	}
	turnID := nestedResultID(turnRaw, "turn")
	if turnID == "" {
		return NewTaskResult{}, ErrPartialNewTask
	}
	return NewTaskResult{ThreadID: threadID, TurnID: turnID}, nil
}

func appServerAttachments(attachments []AttachmentInput) []appserver.AttachmentInput {
	result := make([]appserver.AttachmentInput, len(attachments))
	for index, attachment := range attachments {
		result[index] = appserver.AttachmentInput{ID: attachment.ID, Path: attachment.Path, MediaType: attachment.MediaType}
	}
	return result
}

func desktopAttachments(attachments []AttachmentInput) []desktopipc.AttachmentInput {
	result := make([]desktopipc.AttachmentInput, len(attachments))
	for index, attachment := range attachments {
		result[index] = desktopipc.AttachmentInput{ID: attachment.ID, Path: attachment.Path, MediaType: attachment.MediaType}
	}
	return result
}

func nestedResultID(raw json.RawMessage, field string) string {
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) != nil {
		return ""
	}
	var value struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(response[field], &value) != nil || strings.TrimSpace(value.ID) != value.ID || value.ID == "" || len(value.ID) > 256 {
		return ""
	}
	return value.ID
}

func nestedNestedResultID(raw json.RawMessage, outer, inner string) string {
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) != nil {
		return ""
	}
	return nestedResultID(response[outer], inner)
}

func nestedNestedString(raw json.RawMessage, outer, inner string) string {
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) != nil {
		return ""
	}
	return topLevelString(response[outer], inner)
}

func topLevelString(raw json.RawMessage, field string) string {
	var response map[string]json.RawMessage
	var value string
	if json.Unmarshal(raw, &response) != nil || json.Unmarshal(response[field], &value) != nil || strings.TrimSpace(value) != value || value == "" || len(value) > 256 {
		return ""
	}
	return value
}

func (set Set) ListRecentCandidates(ctx context.Context, limit int) ([]taskstate.Task, error) {
	if set.catalog == nil {
		return nil, errors.New("task catalog is unavailable")
	}
	return set.catalog.ListRecent(ctx, limit)
}

func (set Set) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	tasks, err := set.ListRecentCandidates(ctx, limit)
	if err != nil {
		return nil, err
	}
	if len(tasks) > taskstate.MaxHomeTasks {
		tasks = tasks[:taskstate.MaxHomeTasks]
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return orderHomeTasks(tasks), nil
}

func (set Set) NewTaskOptions(ctx context.Context) (taskoptions.Catalog, error) {
	if set.app == nil {
		return taskoptions.Catalog{}, errors.New("app-server adapter is unavailable")
	}
	return taskoptions.Load(ctx, set.app.ListModels)
}

func orderHomeTasks(tasks []taskstate.Task) []taskstate.Task {
	ordered := make([]taskstate.Task, 0, len(tasks))
	for _, task := range tasks {
		if homeTaskNeedsAttention(task.State) {
			ordered = append(ordered, task)
		}
	}
	for _, task := range tasks {
		if !homeTaskNeedsAttention(task.State) {
			ordered = append(ordered, task)
		}
	}
	return ordered
}

func homeTaskNeedsAttention(state taskstate.State) bool {
	switch state {
	case taskstate.Working, taskstate.WaitingForApproval, taskstate.WaitingForAnswer, taskstate.Failed:
		return true
	default:
		return false
	}
}

func (set Set) ResolveDesktopOwner(ctx context.Context, taskID string) (taskstate.Task, error) {
	if set.catalog == nil {
		return taskstate.Task{}, errors.New("task catalog is unavailable")
	}
	return set.catalog.ResolveDesktopOwner(ctx, taskID)
}

func (set Set) ResumeWithAppServer(ctx context.Context, taskID string) (taskstate.Task, error) {
	if set.catalog == nil {
		return taskstate.Task{}, errors.New("task catalog is unavailable")
	}
	return set.catalog.ResumeWithAppServer(ctx, taskID)
}

func (set Set) Rename(ctx context.Context, taskID, name string) error {
	return set.catalog.Rename(ctx, taskID, name)
}

func (set Set) Archive(ctx context.Context, taskID string) error {
	return set.catalog.Archive(ctx, taskID)
}

func (set Set) ReadTranscript(ctx context.Context, taskID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	if set.catalog == nil {
		return tasktranscript.Page{}, errors.New("task catalog is unavailable")
	}
	return set.catalog.ReadTranscript(ctx, taskID, options)
}

func (set Set) ForkToAppServer(ctx context.Context, taskID string) (taskstate.Task, error) {
	return set.catalog.ForkToAppServer(ctx, taskID)
}

func (set Set) CurrentDesktopTask(taskID string) (taskstate.Task, error) {
	if set.desktop == nil {
		return taskstate.Task{}, errors.New("desktop adapter is unavailable")
	}
	materialized := set.desktop.Stream(taskID).State().Materialized
	if len(materialized) == 0 {
		return taskstate.Task{}, errors.New("desktop task state is not loaded")
	}
	return taskstate.MapDesktopConversationState(materialized)
}

func (set Set) For(task taskstate.Task) (taskstate.TaskAdapter, error) {
	if set.router != nil {
		return set.router.For(task)
	}
	switch task.Source {
	case taskstate.SourceAppServer:
		return set.app, nil
	case taskstate.SourceDesktop:
		return nil, ErrDesktopUnavailable
	case taskstate.SourceCatalog:
		return nil, taskstate.ErrUnresolvedTaskSource
	default:
		return nil, taskstate.ErrUnknownTaskSource
	}
}
