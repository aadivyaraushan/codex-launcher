package taskadapter

import (
	"context"
	"errors"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

type Set struct {
	router  *taskstate.AdapterRouter
	desktop *desktopipc.Client
	app     *appserver.Client
	catalog *Catalog
}

var ErrDesktopUnavailable = errors.New("desktop adapter is unavailable")

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
	return Set{router: &router, desktop: desktop, app: appServer, catalog: catalog}, nil
}

func NewAppServerOnly(appServer *appserver.Client) (Set, error) {
	if appServer == nil {
		return Set{}, errors.New("app-server adapter is required")
	}
	catalog, err := NewAppServerCatalog(appServer)
	if err != nil {
		return Set{}, err
	}
	return Set{app: appServer, catalog: catalog}, nil
}

func (set Set) ListRecentCandidates(ctx context.Context, limit int) ([]taskstate.Task, error) {
	return set.ListRecent(ctx, limit)
}

func (set Set) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	if set.catalog == nil {
		return nil, errors.New("task catalog is unavailable")
	}
	return set.catalog.ListRecent(ctx, limit)
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
