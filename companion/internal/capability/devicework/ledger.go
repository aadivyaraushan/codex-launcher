// Package devicework is the bookkeeper for capability requests whose work
// has to finish on the phone instead of the Mac. Everything up to this point
// runs inside one function call and returns an Outcome directly; once a
// result has to travel back over the wire, something has to remember which
// requests are still open and who is expected to answer for them. This file
// is that memory, and nothing more: it holds no adapters, sends nothing, and
// carries no user-facing wording.
package devicework

import (
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Record is one outstanding request: who it was handed to, what kind of
// work it is, and when the wait for an answer began. StartedAt is what lets
// the ledger tell a request that is merely slow from one that has genuinely
// run out of time, without ever consulting a real clock itself.
//
// A request carries two names, not one. RequestID is what the phone's
// capability sheet is keyed on, and what a real answer comes back under, on
// the capability_result frame. ActionID is what "we could not find out" has
// to be reported under instead, because that sentence rides the
// action_result frame, not the capability one. Every one of the three bad
// endings this package exists for hands the caller only a Record, so a
// Record missing either name would leave that ending unreportable.
//
// AdapterID and Ceiling exist so the answer that eventually comes back can
// be measured against something. Without them, the code answering the phone
// has no way to know which adapter asked or what it was ever allowed to
// claim — Ceiling is stamped in already resolved (never blank; see
// manifest.Ceiling.OrHandsOff), so nothing downstream has to re-decide what
// an unset value means.
type Record struct {
	RequestID string
	DeviceID  string
	Kind      string
	ActionID  string
	AdapterID string
	Ceiling   manifest.Ceiling
	StartedAt time.Time
}

// Ledger tracks every request currently waiting on a phone to answer. It has
// no goroutines of its own — timeouts and disconnects are only noticed when
// a caller asks about them — because a background sweeper would make the
// exact deadline a test observes unpredictable. The mutex exists because the
// same request can be settled, expired, or abandoned from three different
// places at once (a read loop, a delivery goroutine, a timeout sweep), and
// only one of those attempts is allowed to win.
type Ledger struct {
	mu      sync.Mutex
	waiting map[string]Record
	timeout time.Duration
	now     func() time.Time
}

// NewLedger builds a Ledger that gives up on a request after timeout has
// elapsed since it started waiting. now is injected rather than read from
// the system clock so that tests can step past a deadline instantly instead
// of sleeping for it.
func NewLedger(timeout time.Duration, now func() time.Time) *Ledger {
	return &Ledger{
		waiting: make(map[string]Record),
		timeout: timeout,
		now:     now,
	}
}

// Wait records that record has been handed to its device and is now
// awaiting a reply. The caller leaves StartedAt zero; Wait stamps it with
// the ledger's own clock and stores that stamped copy, so the caller cannot
// accidentally record a wait that started at the wrong time. It returns
// false, and records nothing, if that request is already on the books —
// otherwise a duplicate confirm would put two waits on the same request and
// cause the same action to be sent to a real person twice.
func (l *Ledger) Wait(record Record) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.waiting[record.RequestID]; exists {
		return false
	}
	record.StartedAt = l.now()
	l.waiting[record.RequestID] = record
	return true
}

// Settle claims the result for requestID. Only the first caller for a given
// request gets true and the Record back; every later caller — including one
// that arrives after DeviceGone or Expired has already given up on the
// request — gets false. That second-caller case is not a technicality: a
// late answer accepted after the request was already reported as
// outcome_unknown would overwrite an honest "we don't know" with a claim,
// which is the one direction this ledger must never allow.
func (l *Ledger) Settle(requestID string) (Record, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	record, exists := l.waiting[requestID]
	if !exists {
		return Record{}, false
	}
	delete(l.waiting, requestID)
	return record, true
}

// DeviceGone removes and returns every request still waiting on deviceID,
// leaving requests waiting on other devices untouched. Once a phone has
// disconnected it can never deliver the answer it owed, so every one of
// those requests has to be reported to the user as an outcome we could not
// learn — hence the full list of records, not just a count.
func (l *Ledger) DeviceGone(deviceID string) []Record {
	l.mu.Lock()
	defer l.mu.Unlock()

	var abandoned []Record
	for id, record := range l.waiting {
		if record.DeviceID == deviceID {
			abandoned = append(abandoned, record)
			delete(l.waiting, id)
		}
	}
	return abandoned
}

// Expired removes and returns every request that has been waiting longer
// than the ledger's timeout, as of the injected clock's current time. A
// caller is expected to call this on a timer; a record is only ever handed
// back once, on the sweep that first notices it has expired, so the same
// request is never reported to the user twice.
func (l *Ledger) Expired() []Record {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	var expired []Record
	for id, record := range l.waiting {
		if now.Sub(record.StartedAt) > l.timeout {
			expired = append(expired, record)
			delete(l.waiting, id)
		}
	}
	return expired
}
