package eventjournal

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

var journalNow = time.Date(2026, 7, 13, 3, 0, 0, 0, time.UTC)

func TestAppendAndWarmReplayAreContiguousAcrossRestart(t *testing.T) {
	store := NewMemoryStore(Limits{MaxEvents: 8, MaxBytes: 4096})
	journal := New(store, nil)
	initializeTestJournal(t, journal)
	for index, name := range []string{"task.started", "task.delta", "turn.replied"} {
		event, err := applyTestEvent(journal, context.Background(), name, json.RawMessage(`{"threadId":"thread-1"}`), journalNow.Add(time.Duration(index)*time.Second))
		if err != nil || event.Sequence != uint64(index+2) {
			t.Fatalf("append %d = %#v, %v", index, event, err)
		}
	}
	restarted := New(store, nil)
	events, err := restarted.ReplayAfter(context.Background(), 2)
	if err != nil || len(events) != 2 || events[0].Sequence != 3 || events[1].Sequence != 4 {
		t.Fatalf("replay = %#v, %v", events, err)
	}
}

func TestCompactedCursorRequiresSnapshot(t *testing.T) {
	store := NewMemoryStore(Limits{MaxEvents: 2, MaxBytes: 4096})
	journal := New(store, nil)
	initializeTestJournal(t, journal)
	for index := 0; index < 3; index++ {
		_, err := applyTestEvent(journal, context.Background(), "task.delta", json.RawMessage(`{"threadId":"thread-1"}`), journalNow.Add(time.Duration(index)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := journal.ReplayAfter(context.Background(), 0); !errors.Is(err, ErrCursorCompacted) {
		t.Fatalf("compacted replay error = %v", err)
	}
	snapshot, err := journal.Snapshot(journalNow)
	if err != nil || snapshot.BaseSequence != 4 {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
}

func TestAcknowledgementsAreCumulativeAndBounded(t *testing.T) {
	store := NewMemoryStore(Limits{MaxEvents: 8, MaxBytes: 4096})
	journal := New(store, nil)
	initializeTestJournal(t, journal)
	for index := 0; index < 3; index++ {
		_, _ = applyTestEvent(journal, context.Background(), "task.delta", json.RawMessage(`{"threadId":"thread-1"}`), journalNow)
	}
	if err := journal.Acknowledge(context.Background(), "pixel-9", 3); err != nil {
		t.Fatal(err)
	}
	if err := journal.Acknowledge(context.Background(), "pixel-9", 2); !errors.Is(err, ErrAckBackward) {
		t.Fatalf("backward ack = %v", err)
	}
	if err := journal.Acknowledge(context.Background(), "pixel-9", 5); !errors.Is(err, ErrAckBeyondJournal) {
		t.Fatalf("future ack = %v", err)
	}
	if ack, err := store.Acknowledged(context.Background(), "pixel-9"); err != nil || ack != 3 {
		t.Fatalf("stored ack = %d, %v", ack, err)
	}
}

func TestSnapshotWaitsForAtomicStateAndEventCommit(t *testing.T) {
	store := NewMemoryStore(Limits{MaxEvents: 8, MaxBytes: 4096})
	journal := New(store, nil)
	initializeTestJournal(t, journal)
	reducerEntered := make(chan struct{})
	releaseReducer := make(chan struct{})
	applyResult := make(chan Event, 1)
	go func() {
		event, _ := journal.Apply(context.Background(), "task.delta", json.RawMessage(`{"threadId":"thread-1"}`), journalNow, func(json.RawMessage, Event) (json.RawMessage, error) {
			close(reducerEntered)
			<-releaseReducer
			return json.RawMessage(`{"tasks":[{"id":"thread-1","state":"updated"}]}`), nil
		})
		applyResult <- event
	}()
	<-reducerEntered
	snapshotResult := make(chan Snapshot, 1)
	go func() {
		snapshot, _ := journal.Snapshot(journalNow)
		snapshotResult <- snapshot
	}()
	select {
	case <-snapshotResult:
		t.Fatal("snapshot crossed the atomic state/event commit")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseReducer)
	appended := <-applyResult
	snapshot := <-snapshotResult
	if snapshot.BaseSequence != 2 || appended.Sequence != 2 || !json.Valid(snapshot.Body) {
		t.Fatalf("snapshot base = %d, appended seq = %d", snapshot.BaseSequence, appended.Sequence)
	}
}

func TestInitialSnapshotReservesNonzeroBaseSequence(t *testing.T) {
	store := NewMemoryStore(Limits{MaxEvents: 8, MaxBytes: 4096})
	journal := New(store, nil)
	snapshot, err := journal.InitializeSnapshot(context.Background(), json.RawMessage(`{"tasks":[]}`), journalNow)
	if err != nil || snapshot.BaseSequence != 1 {
		t.Fatalf("initial snapshot = %#v, %v", snapshot, err)
	}
	event, err := applyTestEvent(journal, context.Background(), "task.started", json.RawMessage(`{"threadId":"thread-1"}`), journalNow)
	if err != nil || event.Sequence != 2 {
		t.Fatalf("post-snapshot event = %#v, %v", event, err)
	}
}

func TestJournalRejectsInvalidOrOversizedEvents(t *testing.T) {
	journal := New(NewMemoryStore(Limits{MaxEvents: 8, MaxBytes: 64}), nil)
	initializeTestJournal(t, journal)
	for _, test := range []struct {
		name string
		body json.RawMessage
	}{
		{name: "", body: json.RawMessage(`{}`)},
		{name: "task.delta", body: json.RawMessage(`not-json`)},
		{name: "task.delta", body: json.RawMessage(`{"value":"this body is too large for the configured journal"}`)},
	} {
		if _, err := applyTestEvent(journal, context.Background(), test.name, test.body, journalNow); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("append(%q) error = %v", test.name, err)
		}
	}
}

func TestConcurrentAppendsNeverDuplicateSequence(t *testing.T) {
	journal := New(NewMemoryStore(Limits{MaxEvents: 64, MaxBytes: 64 * 1024}), nil)
	initializeTestJournal(t, journal)
	var wait sync.WaitGroup
	sequences := make(chan uint64, 32)
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			event, err := applyTestEvent(journal, context.Background(), "task.delta", json.RawMessage(`{"threadId":"thread-1"}`), journalNow)
			if err != nil {
				t.Error(err)
				return
			}
			sequences <- event.Sequence
		}()
	}
	wait.Wait()
	close(sequences)
	seen := make(map[uint64]bool)
	for sequence := range sequences {
		if seen[sequence] {
			t.Fatalf("duplicate sequence %d", sequence)
		}
		seen[sequence] = true
	}
	if len(seen) != 32 {
		t.Fatalf("sequence count = %d", len(seen))
	}
}

func initializeTestJournal(t *testing.T, journal *Journal) {
	t.Helper()
	if _, err := journal.InitializeSnapshot(context.Background(), json.RawMessage(`{"tasks":[]}`), journalNow); err != nil {
		t.Fatal(err)
	}
}

func applyTestEvent(journal *Journal, ctx context.Context, name string, body json.RawMessage, createdAt time.Time) (Event, error) {
	return journal.Apply(ctx, name, body, createdAt, func(current json.RawMessage, _ Event) (json.RawMessage, error) {
		return current, nil
	})
}
