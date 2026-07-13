package eventjournal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"
)

var (
	ErrAckBackward      = errors.New("event acknowledgement moved backward")
	ErrAckBeyondJournal = errors.New("event acknowledgement exceeds the journal")
	ErrCursorCompacted  = errors.New("event cursor was compacted")
	ErrInvalidEvent     = errors.New("journal event is invalid")
	ErrInvalidSnapshot  = errors.New("journal snapshot is invalid")
)

var eventNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

type Event struct {
	Sequence  uint64
	Name      string
	Body      json.RawMessage
	CreatedAt time.Time
}

type Snapshot struct {
	BaseSequence uint64
	Body         json.RawMessage
	CreatedAt    time.Time
}

type StateReducer func(current json.RawMessage, event Event) (json.RawMessage, error)

type Journal struct {
	store         Store
	logger        *slog.Logger
	mu            sync.Mutex
	state         json.RawMessage
	stateSequence uint64
}

func New(store Store, logger *slog.Logger) *Journal {
	if logger == nil {
		logger = slog.Default()
	}
	return &Journal{store: store, logger: logger}
}

func (journal *Journal) Apply(ctx context.Context, name string, body json.RawMessage, createdAt time.Time, reduce StateReducer) (Event, error) {
	if journal == nil || journal.store == nil || !eventNamePattern.MatchString(name) || !json.Valid(body) || len(body) == 0 || createdAt.IsZero() {
		return Event{}, ErrInvalidEvent
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(journal.state) == 0 || reduce == nil {
		return Event{}, ErrInvalidSnapshot
	}
	pending := Event{Name: name, Body: append(json.RawMessage(nil), body...), CreatedAt: createdAt}
	nextState, err := reduce(append(json.RawMessage(nil), journal.state...), cloneEvent(pending))
	if err != nil {
		journal.logger.Error("[event-journal] state reduction failed", "event_name", name, "error_class", "state_reducer_error")
		return Event{}, fmt.Errorf("reduce journal state: %w", err)
	}
	if !json.Valid(nextState) || len(nextState) == 0 || len(nextState) > 1024*1024 {
		return Event{}, ErrInvalidSnapshot
	}
	event, err := journal.store.Append(ctx, pending)
	if err != nil {
		if errors.Is(err, ErrInvalidEvent) {
			return Event{}, ErrInvalidEvent
		}
		journal.logger.Error("[event-journal] append failed", "event_name", name, "error", err)
		return Event{}, fmt.Errorf("append event journal: %w", err)
	}
	journal.state = append(journal.state[:0], nextState...)
	journal.stateSequence = event.Sequence
	journal.logger.Debug("[event-journal] event appended", "event_name", name, "sequence", event.Sequence)
	return event, nil
}

func (journal *Journal) ReplayAfter(ctx context.Context, cursor uint64) ([]Event, error) {
	if journal == nil || journal.store == nil {
		return nil, ErrCursorCompacted
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	bounds, err := journal.store.Bounds(ctx)
	if err != nil {
		return nil, err
	}
	if bounds.Count != 0 && cursor+1 < bounds.Earliest {
		journal.logger.Info("[event-journal] replay requires snapshot", "cursor", cursor, "earliest_sequence", bounds.Earliest, "latest_sequence", bounds.Latest, "branch_reason", "cursor_compacted")
		return nil, ErrCursorCompacted
	}
	if cursor > bounds.Latest {
		return nil, ErrAckBeyondJournal
	}
	events, err := journal.store.ReplayAfter(ctx, cursor)
	if err != nil {
		return nil, err
	}
	for index, event := range events {
		if event.Sequence != cursor+uint64(index)+1 {
			return nil, ErrCursorCompacted
		}
	}
	if uint64(len(events)) != bounds.Latest-cursor {
		return nil, ErrCursorCompacted
	}
	journal.logger.Info("[event-journal] replay prepared", "cursor", cursor, "output_count", len(events))
	return events, nil
}

func (journal *Journal) Acknowledge(ctx context.Context, deviceID string, through uint64) error {
	if journal == nil || journal.store == nil || deviceID == "" || through == 0 {
		return ErrAckBeyondJournal
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	bounds, err := journal.store.Bounds(ctx)
	if err != nil {
		return err
	}
	if through > bounds.Latest {
		return ErrAckBeyondJournal
	}
	previous, err := journal.store.Acknowledged(ctx, deviceID)
	if err != nil {
		return err
	}
	if through < previous {
		return ErrAckBackward
	}
	if err := journal.store.Acknowledge(ctx, deviceID, through); err != nil {
		return err
	}
	journal.logger.Debug("[event-journal] events acknowledged", "device_id", deviceID, "through_sequence", through)
	return nil
}

func (journal *Journal) InitializeSnapshot(ctx context.Context, body json.RawMessage, createdAt time.Time) (Snapshot, error) {
	if journal == nil || journal.store == nil || !json.Valid(body) || len(body) == 0 || len(body) > 1024*1024 || createdAt.IsZero() {
		return Snapshot{}, ErrInvalidSnapshot
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(journal.state) != 0 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	baseSequence, err := journal.store.EnsureBase(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	journal.state = append(journal.state[:0], body...)
	journal.stateSequence = baseSequence
	snapshot := Snapshot{BaseSequence: baseSequence, Body: append(json.RawMessage(nil), body...), CreatedAt: createdAt}
	journal.logger.Info("[event-journal] snapshot built", "base_sequence", snapshot.BaseSequence)
	return snapshot, nil
}

func (journal *Journal) ReplaceSnapshot(ctx context.Context, body json.RawMessage, createdAt time.Time) (Snapshot, error) {
	if journal == nil || journal.store == nil || !json.Valid(body) || len(body) == 0 || len(body) > 1024*1024 || createdAt.IsZero() {
		return Snapshot{}, ErrInvalidSnapshot
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(journal.state) == 0 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	baseSequence, err := journal.store.ReplaceBase(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	journal.state = append(journal.state[:0], body...)
	journal.stateSequence = baseSequence
	snapshot := Snapshot{BaseSequence: baseSequence, Body: append(json.RawMessage(nil), body...), CreatedAt: createdAt}
	journal.logger.Info("[event-journal] snapshot replaced", "base_sequence", snapshot.BaseSequence, "branch_reason", "fresh_source_state")
	return snapshot, nil
}

func (journal *Journal) Snapshot(createdAt time.Time) (Snapshot, error) {
	if journal == nil || createdAt.IsZero() {
		return Snapshot{}, ErrInvalidSnapshot
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(journal.state) == 0 || journal.stateSequence == 0 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	return Snapshot{BaseSequence: journal.stateSequence, Body: append(json.RawMessage(nil), journal.state...), CreatedAt: createdAt}, nil
}
