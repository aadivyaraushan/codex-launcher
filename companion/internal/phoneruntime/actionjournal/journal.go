package actionjournal

import (
	"errors"
	"sync"
)

type State string

const (
	StateConfirmed       State = "confirmed"
	StateDispatching     State = "dispatching"
	StateSubmitted       State = "submitted"
	StateDeliveryUnknown State = "delivery_unknown"
	StateObserved        State = "observed"
)

var (
	ErrHashMismatch = errors.New("action journal: same id with different payload hash")
	ErrNotFound     = errors.New("action journal: action not found")
)

type Record struct {
	ActionID         string
	PayloadHash      string
	AccountID        string
	ChatID           string
	State            State
	PendingMessageID string
	ShouldResend     bool
}

type Memory struct {
	mu      sync.Mutex
	records map[string]Record
}

func NewMemory() *Memory {
	return &Memory{records: map[string]Record{}}
}

func (j *Memory) Confirm(actionID, payloadHash, accountID, chatID string) (Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if existing, ok := j.records[actionID]; ok {
		if existing.PayloadHash != payloadHash {
			return Record{}, ErrHashMismatch
		}
		return existing, nil
	}
	rec := Record{
		ActionID: actionID, PayloadHash: payloadHash, AccountID: accountID, ChatID: chatID,
		State: StateConfirmed,
	}
	j.records[actionID] = rec
	return rec, nil
}

func (j *Memory) MarkDispatching(actionID, payloadHash string) (Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[actionID]
	if !ok {
		return Record{}, ErrNotFound
	}
	if rec.PayloadHash != payloadHash {
		return Record{}, ErrHashMismatch
	}
	rec.State = StateDispatching
	j.records[actionID] = rec
	return rec, nil
}

func (j *Memory) MarkSubmitted(actionID, payloadHash, pendingID string) (Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[actionID]
	if !ok {
		return Record{}, ErrNotFound
	}
	if rec.PayloadHash != payloadHash {
		return Record{}, ErrHashMismatch
	}
	rec.State = StateSubmitted
	rec.PendingMessageID = pendingID
	rec.ShouldResend = false
	j.records[actionID] = rec
	return rec, nil
}

func (j *Memory) CrashRecover(actionID string) (Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[actionID]
	if !ok {
		return Record{}, ErrNotFound
	}
	switch rec.State {
	case StateDispatching:
		if rec.PendingMessageID == "" {
			rec.State = StateDeliveryUnknown
			rec.ShouldResend = false
			j.records[actionID] = rec
		}
	case StateSubmitted:
		rec.ShouldResend = false
	}
	return rec, nil
}

func (j *Memory) MarkObserved(actionID, pendingID string) (Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[actionID]
	if !ok {
		return Record{}, ErrNotFound
	}
	if rec.PendingMessageID != pendingID {
		return Record{}, errors.New("action journal: pending id mismatch")
	}
	rec.State = StateObserved
	rec.ShouldResend = false
	j.records[actionID] = rec
	return rec, nil
}
