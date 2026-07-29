package taskstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidLiveNotification     = errors.New("Codex live notification is invalid")
	ErrUnsupportedLiveNotification = errors.New("Codex live notification is not used on mobile")
)

// AgentLabel is the agent name that appears inside phone-visible live
// summaries. Summaries reach the launcher verbatim, so each backend supplies
// its own label instead of every task reading as Codex.
type AgentLabel string

const (
	LabelCodex      AgentLabel = "Codex"
	LabelClaudeCode AgentLabel = "Claude"
)

type MobileEvent struct {
	TaskID        string
	Kind          string
	State         State
	Summary       string
	StartsTurn    bool
	Authorization *EventAuthorization
}

type EventAuthorization struct {
	mu      sync.RWMutex
	valid   bool
	revoked chan struct{}
}

func NewEventAuthorization() *EventAuthorization {
	return &EventAuthorization{valid: true, revoked: make(chan struct{})}
}

func (authorization *EventAuthorization) Revoke() {
	if authorization == nil {
		return
	}
	authorization.mu.Lock()
	if authorization.valid {
		authorization.valid = false
		close(authorization.revoked)
	}
	authorization.mu.Unlock()
}

func (authorization *EventAuthorization) Done() <-chan struct{} {
	if authorization == nil {
		return nil
	}
	return authorization.revoked
}

func (authorization *EventAuthorization) Valid() bool {
	if authorization == nil {
		return true
	}
	authorization.mu.RLock()
	defer authorization.mu.RUnlock()
	return authorization.valid
}

func (authorization *EventAuthorization) RunIfValid(run func() error) (bool, error) {
	if authorization == nil {
		return true, run()
	}
	authorization.mu.RLock()
	defer authorization.mu.RUnlock()
	if !authorization.valid {
		return false, nil
	}
	return true, run()
}

// ProjectNotification decodes a Codex app-server notification, so its
// summaries are always labelled Codex. A backend with a different wire format
// supplies its own projector and passes its label to ProjectTaskState.
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
		event := mobileEvent(envelope.ThreadID, "activity", Map(updated), workingSummary(LabelCodex))
		event.StartsTurn = true
		return event, nil
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
			return mobileEvent(envelope.ThreadID, "reply", state, repliedSummary(LabelCodex)), nil
		case Failed:
			return mobileEvent(envelope.ThreadID, "failure", state, failedSummary(LabelCodex)), nil
		case Interrupted:
			return mobileEvent(envelope.ThreadID, "interrupted", state, interruptedSummary(LabelCodex)), nil
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
			return mobileEvent(envelope.ThreadID, "activity", state, workingSummary(LabelCodex)), nil
		case WaitingForApproval:
			return mobileEvent(envelope.ThreadID, "approval", state, "Needs your approval"), nil
		case WaitingForAnswer:
			return mobileEvent(envelope.ThreadID, "answer", state, "Needs your answer"), nil
		case Failed:
			return mobileEvent(envelope.ThreadID, "failure", state, failedSummary(LabelCodex)), nil
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
		return mobileEvent(envelope.ThreadID, "activity", Working, mobileActivitySummary(LabelCodex, activity.Kind)), nil
	}
}

func mobileEvent(taskID, kind string, state State, summary string) MobileEvent {
	return MobileEvent{TaskID: taskID, Kind: kind, State: state, Summary: summary}
}

func ProjectTaskState(label AgentLabel, taskID string, state State) (MobileEvent, error) {
	if !validTextField(taskID, 256) {
		return MobileEvent{}, ErrInvalidLiveNotification
	}
	switch state {
	case Working:
		return mobileEvent(taskID, "activity", state, workingSummary(label)), nil
	case WaitingForApproval:
		return mobileEvent(taskID, "approval", state, "Needs your approval"), nil
	case WaitingForAnswer:
		return mobileEvent(taskID, "answer", state, "Needs your answer"), nil
	case Failed:
		return mobileEvent(taskID, "failure", state, failedSummary(label)), nil
	case Interrupted:
		return mobileEvent(taskID, "interrupted", state, interruptedSummary(label)), nil
	case IdleAfterReply:
		return mobileEvent(taskID, "reply", state, repliedSummary(label)), nil
	default:
		return MobileEvent{}, ErrInvalidLiveNotification
	}
}

func mobileActivitySummary(label AgentLabel, kind string) string {
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
		return workingSummary(label)
	}
}

func workingSummary(label AgentLabel) string     { return string(label) + " is working" }
func repliedSummary(label AgentLabel) string     { return string(label) + " replied" }
func failedSummary(label AgentLabel) string      { return string(label) + " hit an error" }
func interruptedSummary(label AgentLabel) string { return string(label) + " was interrupted" }

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
