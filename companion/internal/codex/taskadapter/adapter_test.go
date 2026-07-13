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

func TestCatalogAuthorizesMobileEventsOnlyAfterMappedOwnerProof(t *testing.T) {
	var steps []string
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		func(context.Context, string) error {
			steps = append(steps, "load")
			return nil
		},
		func(string) (json.RawMessage, error) {
			steps = append(steps, "map")
			return json.RawMessage(`{"id":"desktop-1","cwd":"/work/project","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[]}`), nil
		},
	)
	catalog.authorize = func(taskID string) error {
		steps = append(steps, "authorize:"+taskID)
		return nil
	}
	if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(steps, ",") != "load,map,authorize:desktop-1" {
		t.Fatalf("owner proof steps = %#v", steps)
	}
}

func TestCatalogRevokesLowLevelAuthorizationWhenMappedOwnerProofFails(t *testing.T) {
	authorized := false
	catalog := newCatalog(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		func(context.Context, string) error {
			authorized = true
			return nil
		},
		func(string) (json.RawMessage, error) {
			return nil, errors.New("retained state unavailable")
		},
	)
	catalog.revoke = func(string) { authorized = false }
	if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1"); err == nil {
		t.Fatal("invalid retained state was accepted")
	}
	if authorized {
		t.Fatal("failed mapped owner proof retained mobile authorization")
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

func TestCatalogOwnerFailuresLogOnlySafeMetadata(t *testing.T) {
	tests := []struct {
		name  string
		load  func(context.Context, string) error
		state func(string) (json.RawMessage, error)
	}{
		{
			name: "load failure",
			load: func(context.Context, string) error { return errors.New("private remote owner error") },
			state: func(string) (json.RawMessage, error) {
				return nil, errors.New("state should not be read")
			},
		},
		{
			name: "state failure",
			load: func(context.Context, string) error { return nil },
			state: func(string) (json.RawMessage, error) {
				return nil, errors.New("private retained state error")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			catalog := newCatalogWithLogger(
				func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
				test.load,
				test.state,
				logger,
			)
			if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if _, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1"); err == nil {
				t.Fatal("owner failure was accepted")
			}
			logs := output.String()
			for _, secret := range []string{"private remote owner error", "private retained state error"} {
				if strings.Contains(logs, secret) {
					t.Fatalf("logs exposed %q: %s", secret, logs)
				}
			}
		})
	}
}

func TestCatalogExternalFailuresNeverLogRawErrorText(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	secretError := errors.New("private remote service error")
	catalog := newCatalogWithLogger(
		func(context.Context, int) (json.RawMessage, error) { return catalogResult(), nil },
		nil,
		nil,
		logger,
	)
	catalog.resume = func(context.Context, string) (json.RawMessage, error) { return nil, secretError }
	catalog.rename = func(context.Context, string, string) error { return secretError }
	catalog.archive = func(context.Context, string) error { return secretError }
	catalog.fork = func(context.Context, string) (json.RawMessage, error) { return nil, secretError }
	if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	_, _ = catalog.ResumeWithAppServer(context.Background(), "desktop-1")
	_ = catalog.Rename(context.Background(), "desktop-1", "Renamed")
	_ = catalog.Archive(context.Background(), "desktop-1")
	_, _ = catalog.ForkToAppServer(context.Background(), "desktop-1")
	if strings.Contains(output.String(), secretError.Error()) {
		t.Fatalf("logs exposed remote error text: %s", output.String())
	}

	output.Reset()
	listFailure := newCatalogWithLogger(
		func(context.Context, int) (json.RawMessage, error) { return nil, secretError },
		nil,
		nil,
		logger,
	)
	_, _ = listFailure.ListRecent(context.Background(), 1)
	if strings.Contains(output.String(), secretError.Error()) {
		t.Fatalf("catalog list log exposed remote error text: %s", output.String())
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

func TestSetListRecentFollowsFourHomeTasksAndOrdersAttentionBeforeRecent(t *testing.T) {
	loaded := make([]string, 0)
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[
			{"id":"task-1","name":"One","preview":"","cwd":"/work/one","updatedAt":46,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},
			{"id":"task-2","name":"Two","preview":"","cwd":"/work/two","updatedAt":45,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},
			{"id":"task-3","name":"Three","preview":"","cwd":"/work/three","updatedAt":44,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},
			{"id":"task-4","name":"Four","preview":"","cwd":"/work/four","updatedAt":43,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},
			{"id":"task-5","name":"Five","preview":"","cwd":"/work/five","updatedAt":42,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},
			{"id":"task-6","name":"Six","preview":"","cwd":"/work/six","updatedAt":41,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]}
		]}`), nil
	}, func(_ context.Context, taskID string) error {
		loaded = append(loaded, taskID)
		return nil
	}, func(taskID string) (json.RawMessage, error) {
		status := `{"type":"idle","activeFlags":[]}`
		if taskID == "task-2" {
			status = `{"type":"active","activeFlags":[]}`
		}
		if taskID == "task-3" {
			status = `{"type":"active","activeFlags":["waitingOnApproval"]}`
		}
		return json.RawMessage(`{"id":"` + taskID + `","cwd":"/work","threadRuntimeStatus":` + status + `,"requests":[],"turns":[]}`), nil
	})
	set := Set{catalog: catalog}

	tasks, err := set.ListRecent(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded, ",") != "task-1,task-2,task-3,task-4" {
		t.Fatalf("loaded tasks = %#v", loaded)
	}
	if len(tasks) != 4 {
		t.Fatalf("Home task count = %d, tasks = %#v", len(tasks), tasks)
	}
	if got := []string{tasks[0].ID, tasks[1].ID, tasks[2].ID, tasks[3].ID}; strings.Join(got, ",") != "task-2,task-3,task-1,task-4" {
		t.Fatalf("Home task order = %#v", got)
	}
	for _, task := range tasks {
		if task.Source != taskstate.SourceDesktop {
			t.Fatalf("task source = %#v", task)
		}
	}
}

func TestSetListRecentSkipsAppServerAndKeepsFailedDesktopCandidateReadable(t *testing.T) {
	loaded := make([]string, 0)
	stateReads := 0
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[
			{"id":"app-1","name":"App","preview":"","cwd":"/work/app","updatedAt":46,"status":{"type":"active","activeFlags":[]},"turns":[{"status":"inProgress"}]},
			{"id":"task-1","name":"One","preview":"","cwd":"/work/one","updatedAt":45,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},
			{"id":"task-2","name":"Two","preview":"","cwd":"/work/two","updatedAt":44,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]}
		]}`), nil
	}, func(_ context.Context, taskID string) error {
		loaded = append(loaded, taskID)
		if taskID == "task-1" {
			return desktopipc.ErrOwnerUnavailable
		}
		return nil
	}, func(taskID string) (json.RawMessage, error) {
		stateReads++
		return json.RawMessage(`{"id":"` + taskID + `","cwd":"/work","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[]}`), nil
	})
	set := Set{catalog: catalog}

	tasks, err := set.ListRecent(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded, ",") != "task-1,task-2" || stateReads != 1 {
		t.Fatalf("loaded = %#v, state reads = %d", loaded, stateReads)
	}
	byID := make(map[string]taskstate.Source, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task.Source
	}
	if byID["app-1"] != taskstate.SourceAppServer || byID["task-1"] != taskstate.SourceCatalog || byID["task-2"] != taskstate.SourceDesktop {
		t.Fatalf("Home task sources = %#v", byID)
	}
}

func TestListRecentCandidatesDoesNotStartDesktopFollowing(t *testing.T) {
	loadCalls := 0
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return catalogResult(), nil
	}, func(context.Context, string) error {
		loadCalls++
		return nil
	}, func(string) (json.RawMessage, error) {
		return nil, errors.New("state should not be read")
	})
	set := Set{catalog: catalog}

	tasks, err := set.ListRecentCandidates(context.Background(), 1)
	if err != nil || len(tasks) != 1 || tasks[0].Source != taskstate.SourceCatalog || loadCalls != 0 {
		t.Fatalf("candidates = %#v, load calls = %d, error = %v", tasks, loadCalls, err)
	}
}

func TestSetListRecentStopsDesktopFollowingWhenSyncIsCancelled(t *testing.T) {
	loadCalls := 0
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return catalogResult(), nil
	}, func(context.Context, string) error {
		loadCalls++
		return nil
	}, func(string) (json.RawMessage, error) {
		return nil, errors.New("state should not be read")
	})
	set := Set{catalog: catalog}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := set.ListRecent(ctx, 1); !errors.Is(err, context.Canceled) || loadCalls != 0 {
		t.Fatalf("cancelled follow error = %v, load calls = %d", err, loadCalls)
	}
}
