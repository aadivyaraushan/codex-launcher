package turn

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/streamjson"
)

const taskID = "11111111-2222-3333-4444-555555555555"

func newTracker() *Tracker { return New(taskID, taskstate.LabelClaudeCode) }

func frame(t *testing.T, line string) streamjson.Frame {
	t.Helper()
	decoded, err := streamjson.DecodeFrame([]byte(line))
	if err != nil {
		t.Fatalf("DecodeFrame() error = %v", err)
	}
	return decoded
}

func assistantWith(content string) string {
	return `{"type":"assistant","message":{"id":"msg_1","model":"claude-sonnet-4-6","content":[` + content + `]},"session_id":"` + taskID + `"}`
}

func TestActivityForToolDescribesWorkTheOwnerWouldRecognise(t *testing.T) {
	tests := map[string]string{
		ToolBash:         taskstate.ActivityCommand,
		ToolEdit:         taskstate.ActivityFile,
		ToolWrite:        taskstate.ActivityFile,
		ToolNotebookEdit: taskstate.ActivityFile,
		ToolTodoWrite:    taskstate.ActivityPlan,
		ToolRead:         taskstate.ActivityRead,
		ToolGrep:         taskstate.ActivityRead,
		ToolGlob:         taskstate.ActivityRead,
	}
	for tool, want := range tests {
		if got := ActivityForTool(tool); got != want {
			t.Fatalf("ActivityForTool(%q) = %q; want %q", tool, got, want)
		}
	}
	// An unknown or MCP tool has no honest description, so it must not be
	// forced into one.
	for _, tool := range []string{"mcp__playwright__browser_click", "SomeFutureTool", ""} {
		if got := ActivityForTool(tool); got != "" {
			t.Fatalf("ActivityForTool(%q) = %q; want no claim", tool, got)
		}
	}
}

func TestTurnStartedReportsWorkBeforeTheModelProducesAnything(t *testing.T) {
	tracker := newTracker()
	event := tracker.TurnStarted()
	if event.State != taskstate.Working || event.TaskID != taskID {
		t.Fatalf("event = %#v", event)
	}
	// StartsTurn is what stops a freshly sent prompt from looking like it
	// vanished on the phone.
	if !event.StartsTurn {
		t.Fatal("turn start did not mark the start of a turn")
	}
	if event.Summary != "Claude is working" {
		t.Fatalf("summary = %q", event.Summary)
	}
	if tracker.State() != taskstate.Working {
		t.Fatalf("state = %q", tracker.State())
	}
}

func TestAssistantFramesDescribeWhatTheAgentIsDoing(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"text", `{"type":"text","text":"Here is the plan"}`, "Writing a reply"},
		{"bash", `{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}`, "Running a command"},
		{"edit", `{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/p/a.go"}}`, "Editing files"},
		{"todo", `{"type":"tool_use","id":"t1","name":"TodoWrite","input":{}}`, "Updating the plan"},
		{"read", `{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/p/a.go"}}`, "Reading files"},
		{"unknown tool", `{"type":"tool_use","id":"t1","name":"mcp__x__y","input":{}}`, "Running a command"},
		{"thinking only", `{"type":"thinking","thinking":"considering"}`, "Claude is working"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracker := newTracker()
			tracker.TurnStarted()
			event, ok := tracker.Frame(frame(t, assistantWith(test.content)))
			if !ok {
				t.Fatal("assistant frame produced no event")
			}
			if event.Summary != test.want {
				t.Fatalf("summary = %q; want %q", event.Summary, test.want)
			}
			if event.State != taskstate.Working {
				t.Fatalf("state = %q", event.State)
			}
		})
	}
}

// A tool call says more about the turn than the prose next to it.
func TestToolUseWinsOverAccompanyingText(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	line := assistantWith(`{"type":"text","text":"Let me look"},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/p/a.go"}}`)
	event, ok := tracker.Frame(frame(t, line))
	if !ok || event.Summary != "Reading files" {
		t.Fatalf("event = %#v", event)
	}
}

func TestAssistantFrameTracksTheActiveTurn(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	tracker.Frame(frame(t, assistantWith(`{"type":"text","text":"hi"}`)))
	if tracker.ActiveTurnID() != "msg_1" {
		t.Fatalf("active turn = %q", tracker.ActiveTurnID())
	}
}

func TestResultEndsTheTurn(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		state taskstate.State
		kind  string
	}{
		{
			"success",
			`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"` + taskID + `"}`,
			taskstate.IdleAfterReply, "reply",
		},
		{
			"error flag",
			`{"type":"result","subtype":"success","is_error":true,"result":"boom","session_id":"` + taskID + `"}`,
			taskstate.Failed, "failure",
		},
		{
			"non-success subtype",
			`{"type":"result","subtype":"error_max_turns","is_error":false,"session_id":"` + taskID + `"}`,
			taskstate.Failed, "failure",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracker := newTracker()
			tracker.TurnStarted()
			event, ok := tracker.Frame(frame(t, test.line))
			if !ok {
				t.Fatal("result produced no event")
			}
			if event.State != test.state || event.Kind != test.kind {
				t.Fatalf("event = %#v; want state %q kind %q", event, test.state, test.kind)
			}
			if tracker.ActiveTurnID() != "" {
				t.Fatalf("finished turn still active: %q", tracker.ActiveTurnID())
			}
		})
	}
}

// A task stuck in "working" forever is worse than one reported as failed: the
// owner can retry a failure, but a stuck task offers nothing.
func TestAnUnreadableResultStillEndsTheTurn(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	event, ok := tracker.Frame(streamjson.Frame{Type: streamjson.TypeResult, Raw: json.RawMessage(`{"type":"result","subtype":`)})
	if !ok || event.State != taskstate.Failed {
		t.Fatalf("event = %#v, ok = %v", event, ok)
	}
	if tracker.State() != taskstate.Failed {
		t.Fatalf("state = %q", tracker.State())
	}
}

func TestInterruptedTurnIsNotReportedAsAReply(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	tracker.Interrupting()
	line := `{"type":"result","subtype":"success","is_error":false,"result":"stopped","session_id":"` + taskID + `"}`
	event, ok := tracker.Frame(frame(t, line))
	if !ok {
		t.Fatal("interrupted result produced no event")
	}
	if event.State != taskstate.Interrupted || event.Summary != "Claude was interrupted" {
		t.Fatalf("event = %#v", event)
	}
	// The flag must not leak into the next turn.
	tracker.TurnStarted()
	next, _ := tracker.Frame(frame(t, line))
	if next.State != taskstate.IdleAfterReply {
		t.Fatalf("next turn state = %q; want idle_after_reply", next.State)
	}
}

func TestCanUseToolBlocksTheTaskOnTheOwner(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	request := streamjson.ControlRequest{RequestID: "req-1", Subtype: streamjson.SubtypeCanUseTool, ToolName: ToolBash}
	event, ok := tracker.ControlRequest(request)
	if !ok {
		t.Fatal("approval request produced no event")
	}
	if event.State != taskstate.WaitingForApproval || event.Kind != "approval" {
		t.Fatalf("event = %#v", event)
	}
	if event.Summary != "Needs your approval" {
		t.Fatalf("summary = %q", event.Summary)
	}
	if tracker.PendingRequestID() != "req-1" {
		t.Fatalf("pending request = %q", tracker.PendingRequestID())
	}
}

// AskUserQuestion is the model asking the owner something, not asking
// permission, so it belongs in the question sheet.
func TestAskUserQuestionBecomesAQuestionNotAnApproval(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	request := streamjson.ControlRequest{RequestID: "req-1", Subtype: streamjson.SubtypeCanUseTool, ToolName: ToolAskUserQuestion}
	event, ok := tracker.ControlRequest(request)
	if !ok || event.State != taskstate.WaitingForAnswer || event.Kind != "answer" {
		t.Fatalf("event = %#v, ok = %v", event, ok)
	}
}

func TestNonApprovalControlRequestsDoNotChangeTaskState(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	for _, subtype := range []string{streamjson.SubtypeHookCallback, streamjson.SubtypeMCPMessage, streamjson.SubtypeControlCancelRequest} {
		if _, ok := tracker.ControlRequest(streamjson.ControlRequest{RequestID: "r", Subtype: subtype}); ok {
			t.Fatalf("%q changed task state", subtype)
		}
	}
	if tracker.State() != taskstate.Working {
		t.Fatalf("state = %q", tracker.State())
	}
}

// While the owner is looking at an approval sheet, later frames from the same
// message must not drop the task back to "working".
func TestFramesDuringAnApprovalDoNotClearTheBlock(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	tracker.ControlRequest(streamjson.ControlRequest{RequestID: "req-1", Subtype: streamjson.SubtypeCanUseTool, ToolName: ToolBash})
	if _, ok := tracker.Frame(frame(t, assistantWith(`{"type":"text","text":"still going"}`))); ok {
		t.Fatal("an assistant frame cleared a pending approval")
	}
	if tracker.State() != taskstate.WaitingForApproval {
		t.Fatalf("state = %q; want waiting_for_approval", tracker.State())
	}
}

func TestAnsweringTheRequestReturnsTheTaskToWorking(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	tracker.ControlRequest(streamjson.ControlRequest{RequestID: "req-1", Subtype: streamjson.SubtypeCanUseTool, ToolName: ToolBash})

	if _, ok := tracker.RequestAnswered("some-other-request"); ok {
		t.Fatal("an unrelated request ID cleared the block")
	}
	event, ok := tracker.RequestAnswered("req-1")
	if !ok || event.State != taskstate.Working {
		t.Fatalf("event = %#v, ok = %v", event, ok)
	}
	if tracker.PendingRequestID() != "" {
		t.Fatalf("pending request survived the answer: %q", tracker.PendingRequestID())
	}
	if _, ok := tracker.RequestAnswered("req-1"); ok {
		t.Fatal("the same request was answered twice")
	}
}

func TestResultClearsAPendingApproval(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	tracker.ControlRequest(streamjson.ControlRequest{RequestID: "req-1", Subtype: streamjson.SubtypeCanUseTool, ToolName: ToolBash})
	line := `{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"` + taskID + `"}`
	tracker.Frame(frame(t, line))
	if tracker.PendingRequestID() != "" {
		t.Fatalf("approval outlived the turn: %q", tracker.PendingRequestID())
	}
}

func TestNonStateFramesAreIgnored(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"` + taskID + `","cwd":"/p","model":"m","permissionMode":"default"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"out"}]},"session_id":"` + taskID + `"}`,
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"},"session_id":"` + taskID + `"}`,
	}
	for _, line := range lines {
		if _, ok := tracker.Frame(frame(t, line)); ok {
			t.Fatalf("frame produced a spurious event: %s", line)
		}
	}
	if tracker.State() != taskstate.Working {
		t.Fatalf("state = %q", tracker.State())
	}
}

func TestToolInputPathOnlyClaimsAPathWhenTheToolCarriesOne(t *testing.T) {
	tests := []struct {
		tool  string
		input string
		want  string
	}{
		{ToolEdit, `{"file_path":"/p/a.go"}`, "/p/a.go"},
		{ToolWrite, `{"file_path":"/p/b.go"}`, "/p/b.go"},
		{ToolNotebookEdit, `{"notebook_path":"/p/c.ipynb"}`, "/p/c.ipynb"},
		{ToolRead, `{"file_path":"/p/d.go"}`, "/p/d.go"},
		{ToolBash, `{"command":"rm /p/e.go"}`, ""},
		{ToolEdit, `not json`, ""},
		{ToolEdit, `{}`, ""},
	}
	for _, test := range tests {
		if got := ToolInputPath(test.tool, json.RawMessage(test.input)); got != test.want {
			t.Fatalf("ToolInputPath(%q, %s) = %q; want %q", test.tool, test.input, got, test.want)
		}
	}
}

// Frames and control requests arrive on different goroutines in production.
func TestTrackerIsSafeUnderConcurrentDrivers(t *testing.T) {
	tracker := newTracker()
	tracker.TurnStarted()
	assistant := frame(t, assistantWith(`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}`))

	var group sync.WaitGroup
	group.Add(3)
	go func() {
		defer group.Done()
		for range 200 {
			tracker.Frame(assistant)
		}
	}()
	go func() {
		defer group.Done()
		for index := range 200 {
			tracker.ControlRequest(streamjson.ControlRequest{RequestID: "req", Subtype: streamjson.SubtypeCanUseTool, ToolName: ToolBash})
			if index%2 == 0 {
				tracker.RequestAnswered("req")
			}
		}
	}()
	go func() {
		defer group.Done()
		for range 200 {
			_ = tracker.State()
			_ = tracker.ActiveTurnID()
			_ = tracker.PendingRequestID()
		}
	}()
	group.Wait()
}
