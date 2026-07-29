package taskstate

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestProjectNotificationProducesOnlyStableMobileSummaries(t *testing.T) {
	tests := []struct {
		name   string
		method string
		params string
		want   MobileEvent
	}{
		{name: "turn started", method: "turn/started", params: `{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}`, want: MobileEvent{TaskID: "thread-1", Kind: "activity", State: Working, Summary: "Codex is working", StartsTurn: true}},
		{name: "reply", method: "turn/completed", params: `{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}`, want: MobileEvent{TaskID: "thread-1", Kind: "reply", State: IdleAfterReply, Summary: "Codex replied"}},
		{name: "failure", method: "turn/completed", params: `{"threadId":"thread-1","turn":{"id":"turn-1","status":"failed","items":[]}}`, want: MobileEvent{TaskID: "thread-1", Kind: "failure", State: Failed, Summary: "Codex hit an error"}},
		{name: "interrupted", method: "turn/completed", params: `{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}`, want: MobileEvent{TaskID: "thread-1", Kind: "interrupted", State: Interrupted, Summary: "Codex was interrupted"}},
		{name: "approval", method: "thread/status/changed", params: `{"threadId":"thread-1","status":{"type":"active","activeFlags":["waitingOnApproval"]}}`, want: MobileEvent{TaskID: "thread-1", Kind: "approval", State: WaitingForApproval, Summary: "Needs your approval"}},
		{name: "answer", method: "thread/status/changed", params: `{"threadId":"thread-1","status":{"type":"active","activeFlags":["waitingOnUserInput"]}}`, want: MobileEvent{TaskID: "thread-1", Kind: "answer", State: WaitingForAnswer, Summary: "Needs your answer"}},
		{name: "command", method: "item/commandExecution/outputDelta", params: `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"private command output"}`, want: MobileEvent{TaskID: "thread-1", Kind: "activity", State: Working, Summary: "Running a command"}},
		{name: "file", method: "item/fileChange/patchUpdated", params: `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","changes":[{"path":"private/file.go","kind":{"type":"update"},"diff":"private diff"}]}`, want: MobileEvent{TaskID: "thread-1", Kind: "activity", State: Working, Summary: "Editing files"}},
		{name: "writing reply", method: "item/agentMessage/delta", params: `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"private reply"}`, want: MobileEvent{TaskID: "thread-1", Kind: "activity", State: Working, Summary: "Writing a reply"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ProjectNotification(test.method, json.RawMessage(test.params))
			if err != nil || got != test.want {
				t.Fatalf("ProjectNotification() = %#v, %v; want %#v", got, err, test.want)
			}
		})
	}
}

func TestProjectNotificationRejectsInvalidSupportedEventsAndSkipsUnrelatedOnes(t *testing.T) {
	if _, err := ProjectNotification("turn/completed", json.RawMessage(`{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress"}}`)); !errors.Is(err, ErrInvalidLiveNotification) {
		t.Fatalf("in-progress completion error = %v", err)
	}
	if _, err := ProjectNotification("thread/status/changed", json.RawMessage(`{"threadId":"thread-1","status":{"type":"idle"}}`)); !errors.Is(err, ErrUnsupportedLiveNotification) {
		t.Fatalf("idle status error = %v", err)
	}
	if _, err := ProjectNotification("account/updated", json.RawMessage(`{"private":"content"}`)); !errors.Is(err, ErrUnsupportedLiveNotification) {
		t.Fatalf("unrelated notification error = %v", err)
	}
}

func TestProjectTaskStateProducesOnlyFixedMobileSummaries(t *testing.T) {
	tests := []struct {
		state State
		want  MobileEvent
	}{
		{Working, MobileEvent{TaskID: "thread-1", Kind: "activity", State: Working, Summary: "Codex is working"}},
		{WaitingForApproval, MobileEvent{TaskID: "thread-1", Kind: "approval", State: WaitingForApproval, Summary: "Needs your approval"}},
		{WaitingForAnswer, MobileEvent{TaskID: "thread-1", Kind: "answer", State: WaitingForAnswer, Summary: "Needs your answer"}},
		{Failed, MobileEvent{TaskID: "thread-1", Kind: "failure", State: Failed, Summary: "Codex hit an error"}},
		{Interrupted, MobileEvent{TaskID: "thread-1", Kind: "interrupted", State: Interrupted, Summary: "Codex was interrupted"}},
		{IdleAfterReply, MobileEvent{TaskID: "thread-1", Kind: "reply", State: IdleAfterReply, Summary: "Codex replied"}},
	}
	for _, test := range tests {
		got, err := ProjectTaskState(LabelCodex, "thread-1", test.state)
		if err != nil || got != test.want {
			t.Fatalf("ProjectTaskState(%q) = %#v, %v; want %#v", test.state, got, err, test.want)
		}
	}
	if _, err := ProjectTaskState(LabelCodex, "", Working); err == nil {
		t.Fatal("empty task ID was accepted")
	}
	if _, err := ProjectTaskState(LabelCodex, "thread-1", State("future")); err == nil {
		t.Fatal("unknown task state was accepted")
	}
}

// Two agents doing the same thing must describe it to the owner with the same
// words, so the activity vocabulary is shared rather than per-backend.
func TestActivitySummaryIsTheSameForEveryAgent(t *testing.T) {
	shared := map[string]string{
		ActivityReply:   "Writing a reply",
		ActivityCommand: "Running a command",
		ActivityFile:    "Editing files",
		ActivityPlan:    "Updating the plan",
		ActivityDiff:    "Reviewing changes",
		ActivityRead:    "Reading files",
	}
	for kind, want := range shared {
		for _, label := range []AgentLabel{LabelCodex, LabelClaudeCode} {
			if got := ActivitySummary(label, kind); got != want {
				t.Fatalf("ActivitySummary(%q, %q) = %q; want %q", label, kind, got, want)
			}
		}
	}
	// An unknown kind falls back to the agent's generic line rather than
	// leaking a raw tool or item name to the launcher.
	if got := ActivitySummary(LabelClaudeCode, "mcp__something__private"); got != "Claude is working" {
		t.Fatalf("unknown kind summary = %q", got)
	}
	if got := ActivitySummary(LabelCodex, ""); got != "Codex is working" {
		t.Fatalf("empty kind summary = %q", got)
	}
}

// The summary reaches the launcher verbatim, so a non-Codex backend must not
// tell the owner that Codex is doing the work.
func TestProjectTaskStateNamesTheAgentThatOwnsTheTask(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{Working, "Claude is working"},
		{IdleAfterReply, "Claude replied"},
		{Failed, "Claude hit an error"},
		{Interrupted, "Claude was interrupted"},
	}
	for _, test := range tests {
		got, err := ProjectTaskState(LabelClaudeCode, "session-1", test.state)
		if err != nil || got.Summary != test.want {
			t.Fatalf("ProjectTaskState(%q, %q).Summary = %q, %v; want %q", LabelClaudeCode, test.state, got.Summary, err, test.want)
		}
	}
	// Decision prompts are about the owner, not the agent, so they stay fixed.
	for _, state := range []State{WaitingForApproval, WaitingForAnswer} {
		codex, _ := ProjectTaskState(LabelCodex, "session-1", state)
		claude, _ := ProjectTaskState(LabelClaudeCode, "session-1", state)
		if codex.Summary != claude.Summary {
			t.Fatalf("decision summary for %q differs by agent: %q vs %q", state, codex.Summary, claude.Summary)
		}
	}
}
