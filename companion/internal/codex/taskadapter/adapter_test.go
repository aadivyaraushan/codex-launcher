package taskadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
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

func TestStartNewTaskCreatesConfiguredThreadThenStartsItsFirstTurn(t *testing.T) {
	var steps []string
	set := Set{
		startThread: func(_ context.Context, options appserver.ThreadOptions) (json.RawMessage, error) {
			steps = append(steps, "thread:"+options.CWD+":"+options.Model+":"+string(options.Sandbox)+":"+string(options.ApprovalPolicy))
			return json.RawMessage(`{"thread":{"id":"thread-created"}}`), nil
		},
		startTurn: func(_ context.Context, options appserver.TurnOptions) (json.RawMessage, error) {
			steps = append(steps, "turn:"+options.ThreadID+":"+options.Text+":"+options.Effort)
			return json.RawMessage(`{"turn":{"id":"turn-created"}}`), nil
		},
	}
	result, err := set.StartNewTask(context.Background(), NewTaskRequest{
		ProjectPath: "/work/project", Prompt: "Fix it", Model: "gpt-5.4", Effort: "high",
		Sandbox: appserver.SandboxWorkspaceWrite, ApprovalPolicy: json.RawMessage(`"on-request"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != "thread-created" || result.TurnID != "turn-created" {
		t.Fatalf("result = %#v", result)
	}
	want := []string{`thread:/work/project:gpt-5.4:workspace-write:"on-request"`, "turn:thread-created:Fix it:high"}
	if !slices.Equal(steps, want) {
		t.Fatalf("steps = %#v, want %#v", steps, want)
	}
}

func TestStartNewTaskPassesAttachmentsToTheFirstCodexTurn(t *testing.T) {
	var got appserver.TurnOptions
	set := Set{
		startThread: func(context.Context, appserver.ThreadOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"thread":{"id":"thread-created"}}`), nil
		},
		startTurn: func(_ context.Context, options appserver.TurnOptions) (json.RawMessage, error) {
			got = options
			return json.RawMessage(`{"turn":{"id":"turn-created"}}`), nil
		},
	}
	attachments := []AttachmentInput{{ID: "photo-1", Path: "/tmp/photo.png", MediaType: "image/png"}, {ID: "notes-2", Path: "/tmp/notes.pdf", MediaType: "application/pdf"}}
	if _, err := set.StartNewTask(context.Background(), NewTaskRequest{
		ProjectPath: "/work", Prompt: "Inspect", Model: "model", Effort: "high", Sandbox: appserver.SandboxReadOnly, Attachments: attachments,
	}); err != nil {
		t.Fatal(err)
	}
	want := []appserver.AttachmentInput{{ID: "photo-1", Path: "/tmp/photo.png", MediaType: "image/png"}, {ID: "notes-2", Path: "/tmp/notes.pdf", MediaType: "application/pdf"}}
	if !reflect.DeepEqual(got.Attachments, want) {
		t.Fatalf("turn attachments = %#v, want %#v", got.Attachments, want)
	}
}

func TestExistingTaskStartAndRedirectPassAttachmentsToTheOwnedAdapter(t *testing.T) {
	attachments := []AttachmentInput{{ID: "notes-1", Path: "/tmp/notes.txt", MediaType: "text/plain"}}
	states := []taskstate.Task{
		{ID: "thread-1", State: taskstate.IdleAfterReply, Source: taskstate.SourceDesktop},
		{ID: "thread-1", State: taskstate.Working, Source: taskstate.SourceDesktop, ActiveTurnID: "turn-1", CanRedirect: true},
	}
	var calls []string
	set := Set{
		currentTask: func(context.Context, string) (taskstate.Task, error) {
			task := states[0]
			states = states[1:]
			return task, nil
		},
		startExistingTurnWithAttachments: func(_ context.Context, task taskstate.Task, text string, got []AttachmentInput) (string, error) {
			if !reflect.DeepEqual(got, attachments) {
				t.Fatalf("start attachments = %#v", got)
			}
			calls = append(calls, "start:"+text)
			return "turn-started", nil
		},
		redirectExistingTurnWithAttachments: func(_ context.Context, task taskstate.Task, text string, got []AttachmentInput) (string, error) {
			if !reflect.DeepEqual(got, attachments) {
				t.Fatalf("redirect attachments = %#v", got)
			}
			calls = append(calls, "redirect:"+text)
			return task.ActiveTurnID, nil
		},
	}
	if _, err := set.StartExistingTurnWithAttachments(context.Background(), "thread-1", "inspect", attachments); err != nil {
		t.Fatal(err)
	}
	if _, err := set.RedirectExistingTurnWithAttachments(context.Background(), "thread-1", "more", attachments); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"start:inspect", "redirect:more"}) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestStartNewTaskFailsClosedOnMalformedResultsAndPartialCreation(t *testing.T) {
	turnCalls := 0
	set := Set{
		startThread: func(context.Context, appserver.ThreadOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"thread":{}}`), nil
		},
		startTurn: func(context.Context, appserver.TurnOptions) (json.RawMessage, error) {
			turnCalls++
			return nil, nil
		},
	}
	if _, err := set.StartNewTask(context.Background(), NewTaskRequest{ProjectPath: "/work", Prompt: "Fix it", Model: "model", Effort: "high", Sandbox: appserver.SandboxReadOnly}); !errors.Is(err, ErrPartialNewTask) || turnCalls != 0 {
		t.Fatalf("malformed thread result error = %v, want partial task; turn calls = %d", err, turnCalls)
	}

	set.startThread = func(context.Context, appserver.ThreadOptions) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"thread-created"}}`), nil
	}
	set.startTurn = func(context.Context, appserver.TurnOptions) (json.RawMessage, error) {
		return nil, errors.New("known turn failure")
	}
	if _, err := set.StartNewTask(context.Background(), NewTaskRequest{ProjectPath: "/work", Prompt: "Fix it", Model: "model", Effort: "high", Sandbox: appserver.SandboxReadOnly}); !errors.Is(err, ErrPartialNewTask) {
		t.Fatalf("partial task error = %v, want %v", err, ErrPartialNewTask)
	}
}

func TestExistingTaskControlsReloadStateAndRouteIdleBusyAndStop(t *testing.T) {
	states := []taskstate.Task{
		{ID: "thread-1", State: taskstate.IdleAfterReply, Source: taskstate.SourceAppServer},
		{ID: "thread-1", State: taskstate.Working, Source: taskstate.SourceAppServer, ActiveTurnID: "turn-1", CanRedirect: true},
		{ID: "thread-1", State: taskstate.Working, Source: taskstate.SourceAppServer, ActiveTurnID: "turn-1", CanRedirect: true},
	}
	var calls []string
	set := Set{
		currentTask: func(context.Context, string) (taskstate.Task, error) {
			task := states[0]
			states = states[1:]
			return task, nil
		},
		startExistingTurn: func(_ context.Context, task taskstate.Task, text string) (string, error) {
			calls = append(calls, "start:"+task.ID+":"+text)
			return "turn-started", nil
		},
		redirectExistingTurn: func(_ context.Context, task taskstate.Task, text string) (string, error) {
			calls = append(calls, "redirect:"+task.ActiveTurnID+":"+text)
			return task.ActiveTurnID, nil
		},
		interruptExistingTurn: func(_ context.Context, task taskstate.Task) (string, error) {
			calls = append(calls, "stop:"+task.ActiveTurnID)
			return task.ActiveTurnID, nil
		},
	}
	started, err := set.StartExistingTurn(context.Background(), "thread-1", "continue")
	if err != nil || started.TurnID != "turn-started" {
		t.Fatalf("idle start = %#v, %v", started, err)
	}
	redirected, err := set.RedirectExistingTurn(context.Background(), "thread-1", "change direction")
	if err != nil || redirected.TurnID != "turn-1" {
		t.Fatalf("busy redirect = %#v, %v", redirected, err)
	}
	if _, err := set.InterruptExistingTurn(context.Background(), "thread-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"start:thread-1:continue", "redirect:turn-1:change direction", "stop:turn-1"}
	if !slices.Equal(calls, want) {
		t.Fatalf("control calls = %#v, want %#v", calls, want)
	}
}

func TestRecoveryReadsAStoredTaskByOwnerWithoutTheBoundedRecentCatalog(t *testing.T) {
	listCalled := false
	set := Set{
		catalog: newCatalog(func(context.Context, int) (json.RawMessage, error) {
			listCalled = true
			return json.RawMessage(`{"data":[]}`), nil
		}, nil, nil),
		readAppTask: func(context.Context, string) (json.RawMessage, error) {
			return json.RawMessage(`{"thread":{"id":"thread-21","name":"Older task","preview":"","cwd":"/work/project","updatedAt":42,"status":{"type":"idle","activeFlags":[]},"turns":[]}}`), nil
		},
		startExistingTurn: func(_ context.Context, task taskstate.Task, text string) (string, error) {
			if task.ID != "thread-21" || text != "Resume it" {
				t.Fatalf("source-specific start = %#v, %q", task, text)
			}
			return "turn-started", nil
		},
	}
	task, err := set.CurrentTaskFromSource(context.Background(), "thread-21", taskstate.SourceAppServer)
	if err != nil || task.ID != "thread-21" || task.Source != taskstate.SourceAppServer || listCalled {
		t.Fatalf("source-specific recovery task = %#v, error = %v, list called = %v", task, err, listCalled)
	}
	started, err := set.StartExistingTurnFromSource(context.Background(), "thread-21", "Resume it", taskstate.SourceAppServer)
	if err != nil || started.TurnID != "turn-started" || listCalled {
		t.Fatalf("source-specific recovery start = %#v, error = %v, list called = %v", started, err, listCalled)
	}
}

func TestSourceSpecificStartMarksOnlyItsPreWriteLookupFailureAsTransient(t *testing.T) {
	set := Set{
		readAppTask: func(context.Context, string) (json.RawMessage, error) { return nil, context.DeadlineExceeded },
		startExistingTurn: func(context.Context, taskstate.Task, string) (string, error) {
			t.Fatal("write ran after failed lookup")
			return "", nil
		},
	}
	if _, err := set.StartExistingTurnFromSource(context.Background(), "thread-1", "Keep me", taskstate.SourceAppServer); !errors.Is(err, ErrTaskLookupTransient) || errors.Is(err, ErrTaskUnavailable) {
		t.Fatalf("source-specific lookup error = %v", err)
	}

	set.readDesktopTask = func(context.Context, string) (json.RawMessage, error) { return nil, desktopipc.ErrOwnerUnavailable }
	if _, err := set.CurrentTaskFromSource(context.Background(), "thread-1", taskstate.SourceDesktop); !errors.Is(err, desktopipc.ErrOwnerUnavailable) || errors.Is(err, ErrTaskUnavailable) {
		t.Fatalf("temporary Desktop owner error = %v", err)
	}
}

func TestExistingTaskControlsFailClosedWhenTaskStateOrCapabilityChanged(t *testing.T) {
	tests := []struct {
		name string
		task taskstate.Task
		run  func(Set) error
		want error
	}{
		{name: "start became busy", task: taskstate.Task{ID: "thread-1", State: taskstate.Working, Source: taskstate.SourceAppServer, ActiveTurnID: "turn-1"}, run: func(set Set) error {
			_, err := set.StartExistingTurn(context.Background(), "thread-1", "next")
			return err
		}, want: ErrTaskBusy},
		{name: "redirect became idle", task: taskstate.Task{ID: "thread-1", State: taskstate.IdleAfterReply, Source: taskstate.SourceAppServer}, run: func(set Set) error {
			_, err := set.RedirectExistingTurn(context.Background(), "thread-1", "next")
			return err
		}, want: ErrTaskNotBusy},
		{name: "redirect disabled", task: taskstate.Task{ID: "thread-1", State: taskstate.Working, Source: taskstate.SourceAppServer, ActiveTurnID: "turn-1"}, run: func(set Set) error {
			_, err := set.RedirectExistingTurn(context.Background(), "thread-1", "next")
			return err
		}, want: ErrRedirectUnsupported},
		{name: "stop became idle", task: taskstate.Task{ID: "thread-1", State: taskstate.IdleAfterReply, Source: taskstate.SourceAppServer}, run: func(set Set) error { _, err := set.InterruptExistingTurn(context.Background(), "thread-1"); return err }, want: ErrTaskNotBusy},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			set := Set{
				currentTask:           func(context.Context, string) (taskstate.Task, error) { return test.task, nil },
				startExistingTurn:     func(context.Context, taskstate.Task, string) (string, error) { called = true; return "", nil },
				redirectExistingTurn:  func(context.Context, taskstate.Task, string) (string, error) { called = true; return "", nil },
				interruptExistingTurn: func(context.Context, taskstate.Task) (string, error) { called = true; return "turn-1", nil },
			}
			if err := test.run(set); !errors.Is(err, test.want) || called {
				t.Fatalf("error = %v, want %v; write called = %v", err, test.want, called)
			}
		})
	}
}

func TestStopCanInterruptATurnWaitingForApprovalOrAnswer(t *testing.T) {
	for _, state := range []taskstate.State{taskstate.WaitingForApproval, taskstate.WaitingForAnswer} {
		called := false
		set := Set{
			currentTask: func(context.Context, string) (taskstate.Task, error) {
				return taskstate.Task{ID: "thread-1", State: state, Source: taskstate.SourceAppServer, ActiveTurnID: "turn-1"}, nil
			},
			interruptExistingTurn: func(context.Context, taskstate.Task) (string, error) { called = true; return "turn-1", nil },
		}
		if _, err := set.InterruptExistingTurn(context.Background(), "thread-1"); err != nil || !called {
			t.Fatalf("stop waiting state %q = called %v, error %v", state, called, err)
		}
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
			return json.RawMessage(`{"data":[{"id":"cli-1","name":"CLI task","preview":"","cwd":"/work/project","updatedAt":43,"status":{"type":"active","activeFlags":[]},"turns":[{"id":"turn-1","status":"inProgress"}]}]}`), nil
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
		return json.RawMessage(`{"data":[{"id":"task-1","preview":"Build launcher","cwd":"/work/launcher","updatedAt":42,"status":{"type":"active","activeFlags":[]},"turns":[{"id":"turn-1","status":"inProgress"}]}]}`), nil
	}, nil, nil)
	set := Set{catalog: catalog}

	tasks, err := set.ListRecent(context.Background(), 1)
	if err != nil || len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("recent tasks = %#v, %v", tasks, err)
	}
}

func TestSetCurrentTaskUsesTheConfiguredCatalogLookup(t *testing.T) {
	want := taskstate.Task{ID: "task-1", Source: taskstate.SourceAppServer}
	set := Set{currentTask: func(_ context.Context, taskID string) (taskstate.Task, error) {
		if taskID != want.ID {
			t.Fatalf("task ID = %q", taskID)
		}
		return want, nil
	}}
	task, err := set.CurrentTask(context.Background(), want.ID)
	if err != nil || task != want {
		t.Fatalf("CurrentTask() = %#v, %v", task, err)
	}
}

func TestSetListRecentShowsFourCandidatesWithoutStartingDesktopFollowing(t *testing.T) {
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
	if len(loaded) != 0 {
		t.Fatalf("loaded tasks = %#v", loaded)
	}
	if len(tasks) != 4 {
		t.Fatalf("Home task count = %d, tasks = %#v", len(tasks), tasks)
	}
	if got := []string{tasks[0].ID, tasks[1].ID, tasks[2].ID, tasks[3].ID}; strings.Join(got, ",") != "task-1,task-2,task-3,task-4" {
		t.Fatalf("Home task order = %#v", got)
	}
	for _, task := range tasks {
		if task.Source != taskstate.SourceCatalog {
			t.Fatalf("task source = %#v", task)
		}
	}
}

func TestSetListRecentKeepsDesktopCandidatesUnreadUntilRequested(t *testing.T) {
	loaded := make([]string, 0)
	stateReads := 0
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[
			{"id":"app-1","name":"App","preview":"","cwd":"/work/app","updatedAt":46,"status":{"type":"active","activeFlags":[]},"turns":[{"id":"turn-app","status":"inProgress"}]},
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
	if len(loaded) != 0 || stateReads != 0 {
		t.Fatalf("loaded = %#v, state reads = %d", loaded, stateReads)
	}
	byID := make(map[string]taskstate.Source, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task.Source
	}
	if byID["app-1"] != taskstate.SourceAppServer || byID["task-1"] != taskstate.SourceCatalog || byID["task-2"] != taskstate.SourceCatalog {
		t.Fatalf("Home task sources = %#v", byID)
	}
}

func TestSetListRecentDoesNotLetAnUnverifiedDesktopTaskBlockStartup(t *testing.T) {
	loadCalls := 0
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"slow","name":"Slow","preview":"","cwd":"/work","updatedAt":42,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]},{"id":"ready","name":"Ready","preview":"","cwd":"/work","updatedAt":43,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]}]}`), nil
	}, func(context.Context, string) error {
		loadCalls++
		return nil
	}, func(taskID string) (json.RawMessage, error) {
		return json.RawMessage(`{"id":"` + taskID + `","cwd":"/work","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[]}`), nil
	})
	set := Set{catalog: catalog}
	tasks, err := set.ListRecent(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if loadCalls != 0 || len(tasks) != 2 || tasks[0].Source != taskstate.SourceCatalog || tasks[1].Source != taskstate.SourceCatalog {
		t.Fatalf("load calls = %d, tasks = %#v", loadCalls, tasks)
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

func TestCatalogReadsBoundedAppServerTranscriptWithTurns(t *testing.T) {
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"app-1","name":"App","preview":"","cwd":"/work/app","updatedAt":46,"status":{"type":"active","activeFlags":[]},"turns":[{"status":"completed"}]}]}`), nil
	}, nil, nil)
	readTaskID := ""
	catalog.readAppTranscript = func(_ context.Context, taskID string) (json.RawMessage, error) {
		readTaskID = taskID
		return json.RawMessage(`{"thread":{"id":"app-1","turns":[{"id":"turn-1","status":"completed","items":[{"id":"agent-1","type":"agentMessage","text":"Done"}]}]}}`), nil
	}
	if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
		t.Fatal(err)
	}

	page, err := catalog.ReadTranscript(context.Background(), "app-1", tasktranscript.PageOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if readTaskID != "app-1" || page.TaskID != "app-1" || len(page.Entries) != 1 || page.Entries[0].Text != "Done" {
		t.Fatalf("app-server transcript = %#v, read task = %q", page, readTaskID)
	}
}

func TestCatalogReadsOnlyVerifiedDesktopTranscriptAndRejectsCandidate(t *testing.T) {
	state := json.RawMessage(`{"id":"desktop-1","cwd":"/work/desktop","threadRuntimeStatus":{"type":"idle","activeFlags":[]},"requests":[],"turns":[{"role":"assistant","text":"Desktop reply"}]}`)
	catalog := newCatalog(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"desktop-1","name":"Desktop","preview":"","cwd":"/work/desktop","updatedAt":45,"status":{"type":"notLoaded","activeFlags":[]},"turns":[]}]}`), nil
	}, func(context.Context, string) error { return nil }, func(string) (json.RawMessage, error) { return state, nil })
	if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ReadTranscript(context.Background(), "desktop-1", tasktranscript.PageOptions{Limit: 10}); !errors.Is(err, taskstate.ErrUnresolvedTaskSource) {
		t.Fatalf("unverified Desktop transcript error = %v", err)
	}
	if _, err := catalog.ResolveDesktopOwner(context.Background(), "desktop-1"); err != nil {
		t.Fatal(err)
	}
	page, err := catalog.ReadTranscript(context.Background(), "desktop-1", tasktranscript.PageOptions{Limit: 10})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "Desktop reply" {
		t.Fatalf("verified Desktop transcript = %#v, %v", page, err)
	}
}

func TestCatalogTranscriptFailuresLogOnlySafeMetadata(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	catalog := newCatalogWithLogger(func(context.Context, int) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"app-1","name":"App","preview":"","cwd":"/work/app","updatedAt":46,"status":{"type":"active","activeFlags":[]},"turns":[]}]}`), nil
	}, nil, nil, logger)
	catalog.readAppTranscript = func(context.Context, string) (json.RawMessage, error) {
		return nil, errors.New("private transcript provider failure")
	}
	if _, err := catalog.ListRecent(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ReadTranscript(context.Background(), "app-1", tasktranscript.PageOptions{Limit: 10}); err == nil {
		t.Fatal("transcript provider error was hidden")
	}
	if strings.Contains(logs.String(), "private transcript provider failure") || !strings.Contains(logs.String(), "error_class") {
		t.Fatalf("unsafe transcript logs: %s", logs.String())
	}
}
