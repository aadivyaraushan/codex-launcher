// Package devicework is the bookkeeper for capability requests whose work
// has to finish on the phone instead of the Mac. A notification reply's
// reply box, and a YouTube player, both live inside apps only the phone can
// reach — so an adapter that needs one refuses with an
// *adapter.DeviceWorkError, the agent bridge hands the resulting Ask to
// mobilesession.Handler.RunOnDevice, and that call blocks until the phone
// answers. This file is what lets one goroutine send an Ask out to the
// phone and a different goroutine — the one reading the phone's
// device_action_result — hand the Result back to it. It holds no adapters,
// sends nothing itself, and carries no user-facing wording.
package devicework

import (
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Timeout is how long a caller should wait for the phone to answer an Ask
// before giving up on it. A notification reply either lands within seconds
// or it is not going to land at all, and the cost of waiting longer than
// that is not patience — it is the user's next prompt sitting blocked
// behind a reply that was never coming. It lives here, not in
// mobilesession, so agentbridge can share the same number without
// importing mobilesession.
const Timeout = 60 * time.Second

// Ask is what an adapter handed back instead of finishing itself: the least
// a caller needs to hand the same instruction to the phone. Handle and Text
// are things a real person wrote — a contact's name and the words routed to
// them — so nothing that carries an Ask may log them. Ceiling is the most
// this ask will ever claim to have done, the same promise every other
// adapter makes through execution.Runner's clamp; device work skips that
// runner, so this is where the promise is written down before the phone
// answers.
type Ask struct {
	AdapterID string
	Kind      string
	Handle    string
	Text      string
	Ceiling   manifest.Ceiling
}

// Result is the phone's answer to an Ask, already turned from its one
// outcome word into what the caller needs. Answered is false for every way
// an Ask can go unanswered — the wait timed out, the phone disconnected, or
// a fresh session orphaned it — and Done is always false alongside it: none
// of those put anything in anybody's chat, so neither "done" nor "failed"
// would be honest.
type Result struct {
	Answered bool
	Reached  manifest.Ceiling
	Done     bool
	Detail   string
}

// Record is one outstanding request: who it was handed to and what kind of
// work it is. AdapterID and Ceiling exist so the answer that eventually
// comes back can be measured against something — Ceiling is stamped in
// already resolved (never blank; see manifest.Ceiling.OrHandsOff), so
// nothing downstream has to re-decide what an unset value means.
type Record struct {
	RequestID string
	DeviceID  string
	Kind      string
	AdapterID string
	Ceiling   manifest.Ceiling
}

// disconnectedDetail is what a waiter is told when its phone leaves before
// answering — either a real disconnect or a fresh session that has no
// memory of the ask (device_action is never replayed). Both are the same
// honest non-answer: whether the phone sent it before it left is exactly
// what nobody can find out.
const disconnectedDetail = "The phone disconnected before answering; whether it went out is unknown."

type waiter struct {
	record Record
	result chan Result
}

// Ledger tracks every Ask currently waiting on a phone to answer. It runs no
// timeout of its own — the caller that registered the wait owns its own
// deadline — because the one place that timeout would fire, RunOnDevice,
// already has a context to watch. The mutex exists because the same request
// can be settled and given up on from two different places at once (the
// read loop delivering an answer, a hello handler declaring the phone
// gone), and only one of those attempts is allowed to win.
type Ledger struct {
	mu      sync.Mutex
	waiting map[string]waiter
}

// NewLedger returns an empty Ledger.
func NewLedger() *Ledger {
	return &Ledger{waiting: make(map[string]waiter)}
}

// Wait records that record has been handed to its device and is now
// awaiting a reply, and returns the channel its eventual Result will arrive
// on. It returns false, and records nothing, if that request id is already
// on the books — otherwise a duplicate hand-off would put two waits on the
// same request and risk the same action reaching a real person twice.
func (l *Ledger) Wait(record Record) (<-chan Result, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.waiting[record.RequestID]; exists {
		return nil, false
	}
	ch := make(chan Result, 1)
	l.waiting[record.RequestID] = waiter{record: record, result: ch}
	return ch, true
}

// Claim removes requestID from the ledger and hands back its Record along
// with the channel its waiter is listening on, so the caller can compute a
// Result — which needs the Record's Kind and Ceiling — before delivering
// it. Only the first caller for a given request id gets true; every later
// caller, including one that arrives after DeviceGone has already given up
// on the request, gets false. That second-caller case is not a technicality:
// a late answer accepted after the request was already reported as
// unanswered would overwrite an honest "we don't know" with a claim, which
// is the one direction this ledger must never allow.
func (l *Ledger) Claim(requestID string) (Record, chan<- Result, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, exists := l.waiting[requestID]
	if !exists {
		return Record{}, nil, false
	}
	delete(l.waiting, requestID)
	return w.record, w.result, true
}

// DeviceGone fails every request still waiting on deviceID with an
// unanswered Result, leaving requests waiting on other devices untouched.
// Once a phone has left it can never deliver the answer it owed, so every
// one of its waiters has to be told the outcome could not be learned rather
// than left blocked until their own caller's deadline.
func (l *Ledger) DeviceGone(deviceID string) {
	l.mu.Lock()
	var abandoned []waiter
	for id, w := range l.waiting {
		if w.record.DeviceID == deviceID {
			abandoned = append(abandoned, w)
			delete(l.waiting, id)
		}
	}
	l.mu.Unlock()

	for _, w := range abandoned {
		w.result <- Result{Answered: false, Done: false, Detail: disconnectedDetail}
	}
}
