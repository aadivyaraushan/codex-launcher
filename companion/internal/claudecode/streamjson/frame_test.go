package streamjson

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Captured verbatim from claude 2.1.153 driven with
// `-p --input-format stream-json --output-format stream-json --verbose`.
const (
	realSystemInit = `{"type":"system","subtype":"init","cwd":"/tmp/probe","session_id":"f470391b-60d9-4dc5-8aed-9904549aafcb","tools":["Task","Bash","Edit"],"mcp_servers":[{"name":"playwright","status":"pending"}],"model":"claude-sonnet-4-6","permissionMode":"default","apiKeySource":"none","claude_code_version":"2.1.153","output_style":"default"}`
	realToolUse    = `{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_011CdWnB1kwuRCzZABVyViyU","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_01JaqnRjsyk493n79eA4tUQ1","name":"Bash","input":{"command":"echo hello-from-probe","description":"Print a greeting"},"caller":{"type":"direct"}}],"stop_reason":null},"session_id":"f470391b-60d9-4dc5-8aed-9904549aafcb"}`
	realToolResult = `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01JaqnRjsyk493n79eA4tUQ1","type":"tool_result","content":"hello-from-probe","is_error":false}]},"parent_tool_use_id":null,"session_id":"f470391b-60d9-4dc5-8aed-9904549aafcb"}`
	realText       = `{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_011CdWnBH2Do8gMpkVirfUhH","type":"message","role":"assistant","content":[{"type":"text","text":"DONE"}],"stop_reason":null},"session_id":"f470391b-60d9-4dc5-8aed-9904549aafcb"}`
	realThinking   = `{"type":"assistant","message":{"model":"claude-sonnet-4-6","id":"msg_011CdWnDRprg9CUa87NeGDN7","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"The user wants me to run a bash command.","signature":"Eq8CCos"}],"stop_reason":null},"session_id":"16542dc5-6d67-4323-90ff-ec271187d828"}`
	realCanUseTool = `{"type":"control_request","request_id":"c0783bf9-84ec-429f-bc4f-5e418baf6eee","request":{"subtype":"can_use_tool","tool_name":"Bash","display_name":"Bash","input":{"command":"rm -rf /tmp/nonexistent-probe-dir","description":"Remove nonexistent directory"},"description":"Remove nonexistent directory","permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash"}]}]}}`
	realResult     = `{"type":"result","subtype":"success","is_error":false,"duration_ms":4882,"num_turns":2,"result":"DONE","stop_reason":"end_turn","session_id":"f470391b-60d9-4dc5-8aed-9904549aafcb","total_cost_usd":0.0252}`
	realInitAnswer = `{"type":"control_response","response":{"subtype":"success","request_id":"companion-1","response":{"commands":[],"output_style":"default"}}}`
	realRateLimit  = `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1785364800},"session_id":"f470391b-60d9-4dc5-8aed-9904549aafcb"}`
)

func decode(t *testing.T, line string) Frame {
	t.Helper()
	frame, err := DecodeFrame([]byte(line))
	if err != nil {
		t.Fatalf("DecodeFrame(%.40s) error = %v", line, err)
	}
	return frame
}

func TestDecodeSystemInitCarriesTheAuthoritativeSessionID(t *testing.T) {
	initFrame, err := decode(t, realSystemInit).SystemInit()
	if err != nil {
		t.Fatalf("SystemInit() error = %v", err)
	}
	if initFrame.SessionID != "f470391b-60d9-4dc5-8aed-9904549aafcb" {
		t.Fatalf("session ID = %q", initFrame.SessionID)
	}
	if initFrame.CWD != "/tmp/probe" || initFrame.Model != "claude-sonnet-4-6" || initFrame.PermissionMode != "default" {
		t.Fatalf("system init = %#v", initFrame)
	}
	if initFrame.Version != "2.1.153" {
		t.Fatalf("version = %q", initFrame.Version)
	}
}

func TestDecodeAssistantToolUse(t *testing.T) {
	message, err := decode(t, realToolUse).AssistantMessage()
	if err != nil {
		t.Fatalf("AssistantMessage() error = %v", err)
	}
	if len(message.Content) != 1 {
		t.Fatalf("content blocks = %d", len(message.Content))
	}
	block := message.Content[0]
	if block.Type != "tool_use" || block.Name != "Bash" || block.ID != "toolu_01JaqnRjsyk493n79eA4tUQ1" {
		t.Fatalf("block = %#v", block)
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(block.Input, &input); err != nil || input.Command != "echo hello-from-probe" {
		t.Fatalf("tool input = %q, %v", input.Command, err)
	}
}

func TestDecodeAssistantTextAndThinking(t *testing.T) {
	text, err := decode(t, realText).AssistantMessage()
	if err != nil || len(text.Content) != 1 || text.Content[0].Type != "text" || text.Content[0].Text != "DONE" {
		t.Fatalf("text message = %#v, %v", text, err)
	}
	thinking, err := decode(t, realThinking).AssistantMessage()
	if err != nil || len(thinking.Content) != 1 || thinking.Content[0].Type != "thinking" {
		t.Fatalf("thinking message = %#v, %v", thinking, err)
	}
	if !strings.HasPrefix(thinking.Content[0].Thinking, "The user wants") {
		t.Fatalf("thinking text = %q", thinking.Content[0].Thinking)
	}
}

func TestDecodeToolResultUserFrame(t *testing.T) {
	message, err := decode(t, realToolResult).UserMessage()
	if err != nil {
		t.Fatalf("UserMessage() error = %v", err)
	}
	if message.ParentToolUseID != "" {
		t.Fatalf("top-level frame reported a parent tool use: %q", message.ParentToolUseID)
	}
	if len(message.Content) != 1 || message.Content[0].Type != "tool_result" || message.Content[0].IsError {
		t.Fatalf("content = %#v", message.Content)
	}
	if message.Content[0].ToolUseID != "toolu_01JaqnRjsyk493n79eA4tUQ1" {
		t.Fatalf("tool_use_id = %q", message.Content[0].ToolUseID)
	}
}

func TestDecodeCanUseToolCarriesWhatTheApprovalSheetNeeds(t *testing.T) {
	request, err := decode(t, realCanUseTool).ControlRequest()
	if err != nil {
		t.Fatalf("ControlRequest() error = %v", err)
	}
	if request.Subtype != SubtypeCanUseTool || request.ToolName != "Bash" {
		t.Fatalf("request = %#v", request)
	}
	if request.RequestID != "c0783bf9-84ec-429f-bc4f-5e418baf6eee" {
		t.Fatalf("request ID = %q", request.RequestID)
	}
	// "Allow for this session" replays these back, so they must survive decoding.
	if len(request.Suggestions) == 0 {
		t.Fatal("permission suggestions were dropped")
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil || input.Command == "" {
		t.Fatalf("tool input = %q, %v", input.Command, err)
	}
}

func TestDecodeResultAndControlResponse(t *testing.T) {
	result, err := decode(t, realResult).Result()
	if err != nil || result.Subtype != ResultSuccess || result.IsError || result.Result != "DONE" {
		t.Fatalf("result = %#v, %v", result, err)
	}
	answer, err := decode(t, realInitAnswer).ControlResponse()
	if err != nil || answer.RequestID != "companion-1" || answer.Failed() {
		t.Fatalf("control response = %#v, %v", answer, err)
	}
}

func TestDecodeKeepsUnmodelledFramesInsteadOfFailing(t *testing.T) {
	frame := decode(t, realRateLimit)
	if frame.Type != TypeRateLimitEvent {
		t.Fatalf("frame type = %q", frame.Type)
	}
}

func TestDecodeRejectsFramesThatAreNotProtocolJSON(t *testing.T) {
	tests := []string{``, `not json`, `{"subtype":"init"}`, `[1,2,3]`, `{"type":""}`, `{"type":"system","session_id":"has\nnewline"}`}
	for _, line := range tests {
		if _, err := DecodeFrame([]byte(line)); !errors.Is(err, ErrFrameMalformed) {
			t.Fatalf("DecodeFrame(%q) error = %v; want ErrFrameMalformed", line, err)
		}
	}
}

func TestDecodeRejectsAnOversizedFrame(t *testing.T) {
	oversized := make([]byte, MaxFrameBytes+1)
	if _, err := DecodeFrame(oversized); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized frame error = %v; want ErrFrameTooLarge", err)
	}
}

func TestControlRequestRejectsAnUnaddressableFrame(t *testing.T) {
	// Without a request_id the companion could never answer, which would hang
	// the CLI's turn; that must be an error, not a silently kept request.
	line := `{"type":"control_request","request":{"subtype":"can_use_tool","tool_name":"Bash"}}`
	if _, err := decode(t, line).ControlRequest(); !errors.Is(err, ErrFrameMalformed) {
		t.Fatalf("error = %v; want ErrFrameMalformed", err)
	}
}

func TestAllowEchoesTheApprovedInputBack(t *testing.T) {
	input := json.RawMessage(`{"command":"echo hi"}`)
	frame, err := Allow("req-1", input, nil)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	var decoded struct {
		Type     string `json:"type"`
		Response struct {
			Subtype   string `json:"subtype"`
			RequestID string `json:"request_id"`
			Response  struct {
				Behavior     string          `json:"behavior"`
				UpdatedInput json.RawMessage `json:"updatedInput"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(frame, &decoded); err != nil {
		t.Fatalf("Allow() produced undecodable JSON: %v", err)
	}
	if decoded.Type != TypeControlResponse || decoded.Response.RequestID != "req-1" || decoded.Response.Response.Behavior != "allow" {
		t.Fatalf("allow frame = %s", frame)
	}
	// The phone approves the call the model asked for; substituting a different
	// input would make the approval sheet a lie.
	if string(decoded.Response.Response.UpdatedInput) != string(input) {
		t.Fatalf("updatedInput = %s; want %s", decoded.Response.Response.UpdatedInput, input)
	}
}

func TestAllowCarriesSessionScopedPermissions(t *testing.T) {
	suggestions := json.RawMessage(`[{"type":"addRules","rules":[{"toolName":"Bash"}]}]`)
	frame, err := Allow("req-1", json.RawMessage(`{}`), suggestions)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !strings.Contains(string(frame), `"updatedPermissions"`) {
		t.Fatalf("allow frame dropped permissions: %s", frame)
	}
}

func TestDenyAlwaysCarriesAModelVisibleReason(t *testing.T) {
	frame, err := Deny("req-1", "   ")
	if err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	var decoded struct {
		Response struct {
			Response struct {
				Behavior string `json:"behavior"`
				Message  string `json:"message"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(frame, &decoded); err != nil {
		t.Fatalf("Deny() produced undecodable JSON: %v", err)
	}
	if decoded.Response.Response.Behavior != "deny" || decoded.Response.Response.Message == "" {
		t.Fatalf("deny frame = %s", frame)
	}
}

func TestOutboundFramesRejectUnaddressableIDs(t *testing.T) {
	for _, id := range []string{"", "has\nnewline", " leading", strings.Repeat("x", MaxIDBytes+1)} {
		if _, err := Allow(id, nil, nil); err == nil {
			t.Fatalf("Allow(%q) was accepted", id)
		}
		if _, err := Deny(id, "no"); err == nil {
			t.Fatalf("Deny(%q) was accepted", id)
		}
		if _, err := Request(id, SubtypeInterrupt, nil); err == nil {
			t.Fatalf("Request(%q) was accepted", id)
		}
	}
}

func TestUserTextRejectsEmptyAndOversizedPrompts(t *testing.T) {
	if _, err := UserText("   "); !errors.Is(err, ErrFrameMalformed) {
		t.Fatalf("empty prompt error = %v", err)
	}
	if _, err := UserText(strings.Repeat("x", MaxTextBytes+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized prompt error = %v", err)
	}
	frame, err := UserText("hello")
	if err != nil {
		t.Fatalf("UserText() error = %v", err)
	}
	if !strings.Contains(string(frame), `"role":"user"`) || !strings.Contains(string(frame), `"hello"`) {
		t.Fatalf("user frame = %s", frame)
	}
}

// Every outbound frame is written as one line, so none may contain a raw
// newline that would split it into two frames on the wire.
func TestOutboundFramesAreSingleLine(t *testing.T) {
	frames := make([]json.RawMessage, 0, 5)
	for _, build := range []func() (json.RawMessage, error){
		func() (json.RawMessage, error) { return UserText("line one\nline two") },
		func() (json.RawMessage, error) { return Allow("req-1", json.RawMessage(`{"a":"b\nc"}`), nil) },
		func() (json.RawMessage, error) { return Deny("req-1", "no\nreally") },
		func() (json.RawMessage, error) { return ErrorResponse("req-1", "bad\nnews") },
		func() (json.RawMessage, error) { return Request("req-1", SubtypeInterrupt, nil) },
	} {
		frame, err := build()
		if err != nil {
			t.Fatalf("build error = %v", err)
		}
		frames = append(frames, frame)
	}
	for _, frame := range frames {
		if strings.ContainsAny(string(frame), "\n\r") {
			t.Fatalf("frame contains a raw newline: %s", frame)
		}
	}
}
