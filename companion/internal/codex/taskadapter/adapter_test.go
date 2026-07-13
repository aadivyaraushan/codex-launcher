package taskadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

func TestRealAdapterSetRoutesByOwnedSourceWithoutFallback(t *testing.T) {
	set, err := New(&desktopipc.Client{}, &appserver.Client{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := set.For(taskstate.Task{ID: "desktop-1", Source: taskstate.SourceDesktop})
	if err != nil || adapter.TaskSource() != taskstate.SourceDesktop {
		t.Fatalf("desktop route = %#v, %v", adapter, err)
	}
	adapter, err = set.For(taskstate.Task{ID: "cli-1", Source: taskstate.SourceAppServer})
	if err != nil || adapter.TaskSource() != taskstate.SourceAppServer {
		t.Fatalf("app-server route = %#v, %v", adapter, err)
	}
	if _, err := set.For(taskstate.Task{ID: "unknown"}); !errors.Is(err, taskstate.ErrUnknownTaskSource) {
		t.Fatalf("unknown route = %v", err)
	}
}

func TestRealAdapterSetRequiresBothRuntimeBoundaries(t *testing.T) {
	if _, err := New(nil, &appserver.Client{}); err == nil {
		t.Fatal("missing desktop client was accepted")
	}
	if _, err := New(&desktopipc.Client{}, nil); err == nil {
		t.Fatal("missing app-server client was accepted")
	}
}

func TestCatalogListsBoundedCandidatesWithoutClaimingRuntimeOwnership(t *testing.T) {
	requestedLimit := 0
	catalog := newCatalog(
		func(_ context.Context, limit int) (json.RawMessage, error) {
			requestedLimit = limit
			return json.RawMessage(`{"data":[{"id":"desktop-1","name":"Existing task","preview":"","cwd":"/work/project","updatedAt":42,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]}]}`), nil
		}, nil, nil,
	)
	tasks, err := catalog.ListRecent(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if requestedLimit != 3 || len(tasks) != 1 || tasks[0].ID != "desktop-1" || tasks[0].Source != taskstate.SourceCatalog {
		t.Fatalf("catalog result = %#v, requested limit = %d", tasks, requestedLimit)
	}
	if _, err := catalog.ListRecent(context.Background(), 0); !errors.Is(err, ErrInvalidCatalogLimit) {
		t.Fatalf("zero limit error = %v", err)
	}
}

func TestCatalogKeepsRuntimeLocalAppServerTasksOwned(t *testing.T) {
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) {
			return json.RawMessage(`{"data":[{"id":"cli-1","name":"CLI task","preview":"","cwd":"/work/project","updatedAt":43,"status":{"type":"active","activeFlags":[]},"turns":[{"status":"inProgress"}]}]}`), nil
		}, nil, nil,
	)
	tasks, err := catalog.ListRecent(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Source != taskstate.SourceAppServer {
		t.Fatalf("app-server task = %#v", tasks)
	}
}

func TestCatalogPromotesTaskOnlyAfterDesktopOwnerProof(t *testing.T) {
	loadCalls := 0
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) {
			return catalogResult(), nil
		},
		func(_ context.Context, taskID string) error {
			loadCalls++
			if taskID != "desktop-1" {
				t.Fatalf("task ID = %q", taskID)
			}
			return nil
		},
		func(taskID string) (json.RawMessage, error) {
			return json.RawMessage(`{"id":"desktop-1","cwd":"/work/project","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[{"status":"completed"}]}`), nil
		},
	)
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	task, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1")
	if err != nil {
		t.Fatal(err)
	}
	if loadCalls != 1 || task.Source != taskstate.SourceDesktop || task.ID != "desktop-1" {
		t.Fatalf("resolved task = %#v, load calls = %d", task, loadCalls)
	}
}

func TestCatalogOwnerFailureDoesNotFallBackToAppServer(t *testing.T) {
	ownerUnavailable := errors.New("owner unavailable")
	stateRead := false
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) {
			return catalogResult(), nil
		},
		func(context.Context, string) error { return ownerUnavailable },
		func(string) (json.RawMessage, error) {
			stateRead = true
			return nil, nil
		},
	)
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	_, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1")
	if !errors.Is(err, ownerUnavailable) || stateRead {
		t.Fatalf("resolution error = %v, state read = %v", err, stateRead)
	}
}

func TestCatalogExplicitAppServerResumeChangesSource(t *testing.T) {
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil }, nil, nil,
	)
	catalog.resume = func(_ context.Context, taskID string) (json.RawMessage, error) {
		if taskID != "desktop-1" {
			t.Fatalf("task ID = %q", taskID)
		}
		return json.RawMessage(`{"id":"desktop-1","name":"Existing task","preview":"","cwd":"/work/project","updatedAt":44,"status":{"type":"idle","activeFlags":[]},"turns":[{"status":"completed"}]}`), nil
	}
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	task, err := catalog.ResumeWithAppServer(context.Background(), "desktop-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Source != taskstate.SourceAppServer || task.ID != "desktop-1" {
		t.Fatalf("resumed task = %#v", task)
	}
}

func TestCatalogNeverResumesAfterDesktopOwnerProof(t *testing.T) {
	resumeCalled := false
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		func(context.Context, string) error { return nil },
		func(string) (json.RawMessage, error) {
			return json.RawMessage(`{"id":"desktop-1","cwd":"/work/project","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[]}`), nil
		},
	)
	catalog.resume = func(context.Context, string) (json.RawMessage, error) {
		resumeCalled = true
		return nil, nil
	}
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResumeWithAppServer(context.Background(), "desktop-1"); !errors.Is(err, ErrDesktopOwned) || resumeCalled {
		t.Fatalf("resume error = %v, app-server called = %v", err, resumeCalled)
	}
}

func TestCatalogResumeFailsClosedOnDesktopVerificationError(t *testing.T) {
	resumeCalled := false
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		func(context.Context, string) error { return desktopipc.ErrIncompatibleBuild },
		nil,
	)
	catalog.resume = func(context.Context, string) (json.RawMessage, error) {
		resumeCalled = true
		return nil, nil
	}
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResumeWithAppServer(context.Background(), "desktop-1"); !errors.Is(err, desktopipc.ErrIncompatibleBuild) || resumeCalled {
		t.Fatalf("resume error = %v, app-server called = %v", err, resumeCalled)
	}
}

func TestCatalogResumeProceedsOnlyWhenDesktopOwnerIsUnavailable(t *testing.T) {
	resumeCalled := false
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		func(context.Context, string) error { return desktopipc.ErrOwnerUnavailable },
		nil,
	)
	catalog.resume = func(context.Context, string) (json.RawMessage, error) {
		resumeCalled = true
		return json.RawMessage(`{"id":"desktop-1","name":"Existing task","preview":"","cwd":"/work/project","updatedAt":44,"status":{"type":"idle","activeFlags":[]},"turns":[]}`), nil
	}
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResumeWithAppServer(context.Background(), "desktop-1"); err != nil || !resumeCalled {
		t.Fatalf("resume error = %v, app-server called = %v", err, resumeCalled)
	}
}

func TestCatalogMetadataControlsAndForkHaveExplicitOwnership(t *testing.T) {
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil }, nil, nil,
	)
	var calls []string
	catalog.rename = func(_ context.Context, taskID, name string) error {
		calls = append(calls, "rename:"+taskID+":"+name)
		return nil
	}
	catalog.archive = func(_ context.Context, taskID string) error {
		calls = append(calls, "archive:"+taskID)
		return nil
	}
	catalog.fork = func(_ context.Context, taskID string) (json.RawMessage, error) {
		calls = append(calls, "fork:"+taskID)
		return json.RawMessage(`{"id":"fork-1","name":"Fork","preview":"","cwd":"/work/project","updatedAt":45,"status":{"type":"idle","activeFlags":[]},"turns":[]}`), nil
	}
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Rename(context.Background(), "desktop-1", "Renamed"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Archive(context.Background(), "desktop-1"); err != nil {
		t.Fatal(err)
	}
	forked, err := catalog.ForkToAppServer(context.Background(), "desktop-1")
	if err != nil {
		t.Fatal(err)
	}
	if forked.Source != taskstate.SourceAppServer || forked.ID != "fork-1" {
		t.Fatalf("forked task = %#v", forked)
	}
	want := []string{"rename:desktop-1:Renamed", "archive:desktop-1", "fork:desktop-1"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v", calls)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("calls = %#v", calls)
		}
	}
}

func catalogResult() json.RawMessage {
	return json.RawMessage(`{"data":[{"id":"desktop-1","name":"Existing task","preview":"","cwd":"/work/project","updatedAt":42,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]}]}`)
}

func TestSourceCapabilitiesExposeSharedMetadataWithoutUnverifiedLiveControl(t *testing.T) {
	candidate := CapabilitiesFor(taskstate.SourceCatalog)
	if candidate.LiveControl || !candidate.Rename || !candidate.Archive || !candidate.Fork {
		t.Fatalf("catalog candidate capabilities = %#v", candidate)
	}
	desktop := CapabilitiesFor(taskstate.SourceDesktop)
	if !desktop.LiveControl || !desktop.Rename || !desktop.Archive || !desktop.Fork {
		t.Fatalf("desktop capabilities = %#v", desktop)
	}
	appServer := CapabilitiesFor(taskstate.SourceAppServer)
	if !appServer.LiveControl || !appServer.Rename || !appServer.Archive || !appServer.Fork {
		t.Fatalf("app-server capabilities = %#v", appServer)
	}
}

func TestCatalogLogsOwnershipBranchesWithoutWorkContent(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	catalog := newCatalogWithLogger(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		func(context.Context, string) error { return nil },
		func(string) (json.RawMessage, error) {
			return json.RawMessage(`{"id":"desktop-1","cwd":"/work/project","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[]}`), nil
		},
		logger,
	)
	if _, err := catalog.ListRecent(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1"); err != nil {
		t.Fatal(err)
	}
	logs := output.String()
	for _, want := range []string{"[codex-adapter] catalog listed", "output_count=1", "branch_reason=desktop_owner_verified"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q: %s", want, logs)
		}
	}
	for _, secret := range []string{"Existing task", "/work/project"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs contain work content %q: %s", secret, logs)
		}
	}
}

func TestAppServerOnlySetSupportsLinuxWithoutDesktop(t *testing.T) {
	set, err := NewAppServerOnly(&appserver.Client{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := set.For(taskstate.Task{ID: "cli-1", Source: taskstate.SourceAppServer})
	if err != nil || adapter.TaskSource() != taskstate.SourceAppServer {
		t.Fatalf("app-server-only route = %#v, %v", adapter, err)
	}
	if _, err := set.For(taskstate.Task{ID: "desktop-1", Source: taskstate.SourceDesktop}); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("desktop route error = %v", err)
	}
}

func TestSetProvidesTheMobileRecentTaskSourceContract(t *testing.T) {
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"task-1","preview":"Build launcher","cwd":"/work/launcher","updatedAt":42,"status":{"type":"active","activeFlags":[]},"turns":[{"status":"inProgress"}]}]}`), nil
	}, nil, nil)
	set := Set{catalog: catalog}

	tasks, err := set.ListRecent(context.Background(), 1)
	if err != nil || len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("recent tasks = %#v, %v", tasks, err)
	}
}
