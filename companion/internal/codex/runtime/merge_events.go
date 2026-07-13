package runtime

import (
	"context"
	"log/slog"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

func mergeTaskEvents(
	ctx context.Context,
	output chan<- taskstate.MobileEvent,
	logger *slog.Logger,
	first <-chan taskstate.MobileEvent,
	second <-chan taskstate.MobileEvent,
) {
	defer close(output)
	if logger == nil {
		logger = slog.Default()
	}
	pending := newPendingTaskEvents(maxMergedTaskEvents)
	for first != nil || second != nil || pending.Len() != 0 {
		if pending.Len() != 0 {
			select {
			case output <- pending.First():
				pending.Pop()
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
			return
		case outputCase <- next:
			pending.Pop()
		case event, open := <-first:
			if !open {
				first = nil
				continue
			}
			if evicted, didEvict := pending.Upsert(event); didEvict {
				logger.Warn("[codex-runtime] merged task event evicted oldest task", "thread_id", evicted.TaskID, "branch_reason", "recent_task_capacity")
			}
		case event, open := <-second:
			if !open {
				second = nil
				continue
			}
			if evicted, didEvict := pending.Upsert(event); didEvict {
				logger.Warn("[codex-runtime] merged task event evicted oldest task", "thread_id", evicted.TaskID, "branch_reason", "recent_task_capacity")
			}
		}
	}
}

const maxMergedTaskEvents = 20
