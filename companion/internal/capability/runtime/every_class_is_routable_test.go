package runtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
)

// The same defect, one link further up.
//
// A reply adapter now exists and a hand-built route reaches it
// (notification_reply_reachable_test.go). That test starts at stage 2, with a
// `stage1.Route` already carrying `AppClass: "notification_reply"`. Nothing in
// the product ever builds that value.
//
// Routing is two stages. Stage 1 asks the model to turn what a person said into
// a verb and an app class. Stage 2 turns that class into an adapter. Stage 2 is
// now correct. Stage 1 cannot get there: `app_class` is a free-form string in
// the schema (`client.go:242`), so the model is not choosing from a list — it
// writes whatever word the instructions taught it, and the instructions have no
// line about replying at all. The nearest lines point the other way; WhatsApp's
// says "never claim the message was sent (notification reply is a separate
// path)", and that separate path was never described.
//
// So "reply to Maya: on my way" comes back as messaging/whatsapp/compose, and
// the user gets WhatsApp opened with a draft — the exact hand-off the reply
// route exists to replace, delivered silently, with every test green.
//
// This is the fourth-variant defect again: a finished chain with nothing able
// to start it. Closing it at the adapter moved the missing first mover one link
// upstream rather than removing it. That is why the general rule below matters
// more than a line of coaching text:
//
//	**a class the router cannot name is an adapter nobody can reach.**
//
// A per-app coaching test would have to be remembered for the next adapter too,
// and this repository has already lost an adapter to a hand-written list nobody
// updated (notion, in the OAuth skip list). This one derives both sides: the
// classes come from the production registry, the words come from the request
// actually put on the wire.

// modelInstructions runs one routing call against a fake OpenAI and returns the
// instruction text the client really sent. Reading the constant directly is not
// an option and should not be — what matters is what leaves the machine.
func modelInstructions(t *testing.T) string {
	t.Helper()
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"send\",\"app_class\":\"notification_reply\",\"app_named\":\"\",\"subject\":\"Maya\",\"body\":\"on my way\",\"confidence\":0.95}"}]}],"usage":{"input_tokens":10,"output_tokens":10,"total_tokens":20}}`)
	}))
	defer server.Close()

	client, err := stage1openai.New(stage1openai.Config{
		APIKey: "test-only-not-a-real-key", BaseURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("building the router client: %v", err)
	}
	if _, err := client.Model(context.Background(), "reply to Maya: on my way"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	instructions, _ := request["instructions"].(string)
	if instructions == "" {
		t.Fatal("the routing call carried no instructions at all")
	}
	return instructions
}

// The general rule. Every class the production build can resolve has to be a
// word the router was taught to write, or nothing a person says can reach it.
func TestEveryClassInTheBuildIsOneTheRouterCanName(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	instructions := strings.ToLower(modelInstructions(t))

	var unreachable []string
	for class := range inv.Classes {
		if !strings.Contains(instructions, strings.ToLower(class)) {
			unreachable = append(unreachable, class)
		}
	}
	if len(unreachable) > 0 {
		t.Fatalf("the router was never taught to write these classes, so no utterance can reach their adapters: %s — "+
			"add a line to the instructions in routing/stage1/openai/client.go saying when to use each",
			strings.Join(unreachable, ", "))
	}
}

// The specific one, spelled out, because the general rule above is satisfied by
// the word appearing anywhere and a reply needs more than its own name: which
// verb, and what goes in subject versus body. Getting subject wrong is not a
// missed route, it is a message sent to the wrong person.
func TestTheRouterIsToldHowToAddressAReply(t *testing.T) {
	instructions := strings.ToLower(modelInstructions(t))

	for _, want := range []string{"notification_reply", "reply"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("the router is never told about %q, so it can never route a reply", want)
		}
	}
	// The reply line has to say the person's name goes in subject. The phone
	// matches that name against its live conversations, and the Mac must not
	// resolve or tidy it first.
	line := replyInstructionLine(t, instructions)
	for _, want := range []string{"send", "subject", "body"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the reply instruction never says where %q goes: %q", want, line)
		}
	}
}

// First control. Teaching the router a new class must not blur the two jobs
// that were deliberately split: answering a thread already on screen, and
// starting a conversation from nothing. Only the first can be finished without
// the user, and the compose adapters must keep the second.
func TestStartingAConversationIsStillTaughtAsSomethingElse(t *testing.T) {
	instructions := strings.ToLower(modelInstructions(t))
	line := replyInstructionLine(t, instructions)

	if strings.Contains(line, "compose") {
		t.Fatalf("the reply instruction mentions compose, which would let it swallow requests to start a conversation: %q", line)
	}
	// The compose coaching for the messaging apps has to survive untouched.
	for _, want := range []string{"whatsapp", "messenger", "signal"} {
		if !strings.Contains(instructions, want+" prepare-and-open") {
			t.Fatalf("the %s prepare-and-open coaching is gone; a request to message someone new has nowhere to go", want)
		}
	}
}

// Second control, and the one that keeps this honest rather than merely
// routable. The instructions must not teach the router that a reply is
// delivered — the phone only ever learns that the app accepted the text.
func TestTheRouterIsNotTaughtThatARepliedMessageArrives(t *testing.T) {
	line := replyInstructionLine(t, strings.ToLower(modelInstructions(t)))

	for _, claim := range []string{"delivered", "received", "guaranteed"} {
		if strings.Contains(line, claim) {
			t.Fatalf("the reply instruction claims %q: %q", claim, line)
		}
	}
}

// replyInstructionLine pulls out the one sentence about replying, so the tests
// above check that sentence rather than the whole prompt — where "send" and
// "subject" appear dozens of times for unrelated reasons and every assertion
// would pass by accident.
func replyInstructionLine(t *testing.T, instructions string) string {
	t.Helper()
	for _, sentence := range strings.Split(instructions, ". ") {
		if strings.Contains(sentence, "notification_reply") {
			return sentence
		}
	}
	t.Fatalf("no sentence in the %d-character instructions mentions notification_reply", len(instructions))
	return ""
}
