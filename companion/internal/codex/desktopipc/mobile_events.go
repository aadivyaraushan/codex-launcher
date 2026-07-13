package desktopipc

import (
	"encoding/json"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

const maxPendingMobileTasks = 20

func (client *Client) TaskEvents() <-chan taskstate.MobileEvent {
	if client == nil || client.mobileEvents == nil {
		closed := make(chan taskstate.MobileEvent)
		close(closed)
		return closed
	}
	client.mobileEventOnce.Do(func() { go client.deliverMobileEvents() })
	return client.mobileEvents
}

func (client *Client) queueMobileState(threadID string, materialized json.RawMessage) {
	client.mobileEventMu.Lock()
	queued := client.queueMobileStateLocked(threadID, materialized)
	client.mobileEventMu.Unlock()
	if !queued {
		return
	}
	client.signalMobileEvent()
}

func (client *Client) queueMobileStateLocked(threadID string, materialized json.RawMessage) bool {
	if !client.mobileVerified[threadID] {
		return false
	}
	task, err := taskstate.MapDesktopConversationState(materialized)
	if err != nil || task.ID != threadID {
		client.logger.Debug("[desktop-ipc] mobile state projection skipped", "thread_id", threadID, "branch_reason", "state_not_mobile_safe")
		return false
	}
	event, err := taskstate.ProjectTaskState(task.ID, task.State)
	if err != nil {
		client.logger.Debug("[desktop-ipc] mobile state projection skipped", "thread_id", threadID, "branch_reason", "state_not_mobile_safe")
		return false
	}
	if _, exists := client.mobilePending[threadID]; exists {
		client.mobilePending[threadID] = event
	} else {
		if len(client.mobileOrder) == maxPendingMobileTasks {
			evicted := client.mobileOrder[0]
			client.mobileOrder = client.mobileOrder[1:]
			delete(client.mobilePending, evicted)
			client.logger.Warn("[desktop-ipc] mobile state projection evicted oldest task", "thread_id", evicted, "branch_reason", "recent_task_capacity")
		}
		client.mobilePending[threadID] = event
		client.mobileOrder = append(client.mobileOrder, threadID)
	}
	return true
}

func (client *Client) signalMobileEvent() {
	select {
	case client.mobileSignal <- struct{}{}:
	default:
	}
}

func (client *Client) markMobileStreamVerified(threadID string, stream *StreamState) {
	if stream == nil {
		return
	}
	client.mobileEventMu.Lock()
	client.mobileVerified[threadID] = true
	queued := client.queueMobileStateLocked(threadID, stream.State().Materialized)
	client.mobileEventMu.Unlock()
	if queued {
		client.signalMobileEvent()
	}
}

func (client *Client) deliverMobileEvents() {
	defer close(client.mobileEvents)
	for {
		select {
		case <-client.done:
			return
		case <-client.mobileSignal:
		}
		for {
			event, okay := client.popMobileEvent()
			if !okay {
				break
			}
			select {
			case <-client.done:
				return
			case client.mobileEvents <- event:
				client.logger.Debug("[desktop-ipc] mobile state projected", "thread_id", event.TaskID, "event_kind", event.Kind, "task_state", event.State)
			}
		}
	}
}

func (client *Client) popMobileEvent() (taskstate.MobileEvent, bool) {
	client.mobileEventMu.Lock()
	defer client.mobileEventMu.Unlock()
	if len(client.mobileOrder) == 0 {
		return taskstate.MobileEvent{}, false
	}
	threadID := client.mobileOrder[0]
	client.mobileOrder = client.mobileOrder[1:]
	event := client.mobilePending[threadID]
	delete(client.mobilePending, threadID)
	return event, true
}
