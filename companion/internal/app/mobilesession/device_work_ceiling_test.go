package mobilesession

import (
	"context"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// The one path where an adapter can claim more than it declared.
//
// Every adapter declares a ceiling — the most it can ever honestly claim to
// have done. `completes` means it finished the thing. `one_tap` means it got
// the user one tap away. `hands_off` means it opened the right app and stopped.
// The declaration is not a comment: `execution.Runner` clamps every outcome
// down to it before anyone sees it (`runner.go:215`,
// `a.Describe().Ceiling.AtMost(out.Reached)`), so an adapter that returns
// "completes" while declaring "hands_off" is reported as hands_off. That clamp
// is the only thing standing between a declaration and a claim.
//
// Device work does not go through that runner. The reply is carried out on the
// phone, so the answer comes back as its own frame and `handleDeviceActionResult`
// builds the result itself — with the ceiling written in by hand:
//
//	}{record.RequestID, string(manifest.Completes), done, detail, ""}   // handler.go:1131
//
// So every reply that lands is reported as `completes`, whatever the adapter
// declared, and the clamp never runs. It is the same defect this session has
// now found three times: **a constant standing in for something nobody worked
// out**. It reads as a decision and it is not one.
//
// It cannot be fixed by looking the adapter up, either. `devicework.Record`
// carries RequestID, DeviceID, Kind and ActionID — the adapter id is known at
// hand-off, is deliberately logged there (`handler.go:1081`), and is then
// dropped. Nothing downstream can ask what the adapter declared, because
// nothing downstream knows which adapter it was.
//
// This matters most for exactly the capability it was written for. Android's
// reply box only exists while a notification is still live. An adapter that
// honestly declares it cannot always finish is the one this path reports as
// having finished every time.
//
// The controls below are the point. "Always say hands_off" fixes every failing
// test here and silently demotes the adapters that really do finish.

// declaringFlow is a capability whose Confirm hands the work to the phone and
// says, as part of doing so, what its adapter is allowed to claim.
type declaringFlow struct {
	recordingCapabilityFlow
	ceiling manifest.Ceiling
}

func (f *declaringFlow) Confirm(_ context.Context, _, _, _ string) (capabilityadapter.Outcome, error) {
	f.confirmed++
	return capabilityadapter.Outcome{}, &capabilityadapter.DeviceWorkError{
		AdapterID: "notification-reply",
		Kind:      "notification_reply",
		Handle:    "maya",
		Text:      "on my way",
		Ceiling:   f.ceiling,
	}
}

// A reply that really was typed into a notification is still only as much as
// the adapter said it could do. Instagram's adapter declares hands_off today,
// and a reply handed to the app must not quietly upgrade that to "finished".
func TestAHandedToTheAppReplyClaimsOnlyWhatTheAdapterDeclared(t *testing.T) {
	if ceiling := ceilingAfterAnswer(t, manifest.HandsOff, "handed_to_the_app"); ceiling != string(manifest.HandsOff) {
		t.Fatalf("a hands_off adapter reported %q; the declaration is what caps the claim", ceiling)
	}
}

// First control, and the one that stops the obvious over-correction. An
// adapter that declares it finishes must still say so, or "always report
// hands_off" passes the test above and demotes every capability that works.
func TestAnAdapterThatDeclaresItFinishesStillSaysSo(t *testing.T) {
	if ceiling := ceilingAfterAnswer(t, manifest.Completes, "handed_to_the_app"); ceiling != string(manifest.Completes) {
		t.Fatalf("a completes adapter reported %q; the clamp must cap, not overwrite", ceiling)
	}
}

// Second control, on the middle value, so neither test above can be satisfied
// by a special case for the two ends.
func TestAOneTapAdapterReportsOneTap(t *testing.T) {
	if ceiling := ceilingAfterAnswer(t, manifest.OneTap, "handed_to_the_app"); ceiling != string(manifest.OneTap) {
		t.Fatalf("a one_tap adapter reported %q", ceiling)
	}
}

// An adapter that names nothing has declared nothing, and the honest reading of
// nothing is the most modest answer — not the boldest. Reporting "completes"
// for a blank is how the current bug reads in code: a value chosen because it
// had to be something.
func TestAnAdapterThatNamesNoCeilingGetsTheModestOne(t *testing.T) {
	if ceiling := ceilingAfterAnswer(t, "", "handed_to_the_app"); ceiling != string(manifest.HandsOff) {
		t.Fatalf("a blank declaration reported %q; a blank is not a claim", ceiling)
	}
}

// Third control, guarding the other half of the frame. The ceiling says how
// much this adapter could ever do; `done` says whether it did it this time.
// Changing the first must not disturb the second — a refusal is still a
// refusal, and it still has to explain itself.
func TestARefusalIsStillARefusalWhateverTheAdapterDeclared(t *testing.T) {
	handler, sender, _ := newDeviceWorkHandler(t)
	handOffDeclaring(t, handler, sender, manifest.Completes)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("refused"))); err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, awaitSentMessage(t, sender.sent))

	if done, _ := body["done"].(bool); done {
		t.Fatalf("a refusal was reported as done: %v", body)
	}
	if detail, _ := body["detail"].(string); detail == "" {
		t.Fatalf("a refusal with no explanation leaves the user nothing to act on: %v", body)
	}
}

// --- harness ---

// ceilingAfterAnswer runs one reply end to end — hand off to the phone, phone
// answers — and returns the ceiling word the phone is told.
func ceilingAfterAnswer(t *testing.T, declared manifest.Ceiling, outcome string) string {
	t.Helper()
	handler, sender, _ := newDeviceWorkHandler(t)
	handOffDeclaring(t, handler, sender, declared)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame(outcome))); err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, awaitSentMessage(t, sender.sent))
	ceiling, _ := body["ceiling"].(string)
	if ceiling == "" {
		t.Fatalf("the result named no ceiling at all: %v", body)
	}
	return ceiling
}

func handOffDeclaring(t *testing.T, handler *Handler, sender *recordingSender, declared manifest.Ceiling) {
	t.Helper()
	handler.EnableCapabilities(&declaringFlow{ceiling: declared})
	if err := handler.Handle(context.Background(), sender, decode(t, helloFrame)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 8)
	if err := handler.Handle(context.Background(), sender, decode(t, askFrame)); err != nil {
		t.Fatal(err)
	}
	awaitSentMessage(t, sender.sent)
}
