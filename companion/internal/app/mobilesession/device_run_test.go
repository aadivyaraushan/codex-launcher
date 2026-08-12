package mobilesession

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/devicework"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

// The agent's tool call that cannot finish where it started.
//
// A notification reply (and a YouTube playback) can only be carried out by
// the phone, so the adapter's Execute refuses with a DeviceWorkError and
// someone has to ask the phone and wait for its answer. In the deleted
// pipeline that someone was handOffToDevice, answering the phone's own
// capability sheet. Now the caller is the agent bridge, sitting inside a
// synchronous tool call — so the ask-and-wait becomes one blocking method,
// RunOnDevice, and the answer goes back to the agent instead of being
// journaled as a frame for the phone's deleted result sheet.
//
//	RunOnDevice          ->  device_action goes out, the call blocks
//	the phone answers    ->  the call returns, one word turned into a sentence
//	nothing in time      ->  the call returns unanswered — never "failed"
//	the phone comes back ->  the wait is orphaned, same honest non-answer
//
// The unanswered cases are the whole point, unchanged from the old world: a
// reply that timed out may already be sitting in somebody's chat, so
// "failed" would invite a second send to a real person, and "done" would be
// a claim nobody can back.

func newDeviceRunHandler(t *testing.T) (*Handler, *recordingSender) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	store := eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 32, MaxBytes: 64 * 1024})
	handler, err := NewWithTaskSource(context.Background(), "Studio Mac", projectService, eventjournal.New(store, nil), nil, nil, func() time.Time { return sessionNow })
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{deviceID: "pixel-9", sessionID: "session-1", connectionID: 1, projectPath: root, store: store}
	return handler, sender
}

const deviceRunHelloFrame = `{"version":{"major":1,"minor":0},"messageId":"hello-dev-run","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`

// connectPhone walks the handler through a hello so pixel-9 is the active
// device, then arms the sent channel so the test can watch what goes out.
func connectPhone(t *testing.T, handler *Handler, sender *recordingSender) {
	t.Helper()
	if err := handler.Handle(context.Background(), sender, decode(t, deviceRunHelloFrame)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 8)
}

// startRun calls RunOnDevice on its own goroutine — the method blocks by
// design — and hands back the channel its return value will land on.
func startRun(handler *Handler, ctx context.Context, ask devicework.Ask) chan runReturn {
	returned := make(chan runReturn, 1)
	go func() {
		result, err := handler.RunOnDevice(ctx, ask)
		returned <- runReturn{result, err}
	}()
	return returned
}

type runReturn struct {
	result devicework.Result
	err    error
}

func awaitRunReturn(t *testing.T, returned chan runReturn) runReturn {
	t.Helper()
	select {
	case r := <-returned:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("RunOnDevice never returned")
		return runReturn{}
	}
}

// deviceAsk reads the device_action frame the handler sent and returns its
// body, failing if anything else went out first.
func deviceAsk(t *testing.T, sender *recordingSender) map[string]any {
	t.Helper()
	message := awaitSentMessage(t, sender.sent)
	if message.Type != "device_action" {
		t.Fatalf("the phone was not asked to act, a %s was sent: %s", message.Type, string(message.Body))
	}
	var body map[string]any
	if err := json.Unmarshal(message.Body, &body); err != nil {
		t.Fatalf("could not read the device_action body: %v", err)
	}
	return body
}

func deviceAnswerFrame(requestID, outcome string) string {
	return `{"version":{"major":1,"minor":0},"messageId":"dev-run-answer","sender":"phone","type":"device_action_result","body":{"requestId":"` + requestID + `","outcome":"` + outcome + `"}}`
}

// silenceAfter fails if the handler sends anything further — the answer to
// device work now goes back to the waiting caller, never out to the phone
// as a result frame of any kind.
func silenceAfter(t *testing.T, sender *recordingSender) {
	t.Helper()
	select {
	case message := <-sender.sent:
		t.Fatalf("a %s was sent when the answer belonged to the waiting caller: %s", message.Type, string(message.Body))
	case <-time.After(150 * time.Millisecond):
	}
}

func TestRunOnDeviceAsksThePhoneAndReturnsItsAnswer(t *testing.T) {
	handler, sender := newDeviceRunHandler(t)
	connectPhone(t, handler, sender)

	returned := startRun(handler, context.Background(), devicework.Ask{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "maya", Text: "on my way", Ceiling: manifest.Completes,
	})

	body := deviceAsk(t, sender)
	requestID, _ := body["requestId"].(string)
	if requestID == "" {
		t.Fatalf("the ask carries no request id for the phone to answer: %v", body)
	}
	if body["kind"] != "notification_reply" || body["handle"] != "maya" || body["text"] != "on my way" {
		t.Fatalf("the ask lost what the agent had already decided: %v", body)
	}

	if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(requestID, "handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}

	r := awaitRunReturn(t, returned)
	if r.err != nil {
		t.Fatalf("RunOnDevice: %v", r.err)
	}
	if !r.result.Answered {
		t.Fatal("the phone answered but the result claims it did not")
	}
	if r.result.Reached != manifest.Completes || !r.result.Done {
		t.Fatalf("a completes-ceiling reply the phone handed off must report done, got %+v", r.result)
	}
	if r.result.Detail != "Handed to the app — we can't see whether it reached them." {
		t.Fatalf("the one outcome word was not turned into its sentence: %q", r.result.Detail)
	}
	// The answer belongs to the waiting caller. The old world's result
	// frame must not follow it out to the phone.
	silenceAfter(t, sender)
}

func TestAHandsOffAdapterCannotClaimItFinished(t *testing.T) {
	handler, sender := newDeviceRunHandler(t)
	connectPhone(t, handler, sender)

	returned := startRun(handler, context.Background(), devicework.Ask{
		AdapterID: "youtube", Kind: "youtube_play",
		Handle: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", Text: "Never Gonna Give You Up",
		Ceiling: manifest.HandsOff,
	})

	body := deviceAsk(t, sender)
	requestID, _ := body["requestId"].(string)
	if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(requestID, "handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}

	r := awaitRunReturn(t, returned)
	if r.err != nil {
		t.Fatalf("RunOnDevice: %v", r.err)
	}
	if r.result.Reached != manifest.HandsOff || r.result.Done {
		t.Fatalf("a hands_off adapter's handed-off work was reported as finished: %+v", r.result)
	}
	if r.result.Detail != "Opened the selected video in YouTube; playback was not verified." {
		t.Fatalf("the YouTube acknowledgement was reported incorrectly: %q", r.result.Detail)
	}
}

func TestABlankCeilingIsReadAsHandsOff(t *testing.T) {
	// An ask that names no ceiling gets the most modest reading, not the
	// strongest — the same rule handOffToDevice enforced.
	handler, sender := newDeviceRunHandler(t)
	connectPhone(t, handler, sender)

	returned := startRun(handler, context.Background(), devicework.Ask{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "maya", Text: "on my way",
	})

	body := deviceAsk(t, sender)
	requestID, _ := body["requestId"].(string)
	if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(requestID, "handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}

	r := awaitRunReturn(t, returned)
	if r.result.Reached != manifest.HandsOff || r.result.Done {
		t.Fatalf("a blank ceiling was read as something stronger than hands_off: %+v", r.result)
	}
}

func TestEveryWayAReplyCanMissComesBackAsNotDone(t *testing.T) {
	// Three ways to not send, each with its own sentence. They share
	// done=false because none of them put anything in anybody's chat —
	// "we definitely did not" is a real answer and must not be blurred
	// into "we don't know".
	for outcome, detail := range map[string]string{
		"notification_gone": "The notification is gone, so there was no reply box left to use.",
		"refused":           "Nothing was sent — this one has to be replied to in its own app.",
		"failed":            "The reply didn't go through.",
	} {
		t.Run(outcome, func(t *testing.T) {
			handler, sender := newDeviceRunHandler(t)
			connectPhone(t, handler, sender)

			returned := startRun(handler, context.Background(), devicework.Ask{
				AdapterID: "notification_reply", Kind: "notification_reply",
				Handle: "maya", Text: "on my way", Ceiling: manifest.Completes,
			})

			body := deviceAsk(t, sender)
			requestID, _ := body["requestId"].(string)
			if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(requestID, outcome))); err != nil {
				t.Fatal(err)
			}

			r := awaitRunReturn(t, returned)
			if r.err != nil {
				t.Fatalf("RunOnDevice: %v", r.err)
			}
			if !r.result.Answered || r.result.Done {
				t.Fatalf("%s was not reported as an answered non-send: %+v", outcome, r.result)
			}
			if r.result.Detail != detail {
				t.Fatalf("%s lost its explanation: got %q", outcome, r.result.Detail)
			}
		})
	}
}

func TestRunOnDeviceWithNoPhoneConnectedIsAnError(t *testing.T) {
	handler, _ := newDeviceRunHandler(t)

	done := make(chan error, 1)
	go func() {
		_, err := handler.RunOnDevice(context.Background(), devicework.Ask{
			AdapterID: "notification_reply", Kind: "notification_reply",
			Handle: "maya", Text: "on my way",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("no phone is connected, but the ask was reported as if one answered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("with no phone connected RunOnDevice must fail fast, not sit out a deadline")
	}
}

func TestAnUnansweredAskIsUnknownNotFailed(t *testing.T) {
	handler, sender := newDeviceRunHandler(t)
	connectPhone(t, handler, sender)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	returned := startRun(handler, ctx, devicework.Ask{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "maya", Text: "on my way", Ceiling: manifest.Completes,
	})

	body := deviceAsk(t, sender)
	requestID, _ := body["requestId"].(string)

	r := awaitRunReturn(t, returned)
	if r.err != nil {
		t.Fatalf("an unanswered ask is a real answer, not an error: %v", r.err)
	}
	if r.result.Answered || r.result.Done {
		t.Fatalf("nobody answered, yet the result claims otherwise: %+v", r.result)
	}
	if r.result.Detail == "" {
		t.Fatal("an unanswered ask must explain itself to the agent")
	}

	// The reply may have gone out just after we gave up. A late answer is
	// silence — not a crash, not a second result for anyone.
	if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(requestID, "handed_to_the_app"))); err != nil {
		t.Fatalf("a late answer must land as silence: %v", err)
	}
	silenceAfter(t, sender)
}

func TestAPhoneThatComesBackFreshOrphansItsAsk(t *testing.T) {
	// device_action is never replayed: a phone starting a fresh session has
	// no memory of the ask and will never answer it. Whether it sent the
	// reply before it went away is exactly what nobody can find out.
	handler, sender := newDeviceRunHandler(t)
	connectPhone(t, handler, sender)

	returned := startRun(handler, context.Background(), devicework.Ask{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "maya", Text: "on my way", Ceiling: manifest.Completes,
	})
	deviceAsk(t, sender)

	fresh := &recordingSender{deviceID: "pixel-9", sessionID: "session-2", connectionID: 2, projectPath: sender.projectPath, store: sender.store}
	if err := handler.Handle(context.Background(), fresh, decode(t, deviceRunHelloFrame)); err != nil {
		t.Fatal(err)
	}

	r := awaitRunReturn(t, returned)
	if r.err != nil {
		t.Fatalf("an orphaned ask is a real answer, not an error: %v", r.err)
	}
	if r.result.Answered || r.result.Done {
		t.Fatalf("the phone never answered this session's ask, yet the result claims otherwise: %+v", r.result)
	}
}

func TestConcurrentAsksGetTheirOwnAnswers(t *testing.T) {
	handler, sender := newDeviceRunHandler(t)
	connectPhone(t, handler, sender)

	first := startRun(handler, context.Background(), devicework.Ask{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "maya", Text: "on my way", Ceiling: manifest.Completes,
	})
	firstAsk := deviceAsk(t, sender)
	second := startRun(handler, context.Background(), devicework.Ask{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "sam", Text: "running late", Ceiling: manifest.Completes,
	})
	secondAsk := deviceAsk(t, sender)

	firstID, _ := firstAsk["requestId"].(string)
	secondID, _ := secondAsk["requestId"].(string)
	if firstID == secondID {
		t.Fatalf("two outstanding asks share request id %q — their answers cannot be told apart", firstID)
	}

	// Answer them out of order: each answer must reach its own waiter.
	if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(secondID, "failed"))); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), sender, decode(t, deviceAnswerFrame(firstID, "handed_to_the_app"))); err != nil {
		t.Fatal(err)
	}

	firstReturn := awaitRunReturn(t, first)
	secondReturn := awaitRunReturn(t, second)
	if !firstReturn.result.Done {
		t.Fatalf("the handed-off reply was not the one reported done: %+v", firstReturn.result)
	}
	if secondReturn.result.Done || secondReturn.result.Detail != "The reply didn't go through." {
		t.Fatalf("the failed reply's answer went to the wrong waiter: %+v", secondReturn.result)
	}
}
