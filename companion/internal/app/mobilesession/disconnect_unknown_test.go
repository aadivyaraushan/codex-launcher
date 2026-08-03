package mobilesession

import (
	"bytes"
	"context"
	"errors"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// The half of the unknown-outcome rule that was left out.
//
// handler.go:920-929 checks for an OutcomeUnknownError coming out of Confirm.
// handler.go:948-951, the disconnect branch four lines further down, does not:
//
//	if err := handler.capabilityFlow.Disconnect(ctx, adapterID); err != nil {
//	    ... publishCapabilityActionResult(ctx, sender, actionID, "failed")
//	}
//
// That was harmless while nothing on the revoke path could produce such an
// error. It is not harmless now: the Google revoke calls
// (gcalendar/client.go:145, gdrive/client.go:135) run their transport error
// through adapter.ClassifyHTTPFailure, so a revoke request that went out and
// whose reply never came back arrives here as an OutcomeUnknownError and is
// reported as a definite failure.
//
// Why that particular lie is expensive here. Google's revoke endpoint answers
// an already-revoked token with a 4xx. So: the reply is lost, the phone is
// told "failed", the user taps disconnect again, Google says 400 because the
// token is already dead, and they are told "failed" again — for as long as
// they keep trying. They end up believing an app still has access to their
// calendar when the access is already gone. The screen and the world disagree
// permanently, and nothing in the loop can ever correct it.
//
// The opposite mistake matters too, which is why "confirmed" is not the answer
// either: this is the one action whose whole point is cutting off access. Being
// told an app was disconnected when nobody knows whether it was is worse than
// being told to go and look.
//
// So the honest answer is the one already on the wire: outcome_unknown, with
// the error object the phone's decoder requires (ProtocolCodec.kt:198, :446-452,
// :591). Note the retry rule differs from a message send — retrying a revoke
// harms nobody — but the report of what happened must still be true.

// lostRevokeReplyFlow answers a disconnect the way the Google adapters now do
// when the revoke request went out and the answer never came back.
type lostRevokeReplyFlow struct{ recordingCapabilityFlow }

func (f *lostRevokeReplyFlow) Disconnect(_ context.Context, adapterID string) error {
	f.disconnected = append(f.disconnected, adapterID)
	return &capabilityadapter.OutcomeUnknownError{
		AdapterID: "gcalendar",
		Verb:      "revoke",
		Cause:     context.DeadlineExceeded,
	}
}

func disconnectAndReadResult(t *testing.T, flow CapabilityFlow) contract.Message {
	t.Helper()
	handler, sender := newTestHandler(t)
	handler.EnableCapabilities(flow)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-cap","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"cap-disconnect","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"gcalendar"}}`)); err != nil {
		t.Fatal(err)
	}
	return awaitSentMessage(t, sender.sent)
}

func TestARevokeWhoseReplyWasLostIsNotReportedAsAFailure(t *testing.T) {
	result := disconnectAndReadResult(t, &lostRevokeReplyFlow{})

	if result.Type != "action_result" {
		t.Fatalf("expected an action_result, got %q", result.Type)
	}
	if bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a lost revoke reply was reported as a definite failure: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"state":"outcome_unknown"`)) {
		t.Fatalf("expected state outcome_unknown, got: %s", result.Body)
	}
}

// The other direction, and the one with teeth. Whatever else this reports, it
// must never claim the app was disconnected — that is the sentence that makes
// someone stop checking.
func TestARevokeWhoseReplyWasLostIsNeverReportedAsConfirmed(t *testing.T) {
	result := disconnectAndReadResult(t, &lostRevokeReplyFlow{})

	if bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) {
		t.Fatalf("an unverified revoke was reported as a completed disconnect: %s", result.Body)
	}
}

// The phone throws away the whole envelope if an outcome_unknown arrives
// without an error object whose code it knows. A discarded frame means the
// user is told nothing at all, which is worse than the wrong word.
func TestTheUnknownRevokeCarriesAnErrorThePhoneWillAccept(t *testing.T) {
	result := disconnectAndReadResult(t, &lostRevokeReplyFlow{})

	if !bytes.Contains(result.Body, []byte(`"code":"outcome_unknown"`)) {
		t.Fatalf("expected error code outcome_unknown, got: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("the hardcoded failure code leaked onto an unknown outcome: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"retryable":true`)) {
		t.Fatalf("an unverified outcome must not be advertised as retryable: %s", result.Body)
	}
}

// The guard against over-correcting. A revoke that plainly did not happen —
// the token endpoint answered and said no — is still a failure, and blurring
// that into "we don't know" would send people looking for a problem that has
// a clear answer.
func TestARevokeThatPlainlyFailedIsStillAFailure(t *testing.T) {
	result := disconnectAndReadResult(t, &recordingCapabilityFlow{disconnectErr: errors.New("token endpoint said no")})

	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a definite refusal must still be reported as failed: %s", result.Body)
	}
	// "token endpoint said no" is an ordinary errors.New — nothing here
	// establishes that the *request* was invalid, so the honest code is
	// internal, not invalid_action.
	if !bytes.Contains(result.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("an unexplained refusal must report code internal: %s", result.Body)
	}
}
