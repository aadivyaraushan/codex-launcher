package taskstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMapSignalsCoversEveryLauncherState(t *testing.T) {
	tests := []struct {
		name    string
		signals Signals
		want    State
	}{
		{name: "working", signals: Signals{RuntimeStatus: "active", LastTurnStatus: "inProgress"}, want: Working},
		{name: "approval", signals: Signals{RuntimeStatus: "active", ActiveFlags: []string{"waitingOnApproval"}}, want: WaitingForApproval},
		{name: "answer", signals: Signals{RuntimeStatus: "active", ActiveFlags: []string{"waitingOnUserInput"}}, want: WaitingForAnswer},
		{name: "failed", signals: Signals{RuntimeStatus: "idle", LastTurnStatus: "failed"}, want: Failed},
		{name: "interrupted", signals: Signals{RuntimeStatus: "idle", LastTurnStatus: "interrupted"}, want: Interrupted},
		{name: "successful reply is idle", signals: Signals{RuntimeStatus: "idle", LastTurnStatus: "completed"}, want: IdleAfterReply},
		{name: "unloaded history is idle", signals: Signals{RuntimeStatus: "notLoaded", LastTurnStatus: "completed"}, want: IdleAfterReply},
		{name: "system error", signals: Signals{RuntimeStatus: "systemError"}, want: Failed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Map(test.signals); got != test.want {
				t.Fatalf("Map(%#v) = %q, want %q", test.signals, got, test.want)
			}
		})
	}
}

func TestTurnCompletedNeverInventsSemanticTaskCompletion(t *testing.T) {
	for _, status := range []string{"completed", "failed", "interrupted"} {
		got := Map(Signals{RuntimeStatus: "idle", LastTurnStatus: status})
		if got == State("completed") {
			t.Fatalf("turn status %q became semantic task completion", status)
		}
	}
}

func TestPendingRequestKindOverridesStaleRuntimeStatus(t *testing.T) {
	if got := Map(Signals{RuntimeStatus: "idle", PendingRequestKind: "command"}); got != WaitingForApproval {
		t.Fatalf("command request state = %q", got)
	}
	if got := Map(Signals{RuntimeStatus: "idle", PendingRequestKind: "question"}); got != WaitingForAnswer {
		t.Fatalf("question request state = %q", got)
	}
}

func TestUnknownItemsBecomeSafeGenericActivity(t *testing.T) {
	activity := MapItem(Item{Type: "futurePrivateItem", ID: "item-1", RawSummary: "secret payload"})
	if activity.Kind != "activity" || activity.Summary != "Codex activity" || activity.ItemID != "item-1" {
		t.Fatalf("unknown activity = %#v", activity)
	}
}

func TestMapAppServerThreadUsesRuntimeFlagsAndStableMetadata(t *testing.T) {
	raw := json.RawMessage(`{"id":"thread-1","name":"Launcher task","preview":"private preview","cwd":"/work/project","updatedAt":1783900000,"status":{"type":"active","activeFlags":["waitingOnUserInput"]},"turns":[{"id":"turn-1","status":"inProgress","items":[]}]}`)
	task, err := MapAppServerThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "thread-1" || task.Title != "Launcher task" || task.ProjectLabel != "project" || task.State != WaitingForAnswer || task.UpdatedAtUnix != 1783900000 || task.ActiveTurnID != "turn-1" {
		t.Fatalf("mapped app-server task = %#v", task)
	}
	if task.Title == "private preview" {
		t.Fatal("preview content became a title when an explicit name exists")
	}
}

func TestMapAppServerThreadSanitizesDisplayFieldsForThePhoneContract(t *testing.T) {
	raw := json.RawMessage("{\"id\":\"thread-1\",\"name\":\"  Multi\\nline\\tname\\u0000  \",\"cwd\":\"/work/ project\\nname \",\"updatedAt\":1783900000,\"status\":{\"type\":\"idle\",\"activeFlags\":[]},\"turns\":[]}")
	task, err := MapAppServerThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Multi line name" || task.ProjectLabel != "project name" {
		t.Fatalf("sanitized display fields = title %q, project %q", task.Title, task.ProjectLabel)
	}
}

func TestMapDesktopSnapshotFeedsTheSameStateMapper(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "desktopipc", "testdata", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Params struct {
			Change json.RawMessage `json:"change"`
		} `json:"params"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	task, err := MapDesktopSnapshot(envelope.Params.Change)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "thread-1" || task.ProjectLabel != "project" || task.State != WaitingForApproval {
		t.Fatalf("mapped desktop task = %#v", task)
	}
}

func TestDesktopSnapshotUsesLastTurnAndEveryPendingRequest(t *testing.T) {
	raw := json.RawMessage(`{"type":"snapshot","conversationState":{"id":"thread-1","cwd":"/work/project","threadRuntimeStatus":{"type":"idle"},"turns":[{"status":"completed"},{"status":"failed"}],"requests":[{"method":"future/non-actionable"},{"method":"item/tool/requestUserInput"}]}}`)
	task, err := MapDesktopSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != WaitingForAnswer {
		t.Fatalf("desktop task state = %q", task.State)
	}
	raw = json.RawMessage(`{"type":"snapshot","conversationState":{"id":"thread-1","cwd":"/work/project","threadRuntimeStatus":{"type":"idle"},"turns":[{"status":"interrupted"}],"requests":[]}}`)
	task, err = MapDesktopSnapshot(raw)
	if err != nil || task.State != Interrupted {
		t.Fatalf("interrupted desktop task = %#v, %v", task, err)
	}
}

func TestDesktopBusyTaskKeepsItsActiveTurnIDForSafeControls(t *testing.T) {
	raw := json.RawMessage(`{"type":"snapshot","conversationState":{"id":"thread-1","cwd":"/work/project","threadRuntimeStatus":{"type":"active","activeFlags":[]},"turns":[{"id":"turn-1","status":"inProgress"}],"requests":[]}}`)
	task, err := MapDesktopSnapshot(raw)
	if err != nil || task.State != Working || task.ActiveTurnID != "turn-1" {
		t.Fatalf("busy Desktop task = %#v, %v", task, err)
	}
}

func TestPublicServerRequestsMapToStablePendingKinds(t *testing.T) {
	tests := map[string]string{
		"item/commandExecution/requestApproval": "command",
		"item/fileChange/requestApproval":       "file",
		"item/permissions/requestApproval":      "permissions",
		"item/tool/requestUserInput":            "question",
		"mcpServer/elicitation/request":         "mcp_elicitation",
	}
	for method, want := range tests {
		if got := PendingKindForServerRequest(method); got != want {
			t.Fatalf("%s = %q, want %q", method, got, want)
		}
	}
	if got := PendingKindForServerRequest("future/request"); got != "" {
		t.Fatalf("unknown request kind = %q", got)
	}
}

func TestNotificationSignalsKeepTurnCompletionNonSemantic(t *testing.T) {
	signals, err := ApplyNotification(Signals{ThreadID: "thread-1", RuntimeStatus: "active"}, "turn/completed", json.RawMessage(`{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := Map(signals); got != IdleAfterReply {
		t.Fatalf("completed turn mapped to %q", got)
	}
	signals, err = ApplyNotification(signals, "thread/status/changed", json.RawMessage(`{"threadId":"thread-1","status":{"type":"active","activeFlags":["waitingOnApproval"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := Map(signals); got != WaitingForApproval {
		t.Fatalf("status change mapped to %q", got)
	}
}

func TestLifecycleNotificationsRequireTheirOwnTimestampAndCompleteTurnID(t *testing.T) {
	if _, err := DecodeNotification("thread-1", "item/started", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","completedAtMs":1,"item":{"id":"item-1","type":"agentMessage","text":"x"}}`)); err == nil {
		t.Fatal("item/started accepted completedAtMs")
	}
	if _, err := DecodeNotification("thread-1", "item/completed", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","completedAtMs":-1,"item":{"id":"item-1","type":"agentMessage","text":"x"}}`)); err == nil {
		t.Fatal("item/completed accepted negative timestamp")
	}
	current := Signals{ThreadID: "thread-1", RuntimeStatus: "active"}
	if _, err := ApplyNotification(current, "turn/completed", json.RawMessage(`{"threadId":"thread-1","turn":{"status":"completed","items":[]}}`)); err == nil {
		t.Fatal("turn/completed accepted missing turn ID")
	}
}

func TestNotificationsAreBoundToTheirThread(t *testing.T) {
	current := Signals{ThreadID: "thread-1", RuntimeStatus: "active", LastTurnStatus: "inProgress"}
	got, err := ApplyNotification(current, "turn/completed", json.RawMessage(`{"threadId":"thread-2","turn":{"status":"completed"}}`))
	if err == nil || !reflect.DeepEqual(got, current) {
		t.Fatalf("cross-thread notification changed state: got=%#v err=%v", got, err)
	}
}

func TestResolvedRequestMustMatchThePendingRequest(t *testing.T) {
	current := Signals{ThreadID: "thread-1", RuntimeStatus: "active", PendingRequestKind: "command", PendingRequestID: "approval-1"}
	got, err := ApplyNotification(current, "serverRequest/resolved", json.RawMessage(`{"threadId":"thread-1","requestId":"other"}`))
	if err == nil || !reflect.DeepEqual(got, current) {
		t.Fatalf("wrong request resolution = %#v, %v", got, err)
	}
	got, err = ApplyNotification(current, "serverRequest/resolved", json.RawMessage(`{"threadId":"thread-1","requestId":"approval-1"}`))
	if err != nil || got.PendingRequestKind != "" || got.PendingRequestID != "" {
		t.Fatalf("matching resolution = %#v, %v", got, err)
	}
}

func TestStrictThreadMappingRejectsUnknownOrIncompleteState(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"id":"thread-1","cwd":"/work","updatedAt":1,"status":{"type":"future"},"turns":[]}`),
		json.RawMessage(`{"id":"thread-1","cwd":"/work","updatedAt":0,"status":{"type":"idle"},"turns":[]}`),
		json.RawMessage(`{"id":"thread-1","cwd":"/work","updatedAt":1,"status":{"type":"active","activeFlags":["futureFlag"]},"turns":[]}`),
		json.RawMessage(`{"id":"thread-1","cwd":"/work","updatedAt":1,"status":{"type":"idle"},"turns":[{"status":"futureTurn"}]}`),
	} {
		if task, err := MapAppServerThread(raw); err == nil {
			t.Fatalf("accepted unsafe thread as %#v: %s", task, raw)
		}
	}
}

func TestNotificationDecoderProducesBoundedStableActivitiesAndDiffs(t *testing.T) {
	activity, err := DecodeNotification("thread-1", "item/completed", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","completedAtMs":1,"item":{"id":"item-1","type":"fileChange","status":"completed","changes":[{"path":"src/main.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if activity.Kind != "file" || activity.ItemID != "item-1" || len(activity.Diffs) != 1 || activity.Diffs[0].Path != "src/main.go" || activity.Diffs[0].Kind != "update" {
		t.Fatalf("decoded activity = %#v", activity)
	}
	if _, err := DecodeNotification("thread-1", "item/completed", json.RawMessage(`{"threadId":"thread-2","turnId":"turn-1","completedAtMs":1,"item":{"id":"item-1","type":"agentMessage","text":"x"}}`)); err == nil {
		t.Fatal("accepted activity for another thread")
	}
	message, err := DecodeNotification("thread-1", "item/agentMessage/delta", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-2","delta":"hello"}`))
	if err != nil || message.Kind != "reply" || message.ItemID != "item-2" || message.Delta != "hello" {
		t.Fatalf("message delta = %#v, %v", message, err)
	}
	patch, err := DecodeNotification("thread-1", "item/fileChange/patchUpdated", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-3","changes":[{"path":"src/main.go","kind":{"type":"add"},"diff":"+package main"}]}`))
	if err != nil || patch.Kind != "file" || len(patch.Diffs) != 1 || patch.Diffs[0].Kind != "add" {
		t.Fatalf("patch update = %#v, %v", patch, err)
	}
	terminal, err := DecodeNotification("thread-1", "item/commandExecution/terminalInteraction", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-4","processId":"7","stdin":"yes\n"}`))
	if err != nil || terminal.Delta != "yes\n" {
		t.Fatalf("terminal input = %#v, %v", terminal, err)
	}
	diff, err := DecodeNotification("thread-1", "turn/diff/updated", json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","diff":"@@ change"}`))
	if err != nil || diff.Kind != "diff" || diff.Delta != "@@ change" {
		t.Fatalf("turn diff = %#v, %v", diff, err)
	}
	if _, err := DecodeNotification("thread-1", "item/updated", json.RawMessage(`{"threadId":"thread-1","item":{"id":"item-1","type":"agentMessage"}}`)); err == nil {
		t.Fatal("accepted a notification method outside the current schema")
	}
}
