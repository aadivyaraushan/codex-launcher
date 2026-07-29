package runtime

import (
	"context"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
)

func TestMergeTaskEventsCombinesAppServerAndDesktopSources(t *testing.T) {
	appEvents := make(chan taskstate.MobileEvent, 1)
	desktopEvents := make(chan taskstate.MobileEvent, 1)
	appEvents <- taskstate.MobileEvent{TaskID: "app-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	desktopEvents <- taskstate.MobileEvent{TaskID: "desktop-1", Kind: "approval", State: taskstate.WaitingForApproval, Summary: "Needs your approval"}
	close(appEvents)
	close(desktopEvents)
	output := make(chan taskstate.MobileEvent)
	go mergeTaskEvents(context.Background(), output, nil, appEvents, desktopEvents)

	got := make(map[string]taskstate.MobileEvent)
	for event := range output {
		got[event.TaskID] = event
	}
	if len(got) != 2 || got["app-1"].State != taskstate.Working || got["desktop-1"].State != taskstate.WaitingForApproval {
		t.Fatalf("merged events = %#v", got)
	}
}
