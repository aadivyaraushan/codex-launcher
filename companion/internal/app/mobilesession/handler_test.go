package mobilesession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

var sessionNow = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

func TestColdHelloSendsWelcomeAndSafeProjectSnapshot(t *testing.T) {
	handler, sender := newTestHandler(t)

	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}

	if len(sender.messages) != 2 || sender.messages[0].Type != "welcome" || sender.messages[1].Type != "snapshot" {
		t.Fatalf("outputs = %#v", sender.messages)
	}
	var snapshot struct {
		BaseSequence uint64 `json:"baseSeq"`
		ComputerName string `json:"computerName"`
		Projects     []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"projects"`
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(sender.messages[1].Body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.BaseSequence != 1 || snapshot.ComputerName != "Studio Mac" || len(snapshot.Projects) != 1 || snapshot.Projects[0].ID != "main" || snapshot.Projects[0].DisplayName != "Main" || len(snapshot.Tasks) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if bytes.Contains(sender.messages[1].Body, []byte(sender.projectPath)) {
		t.Fatal("snapshot exposed a configured path")
	}
}

func TestColdHelloIncludesOnlyTypedSafeTaskSummaries(t *testing.T) {
	handler, sender := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{{
			ID: "thread-1", Title: "Build launcher", ProjectLabel: "uf-u", State: taskstate.Working,
			UpdatedAtUnix: sessionNow.Add(-time.Minute).Unix(), Source: taskstate.SourceDesktop,
		}}, nil
	}))

	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-tasks","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	var welcome struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(sender.messages[0].Body, &welcome); err != nil {
		t.Fatal(err)
	}
	if len(welcome.Capabilities) != 2 || welcome.Capabilities[0] != "set_project" || welcome.Capabilities[1] != "desktop_tasks" {
		t.Fatalf("capabilities = %#v", welcome.Capabilities)
	}
	var snapshot struct {
		Tasks []struct {
			TaskID         string `json:"taskId"`
			Title          string `json:"title"`
			ProjectLabel   string `json:"projectLabel"`
			State          string `json:"state"`
			LastActivityAt string `json:"lastActivityAt"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(sender.messages[1].Body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].TaskID != "thread-1" || snapshot.Tasks[0].Title != "Build launcher" ||
		snapshot.Tasks[0].ProjectLabel != "uf-u" || snapshot.Tasks[0].State != "working" || snapshot.Tasks[0].LastActivityAt != "2026-07-13T11:59:00Z" {
		t.Fatalf("tasks = %#v", snapshot.Tasks)
	}
	if bytes.Contains(sender.messages[1].Body, []byte(`"source"`)) || bytes.Contains(sender.messages[1].Body, []byte(`"raw"`)) {
		t.Fatalf("snapshot exposed an internal task field: %s", sender.messages[1].Body)
	}
}

func TestTaskReadSendsUnsequencedPageWithoutPersistingTranscriptContent(t *testing.T) {
	privateReply := "private Desktop reply"
	var gotOptions tasktranscript.PageOptions
	source := transcriptTaskSource{
		list: func(context.Context, int) ([]taskstate.Task, error) {
			return []taskstate.Task{{
				ID: "thread-1", Title: "Build launcher", ProjectLabel: "uf-u", State: taskstate.IdleAfterReply,
				UpdatedAtUnix: sessionNow.Unix(), Source: taskstate.SourceDesktop,
			}}, nil
		},
		read: func(_ context.Context, taskID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
			gotOptions = options
			return tasktranscript.Page{
				TaskID:  taskID,
				Entries: []tasktranscript.Entry{{ID: "agent-1", TurnID: "turn-1", Kind: tasktranscript.KindAgent, Text: privateReply}},
			}, nil
		},
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-transcript","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	var welcome struct {
		Capabilities []string `json:"capabilities"`
	}
	if json.Unmarshal(sender.messages[0].Body, &welcome) != nil || strings.Join(welcome.Capabilities, ",") != "set_project,desktop_tasks,task_transcripts" {
		t.Fatalf("capabilities = %#v", welcome.Capabilities)
	}
	read := decode(t, `{"version":{"major":1,"minor":0},"messageId":"read-transcript","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":32,"beforeEntryId":"agent-2"}}`)
	if err := handler.Handle(context.Background(), sender, read); err != nil {
		t.Fatal(err)
	}
	if gotOptions.TaskID != "thread-1" || gotOptions.Limit != 32 || gotOptions.BeforeEntryID != "agent-2" {
		t.Fatalf("read options = %#v", gotOptions)
	}
	if len(sender.messages) != 3 || sender.messages[2].Type != "task_page" || sender.messages[2].Sequence != nil || !bytes.Contains(sender.messages[2].Body, []byte(privateReply)) {
		t.Fatalf("task page = %#v", sender.messages)
	}
	snapshot, err := handler.journal.Snapshot(sessionNow)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(snapshot.Body, []byte(privateReply)) {
		t.Fatalf("transcript content reached durable snapshot: %s", snapshot.Body)
	}
}

func TestTaskReadIsUnavailableWhenSourceCannotReadTranscripts(t *testing.T) {
	handler, sender := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
	}))
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-no-transcript","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	read := decode(t, `{"version":{"major":1,"minor":0},"messageId":"read-transcript","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":32}}`)
	if err := handler.Handle(context.Background(), sender, read); !errors.Is(err, ErrUnsupportedMessage) {
		t.Fatalf("task read error = %v, want %v", err, ErrUnsupportedMessage)
	}
}

func TestTaskReadRejectsCrossTaskSourceResultWithoutLeakingContent(t *testing.T) {
	privateOtherTaskReply := "private reply from another task"
	source := transcriptTaskSource{
		list: func(context.Context, int) ([]taskstate.Task, error) {
			return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
		},
		read: func(context.Context, string, tasktranscript.PageOptions) (tasktranscript.Page, error) {
			return tasktranscript.Page{
				TaskID:  "thread-2",
				Entries: []tasktranscript.Entry{{ID: "agent-1", TurnID: "turn-1", Kind: tasktranscript.KindAgent, Text: privateOtherTaskReply}},
			}, nil
		},
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-cross-task","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	read := decode(t, `{"version":{"major":1,"minor":0},"messageId":"read-cross-task","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":32}}`)
	if err := handler.Handle(context.Background(), sender, read); err != nil {
		t.Fatal(err)
	}
	response := sender.messages[len(sender.messages)-1]
	if response.Type != "task_page" || bytes.Contains(response.Body, []byte(privateOtherTaskReply)) || !bytes.Contains(response.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("cross-task response = %s", response.Body)
	}
}

func TestTaskReadConvertsInvalidSourcePageToContentFreeInternalError(t *testing.T) {
	privateReply := "must not leave the companion"
	source := transcriptTaskSource{
		list: func(context.Context, int) ([]taskstate.Task, error) {
			return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
		},
		read: func(context.Context, string, tasktranscript.PageOptions) (tasktranscript.Page, error) {
			return tasktranscript.Page{TaskID: "thread-1", Entries: []tasktranscript.Entry{{
				ID: "plan-1", TurnID: "turn-1", Kind: tasktranscript.KindPlan, Text: "", Output: privateReply,
			}}}, nil
		},
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-invalid-page","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	read := decode(t, `{"version":{"major":1,"minor":0},"messageId":"read-invalid-page","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":32}}`)
	if err := handler.Handle(context.Background(), sender, read); err != nil {
		t.Fatal(err)
	}
	response := sender.messages[len(sender.messages)-1]
	if response.Type != "task_page" || bytes.Contains(response.Body, []byte(privateReply)) || !bytes.Contains(response.Body, []byte(`"entries":[]`)) || !bytes.Contains(response.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("invalid source response = %s", response.Body)
	}
}

func TestTaskSnapshotFailsClosedWhenCatalogFailsOrReturnsUnsafeData(t *testing.T) {
	for _, test := range []struct {
		name   string
		source TaskSource
	}{
		{name: "catalog failure", source: taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) { return nil, errors.New("offline") })},
		{name: "unsafe id", source: taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
			return []taskstate.Task{{ID: "thread\n1", Title: "Task", ProjectLabel: "Project", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
		})},
		{name: "missing activity time", source: taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
			return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "Project", State: taskstate.Working}}, nil
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
			if err != nil {
				t.Fatal(err)
			}
			journal := eventjournal.New(eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 16, MaxBytes: 64 * 1024}), nil)
			if _, err := NewWithTaskSource(context.Background(), "Studio Mac", projectService, journal, test.source, nil, func() time.Time { return sessionNow }); err == nil {
				t.Fatal("unsafe task source was accepted")
			}
		})
	}
}

func TestTaskSnapshotRequestsAndEnforcesFourHomeTasks(t *testing.T) {
	requestedLimit := 0
	source := taskSourceFunc(func(_ context.Context, limit int) ([]taskstate.Task, error) {
		requestedLimit = limit
		return []taskstate.Task{
			{ID: "thread-1", Title: "One", ProjectLabel: "Project", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()},
			{ID: "thread-2", Title: "Two", ProjectLabel: "Project", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()},
			{ID: "thread-3", Title: "Three", ProjectLabel: "Project", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()},
			{ID: "thread-4", Title: "Four", ProjectLabel: "Project", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()},
			{ID: "thread-5", Title: "Five", ProjectLabel: "Project", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()},
		}, nil
	})
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	journal := eventjournal.New(eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 16, MaxBytes: 64 * 1024}), nil)
	if _, err := NewWithTaskSource(context.Background(), "Studio Mac", projectService, journal, source, nil, func() time.Time { return sessionNow }); err == nil {
		t.Fatal("five Home tasks were accepted")
	}
	if requestedLimit != 4 {
		t.Fatalf("Home task request limit = %d", requestedLimit)
	}
}

func TestEachHelloRefreshesTasksWithANewSnapshotBase(t *testing.T) {
	calls := 0
	source := taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		calls++
		return []taskstate.Task{{
			ID: "thread-1", Title: "Task version", ProjectLabel: "uf-u", State: taskstate.IdleAfterReply,
			UpdatedAtUnix: sessionNow.Add(time.Duration(calls) * time.Second).Unix(),
		}}, nil
	})
	handler, first := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), first, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-first","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	second := &recordingSender{deviceID: first.deviceID, sessionID: "session-2", connectionID: 2, projectPath: first.projectPath, store: first.store}
	if err := handler.Handle(context.Background(), second, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-second","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("task source calls = %d, want startup plus both hellos", calls)
	}
	firstSnapshot, secondSnapshot := first.messages[1], second.messages[1]
	if firstSnapshot.Sequence == nil || secondSnapshot.Sequence == nil || *secondSnapshot.Sequence <= *firstSnapshot.Sequence {
		t.Fatalf("snapshot sequences = %v then %v", firstSnapshot.Sequence, secondSnapshot.Sequence)
	}
	if bytes.Equal(firstSnapshot.Body, secondSnapshot.Body) {
		t.Fatal("second hello reused stale task snapshot")
	}
}

func TestSetProjectReturnsSequencedConfirmedOrFailedResult(t *testing.T) {
	handler, sender := newTestHandler(t)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)

	for _, test := range []struct {
		actionID string
		project  string
		state    string
		error    string
	}{
		{actionID: "action-ok", project: "main", state: "confirmed"},
		{actionID: "action-missing", project: "removed", state: "failed", error: "invalid_action"},
	} {
		body := `{"actionId":"` + test.actionID + `","kind":"set_project","projectId":"` + test.project + `"}`
		message := contract.Message{Version: contract.Version{Major: 1}, MessageID: "request-" + test.actionID, Sender: "phone", Type: "action", Body: json.RawMessage(body)}
		if err := handler.Handle(context.Background(), sender, message); err != nil {
			t.Fatal(err)
		}
		result := awaitSentMessage(t, sender.sent)
		if result.Type != "action_result" || result.Sequence == nil {
			t.Fatalf("result = %#v", result)
		}
		var resultBody struct {
			ActionID string `json:"actionId"`
			State    string `json:"state"`
			Error    *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(result.Body, &resultBody)
		if resultBody.ActionID != test.actionID || resultBody.State != test.state || test.error != "" && (resultBody.Error == nil || resultBody.Error.Code != test.error) {
			t.Fatalf("result body = %#v", resultBody)
		}
	}
	if *sender.messages[0].Sequence != 2 || *sender.messages[1].Sequence != 3 {
		t.Fatalf("result sequences = %d, %d", *sender.messages[0].Sequence, *sender.messages[1].Sequence)
	}
}

func TestTaskManagementActionsUseTheVerifiedAdapterAndRefreshTheSnapshot(t *testing.T) {
	for _, test := range []struct {
		kind string
		body string
		want string
	}{
		{kind: "rename_task", body: `,"title":"Renamed task"`, want: "rename:thread-1:Renamed task"},
		{kind: "archive_task", want: "archive:thread-1"},
		{kind: "fork_task", want: "fork:thread-1"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			source := &taskManagementSource{tasks: []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}}}
			handler, sender := newTestHandlerWithTasks(t, source)
			if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-manage","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
				t.Fatal(err)
			}
			var welcome struct {
				Capabilities []string `json:"capabilities"`
			}
			if json.Unmarshal(sender.messages[0].Body, &welcome) != nil || !slices.Contains(welcome.Capabilities, "task_management") {
				t.Fatalf("capabilities = %#v", welcome.Capabilities)
			}
			sender.messages = nil
			sender.sent = make(chan contract.Message, 2)
			action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"manage","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"`+test.kind+`","taskId":"thread-1"`+test.body+`}}`)

			if err := handler.Handle(context.Background(), sender, action); err != nil {
				t.Fatal(err)
			}
			result := awaitSentMessage(t, sender.sent)
			snapshot := awaitSentMessage(t, sender.sent)
			if result.Type != "action_result" || result.Sequence == nil || *result.Sequence != 3 || !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) {
				t.Fatalf("result = %#v", result)
			}
			if snapshot.Type != "snapshot" || snapshot.Sequence == nil || *snapshot.Sequence != 4 {
				t.Fatalf("snapshot = %#v", snapshot)
			}
			if len(source.calls) != 1 || source.calls[0] != test.want {
				t.Fatalf("calls = %#v", source.calls)
			}
		})
	}
}

func TestTaskManagementFailureReturnsSafeErrorWithoutPublishingStaleSnapshot(t *testing.T) {
	source := &taskManagementSource{
		tasks: []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}},
		fail:  errors.New("private adapter detail"),
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-manage","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"archive_task","taskId":"thread-1"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"failed"`)) || !bytes.Contains(result.Body, []byte(`"code":"internal"`)) || bytes.Contains(result.Body, []byte("private adapter detail")) {
		t.Fatalf("result = %#v", result)
	}
	select {
	case extra := <-sender.sent:
		t.Fatalf("unexpected stale snapshot = %#v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestOutcomeUnknownTaskMutationIsNeverReportedAsRetryableFailure(t *testing.T) {
	source := &taskManagementSource{
		tasks: []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}},
		fail:  fmt.Errorf("wrapped adapter failure: %w", &appserver.OutcomeUnknownError{Method: "thread/fork", Cause: errors.New("response lost")}),
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 1)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"fork","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"fork_task","taskId":"thread-1"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"outcome_unknown"`)) || !bytes.Contains(result.Body, []byte(`"code":"outcome_unknown"`)) || !bytes.Contains(result.Body, []byte(`"retryable":false`)) {
		t.Fatalf("result = %s", result.Body)
	}
}

func TestSupersededSessionCannotReachTaskManagementSource(t *testing.T) {
	source := &taskManagementSource{tasks: []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}}}
	handler, first := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), first, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	second := &recordingSender{deviceID: first.deviceID, sessionID: "session-2", connectionID: 2, projectPath: first.projectPath, store: first.store}
	if err := handler.Handle(context.Background(), second, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-2","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"action-old","kind":"archive_task","taskId":"thread-1"}}`)

	if err := handler.Handle(context.Background(), first, action); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("old session error = %v", err)
	}
	if len(source.calls) != 0 {
		t.Fatalf("superseded session reached task source: %#v", source.calls)
	}
}

func TestConfirmedTaskActionSurvivesSnapshotRefreshFailure(t *testing.T) {
	source := &taskManagementSource{
		tasks:                 []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}},
		listFailuresRemaining: 1,
		failListAfter:         2,
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	handler.taskRefreshWait = func(context.Context, int) bool { return true }
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-manage","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"rename","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"rename_task","taskId":"thread-1","title":"Renamed"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatalf("confirmed task action was turned into a session failure: %v", err)
	}
	result := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) {
		t.Fatalf("result = %#v", result)
	}
	snapshot := awaitSentMessage(t, sender.sent)
	if snapshot.Type != "snapshot" || snapshot.Sequence == nil || *snapshot.Sequence != 4 || !bytes.Contains(snapshot.Body, []byte(`"title":"Renamed"`)) {
		t.Fatalf("recovered snapshot = %#v", snapshot)
	}
}

func TestExhaustedTaskRefreshForcesFreshReconnect(t *testing.T) {
	source := &taskManagementSource{
		tasks:                 []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}},
		listFailuresRemaining: 4,
		failListAfter:         2,
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	handler.taskRefreshWait = func(context.Context, int) bool { return true }
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	sender.closedSignal = make(chan struct{})
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"archive_task","taskId":"thread-1"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	_ = awaitSentMessage(t, sender.sent)
	select {
	case <-sender.closedSignal:
	case <-time.After(time.Second):
		t.Fatal("session was not closed after bounded refresh retries")
	}
}

func TestAcknowledgementIsStoredPerAuthenticatedDevice(t *testing.T) {
	handler, sender := newTestHandler(t)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	ack := contract.Message{Version: contract.Version{Major: 1}, MessageID: "ack-1", Sender: "phone", Type: "ack", Body: json.RawMessage(`{"throughSeq":1}`)}
	if err := handler.Handle(context.Background(), sender, ack); err != nil {
		t.Fatal(err)
	}
	through, err := sender.store.Acknowledged(context.Background(), sender.DeviceID())
	if err != nil || through != 1 {
		t.Fatalf("acknowledged = %d, %v", through, err)
	}
}

func TestNewHelloReplacesTheOlderDeviceSessionBeforeMoreEventsAreApplied(t *testing.T) {
	handler, first := newTestHandler(t)
	if err := handler.Handle(context.Background(), first, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	second := &recordingSender{deviceID: first.deviceID, sessionID: "session-2", connectionID: 2, projectPath: first.projectPath, store: first.store}
	if err := handler.Handle(context.Background(), second, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-2","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	if !first.closed {
		t.Fatal("older device session remained open")
	}
	action := contract.Message{Version: contract.Version{Major: 1}, MessageID: "request-old", Sender: "phone", Type: "action", Body: json.RawMessage(`{"actionId":"action-old","kind":"set_project","projectId":"main"}`)}
	if err := handler.Handle(context.Background(), first, action); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("older session action error = %v", err)
	}
	action.MessageID = "request-current"
	action.Body = json.RawMessage(`{"actionId":"action-current","kind":"set_project","projectId":"main"}`)
	second.sent = make(chan contract.Message, 1)
	if err := handler.Handle(context.Background(), second, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, second.sent)
	if result.Sequence == nil || *result.Sequence != 2 {
		t.Fatalf("current session result sequence = %v, want 2", result.Sequence)
	}
}

type recordingSender struct {
	deviceID     string
	sessionID    string
	messages     []contract.Message
	projectPath  string
	store        *eventjournal.MemoryStore
	connectionID uint64
	closed       bool
	closedSignal chan struct{}
	sent         chan contract.Message
}

func (sender *recordingSender) DeviceID() string     { return sender.deviceID }
func (sender *recordingSender) SessionID() string    { return sender.sessionID }
func (sender *recordingSender) ConnectionID() uint64 { return sender.connectionID }
func (sender *recordingSender) Close() {
	if sender.closed {
		return
	}
	sender.closed = true
	if sender.closedSignal != nil {
		close(sender.closedSignal)
	}
}
func (sender *recordingSender) Send(_ context.Context, message contract.Message) error {
	encoded, err := contract.EncodeText(message)
	if err != nil {
		return err
	}
	decoded, err := contract.DecodeText(encoded)
	if err == nil {
		sender.messages = append(sender.messages, decoded)
		if sender.sent != nil {
			sender.sent <- decoded
		}
	}
	return err
}

func newTestHandler(t *testing.T) (*Handler, *recordingSender) {
	return newTestHandlerWithTasks(t, nil)
}

func newTestHandlerWithTasks(t *testing.T, taskSource TaskSource) (*Handler, *recordingSender) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	store := eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 16, MaxBytes: 64 * 1024})
	journal := eventjournal.New(store, nil)
	handler, err := NewWithTaskSource(context.Background(), "Studio Mac", projectService, journal, taskSource, nil, func() time.Time { return sessionNow })
	if err != nil {
		t.Fatal(err)
	}
	return handler, &recordingSender{deviceID: "pixel-9", sessionID: "session-1", connectionID: 1, projectPath: root, store: store}
}

type taskSourceFunc func(context.Context, int) ([]taskstate.Task, error)

func (source taskSourceFunc) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	return source(ctx, limit)
}

type transcriptTaskSource struct {
	list func(context.Context, int) ([]taskstate.Task, error)
	read func(context.Context, string, tasktranscript.PageOptions) (tasktranscript.Page, error)
}

type taskManagementSource struct {
	tasks                 []taskstate.Task
	calls                 []string
	fail                  error
	listCalls             int
	failListAfter         int
	listFailuresRemaining int
}

func (source *taskManagementSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	source.listCalls++
	if source.failListAfter > 0 && source.listCalls > source.failListAfter && source.listFailuresRemaining > 0 {
		source.listFailuresRemaining--
		return nil, errors.New("private list failure")
	}
	return append([]taskstate.Task(nil), source.tasks...), nil
}

func (source *taskManagementSource) Rename(_ context.Context, taskID, title string) error {
	source.calls = append(source.calls, "rename:"+taskID+":"+title)
	if source.fail != nil {
		return source.fail
	}
	for index := range source.tasks {
		if source.tasks[index].ID == taskID {
			source.tasks[index].Title = title
		}
	}
	return nil
}

func (source *taskManagementSource) Archive(_ context.Context, taskID string) error {
	source.calls = append(source.calls, "archive:"+taskID)
	if source.fail != nil {
		return source.fail
	}
	filtered := source.tasks[:0]
	for _, task := range source.tasks {
		if task.ID != taskID {
			filtered = append(filtered, task)
		}
	}
	source.tasks = filtered
	return nil
}

func (source *taskManagementSource) ForkToAppServer(_ context.Context, taskID string) (taskstate.Task, error) {
	source.calls = append(source.calls, "fork:"+taskID)
	if source.fail != nil {
		return taskstate.Task{}, source.fail
	}
	fork := taskstate.Task{ID: "fork-1", Title: "Fork", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix() + 1}
	source.tasks = append(source.tasks, fork)
	return fork, nil
}

func (source transcriptTaskSource) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	return source.list(ctx, limit)
}

func (source transcriptTaskSource) ReadTranscript(ctx context.Context, taskID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	return source.read(ctx, taskID, options)
}

func decode(t *testing.T, frame string) contract.Message {
	t.Helper()
	message, err := contract.DecodeText([]byte(frame))
	if err != nil {
		t.Fatal(err)
	}
	return message
}
