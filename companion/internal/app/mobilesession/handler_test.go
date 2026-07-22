package mobilesession

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/attachments"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskoptions"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

var sessionNow = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

func TestDecisionReadIsLiveOnlyAndApprovalRoutesToExactPendingRequest(t *testing.T) {
	handler, sender := newTestHandler(t)
	responder := &recordingDecisionResponder{}
	router := decisions.NewRouter(responder, nil)
	request := decisions.Request{
		ID: "approval-1", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Kind: decisions.KindCommand,
		ComputerName: "Studio Mac", ProjectLabel: "Main", WorkingDirectory: "/work", Reason: "Run tests", Command: "npm test", CommandUnderstandable: true,
		AllowedDecisions: []decisions.Decision{decisions.DecisionAcceptOnce, decisions.DecisionDecline}, ExpiresAt: sessionNow.Add(time.Minute),
	}
	if err := router.Add(request); err != nil {
		t.Fatal(err)
	}
	handler.EnableDecisions(router)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"read","sender":"phone","type":"decision_read","body":{"requestId":"read-1","taskId":"thread-1"}}`)); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 1 || sender.messages[0].Type != "decision_page" || sender.messages[0].Sequence != nil {
		t.Fatalf("decision response = %#v", sender.messages)
	}
	if events, err := sender.store.ReplayAfter(context.Background(), 1); err != nil || len(events) != 0 {
		t.Fatalf("decision page entered replay journal: %#v, %v", events, err)
	}
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"decline","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"approval","taskId":"thread-1","requestId":"approval-1","requestKind":"command","decision":"decline"}}`)); err != nil {
		t.Fatal(err)
	}
	if len(responder.responses) != 1 || responder.responses[0].RequestID != "approval-1" || responder.responses[0].Decision != decisions.DecisionDecline {
		t.Fatalf("routed responses = %#v", responder.responses)
	}
}

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

func TestAttachmentCapabilityAndAcknowledgementUseDurableSequence(t *testing.T) {
	handler, sender := newTestHandler(t)
	handler.EnableAttachments(openAttachmentStore(t))
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-attachments","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	var welcome struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(sender.messages[0].Body, &welcome); err != nil || !slices.Contains(welcome.Capabilities, "attachments") {
		t.Fatalf("attachment welcome = %#v, %v", welcome, err)
	}

	sender.sent = make(chan contract.Message, 1)
	event := transport.AttachmentEvent{
		UploadID: "upload-1", State: "accepted", ReceivedBytes: 4,
		SHA256: strings.Repeat("a", 64), NextChunk: 1,
	}
	if err := handler.PublishAttachmentAck(context.Background(), sender, event); err != nil {
		t.Fatal(err)
	}
	message := awaitSentMessage(t, sender.sent)
	if message.Type != "attachment_ack" || message.Sequence == nil || *message.Sequence != 2 {
		t.Fatalf("attachment ack envelope = %#v", message)
	}
	var body transport.AttachmentEvent
	if err := json.Unmarshal(message.Body, &body); err != nil || body != event {
		t.Fatalf("attachment ack body = %#v, %v", body, err)
	}
	events, err := sender.store.ReplayAfter(context.Background(), 1)
	if err != nil || len(events) != 1 || events[0].Name != "attachment_ack" || events[0].Sequence != 2 {
		t.Fatalf("durable attachment events = %#v, %v", events, err)
	}
}

func TestBusyAttachmentActionSurvivesQueueAndReleasesOnlyAfterCodexHandoff(t *testing.T) {
	attachmentStore := openAttachmentStore(t)
	completeAttachment(t, attachmentStore, "pixel-9", "upload-1", []byte("private notes"))
	source := &existingTaskSource{task: taskstate.Task{
		ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-active", CanRedirect: true, UpdatedAtUnix: sessionNow.Unix(),
	}}
	promptStore := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptStore)
	handler.EnableAttachments(attachmentStore)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 4)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"with-file","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","taskId":"thread-1","text":"Inspect this","attachmentIds":["upload-1"]}}`)
	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"resultCode":"queued"`)) {
		t.Fatalf("queued result = %s", result.Body)
	}
	entry, err := promptStore.Entry(context.Background(), "action-1")
	if err != nil || entry.DeviceID != "pixel-9" || !slices.Equal(entry.AttachmentIDs, []string{"upload-1"}) {
		t.Fatalf("queued attachment entry = %#v, %v", entry, err)
	}
	if _, err := attachmentStore.ResolveClaimed("pixel-9", []string{"upload-1"}, "action-1", sessionNow.Add(2*time.Minute)); err != nil {
		t.Fatalf("queued claim did not outlive upload expiry: %v", err)
	}

	source.task.State = taskstate.IdleAfterReply
	source.task.ActiveTurnID = ""
	source.task.CanRedirect = false
	if err := handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{TaskID: "thread-1", Kind: "reply", State: taskstate.IdleAfterReply, Summary: "Codex replied"}); err != nil {
		t.Fatal(err)
	}
	if len(source.attachments) != 1 || source.attachments[0].ID != "upload-1" || source.attachments[0].Path == "" || source.attachments[0].MediaType == "" {
		t.Fatalf("Codex attachment handoff = %#v", source.attachments)
	}
	if _, err := attachmentStore.ResolveClaimed("pixel-9", []string{"upload-1"}, "action-1", sessionNow); !errors.Is(err, attachments.ErrAttachmentUnavailable) {
		t.Fatalf("confirmed handoff retained attachment: %v", err)
	}
}

func TestAttachmentActionFailsBeforeQueueWhenUploadIsMissing(t *testing.T) {
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}}
	promptStore := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptStore)
	handler.EnableAttachments(openAttachmentStore(t))
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"missing","sender":"phone","type":"action","body":{"actionId":"action-missing","kind":"start_turn","taskId":"thread-1","text":"Inspect","attachmentIds":["missing"]}}`)); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) || len(source.calls) != 0 {
		t.Fatalf("missing attachment result = %s, calls = %#v", result.Body, source.calls)
	}
	if _, err := promptStore.Entry(context.Background(), "action-missing"); !errors.Is(err, promptqueue.ErrActionNotFound) {
		t.Fatalf("missing attachment reached durable queue: %v", err)
	}
}

func TestStartupReconcilesAttachmentClaimsAgainstDurableQueueBeforeRecovery(t *testing.T) {
	attachmentStore := openAttachmentStore(t)
	for _, uploadID := range []string{"active-upload", "orphan-upload"} {
		completeAttachment(t, attachmentStore, "pixel-9", uploadID, []byte(uploadID))
		if err := attachmentStore.Claim("pixel-9", []string{uploadID}, strings.TrimSuffix(uploadID, "-upload")+"-action", sessionNow); err != nil {
			t.Fatal(err)
		}
	}
	promptStore := promptqueue.NewMemoryStore()
	queue := promptqueue.New(promptStore, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "active-action", QueueKey: "thread-1", ThreadID: "thread-1", Prompt: "inspect", DeviceID: "pixel-9", AttachmentIDs: []string{"active-upload"}, CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	journal := eventjournal.New(eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 16, MaxBytes: 64 * 1024}), nil)
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-1", UpdatedAtUnix: sessionNow.Unix()}}
	if _, err := NewWithTaskSourceQueueAndAttachments(
		context.Background(), "Studio Mac", projectService, journal, source, queue, attachmentStore, nil, func() time.Time { return sessionNow },
	); err != nil {
		t.Fatal(err)
	}
	if _, err := attachmentStore.ResolveClaimed("pixel-9", []string{"active-upload"}, "active-action", sessionNow.Add(365*24*time.Hour)); err != nil {
		t.Fatalf("active startup claim was lost: %v", err)
	}
	if _, err := attachmentStore.ResolveClaimed("pixel-9", []string{"orphan-upload"}, "orphan-action", sessionNow); !errors.Is(err, attachments.ErrAttachmentUnavailable) {
		t.Fatalf("orphan startup claim remains: %v", err)
	}
}

func TestUnknownNewTaskAttachmentCanBeDismissedWithoutAThreadID(t *testing.T) {
	attachmentStore := openAttachmentStore(t)
	completeAttachment(t, attachmentStore, "pixel-9", "upload-new", []byte("new task file"))
	source := &newTaskSource{
		catalog:  testTaskOptionsCatalog(),
		startErr: &appserver.OutcomeUnknownError{Method: "turn/start", Cause: errors.New("connection lost")},
	}
	promptStore := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptStore)
	handler.EnableAttachments(attachmentStore)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 3)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"new","sender":"phone","type":"action","body":{"actionId":"unknown-new","kind":"start_turn","projectId":"main","text":"Inspect","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write","attachmentIds":["upload-new"]}}`)); err != nil {
		t.Fatal(err)
	}
	if result := awaitSentMessage(t, sender.sent); !bytes.Contains(result.Body, []byte(`"state":"outcome_unknown"`)) {
		t.Fatalf("new task result = %s", result.Body)
	}
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"dismiss","sender":"phone","type":"action","body":{"actionId":"dismiss-new","kind":"dismiss_unknown_control","targetActionId":"unknown-new"}}`)); err != nil {
		t.Fatal(err)
	}
	if result := awaitSentMessage(t, sender.sent); !bytes.Contains(result.Body, []byte(`"resultCode":"accepted"`)) {
		t.Fatalf("dismiss result = %s", result.Body)
	}
	entry, err := promptStore.Entry(context.Background(), "unknown-new")
	if err != nil || entry.State != promptqueue.StateCanceled {
		t.Fatalf("dismissed new task = %#v, %v", entry, err)
	}
	if _, err := attachmentStore.ResolveClaimed("pixel-9", []string{"upload-new"}, "unknown-new", sessionNow); !errors.Is(err, attachments.ErrAttachmentUnavailable) {
		t.Fatalf("dismissed new-task claim remains: %v", err)
	}
}

func TestDeviceCannotDismissAnotherDevicesUnknownAttachmentAction(t *testing.T) {
	attachmentStore := openAttachmentStore(t)
	completeAttachment(t, attachmentStore, "other-phone", "upload-other", []byte("private file"))
	if err := attachmentStore.Claim("other-phone", []string{"upload-other"}, "unknown-other", sessionNow); err != nil {
		t.Fatal(err)
	}
	promptStore := promptqueue.NewMemoryStore()
	queue := promptqueue.New(promptStore, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "unknown-other", QueueKey: "thread-1", ThreadID: "thread-1", DeviceID: "other-phone",
		AttachmentIDs: []string{"upload-other"}, Prompt: "Private", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = queue.DispatchNext(context.Background(), "thread-1", func(context.Context, promptqueue.Entry) (promptqueue.Result, error) {
		return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
	}, nil, sessionNow.Add(time.Second))
	handler, sender := newTestHandlerWithTaskQueue(t, nil, promptStore)
	handler.EnableAttachments(attachmentStore)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 1)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"dismiss","sender":"phone","type":"action","body":{"actionId":"dismiss-other","kind":"dismiss_unknown_control","taskId":"thread-1","targetActionId":"unknown-other"}}`)); err != nil {
		t.Fatal(err)
	}
	if result := awaitSentMessage(t, sender.sent); !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("cross-device dismissal result = %s", result.Body)
	}
	entry, err := promptStore.Entry(context.Background(), "unknown-other")
	if err != nil || entry.State != promptqueue.StateSentUnknown {
		t.Fatalf("other device entry changed = %#v, %v", entry, err)
	}
	if _, err := attachmentStore.ResolveClaimed("other-phone", []string{"upload-other"}, "unknown-other", sessionNow); err != nil {
		t.Fatalf("other device claim was released: %v", err)
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

func TestColdHelloAdvertisesHostProvidedNewTaskOptions(t *testing.T) {
	source := taskOptionsSource{
		catalog: taskoptions.Catalog{
			Models: []taskoptions.Model{{
				ID: "codex-1", DisplayName: "Codex 1", Default: true, DefaultReasoningID: "medium",
				Reasoning: []taskoptions.Reasoning{{ID: "medium", DisplayName: "Medium", Description: "Balances speed and depth."}},
			}},
			PermissionModes: []taskoptions.PermissionMode{{ID: "workspace-write", DisplayName: "Workspace", Description: "Can change the selected project.", Default: true}},
		},
	}
	handler, sender := newTestHandlerWithTasks(t, source)

	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-options","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	var welcome struct {
		Capabilities   []string            `json:"capabilities"`
		NewTaskOptions taskoptions.Catalog `json:"newTaskOptions"`
	}
	if err := json.Unmarshal(sender.messages[0].Body, &welcome); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(welcome.Capabilities, "new_task_options") {
		t.Fatalf("capabilities = %#v", welcome.Capabilities)
	}
	if len(welcome.NewTaskOptions.Models) != 1 || welcome.NewTaskOptions.Models[0].ID != "codex-1" ||
		welcome.NewTaskOptions.Models[0].DefaultReasoningID != "medium" || len(welcome.NewTaskOptions.PermissionModes) != 1 {
		t.Fatalf("new task options = %#v", welcome.NewTaskOptions)
	}
	if bytes.Contains(sender.messages[0].Body, []byte(`"wireName"`)) {
		t.Fatalf("welcome exposed an internal wire name: %s", sender.messages[0].Body)
	}
}

func TestColdHelloKeepsExistingFeaturesWhenNewTaskOptionsAreUnavailable(t *testing.T) {
	source := taskOptionsSource{optionErr: errors.New("private model catalog failure")}
	handler, sender := newTestHandlerWithTasks(t, source)

	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-options-fail","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 2 || sender.messages[0].Type != "welcome" || sender.messages[1].Type != "snapshot" {
		t.Fatalf("outputs = %#v", sender.messages)
	}
	var welcome struct {
		Capabilities   []string            `json:"capabilities"`
		NewTaskOptions taskoptions.Catalog `json:"newTaskOptions"`
	}
	if err := json.Unmarshal(sender.messages[0].Body, &welcome); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(welcome.Capabilities, "new_task_options") || len(welcome.NewTaskOptions.Models) != 0 || len(welcome.NewTaskOptions.PermissionModes) != 0 {
		t.Fatalf("welcome = %#v", welcome)
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

func TestNewTaskActionReloadsOptionsResolvesProjectAndUsesDurableQueue(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog()}
	promptStore := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptStore)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"start-new","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	snapshot := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) || snapshot.Type != "snapshot" {
		t.Fatalf("result = %#v, snapshot = %#v", result, snapshot)
	}
	if source.optionCalls != 2 || len(source.starts) != 1 {
		t.Fatalf("option calls = %d, starts = %#v", source.optionCalls, source.starts)
	}
	started := source.starts[0]
	if started.ProjectPath != sender.projectPath || started.Model != "private-wire-model" || started.Effort != "high" || started.Sandbox != appserver.SandboxWorkspaceWrite || string(started.ApprovalPolicy) != `"on-request"` {
		t.Fatalf("resolved request = %#v", started)
	}
	stored, err := promptStore.Entry(context.Background(), "action-1")
	if err != nil || stored.State != promptqueue.StateConfirmed || stored.Prompt != "" || stored.Result.ThreadID != "thread-created" {
		t.Fatalf("durable queue entry = %#v, %v", stored, err)
	}
}

func TestConfirmedNewTaskAppearsWorkingBeforeTheSharedCatalogCatchesUp(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog(), delayStartedTaskCatalog: true}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"start-new","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	_ = awaitSentMessage(t, sender.sent)
	snapshot := awaitSentMessage(t, sender.sent)
	var body struct {
		Tasks []snapshotTask `json:"tasks"`
	}
	if err := json.Unmarshal(snapshot.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tasks) != 1 || body.Tasks[0].TaskID != "thread-created" || body.Tasks[0].Title != "Fix it" || body.Tasks[0].State != string(taskstate.Working) || body.Tasks[0].ActiveTurnID != "turn-created" || !body.Tasks[0].CanRedirect {
		t.Fatalf("provisional new task = %#v", body.Tasks)
	}
}

func TestProvisionalNewTaskPromotesAStaleCatalogEntryToWorking(t *testing.T) {
	tasks := []snapshotTask{{TaskID: "thread-created", Title: "Catalog title", ProjectLabel: "Main", State: string(taskstate.IdleAfterReply), QueueState: string(promptqueue.QueueEmpty)}}
	provisional := snapshotTask{TaskID: "thread-created", Title: "Prompt title", ProjectLabel: "Main", State: string(taskstate.Working), ActiveTurnID: "turn-created", CanRedirect: true, QueueState: string(promptqueue.QueueEmpty), LastActivityAt: sessionNow.Format(time.RFC3339)}

	merged := mergeProvisionalTask(tasks, provisional)

	if len(merged) != 1 || merged[0].Title != "Catalog title" || merged[0].State != string(taskstate.Working) || merged[0].ActiveTurnID != "turn-created" || !merged[0].CanRedirect {
		t.Fatalf("merged provisional task = %#v", merged)
	}
}

func TestNewTaskDuplicateActionIDMustMatchTheOriginalRequest(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog()}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 4)
	first := decode(t, `{"version":{"major":1,"minor":0},"messageId":"first","sender":"phone","type":"action","body":{"actionId":"same-action","kind":"start_turn","projectId":"main","text":"First prompt","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)
	if err := handler.Handle(context.Background(), sender, first); err != nil {
		t.Fatal(err)
	}
	_ = awaitSentMessage(t, sender.sent)
	_ = awaitSentMessage(t, sender.sent)
	changed := decode(t, `{"version":{"major":1,"minor":0},"messageId":"changed","sender":"phone","type":"action","body":{"actionId":"same-action","kind":"start_turn","projectId":"main","text":"Different prompt","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)
	if err := handler.Handle(context.Background(), sender, changed); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) || !bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) || len(source.starts) != 1 {
		t.Fatalf("duplicate result = %s, starts = %d", result.Body, len(source.starts))
	}
}

func TestConfirmedNewTaskDuplicateReplaysBeforeAChangedCatalogIsRevalidated(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog()}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 4)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"first","sender":"phone","type":"action","body":{"actionId":"stable-action","kind":"start_turn","projectId":"main","text":"Same prompt","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)
	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	_ = awaitSentMessage(t, sender.sent)
	_ = awaitSentMessage(t, sender.sent)
	source.catalog.Models[0].WireName = "changed-private-wire-name"
	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	replayed := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(replayed.Body, []byte(`"state":"confirmed"`)) || len(source.starts) != 1 {
		t.Fatalf("replayed result = %s, starts = %d", replayed.Body, len(source.starts))
	}
}

func TestNewTaskDefiniteFailureReplaysAsFailedWithoutKeepingPrompt(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog(), startErr: errors.New("definite local rejection")}
	store := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"failed","sender":"phone","type":"action","body":{"actionId":"failed-action","kind":"start_turn","projectId":"main","text":"Private prompt","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)
	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	first := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(first.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("first result = %s", first.Body)
	}
	stored, err := store.Entry(context.Background(), "failed-action")
	if err != nil || stored.State != promptqueue.StateFailed || stored.Prompt != "" {
		t.Fatalf("failed stored entry = %#v, error = %v", stored, err)
	}
	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	replayed := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(replayed.Body, []byte(`"state":"failed"`)) || len(source.starts) != 1 {
		t.Fatalf("replayed result = %s, starts = %d", replayed.Body, len(source.starts))
	}
}

func TestNewTaskRejectsStaleHostOptionsBeforeCodexWrite(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog(), changeCatalogAfterHello: true}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 1)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"start-new","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) || !bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) || len(source.starts) != 0 {
		t.Fatalf("result = %s, starts = %#v", result.Body, source.starts)
	}
}

func TestNewTaskOptionResolutionFailsClosedOnAnUnmappedPermissionMode(t *testing.T) {
	catalog := testTaskOptionsCatalog()
	catalog.PermissionModes = append(catalog.PermissionModes, taskoptions.PermissionMode{ID: "future-write", DisplayName: "Future", Description: "Unknown"})
	_, _, ok := resolveNewTaskOptions(catalog, "public-model", "high", "future-write")
	if ok {
		t.Fatal("unmapped permission mode was accepted")
	}
}

func TestExistingIdleSendStartsImmediatelyAndBusySendQueuesDurably(t *testing.T) {
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}}
	store := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 4)
	idle := decode(t, `{"version":{"major":1,"minor":0},"messageId":"idle","sender":"phone","type":"action","body":{"actionId":"idle-action","kind":"start_turn","taskId":"thread-1","text":"Continue"}}`)
	if err := handler.Handle(context.Background(), sender, idle); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) || !bytes.Contains(result.Body, []byte(`"resultCode":"accepted"`)) || !slices.Equal(source.calls, []string{"start:Continue"}) {
		t.Fatalf("idle result = %s, calls = %#v", result.Body, source.calls)
	}
	_ = awaitSentMessage(t, sender.sent)

	source.task.State = taskstate.Working
	source.task.ActiveTurnID = "turn-active"
	source.task.CanRedirect = true
	busy := decode(t, `{"version":{"major":1,"minor":0},"messageId":"busy","sender":"phone","type":"action","body":{"actionId":"busy-action","kind":"start_turn","taskId":"thread-1","text":"After that"}}`)
	if err := handler.Handle(context.Background(), sender, busy); err != nil {
		t.Fatal(err)
	}
	queued := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(queued.Body, []byte(`"state":"confirmed"`)) || !bytes.Contains(queued.Body, []byte(`"resultCode":"queued"`)) || len(source.calls) != 1 {
		t.Fatalf("busy result = %s, calls = %#v", queued.Body, source.calls)
	}
	entry, err := store.Entry(context.Background(), "busy-action")
	if err != nil || entry.State != promptqueue.StatePrepared || entry.Prompt != "After that" {
		t.Fatalf("busy queue entry = %#v, %v", entry, err)
	}
	snapshot, err := handler.refreshTaskSnapshot(context.Background())
	if err != nil || !bytes.Contains(snapshot.Body, []byte(`"queueState":"queued"`)) {
		t.Fatalf("queued snapshot = %s, %v", snapshot.Body, err)
	}
}

func TestDesktopExistingTaskUnknownWritesStayUnknownAndNeverReplay(t *testing.T) {
	for _, test := range []struct {
		name       string
		state      taskstate.State
		activeTurn string
		redirect   bool
		action     string
		wantCall   string
	}{
		{name: "start", state: taskstate.IdleAfterReply, action: `{"version":{"major":1,"minor":0},"messageId":"start","sender":"phone","type":"action","body":{"actionId":"unknown-start","kind":"start_turn","taskId":"thread-1","text":"Continue"}}`, wantCall: "start:Continue"},
		{name: "redirect", state: taskstate.Working, activeTurn: "turn-1", redirect: true, action: `{"version":{"major":1,"minor":0},"messageId":"redirect","sender":"phone","type":"action","body":{"actionId":"unknown-redirect","kind":"steer_turn","taskId":"thread-1","text":"Change course"}}`, wantCall: "redirect:Change course"},
		{name: "stop", state: taskstate.Working, activeTurn: "turn-1", action: `{"version":{"major":1,"minor":0},"messageId":"stop","sender":"phone","type":"action","body":{"actionId":"unknown-stop","kind":"interrupt_turn","taskId":"thread-1"}}`, wantCall: "stop"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &existingTaskSource{
				task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: test.state, ActiveTurnID: test.activeTurn, CanRedirect: test.redirect, UpdatedAtUnix: sessionNow.Unix()},
				err:  fmt.Errorf("desktop response lost: %w", desktopipc.ErrWriteOutcomeUnknown),
			}
			store := promptqueue.NewMemoryStore()
			handler, sender := newTestHandlerWithTaskQueue(t, source, store)
			if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
				t.Fatal(err)
			}
			sender.messages = nil
			sender.sent = make(chan contract.Message, 4)
			action := decode(t, test.action)
			for attempt := 0; attempt < 2; attempt++ {
				if err := handler.Handle(context.Background(), sender, action); err != nil {
					t.Fatal(err)
				}
				result := awaitSentMessage(t, sender.sent)
				if !bytes.Contains(result.Body, []byte(`"state":"outcome_unknown"`)) {
					t.Fatalf("attempt %d result = %s", attempt, result.Body)
				}
			}
			if !slices.Equal(source.calls, []string{test.wantCall}) {
				t.Fatalf("calls = %#v", source.calls)
			}
			var body struct {
				ActionID string `json:"actionId"`
			}
			if json.Unmarshal(action.Body, &body) != nil {
				t.Fatal("decode action body")
			}
			entry, err := store.Entry(context.Background(), body.ActionID)
			if err != nil || entry.State != promptqueue.StateSentUnknown {
				t.Fatalf("durable entry = %#v, %v", entry, err)
			}
		})
	}
}

func TestRedirectUsesCurrentCapabilityAndFallsBackToQueueIfItChanged(t *testing.T) {
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-active", CanRedirect: true, UpdatedAtUnix: sessionNow.Unix()}}
	store := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 4)
	redirect := decode(t, `{"version":{"major":1,"minor":0},"messageId":"redirect","sender":"phone","type":"action","body":{"actionId":"redirect-action","kind":"steer_turn","taskId":"thread-1","text":"Do this first"}}`)
	if err := handler.Handle(context.Background(), sender, redirect); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"resultCode":"redirected"`)) || !slices.Equal(source.calls, []string{"redirect:Do this first"}) {
		t.Fatalf("redirect result = %s, calls = %#v", result.Body, source.calls)
	}
	_ = awaitSentMessage(t, sender.sent)

	source.task.CanRedirect = false
	fallback := decode(t, `{"version":{"major":1,"minor":0},"messageId":"fallback","sender":"phone","type":"action","body":{"actionId":"fallback-action","kind":"steer_turn","taskId":"thread-1","text":"Queue this"}}`)
	if err := handler.Handle(context.Background(), sender, fallback); err != nil {
		t.Fatal(err)
	}
	queued := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(queued.Body, []byte(`"resultCode":"queued"`)) || len(source.calls) != 1 {
		t.Fatalf("fallback result = %s, calls = %#v", queued.Body, source.calls)
	}
	entry, err := store.Entry(context.Background(), "fallback-action")
	if err != nil || entry.State != promptqueue.StatePrepared {
		t.Fatalf("fallback queue entry = %#v, %v", entry, err)
	}
}

func TestStopReloadsTaskAndReturnsInterruptedOnlyAfterHostConfirmation(t *testing.T) {
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-active", CanRedirect: true, UpdatedAtUnix: sessionNow.Unix()}}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	stop := decode(t, `{"version":{"major":1,"minor":0},"messageId":"stop","sender":"phone","type":"action","body":{"actionId":"stop-action","kind":"interrupt_turn","taskId":"thread-1"}}`)
	if err := handler.Handle(context.Background(), sender, stop); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"resultCode":"interrupted"`)) || !slices.Equal(source.calls, []string{"stop"}) {
		t.Fatalf("stop result = %s, calls = %#v", result.Body, source.calls)
	}
	_ = awaitSentMessage(t, sender.sent)
	if err := handler.Handle(context.Background(), sender, stop); err != nil {
		t.Fatal(err)
	}
	replayed := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(replayed.Body, []byte(`"resultCode":"interrupted"`)) || !slices.Equal(source.calls, []string{"stop"}) {
		t.Fatalf("duplicate stop result = %s, calls = %#v", replayed.Body, source.calls)
	}
}

func TestExistingTaskErrorCodeUsesOnlyStableSafeCategories(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		idMatches bool
		want      string
	}{
		{name: "matching", idMatches: true, want: "none"},
		{name: "mismatched", want: "task_id_mismatch"},
		{name: "busy", err: taskadapter.ErrTaskBusy, idMatches: true, want: "task_busy"},
		{name: "unavailable", err: taskadapter.ErrTaskUnavailable, idMatches: true, want: "task_unavailable"},
		{name: "invalid app server input", err: appserver.ErrInvalidInput, idMatches: true, want: "app_server_invalid_input"},
		{name: "unknown", err: errors.New("private remote detail"), idMatches: true, want: "unclassified"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := existingTaskErrorCode(test.err, test.idMatches); got != test.want {
				t.Fatalf("existingTaskErrorCode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUnknownStopOutcomeReplaysWithoutBlindlyInterruptingAgain(t *testing.T) {
	source := &existingTaskSource{
		task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-active", CanRedirect: true, UpdatedAtUnix: sessionNow.Unix()},
		err:  fmt.Errorf("lost response: %w", &appserver.OutcomeUnknownError{Method: "turn/interrupt", Cause: errors.New("connection lost")}),
	}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	stop := decode(t, `{"version":{"major":1,"minor":0},"messageId":"stop","sender":"phone","type":"action","body":{"actionId":"stop-unknown","kind":"interrupt_turn","taskId":"thread-1"}}`)
	for attempt := 0; attempt < 2; attempt++ {
		if err := handler.Handle(context.Background(), sender, stop); err != nil {
			t.Fatal(err)
		}
		result := awaitSentMessage(t, sender.sent)
		if !bytes.Contains(result.Body, []byte(`"state":"outcome_unknown"`)) {
			t.Fatalf("unknown stop result = %s", result.Body)
		}
	}
	if !slices.Equal(source.calls, []string{"stop"}) {
		t.Fatalf("unknown stop calls = %#v", source.calls)
	}
}

func TestReplyEventStartsOnlyTheOldestDurableQueuedFollowUp(t *testing.T) {
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-active", CanRedirect: true, UpdatedAtUnix: sessionNow.Unix()}}
	store := promptqueue.NewMemoryStore()
	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 8)
	for _, action := range []string{
		`{"version":{"major":1,"minor":0},"messageId":"first","sender":"phone","type":"action","body":{"actionId":"first-action","kind":"start_turn","taskId":"thread-1","text":"First"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"second","sender":"phone","type":"action","body":{"actionId":"second-action","kind":"start_turn","taskId":"thread-1","text":"Second"}}`,
	} {
		if err := handler.Handle(context.Background(), sender, decode(t, action)); err != nil {
			t.Fatal(err)
		}
		_ = awaitSentMessage(t, sender.sent)
	}
	source.task.State = taskstate.IdleAfterReply
	source.task.ActiveTurnID = ""
	source.task.CanRedirect = false
	if err := handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{TaskID: "thread-1", Kind: "reply", State: taskstate.IdleAfterReply, Summary: "Codex replied"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(source.calls, []string{"start:First"}) {
		t.Fatalf("queue dispatch calls = %#v", source.calls)
	}
	first, _ := store.Entry(context.Background(), "first-action")
	second, _ := store.Entry(context.Background(), "second-action")
	if first.State != promptqueue.StateConfirmed || second.State != promptqueue.StatePrepared {
		t.Fatalf("queue order states = first %s, second %s", first.State, second.State)
	}
}

func TestCompanionRestartDispatchesAnAlreadyQueuedPromptWhenTaskIsIdle(t *testing.T) {
	started := make(chan struct{}, 1)
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}, started: started}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "queued-before-restart", QueueKey: "thread-1", ActionKind: "start_turn", ThreadID: "thread-1", Prompt: "Resume me", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = newTestHandlerWithTaskQueue(t, source, store)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("restart recovery did not dispatch")
	}
	if !slices.Equal(source.calls, []string{"start:Resume me"}) {
		t.Fatalf("restart recovery calls = %#v", source.calls)
	}
	entry, err := store.Entry(context.Background(), "queued-before-restart")
	if err != nil || entry.State != promptqueue.StateConfirmed {
		t.Fatalf("recovered queue entry = %#v, %v", entry, err)
	}
}

func TestCompanionRestartRecoversQueuedTaskOutsideTheHomeSnapshot(t *testing.T) {
	started := make(chan struct{}, 1)
	tasks := make([]taskstate.Task, 0, 5)
	for index := 1; index <= 5; index++ {
		tasks = append(tasks, taskstate.Task{ID: fmt.Sprintf("thread-%d", index), Title: "Task", ProjectLabel: "Main", State: taskstate.IdleAfterReply, Source: taskstate.SourceAppServer, UpdatedAtUnix: sessionNow.Unix() - int64(index)})
	}
	source := &recoveryTaskSource{tasks: tasks, started: started, currentErr: taskadapter.ErrUnknownCatalogTask, requireSourceStart: true}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "queued-fifth", QueueKey: "thread-5", OwnerSource: string(taskstate.SourceAppServer), ThreadID: "thread-5", Prompt: "Resume hidden task", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = newTestHandlerWithTaskQueue(t, source, store)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("queued task outside Home was not recovered")
	}
	if !slices.Equal(source.calls, []string{"start:thread-5:Resume hidden task"}) {
		t.Fatalf("recovery calls = %#v", source.calls)
	}
}

func TestCompanionRestartCancelsPromptForTaskRemovedWhileStopped(t *testing.T) {
	source := &recoveryTaskSource{}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "queued-deleted", QueueKey: "thread-deleted", OwnerSource: string(taskstate.SourceAppServer), ThreadID: "thread-deleted", Prompt: "Private prompt", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = queue.DispatchNext(context.Background(), "thread-deleted", func(context.Context, promptqueue.Entry) (promptqueue.Result, error) {
		return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
	}, nil, sessionNow.Add(time.Second))
	_, _ = newTestHandlerWithTaskQueue(t, source, store)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		entry, err := store.Entry(context.Background(), "queued-deleted")
		if err == nil && entry.State == promptqueue.StateCanceled {
			if entry.Prompt != "" || entry.ErrorCode != "task_unavailable" {
				t.Fatalf("cancelled entry retained content = %#v", entry)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("removed task queue was not cancelled")
}

func TestCompanionRestartRetainsQueueDuringTransientTaskLookupFailure(t *testing.T) {
	source := &recoveryTaskSource{sourceErr: context.DeadlineExceeded}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "queued-transient", QueueKey: "thread-transient", OwnerSource: string(taskstate.SourceAppServer), ThreadID: "thread-transient", Prompt: "Keep me", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = newTestHandlerWithTaskQueue(t, source, store)
	time.Sleep(20 * time.Millisecond)
	entry, err := store.Entry(context.Background(), "queued-transient")
	if err != nil || entry.State != promptqueue.StatePrepared || entry.Prompt != "Keep me" {
		t.Fatalf("queue after transient lookup = %#v, %v", entry, err)
	}
}

func TestCompanionRestartRetainsQueueWhenSecondSafetyReadFailsBeforeWrite(t *testing.T) {
	source := &recoveryTaskSource{
		tasks:          []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.IdleAfterReply, Source: taskstate.SourceAppServer, UpdatedAtUnix: sessionNow.Unix()}},
		sourceStartErr: taskadapter.ErrTaskLookupTransient,
	}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "queued-second-read", QueueKey: "thread-1", OwnerSource: string(taskstate.SourceAppServer), ThreadID: "thread-1", Prompt: "Keep my position", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = newTestHandlerWithTaskQueue(t, source, store)
	time.Sleep(20 * time.Millisecond)
	entry, err := store.Entry(context.Background(), "queued-second-read")
	if err != nil || entry.State != promptqueue.StatePrepared || entry.Prompt != "Keep my position" || len(source.calls) != 0 {
		t.Fatalf("queue after second lookup failure = %#v, error = %v, calls = %#v", entry, err, source.calls)
	}
}

func TestArchivingTaskCancelsEveryQueuedPromptAndClearsItsText(t *testing.T) {
	source := &taskManagementSource{tasks: []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-1", UpdatedAtUnix: sessionNow.Unix()}}}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	attachmentStore := openAttachmentStore(t)
	completeAttachment(t, attachmentStore, "pixel-9", "upload-archive", []byte("archive me"))
	if err := attachmentStore.Claim("pixel-9", []string{"upload-archive"}, "queued-0", sessionNow); err != nil {
		t.Fatal(err)
	}
	for index, prompt := range []string{"Private first", "Private second"} {
		entry := promptqueue.Entry{
			ActionID: fmt.Sprintf("queued-%d", index), QueueKey: "thread-1", ThreadID: "thread-1", Prompt: prompt, CreatedAt: sessionNow.Add(time.Duration(index) * time.Second),
		}
		if index == 0 {
			entry.DeviceID = "pixel-9"
			entry.AttachmentIDs = []string{"upload-archive"}
		}
		if err := queue.Enqueue(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	handler.EnableAttachments(attachmentStore)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 3)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"archive-1","kind":"archive_task","taskId":"thread-1"}}`)); err != nil {
		t.Fatal(err)
	}
	_ = awaitSentMessage(t, sender.sent)
	for index := range 2 {
		entry, err := store.Entry(context.Background(), fmt.Sprintf("queued-%d", index))
		if err != nil || entry.State != promptqueue.StateCanceled || entry.Prompt != "" || entry.ErrorCode != "task_archived" {
			t.Fatalf("archived queue entry %d = %#v, %v", index, entry, err)
		}
	}
	if _, err := attachmentStore.ResolveClaimed("pixel-9", []string{"upload-archive"}, "queued-0", sessionNow); !errors.Is(err, attachments.ErrAttachmentUnavailable) {
		t.Fatalf("archived queue retained claimed attachment: %v", err)
	}
}

func TestArchiveIsBlockedWhileAnUnknownControlStillNeedsReview(t *testing.T) {
	source := &taskManagementSource{tasks: []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-1", UpdatedAtUnix: sessionNow.Unix()}}}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "unknown-before-archive", QueueKey: "thread-1", ThreadID: "thread-1", Prompt: "Private prompt", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = queue.DispatchNext(context.Background(), "thread-1", func(context.Context, promptqueue.Entry) (promptqueue.Result, error) {
		return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
	}, nil, sessionNow.Add(time.Second))
	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"archive-unknown","kind":"archive_task","taskId":"thread-1"}}`)); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) || slices.Contains(source.calls, "archive:thread-1") {
		t.Fatalf("archive result = %s, calls = %#v", result.Body, source.calls)
	}
	entry, err := store.Entry(context.Background(), "unknown-before-archive")
	if err != nil || entry.State != promptqueue.StateSentUnknown || entry.Prompt != "Private prompt" {
		t.Fatalf("unknown entry after blocked archive = %#v, %v", entry, err)
	}
}

func TestExplicitComputerReviewClearsCompanionUnknownWithoutRetryingCodex(t *testing.T) {
	source := &existingTaskSource{task: taskstate.Task{ID: "thread-1", Title: "Task", ProjectLabel: "Main", State: taskstate.Working, ActiveTurnID: "turn-1", UpdatedAtUnix: sessionNow.Unix()}}
	store := promptqueue.NewMemoryStore()
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "unknown-control", QueueKey: "thread-1", ThreadID: "thread-1", Prompt: "Private prompt", CreatedAt: sessionNow,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = queue.DispatchNext(context.Background(), "thread-1", func(context.Context, promptqueue.Entry) (promptqueue.Result, error) {
		return promptqueue.Result{}, promptqueue.ErrSendOutcomeUnknown
	}, nil, sessionNow.Add(time.Second))

	handler, sender := newTestHandlerWithTaskQueue(t, source, store)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 3)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"dismiss","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control","taskId":"thread-1","targetActionId":"unknown-control"}}`)
	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) || !bytes.Contains(result.Body, []byte(`"resultCode":"accepted"`)) {
		t.Fatalf("dismiss result = %s", result.Body)
	}
	entry, err := store.Entry(context.Background(), "unknown-control")
	if err != nil || entry.State != promptqueue.StateCanceled || entry.Prompt != "" || len(source.calls) != 0 {
		t.Fatalf("dismissed entry = %#v, error = %v, Codex calls = %#v", entry, err, source.calls)
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

func TestConfirmedForkReturnsTheNewTaskID(t *testing.T) {
	source := &taskManagementSource{
		tasks: []taskstate.Task{{ID: "thread-1", Title: "Original", ProjectLabel: "Main", State: taskstate.IdleAfterReply, UpdatedAtUnix: sessionNow.Unix()}},
	}
	handler, sender := newTestHandlerWithTasks(t, source)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-fork-id","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	action := decode(t, `{"version":{"major":1,"minor":0},"messageId":"fork-id","sender":"phone","type":"action","body":{"actionId":"action-fork-id","kind":"fork_task","taskId":"thread-1"}}`)

	if err := handler.Handle(context.Background(), sender, action); err != nil {
		t.Fatal(err)
	}
	result := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) || !bytes.Contains(result.Body, []byte(`"forkTaskId":"fork-1"`)) {
		t.Fatalf("fork result = %s", result.Body)
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

type recordingDecisionResponder struct {
	responses []decisions.Response
	err       error
}

func (responder *recordingDecisionResponder) Respond(_ context.Context, response decisions.Response) error {
	responder.responses = append(responder.responses, response)
	return responder.err
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

func newTestHandlerWithTaskQueue(t *testing.T, taskSource TaskSource, promptStore promptqueue.Store) (*Handler, *recordingSender) {
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
	handler, err := NewWithTaskSourceAndQueue(
		context.Background(), "Studio Mac", projectService, journal, taskSource, promptqueue.New(promptStore, nil), nil, func() time.Time { return sessionNow },
	)
	if err != nil {
		t.Fatal(err)
	}
	return handler, &recordingSender{deviceID: "pixel-9", sessionID: "session-1", connectionID: 1, projectPath: root, store: store}
}

type taskSourceFunc func(context.Context, int) ([]taskstate.Task, error)

func (source taskSourceFunc) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	return source(ctx, limit)
}

type taskOptionsSource struct {
	catalog   taskoptions.Catalog
	optionErr error
}

type newTaskSource struct {
	catalog                 taskoptions.Catalog
	changeCatalogAfterHello bool
	optionCalls             int
	starts                  []taskadapter.NewTaskRequest
	delayStartedTaskCatalog bool
	tasks                   []taskstate.Task
	startErr                error
}

type existingTaskSource struct {
	task        taskstate.Task
	calls       []string
	attachments []taskadapter.AttachmentInput
	err         error
	started     chan struct{}
}

type recoveryTaskSource struct {
	tasks              []taskstate.Task
	calls              []string
	started            chan struct{}
	currentErr         error
	sourceErr          error
	requireSourceStart bool
	sourceStartErr     error
}

func (source *recoveryTaskSource) ListRecent(_ context.Context, limit int) ([]taskstate.Task, error) {
	if limit > len(source.tasks) {
		limit = len(source.tasks)
	}
	return append([]taskstate.Task(nil), source.tasks[:limit]...), nil
}

func (source *recoveryTaskSource) CurrentTask(_ context.Context, taskID string) (taskstate.Task, error) {
	if source.currentErr != nil {
		return taskstate.Task{}, source.currentErr
	}
	for _, task := range source.tasks {
		if task.ID == taskID {
			return task, nil
		}
	}
	return taskstate.Task{}, taskadapter.ErrTaskUnavailable
}

func (source *recoveryTaskSource) CurrentTaskFromSource(_ context.Context, taskID string, owner taskstate.Source) (taskstate.Task, error) {
	if source.sourceErr != nil {
		return taskstate.Task{}, source.sourceErr
	}
	for _, task := range source.tasks {
		if task.ID == taskID && task.Source == owner {
			return task, nil
		}
	}
	return taskstate.Task{}, taskadapter.ErrTaskUnavailable
}

func (source *recoveryTaskSource) StartExistingTurn(_ context.Context, taskID, text string) (taskadapter.ExistingTaskResult, error) {
	if source.requireSourceStart {
		return taskadapter.ExistingTaskResult{}, taskadapter.ErrUnknownCatalogTask
	}
	return source.start(taskID, text)
}

func (source *recoveryTaskSource) StartExistingTurnFromSource(_ context.Context, taskID, text string, owner taskstate.Source) (taskadapter.ExistingTaskResult, error) {
	if source.sourceStartErr != nil {
		return taskadapter.ExistingTaskResult{}, source.sourceStartErr
	}
	if owner != taskstate.SourceAppServer {
		return taskadapter.ExistingTaskResult{}, taskstate.ErrAdapterSourceMismatch
	}
	return source.start(taskID, text)
}

func (source *recoveryTaskSource) start(taskID, text string) (taskadapter.ExistingTaskResult, error) {
	source.calls = append(source.calls, "start:"+taskID+":"+text)
	if source.started != nil {
		select {
		case source.started <- struct{}{}:
		default:
		}
	}
	return taskadapter.ExistingTaskResult{ThreadID: taskID, TurnID: "turn-started"}, nil
}

func (*recoveryTaskSource) RedirectExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{}, errors.New("not used")
}

func (*recoveryTaskSource) InterruptExistingTurn(context.Context, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{}, errors.New("not used")
}

func (source *existingTaskSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	return []taskstate.Task{source.task}, nil
}

func (source *existingTaskSource) CurrentTask(context.Context, string) (taskstate.Task, error) {
	return source.task, nil
}

func (source *existingTaskSource) StartExistingTurn(_ context.Context, _ string, text string) (taskadapter.ExistingTaskResult, error) {
	source.calls = append(source.calls, "start:"+text)
	if source.started != nil {
		select {
		case source.started <- struct{}{}:
		default:
		}
	}
	if source.err != nil {
		return taskadapter.ExistingTaskResult{}, source.err
	}
	source.task.State = taskstate.Working
	source.task.ActiveTurnID = "turn-started"
	source.task.CanRedirect = true
	return taskadapter.ExistingTaskResult{ThreadID: source.task.ID, TurnID: "turn-started"}, nil
}

func (source *existingTaskSource) StartExistingTurnWithAttachments(ctx context.Context, taskID, text string, values []taskadapter.AttachmentInput) (taskadapter.ExistingTaskResult, error) {
	source.attachments = append([]taskadapter.AttachmentInput(nil), values...)
	return source.StartExistingTurn(ctx, taskID, text)
}

func (source *existingTaskSource) RedirectExistingTurn(_ context.Context, _ string, text string) (taskadapter.ExistingTaskResult, error) {
	source.calls = append(source.calls, "redirect:"+text)
	if source.err != nil {
		return taskadapter.ExistingTaskResult{}, source.err
	}
	return taskadapter.ExistingTaskResult{ThreadID: source.task.ID, TurnID: source.task.ActiveTurnID}, nil
}

func (source *existingTaskSource) RedirectExistingTurnWithAttachments(ctx context.Context, taskID, text string, values []taskadapter.AttachmentInput) (taskadapter.ExistingTaskResult, error) {
	source.attachments = append([]taskadapter.AttachmentInput(nil), values...)
	return source.RedirectExistingTurn(ctx, taskID, text)
}

func (source *existingTaskSource) InterruptExistingTurn(context.Context, string) (taskadapter.ExistingTaskResult, error) {
	source.calls = append(source.calls, "stop")
	turnID := source.task.ActiveTurnID
	if source.err == nil {
		source.task.State = taskstate.Interrupted
		source.task.ActiveTurnID = ""
		source.task.CanRedirect = false
	}
	return taskadapter.ExistingTaskResult{ThreadID: source.task.ID, TurnID: turnID}, source.err
}

func (source *newTaskSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	return append([]taskstate.Task(nil), source.tasks...), nil
}

func (source *newTaskSource) NewTaskOptions(context.Context) (taskoptions.Catalog, error) {
	source.optionCalls++
	if source.changeCatalogAfterHello && source.optionCalls > 1 {
		changed := testTaskOptionsCatalog()
		changed.Models[0].ID = "replacement-model"
		return changed, nil
	}
	return source.catalog, nil
}

func (source *newTaskSource) StartNewTask(_ context.Context, request taskadapter.NewTaskRequest) (taskadapter.NewTaskResult, error) {
	source.starts = append(source.starts, request)
	if source.startErr != nil {
		return taskadapter.NewTaskResult{}, source.startErr
	}
	if !source.delayStartedTaskCatalog {
		source.tasks = append(source.tasks, taskstate.Task{
			ID: "thread-created", Title: "Fix it", ProjectLabel: "Main", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix(),
		})
	}
	return taskadapter.NewTaskResult{ThreadID: "thread-created", TurnID: "turn-created"}, nil
}

func testTaskOptionsCatalog() taskoptions.Catalog {
	return taskoptions.Catalog{
		Models: []taskoptions.Model{{
			ID: "public-model", WireName: "private-wire-model", DisplayName: "Model", Default: true, DefaultReasoningID: "medium",
			Reasoning: []taskoptions.Reasoning{{ID: "medium", DisplayName: "Medium", Description: "Balanced"}, {ID: "high", DisplayName: "High", Description: "More reasoning"}},
		}},
		PermissionModes: []taskoptions.PermissionMode{
			{ID: "read-only", DisplayName: "Read only", Description: "Read", Default: false},
			{ID: "workspace-write", DisplayName: "Workspace", Description: "Write", Default: true},
			{ID: "danger-full-access", DisplayName: "Full access", Description: "Full", Default: false},
		},
	}
}

func openAttachmentStore(t *testing.T) *attachments.Store {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := attachments.Open(root, attachments.DefaultLimits(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func completeAttachment(t *testing.T, store *attachments.Store, deviceID, uploadID string, payload []byte) {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	if _, err := store.Begin(deviceID, uploadID, int64(len(payload)), digest, sessionNow); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(deviceID, uploadID, 0, 0, true, payload, sessionNow); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(deviceID, uploadID, sessionNow); err != nil {
		t.Fatal(err)
	}
}

func (source taskOptionsSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	return nil, nil
}

func (source taskOptionsSource) NewTaskOptions(context.Context) (taskoptions.Catalog, error) {
	return source.catalog, source.optionErr
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
