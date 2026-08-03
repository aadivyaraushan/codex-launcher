package mobilesession

import (
	"bytes"
	"context"
	"strings"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// Telling someone a message failed when it may already have been sent.
//
// handler.go:921-924 turns every error out of capabilityFlow.Confirm into the
// same thing:
//
//	outcome, err := handler.capabilityFlow.Confirm(...)
//	if err != nil {
//	    ... publishCapabilityActionResult(ctx, sender, actionID, "failed")
//	}
//
// and publishCapabilityActionResult then attaches a hardcoded
// {"code":"invalid_action","retryable":false} (handler.go:960-962).
//
// For most errors that is right. No credentials, a verb the adapter never
// offered, consent refused, a server that plainly heard us and said no — all
// of those are failures, and saying so is honest.
//
// It is wrong for the ones where the request demonstrably left the machine and
// only the reply went missing. Every network adapter's HTTP round-trip can end
// that way (slack client.go:171, todoist :131, notion session.go:85, outlook
// :168, gcalendar :139, gdrive :130, msteams :133), as can an osascript run
// killed by a deadline after the Apple Event already reached Reminders.app
// (applereminders/reminders.go:47). Slack may well have posted the message.
//
// Told "failed", a person sends it again, and the other end gets it twice.
//
// Nothing on the wire needs to change to fix this. `outcome_unknown` is
// already a valid action_result state AND a valid error code
// (ProtocolCodec.kt:198, :591), and this very file already emits that exact
// shape elsewhere (handler.go:1400-1401). The precedent for deciding *when*
// to use it is appserver.OutcomeUnknownError (client.go:131-139, :465), which
// is strict in the right way: request demonstrably transmitted, response
// demonstrably lost, method demonstrably mutating.

// unknownOutcomeFlow answers a confirm the way a real adapter does when it
// sent the request and never learned what happened.
type unknownOutcomeFlow struct{ recordingCapabilityFlow }

func (f *unknownOutcomeFlow) Confirm(_ context.Context, _, _, _ string) (capabilityadapter.Outcome, error) {
	f.confirmed++
	return capabilityadapter.Outcome{}, &capabilityadapter.OutcomeUnknownError{
		AdapterID: "slack",
		Verb:      "send",
		Cause:     context.DeadlineExceeded,
	}
}

// plainFailureFlow is the guard: an ordinary refusal must stay a refusal.
type plainFailureFlow struct{ recordingCapabilityFlow }

func (f *plainFailureFlow) Confirm(_ context.Context, _, _, _ string) (capabilityadapter.Outcome, error) {
	f.confirmed++
	return capabilityadapter.Outcome{}, capabilityflow.ErrFingerprintMismatch
}

func confirmAndReadResult(t *testing.T, flow CapabilityFlow) contract.Message {
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
	_ = awaitSentMessage(t, sender.sent)
	confirm := `{"version":{"major":1,"minor":0},"messageId":"cap-confirm","sender":"phone","type":"action","body":{"actionId":"cap-confirm-1","kind":"capability_confirm","requestId":"cap-request-1","fingerprint":"` + strings.Repeat("a", 64) + `","decision":"confirm"}}`
	if err := handler.Handle(context.Background(), sender, decode(t, confirm)); err != nil {
		t.Fatal(err)
	}
	return awaitSentMessage(t, sender.sent)
}

func TestAnOutcomeWeCouldNotLearnIsNotReportedAsAFailure(t *testing.T) {
	result := confirmAndReadResult(t, &unknownOutcomeFlow{})

	if result.Type != "action_result" {
		t.Fatalf("expected an action_result, got %q", result.Type)
	}
	if bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a lost reply was reported as a definite failure: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"state":"outcome_unknown"`)) {
		t.Fatalf("expected state outcome_unknown, got: %s", result.Body)
	}
}

// The error object is not decoration. The phone refuses the whole envelope if
// an outcome_unknown arrives without one whose code is in its known set
// (ProtocolCodec.kt:198 requires it, :446-452 checks the shape, :591 lists the
// codes). Getting this wrong means the message is silently discarded and the
// user is told nothing at all — worse than the bug being fixed.
func TestTheUnknownOutcomeCarriesAnErrorThePhoneWillAccept(t *testing.T) {
	result := confirmAndReadResult(t, &unknownOutcomeFlow{})

	if !bytes.Contains(result.Body, []byte(`"code":"outcome_unknown"`)) {
		t.Fatalf("expected error code outcome_unknown, got: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("the hardcoded failure code leaked onto an unknown outcome: %s", result.Body)
	}
}

// Never "try again" on an unknown. Retrying a send that may already have gone
// sends it twice, and the person on the other end gets it twice with no idea
// why. This is the same rule the phone already enforces for itself.
func TestAnUnknownOutcomeIsNeverMarkedRetryable(t *testing.T) {
	result := confirmAndReadResult(t, &unknownOutcomeFlow{})

	if bytes.Contains(result.Body, []byte(`"retryable":true`)) {
		t.Fatalf("an unverified send must never be advertised as retryable: %s", result.Body)
	}
}

// The guard against over-correcting. Most errors really are failures, and
// blurring them into "we don't know" would be its own dishonesty — it would
// make every ordinary refusal look alarming and unresolvable.
func TestAnOrdinaryRefusalIsStillReportedAsAFailure(t *testing.T) {
	result := confirmAndReadResult(t, &plainFailureFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a definite refusal must still be reported as failed: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("a definite refusal must keep its existing error code: %s", result.Body)
	}
}
