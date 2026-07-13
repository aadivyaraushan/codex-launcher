package taskstate

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrInvalidLiveNotification     = errors.New("Codex live notification is invalid")
	ErrUnsupportedLiveNotification = errors.New("Codex live notification is not used on mobile")
)

type MobileEvent struct {
	TaskID  string
	Kind    string
	State   State
	Summary string
}

func ProjectNotification(method string, params json.RawMessage) (MobileEvent, error) {
	var envelope struct {
		ThreadID string `json:"threadId"`
	}
	if json.Unmarshal(params, &envelope) != nil || !validTextField(envelope.ThreadID, 256) {
		if supportedLiveMethod(method) {
			return MobileEvent{}, fmt.Errorf("%w: %s", ErrInvalidLiveNotification, method)
		}
		return MobileEvent{}, ErrUnsupportedLiveNotification
	}
	signals := Signals{ThreadID: envelope.ThreadID}
	switch method {
	case "turn/started":
		updated, err := ApplyNotification(signals, method, params)
		if err != nil {
			return MobileEvent{}, fmt.Errorf("%w: %s", ErrInvalidLiveNotification, method)
		}
		return mobileEvent(envelope.ThreadID, "activity", Map(updated), "Codex is working"), nil
	case "turn/completed":
		var value struct {
			Turn struct {
				Status string `json:"status"`
			} `json:"turn"`
		}
		updated, err := ApplyNotification(signals, method, params)
		if err != nil || json.Unmarshal(params, &value) != nil || value.Turn.Status == "inProgress" {
			return MobileEvent{}, fmt.Errorf("%w: %s", ErrInvalidLiveNotification, method)
		}
		switch state := Map(updated); state {
		case IdleAfterReply:
			return mobileEvent(envelope.ThreadID, "reply", state, "Codex replied"), nil
		case Failed:
			return mobileEvent(envelope.ThreadID, "failure", state, "Codex hit an error"), nil
		case Interrupted:
			return mobileEvent(envelope.ThreadID, "interrupted", state, "Codex was interrupted"), nil
		default:
			return MobileEvent{}, fmt.Errorf("%w: %s", ErrInvalidLiveNotification, method)
		}
	case "thread/status/changed":
		updated, err := ApplyNotification(signals, method, params)
		if err != nil {
			return MobileEvent{}, fmt.Errorf("%w: %s", ErrInvalidLiveNotification, method)
		}
		switch state := Map(updated); state {
		case Working:
			return mobileEvent(envelope.ThreadID, "activity", state, "Codex is working"), nil
		case WaitingForApproval:
			return mobileEvent(envelope.ThreadID, "approval", state, "Needs your approval"), nil
		case WaitingForAnswer:
			return mobileEvent(envelope.ThreadID, "answer", state, "Needs your answer"), nil
		case Failed:
			return mobileEvent(envelope.ThreadID, "failure", state, "Codex hit an error"), nil
		default:
			return MobileEvent{}, ErrUnsupportedLiveNotification
		}
	default:
		activity, err := DecodeNotification(envelope.ThreadID, method, params)
		if err != nil {
			if supportedLiveMethod(method) {
				return MobileEvent{}, fmt.Errorf("%w: %s", ErrInvalidLiveNotification, method)
			}
			return MobileEvent{}, ErrUnsupportedLiveNotification
		}
		return mobileEvent(envelope.ThreadID, "activity", Working, mobileActivitySummary(activity.Kind)), nil
	}
}

func mobileEvent(taskID, kind string, state State, summary string) MobileEvent {
	return MobileEvent{TaskID: taskID, Kind: kind, State: state, Summary: summary}
}

func mobileActivitySummary(kind string) string {
	switch kind {
	case "reply":
		return "Writing a reply"
	case "command":
		return "Running a command"
	case "file":
		return "Editing files"
	case "plan":
		return "Updating the plan"
	case "diff":
		return "Reviewing changes"
	default:
		return "Codex is working"
	}
}

func supportedLiveMethod(method string) bool {
	switch method {
	case "turn/started", "turn/completed", "thread/status/changed",
		"item/started", "item/completed", "item/agentMessage/delta",
		"item/commandExecution/outputDelta", "item/commandExecution/terminalInteraction",
		"item/fileChange/outputDelta", "item/plan/delta", "item/mcpToolCall/progress",
		"item/fileChange/patchUpdated", "turn/diff/updated":
		return true
	default:
		return false
	}
}
