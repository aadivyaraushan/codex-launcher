package devicework

import (
	"testing"
	"time"
)

// The state nobody owns yet.
//
// Every capability so far finishes inside one function call:
// flow.Service.Confirm deletes its pending-preview entry (service.go:156) and
// then calls Execute, which runs the whole thing on the Mac and returns an
// Outcome. Once execution needs a round trip to the phone, that no longer
// holds — Confirm has to come back before anybody knows what happened, and
// something has to remember "request X is waiting on device Y" in the meantime.
//
// This file is that something, and it is deliberately just a bookkeeper: it
// holds no adapters, sends nothing, and knows no wording. It answers one
// question — is this request still outstanding, and who owes us an answer.
//
// Three ways a wait ends badly, and all three end the same way: the request
// becomes outcome_unknown. Not done (nobody saw it land) and not failed (a
// reply that timed out may well have been sent). This is the whole reason the
// third state exists.
//
//	the phone disconnects while waiting  ->  outcome_unknown
//	nothing arrives before the timeout   ->  outcome_unknown
//	a second result for the same request ->  ignored, never applied twice
//
// The last one is not a tidiness rule. Applying a result twice would emit two
// capability_result frames for one request, and the phone's sheet would settle
// on whichever arrived last — including, in the worst order, a stale "delivered"
// overwriting an honest "we don't know" that the timeout had already reported.

const (
	timeout = 30 * time.Second
	device  = "pixel-9"
	other   = "pixel-7"
)

// clock is a hand-wound one, so a test can sit at a moment and step past a
// deadline without waiting for real seconds to pass.
type clock struct{ at time.Time }

func (c *clock) now() time.Time       { return c.at }
func (c *clock) tick(d time.Duration) { c.at = c.at.Add(d) }

func newLedgerAt(start time.Time) (*Ledger, *clock) {
	c := &clock{at: start}
	return NewLedger(timeout, c.now), c
}

func startOfTest() time.Time { return time.Date(2026, 8, 3, 6, 0, 0, 0, time.UTC) }

// A request carries two names, not one, and the ledger has to hand both back.
// The capability request id is what the phone's sheet is keyed on and what a
// real answer comes back under; the action id is what "we could not find out"
// has to be reported under, because that sentence rides the action_result
// frame. Whichever of the three bad endings a wait meets, the caller only has
// the record to work from — so a record missing either name cannot be reported
// at all.
func waiting(requestID, deviceID string) Record {
	return Record{
		RequestID: requestID,
		DeviceID:  deviceID,
		Kind:      "notification_reply",
		ActionID:  "act-" + requestID,
	}
}

func TestARequestHandedToThePhoneIsRemembered(t *testing.T) {
	ledger, _ := newLedgerAt(startOfTest())

	if !ledger.Wait(waiting("cap-1", device)) {
		t.Fatal("the first hand-over of a request was refused")
	}

	record, settled := ledger.Settle("cap-1")
	if !settled {
		t.Fatal("a request that was handed over could not be settled")
	}
	if record.RequestID != "cap-1" || record.DeviceID != device || record.Kind != "notification_reply" {
		t.Fatalf("the record lost what it was about: %+v", record)
	}
	if record.ActionID != "act-cap-1" {
		t.Fatalf("the record lost the name the bad endings are reported under: %+v", record)
	}
	if !record.StartedAt.Equal(startOfTest()) {
		t.Fatalf("the ledger did not stamp when the wait began: %+v", record)
	}
}

func TestTheSameRequestCannotBeHandedOverTwice(t *testing.T) {
	// A duplicate confirm for one request would otherwise put two waits on the
	// books and fire two device_actions — meaning the same message sent twice
	// to a real person, which is the exact harm this whole area exists to stop.
	ledger, _ := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))

	if ledger.Wait(waiting("cap-1", device)) {
		t.Fatal("the same request was handed to the phone a second time")
	}
}

func TestOnlyTheFirstResultForARequestIsAccepted(t *testing.T) {
	ledger, _ := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))

	if _, settled := ledger.Settle("cap-1"); !settled {
		t.Fatal("the first result was not accepted")
	}
	if _, settled := ledger.Settle("cap-1"); settled {
		t.Fatal("a second result for the same request was accepted")
	}
}

func TestAResultForARequestNobodyIsWaitingOnIsIgnored(t *testing.T) {
	// A phone that reconnects and replays, or one that is simply confused,
	// must not be able to make the Mac emit a capability_result for a request
	// it never handed out.
	ledger, _ := newLedgerAt(startOfTest())

	if _, settled := ledger.Settle("a-request-nobody-made"); settled {
		t.Fatal("a result for an unknown request was accepted")
	}
}

func TestEveryRequestWaitingOnAPhoneThatLeftComesBack(t *testing.T) {
	// The caller needs the full list, not a count: each one has to be reported
	// to the user separately as an outcome we could not learn.
	ledger, _ := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))
	ledger.Wait(waiting("cap-2", device))

	abandoned := ledger.DeviceGone(device)

	if len(abandoned) != 2 {
		t.Fatalf("expected both waiting requests back, got %d: %+v", len(abandoned), abandoned)
	}
	for _, record := range abandoned {
		if _, settled := ledger.Settle(record.RequestID); settled {
			t.Fatalf("%s was still settleable after its phone left", record.RequestID)
		}
	}
}

func TestAPhoneLeavingDoesNotTouchAnotherPhonesRequests(t *testing.T) {
	ledger, _ := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))
	ledger.Wait(waiting("cap-2", other))

	abandoned := ledger.DeviceGone(device)

	if len(abandoned) != 1 || abandoned[0].RequestID != "cap-1" {
		t.Fatalf("the wrong requests were abandoned: %+v", abandoned)
	}
	if _, settled := ledger.Settle("cap-2"); !settled {
		t.Fatal("a request on a phone that is still here was abandoned with it")
	}
}

func TestARequestThatWaitedTooLongComesBack(t *testing.T) {
	ledger, clock := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))

	clock.tick(timeout + time.Second)
	expired := ledger.Expired()

	if len(expired) != 1 || expired[0].RequestID != "cap-1" {
		t.Fatalf("the request did not come back after its deadline: %+v", expired)
	}
}

func TestARequestStillInsideItsWindowIsLeftAlone(t *testing.T) {
	// The guard. A ledger that gave up early would report "we don't know" on a
	// reply that was about to succeed, and block the user's next prompt for
	// nothing — the mirror-image dishonesty.
	ledger, clock := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))

	clock.tick(timeout - time.Second)

	if expired := ledger.Expired(); len(expired) != 0 {
		t.Fatalf("a request still inside its window was given up on: %+v", expired)
	}
}

func TestARequestThatTimedOutCannotThenBeSettled(t *testing.T) {
	// The race that matters: the deadline passes, the user has already been
	// told we could not find out, and then the phone's answer finally arrives.
	// Accepting it would emit a second capability_result and overwrite an
	// honest "we don't know" with a claim — the one direction that must never
	// happen.
	ledger, clock := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))
	clock.tick(timeout + time.Second)
	ledger.Expired()

	if _, settled := ledger.Settle("cap-1"); settled {
		t.Fatal("a late answer was accepted after the request had already been given up on")
	}
}

func TestAskingTwiceForExpiredRequestsReturnsThemOnce(t *testing.T) {
	// Whatever drives this will call it on a timer. Handing the same record
	// back on every tick would tell the user the same thing over and over.
	ledger, clock := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))
	clock.tick(timeout + time.Second)

	first := ledger.Expired()
	second := ledger.Expired()

	if len(first) != 1 {
		t.Fatalf("the first sweep missed it: %+v", first)
	}
	if len(second) != 0 {
		t.Fatalf("the second sweep handed the same record back again: %+v", second)
	}
}

func TestAnEmptyLedgerIsQuiet(t *testing.T) {
	ledger, clock := newLedgerAt(startOfTest())
	clock.tick(timeout * 10)

	if expired := ledger.Expired(); len(expired) != 0 {
		t.Fatalf("an empty ledger produced records: %+v", expired)
	}
	if abandoned := ledger.DeviceGone(device); len(abandoned) != 0 {
		t.Fatalf("an empty ledger produced records: %+v", abandoned)
	}
}

func TestManyGoroutinesCannotSettleTheSameRequestTwice(t *testing.T) {
	// The read loop, the delivery goroutine and the timeout sweep can all
	// reach this at once. Exactly one of them may win.
	ledger, _ := newLedgerAt(startOfTest())
	ledger.Wait(waiting("cap-1", device))

	const racers = 32
	wins := make(chan bool, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		go func() {
			<-start
			_, settled := ledger.Settle("cap-1")
			wins <- settled
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
		t.Fatalf("%d goroutines settled the same request", won)
	}
}
