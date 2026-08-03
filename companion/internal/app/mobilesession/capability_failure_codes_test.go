package mobilesession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// Every failure is reported to the phone with the same three words.
//
// publishCapabilityActionResult (handler.go:1147) builds the error object from
// the state alone:
//
//	case "failed":
//	    result["error"] = map[string]any{"code": "invalid_action", ...}
//
// so "invalid_action" is not a description of anything — it is the only thing
// this companion can say. Six different situations arrive at it:
//
//   - this build has no capability support at all (handler.go:915)
//   - the user has not granted this app access yet (consent.ErrNotGranted)
//   - the adapter declares a checkpoint nobody can clear
//     (manifest.ErrGateNotCleared)
//   - the request named something that does not exist here
//   - a confirmation did not match what was previewed
//   - something inside us broke that we cannot explain
//
// Only two of those are the user's request being invalid. For the rest the
// phone shows "invalid action" over a request that was perfectly valid, and
// the one piece of advice the user could have acted on — "you need to connect
// this app first" — is the one never given.
//
// The phone's decoder holds a closed set of eleven codes
// (ProtocolCodec.kt:618) and throws away any envelope carrying a word outside
// it, so this cannot be fixed by inventing a better name. It has to be fixed
// by choosing correctly from the eleven that already exist. Three of them fit
// situations currently mislabelled: "unauthorized" for not-granted and for an
// uncleared gate, "desktop_incompatible" for a build that cannot do
// capabilities, and "internal" for a failure we cannot explain — which is the
// honest default, because "invalid_action" claims to know the user did
// something wrong and we do not know that.

// notGrantedFlow refuses at preview time because the user has never connected
// this app. This is the case the current code describes worst: the person is
// told their request was invalid, when the request was fine and the fix is one
// tap away.
type notGrantedFlow struct{ recordingCapabilityFlow }

func (f *notGrantedFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, consent.ErrNotGranted
}

// gateNotClearedFlow refuses because the adapter declares a billing or
// approval checkpoint and nothing in this product can clear one yet.
type gateNotClearedFlow struct{ recordingCapabilityFlow }

func (f *gateNotClearedFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, manifest.ErrGateNotCleared
}

// lateConsentFlow previews fine and then finds consent gone at confirm time.
// Consent can be revoked between the two, so the confirm path needs the same
// answer as the preview path, not a different one.
type lateConsentFlow struct{ recordingCapabilityFlow }

func (f *lateConsentFlow) Confirm(_ context.Context, _, _, _ string) (capabilityadapter.Outcome, error) {
	return capabilityadapter.Outcome{}, consent.ErrNotGranted
}

// verbNotOfferedFlow is a control: this really is a request the adapter cannot
// serve, so "invalid_action" is the truth and must survive the change.
type verbNotOfferedFlow struct{ recordingCapabilityFlow }

func (f *verbNotOfferedFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, execution.ErrVerbNotOffered
}

// unexplainableFlow breaks in a way we have no name for.
type unexplainableFlow struct{ recordingCapabilityFlow }

func (f *unexplainableFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, errors.New("reading the reply body: unexpected EOF")
}

func TestAppNotConnectedYetIsNotReportedAsAnInvalidRequest(t *testing.T) {
	result := requestAndReadResult(t, &notGrantedFlow{})

	if bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("a valid request was called invalid because the app is not connected: %s", result.Body)
	}
}

// "unauthorized" is the one word in the phone's fixed vocabulary that says
// what actually happened, and it is the only one the user can act on.
func TestAppNotConnectedYetIsReportedAsUnauthorized(t *testing.T) {
	result := requestAndReadResult(t, &notGrantedFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("expected state failed so the error object is allowed at all: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"code":"unauthorized"`)) {
		t.Fatalf("expected code unauthorized, got: %s", result.Body)
	}
}

func TestACheckpointNobodyCanClearIsReportedAsUnauthorized(t *testing.T) {
	result := requestAndReadResult(t, &gateNotClearedFlow{})

	if !bytes.Contains(result.Body, []byte(`"code":"unauthorized"`)) {
		t.Fatalf("an uncleared gate should read as unauthorized, got: %s", result.Body)
	}
}

// Consent can disappear between the preview and the confirm. Whichever side of
// that line the user is on, the reason they are being refused is the same, so
// the word they are given must be the same too.
func TestConsentLostBeforeConfirmIsAlsoReportedAsUnauthorized(t *testing.T) {
	result := confirmAndReadResult(t, &lateConsentFlow{})

	if !bytes.Contains(result.Body, []byte(`"code":"unauthorized"`)) {
		t.Fatalf("expected code unauthorized at confirm time, got: %s", result.Body)
	}
}

// The honest default. When we cannot say what went wrong, saying "internal"
// admits that; saying "invalid_action" blames the user for something we have
// not established.
func TestAFailureWeCannotExplainIsReportedAsInternal(t *testing.T) {
	result := requestAndReadResult(t, &unexplainableFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("an unexplained break is still a failure: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("expected code internal, got: %s", result.Body)
	}
}

// First control. Without it, "never say invalid_action" passes everything above
// and the one code that was sometimes right is lost.
func TestARequestTheAdapterCannotServeIsStillAnInvalidRequest(t *testing.T) {
	result := requestAndReadResult(t, &verbNotOfferedFlow{})

	if !bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("a request for something no adapter offers really is invalid: %s", result.Body)
	}
}

// Second control. A confirmation that does not match what was shown is the
// other genuinely invalid case, and it lives on the confirm path rather than
// the preview path, so it is checked separately.
func TestConfirmingSomethingOtherThanWhatWasShownIsStillInvalid(t *testing.T) {
	result := confirmAndReadResult(t, &plainFailureFlow{})

	if !bytes.Contains(result.Body, []byte(`"code":"invalid_action"`)) {
		t.Fatalf("a mismatched confirmation really is invalid: %s", result.Body)
	}
}

// Third control, and the one that keeps this from silently breaking later. The
// phone rejects the entire envelope on a code outside its set
// (ProtocolCodec.kt:618), so a well-meant new word would leave the user with
// no message at all — worse than the wrong word. Every code this handler can
// send has to be in the list below.
func TestEveryFailureCodeIsOneThePhoneAccepts(t *testing.T) {
	accepted := map[string]bool{
		"computer_offline": true, "connection_lost": true, "desktop_incompatible": true,
		"owner_unavailable": true, "invalid_action": true, "outcome_unknown": true,
		"sequence_gap": true, "unauthorized": true, "quota_exceeded": true,
		"attachment_invalid": true, "internal": true,
	}

	previewSide := map[string]CapabilityFlow{
		"not granted":      &notGrantedFlow{},
		"gate not cleared": &gateNotClearedFlow{},
		"verb not offered": &verbNotOfferedFlow{},
		"unexplainable":    &unexplainableFlow{},
	}
	confirmSide := map[string]CapabilityFlow{
		"consent lost late":    &lateConsentFlow{},
		"fingerprint mismatch": &plainFailureFlow{},
	}

	check := func(name string, result contract.Message) {
		t.Helper()
		code := errorCodeOf(t, result)
		if code == "" {
			t.Fatalf("%s: failure carried no error code at all: %s", name, result.Body)
		}
		if !accepted[code] {
			t.Fatalf("%s: code %q is not one the phone accepts, so it would drop the whole message", name, code)
		}
	}

	for name, flow := range previewSide {
		check(name, requestAndReadResult(t, flow))
	}
	for name, flow := range confirmSide {
		check(name, confirmAndReadResult(t, flow))
	}
}

// errorCodeOf pulls the error code out of an action_result body, or returns ""
// if there is no error object.
func errorCodeOf(t *testing.T, result contract.Message) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.Body, &decoded); err != nil {
		t.Fatalf("could not read the result body: %v (%s)", err, result.Body)
	}
	return decoded.Error.Code
}
