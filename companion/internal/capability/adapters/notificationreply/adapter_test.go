package notificationreply

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// The whole reply route is built and nothing starts it.
//
// Operator can answer a live message thread by using the reply box Android
// puts in the notification. Every link of that chain exists and has a real
// caller: handOffToDevice (`handler.go:1062`) sends a `device_action` frame,
// `handleDeviceAction` (`LauncherSessionViewModel.kt:1189`) receives it,
// `DeviceReplyRequest.carryOut` decides, `AndroidReplyDispatch` fires the
// RemoteInput. Grepping any one link finds a caller, so link-by-link checking
// says the route is healthy.
//
// It has never run. The chain starts when an adapter returns
// `adapter.DeviceWorkError` — "this cannot be done from the Mac, the phone has
// to do it" — and **no adapter in the product ever constructs one.** The only
// construction site in the whole Go tree is a test fake
// (`mobilesession/device_work_test.go`). Six references, one constructor, zero
// in production.
//
// This file specifies the missing first mover: the adapter that hands a reply
// to the phone.
//
// Three decisions are baked into these tests, each written down because a
// future reader will otherwise "fix" them back:
//
//  1. **The verb is `send`, not `compose`.** Compose already belongs to the
//     hand-off adapters (instagram, and whatsapp via deeplink) which open the
//     app with drafted text and claim nothing. Those two jobs are genuinely
//     different — answering a thread that is already on screen, versus starting
//     a conversation from nothing — and only the first one can be finished
//     without the user. Splitting them by verb is what keeps this adapter from
//     swallowing the case it cannot do.
//
//  2. **The ceiling is `completes`.** A ceiling answers "how much is left for
//     the user to do", and after the reply is fired there is nothing left. The
//     open question about a reply is not effort, it is *certainty* — we know
//     Android accepted the text, not that the recipient received it. This
//     codebase keeps those apart on purpose and already has a word for the
//     second one (`outcome_unknown`). Lowering the ceiling to say "we are not
//     sure" would put the doubt in the field that does not mean doubt.
//
//  3. **The person's name is passed through exactly as typed.** The Mac must
//     not resolve it. Only the phone knows which conversations still have a
//     live notification, and it already matches a name against them
//     (`ReplyHandleSource.candidatesFor`). Any tidying here — lowercasing,
//     stripping, mapping to a canonical id — makes the phone's match fail on a
//     name that would have worked.

const anyRequest = "req-1"

func replyIntent(subject, body string) adapter.Intent {
	return adapter.Intent{Verb: manifest.Send, Subject: subject, Body: body}
}

// carryOut runs the adapter the way the flow does — Resolve, then Execute —
// and hands back whatever Execute returned. The device hand-off is an *error*
// return, not an outcome, so a test that only looked at the outcome would see
// an empty struct and conclude nothing happened.
func carryOut(t *testing.T, in adapter.Intent) (adapter.Outcome, error) {
	t.Helper()
	a := New(nil)
	plan, err := a.Resolve(context.Background(), in)
	if err != nil {
		return adapter.Outcome{}, err
	}
	return a.Execute(context.Background(), plan)
}

// deviceWork pulls the hand-off out of an error, failing the test if the error
// was anything else. This is the assertion the whole file is built around.
func deviceWork(t *testing.T, err error) *adapter.DeviceWorkError {
	t.Helper()
	var work *adapter.DeviceWorkError
	if !errors.As(err, &work) {
		t.Fatalf("the adapter did not hand the reply to the phone; it returned %v", err)
	}
	return work
}

// The one that matters. Everything downstream is already built and waiting for
// exactly this value to exist.
func TestAReplyIsHandedToThePhoneToCarryOut(t *testing.T) {
	_, err := carryOut(t, replyIntent("Maya", "on my way"))

	work := deviceWork(t, err)
	if work.Kind != "notification_reply" {
		t.Fatalf("kind = %q, want notification_reply — the phone accepts no other kind (contract/validation.go:1098)", work.Kind)
	}
	if work.Handle != "Maya" {
		t.Fatalf("handle = %q, want the name exactly as typed", work.Handle)
	}
	if work.Text != "on my way" {
		t.Fatalf("text = %q, want the message body unchanged", work.Text)
	}
	if work.AdapterID != ID {
		t.Fatalf("adapter id = %q, want %q — the ledger cannot look up a ceiling without it", work.AdapterID, ID)
	}
}

// The ceiling travels with the work or the clamp has nothing to clamp against.
// Before this existed, the result path hardcoded `completes` for every reply.
func TestTheCeilingHandedToThePhoneIsTheOneTheAdapterDeclared(t *testing.T) {
	_, err := carryOut(t, replyIntent("Maya", "on my way"))

	work := deviceWork(t, err)
	declared := New(nil).Describe().Ceiling
	if work.Ceiling != declared {
		t.Fatalf("handed the phone ceiling %q while declaring %q; these must never differ", work.Ceiling, declared)
	}
	if declared != manifest.Completes {
		t.Fatalf("declared ceiling = %q, want completes — see decision 2 at the top of this file", declared)
	}
}

// A name is a name. The phone matches it against live conversations, so any
// tidying here breaks a match that would have worked.
func TestThePersonsNameIsPassedThroughUntouched(t *testing.T) {
	for _, name := range []string{"Maya K", "maya", "Dr. Ana-María O'Brien", "मीरा"} {
		_, err := carryOut(t, replyIntent(name, "on my way"))
		work := deviceWork(t, err)
		if work.Handle != name {
			t.Fatalf("handle = %q, want %q unchanged — the Mac must not resolve people", work.Handle, name)
		}
	}
}

// First control. Starting a conversation is a different job that this adapter
// cannot do, and the hand-off adapters already do. If this one answers to
// compose it will win requests it cannot fulfil, and a user who asked to
// message someone new gets a refusal instead of a draft.
func TestItRefusesToStartAConversation(t *testing.T) {
	a := New(nil)
	for _, verb := range []manifest.Verb{manifest.Compose, manifest.Read, manifest.Write} {
		if _, err := a.Resolve(context.Background(), adapter.Intent{Verb: verb, Subject: "Maya", Body: "hi"}); err == nil {
			t.Fatalf("verb %q was accepted; only send belongs to this adapter", verb)
		}
	}
	if !contains(a.Describe().Verbs, manifest.Send) {
		t.Fatal("the adapter does not declare send, so the resolver would never pick it")
	}
	if contains(a.Describe().Verbs, manifest.Compose) {
		t.Fatal("the adapter declares compose, which would steal requests it cannot do")
	}
}

// Second control. An empty reply must never reach the phone. Firing a blank
// RemoteInput would post an empty message into a real person's chat, which
// cannot be taken back, and the wire rejects it anyway
// (contract/validation.go: text must not be blank) — so the phone would answer
// with a protocol error rather than anything a user could act on.
func TestAnEmptyReplyNeverLeavesTheMac(t *testing.T) {
	for _, body := range []string{"", "   ", "\n\t "} {
		_, err := carryOut(t, replyIntent("Maya", body))
		var work *adapter.DeviceWorkError
		if errors.As(err, &work) {
			t.Fatalf("an empty reply (%q) was handed to the phone", body)
		}
		if err == nil {
			t.Fatalf("an empty reply (%q) was accepted silently", body)
		}
	}
}

// Third control. A reply with nobody to send it to is the same danger from the
// other side: the phone matches a blank name against every live conversation.
func TestAReplyWithNoNameNeverLeavesTheMac(t *testing.T) {
	_, err := carryOut(t, replyIntent("   ", "on my way"))
	var work *adapter.DeviceWorkError
	if errors.As(err, &work) {
		t.Fatal("a reply with no name was handed to the phone, which would match it against every open chat")
	}
	if err == nil {
		t.Fatal("a reply with no name was accepted silently")
	}
}

// Fourth control, on the sheet the user actually reads before tapping. It has
// to name the person and show the words that will be sent — a confirm step
// that hides either one is not consent to anything.
func TestTheConfirmSheetShowsWhoAndWhat(t *testing.T) {
	a := New(nil)
	plan, err := a.Resolve(context.Background(), replyIntent("Maya", "on my way"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	preview, err := a.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	shown := strings.Join(append(preview.Lines, preview.Headline, preview.Confirm), "\n")
	if !strings.Contains(shown, "Maya") {
		t.Fatalf("the confirm sheet never names the person: %q", shown)
	}
	if !strings.Contains(shown, "on my way") {
		t.Fatalf("the confirm sheet never shows the message: %q", shown)
	}
}

// Fifth control, and the one that stops this becoming the next dead subsystem.
// An adapter that declares a gate nobody can clear is refused at every door
// (manifest.CheckGates), so declaring one here would leave the whole route
// built and unreachable all over again — the exact failure this file exists to
// end.
func TestNothingBlocksThisAdapterFromEverRunning(t *testing.T) {
	m := New(nil).Describe()
	if err := m.CheckGates(); err != nil {
		t.Fatalf("the adapter declares a gate nobody can clear, so it can never run: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("the manifest is invalid, so registration would refuse it: %v", err)
	}
	if m.Platform != manifest.PlatformAndroid {
		t.Fatalf("platform = %q; the reply box is Android's, so the resolver must only offer this on Android", m.Platform)
	}
}

func contains(verbs []manifest.Verb, want manifest.Verb) bool {
	for _, v := range verbs {
		if v == want {
			return true
		}
	}
	return false
}
