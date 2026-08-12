package devicework

import (
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// A rendezvous, not a poll target.
//
// Every capability up to this point runs inside one function call and
// returns an Outcome directly; once execution needs a round trip to the
// phone, that no longer holds — the caller (mobilesession.Handler.RunOnDevice)
// has to block until somebody hands it a Result. This file is that hand-off
// point: Wait registers a wait and returns the channel its Result will
// arrive on, Claim lets the read loop atomically take ownership of a
// request so it can compute that Result, and DeviceGone fails every waiter
// a phone leaves behind.
//
//	the phone answers        ->  Claim, then the caller delivers a Result
//	a second answer arrives  ->  Claim refuses; the channel already delivered
//	the phone disconnects    ->  DeviceGone delivers an unanswered Result
//	a request nobody made    ->  Claim refuses, nothing to deliver into

const (
	device = "pixel-9"
	other  = "pixel-7"
)

func waiting(requestID, deviceID string) Record {
	return Record{RequestID: requestID, DeviceID: deviceID, Kind: "notification_reply", AdapterID: "notification_reply", Ceiling: manifest.Completes}
}

func TestARequestHandedToThePhoneIsRemembered(t *testing.T) {
	ledger := NewLedger()

	ch, ok := ledger.Wait(waiting("cap-1", device))
	if !ok {
		t.Fatal("the first hand-over of a request was refused")
	}
	if ch == nil {
		t.Fatal("Wait did not hand back a channel to listen on")
	}

	record, deliver, claimed := ledger.Claim("cap-1")
	if !claimed {
		t.Fatal("a request that was handed over could not be claimed")
	}
	if record.RequestID != "cap-1" || record.DeviceID != device || record.Kind != "notification_reply" {
		t.Fatalf("the record lost what it was about: %+v", record)
	}

	want := Result{Answered: true, Reached: manifest.Completes, Done: true, Detail: "handed off"}
	deliver <- want
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("the channel delivered the wrong result: got %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("the delivered result never reached the waiting channel")
	}
}

func TestTheSameRequestCannotBeHandedOverTwice(t *testing.T) {
	// A duplicate hand-off for one request would otherwise put two waits on
	// the books and fire two device_actions — meaning the same message sent
	// twice to a real person, which is the exact harm this area exists to
	// stop.
	ledger := NewLedger()
	ledger.Wait(waiting("cap-1", device))

	if _, ok := ledger.Wait(waiting("cap-1", device)); ok {
		t.Fatal("the same request was handed to the phone a second time")
	}
}

func TestOnlyTheFirstAnswerForARequestIsClaimed(t *testing.T) {
	ledger := NewLedger()
	ledger.Wait(waiting("cap-1", device))

	if _, _, claimed := ledger.Claim("cap-1"); !claimed {
		t.Fatal("the first answer was not accepted")
	}
	if _, _, claimed := ledger.Claim("cap-1"); claimed {
		t.Fatal("a second answer for the same request was accepted")
	}
}

func TestAnAnswerForARequestNobodyIsWaitingOnIsRefused(t *testing.T) {
	// A phone that reconnects and replays, or one that is simply confused,
	// must not be able to make the Mac deliver a result for a request it
	// never handed out.
	ledger := NewLedger()

	if _, _, claimed := ledger.Claim("a-request-nobody-made"); claimed {
		t.Fatal("an answer for an unknown request was accepted")
	}
}

func TestEveryRequestWaitingOnAPhoneThatLeftIsToldSo(t *testing.T) {
	ledger := NewLedger()
	first, _ := ledger.Wait(waiting("cap-1", device))
	second, _ := ledger.Wait(waiting("cap-2", device))

	ledger.DeviceGone(device)

	for _, ch := range []<-chan Result{first, second} {
		select {
		case result := <-ch:
			if result.Answered || result.Done {
				t.Fatalf("a request abandoned by its phone was reported as answered: %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("a waiter was never told its phone left")
		}
	}
	if _, _, claimed := ledger.Claim("cap-1"); claimed {
		t.Fatal("cap-1 was still claimable after its phone left")
	}
}

func TestAPhoneLeavingDoesNotTouchAnotherPhonesRequests(t *testing.T) {
	ledger := NewLedger()
	ledger.Wait(waiting("cap-1", device))
	ledger.Wait(waiting("cap-2", other))

	ledger.DeviceGone(device)

	if _, _, claimed := ledger.Claim("cap-2"); !claimed {
		t.Fatal("a request on a phone that is still here was abandoned with it")
	}
}

func TestAnEmptyLedgerIsQuiet(t *testing.T) {
	ledger := NewLedger()

	ledger.DeviceGone(device)

	if _, _, claimed := ledger.Claim("cap-1"); claimed {
		t.Fatal("an empty ledger produced a claimable record")
	}
}

func TestManyGoroutinesCannotClaimTheSameRequestTwice(t *testing.T) {
	// The read loop and a disconnect handler can both reach this at once.
	// Exactly one of them may win.
	ledger := NewLedger()
	ledger.Wait(waiting("cap-1", device))

	const racers = 32
	wins := make(chan bool, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, _, claimed := ledger.Claim("cap-1")
			wins <- claimed
		}()
	}
	close(start)

	won := 0
	for i := 0; i < racers; i++ {
		if <-wins {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("%d goroutines claimed the same request", won)
	}
}
