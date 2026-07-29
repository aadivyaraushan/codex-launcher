// Package turn converts one Claude Code session's frame stream into the task
// state and live events the phone already understands.
//
// The Codex app-server reports task state directly, as a status the companion
// can read. Claude Code does not: state has to be inferred from the sequence of
// frames a turn produces. This package is that inference, kept in one place so
// the rule for "what is this task doing right now" is testable on its own.
package turn

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/streamjson"
)

// Tool names the CLI reports. Mapping is by tool rather than by guesswork so an
// unknown tool degrades to generic activity instead of a wrong description.
const (
	ToolBash            = "Bash"
	ToolEdit            = "Edit"
	ToolWrite           = "Write"
	ToolNotebookEdit    = "NotebookEdit"
	ToolTodoWrite       = "TodoWrite"
	ToolRead            = "Read"
	ToolGrep            = "Grep"
	ToolGlob            = "Glob"
	ToolAskUserQuestion = "AskUserQuestion"
	ToolTask            = "Task"
)

// ActivityForTool maps a tool to a shared activity kind.
func ActivityForTool(tool string) string {
	switch tool {
	case ToolBash:
		return taskstate.ActivityCommand
	case ToolEdit, ToolWrite, ToolNotebookEdit:
		return taskstate.ActivityFile
	case ToolTodoWrite:
		return taskstate.ActivityPlan
	case ToolRead, ToolGrep, ToolGlob:
		return taskstate.ActivityRead
	default:
		// Includes MCP tools and subagent launches: real work, but nothing the
		// companion can honestly describe more precisely.
		return ""
	}
}

// Tracker holds the live state of one task. It is driven from the session's
// frame and control-request streams, which arrive on different goroutines, so
// it is safe for concurrent use.
type Tracker struct {
	taskID string
	label  taskstate.AgentLabel

	mu           sync.Mutex
	state        taskstate.State
	activeTurnID string
	// pendingRequestID is the can_use_tool request the phone is being asked
	// about. A task can only be blocked on one at a time.
	pendingRequestID string
	interrupting     bool
}

func New(taskID string, label taskstate.AgentLabel) *Tracker {
	return &Tracker{taskID: taskID, label: label, state: taskstate.IdleAfterReply}
}

// State reports what the task is doing now.
func (tracker *Tracker) State() taskstate.State {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.state
}

// ActiveTurnID is the in-flight assistant message, or empty when idle.
func (tracker *Tracker) ActiveTurnID() string {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.activeTurnID
}

// PendingRequestID is the approval the task is blocked on, or empty.
func (tracker *Tracker) PendingRequestID() string {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.pendingRequestID
}

// TurnStarted marks the moment a prompt is handed to the CLI. The phone shows
// the task as working from here, before the model has produced anything, so a
// sent prompt never looks like it vanished.
func (tracker *Tracker) TurnStarted() taskstate.MobileEvent {
	tracker.mu.Lock()
	tracker.state = taskstate.Working
	tracker.interrupting = false
	tracker.mu.Unlock()
	event := tracker.event("activity", taskstate.Working, taskstate.ActivitySummary(tracker.label, ""))
	event.StartsTurn = true
	return event
}

// Interrupting records that an interrupt was requested, so the result frame the
// CLI emits afterwards is reported as interrupted rather than as a normal
// reply.
func (tracker *Tracker) Interrupting() {
	tracker.mu.Lock()
	tracker.interrupting = true
	tracker.mu.Unlock()
}

// Frame folds one stdout frame into the task state. The second return is false
// when the frame carries nothing the phone needs to see.
func (tracker *Tracker) Frame(frame streamjson.Frame) (taskstate.MobileEvent, bool) {
	switch frame.Type {
	case streamjson.TypeAssistant:
		return tracker.assistant(frame)
	case streamjson.TypeResult:
		return tracker.result(frame)
	default:
		// system init, tool results, rate limit notices, and anything the CLI
		// adds later: real protocol traffic, but not a state change.
		return taskstate.MobileEvent{}, false
	}
}

func (tracker *Tracker) assistant(frame streamjson.Frame) (taskstate.MobileEvent, bool) {
	message, err := frame.AssistantMessage()
	if err != nil {
		return taskstate.MobileEvent{}, false
	}
	kind := ""
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" && kind == "" {
				kind = taskstate.ActivityReply
			}
		case "tool_use":
			// A tool call describes the turn better than accompanying prose, so
			// it wins when a message carries both.
			if activity := ActivityForTool(block.Name); activity != "" {
				kind = activity
			} else if kind == "" {
				kind = taskstate.ActivityCommand
			}
		}
	}
	tracker.mu.Lock()
	// An approval already asked about must not be overwritten by the same
	// message's tool call, or the phone would drop back to "working" while the
	// owner is still looking at the sheet.
	blocked := tracker.pendingRequestID != ""
	if !blocked {
		tracker.state = taskstate.Working
	}
	if message.ID != "" {
		tracker.activeTurnID = message.ID
	}
	tracker.mu.Unlock()
	if blocked {
		return taskstate.MobileEvent{}, false
	}
	return tracker.event("activity", taskstate.Working, taskstate.ActivitySummary(tracker.label, kind)), true
}

func (tracker *Tracker) result(frame streamjson.Frame) (taskstate.MobileEvent, bool) {
	result, err := frame.Result()
	if err != nil {
		// A result that cannot be read still ends the turn; reporting it as
		// working forever would strand the task.
		return tracker.finish(taskstate.Failed), true
	}
	switch {
	case tracker.wasInterrupting():
		return tracker.finish(taskstate.Interrupted), true
	case result.IsError || result.Subtype != streamjson.ResultSuccess:
		return tracker.finish(taskstate.Failed), true
	default:
		return tracker.finish(taskstate.IdleAfterReply), true
	}
}

func (tracker *Tracker) wasInterrupting() bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.interrupting
}

func (tracker *Tracker) finish(state taskstate.State) taskstate.MobileEvent {
	tracker.mu.Lock()
	tracker.state = state
	tracker.activeTurnID = ""
	tracker.pendingRequestID = ""
	tracker.interrupting = false
	label := tracker.label
	tracker.mu.Unlock()
	event, err := taskstate.ProjectTaskState(label, tracker.taskID, state)
	if err != nil {
		return tracker.event("failure", taskstate.Failed, "")
	}
	return event
}

// ControlRequest folds an inbound control request into the task state. Only
// can_use_tool changes what the phone shows; everything else is protocol
// housekeeping the caller still has to answer.
func (tracker *Tracker) ControlRequest(request streamjson.ControlRequest) (taskstate.MobileEvent, bool) {
	if request.Subtype != streamjson.SubtypeCanUseTool {
		return taskstate.MobileEvent{}, false
	}
	state := taskstate.WaitingForApproval
	kind := "approval"
	if request.ToolName == ToolAskUserQuestion {
		// The model is asking the owner something, not asking permission, so it
		// belongs in the question sheet.
		state = taskstate.WaitingForAnswer
		kind = "answer"
	}
	tracker.mu.Lock()
	tracker.state = state
	tracker.pendingRequestID = request.RequestID
	label := tracker.label
	tracker.mu.Unlock()

	event, err := taskstate.ProjectTaskState(label, tracker.taskID, state)
	if err != nil {
		return taskstate.MobileEvent{}, false
	}
	event.Kind = kind
	return event, true
}

// RequestAnswered clears the block once a decision has been sent back, so the
// task reads as working again while the tool actually runs.
func (tracker *Tracker) RequestAnswered(requestID string) (taskstate.MobileEvent, bool) {
	tracker.mu.Lock()
	if tracker.pendingRequestID == "" || tracker.pendingRequestID != requestID {
		tracker.mu.Unlock()
		return taskstate.MobileEvent{}, false
	}
	tracker.pendingRequestID = ""
	tracker.state = taskstate.Working
	label := tracker.label
	tracker.mu.Unlock()
	return tracker.eventWithLabel(label, "activity", taskstate.Working, taskstate.ActivitySummary(label, "")), true
}

func (tracker *Tracker) event(kind string, state taskstate.State, summary string) taskstate.MobileEvent {
	return tracker.eventWithLabel(tracker.label, kind, state, summary)
}

func (tracker *Tracker) eventWithLabel(label taskstate.AgentLabel, kind string, state taskstate.State, summary string) taskstate.MobileEvent {
	if summary == "" {
		summary = taskstate.ActivitySummary(label, "")
	}
	return taskstate.MobileEvent{TaskID: tracker.taskID, Kind: kind, State: state, Summary: summary}
}

// ToolInputPath pulls the file a tool is acting on, when it names one. It is
// used for transcript file-change entries, and returns empty rather than
// guessing when the tool does not carry a path.
func ToolInputPath(tool string, input json.RawMessage) string {
	switch tool {
	case ToolEdit, ToolWrite, ToolNotebookEdit, ToolRead:
	default:
		return ""
	}
	var value struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	if json.Unmarshal(input, &value) != nil {
		return ""
	}
	if value.FilePath != "" {
		return value.FilePath
	}
	return value.NotebookPath
}
