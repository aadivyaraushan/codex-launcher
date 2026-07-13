package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

type taskEventPublisher interface {
	PublishTaskEvent(context.Context, taskstate.MobileEvent) error
	RefreshTaskSnapshot(context.Context) error
	TaskSnapshotGeneration() uint64
}

func pumpTaskEvents(ctx context.Context, events <-chan taskstate.MobileEvent, publisher taskEventPublisher, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	published := newPublishedEventCache(contract.MaxSnapshotTasks)
	snapshotGeneration := publisher.TaskSnapshotGeneration()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			if currentGeneration := publisher.TaskSnapshotGeneration(); currentGeneration != snapshotGeneration {
				published.Reset()
				snapshotGeneration = currentGeneration
			}
			if published.Contains(event) {
				continue
			}
			publishGeneration := snapshotGeneration
			if err := publishTaskEvent(ctx, publisher, event, logger); err != nil {
				if errors.Is(err, mobilesession.ErrTaskEventAuthorizationRevoked) {
					logger.Debug("[app] revoked Desktop task event skipped", "thread_id", event.TaskID, "event_kind", event.Kind, "branch_reason", "desktop_authorization_revoked")
					continue
				}
				if ctx.Err() != nil && errors.Is(err, context.Canceled) {
					return
				}
				logger.Error("[app] live task event publish failed", "thread_id", event.TaskID, "event_kind", event.Kind, "error_class", fmt.Sprintf("%T", err), "attempt_count", maxTaskEventPublishAttempts)
				continue
			}
			if currentGeneration := publisher.TaskSnapshotGeneration(); currentGeneration != publishGeneration {
				published.Reset()
				snapshotGeneration = currentGeneration
				continue
			}
			published.Record(event)
		}
	}
}

const maxTaskEventPublishAttempts = 3

func publishTaskEvent(ctx context.Context, publisher taskEventPublisher, event taskstate.MobileEvent, logger *slog.Logger) error {
	refreshed := false
	var lastErr error
	for attempt := 1; attempt <= maxTaskEventPublishAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = publisher.PublishTaskEvent(ctx, event)
		if errors.Is(lastErr, mobilesession.ErrTaskEventAuthorizationRevoked) {
			return lastErr
		}
		if lastErr == nil {
			return nil
		}
		if errors.Is(lastErr, mobilesession.ErrUnknownTaskEvent) && !refreshed {
			if refreshErr := publisher.RefreshTaskSnapshot(ctx); refreshErr != nil {
				return fmt.Errorf("refresh task snapshot: %w", refreshErr)
			}
			refreshed = true
			logger.Info("[app] mobile task catalog refreshed", "thread_id", event.TaskID, "branch_reason", "unknown_live_task")
			continue
		}
		logger.Warn("[app] live task event publish retry", "thread_id", event.TaskID, "event_kind", event.Kind, "attempt", attempt, "error_class", fmt.Sprintf("%T", lastErr))
	}
	return lastErr
}

type publishedEventCache struct {
	limit   int
	entries map[string]taskstate.MobileEvent
	order   []string
}

func newPublishedEventCache(limit int) *publishedEventCache {
	return &publishedEventCache{limit: limit, entries: make(map[string]taskstate.MobileEvent)}
}

func (cache *publishedEventCache) Contains(event taskstate.MobileEvent) bool {
	previous, exists := cache.entries[event.TaskID]
	return exists && previous == event
}

func (cache *publishedEventCache) Record(event taskstate.MobileEvent) {
	if _, exists := cache.entries[event.TaskID]; exists {
		cache.entries[event.TaskID] = event
		return
	}
	if len(cache.order) == cache.limit {
		delete(cache.entries, cache.order[0])
		cache.order = cache.order[1:]
	}
	cache.entries[event.TaskID] = event
	cache.order = append(cache.order, event.TaskID)
}

func (cache *publishedEventCache) Reset() {
	clear(cache.entries)
	cache.order = cache.order[:0]
}
