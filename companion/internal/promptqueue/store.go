package promptqueue

import (
	"context"
	"sort"
	"sync"
)

type Store interface {
	Create(context.Context, Entry) error
	CompareAndSwap(context.Context, State, Entry) error
	Entry(context.Context, string) (Entry, error)
	NextPending(context.Context, string) (Entry, error)
	NextPrepared(context.Context, string) (Entry, error)
	ThreadEntries(context.Context, string) ([]Entry, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	entries   map[string]Entry
	failAt    int
	saveCount int
	failError error
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]Entry)}
}

func (store *MemoryStore) Create(ctx context.Context, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.entries[entry.ActionID]; exists {
		return ErrDuplicateAction
	}
	store.entries[entry.ActionID] = entry
	return nil
}

func (store *MemoryStore) CompareAndSwap(ctx context.Context, expected State, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.saveCount++
	if store.failAt > 0 && store.saveCount == store.failAt {
		err := store.failError
		store.failAt = 0
		store.failError = nil
		return err
	}
	current, exists := store.entries[entry.ActionID]
	if !exists {
		return ErrActionNotFound
	}
	if current.State != expected {
		return ErrStateConflict
	}
	store.entries[entry.ActionID] = entry
	return nil
}

func (store *MemoryStore) Entry(ctx context.Context, actionID string) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	entry, exists := store.entries[actionID]
	if !exists {
		return Entry{}, ErrActionNotFound
	}
	return entry, nil
}

func (store *MemoryStore) NextPending(ctx context.Context, queueKey string) (Entry, error) {
	return store.next(ctx, queueKey, func(state State) bool { return state == StatePrepared || state == StateSentUnknown })
}

func (store *MemoryStore) NextPrepared(ctx context.Context, queueKey string) (Entry, error) {
	return store.next(ctx, queueKey, func(state State) bool { return state == StatePrepared })
}

func (store *MemoryStore) next(ctx context.Context, queueKey string, include func(State) bool) (Entry, error) {
	entries, err := store.ThreadEntries(ctx, queueKey)
	if err != nil {
		return Entry{}, err
	}
	for _, entry := range entries {
		if include(entry.State) {
			return entry, nil
		}
	}
	return Entry{}, ErrNoPreparedAction
}

func (store *MemoryStore) ThreadEntries(ctx context.Context, queueKey string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	entries := make([]Entry, 0)
	for _, entry := range store.entries {
		if entry.QueueKey == queueKey {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].CreatedAt.Equal(entries[right].CreatedAt) {
			return entries[left].ActionID < entries[right].ActionID
		}
		return entries[left].CreatedAt.Before(entries[right].CreatedAt)
	})
	return entries, nil
}

func (store *MemoryStore) FailNextSave(err error) {
	store.FailSaveNumber(1, err)
}

func (store *MemoryStore) FailSaveNumber(number int, err error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.saveCount = 0
	store.failAt = number
	store.failError = err
}

var _ Store = (*MemoryStore)(nil)
