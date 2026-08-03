package mobilesession

import (
	"bytes"
	"context"
	"errors"
	"testing"

	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// Telling someone their request was invalid when the only problem is that we
// do not know who they meant.
//
// The flow has a third answer besides "here is a preview" and "this went
// wrong": a question. stage2 returns MustAsk when it cannot pick a single
// recipient, and service.go:102 wraps that as &QuestionError{Question: ...}.
// That is not an error in the ordinary sense — nothing broke, nothing was
// refused, the request was perfectly valid and we simply need one more word
// from the person who made it.
//
// handler.go:921-924 collapses every Prepare error into the same thing:
//
//	preview, err := handler.capabilityFlow.Prepare(...)
//	if err != nil {
//	    ... publishCapabilityActionResult(ctx, sender, actionID, "failed")
//	}
//
// and publishCapabilityActionResult then attaches a hardcoded
// {"code":"invalid_action","retryable":false}. So the phone shows "failed —
// invalid action" for a request that was neither.
//
// This is not a corner case today, it is the common case. The contact graph
// that stage2 consults has no production writer at all (graph.Add's only
// caller is the offline eval harness), so the graph is permanently empty and
// *every* "message Maya" resolves to MustAsk. Every single one of them is
// currently reported to the user as a failure.
//
// The fix has to live inside Go, because the phone's decoder will not take a
// new word for this. ProtocolCodec.kt:618 holds a closed set of eleven error
// codes and rejects the whole envelope on an unknown one, so a code like
// "needs_disambiguation" would be dropped silently — worse than the bug. But
// :601 already accepts "cancelled" as an action state, and :216 permits an
// error object only when the state is "failed" or "outcome_unknown". So the
// honest shape that the shipped phone will accept is: state "cancelled", no
// error object. Nothing broke (so not "failed"), and we do know what happened
// (so not "outcome_unknown") — we stopped and did not act.
//
// Answering the question is a separate, larger piece of work: it needs the
// question text delivered to the phone and an answer route back, which is a
// wire change on both sides. This fixes only the word, which needs neither.

// questionFlow answers a request the way the real flow does when stage2 cannot
// tell which Maya was meant.
type questionFlow struct{ recordingCapabilityFlow }

func (f *questionFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, &capabilityflow.QuestionError{Question: "Which Maya did you mean?"}
}

// prepareFailureFlow is the guard: a request that genuinely cannot be served
// must still be reported as a failure.
type prepareFailureFlow struct{ recordingCapabilityFlow }

func (f *prepareFailureFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, errors.New("no adapter offers that verb")
}

// requestAndReadResult sends hello and one capability_request, then returns
// whatever the handler sends back. Unlike confirmAndReadResult it never sends a
// confirm: these tests are about what happens when Prepare itself answers, so
// the first message out is the one under test.
func requestAndReadResult(t *testing.T, flow CapabilityFlow) contract.Message {
	t.Helper()
	handler, sender := newTestHandler(t)
	handler.EnableCapabilities(flow)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-cap","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"cap-request","sender":"phone","type":"action","body":{"actionId":"cap-request-1","kind":"capability_request","utterance":"Tell Maya I am running late"}}`)); err != nil {
		t.Fatal(err)
	}
	return awaitSentMessage(t, sender.sent)
}

func TestAQuestionAboutWhoYouMeantIsNotReportedAsAFailure(t *testing.T) {
	result := requestAndReadResult(t, &questionFlow{})

	if result.Type != "action_result" {
		t.Fatalf("expected an action_result, got %q", result.Type)
	}
	if bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a question was reported as a failure: %s", result.Body)
	}
}

// "cancelled" is the only word the shipped phone already accepts that is true
// here: we stopped without acting, and nothing broke. Picking it is not a
// stylistic choice — it is forced by the phone's closed vocabulary, and it is
// also the honest one.
func TestAQuestionIsReportedAsCancelled(t *testing.T) {
	result := requestAndReadResult(t, &questionFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"cancelled"`)) {
		t.Fatalf("expected state cancelled, got: %s", result.Body)
	}
}

// The error object is not optional decoration in the other direction either.
// ProtocolCodec.kt:216 permits one only when the state is "failed" or
// "outcome_unknown", so a "cancelled" carrying an error would be thrown away by
// the phone and the user would be told nothing at all.
func TestAQuestionCarriesNoErrorObject(t *testing.T) {
	result := requestAndReadResult(t, &questionFlow{})

	if bytes.Contains(result.Body, []byte(`"error"`)) {
		t.Fatalf("a cancelled result carried an error object the phone will reject: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("the hardcoded failure code leaked onto a question: %s", result.Body)
	}
}

// The guard against over-correcting. If every Prepare error became "cancelled",
// real breakage would go silent — the user would see a request quietly stop
// with no indication anything was wrong, which is its own dishonesty.
func TestAPrepareFailureThatIsNotAQuestionIsStillAFailure(t *testing.T) {
	result := requestAndReadResult(t, &prepareFailureFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a genuine failure must still be reported as failed: %s", result.Body)
	}
	// prepareFailureFlow's error is an ordinary errors.New, not one of the
	// errors that actually establishes the request was invalid. "internal"
	// is the honest code for a failure we cannot explain; "invalid_action"
	// is reserved for requests that really were invalid.
	if !bytes.Contains(result.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("an unexplained failure must report code internal: %s", result.Body)
	}
}

// The second control. The ordinary path — Prepare succeeds — must still send a
// preview, not a result. Without this, "answer every request with cancelled"
// passes both tests above and the product stops working entirely.
func TestASuccessfulPrepareStillSendsAPreview(t *testing.T) {
	result := requestAndReadResult(t, &recordingCapabilityFlow{})

	if result.Type != "capability_preview" {
		t.Fatalf("expected a capability_preview, got %q: %s", result.Type, result.Body)
	}
}

// --- Carrying the sentence itself -------------------------------------------
//
// Everything above fixed the *word*. The sentence was still thrown away:
// handler.go:962 logs its length and nothing else, so the phone knew the
// request had stopped and never learned why. On the phone that lands while the
// phase is still ROUTING, where CapabilityInteraction.kt:329 sends every state
// but "failed" to an else branch reading "The app router returned an unexpected
// result. It was not sent to Codex." — under a dialog titled "App action
// failed". So a normal "say that again a bit more specifically" is presented as
// a malfunction of the router, and the router worked perfectly.
//
// The sentence rides on the action_result that already reports the stop, rather
// than a message of its own. action_result is already sequenced, journaled and
// replayed on a warm reconnect; a separate unsequenced message would opt out of
// all three and be lost outright if the connection dropped between the two
// sends. One message also means there is no order to get right.

func TestTheQuestionItselfReachesThePhone(t *testing.T) {
	result := requestAndReadResult(t, &questionFlow{})

	if !bytes.Contains(result.Body, []byte(`"question":"Which Maya did you mean?"`)) {
		t.Fatalf("the question was dropped on the way to the phone: %s", result.Body)
	}
}

// The sentence must not turn the result into something the phone throws away.
// Everything already true of a cancelled result stays true.
func TestCarryingTheQuestionDoesNotDisturbTheRestOfTheResult(t *testing.T) {
	result := requestAndReadResult(t, &questionFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"cancelled"`)) {
		t.Fatalf("expected state cancelled, got: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"error"`)) {
		t.Fatalf("a cancelled result carried an error object the phone will reject: %s", result.Body)
	}
}

// The control. A genuine failure has no question to ask, and attaching one
// would be inventing an explanation we do not have.
func TestAGenuineFailureCarriesNoQuestion(t *testing.T) {
	result := requestAndReadResult(t, &prepareFailureFlow{})

	if bytes.Contains(result.Body, []byte(`"question"`)) {
		t.Fatalf("a failure carried a question: %s", result.Body)
	}
}

// The second control. An ordinary result — nothing asked, nothing wrong — must
// not grow the field at all, or every result on the wire pays for this.
func TestAnOrdinaryResultDoesNotCarryAQuestionField(t *testing.T) {
	result := confirmAndReadResult(t, &recordingCapabilityFlow{})

	if bytes.Contains(result.Body, []byte(`"question"`)) {
		t.Fatalf("an ordinary result grew a question field: %s", result.Body)
	}
}
