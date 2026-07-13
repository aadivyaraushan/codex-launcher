package eventjournal

import (
	"context"
	"sync"
)

type Limits struct {
	MaxEvents int
	MaxBytes  int
}

type Bounds struct {
	Earliest uint64
	Latest   uint64
	Count    int
}

type Store interface {
	Append(context.Context, Event) (Event, error)
	ReplayAfter(context.Context, uint64) ([]Event, error)
	Bounds(context.Context) (Bounds, error)
	EnsureBase(context.Context) (uint64, error)
	Acknowledge(context.Context, string, uint64) error
	Acknowledged(context.Context, string) (uint64, error)
}

type MemoryStore struct {
	mu           sync.RWMutex
	limits       Limits
	nextSequence uint64
	events       []Event
	retained     int
	acks         map[string]uint64
}

func NewMemoryStore(limits Limits) *MemoryStore {
	return &MemoryStore{limits: limits, acks: make(map[string]uint64)}
}

func (store *MemoryStore) Append(ctx context.Context, event Event) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	size := eventSize(event)
	if store.limits.MaxEvents < 1 || store.limits.MaxBytes < 1 || size > store.limits.MaxBytes {
		return Event{}, ErrInvalidEvent
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.nextSequence++
	event.Sequence = store.nextSequence
	event.Body = append([]byte(nil), event.Body...)
	store.events = append(store.events, event)
	store.retained += size
	for len(store.events) > store.limits.MaxEvents || store.retained > store.limits.MaxBytes {
		store.retained -= eventSize(store.events[0])
		store.events = store.events[1:]
	}
	return cloneEvent(event), nil
}

func (store *MemoryStore) ReplayAfter(ctx context.Context, sequence uint64) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	events := make([]Event, 0)
	for _, event := range store.events {
		if event.Sequence > sequence {
			events = append(events, cloneEvent(event))
		}
	}
	return events, nil
}

func (store *MemoryStore) Bounds(ctx context.Context) (Bounds, error) {
	if err := ctx.Err(); err != nil {
		return Bounds{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	bounds := Bounds{Latest: store.nextSequence, Count: len(store.events)}
	if len(store.events) != 0 {
		bounds.Earliest = store.events[0].Sequence
	}
	return bounds, nil
}

func (store *MemoryStore) EnsureBase(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.nextSequence == 0 {
		store.nextSequence = 1
	}
	return store.nextSequence, nil
}

func (store *MemoryStore) Acknowledge(ctx context.Context, deviceID string, sequence uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.acks[deviceID] = sequence
	return nil
}

func (store *MemoryStore) Acknowledged(ctx context.Context, deviceID string) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.acks[deviceID], nil
}

func cloneEvent(event Event) Event {
	event.Body = append([]byte(nil), event.Body...)
	return event
}

func eventSize(event Event) int { return len(event.Name) + len(event.Body) + 32 }

var _ Store = (*MemoryStore)(nil)
