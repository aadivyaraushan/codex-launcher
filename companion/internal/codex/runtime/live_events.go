package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
)

func projectAppServerEvents(
	ctx context.Context,
	input <-chan appserver.Notification,
	output chan<- taskstate.MobileEvent,
	logger *slog.Logger,
) error {
	defer close(output)
	if logger == nil {
		logger = slog.Default()
	}
	pending := newPendingTaskEvents(taskadapter.MaxRecentCatalogTasks)
	inputOpen := true
	for inputOpen || pending.Len() != 0 {
		if pending.Len() != 0 {
			select {
			case output <- pending.First():
				event := pending.Pop()
				logger.Debug("[codex-runtime] live task event projected", "thread_id", event.TaskID, "event_kind", event.Kind, "task_state", event.State)
				continue
			default:
			}
		}
		var outputCase chan<- taskstate.MobileEvent
		var next taskstate.MobileEvent
		if pending.Len() != 0 {
			outputCase = output
			next = pending.First()
		}
		select {
		case <-ctx.Done():
			return nil
		case outputCase <- next:
			event := pending.Pop()
			logger.Debug("[codex-runtime] live task event projected", "thread_id", event.TaskID, "event_kind", event.Kind, "task_state", event.State)
		case notification, open := <-input:
			if !open {
				inputOpen = false
				input = nil
				continue
			}
			event, err := taskstate.ProjectNotification(notification.Method, notification.Params)
			if errors.Is(err, taskstate.ErrUnsupportedLiveNotification) {
				continue
			}
			if err != nil {
				logger.Error("[codex-runtime] invalid live notification", "method", notification.Method, "error_class", fmt.Sprintf("%T", err), "decision", "close_owned_runtime")
				return fmt.Errorf("project app-server notification %s: %w", notification.Method, err)
			}
			if evicted, didEvict := pending.Upsert(event); didEvict {
				logger.Warn("[codex-runtime] live task event coalescer evicted oldest task", "thread_id", evicted.TaskID, "branch_reason", "recent_task_capacity")
			}
		}
	}
	return nil
}

type pendingTaskEvents struct {
	limit   int
	entries map[string]taskstate.MobileEvent
	order   []string
}

func newPendingTaskEvents(limit int) *pendingTaskEvents {
	return &pendingTaskEvents{limit: limit, entries: make(map[string]taskstate.MobileEvent)}
}

func (pending *pendingTaskEvents) Len() int { return len(pending.order) }

func (pending *pendingTaskEvents) First() taskstate.MobileEvent {
	return pending.entries[pending.order[0]]
}

func (pending *pendingTaskEvents) Pop() taskstate.MobileEvent {
	taskID := pending.order[0]
	event := pending.entries[taskID]
	delete(pending.entries, taskID)
	pending.order = pending.order[1:]
	return event
}

func (pending *pendingTaskEvents) Upsert(event taskstate.MobileEvent) (taskstate.MobileEvent, bool) {
	if _, exists := pending.entries[event.TaskID]; exists {
		pending.entries[event.TaskID] = event
		return taskstate.MobileEvent{}, false
	}
	var evicted taskstate.MobileEvent
	didEvict := false
	if len(pending.order) == pending.limit {
		evicted = pending.Pop()
		didEvict = true
	}
	pending.entries[event.TaskID] = event
	pending.order = append(pending.order, event.TaskID)
	return evicted, didEvict
}
