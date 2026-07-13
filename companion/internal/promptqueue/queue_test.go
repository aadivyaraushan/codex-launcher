package promptqueue

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var queueNow = time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

func TestQueuePersistsOrderAndMarksUnknownBeforeSend(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	for _, entry := range []Entry{
		{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "first", CreatedAt: queueNow},
		{ActionID: "a-2", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "second", CreatedAt: queueNow.Add(time.Second)},
	} {
		if err := queue.Enqueue(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	sender := func(_ context.Context, entry Entry) (Result, error) {
		stored, err := store.Entry(context.Background(), entry.ActionID)
		if err != nil || stored.State != StateSentUnknown {
			t.Fatalf("state at send = %#v, %v", stored, err)
		}
		return Result{Code: "accepted", ThreadID: entry.ThreadID, TurnID: "turn-1"}, nil
	}
	result, err := queue.DispatchNext(context.Background(), "thread-1", sender, nil, queueNow)
	if err != nil || result.Code != "accepted" {
		t.Fatalf("first dispatch = %#v, %v", result, err)
	}
	next, err := store.NextPrepared(context.Background(), "thread-1")
	if err != nil || next.ActionID != "a-2" {
		t.Fatalf("next queued = %#v, %v", next, err)
	}
}

func TestNewTaskQueueUsesAStableQueueKeyAndAcceptsTheCreatedThread(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	entry := Entry{
		ActionID: "action-1", QueueKey: "new:action-1", ProjectID: "project-1", Prompt: "build it",
		Model: "gpt-5.4", Effort: "high", PermissionMode: "workspace-write", CreatedAt: queueNow,
	}
	if err := queue.Enqueue(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	want := Result{Code: "accepted", ThreadID: "thread-created", TurnID: "turn-created"}
	got, err := queue.DispatchNext(context.Background(), entry.QueueKey, func(_ context.Context, stored Entry) (Result, error) {
		if stored.ThreadID != "" || stored.Model != "gpt-5.4" || stored.Effort != "high" || stored.PermissionMode != "workspace-write" {
			t.Fatalf("stored new-task settings = %#v", stored)
		}
		return want, nil
	}, nil, queueNow)
	if err != nil || got != want {
		t.Fatalf("new-task dispatch = %#v, %v", got, err)
	}
}

func TestPreparedWriteFailureIsSafeToRetryWithoutSending(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	if err := queue.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow}); err != nil {
		t.Fatal(err)
	}
	store.FailNextSave(errors.New("disk unavailable"))
	sends := 0
	_, err := queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
		sends++
		return Result{}, nil
	}, nil, queueNow)
	if err == nil || sends != 0 {
		t.Fatalf("dispatch error = %v, sends = %d", err, sends)
	}
	entry, _ := store.Entry(context.Background(), "a-1")
	if entry.State != StatePrepared {
		t.Fatalf("state after pre-send failure = %s", entry.State)
	}
}

func TestUnknownSendNeverBlindlyRetries(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow})
	sends := 0
	_, err := queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
		sends++
		return Result{}, ErrSendOutcomeUnknown
	}, nil, queueNow)
	if !errors.Is(err, ErrOutcomeUnknown) || sends != 1 {
		t.Fatalf("first dispatch = %v, sends = %d", err, sends)
	}
	_, err = queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
		sends++
		return Result{}, nil
	}, func(context.Context, Entry) (Result, bool, error) {
		return Result{}, false, nil
	}, queueNow.Add(time.Second))
	if !errors.Is(err, ErrOutcomeUnknown) || sends != 1 {
		t.Fatalf("retry dispatch = %v, sends = %d", err, sends)
	}
}

func TestDefiniteSendFailureIsDurableTerminalAndClearsPrompt(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-failed", QueueKey: "new:a-failed", ProjectID: "project-1", Prompt: "private prompt", CreatedAt: queueNow})

	_, err := queue.DispatchNext(context.Background(), "new:a-failed", func(context.Context, Entry) (Result, error) {
		return Result{}, ErrSendNotSent
	}, nil, queueNow)
	if !errors.Is(err, ErrSendNotSent) {
		t.Fatalf("dispatch error = %v", err)
	}
	entry, err := queue.Entry(context.Background(), "a-failed")
	if err != nil || entry.State != StateFailed || entry.Prompt != "" || entry.ErrorCode != "send_not_sent" {
		t.Fatalf("failed entry = %#v, error = %v", entry, err)
	}
	if _, err := New(store, nil).DispatchNext(context.Background(), "new:a-failed", nil, nil, queueNow.Add(time.Second)); !errors.Is(err, ErrNoPreparedAction) {
		t.Fatalf("restarted dispatch error = %v", err)
	}
}

func TestDefiniteFailureCommitErrorRemainsUnknownToThePhone(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-failed", QueueKey: "new:a-failed", ProjectID: "project-1", Prompt: "private prompt", CreatedAt: queueNow})
	store.FailSaveNumber(2, errors.New("disk unavailable"))

	_, err := queue.DispatchNext(context.Background(), "new:a-failed", func(context.Context, Entry) (Result, error) {
		return Result{}, ErrSendNotSent
	}, nil, queueNow)
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("failure commit error = %v", err)
	}
	entry, _ := store.Entry(context.Background(), "a-failed")
	if entry.State != StateSentUnknown {
		t.Fatalf("state after failed terminal commit = %s", entry.State)
	}
}

func TestUnknownSendCanBeConfirmedOnlyByReconciliation(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow})
	_, _ = queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
		return Result{}, ErrSendOutcomeUnknown
	}, nil, queueNow)
	want := Result{Code: "accepted", ThreadID: "thread-1", TurnID: "turn-1"}
	got, err := queue.DispatchNext(context.Background(), "thread-1", nil, func(_ context.Context, entry Entry) (Result, bool, error) {
		if entry.State != StateSentUnknown {
			t.Fatalf("reconcile state = %s", entry.State)
		}
		return want, true, nil
	}, queueNow.Add(time.Second))
	if err != nil || got != want {
		t.Fatalf("reconciled = %#v, %v", got, err)
	}
}

func TestConfirmedActionReplaysWithoutSendAfterRestart(t *testing.T) {
	store := NewMemoryStore()
	first := New(store, nil)
	_ = first.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow})
	want := Result{Code: "accepted", ThreadID: "thread-1", TurnID: "turn-1"}
	if _, err := first.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) { return want, nil }, nil, queueNow); err != nil {
		t.Fatal(err)
	}
	restarted := New(store, nil)
	sends := 0
	got, err := restarted.Result(context.Background(), "a-1")
	if err != nil || got != want || sends != 0 {
		t.Fatalf("replayed result = %#v, %v", got, err)
	}
}

func TestResponseCommitFailureRemainsUnknown(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow})
	store.FailSaveNumber(2, errors.New("commit failed"))
	_, err := queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
		return Result{Code: "accepted", ThreadID: "thread-1", TurnID: "turn-1"}, nil
	}, nil, queueNow)
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("commit failure error = %v", err)
	}
	entry, _ := store.Entry(context.Background(), "a-1")
	if entry.State != StateSentUnknown {
		t.Fatalf("state after commit failure = %s", entry.State)
	}
}

func TestDuplicateActionAndDeletedThreadFailClosed(t *testing.T) {
	store := NewMemoryStore()
	queue := New(store, nil)
	entry := Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow}
	if err := queue.Enqueue(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(context.Background(), entry); !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := queue.CancelThread(context.Background(), "thread-1", "thread_deleted", queueNow); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.DispatchNext(context.Background(), "thread-1", nil, nil, queueNow); !errors.Is(err, ErrNoPreparedAction) {
		t.Fatalf("deleted thread dispatch = %v", err)
	}
}

func TestConcurrentDispatchClaimsPreparedActionOnlyOnce(t *testing.T) {
	store := &dispatchBarrierStore{MemoryStore: NewMemoryStore(), bothRead: make(chan struct{})}
	queue := New(store, nil)
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: "hello", CreatedAt: queueNow})
	var sends atomic.Int32
	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			_, err := queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
				sends.Add(1)
				return Result{Code: "accepted", ThreadID: "thread-1", TurnID: "turn-1"}, nil
			}, nil, queueNow)
			results <- err
		}()
	}
	first, second := <-results, <-results
	if sends.Load() != 1 {
		t.Fatalf("send count = %d, errors = %v / %v", sends.Load(), first, second)
	}
}

func TestCallbackErrorsNeverLeakPromptContentToLogs(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := NewMemoryStore()
	queue := New(store, logger)
	prompt := "private prompt contents"
	_ = queue.Enqueue(context.Background(), Entry{ActionID: "a-1", ThreadID: "thread-1", ProjectID: "project-1", Prompt: prompt, CreatedAt: queueNow})
	_, _ = queue.DispatchNext(context.Background(), "thread-1", func(context.Context, Entry) (Result, error) {
		return Result{}, errors.New("provider accidentally included " + prompt)
	}, nil, queueNow)
	_, _ = queue.DispatchNext(context.Background(), "thread-1", nil, func(context.Context, Entry) (Result, bool, error) {
		return Result{}, false, errors.New("reconciler accidentally included " + prompt)
	}, queueNow)
	if strings.Contains(output.String(), prompt) {
		t.Fatalf("callback content leaked to logs: %s", output.String())
	}
	if !strings.Contains(output.String(), "error_class=external_error") {
		t.Fatalf("safe error class missing: %s", output.String())
	}
}

type dispatchBarrierStore struct {
	*MemoryStore
	mu        sync.Mutex
	readCount int
	bothRead  chan struct{}
	closeOnce sync.Once
}

func (store *dispatchBarrierStore) NextPending(ctx context.Context, threadID string) (Entry, error) {
	entry, err := store.MemoryStore.NextPending(ctx, threadID)
	if err != nil {
		return Entry{}, err
	}
	store.mu.Lock()
	store.readCount++
	if store.readCount == 2 {
		store.closeOnce.Do(func() { close(store.bothRead) })
	}
	store.mu.Unlock()
	select {
	case <-store.bothRead:
	case <-time.After(100 * time.Millisecond):
	}
	return entry, nil
}
