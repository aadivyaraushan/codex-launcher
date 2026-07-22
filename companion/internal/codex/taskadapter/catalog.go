package taskadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
)

const MaxRecentCatalogTasks = 20

var (
	ErrInvalidCatalogLimit = errors.New("recent task limit is invalid")
	ErrUnknownCatalogTask  = errors.New("task is not in the current Codex catalog")
	ErrDesktopOwned        = errors.New("task is owned by ChatGPT Desktop")
)

type Catalog struct {
	list              func(context.Context, int) (json.RawMessage, error)
	load              func(context.Context, string) error
	state             func(string) (json.RawMessage, error)
	resume            func(context.Context, string) (json.RawMessage, error)
	rename            func(context.Context, string, string) error
	archive           func(context.Context, string) error
	fork              func(context.Context, string) (json.RawMessage, error)
	readAppTranscript func(context.Context, string) (json.RawMessage, error)
	authorize         func(string) error
	revoke            func(string)
	logger            *slog.Logger

	mu         sync.RWMutex
	candidates map[string]taskstate.Task
}

func NewCatalog(desktop *desktopipc.Client, appServer *appserver.Client) (*Catalog, error) {
	if desktop == nil || appServer == nil {
		return nil, errors.New("desktop follower and Codex catalog clients are required")
	}
	catalog := newCatalogWithLogger(
		func(ctx context.Context, limit int) (json.RawMessage, error) {
			return appServer.ListThreads(ctx, appserver.ListOptions{Limit: limit})
		},
		func(ctx context.Context, taskID string) error {
			_, err := desktop.LoadCompleteHistory(ctx, taskID)
			return err
		},
		func(taskID string) (json.RawMessage, error) {
			materialized := desktop.Stream(taskID).State().Materialized
			if len(materialized) == 0 {
				return nil, errors.New("desktop owner returned no task state")
			}
			return append(json.RawMessage(nil), materialized...), nil
		},
		slog.Default(),
	)
	catalog.authorize = desktop.AuthorizeMobileEvents
	catalog.revoke = desktop.RevokeMobileEvents
	configureAppServerActions(catalog, appServer)
	return catalog, nil
}

func NewAppServerCatalog(appServer *appserver.Client) (*Catalog, error) {
	if appServer == nil {
		return nil, errors.New("Codex app-server catalog client is required")
	}
	catalog := newCatalogWithLogger(func(ctx context.Context, limit int) (json.RawMessage, error) {
		return appServer.ListThreads(ctx, appserver.ListOptions{Limit: limit})
	}, nil, nil, slog.Default())
	configureAppServerActions(catalog, appServer)
	return catalog, nil
}

func configureAppServerActions(catalog *Catalog, appServer *appserver.Client) {
	catalog.readAppTranscript = func(ctx context.Context, taskID string) (json.RawMessage, error) {
		return appServer.ReadThread(ctx, taskID, true)
	}
	catalog.resume = func(ctx context.Context, taskID string) (json.RawMessage, error) {
		result, err := appServer.ResumeThread(ctx, taskID, appserver.ThreadOptions{})
		if err != nil {
			return nil, err
		}
		return extractThread(result)
	}
	catalog.rename = func(ctx context.Context, taskID, name string) error {
		_, err := appServer.SetThreadName(ctx, taskID, name)
		return err
	}
	catalog.archive = func(ctx context.Context, taskID string) error {
		_, err := appServer.ArchiveThread(ctx, taskID)
		return err
	}
	catalog.fork = func(ctx context.Context, taskID string) (json.RawMessage, error) {
		result, err := appServer.ForkThread(ctx, taskID, "")
		if err != nil {
			return nil, err
		}
		return extractThread(result)
	}
}

func (catalog *Catalog) ReadTranscript(ctx context.Context, taskID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	if catalog == nil {
		return tasktranscript.Page{}, errors.New("task catalog is unavailable")
	}
	if options.TaskID != "" && options.TaskID != taskID {
		return tasktranscript.Page{}, tasktranscript.ErrTaskMismatch
	}
	options.TaskID = taskID
	candidate, err := catalog.candidate(taskID)
	if err != nil {
		if !errors.Is(err, ErrUnknownCatalogTask) || catalog.readAppTranscript == nil {
			return tasktranscript.Page{}, err
		}
		candidate = taskstate.Task{ID: taskID, Source: taskstate.SourceAppServer}
		catalog.logger.Info(
			"[codex-adapter] transcript task missing from bounded catalog; reading directly",
			"task_id", taskID,
			"branch_reason", "shared_catalog_pending",
		)
	}
	catalog.logger.Debug("[codex-adapter] transcript requested", "task_id", taskID, "source", candidate.Source, "input_limit", options.Limit, "has_cursor", options.BeforeEntryID != "")
	var page tasktranscript.Page
	switch candidate.Source {
	case taskstate.SourceAppServer, taskstate.SourceCatalog:
		if catalog.readAppTranscript == nil {
			if candidate.Source == taskstate.SourceCatalog {
				return tasktranscript.Page{}, taskstate.ErrUnresolvedTaskSource
			}
			return tasktranscript.Page{}, errors.New("app-server transcript reader is unavailable")
		}
		raw, readErr := catalog.readAppTranscript(ctx, taskID)
		if readErr != nil {
			catalog.logger.Error("[codex-adapter] app-server transcript read failed", "task_id", taskID, "error_class", fmt.Sprintf("%T", readErr))
			return tasktranscript.Page{}, fmt.Errorf("read app-server task transcript: %w", readErr)
		}
		page, err = tasktranscript.MapAppServerPage(raw, options)
	case taskstate.SourceDesktop:
		if catalog.state == nil {
			return tasktranscript.Page{}, errors.New("Desktop transcript reader is unavailable")
		}
		raw, readErr := catalog.state(taskID)
		if readErr != nil {
			catalog.logger.Error("[codex-adapter] Desktop transcript read failed", "task_id", taskID, "error_class", fmt.Sprintf("%T", readErr))
			return tasktranscript.Page{}, fmt.Errorf("read Desktop task transcript: %w", readErr)
		}
		page, err = tasktranscript.MapDesktopPage(raw, options)
	default:
		return tasktranscript.Page{}, taskstate.ErrUnknownTaskSource
	}
	if err != nil {
		catalog.logger.Error("[codex-adapter] transcript mapping failed", "task_id", taskID, "source", candidate.Source, "error_class", fmt.Sprintf("%T", err))
		return tasktranscript.Page{}, fmt.Errorf("map Codex task transcript: %w", err)
	}
	encoded, _ := json.Marshal(page)
	catalog.logger.Info("[codex-adapter] transcript ready", "task_id", taskID, "source", candidate.Source, "output_count", len(page.Entries), "output_bytes", len(encoded), "truncated", page.Truncated)
	return page, nil
}

func newCatalog(
	list func(context.Context, int) (json.RawMessage, error),
	load func(context.Context, string) error,
	state func(string) (json.RawMessage, error),
) *Catalog {
	return newCatalogWithLogger(list, load, state, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newCatalogWithLogger(
	list func(context.Context, int) (json.RawMessage, error),
	load func(context.Context, string) error,
	state func(string) (json.RawMessage, error),
	logger *slog.Logger,
) *Catalog {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Catalog{list: list, load: load, state: state, logger: logger, candidates: make(map[string]taskstate.Task)}
}

func (catalog *Catalog) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	if catalog != nil && catalog.logger != nil {
		catalog.logger.Debug("[codex-adapter] catalog list requested", "input_limit", limit)
	}
	if catalog == nil || catalog.list == nil || limit < 1 || limit > MaxRecentCatalogTasks {
		return nil, ErrInvalidCatalogLimit
	}
	raw, err := catalog.list(ctx, limit)
	if err != nil {
		catalog.logger.Error("[codex-adapter] catalog list failed", "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("list shared Codex catalog: %w", err)
	}
	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Data) > limit {
		catalog.logger.Error("[codex-adapter] catalog list rejected", "branch_reason", "invalid_response_shape")
		return nil, errors.New("invalid shared Codex catalog response")
	}
	tasks := make([]taskstate.Task, 0, len(result.Data))
	nextCandidates := make(map[string]taskstate.Task, len(result.Data))
	for _, thread := range result.Data {
		task, mapErr := taskstate.MapAppServerThread(thread)
		if mapErr != nil {
			catalog.logger.Error("[codex-adapter] catalog task rejected", "branch_reason", "invalid_task_shape", "error_class", fmt.Sprintf("%T", mapErr))
			return nil, fmt.Errorf("map shared Codex catalog task: %w", mapErr)
		}
		var runtime struct {
			Status struct {
				Type string `json:"type"`
			} `json:"status"`
		}
		if json.Unmarshal(thread, &runtime) != nil {
			catalog.logger.Error("[codex-adapter] catalog ownership rejected", "task_id", task.ID, "branch_reason", "invalid_runtime_shape")
			return nil, errors.New("invalid shared Codex catalog ownership state")
		}
		if runtime.Status.Type == "active" || runtime.Status.Type == "idle" {
			task.Source = taskstate.SourceAppServer
		} else {
			task.Source = taskstate.SourceCatalog
		}
		catalog.logger.Debug("[codex-adapter] catalog task classified", "task_id", task.ID, "runtime_status", runtime.Status.Type, "source", task.Source)
		tasks = append(tasks, task)
		nextCandidates[task.ID] = task
	}
	catalog.mu.Lock()
	catalog.candidates = nextCandidates
	catalog.mu.Unlock()
	catalog.logger.Info("[codex-adapter] catalog listed", "output_count", len(tasks))
	return tasks, nil
}

func (catalog *Catalog) ResolveDesktopOwner(ctx context.Context, taskID string) (taskstate.Task, error) {
	if catalog == nil || catalog.load == nil || catalog.state == nil {
		return taskstate.Task{}, errors.New("desktop owner resolver is unavailable")
	}
	catalog.logger.Debug("[codex-adapter] Desktop owner check requested", "task_id", taskID)
	candidate, err := catalog.candidate(taskID)
	if err != nil {
		return taskstate.Task{}, err
	}
	if candidate.Source != taskstate.SourceCatalog {
		return taskstate.Task{}, taskstate.ErrAdapterSourceMismatch
	}
	if err := catalog.load(ctx, taskID); err != nil {
		catalog.revokeMobileEvents(taskID)
		catalog.logger.Info("[codex-adapter] Desktop owner check rejected", "task_id", taskID, "branch_reason", "owner_not_verified", "error_class", fmt.Sprintf("%T", err))
		return taskstate.Task{}, fmt.Errorf("verify desktop task owner: %w", err)
	}
	raw, err := catalog.state(taskID)
	if err != nil {
		catalog.revokeMobileEvents(taskID)
		catalog.logger.Error("[codex-adapter] verified Desktop state unavailable", "task_id", taskID, "error_class", fmt.Sprintf("%T", err))
		return taskstate.Task{}, fmt.Errorf("read verified desktop task: %w", err)
	}
	task, err := taskstate.MapDesktopConversationState(raw)
	if err != nil || task.ID != taskID {
		catalog.revokeMobileEvents(taskID)
		catalog.logger.Error("[codex-adapter] verified Desktop state rejected", "task_id", taskID, "branch_reason", "state_mismatch")
		return taskstate.Task{}, errors.New("verified desktop state does not match catalog task")
	}
	if catalog.authorize != nil {
		if err := catalog.authorize(taskID); err != nil {
			catalog.revokeMobileEvents(taskID)
			catalog.logger.Error("[codex-adapter] verified Desktop authorization rejected", "task_id", taskID, "branch_reason", "authorization_failed", "error_class", fmt.Sprintf("%T", err))
			return taskstate.Task{}, fmt.Errorf("authorize verified desktop task: %w", err)
		}
	}
	task.Title = candidate.Title
	task.UpdatedAtUnix = candidate.UpdatedAtUnix
	catalog.mu.Lock()
	catalog.candidates[taskID] = task
	catalog.mu.Unlock()
	catalog.logger.Info("[codex-adapter] Desktop owner resolved", "task_id", taskID, "branch_reason", "desktop_owner_verified")
	return task, nil
}

func (catalog *Catalog) revokeMobileEvents(taskID string) {
	if catalog.revoke != nil {
		catalog.revoke(taskID)
	}
}

func (catalog *Catalog) ResumeWithAppServer(ctx context.Context, taskID string) (taskstate.Task, error) {
	candidate, err := catalog.candidate(taskID)
	if err != nil {
		return taskstate.Task{}, err
	}
	if candidate.Source == taskstate.SourceDesktop {
		catalog.logger.Info("[codex-adapter] app-server resume rejected", "task_id", taskID, "branch_reason", "desktop_owner_recorded")
		return taskstate.Task{}, ErrDesktopOwned
	}
	if candidate.Source != taskstate.SourceCatalog || catalog.resume == nil {
		return taskstate.Task{}, taskstate.ErrAdapterSourceMismatch
	}
	if catalog.load != nil {
		ownerErr := catalog.load(ctx, taskID)
		switch {
		case ownerErr == nil:
			candidate.Source = taskstate.SourceDesktop
			catalog.mu.Lock()
			catalog.candidates[taskID] = candidate
			catalog.mu.Unlock()
			catalog.logger.Info("[codex-adapter] app-server resume rejected", "task_id", taskID, "branch_reason", "desktop_owner_verified")
			return taskstate.Task{}, ErrDesktopOwned
		case errors.Is(ownerErr, desktopipc.ErrOwnerUnavailable):
			// An exact owner-unavailable result is the only safe Desktop check
			// that permits the user's explicit app-server resume choice.
		default:
			catalog.logger.Error("[codex-adapter] app-server resume blocked", "task_id", taskID, "branch_reason", "desktop_check_failed", "error_class", fmt.Sprintf("%T", ownerErr))
			return taskstate.Task{}, fmt.Errorf("verify no Desktop owner before app-server resume: %w", ownerErr)
		}
	}
	raw, err := catalog.resume(ctx, taskID)
	if err != nil {
		catalog.logger.Error("[codex-adapter] explicit app-server resume failed", "task_id", taskID, "error_class", fmt.Sprintf("%T", err))
		return taskstate.Task{}, fmt.Errorf("explicitly resume task with app-server: %w", err)
	}
	thread, err := extractThread(raw)
	if err != nil {
		return taskstate.Task{}, err
	}
	task, err := taskstate.MapAppServerThread(thread)
	if err != nil || task.ID != taskID {
		catalog.logger.Error("[codex-adapter] explicit app-server resume rejected", "task_id", taskID, "branch_reason", "result_mismatch")
		return taskstate.Task{}, errors.New("resumed app-server task does not match catalog task")
	}
	catalog.logger.Info("[codex-adapter] explicit app-server resume confirmed", "task_id", taskID, "branch_reason", "user_selected_app_server")
	return task, nil
}

func (catalog *Catalog) Rename(ctx context.Context, taskID, name string) error {
	if _, err := catalog.candidate(taskID); err != nil {
		return err
	}
	if catalog.rename == nil {
		return errors.New("shared task rename is unavailable")
	}
	if err := catalog.rename(ctx, taskID, name); err != nil {
		catalog.logger.Error("[codex-adapter] shared task rename failed", "task_id", taskID, "error_class", fmt.Sprintf("%T", err))
		return fmt.Errorf("rename shared Codex task: %w", err)
	}
	catalog.logger.Info("[codex-adapter] shared task renamed", "task_id", taskID)
	return nil
}

func (catalog *Catalog) Archive(ctx context.Context, taskID string) error {
	if _, err := catalog.candidate(taskID); err != nil {
		return err
	}
	if catalog.archive == nil {
		return errors.New("shared task archive is unavailable")
	}
	if err := catalog.archive(ctx, taskID); err != nil {
		catalog.logger.Error("[codex-adapter] shared task archive failed", "task_id", taskID, "error_class", fmt.Sprintf("%T", err))
		return fmt.Errorf("archive shared Codex task: %w", err)
	}
	catalog.logger.Info("[codex-adapter] shared task archived", "task_id", taskID)
	return nil
}

func (catalog *Catalog) ForkToAppServer(ctx context.Context, taskID string) (taskstate.Task, error) {
	if _, err := catalog.candidate(taskID); err != nil {
		return taskstate.Task{}, err
	}
	if catalog.fork == nil {
		return taskstate.Task{}, errors.New("shared task fork is unavailable")
	}
	raw, err := catalog.fork(ctx, taskID)
	if err != nil {
		catalog.logger.Error("[codex-adapter] shared task fork failed", "task_id", taskID, "error_class", fmt.Sprintf("%T", err))
		return taskstate.Task{}, fmt.Errorf("fork shared Codex task: %w", err)
	}
	task, err := taskstate.MapAppServerThread(raw)
	if err != nil {
		catalog.logger.Error("[codex-adapter] shared task fork rejected", "task_id", taskID, "branch_reason", "invalid_result", "error_class", fmt.Sprintf("%T", err))
		return taskstate.Task{}, fmt.Errorf("map app-server fork: %w", err)
	}
	catalog.logger.Info("[codex-adapter] shared task forked", "task_id", taskID, "fork_task_id", task.ID, "source", task.Source)
	return task, nil
}

func (catalog *Catalog) candidate(taskID string) (taskstate.Task, error) {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	candidate, known := catalog.candidates[taskID]
	if !known {
		return taskstate.Task{}, ErrUnknownCatalogTask
	}
	return candidate, nil
}

func extractThread(raw json.RawMessage) (json.RawMessage, error) {
	var wrapped struct {
		Thread json.RawMessage `json:"thread"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Thread) != 0 {
		return wrapped.Thread, nil
	}
	var direct map[string]json.RawMessage
	if json.Unmarshal(raw, &direct) != nil || len(direct["id"]) == 0 {
		return nil, errors.New("app-server result contains no task")
	}
	return raw, nil
}

type Capabilities struct {
	LiveControl bool
	Rename      bool
	Archive     bool
	Fork        bool
}

func CapabilitiesFor(source taskstate.Source) Capabilities {
	switch source {
	case taskstate.SourceDesktop:
		return Capabilities{LiveControl: true, Rename: true, Archive: true, Fork: true}
	case taskstate.SourceAppServer:
		return Capabilities{LiveControl: true, Rename: true, Archive: true, Fork: true}
	case taskstate.SourceCatalog:
		return Capabilities{Rename: true, Archive: true, Fork: true}
	default:
		return Capabilities{}
	}
}
