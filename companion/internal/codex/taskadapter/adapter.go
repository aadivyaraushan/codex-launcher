package taskadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskoptions"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
)

type Set struct {
	router      *taskstate.AdapterRouter
	desktop     *desktopipc.Client
	app         *appserver.Client
	catalog     *Catalog
	startThread func(context.Context, appserver.ThreadOptions) (json.RawMessage, error)
	startTurn   func(context.Context, appserver.TurnOptions) (json.RawMessage, error)
}

var (
	ErrDesktopUnavailable = errors.New("desktop adapter is unavailable")
	ErrInvalidNewTask     = errors.New("new task request or result is invalid")
	ErrPartialNewTask     = errors.New("new task thread exists but its first turn is unconfirmed")
)

type NewTaskRequest struct {
	ProjectPath    string
	Prompt         string
	Model          string
	Effort         string
	Sandbox        appserver.SandboxMode
	ApprovalPolicy json.RawMessage
}

type NewTaskResult struct {
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
	return Set{router: &router, desktop: desktop, app: appServer, catalog: catalog, startThread: appServer.StartThread, startTurn: appServer.StartTurn}, nil
}

func NewAppServerOnly(appServer *appserver.Client) (Set, error) {
	if appServer == nil {
		return Set{}, errors.New("app-server adapter is required")
	}
	catalog, err := NewAppServerCatalog(appServer)
	if err != nil {
		return Set{}, err
	}
	return Set{app: appServer, catalog: catalog, startThread: appServer.StartThread, startTurn: appServer.StartTurn}, nil
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
	turnRaw, err := set.startTurn(ctx, appserver.TurnOptions{ThreadID: threadID, Text: request.Prompt, Effort: request.Effort})
	if err != nil {
		return NewTaskResult{}, fmt.Errorf("%w: %v", ErrPartialNewTask, err)
	}
	turnID := nestedResultID(turnRaw, "turn")
	if turnID == "" {
		return NewTaskResult{}, ErrPartialNewTask
	}
	return NewTaskResult{ThreadID: threadID, TurnID: turnID}, nil
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
	for index := range tasks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if tasks[index].Source != taskstate.SourceCatalog || set.catalog.load == nil {
			continue
		}
		resolved, resolveErr := set.catalog.ResolveDesktopOwner(ctx, tasks[index].ID)
		if resolveErr != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			continue
		}
		tasks[index] = resolved
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
