package mobilesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

func TestPublishTaskEventCommitsSnapshotStateBeforeBroadcast(t *testing.T) {
	handler, sender := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{{ID: "thread-1", Title: "Build launcher", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Add(-1).Unix()}}, nil
	}))
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-live","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	baseSequence := *sender.messages[1].Sequence
	sender.messages = nil
	sender.sent = make(chan contract.Message, 1)

	if err := handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{TaskID: "thread-1", Kind: "reply", State: taskstate.IdleAfterReply, Summary: "Codex replied"}); err != nil {
		t.Fatal(err)
	}
	awaitSentMessage(t, sender.sent)
	if len(sender.messages) != 1 || sender.messages[0].Type != "event" || sender.messages[0].Sequence == nil || *sender.messages[0].Sequence != baseSequence+1 {
		t.Fatalf("broadcast = %#v", sender.messages)
	}
	var body struct {
		TaskID, Event, State, Summary string
	}
	if err := json.Unmarshal(sender.messages[0].Body, &body); err != nil || body.TaskID != "thread-1" || body.Event != "reply" || body.State != "idle_after_reply" || body.Summary != "Codex replied" {
		t.Fatalf("event body = %#v, %v", body, err)
	}
	snapshot, err := handler.journal.Snapshot(sessionNow)
	if err != nil {
		t.Fatal(err)
	}
	var state snapshotState
	if err := json.Unmarshal(snapshot.Body, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].Title != "Build launcher" || state.Tasks[0].State != "idle_after_reply" || state.Tasks[0].LastActivityAt != sessionNow.Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("journal snapshot = %#v", state.Tasks)
	}
}

func TestPublishTaskEventRejectsRevokedAuthorizationBeforeJournalCommit(t *testing.T) {
	handler, _ := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
	}))
	authorization := taskstate.NewEventAuthorization()
	authorization.Revoke()
	err := handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{
		TaskID: "thread-1", Kind: "reply", State: taskstate.IdleAfterReply, Summary: "Codex replied",
		Authorization: authorization,
	})
	if !errors.Is(err, ErrTaskEventAuthorizationRevoked) {
		t.Fatalf("revoked event error = %v", err)
	}
	snapshot, snapshotErr := handler.journal.Snapshot(sessionNow)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	var state snapshotState
	if json.Unmarshal(snapshot.Body, &state) != nil || len(state.Tasks) != 1 || state.Tasks[0].State != string(taskstate.Working) {
		t.Fatalf("revoked event changed journal snapshot: %#v", state.Tasks)
	}
}

func TestPublishTaskEventDoesNotWaitForBlockedPhone(t *testing.T) {
	handler, base := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
	}))
	handler.sendTimeout = 20 * time.Millisecond
	sender := &blockingLiveSender{recordingSender: *base, blocked: make(chan struct{}), closedSignal: make(chan struct{})}
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-blocked","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("live publication waited for a blocked phone")
	}
	select {
	case <-sender.blocked:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("background delivery did not reach blocked phone")
	}
	select {
	case <-sender.closedSignal:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocked phone was not disconnected after the write deadline")
	}
}

func TestRefreshTaskSnapshotBroadcastsNewTaskBeforeRetriedEvent(t *testing.T) {
	includeNew := false
	handler, sender := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		tasks := []taskstate.Task{{ID: "thread-1", Title: "Existing", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}
		if includeNew {
			tasks = append(tasks, taskstate.Task{ID: "thread-new", Title: "New task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()})
		}
		return tasks, nil
	}))
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-refresh","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
	sender.sent = make(chan contract.Message, 2)
	includeNew = true

	if err := handler.RefreshTaskSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{TaskID: "thread-new", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}); err != nil {
		t.Fatal(err)
	}
	first := awaitSentMessage(t, sender.sent)
	second := awaitSentMessage(t, sender.sent)
	if first.Type != "snapshot" || second.Type != "event" || first.Sequence == nil || second.Sequence == nil || *second.Sequence != *first.Sequence+1 {
		t.Fatalf("broadcast order = %#v then %#v", first, second)
	}
}

func TestActionRechecksActiveConnectionAtCommitBoundary(t *testing.T) {
	handler, old := newTestHandler(t)
	if err := handler.Handle(context.Background(), old, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-old","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	base := *old.messages[1].Sequence
	replacement := &recordingSender{deviceID: old.deviceID, sessionID: "session-new", connectionID: 2, projectPath: old.projectPath, store: old.store}
	handler.mu.Lock()
	handler.active[replacement.deviceID] = replacement
	handler.storeActiveViewLocked()
	handler.mu.Unlock()

	action := contract.Message{Version: contract.Version{Major: 1}, MessageID: "stale-action", Sender: "phone", Type: "action", Body: json.RawMessage(`{"actionId":"action-old","kind":"set_project","projectId":"main"}`)}
	if err := handler.handleAction(context.Background(), old, action); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("stale action error = %v", err)
	}
	bounds, err := old.store.Bounds(context.Background())
	if err != nil || bounds.Latest != base {
		t.Fatalf("journal bounds = %#v, %v; stale action committed", bounds, err)
	}
}

func TestWarmReplacementReplaysCommittedActionBeforeAnySnapshotReplacement(t *testing.T) {
	handler, base := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{}, nil
	}))
	handler.sendTimeout = 200 * time.Millisecond
	old := &blockingLiveSender{recordingSender: *base, blocked: make(chan struct{}), closedSignal: make(chan struct{})}
	if err := handler.Handle(context.Background(), old, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-old","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	baseSequence := *old.messages[1].Sequence
	action := contract.Message{Version: contract.Version{Major: 1}, MessageID: "action", Sender: "phone", Type: "action", Body: json.RawMessage(`{"actionId":"action-1","kind":"set_project","projectId":"main"}`)}
	if err := handler.Handle(context.Background(), old, action); err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.blocked:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("old connection did not block action-result delivery")
	}

	replacement := &recordingSender{deviceID: old.deviceID, sessionID: "session-new", connectionID: 2, projectPath: old.projectPath, store: old.store}
	hello := fmt.Sprintf(`{"version":{"major":1,"minor":0},"messageId":"hello-new","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"warm","lastAck":%d}}}`, baseSequence)
	if err := handler.Handle(context.Background(), replacement, decode(t, hello)); err != nil {
		t.Fatal(err)
	}
	if len(replacement.messages) != 2 || replacement.messages[0].Type != "welcome" || replacement.messages[1].Type != "action_result" || replacement.messages[1].Sequence == nil || *replacement.messages[1].Sequence != baseSequence+1 {
		t.Fatalf("replacement replay = %#v", replacement.messages)
	}
	bounds, err := replacement.store.Bounds(context.Background())
	if err != nil || bounds.Latest != baseSequence+1 {
		t.Fatalf("journal bounds after warm replay = %#v, %v", bounds, err)
	}
}

func TestPublishTaskEventRejectsUnknownTaskWithoutJournalOrBroadcast(t *testing.T) {
	handler, sender := newTestHandlerWithTasks(t, taskSourceFunc(func(context.Context, int) ([]taskstate.Task, error) {
		return []taskstate.Task{{ID: "thread-1", Title: "Task", ProjectLabel: "uf-u", State: taskstate.Working, UpdatedAtUnix: sessionNow.Unix()}}, nil
	}))
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-live","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	latest := *sender.messages[1].Sequence
	sender.messages = nil

	err := handler.PublishTaskEvent(context.Background(), taskstate.MobileEvent{TaskID: "thread-missing", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"})
	if !errors.Is(err, ErrUnknownTaskEvent) {
		t.Fatalf("unknown task error = %v", err)
	}
	bounds, boundsErr := sender.store.Bounds(context.Background())
	if boundsErr != nil || bounds.Latest != latest || len(sender.messages) != 0 {
		t.Fatalf("bounds=%#v messages=%#v error=%v", bounds, sender.messages, boundsErr)
	}
}

type blockingLiveSender struct {
	recordingSender
	calls        atomic.Int32
	blocked      chan struct{}
	closedSignal chan struct{}
	closeOnce    sync.Once
}

func (sender *blockingLiveSender) Send(ctx context.Context, message contract.Message) error {
	if sender.calls.Add(1) <= 2 {
		return sender.recordingSender.Send(ctx, message)
	}
	close(sender.blocked)
	<-ctx.Done()
	return ctx.Err()
}

func (sender *blockingLiveSender) Close() {
	sender.closeOnce.Do(func() { close(sender.closedSignal) })
}

func awaitSentMessage(t *testing.T, sent <-chan contract.Message) contract.Message {
	t.Helper()
	select {
	case message := <-sent:
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for mobile message")
		return contract.Message{}
	}
}
