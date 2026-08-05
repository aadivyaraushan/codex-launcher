package beeperreadback

import "errors"

type Status string

const (
	StatusPending Status = "pending"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
)

type Event struct {
	PendingMessageID string
	ChatID           string
	Text             string
	IsSender         bool
	SendStatus       string
}

type Decision struct {
	Status Status
	Reason string
}

var ErrEmptyPending = errors.New("beeper readback: empty pending id")

// Decide maps a Beeper final event onto the Wave 2 acceptance machine.
func Decide(pendingID string, event *Event) (Decision, error) {
	if pendingID == "" {
		return Decision{}, ErrEmptyPending
	}
	if event == nil {
		return Decision{Status: StatusPending, Reason: "awaiting_final_event"}, nil
	}
	if event.PendingMessageID != pendingID {
		return Decision{Status: StatusPending, Reason: "pending_id_mismatch"}, nil
	}
	if !event.IsSender {
		return Decision{Status: StatusFailed, Reason: "not_sender"}, nil
	}
	if event.SendStatus != "SUCCESS" {
		return Decision{Status: StatusPending, Reason: "send_status_" + event.SendStatus}, nil
	}
	if event.ChatID == "" || event.Text == "" {
		return Decision{Status: StatusFailed, Reason: "missing_chat_or_text"}, nil
	}
	return Decision{Status: StatusSuccess, Reason: "provider_event_matched"}, nil
}
