package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

func TestPumpTaskEventsPublishesEveryProjectedUpdate(t *testing.T) {
	events := make(chan taskstate.MobileEvent, 2)
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "reply", State: taskstate.IdleAfterReply, Summary: "Codex replied"}
	close(events)
	publisher := &recordingTaskEventPublisher{}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if len(publisher.events) != 2 || publisher.events[0].Summary != "Codex is working" || publisher.events[1].Summary != "Codex replied" {
		t.Fatalf("published = %#v", publisher.events)
	}
}

func TestPumpTaskEventsDoesNotReopenTerminalTaskForLateActivity(t *testing.T) {
	events := make(chan taskstate.MobileEvent, 3)
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "interrupted", State: taskstate.Interrupted, Summary: "Codex was interrupted"}
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Running a command"}
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working", StartsTurn: true}
	close(events)
	publisher := &recordingTaskEventPublisher{}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if len(publisher.events) != 2 || publisher.events[0].State != taskstate.Interrupted || !publisher.events[1].StartsTurn {
		t.Fatalf("published = %#v, want terminal event then explicit new turn", publisher.events)
	}
}

func TestPumpTaskEventsDoesNotHoldAuthorizationWhileWaitingForPublisher(t *testing.T) {
	authorization := taskstate.NewEventAuthorization()
	events := make(chan taskstate.MobileEvent, 1)
	events <- taskstate.MobileEvent{
		TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working",
		Authorization: authorization,
	}
	close(events)
	publisher := &authorizationOrderedPublisher{authorization: authorization, publishStarted: make(chan struct{})}
	publisher.mu.Lock()
	done := make(chan struct{})
	go func() {
		pumpTaskEvents(context.Background(), events, publisher, nil)
		close(done)
	}()
	<-publisher.publishStarted
	revoked := make(chan struct{})
	go func() {
		authorization.Revoke()
		close(revoked)
	}()
	select {
	case <-revoked:
	case <-time.After(200 * time.Millisecond):
		publisher.mu.Unlock()
		t.Fatal("authorization revoke deadlocked behind publisher lock")
	}
	publisher.mu.Unlock()
	<-done
	if publisher.published != 0 {
		t.Fatalf("revoked Desktop events published = %d", publisher.published)
	}
}

func TestPumpTaskEventsDeduplicatesOnlyAfterDurablePublicationSucceeds(t *testing.T) {
	event := taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	events := make(chan taskstate.MobileEvent, 2)
	events <- event
	events <- event
	close(events)
	publisher := &recordingTaskEventPublisher{publishErrors: []error{errors.New("temporary journal failure")}}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if len(publisher.events) != 2 {
		t.Fatalf("publish attempts = %d, want one retry followed by deduplication", len(publisher.events))
	}
}

func TestPumpTaskEventsNeverRetriesPermanentlyRevokedDesktopEvent(t *testing.T) {
	events := make(chan taskstate.MobileEvent, 1)
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	close(events)
	publisher := &recordingTaskEventPublisher{publishErrors: []error{mobilesession.ErrTaskEventAuthorizationRevoked}}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if len(publisher.events) != 1 {
		t.Fatalf("revoked Desktop event publish attempts = %d, want 1", len(publisher.events))
	}
}

func TestPumpTaskEventsRefreshesUnknownTaskAndRetries(t *testing.T) {
	events := make(chan taskstate.MobileEvent, 1)
	events <- taskstate.MobileEvent{TaskID: "thread-new", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	close(events)
	publisher := &recordingTaskEventPublisher{publishErrors: []error{mobilesession.ErrUnknownTaskEvent}}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if publisher.refreshes != 1 || len(publisher.events) != 2 {
		t.Fatalf("refreshes=%d publish attempts=%d", publisher.refreshes, len(publisher.events))
	}
}

func TestPumpTaskEventsKeepsRefreshingUntilANewTaskEntersTheCatalog(t *testing.T) {
	events := make(chan taskstate.MobileEvent, 1)
	events <- taskstate.MobileEvent{TaskID: "thread-new", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	close(events)
	publisher := &recordingTaskEventPublisher{publishErrors: []error{
		mobilesession.ErrUnknownTaskEvent,
		mobilesession.ErrUnknownTaskEvent,
	}}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if publisher.refreshes != 2 || len(publisher.events) != 3 {
		t.Fatalf("refreshes=%d publish attempts=%d, want one catalog refresh before each retry", publisher.refreshes, len(publisher.events))
	}
}

func TestPumpTaskEventsBoundsPerTaskDeduplicationToSnapshotCapacity(t *testing.T) {
	events := make(chan taskstate.MobileEvent, contract.MaxSnapshotTasks+2)
	first := taskstate.MobileEvent{TaskID: "thread-0", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	events <- first
	for index := 1; index <= contract.MaxSnapshotTasks; index++ {
		events <- taskstate.MobileEvent{TaskID: fmt.Sprintf("thread-%d", index), Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	}
	events <- first
	close(events)
	publisher := &recordingTaskEventPublisher{}

	pumpTaskEvents(context.Background(), events, publisher, nil)

	if len(publisher.events) != contract.MaxSnapshotTasks+2 {
		t.Fatalf("published = %d, want oldest dedupe entry evicted", len(publisher.events))
	}
}

func TestPumpTaskEventsDoesNotDeduplicateAcrossSnapshotReplacement(t *testing.T) {
	event := taskstate.MobileEvent{TaskID: "thread-1", Kind: "activity", State: taskstate.Working, Summary: "Codex is working"}
	events := make(chan taskstate.MobileEvent)
	publisher := &snapshotChangingPublisher{firstPublished: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		pumpTaskEvents(context.Background(), events, publisher, nil)
		close(done)
	}()
	events <- event
	<-publisher.firstPublished
	publisher.generation.Store(1)
	events <- event
	close(events)
	<-done

	publisher.mu.Lock()
	published := len(publisher.events)
	publisher.mu.Unlock()
	if published != 2 {
		t.Fatalf("published = %d, want event republished after snapshot replacement", published)
	}
}

type recordingTaskEventPublisher struct {
	events        []taskstate.MobileEvent
	publishErrors []error
	refreshes     int
	generation    uint64
}

func (publisher *recordingTaskEventPublisher) PublishTaskEvent(_ context.Context, event taskstate.MobileEvent) error {
	publisher.events = append(publisher.events, event)
	if len(publisher.publishErrors) != 0 {
		err := publisher.publishErrors[0]
		publisher.publishErrors = publisher.publishErrors[1:]
		return err
	}
	return nil
}

func (publisher *recordingTaskEventPublisher) RefreshTaskSnapshot(context.Context) error {
	publisher.refreshes++
	publisher.generation++
	return nil
}

func (publisher *recordingTaskEventPublisher) TaskSnapshotGeneration() uint64 {
	return publisher.generation
}

type snapshotChangingPublisher struct {
	mu             sync.Mutex
	events         []taskstate.MobileEvent
	generation     atomic.Uint64
	firstPublished chan struct{}
	once           sync.Once
}

type authorizationOrderedPublisher struct {
	mu             sync.Mutex
	authorization  *taskstate.EventAuthorization
	publishStarted chan struct{}
	startedOnce    sync.Once
	published      int
}

func (publisher *authorizationOrderedPublisher) PublishTaskEvent(_ context.Context, _ taskstate.MobileEvent) error {
	publisher.startedOnce.Do(func() { close(publisher.publishStarted) })
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	_, err := publisher.authorization.RunIfValid(func() error {
		publisher.published++
		return nil
	})
	return err
}

func (*authorizationOrderedPublisher) RefreshTaskSnapshot(context.Context) error { return nil }
func (*authorizationOrderedPublisher) TaskSnapshotGeneration() uint64            { return 0 }

func (publisher *snapshotChangingPublisher) PublishTaskEvent(_ context.Context, event taskstate.MobileEvent) error {
	publisher.mu.Lock()
	publisher.events = append(publisher.events, event)
	publisher.mu.Unlock()
	publisher.once.Do(func() { close(publisher.firstPublished) })
	return nil
}

func (publisher *snapshotChangingPublisher) RefreshTaskSnapshot(context.Context) error { return nil }

func (publisher *snapshotChangingPublisher) TaskSnapshotGeneration() uint64 {
	return publisher.generation.Load()
}
