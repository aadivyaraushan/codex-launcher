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
