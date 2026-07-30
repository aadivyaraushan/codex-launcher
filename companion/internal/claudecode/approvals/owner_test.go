package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/streamjson"
	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/turn"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

const taskID = "11111111-2222-3333-4444-555555555555"

// The owner must be usable by the existing router unchanged.
var _ decisions.Responder = (*Owner)(nil)

type fakeSession struct {
	mu       sync.Mutex
	frames   []json.RawMessage
	failures []string
	err      error
}

func (session *fakeSession) Answer(_ context.Context, frame json.RawMessage) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.err != nil {
		return session.err
	}
	session.frames = append(session.frames, append(json.RawMessage(nil), frame...))
	return nil
}

func (session *fakeSession) AnswerError(_ context.Context, requestID, message string) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.failures = append(session.failures, requestID+": "+message)
	return nil
}

func (session *fakeSession) last(t *testing.T) map[string]any {
	t.Helper()
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.frames) == 0 {
		t.Fatal("no answer was written to the CLI")
	}
	var decoded map[string]any
	if err := json.Unmarshal(session.frames[len(session.frames)-1], &decoded); err != nil {
		t.Fatalf("answer frame is not JSON: %v", err)
	}
	return decoded
}

// inner returns the decision payload the CLI reads.
func inner(t *testing.T, frame map[string]any) map[string]any {
	t.Helper()
	response, _ := frame["response"].(map[string]any)
	if response == nil {
		t.Fatalf("frame has no response: %v", frame)
	}
	payload, _ := response["response"].(map[string]any)
	if payload == nil {
		t.Fatalf("frame has no decision payload: %v", frame)
	}
	return payload
}

func canUseTool(tool, input string) streamjson.ControlRequest {
	return streamjson.ControlRequest{
		RequestID:   "cli-request-1",
		Subtype:     streamjson.SubtypeCanUseTool,
		ToolName:    tool,
		ToolUseID:   "toolu_1",
		Input:       json.RawMessage(input),
		Suggestions: json.RawMessage(`[{"type":"addRules","rules":[{"toolName":"` + tool + `"}]}]`),
	}
}

// bashInput builds a Bash tool input so commands containing control
// characters stay visible in the source instead of being pasted in raw.
func bashInput(command string) string {
	encoded, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func display() decisions.DisplayContext {
	return decisions.DisplayContext{ComputerName: "owner-mac", ProjectLabel: "launcher"}
}

func expiry() time.Time { return time.Now().Add(5 * time.Minute) }

func newOwnerWithSession(t *testing.T) (*Owner, *fakeSession) {
	t.Helper()
	owner := NewOwner(nil)
	session := &fakeSession{}
	owner.AttachSession(taskID, session)
	return owner, session
}

func TestBashApprovalBecomesACommandDecision(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"go test ./..."}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if request.Kind != decisions.KindCommand || request.Command != "go test ./..." || !request.CommandUnderstandable {
		t.Fatalf("request = %#v", request)
	}
	if request.ThreadID != taskID || request.ComputerName != "owner-mac" || request.ProjectLabel != "launcher" {
		t.Fatalf("request context = %#v", request)
	}
}

// The launcher already refuses to let an unreadable command be approved; that
// protection must apply to Claude Code's commands too.
func TestUnreadableCommandCannotBeApproved(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, bashInput("printf '\x01\x02'")), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if request.CommandUnderstandable {
		t.Fatalf("a command with control characters was marked understandable: %q", request.Command)
	}
	if request.Command == "" {
		t.Fatal("command was left empty rather than marked unavailable")
	}
}

func TestCommandSecretsAreRedactedBeforeReachingThePhone(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"deploy --api-token=super-secret-value"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if strings.Contains(request.Command, "super-secret-value") {
		t.Fatalf("secret reached the phone: %q", request.Command)
	}
}

func TestFileToolsBecomeFileDecisionsWithTheAffectedPath(t *testing.T) {
	for _, tool := range []string{turn.ToolEdit, turn.ToolWrite, turn.ToolNotebookEdit} {
		owner, _ := newOwnerWithSession(t)
		input := `{"file_path":"/Users/owner/work/launcher/main.go"}`
		if tool == turn.ToolNotebookEdit {
			input = `{"notebook_path":"/Users/owner/work/launcher/nb.ipynb"}`
		}
		request, err := owner.Register(taskID, canUseTool(tool, input), display(), expiry())
		if err != nil {
			t.Fatalf("Register(%s) error = %v", tool, err)
		}
		if request.Kind != decisions.KindFile || len(request.AffectedPaths) != 1 {
			t.Fatalf("%s request = %#v", tool, request)
		}
	}
}

// A Claude Code MCP tool call is a normal tool the owner can allow. Mapping it
// to the Codex MCP-elicitation kind would only ever offer decline.
func TestConnectedToolsCanStillBeAllowed(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool("mcp__playwright__browser_click", `{"selector":"#go"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if request.Kind == decisions.KindMCP {
		t.Fatal("a connected tool call was mapped to MCP elicitation, which cannot be allowed")
	}
	if request.Kind != decisions.KindPermissions {
		t.Fatalf("kind = %q", request.Kind)
	}
	allowed := false
	for _, decision := range request.AllowedDecisions {
		allowed = allowed || decision == decisions.DecisionAcceptOnce
	}
	if !allowed {
		t.Fatalf("connected tool cannot be allowed: %#v", request.AllowedDecisions)
	}
	if strings.Contains(request.Access, "#go") {
		t.Fatalf("tool input leaked into the description: %q", request.Access)
	}
}

func TestAllowEchoesTheApprovedInputBack(t *testing.T) {
	owner, session := newOwnerWithSession(t)
	input := `{"command":"go build ./..."}`
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, input), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	err = owner.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptOnce,
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	payload := inner(t, session.last(t))
	if payload["behavior"] != "allow" {
		t.Fatalf("payload = %v", payload)
	}
	// The phone approved the call it was shown, so the input must go back
	// unchanged rather than being substituted.
	updated, err := json.Marshal(payload["updatedInput"])
	if err != nil || !json.Valid(updated) {
		t.Fatalf("updatedInput = %v", payload["updatedInput"])
	}
	var got, want map[string]any
	_ = json.Unmarshal(updated, &got)
	_ = json.Unmarshal([]byte(input), &want)
	if got["command"] != want["command"] {
		t.Fatalf("updatedInput = %v; want %v", got, want)
	}
	// A single-shot allow must not also grant session-wide permission.
	if _, present := payload["updatedPermissions"]; present {
		t.Fatal("accept once granted session-wide permissions")
	}
}

func TestAcceptForSessionReplaysTheCLIsOwnSuggestions(t *testing.T) {
	owner, session := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	err = owner.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptSession,
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	payload := inner(t, session.last(t))
	if payload["behavior"] != "allow" {
		t.Fatalf("payload = %v", payload)
	}
	// Replaying these is what stops the CLI asking about the same tool again.
	if _, present := payload["updatedPermissions"]; !present {
		t.Fatalf("accept for session did not carry permissions: %v", payload)
	}
}

func TestDeclineDeniesWithAReasonTheModelCanRead(t *testing.T) {
	for _, decision := range []decisions.Decision{decisions.DecisionDecline, decisions.DecisionCancel} {
		owner, session := newOwnerWithSession(t)
		request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"rm -rf /"}`), display(), expiry())
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		err = owner.Respond(context.Background(), decisions.Response{
			RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
			Kind: request.Kind, Decision: decision,
		})
		if err != nil {
			t.Fatalf("Respond(%s) error = %v", decision, err)
		}
		payload := inner(t, session.last(t))
		if payload["behavior"] != "deny" {
			t.Fatalf("%s payload = %v", decision, payload)
		}
		if message, _ := payload["message"].(string); message == "" {
			t.Fatalf("%s carried no reason for the model", decision)
		}
	}
}

func TestAskUserQuestionBecomesAQuestionDecision(t *testing.T) {
	owner, session := newOwnerWithSession(t)
	input := `{"questions":[{"question":"Which database?","header":"Database","multiSelect":false,"options":[{"label":"Postgres"},{"label":"SQLite"}]}]}`
	request, err := owner.Register(taskID, canUseTool(turn.ToolAskUserQuestion, input), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if request.Kind != decisions.KindQuestion || len(request.Questions) != 1 {
		t.Fatalf("request = %#v", request)
	}
	question := request.Questions[0]
	if question.Prompt != "Which database?" || question.Header != "Database" {
		t.Fatalf("question = %#v", question)
	}
	if len(question.Options) != 2 || question.Options[0] != "Postgres" {
		t.Fatalf("options = %#v", question.Options)
	}
	// A question offers answers, never decisions, or the router rejects it.
	if len(request.AllowedDecisions) != 0 {
		t.Fatalf("question offered decisions: %#v", request.AllowedDecisions)
	}

	err = owner.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: decisions.KindQuestion, Answers: map[string][]string{question.ID: {"Postgres"}},
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	payload := inner(t, session.last(t))
	if payload["behavior"] != "allow" {
		t.Fatalf("answer payload = %v", payload)
	}
	if !strings.Contains(string(session.frames[0]), "Postgres") {
		t.Fatalf("answer did not carry the selection: %s", session.frames[0])
	}
}

func TestQuestionWithNoAnswerIsRejected(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	input := `{"questions":[{"question":"Which?","header":"Pick","options":[{"label":"A"}]}]}`
	request, err := owner.Register(taskID, canUseTool(turn.ToolAskUserQuestion, input), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	err = owner.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: decisions.KindQuestion,
	})
	if err == nil {
		t.Fatal("an empty answer was accepted")
	}
}

func TestMalformedQuestionInputIsRejected(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	for _, input := range []string{`{}`, `{"questions":[]}`, `{"questions":[{"question":"  "}]}`, `not json`} {
		if _, err := owner.Register(taskID, canUseTool(turn.ToolAskUserQuestion, input), display(), expiry()); err == nil {
			t.Fatalf("Register(%q) was accepted", input)
		}
	}
}

func TestOnlyApprovalRequestsAreRegistered(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	for _, subtype := range []string{streamjson.SubtypeHookCallback, streamjson.SubtypeMCPMessage, streamjson.SubtypeInitialize} {
		request := canUseTool(turn.ToolBash, `{"command":"ls"}`)
		request.Subtype = subtype
		if _, err := owner.Register(taskID, request, display(), expiry()); !errors.Is(err, ErrNotApproval) {
			t.Fatalf("Register(%s) error = %v; want ErrNotApproval", subtype, err)
		}
	}
}

func TestRespondRejectsAResponseThatDoesNotMatchItsRequest(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	base := decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptOnce,
	}
	mismatches := map[string]func(*decisions.Response){
		"unknown request": func(r *decisions.Response) { r.RequestID = "cc-nope-nope" },
		"wrong task":      func(r *decisions.Response) { r.ThreadID = "22222222-2222-2222-2222-222222222222" },
		"wrong kind":      func(r *decisions.Response) { r.Kind = decisions.KindFile },
	}
	for name, mutate := range mismatches {
		response := base
		mutate(&response)
		if err := owner.Respond(context.Background(), response); !errors.Is(err, decisions.ErrOwnerUnavailable) {
			t.Fatalf("%s: error = %v; want ErrOwnerUnavailable", name, err)
		}
	}
}

// A decision cannot be delivered twice: the CLI would see a second answer to a
// request it has already moved past.
func TestARequestCanOnlyBeAnsweredOnce(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	response := decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptOnce,
	}
	if err := owner.Respond(context.Background(), response); err != nil {
		t.Fatalf("first Respond() error = %v", err)
	}
	if err := owner.Respond(context.Background(), response); !errors.Is(err, decisions.ErrOwnerUnavailable) {
		t.Fatalf("second Respond() error = %v; want ErrOwnerUnavailable", err)
	}
}

// When the child is gone the turn it was blocking is gone too, so the decision
// must be reported as undeliverable rather than silently dropped.
func TestRespondWithoutALiveSessionIsUnavailable(t *testing.T) {
	owner := NewOwner(nil)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	err = owner.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptOnce,
	})
	if !errors.Is(err, decisions.ErrOwnerUnavailable) {
		t.Fatalf("Respond() error = %v; want ErrOwnerUnavailable", err)
	}
}

func TestAClosedStreamIsReportedAsUnavailable(t *testing.T) {
	owner := NewOwner(nil)
	session := &fakeSession{err: streamjson.ErrClosed}
	owner.AttachSession(taskID, session)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	err = owner.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptOnce,
	})
	if !errors.Is(err, decisions.ErrOwnerUnavailable) {
		t.Fatalf("Respond() error = %v; want ErrOwnerUnavailable", err)
	}
}

// Approvals for a finished session can never be delivered, so leaving them
// pending would show the owner sheets that do nothing.
func TestDetachSessionAbandonsItsPendingApprovals(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	abandoned := owner.DetachSession(taskID)
	if len(abandoned) != 1 || abandoned[0] != request.ID {
		t.Fatalf("abandoned = %#v; want %q", abandoned, request.ID)
	}
	if _, found := owner.PendingCLIRequestID(request.ID); found {
		t.Fatal("approval survived its session")
	}
}

// A request nobody can answer must be failed explicitly, or the CLI's turn
// hangs forever.
func TestFailUnblocksTheCLI(t *testing.T) {
	owner, session := newOwnerWithSession(t)
	if _, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry()); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := owner.Fail(context.Background(), taskID, "cli-request-1", "companion cannot serve this"); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.failures) != 1 || !strings.Contains(session.failures[0], "cli-request-1") {
		t.Fatalf("failures = %#v", session.failures)
	}
}

func TestFailWithoutALiveSessionReportsIt(t *testing.T) {
	owner := NewOwner(nil)
	if err := owner.Fail(context.Background(), taskID, "cli-request-1", "gone"); !errors.Is(err, ErrNoLiveSession) {
		t.Fatalf("Fail() error = %v; want ErrNoLiveSession", err)
	}
}

func TestRequestIDsAreStableAndWireSafe(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	first, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	// Registering the same CLI request again must land on the same ID rather
	// than accumulating duplicate sheets.
	second, err := owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil || first.ID != second.ID {
		t.Fatalf("IDs = %q, %q, err = %v", first.ID, second.ID, err)
	}
	// The same CLI request ID under a different task must not collide.
	other, err := owner.Register("22222222-2222-2222-2222-222222222222", canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
	if err != nil || other.ID == first.ID {
		t.Fatalf("cross-task IDs collided: %q", other.ID)
	}
	for _, id := range []string{first.ID, other.ID} {
		if len(id) == 0 || len(id) > 128 || strings.ContainsAny(id, " \r\n/") {
			t.Fatalf("request ID is not wire-safe: %q", id)
		}
	}
}

// The router validates far more strictly than the owner does, and it is what
// actually stands between the phone and the CLI. Registering through it proves
// the produced requests are ones the existing decision path will accept.
func TestRequestsSurviveTheRealRouter(t *testing.T) {
	tests := []struct {
		name     string
		request  streamjson.ControlRequest
		respond  func(decisions.Request) decisions.Response
		wantKind decisions.Kind
	}{
		{
			name:    "command",
			request: canUseTool(turn.ToolBash, `{"command":"go test ./..."}`),
			respond: func(request decisions.Request) decisions.Response {
				return decisions.Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: decisions.DecisionAcceptOnce}
			},
			wantKind: decisions.KindCommand,
		},
		{
			name:    "file",
			request: canUseTool(turn.ToolEdit, `{"file_path":"/Users/owner/work/launcher/main.go"}`),
			respond: func(request decisions.Request) decisions.Response {
				return decisions.Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: decisions.DecisionDecline}
			},
			wantKind: decisions.KindFile,
		},
		{
			name:    "connected tool",
			request: canUseTool("mcp__playwright__browser_click", `{"selector":"#go"}`),
			respond: func(request decisions.Request) decisions.Response {
				return decisions.Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: decisions.DecisionAcceptSession}
			},
			wantKind: decisions.KindPermissions,
		},
		{
			name:    "question",
			request: canUseTool(turn.ToolAskUserQuestion, `{"questions":[{"question":"Which database?","header":"Database","options":[{"label":"Postgres"},{"label":"SQLite"}]}]}`),
			respond: func(request decisions.Request) decisions.Response {
				return decisions.Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Answers: map[string][]string{request.Questions[0].ID: {"Postgres"}}}
			},
			wantKind: decisions.KindQuestion,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			owner, session := newOwnerWithSession(t)
			router := decisions.NewRouter(owner, nil)

			request, err := owner.Register(taskID, test.request, display(), expiry())
			if err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			if request.Kind != test.wantKind {
				t.Fatalf("kind = %q; want %q", request.Kind, test.wantKind)
			}
			if err := router.Add(request); err != nil {
				t.Fatalf("router rejected the request: %v", err)
			}
			if pending := router.Pending(taskID); len(pending) != 1 {
				t.Fatalf("router pending = %#v", pending)
			}
			if err := router.Respond(context.Background(), test.respond(request), time.Now()); err != nil {
				t.Fatalf("router Respond() error = %v", err)
			}
			if len(session.frames) != 1 {
				t.Fatalf("answers written to the CLI = %d; want 1", len(session.frames))
			}
			// The router drops a delivered request, so the sheet cannot linger.
			if pending := router.Pending(taskID); len(pending) != 0 {
				t.Fatalf("request survived delivery: %#v", pending)
			}
		})
	}
}

// The router refuses a decision the request never offered, which is what stops
// a phone from allowing a command the companion could not display.
func TestRouterRefusesADecisionThatWasNotOffered(t *testing.T) {
	owner, session := newOwnerWithSession(t)
	router := decisions.NewRouter(owner, nil)
	request, err := owner.Register(taskID, canUseTool(turn.ToolBash, bashInput("printf '\x01\x02'")), display(), expiry())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if request.CommandUnderstandable {
		t.Fatal("a command with control characters was marked understandable")
	}
	if err := router.Add(request); err != nil {
		t.Fatalf("router rejected the request: %v", err)
	}
	err = router.Respond(context.Background(), decisions.Response{
		RequestID: request.ID, ThreadID: taskID, TurnID: request.TurnID, ItemID: request.ItemID,
		Kind: request.Kind, Decision: decisions.DecisionAcceptOnce,
	}, time.Now())
	if !errors.Is(err, decisions.ErrDecisionNotOffered) {
		t.Fatalf("Respond() error = %v; want ErrDecisionNotOffered", err)
	}
	if len(session.frames) != 0 {
		t.Fatalf("an unreadable command was allowed anyway: %s", session.frames[0])
	}
}

func TestOwnerIsSafeUnderConcurrentUse(t *testing.T) {
	owner, _ := newOwnerWithSession(t)
	var group sync.WaitGroup
	group.Add(3)
	go func() {
		defer group.Done()
		for range 100 {
			owner.Register(taskID, canUseTool(turn.ToolBash, `{"command":"ls"}`), display(), expiry())
		}
	}()
	go func() {
		defer group.Done()
		for range 100 {
			owner.PendingCLIRequestID("cc-anything-anything")
		}
	}()
	go func() {
		defer group.Done()
		for range 100 {
			owner.AttachSession(taskID, &fakeSession{})
		}
	}()
	group.Wait()
}
