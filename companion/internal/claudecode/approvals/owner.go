// Package approvals turns the Claude Code CLI's can_use_tool control requests
// into the decision requests the phone already renders, and routes the owner's
// answer back to the CLI child that is blocked on it.
//
// This is the path the launcher exists to serve. A can_use_tool request holds
// the CLI's turn open until it is answered, so every registered request must
// end in either an answer or an explicit failure -- never silence.
package approvals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/streamjson"
	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/turn"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

var (
	ErrUnknownRequest = errors.New("Claude Code approval request is unknown")
	ErrNoLiveSession  = errors.New("Claude Code session is no longer running")
	ErrNotApproval    = errors.New("Claude Code control request is not a tool approval")
)

// Answerer is the live CLI session an answer is written back to.
type Answerer interface {
	Answer(ctx context.Context, frame json.RawMessage) error
	AnswerError(ctx context.Context, requestID, message string) error
}

// Owner holds the approvals currently waiting on the phone.
type Owner struct {
	logger *slog.Logger

	mu       sync.Mutex
	pending  map[string]pendingApproval
	sessions map[string]Answerer
}

type pendingApproval struct {
	taskID string
	// cliRequestID is the CLI's own request ID, which the answer must quote.
	cliRequestID string
	kind         decisions.Kind
	// input is echoed back on allow: the phone approved the call the model
	// asked for, so the companion must not substitute a different one.
	input json.RawMessage
	// suggestions are the CLI's proposed permission rules, replayed when the
	// owner chooses to allow for the rest of the session.
	suggestions json.RawMessage
	questionIDs []string
}

func NewOwner(logger *slog.Logger) *Owner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Owner{logger: logger, pending: make(map[string]pendingApproval), sessions: make(map[string]Answerer)}
}

// AttachSession registers the live child that answers a task's approvals.
func (owner *Owner) AttachSession(taskID string, session Answerer) {
	if owner == nil || taskID == "" || session == nil {
		return
	}
	owner.mu.Lock()
	owner.sessions[taskID] = session
	owner.mu.Unlock()
}

// DetachSession drops a finished child and abandons anything still pending for
// it. Those approvals can never be delivered now, so keeping them would leave
// the phone showing sheets that do nothing.
func (owner *Owner) DetachSession(taskID string) []string {
	if owner == nil || taskID == "" {
		return nil
	}
	owner.mu.Lock()
	delete(owner.sessions, taskID)
	abandoned := make([]string, 0)
	for id, pending := range owner.pending {
		if pending.taskID == taskID {
			abandoned = append(abandoned, id)
			delete(owner.pending, id)
		}
	}
	owner.mu.Unlock()
	if len(abandoned) > 0 {
		owner.logger.Info("[claude-approvals] abandoned pending approvals", "task_id", taskID, "count", len(abandoned), "branch_reason", "session_ended")
	}
	return abandoned
}

// Register converts a can_use_tool request into a decision request for the
// phone. The returned request ID is what the phone answers with.
func (owner *Owner) Register(taskID string, request streamjson.ControlRequest, display decisions.DisplayContext, expiresAt time.Time) (decisions.Request, error) {
	if owner == nil {
		return decisions.Request{}, ErrUnknownRequest
	}
	if request.Subtype != streamjson.SubtypeCanUseTool {
		return decisions.Request{}, ErrNotApproval
	}
	if taskID == "" || request.RequestID == "" || expiresAt.IsZero() {
		return decisions.Request{}, decisions.ErrInvalidRequest
	}

	id := mobileRequestID(taskID, request.RequestID)
	built := decisions.Request{
		ID:           id,
		ThreadID:     taskID,
		TurnID:       turnIDFor(request),
		ItemID:       itemIDFor(request, id),
		ComputerName: display.ComputerName,
		ProjectLabel: display.ProjectLabel,
		ExpiresAt:    expiresAt,
	}

	owned := pendingApproval{taskID: taskID, cliRequestID: request.RequestID, input: cloneRaw(request.Input), suggestions: cloneRaw(request.Suggestions)}

	switch {
	case request.ToolName == turn.ToolAskUserQuestion:
		questions, err := decodeQuestions(request.Input)
		if err != nil {
			return decisions.Request{}, err
		}
		built.Kind = decisions.KindQuestion
		built.Questions = questions
		owned.questionIDs = make([]string, 0, len(questions))
		for _, question := range questions {
			owned.questionIDs = append(owned.questionIDs, question.ID)
		}
	case request.ToolName == turn.ToolBash:
		built.Kind = decisions.KindCommand
		// Reuse the existing redaction so a command carrying a secret is not
		// displayed verbatim on the phone, and an unparseable one cannot be
		// approved at all.
		built.Command, built.CommandUnderstandable = decisions.RedactCommand(commandFrom(request.Input))
		if built.Command == "" {
			built.Command = "<redacted:unavailable>"
			built.CommandUnderstandable = false
		}
		built.AllowedDecisions = []decisions.Decision{decisions.DecisionAcceptOnce, decisions.DecisionAcceptSession, decisions.DecisionDecline}
	case turn.ActivityForTool(request.ToolName) == "file":
		built.Kind = decisions.KindFile
		if path := turn.ToolInputPath(request.ToolName, request.Input); path != "" {
			built.AffectedPaths = []string{path}
		}
		built.AllowedDecisions = []decisions.Decision{decisions.DecisionAcceptOnce, decisions.DecisionAcceptSession, decisions.DecisionDecline}
	default:
		// Reads, searches, connected MCP tools, and anything the CLI adds
		// later. Deliberately not KindMCP: in Codex that means an MCP server
		// elicitation, which the phone only ever lets you decline, whereas a
		// Claude Code MCP tool call is a normal tool the owner can allow.
		built.Kind = decisions.KindPermissions
		built.Access = accessDescription(request.ToolName)
		built.AllowedDecisions = []decisions.Decision{decisions.DecisionAcceptOnce, decisions.DecisionAcceptSession, decisions.DecisionDecline}
	}
	owned.kind = built.Kind

	owner.mu.Lock()
	if previous, exists := owner.pending[id]; exists && previous.cliRequestID != request.RequestID {
		owner.mu.Unlock()
		return decisions.Request{}, decisions.ErrInvalidRequest
	}
	owner.pending[id] = owned
	owner.mu.Unlock()
	return built, nil
}

// Respond delivers the owner's decision to the CLI child that is waiting.
//
// It satisfies decisions.Responder, so the existing router drives it exactly
// the way it drives the Codex owner.
func (owner *Owner) Respond(ctx context.Context, response decisions.Response) error {
	if owner == nil {
		return decisions.ErrOwnerUnavailable
	}
	owner.mu.Lock()
	owned, exists := owner.pending[response.RequestID]
	var session Answerer
	if exists {
		session = owner.sessions[owned.taskID]
	}
	owner.mu.Unlock()
	if !exists || owned.kind != response.Kind || owned.taskID != response.ThreadID {
		return decisions.ErrOwnerUnavailable
	}
	if session == nil {
		// The child is gone, so the turn it was blocking is gone too. Report it
		// as unavailable rather than pretending the decision landed.
		return decisions.ErrOwnerUnavailable
	}

	frame, err := owner.frameFor(owned, response)
	if err != nil {
		return err
	}
	if err := session.Answer(ctx, frame); err != nil {
		if errors.Is(err, streamjson.ErrClosed) {
			return decisions.ErrOwnerUnavailable
		}
		return err
	}
	owner.mu.Lock()
	delete(owner.pending, response.RequestID)
	owner.mu.Unlock()
	owner.logger.Info("[claude-approvals] decision delivered",
		"task_id", owned.taskID, "request_kind", owned.kind, "decision", string(response.Decision), "answered", len(response.Answers) > 0)
	return nil
}

func (owner *Owner) frameFor(owned pendingApproval, response decisions.Response) (json.RawMessage, error) {
	if owned.kind == decisions.KindQuestion {
		return answerFrame(owned, response.Answers)
	}
	switch response.Decision {
	case decisions.DecisionAcceptOnce:
		return streamjson.Allow(owned.cliRequestID, owned.input, nil)
	case decisions.DecisionAcceptSession:
		// Replaying the CLI's own suggestions is what stops it asking about the
		// same tool again for the rest of the session.
		return streamjson.Allow(owned.cliRequestID, owned.input, owned.suggestions)
	case decisions.DecisionDecline, decisions.DecisionCancel:
		return streamjson.Deny(owned.cliRequestID, "Declined from the paired phone.")
	default:
		return nil, decisions.ErrDecisionNotOffered
	}
}

// answerFrame returns an AskUserQuestion answer as the tool's result. The CLI
// expects the answer in the tool's own shape, so it is allowed with an input
// carrying the owner's selections.
func answerFrame(owned pendingApproval, answers map[string][]string) (json.RawMessage, error) {
	if len(answers) == 0 {
		return nil, decisions.ErrRequestMismatch
	}
	selections := make([]map[string]any, 0, len(owned.questionIDs))
	for index, id := range owned.questionIDs {
		chosen, found := answers[id]
		if !found || len(chosen) == 0 {
			return nil, decisions.ErrRequestMismatch
		}
		selections = append(selections, map[string]any{"questionIndex": index, "answers": chosen})
	}
	updated, err := json.Marshal(map[string]any{"selections": selections})
	if err != nil {
		return nil, err
	}
	return streamjson.Allow(owned.cliRequestID, updated, nil)
}

// Fail unblocks a request the companion cannot serve, so the CLI's turn does
// not hang forever waiting for an answer that is never coming.
func (owner *Owner) Fail(ctx context.Context, taskID, cliRequestID, reason string) error {
	if owner == nil {
		return ErrUnknownRequest
	}
	owner.mu.Lock()
	session := owner.sessions[taskID]
	delete(owner.pending, mobileRequestID(taskID, cliRequestID))
	owner.mu.Unlock()
	if session == nil {
		return ErrNoLiveSession
	}
	return session.AnswerError(ctx, cliRequestID, reason)
}

// PendingCLIRequestID maps a phone-facing request ID back to the CLI's, which
// the tracker needs to clear its own pending state.
func (owner *Owner) PendingCLIRequestID(requestID string) (string, bool) {
	if owner == nil {
		return "", false
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owned, exists := owner.pending[requestID]
	if !exists {
		return "", false
	}
	return owned.cliRequestID, true
}

func commandFrom(input json.RawMessage) string {
	var value struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(input, &value) != nil {
		return ""
	}
	return value.Command
}

func accessDescription(tool string) string {
	switch {
	case tool == "":
		return "Requested to use a tool"
	case strings.HasPrefix(tool, "mcp__"):
		return "Requested to use a connected tool"
	default:
		return "Requested to use " + tool
	}
}

// decodeQuestions reads the AskUserQuestion input. Question IDs are positional
// because the tool input does not carry stable ones, and the position is what
// the answer is keyed back to.
func decodeQuestions(input json.RawMessage) ([]decisions.Question, error) {
	var value struct {
		Questions []struct {
			Question    string `json:"question"`
			Header      string `json:"header"`
			MultiSelect bool   `json:"multiSelect"`
			Options     []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(input, &value) != nil || len(value.Questions) == 0 || len(value.Questions) > 32 {
		return nil, decisions.ErrInvalidRequest
	}
	questions := make([]decisions.Question, 0, len(value.Questions))
	for index, source := range value.Questions {
		prompt := strings.TrimSpace(source.Question)
		if prompt == "" {
			return nil, decisions.ErrInvalidRequest
		}
		options := make([]string, 0, len(source.Options))
		for _, option := range source.Options {
			if label := strings.TrimSpace(option.Label); label != "" {
				options = append(options, label)
			}
		}
		if len(options) > 32 {
			options = options[:32]
		}
		header := strings.TrimSpace(source.Header)
		if header == "" {
			header = "Question"
		}
		questions = append(questions, decisions.Question{
			ID:      "q" + strconv.Itoa(index),
			Header:  header,
			Prompt:  prompt,
			Options: options,
		})
	}
	return questions, nil
}

// mobileRequestID derives a stable phone-facing ID. The CLI's request IDs are
// UUIDs, which already satisfy the wire format, but they are namespaced by task
// so a repeat across sessions cannot collide.
func mobileRequestID(taskID, cliRequestID string) string {
	return fmt.Sprintf("cc-%s-%s", shortHash(taskID), shortHash(cliRequestID))
}

// turnIDFor and itemIDFor fill the identifiers the decision protocol requires.
// Claude Code has no turn or item concept in a control request, so the tool use
// is used where it exists and the request stands in for itself otherwise.
func turnIDFor(request streamjson.ControlRequest) string {
	if request.ToolUseID != "" {
		return request.ToolUseID
	}
	return "cc-turn-" + shortHash(request.RequestID)
}

func itemIDFor(request streamjson.ControlRequest, fallback string) string {
	if request.ToolUseID != "" {
		return request.ToolUseID
	}
	return fallback
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// shortHash keeps derived identifiers inside the wire format's character set
// and length limit regardless of what the CLI used.
func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
