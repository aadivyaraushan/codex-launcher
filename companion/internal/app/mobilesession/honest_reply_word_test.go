package mobilesession

import (
	"context"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// The word the phone sends back when a reply is fired is `delivered`, and it
// is not true.
//
// Here is everything the phone actually observes. `AndroidReplyDispatch.deliver`
// puts the text into the notification action's RemoteInput bundle and calls
// `actionIntent.send(...)`. If that call does not throw, it returns DELIVERED.
// That is the whole basis for the claim. What the call proves is that Android
// handed the bundle to the messaging app that posted the notification. WhatsApp
// never told us anything. The recipient's phone never told us anything. The
// message may be sitting in a send queue behind no signal; the app may have
// dropped it; the account may be logged out.
//
// This plan exists because the route it replaced claimed "sent" without
// checking. Repeating that under a friendlier word is the same lie with better
// manners, and it is worse here than it was there, because the person reading
// "Reply sent." has no reason to open the app and look.
//
// Two things this is deliberately NOT:
//
//   - **It is not a lower ceiling.** A ceiling answers "how much is left for
//     the user to do", and once the text has gone into the reply box there is
//     nothing left for them to do. `completes` is right and stays. The doubt
//     here is about certainty, not effort, and this codebase already keeps
//     those two apart on purpose.
//
//   - **It is not `done: false`.** Same reason. The work was carried out. What
//     is unproven is the result, and reporting the work as unfinished would
//     put the person back in front of a task they cannot do anything about.
//
// So the change is to the word, and to the sentence the person reads. The word
// becomes `handed_to_the_app`, which is the largest true statement available:
// we handed the text to the app that owns the conversation, and that is where
// what we can see ends.
//
// The controls below matter more than the headline. "Never claim anything"
// passes the first test and destroys the other three outcomes, which really do
// know that nothing was sent.

const firedWord = "handed_to_the_app"

// The headline. The word the wire carries for a fired reply must not be a
// claim about arrival.
func TestTheWordForAFiredReplyDoesNotClaimItArrived(t *testing.T) {
	handler, sender, _ := newDeviceWorkHandler(t)
	handOffDeclaring(t, handler, sender, manifest.Completes)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame(firedWord))); err != nil {
		t.Fatalf("the phone reported %q and the Mac would not accept it: %v", firedWord, err)
	}
	body := bodyOf(t, awaitSentMessage(t, sender.sent))

	if done, _ := body["done"].(bool); !done {
		t.Fatalf("a fired reply was reported as unfinished: %v — the doubt belongs in the word, not in the done flag", body)
	}
	if ceiling, _ := body["ceiling"].(string); ceiling != string(manifest.Completes) {
		t.Fatalf("ceiling = %q, want completes — a ceiling is about effort left, and none is", ceiling)
	}
}

// The old word has to actually be gone. Leaving it accepted "for compatibility"
// would mean a phone still shipping it is still making the claim, and nothing
// would ever surface that.
func TestTheOldWordIsGoneFromTheWire(t *testing.T) {
	if _, err := contract.DecodeText([]byte(answerFrame("delivered"))); err == nil {
		t.Fatal("the wire still accepts \"delivered\"; a word that is not true must not be reachable, not merely unused")
	}
}

// What the person reads. The frame's own word is invisible to them; this
// sentence is the whole of what they are told.
func TestWhatThePersonReadsDoesNotClaimTheMessageArrived(t *testing.T) {
	handler, sender, _ := newDeviceWorkHandler(t)
	handOffDeclaring(t, handler, sender, manifest.Completes)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame(firedWord))); err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, awaitSentMessage(t, sender.sent))
	detail, _ := body["detail"].(string)

	if detail == "" {
		t.Fatal("the person is told nothing at all about a reply that was fired at a real conversation")
	}
	// "sent", "delivered" and "received" are all claims about something on the
	// other side of an app we cannot see into.
	for _, claim := range []string{"sent", "delivered", "received", "went through"} {
		if strings.Contains(strings.ToLower(detail), claim) {
			t.Fatalf("the person is told %q, which claims %q — we only know Android handed the text to the app", detail, claim)
		}
	}
}

// First control, and the one that stops the over-correction. Three of the four
// outcomes are certain, and softening them into "we can't tell" would be its
// own dishonesty — worse, because a person who is told nothing was sent can
// act, and a person told "maybe" cannot.
func TestTheOutcomesThatKnowNothingWasSentStillSaySo(t *testing.T) {
	for _, outcome := range []string{"notification_gone", "refused", "failed"} {
		handler, sender, _ := newDeviceWorkHandler(t)
		handOffDeclaring(t, handler, sender, manifest.Completes)

		if err := handler.Handle(context.Background(), sender, decode(t, answerFrame(outcome))); err != nil {
			t.Fatalf("%s: %v", outcome, err)
		}
		body := bodyOf(t, awaitSentMessage(t, sender.sent))

		if done, _ := body["done"].(bool); done {
			t.Fatalf("%s was reported as done: %v", outcome, body)
		}
		if detail, _ := body["detail"].(string); detail == "" {
			t.Fatalf("%s left the person with no explanation: %v", outcome, body)
		}
	}
}

// Second control. Renaming one word in a set of four is exactly the change that
// leaves the two machines disagreeing, and a word one side does not know is not
// a smaller message — the whole frame is dropped and the person sees nothing.
func TestTheWireKnowsFourWordsAndNoOthers(t *testing.T) {
	for _, outcome := range []string{firedWord, "notification_gone", "failed", "refused"} {
		if _, err := contract.DecodeText([]byte(answerFrame(outcome))); err != nil {
			t.Fatalf("the wire refused %q, which the phone can produce: %v", outcome, err)
		}
	}
	for _, outcome := range []string{"", "sent", "ok", "unknown", "HANDED_TO_THE_APP", "handed"} {
		if _, err := contract.DecodeText([]byte(answerFrame(outcome))); err == nil {
			t.Fatalf("the wire accepted %q, which nothing produces", outcome)
		}
	}
}
