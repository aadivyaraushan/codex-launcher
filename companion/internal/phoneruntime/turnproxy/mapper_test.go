package turnproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

const (
	testTaskID     = "phone-agent"
	testSessionKey = "agent:main:main"
)

// goldenTranscript mirrors the testdata files: a sequence of gateway "chat"
// event payloads in, the flattened MobileEvents the phone must see out.
type goldenTranscript struct {
	Note   string             `json:"note"`
	Events []ChatEventPayload `json:"events"`
	Want   []goldenEvent      `json:"want"`
}

type goldenEvent struct {
	TaskID     string `json:"taskId"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	Summary    string `json:"summary"`
	StartsTurn bool   `json:"startsTurn"`
}

func TestGoldenTranscripts(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatalf("glob testdata: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no golden transcripts found in testdata/")
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var transcript goldenTranscript
			if err := json.Unmarshal(raw, &transcript); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}

			mapper := NewTurnMapper(testTaskID, testSessionKey)
			var got []taskstate.MobileEvent
			for _, payload := range transcript.Events {
				got = append(got, mapper.Apply(payload)...)
			}

			if len(got) != len(transcript.Want) {
				t.Fatalf("emitted %d events, want %d\ngot: %+v", len(got), len(transcript.Want), got)
			}
			for i, want := range transcript.Want {
				event := got[i]
				if event.TaskID != want.TaskID {
					t.Errorf("event %d TaskID = %q, want %q", i, event.TaskID, want.TaskID)
				}
				if event.Kind != want.Kind {
					t.Errorf("event %d Kind = %q, want %q", i, event.Kind, want.Kind)
				}
				if string(event.State) != want.State {
					t.Errorf("event %d State = %q, want %q", i, event.State, want.State)
				}
				if event.Summary != want.Summary {
					t.Errorf("event %d Summary = %q, want %q", i, event.Summary, want.Summary)
				}
				if event.StartsTurn != want.StartsTurn {
					t.Errorf("event %d StartsTurn = %v, want %v", i, event.StartsTurn, want.StartsTurn)
				}
			}
		})
	}
}

// The mobile contract refuses summaries over 512 characters
// (contract/validation.go safeDisplayString), so a long reply must arrive
// already trimmed — a publish the contract rejects would drop the whole
// reply event.
func TestReplySummaryIsTruncatedToContractLimit(t *testing.T) {
	long := strings.Repeat("a", 600) + "!"
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.Apply(ChatEventPayload{State: "delta", DeltaText: long, RunID: "run-1", SessionKey: testSessionKey})
	events := mapper.Apply(ChatEventPayload{State: "final", RunID: "run-1", SessionKey: testSessionKey})

	if len(events) != 1 {
		t.Fatalf("final must emit exactly the reply, got %+v", events)
	}
	summary := events[0].Summary
	if utf8.RuneCountInString(summary) > 512 {
		t.Fatalf("summary is %d runes, contract caps display strings at 512", utf8.RuneCountInString(summary))
	}
	if !strings.HasPrefix(summary, strings.Repeat("a", 100)) {
		t.Fatalf("truncation must keep the front of the reply, got %q", summary[:50])
	}
}

// Wire safeDisplayString rejects unicode control characters (including
// newlines). A pretty-printed JSON reply that keeps those runes would make
// PublishTaskEvent return "mobile task event is invalid" and leave Operator
// stuck on Working.
func TestReplySummaryCollapsesNewlinesAndControlChars(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "pretty json", text: "{\n  \"ok\": true\n}", want: "{ \"ok\": true }"},
		{name: "crlf", text: "line one\r\nline two", want: "line one line two"},
		{name: "tab", text: "left\tright", want: "left right"},
		{name: "null byte among words", text: "keep\x00me", want: "keep me"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapper := NewTurnMapper(testTaskID, testSessionKey)
			mapper.Apply(ChatEventPayload{State: "delta", DeltaText: test.text, RunID: "run-1", SessionKey: testSessionKey})
			events := mapper.Apply(ChatEventPayload{State: "final", RunID: "run-1", SessionKey: testSessionKey})
			if len(events) != 1 {
				t.Fatalf("final must emit exactly the reply, got %+v", events)
			}
			summary := events[0].Summary
			if summary != test.want {
				t.Fatalf("summary = %q, want %q", summary, test.want)
			}
			if strings.IndexFunc(summary, func(r rune) bool { return r < 0x20 }) >= 0 {
				t.Fatalf("summary still has a control rune: %q", summary)
			}
		})
	}
}

func TestErrorSummaryCollapsesNewlines(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.Apply(ChatEventPayload{State: "delta", DeltaText: "working", RunID: "run-1", SessionKey: testSessionKey})
	events := mapper.Apply(ChatEventPayload{
		State: "error", ErrorMessage: "provider said:\nrate limited", RunID: "run-1", SessionKey: testSessionKey,
	})
	if len(events) != 1 {
		t.Fatalf("error must emit exactly the failure, got %+v", events)
	}
	if events[0].Summary != "provider said: rate limited" {
		t.Fatalf("summary = %q", events[0].Summary)
	}
}

func TestReplyOfOnlyControlCharsFallsBackToGenericSummary(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.Apply(ChatEventPayload{State: "delta", DeltaText: "\n\t\r", RunID: "run-1", SessionKey: testSessionKey})
	events := mapper.Apply(ChatEventPayload{State: "final", RunID: "run-1", SessionKey: testSessionKey})
	if len(events) != 1 || events[0].Summary != "Codex replied" {
		t.Fatalf("control-only reply must still publish a safe fallback, got %+v", events)
	}
}

// A run on a foreign session must not disturb the active run's accumulation.
func TestForeignSessionDoesNotDisturbActiveRun(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.Apply(ChatEventPayload{State: "delta", DeltaText: "mine", RunID: "run-1", SessionKey: testSessionKey})
	if events := mapper.Apply(ChatEventPayload{State: "delta", DeltaText: "yours", RunID: "run-z", SessionKey: "agent:other:main"}); len(events) != 0 {
		t.Fatalf("foreign session emitted %+v", events)
	}
	events := mapper.Apply(ChatEventPayload{State: "final", RunID: "run-1", SessionKey: testSessionKey})
	if len(events) != 1 || events[0].Summary != "mine" {
		t.Fatalf("active run must assemble only its own deltas, got %+v", events)
	}
}

func TestThinkingAgentEventsEmitUpdatingWorkingSummariesBeforeReply(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	first := mapper.ApplyAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-1", SessionKey: testSessionKey,
		Data: AgentEventData{Text: "Checking the calendar", Delta: "Checking the calendar"},
	})
	if len(first) != 1 || first[0].Kind != "activity" || first[0].State != taskstate.Working || !first[0].StartsTurn {
		t.Fatalf("first thinking event = %+v, want a turn-starting Working activity", first)
	}
	if first[0].Summary != "Checking the calendar" {
		t.Fatalf("first thinking summary = %q, want the safe reasoning text", first[0].Summary)
	}

	second := mapper.ApplyAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-1", SessionKey: testSessionKey,
		Data: AgentEventData{Text: "Checking the calendar then drafting", Delta: " then drafting"},
	})
	if len(second) != 1 || second[0].Kind != "activity" || second[0].State != taskstate.Working || second[0].StartsTurn {
		t.Fatalf("later thinking event = %+v, want Working without StartsTurn", second)
	}
	if second[0].Summary == first[0].Summary {
		t.Fatal("later thinking summary must change so eventpump does not drop it")
	}
	if second[0].Summary != "Checking the calendar then drafting" {
		t.Fatalf("later thinking summary = %q", second[0].Summary)
	}

	reply := mapper.Apply(ChatEventPayload{State: "final", RunID: "run-1", SessionKey: testSessionKey, Message: json.RawMessage(`"pong"`)})
	if len(reply) != 1 || reply[0].Kind != "reply" || reply[0].Summary != "pong" {
		t.Fatalf("final after thinking = %+v, want the reply", reply)
	}
}

func TestThinkingAgentEventsFromOtherSessionsAreIgnored(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	if events := mapper.ApplyAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-z", SessionKey: "agent:other:main",
		Data: AgentEventData{Text: "secret chain of thought"},
	}); len(events) != 0 {
		t.Fatalf("foreign thinking emitted %+v", events)
	}
}

func TestThinkingWithAlternateSessionKeyAttachesToTheActiveRun(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	started := mapper.Apply(ChatEventPayload{State: "delta", DeltaText: "", RunID: "run-1", SessionKey: testSessionKey})
	if len(started) != 1 || !started[0].StartsTurn {
		t.Fatalf("need an in-flight run before the mismatched thinking, got %+v", started)
	}

	events := mapper.ApplyAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-1", SessionKey: "agent:main:phone-other",
		Data: AgentEventData{Text: "Checking the calendar"},
	})
	if len(events) != 1 || events[0].Kind != "activity" || events[0].Summary != "Checking the calendar" {
		t.Fatalf("mismatched sessionKey thinking = %+v, want it attached to the active run", events)
	}

	missingKey := mapper.ApplyAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-1",
		Data: AgentEventData{Text: "Checking the calendar then drafting"},
	})
	if len(missingKey) != 1 || missingKey[0].Summary != "Checking the calendar then drafting" {
		t.Fatalf("missing sessionKey thinking = %+v, want it attached to the active run", missingKey)
	}
}

func TestNormalizeAgentEventAcceptsOpenClawNestedAndTopLevelShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want AgentEventPayload
	}{
		{
			name: "canonical",
			raw:  `{"runId":"run-1","sessionKey":"agent:main:phone-1","stream":"thinking","data":{"text":"Checking","delta":"Checking"}}`,
			want: AgentEventPayload{RunID: "run-1", SessionKey: "agent:main:phone-1", Stream: "thinking", Data: AgentEventData{Text: "Checking", Delta: "Checking"}},
		},
		{
			name: "nested payload",
			raw:  `{"payload":{"runId":"run-1","stream":"thinking","data":{"text":"Nested"}}}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "thinking", Data: AgentEventData{Text: "Nested"}},
		},
		{
			name: "top-level type thinking",
			raw:  `{"type":"thinking","runId":"run-1","text":"Need a shorter path"}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "thinking", Data: AgentEventData{Text: "Need a shorter path"}},
		},
		{
			name: "stream in data",
			raw:  `{"runId":"run-1","data":{"stream":"thinking","text":"Inside data"}}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "thinking", Data: AgentEventData{Text: "Inside data"}},
		},
		{
			name: "item reasoning keeps stream name",
			raw:  `{"runId":"run-1","stream":"item","data":{"item":{"type":"reasoning","summary":["Checking"]}}}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "item", ItemType: "reasoning", Data: AgentEventData{Text: "Checking"}},
		},
		{
			name: "codex_app_server.item reasoning keeps stream name",
			raw:  `{"runId":"run-1","stream":"codex_app_server.item","data":{"type":"reasoning","item":{"type":"reasoning","summary":["Checking"]}}}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "codex_app_server.item", ItemType: "reasoning", DataType: "reasoning", Data: AgentEventData{Text: "Checking"}},
		},
		{
			name: "assistant reply tokens are empty after normalize",
			raw:  `{"runId":"run-1","sessionKey":"agent:main:main","stream":"assistant","data":{"text":"Checking","delta":"Checking"}}`,
			want: AgentEventPayload{RunID: "run-1", SessionKey: "agent:main:main", Stream: "assistant", Data: AgentEventData{}},
		},
		{
			name: "commandExecution keeps the command string",
			raw:  `{"runId":"run-1","stream":"codex_app_server.item","data":{"type":"commandExecution","item":{"type":"commandExecution","command":"ls"}}}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "codex_app_server.item", ItemType: "commandExecution", DataType: "commandExecution", Command: "ls"},
		},
		{
			name: "preamble title is progress text",
			raw:  `{"runId":"run-1","stream":"item","data":{"item":{"type":"preamble","title":"Considering the request"}}}`,
			want: AgentEventPayload{RunID: "run-1", Stream: "item", ItemType: "preamble", Data: AgentEventData{Text: "Considering the request"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := NormalizeAgentEvent(json.RawMessage(test.raw))
			if !ok {
				t.Fatal("NormalizeAgentEvent returned false")
			}
			if got.RunID != test.want.RunID || got.SessionKey != test.want.SessionKey || got.Stream != test.want.Stream {
				t.Fatalf("ids = %+v, want %+v", got, test.want)
			}
			if test.want.ItemType != "" && got.ItemType != test.want.ItemType {
				t.Fatalf("item type = %q, want %q", got.ItemType, test.want.ItemType)
			}
			if test.want.DataType != "" && got.DataType != test.want.DataType {
				t.Fatalf("data type = %q, want %q", got.DataType, test.want.DataType)
			}
			if got.Command != test.want.Command {
				t.Fatalf("command = %q, want %q", got.Command, test.want.Command)
			}
			if got.Data.Text != test.want.Data.Text || got.Data.Delta != test.want.Data.Delta {
				t.Fatalf("data = %+v, want %+v", got.Data, test.want.Data)
			}
		})
	}
}

func TestThinkingSummaryCollapsesNewlinesAndControlChars(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	events := mapper.ApplyAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-1", SessionKey: testSessionKey,
		Data: AgentEventData{Text: "line one\nline two\tkeep"},
	})
	if len(events) != 1 {
		t.Fatalf("thinking must emit Working, got %+v", events)
	}
	if events[0].Summary != "line one line two keep" {
		t.Fatalf("summary = %q", events[0].Summary)
	}
	if strings.IndexFunc(events[0].Summary, func(r rune) bool { return r < 0x20 }) >= 0 {
		t.Fatalf("summary still has a control rune: %q", events[0].Summary)
	}
}

func TestChatMessageThinkingContentUpdatesWorkingBeforeReply(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	message := json.RawMessage(`{"content":[{"type":"thinking","text":"Need a shorter path"},{"type":"text","text":"Done."}]}`)
	working := mapper.Apply(ChatEventPayload{State: "delta", RunID: "run-1", SessionKey: testSessionKey, Message: message})
	if len(working) != 1 || working[0].Kind != "activity" || !working[0].StartsTurn || working[0].Summary != "Need a shorter path" {
		t.Fatalf("chat thinking delta = %+v, want turn-starting Working with reasoning", working)
	}
	reply := mapper.Apply(ChatEventPayload{State: "final", RunID: "run-1", SessionKey: testSessionKey, Message: message})
	if len(reply) != 1 || reply[0].Kind != "reply" || reply[0].Summary != "Done." {
		t.Fatalf("final with thinking+text = %+v, want reply from text content", reply)
	}
}

func TestPixelItemStreamsFillReasoningFromSummaryNotContent(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "item nested reasoning summary strings",
			raw:  `{"runId":"run-1","sessionKey":"agent:main:main","stream":"item","data":{"item":{"id":"rsn-1","type":"reasoning","summary":["Checking the calendar"],"content":["hidden chain of thought"]}}}`,
			want: "Checking the calendar",
		},
		{
			name: "codex_app_server.item nested summary objects",
			raw:  `{"runId":"run-1","sessionKey":"agent:main:main","stream":"codex_app_server.item","data":{"phase":"completed","type":"reasoning","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"then drafting a reply"}],"content":[{"type":"reasoning_text","text":"hidden chain of thought"}]}}}`,
			want: "then drafting a reply",
		},
		{
			name: "nested payload envelope",
			raw:  `{"payload":{"runId":"run-1","stream":"item","data":{"kind":"reasoning","item":{"type":"reasoning","summary":["Need a shorter path"]}}}}`,
			want: "Need a shorter path",
		},
		{
			name: "item kind and summary string",
			raw:  `{"runId":"run-1","sessionKey":"agent:main:main","stream":"item","data":{"kind":"reasoning","summary":"Checking the calendar"}}`,
			want: "Checking the calendar",
		},
		{
			name: "assistant thinking content type",
			raw:  `{"runId":"run-1","sessionKey":"agent:main:main","stream":"assistant","data":{"text":"Done.","content":[{"type":"thinking","text":"Checking the calendar"},{"type":"text","text":"Done."}]}}`,
			want: "Checking the calendar",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, ok := NormalizeAgentEvent(json.RawMessage(test.raw))
			if !ok {
				t.Fatal("NormalizeAgentEvent returned false")
			}
			if payload.ItemType != "reasoning" && payload.ItemType != "thinking" {
				t.Fatalf("item type = %q, want reasoning or thinking", payload.ItemType)
			}
			if strings.Contains(payload.Data.Text, "hidden") || strings.Contains(payload.Data.Delta, "hidden") {
				t.Fatalf("hidden CoT leaked into normalized data: %+v", payload.Data)
			}
			mapper := NewTurnMapper(testTaskID, testSessionKey)
			events := mapper.ApplyAgent(payload)
			if len(events) != 1 || events[0].Kind != "activity" || events[0].State != taskstate.Working || !events[0].StartsTurn {
				t.Fatalf("ApplyAgent = %+v, want turn-starting Working", events)
			}
			if events[0].Summary != test.want {
				t.Fatalf("summary = %q, want %q", events[0].Summary, test.want)
			}
			if mapper.ReasoningDisplay("run-1") != test.want {
				t.Fatalf("ReasoningDisplay = %q, want %q", mapper.ReasoningDisplay("run-1"), test.want)
			}
		})
	}
}

func TestStartRunEmitsWorkingBeforeAnyChatDelta(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	started := mapper.StartRun("run-1")
	if len(started) != 1 || started[0].Kind != "activity" || started[0].State != taskstate.Working || !started[0].StartsTurn || started[0].Summary != genericWorkingSummary {
		t.Fatalf("StartRun = %+v, want StartsTurn Working", started)
	}
	if !mapper.KnowsRun("run-1") {
		t.Fatal("StartRun must remember the run so later thinking can attach")
	}
	if again := mapper.StartRun("run-1"); len(again) != 0 {
		t.Fatalf("second StartRun = %+v, want nothing", again)
	}

	first := mapper.Apply(ChatEventPayload{State: "delta", DeltaText: "Hi", RunID: "run-1", SessionKey: testSessionKey})
	if len(first) != 1 || first[0].StartsTurn || first[0].Summary != "Hi" {
		t.Fatalf("first delta after StartRun = %+v, want updating Working without StartsTurn", first)
	}
	if mapper.ReplyDisplay("run-1") != "Hi" {
		t.Fatalf("ReplyDisplay = %q, want Hi", mapper.ReplyDisplay("run-1"))
	}

	second := mapper.Apply(ChatEventPayload{State: "delta", DeltaText: " there", RunID: "run-1", SessionKey: testSessionKey})
	if len(second) != 1 || second[0].Summary != "Hi there" || second[0].Summary == first[0].Summary {
		t.Fatalf("later delta = %+v, want a changed Working summary", second)
	}
}

func TestTitleOnlyReasoningDoesNotEmitWorkingOrFallbackText(t *testing.T) {
	tests := []string{
		`{"runId":"run-1","sessionKey":"agent:main:main","stream":"item","data":{"itemId":"rsn-1","phase":"start","kind":"analysis","title":"Reasoning","status":"running"}}`,
		`{"runId":"run-1","sessionKey":"agent:main:main","stream":"codex_app_server.item","data":{"phase":"started","itemId":"rsn-1","type":"reasoning"}}`,
	}
	for _, raw := range tests {
		payload, ok := NormalizeAgentEvent(json.RawMessage(raw))
		if !ok {
			t.Fatalf("NormalizeAgentEvent returned false for %s", raw)
		}
		if payload.ItemType != "reasoning" {
			t.Fatalf("item type = %q, want reasoning so logs can say item_type=reasoning", payload.ItemType)
		}
		if payload.Data.Text != "" || payload.Data.Delta != "" {
			t.Fatalf("title-only data = %+v, want empty (no Reasoning fallback)", payload.Data)
		}
		if !payload.carriesReasoning() {
			t.Fatal("title-only reasoning must still carry the reasoning enum so drop_reason is no_text, not not_reasoning")
		}
		mapper := NewTurnMapper(testTaskID, testSessionKey)
		mapper.StartRun("run-1")
		if events := mapper.ApplyAgent(payload); len(events) != 0 {
			t.Fatalf("ApplyAgent = %+v, want no KindReasoning Working from a title-only item", events)
		}
		if got := mapper.ReasoningDisplay("run-1"); got != "" {
			t.Fatalf("ReasoningDisplay = %q, want empty", got)
		}
	}
}

func TestAssistantReplyTokensAreNotChainOfThought(t *testing.T) {
	payload, ok := NormalizeAgentEvent(json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:main","stream":"assistant","data":{"text":"Checking","delta":"Checking"}}`))
	if !ok {
		t.Fatal("NormalizeAgentEvent returned false")
	}
	if payload.Data.Text != "" || payload.Data.Delta != "" {
		t.Fatalf("normalized assistant reply data = %+v, want empty after normalize", payload.Data)
	}
	if payload.carriesReasoning() {
		t.Fatal("assistant reply tokens must drop as not_reasoning")
	}

	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.StartRun("run-1")
	if events := mapper.ApplyAgent(payload); len(events) != 0 {
		t.Fatalf("ApplyAgent = %+v, want no reasoning from assistant reply tokens", events)
	}
	if got := mapper.ReasoningDisplay("run-1"); got != "" {
		t.Fatalf("ReasoningDisplay = %q, want empty", got)
	}
}

func TestItemSummaryBecomesReasoningAndTitleOnlyDoesNotOverwriteIt(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.StartRun("run-1")

	title, ok := NormalizeAgentEvent(json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:main","stream":"item","data":{"kind":"analysis","title":"Reasoning"}}`))
	if !ok {
		t.Fatal("NormalizeAgentEvent returned false")
	}
	if events := mapper.ApplyAgent(title); len(events) != 0 {
		t.Fatalf("title-only = %+v, want no Working", events)
	}

	summary, ok := NormalizeAgentEvent(json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:main","stream":"item","data":{"item":{"type":"reasoning","summary":["Checking the calendar"],"content":["hidden chain of thought"]}}}`))
	if !ok {
		t.Fatal("NormalizeAgentEvent returned false")
	}
	if strings.Contains(summary.Data.Text, "hidden") {
		t.Fatal("hidden CoT leaked into normalized data")
	}
	events := mapper.ApplyAgent(summary)
	if len(events) != 1 || events[0].Summary != "Checking the calendar" || events[0].StartsTurn {
		t.Fatalf("summary = %+v, want updating Working from the real summary", events)
	}
	if mapper.ReasoningDisplay("run-1") != "Checking the calendar" {
		t.Fatalf("ReasoningDisplay = %q", mapper.ReasoningDisplay("run-1"))
	}

	if events := mapper.ApplyAgent(title); len(events) != 0 {
		t.Fatalf("replayed title-only = %+v, want it to keep the summary", events)
	}
	if mapper.ReasoningDisplay("run-1") != "Checking the calendar" {
		t.Fatalf("title-only replay overwrote live summary: %q", mapper.ReasoningDisplay("run-1"))
	}
}

func TestPreambleAndCommandAreProgressNotReasoning(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	started := mapper.StartRun("run-1")
	if len(started) != 1 || started[0].Summary != genericWorkingSummary {
		t.Fatalf("StartRun = %+v", started)
	}

	preamble, ok := NormalizeAgentEvent(json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:main","stream":"item","data":{"item":{"type":"preamble","title":"Considering the request"}}}`))
	if !ok {
		t.Fatal("NormalizeAgentEvent returned false")
	}
	if preamble.carriesReasoning() || !preamble.carriesProgress() {
		t.Fatalf("preamble carriesReasoning=%v carriesProgress=%v", preamble.carriesReasoning(), preamble.carriesProgress())
	}
	if events := mapper.ApplyAgent(preamble); len(events) != 0 {
		t.Fatalf("ApplyAgent preamble = %+v, want no KindReasoning", events)
	}
	progress := mapper.ApplyProgress(preamble)
	if len(progress) != 1 || progress[0].StartsTurn || progress[0].Summary != "Considering the request" {
		t.Fatalf("ApplyProgress preamble = %+v, want updating Working", progress)
	}

	command, ok := NormalizeAgentEvent(json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:main","stream":"codex_app_server.item","data":{"type":"commandExecution","item":{"type":"commandExecution","command":"ls"}}}`))
	if !ok {
		t.Fatal("NormalizeAgentEvent returned false")
	}
	if command.carriesReasoning() || !command.carriesProgress() {
		t.Fatalf("command carriesReasoning=%v carriesProgress=%v", command.carriesReasoning(), command.carriesProgress())
	}
	if events := mapper.ApplyAgent(command); len(events) != 0 {
		t.Fatalf("ApplyAgent command = %+v, want no KindReasoning", events)
	}
	running := mapper.ApplyProgress(command)
	if len(running) != 1 || running[0].StartsTurn || running[0].Summary != "ls" {
		t.Fatalf("ApplyProgress command = %+v, want Working from the command text", running)
	}
	if mapper.ReasoningDisplay("run-1") != "" {
		t.Fatalf("ReasoningDisplay = %q, want empty for progress items", mapper.ReasoningDisplay("run-1"))
	}
}

func TestAliasRunFinishesBothIDs(t *testing.T) {
	mapper := NewTurnMapper(testTaskID, testSessionKey)
	mapper.StartRun("idem")
	mapper.AliasRun("idem", "srv")
	reply := mapper.Apply(ChatEventPayload{State: "final", RunID: "srv", SessionKey: testSessionKey, Message: json.RawMessage(`"pong"`)})
	if len(reply) != 1 || reply[0].Kind != "reply" {
		t.Fatalf("final = %+v, want reply", reply)
	}
	if mapper.KnowsRun("idem") || mapper.KnowsRun("srv") {
		t.Fatal("both the idempotency key and the server run id must be forgotten")
	}
	if !mapper.Finished("idem") || !mapper.Finished("srv") {
		t.Fatal("both ids must be marked finished")
	}
}

func TestAssistantReplyAndNonReasoningItemsDoNotBecomeReasoning(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "item agentMessage",
			raw:  `{"runId":"run-1","stream":"item","data":{"item":{"type":"agentMessage","text":"Done."}}}`,
		},
		{
			name: "lifecycle",
			raw:  `{"runId":"run-1","stream":"lifecycle","data":{"phase":"start"}}`,
		},
		{
			name: "item compaction analysis",
			raw:  `{"runId":"run-1","stream":"item","data":{"kind":"analysis","title":"Context compaction","phase":"start"}}`,
		},
		{
			name: "assistant reply tokens",
			raw:  `{"runId":"run-1","stream":"assistant","data":{"text":"pong","delta":"pong"}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, ok := NormalizeAgentEvent(json.RawMessage(test.raw))
			if !ok {
				t.Fatal("NormalizeAgentEvent returned false")
			}
			mapper := NewTurnMapper(testTaskID, testSessionKey)
			if events := mapper.ApplyAgent(payload); len(events) != 0 {
				t.Fatalf("ApplyAgent = %+v, want no reasoning", events)
			}
			if got := mapper.ReasoningDisplay("run-1"); got != "" {
				t.Fatalf("ReasoningDisplay = %q, want empty", got)
			}
		})
	}
}
