package taskstate

import "testing"

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
