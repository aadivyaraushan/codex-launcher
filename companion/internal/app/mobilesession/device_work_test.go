package mobilesession

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

// The first capability that cannot finish where it started.
//
// Every other capability runs end to end inside Confirm: the Mac decides, the
// Mac acts, and the answer is already in hand by the time Confirm returns. A
// notification reply is split down the middle — the routing needs the language
// model, which is on the Mac, and the reply box lives inside an Android
// notification, which is on the phone. So Confirm has to come back before
// anyone knows what happened.
//
// This file pins what happens in that gap.
//
//	Confirm says "not here"  ->  device_action goes out, nothing is claimed yet
//	the phone answers        ->  capability_result, one word turned into a sentence
//	the phone comes back new ->  action_result outcome_unknown  (it never got the ask)
//	nothing arrives in time  ->  action_result outcome_unknown  (it may well have sent)
//	a second answer arrives  ->  ignored, so a stale claim cannot overwrite the above
//
// The last two are the whole point. A reply that timed out may already be
// sitting in somebody's chat, so "failed" would invite a second send to a real
// person, and "done" would be a claim nobody can back. Only the third state is
// honest, and the ledger is what makes it reachable.

// notificationReplyFlow is a capability that says, on Confirm, that the work
// has to happen on the phone. It carries the routed result — who to reply to
// and what to say — because the Mac has already decided both by this point.
type notificationReplyFlow struct{ recordingCapabilityFlow }

func (f *notificationReplyFlow) Confirm(_ context.Context, _, _, _ string) (capabilityadapter.Outcome, error) {
	f.confirmed++
	return capabilityadapter.Outcome{}, &capabilityadapter.DeviceWorkError{
		AdapterID: "notification-reply",
		Kind:      "notification_reply",
		Handle:    "maya",
		Text:      "on my way",
	}
}

type youtubePlaybackFlow struct{ recordingCapabilityFlow }

func (f *youtubePlaybackFlow) Confirm(_ context.Context, _, _, _ string) (capabilityadapter.Outcome, error) {
	f.confirmed++
	return capabilityadapter.Outcome{}, &capabilityadapter.DeviceWorkError{
		AdapterID: "youtube",
		Kind:      "youtube_play",
		Handle:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Text:      "Never Gonna Give You Up",
		Ceiling:   manifest.HandsOff,
	}
}

// movableClock lets a test step past the wait deadline instead of sleeping for
// it. The shared newTestHandler helper pins time to a single instant, which is
// right for every other test here and useless for this one.
type movableClock struct{ at time.Time }

func (c *movableClock) now() time.Time       { return c.at }
func (c *movableClock) tick(d time.Duration) { c.at = c.at.Add(d) }

func newDeviceWorkHandler(t *testing.T) (*Handler, *recordingSender, *movableClock) {
	t.Helper()
	// Resolved, not raw. On a Mac t.TempDir() hands back a path under /tmp,
	// which is itself a link to /private/tmp, and the project rules reject a
	// path that turns out to point somewhere other than where it says
	// (pathrules.go) — a real protection that a test should satisfy rather
	// than sidestep. Every other helper in this package does the same.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	store := eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 32, MaxBytes: 64 * 1024})
	clock := &movableClock{at: sessionNow}
	handler, err := NewWithTaskSource(context.Background(), "Studio Mac", projectService, eventjournal.New(store, nil), nil, nil, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{deviceID: "pixel-9", sessionID: "session-1", connectionID: 1, projectPath: root, store: store}
	return handler, sender, clock
}

const (
	helloFrame = `{"version":{"major":1,"minor":0},"messageId":"hello-dev","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`
	askFrame   = `{"version":{"major":1,"minor":0},"messageId":"cap-confirm","sender":"phone","type":"action","body":{"actionId":"cap-action-1","kind":"capability_confirm","requestId":"cap-request-1","fingerprint":"` +
		sixtyFourAs + `","decision":"confirm"}}`
	sixtyFourAs = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// handOff walks a handler up to the moment the phone has been asked to send a
// reply, and returns the device_action frame that went out.
func handOff(t *testing.T, handler *Handler, sender *recordingSender) contract.Message {
	t.Helper()
	return handOffWithFlow(t, handler, sender, &notificationReplyFlow{})
}

func handOffWithFlow(t *testing.T, handler *Handler, sender *recordingSender, flow CapabilityFlow) contract.Message {
	t.Helper()
	handler.EnableCapabilities(flow)
	if err := handler.Handle(context.Background(), sender, decode(t, helloFrame)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 8)
	if err := handler.Handle(context.Background(), sender, decode(t, askFrame)); err != nil {
		t.Fatal(err)
	}
	return awaitSentMessage(t, sender.sent)
}

func answerFrame(outcome string) string {
	return `{"version":{"major":1,"minor":0},"messageId":"dev-r-1","sender":"phone","type":"device_action_result","body":{"requestId":"cap-request-1","outcome":"` + outcome + `"}}`
}

func bodyOf(t *testing.T, message contract.Message) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(message.Body, &body); err != nil {
		t.Fatalf("could not read the body of a %s: %v", message.Type, err)
	}
	return body
}

// nothingFurtherIsSent fails if any frame arrives. Used wherever silence is
// the assertion — a request still in flight must not be answered early, and a
// request already closed must not be answered twice.
func nothingFurtherIsSent(t *testing.T, sender *recordingSender) {
	t.Helper()
	select {
	case message := <-sender.sent:
		t.Fatalf("a %s was sent when nothing should have been: %s", message.Type, string(message.Body))
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAReplyTheMacCannotSendItselfIsHandedToThePhone(t *testing.T) {
	handler, sender, _ := newDeviceWorkHandler(t)

	message := handOff(t, handler, sender)

	if message.Type != "device_action" {
		t.Fatalf("the Mac did not ask the phone to act, it sent a %s: %s", message.Type, string(message.Body))
	}
	body := bodyOf(t, message)
	if body["requestId"] != "cap-request-1" {
		t.Fatalf("the ask does not name the request the phone's sheet is waiting on: %v", body)
	}
	if body["kind"] != "notification_reply" || body["handle"] != "maya" || body["text"] != "on my way" {
		t.Fatalf("the ask lost what the Mac had already decided: %v", body)
	}
}

func TestNothingIsClaimedWhileThePhoneIsStillWorking(t *testing.T) {
	// The gap this file exists for. Between the ask and the answer the honest
	// thing to say is nothing at all: a capability_result now would claim an
	// outcome, and an action_result now would close an action that is still
	// running.
	handler, sender, _ := newDeviceWorkHandler(t)

	handOff(t, handler, sender)

	nothingFurtherIsSent(t, sender)
}

func TestAPhoneThatHandedOffTheReplyClosesTheRequest(t *testing.T) {
	handler, sender, _ := newDeviceWorkHandler(t)
	handOff(t, handler, sender)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}

	message := awaitSentMessage(t, sender.sent)
	if message.Type != "capability_result" {
		t.Fatalf("a reply handed to the app did not close the request, it sent a %s", message.Type)
	}
	body := bodyOf(t, message)
	// notificationReplyFlow (above) never names a Ceiling, and a blank
	// declaration is read as the most modest one, hands_off — not the
	// strongest. Reporting "completes" here, or "done":true alongside
	// hands_off with no app named to hand off to, was the bug this file's
	// sibling, device_work_ceiling_test.go, was written to catch: a
	// hands_off adapter cannot claim it finished, even on a reply the phone
	// says it handed off.
	if body["requestId"] != "cap-request-1" || body["ceiling"] != "hands_off" || body["done"] != false {
		t.Fatalf("a hands_off adapter's handed-off reply was reported as finished: %v", body)
	}
}

func TestAYouTubePlaybackThatOpenedNamesTheSelectedVideoRoute(t *testing.T) {
	handler, sender, _ := newDeviceWorkHandler(t)
	message := handOffWithFlow(t, handler, sender, &youtubePlaybackFlow{})
	body := bodyOf(t, message)
	if body["kind"] != "youtube_play" || body["handle"] != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("the YouTube action lost its exact target: %v", body)
	}

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}
	result := bodyOf(t, awaitSentMessage(t, sender.sent))
	if result["ceiling"] != "hands_off" || result["done"] != false || result["detail"] != "Opened the selected video in YouTube; playback was not verified." {
		t.Fatalf("the YouTube acknowledgement was reported incorrectly: %v", result)
	}
}

func TestEveryWayAReplyCanMissComesBackAsNotDone(t *testing.T) {
	// Three ways to not send, each with its own sentence. They share done=false
	// because none of them put anything in anybody's chat — this is the half of
	// honesty that is easy to forget: "we definitely did not" is a real answer
	// and must not be blurred into "we don't know".
	for outcome, detail := range map[string]string{
		"notification_gone": "The notification is gone, so there was no reply box left to use.",
		// Deliberately says nothing about *why*. The phone refuses for more than
		// one reason — it could not tell which conversation was meant, or the
		// notification came back without a usable reply box (ReplyAdapter.kt:122)
		// — and the wire carries one word for both. Naming a cause here would be
		// the Mac inventing a fact, which is the thing this whole round is about.
		// What is true either way is that nothing was sent and the app still can.
		"refused": "Nothing was sent — this one has to be replied to in its own app.",
		"failed":  "The reply didn't go through.",
	} {
		t.Run(outcome, func(t *testing.T) {
			handler, sender, _ := newDeviceWorkHandler(t)
			handOff(t, handler, sender)

			if err := handler.Handle(context.Background(), sender, decode(t, answerFrame(outcome))); err != nil {
				t.Fatal(err)
			}

			message := awaitSentMessage(t, sender.sent)
			if message.Type != "capability_result" {
				t.Fatalf("%s did not close the request, it sent a %s", outcome, message.Type)
			}
			body := bodyOf(t, message)
			if body["done"] != false {
				t.Fatalf("%s was reported as finished: %v", outcome, body)
			}
			if body["detail"] != detail {
				t.Fatalf("%s lost its explanation: got %q", outcome, body["detail"])
			}
		})
	}
}

func TestASecondAnswerForTheSameRequestChangesNothing(t *testing.T) {
	// Two capability_results for one request would leave the phone's sheet on
	// whichever landed last. In the worst order that is a stale claim
	// overwriting an honest "we don't know" that a timeout already reported.
	handler, sender, _ := newDeviceWorkHandler(t)
	handOff(t, handler, sender)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}
	awaitSentMessage(t, sender.sent)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("failed"))); err != nil {
		t.Fatal(err)
	}
	nothingFurtherIsSent(t, sender)
}

func TestAnAnswerForARequestNobodyHandedOutIsIgnored(t *testing.T) {
	// A phone that reconnects and replays, or one that is simply confused, must
	// not be able to make the Mac report an outcome for work it never asked for.
	handler, sender, _ := newDeviceWorkHandler(t)
	handler.EnableCapabilities(&notificationReplyFlow{})
	if err := handler.Handle(context.Background(), sender, decode(t, helloFrame)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 8)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}

	nothingFurtherIsSent(t, sender)
}

func TestAPhoneComingBackGivesUpOnWhatItWasDoing(t *testing.T) {
	// device_action is not replayed from the journal, so a phone that starts a
	// fresh session has no memory of the ask and will never answer it. Whether
	// it sent the reply before it went is exactly what nobody can find out.
	handler, sender, _ := newDeviceWorkHandler(t)
	handOff(t, handler, sender)

	returning := &recordingSender{deviceID: "pixel-9", sessionID: "session-2", connectionID: 2, projectPath: sender.projectPath, store: sender.store}
	returning.sent = make(chan contract.Message, 8)
	if err := handler.Handle(context.Background(), returning, decode(t, helloFrame)); err != nil {
		t.Fatal(err)
	}

	body := unknownOutcomeAmong(t, returning)
	if body["actionId"] != "cap-action-1" {
		t.Fatalf("the give-up names the wrong action: %v", body)
	}
}

func TestARequestThatRanOutOfTimeIsReportedAsUnknown(t *testing.T) {
	handler, sender, clock := newDeviceWorkHandler(t)
	handOff(t, handler, sender)

	clock.tick(DeviceWorkTimeout + time.Second)
	handler.SweepDeviceWork(context.Background())

	unknownOutcomeAmong(t, sender)
}

func TestARequestStillInTimeIsLeftAlone(t *testing.T) {
	// The guard. Giving up early would block the user's next prompt on a reply
	// that was about to land — the mirror-image dishonesty.
	handler, sender, clock := newDeviceWorkHandler(t)
	handOff(t, handler, sender)

	clock.tick(DeviceWorkTimeout - time.Second)
	handler.SweepDeviceWork(context.Background())

	nothingFurtherIsSent(t, sender)
}

func TestALateAnswerAfterTheDeadlineIsNotApplied(t *testing.T) {
	// The race that matters: the user has already been told we could not find
	// out, and then the phone's answer finally arrives. Accepting it would
	// replace an honest "we don't know" with a claim.
	handler, sender, clock := newDeviceWorkHandler(t)
	handOff(t, handler, sender)
	clock.tick(DeviceWorkTimeout + time.Second)
	handler.SweepDeviceWork(context.Background())
	unknownOutcomeAmong(t, sender)

	if err := handler.Handle(context.Background(), sender, decode(t, answerFrame("handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}
	nothingFurtherIsSent(t, sender)
}

// unknownOutcomeAmong waits for an action_result carrying outcome_unknown and
// returns its body. It reads past other frames because a reconnect sends a
// snapshot too, and the give-up is not required to be first in the queue.
func unknownOutcomeAmong(t *testing.T, sender *recordingSender) map[string]any {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case message := <-sender.sent:
			if message.Type != "action_result" {
				continue
			}
			body := bodyOf(t, message)
			if body["state"] != "outcome_unknown" {
				t.Fatalf("the request ended as %q, which is a claim nobody can back: %v", body["state"], body)
			}
			errorBody, ok := body["error"].(map[string]any)
			if !ok || errorBody["code"] != "outcome_unknown" || errorBody["retryable"] != false {
				// Without this exact shape the phone's decoder drops the whole
				// envelope and the user is told nothing at all.
				t.Fatalf("the give-up is missing the error the phone requires: %v", body)
			}
			return body
		case <-deadline:
			t.Fatal("nobody was ever told the outcome could not be found out")
		}
	}
}
