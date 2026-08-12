// Package turnproxy turns OpenClaw Gateway chat-run event payloads into the
// launcher's taskstate.MobileEvents, so the phone sees the same activity /
// reply / failure / interrupted vocabulary it already gets from Codex.
package turnproxy

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

// ChatEventPayload is one "chat" event emitted by the OpenClaw Gateway for a
// run. Runs stream as a sequence of "delta" payloads that accumulate reply
// text, followed by exactly one terminal payload ("final", "error", or
// "aborted").
type ChatEventPayload struct {
	State        string          `json:"state"`
	DeltaText    string          `json:"deltaText"`
	Replace      bool            `json:"replace"`
	Message      json.RawMessage `json:"message"`
	ErrorMessage string          `json:"errorMessage"`
	ErrorKind    string          `json:"errorKind"`
	StopReason   string          `json:"stopReason"`
	RunID        string          `json:"runId"`
	SessionKey   string          `json:"sessionKey"`
	Seq          int             `json:"seq"`
}

// summaryLimit matches the mobile contract's cap on display strings
// (internal/mobileapi/contract/validation.go safeDisplayString): a longer
// summary would fail validation and the whole event would be dropped, so it
// must arrive already trimmed.
const summaryLimit = 512

// runState is the reply text accumulated so far for one in-flight run.
type runState struct {
	text string
}

// finishedRunsLimit bounds how many terminated run ids the mapper
// remembers, FIFO, so a long-lived session's memory does not grow without
// bound.
const finishedRunsLimit = 128

// TurnMapper converts one gateway session's chat events into MobileEvents
// for one task. It accumulates reply text per run and remembers which runs
// have already opened with a Working event. It is meant to be driven by a
// single goroutine per session, so it takes no lock.
type TurnMapper struct {
	taskID     string
	sessionKey string
	runs       map[string]*runState
	// finishedRuns and finishedOrder remember run ids that already reached
	// a terminal event, so a redelivered duplicate final or a late/reordered
	// delta for that run does not fabricate a fresh event. Bounded FIFO at
	// finishedRunsLimit entries.
	finishedRuns  map[string]struct{}
	finishedOrder []string
}

// NewTurnMapper builds a mapper for one task pinned to one gateway session.
// Payloads for any other session are ignored by Apply.
func NewTurnMapper(taskID, sessionKey string) *TurnMapper {
	return &TurnMapper{
		taskID:       taskID,
		sessionKey:   sessionKey,
		runs:         make(map[string]*runState),
		finishedRuns: make(map[string]struct{}),
	}
}

// rememberFinished records runID as terminated, evicting the oldest entry
// once the bound is exceeded.
func (m *TurnMapper) rememberFinished(runID string) {
	if _, exists := m.finishedRuns[runID]; exists {
		return
	}
	if len(m.finishedOrder) >= finishedRunsLimit {
		oldest := m.finishedOrder[0]
		m.finishedOrder = m.finishedOrder[1:]
		delete(m.finishedRuns, oldest)
	}
	m.finishedRuns[runID] = struct{}{}
	m.finishedOrder = append(m.finishedOrder, runID)
}

// Apply folds one gateway event into the mapper's run state and returns the
// MobileEvents the phone should see as a result, in order. It never mutates
// payload and never blocks.
func (m *TurnMapper) Apply(payload ChatEventPayload) []taskstate.MobileEvent {
	if payload.SessionKey != m.sessionKey {
		return nil
	}
	switch payload.State {
	case "delta", "final", "error", "aborted":
	default:
		return nil
	}
	if _, finished := m.finishedRuns[payload.RunID]; finished {
		return nil
	}

	var events []taskstate.MobileEvent
	run, exists := m.runs[payload.RunID]
	if !exists {
		run = &runState{}
		m.runs[payload.RunID] = run
		// StartsTurn matters beyond this one flag: the event pump drops a
		// Working event that follows a terminal state unless it starts a
		// new turn (internal/app/eventpump.go:39), so every run's first
		// activity event must carry it.
		if payload.State == "delta" {
			events = append(events, taskstate.MobileEvent{
				TaskID:     m.taskID,
				Kind:       "activity",
				State:      taskstate.Working,
				Summary:    "Codex is working",
				StartsTurn: true,
			})
		}
	}

	switch payload.State {
	case "delta":
		if payload.Replace {
			run.text = payload.DeltaText
		} else {
			run.text += payload.DeltaText
		}
	case "final":
		summary := run.text
		if summary == "" {
			summary = fallbackMessageText(payload.Message)
		}
		if summary == "" {
			summary = "Codex replied"
		}
		events = append(events, taskstate.MobileEvent{
			TaskID:  m.taskID,
			Kind:    "reply",
			State:   taskstate.IdleAfterReply,
			Summary: truncate(summary),
		})
		delete(m.runs, payload.RunID)
		m.rememberFinished(payload.RunID)
	case "error":
		summary := payload.ErrorMessage
		if summary == "" {
			summary = "Codex hit an error"
		}
		events = append(events, taskstate.MobileEvent{
			TaskID:  m.taskID,
			Kind:    "failure",
			State:   taskstate.Failed,
			Summary: truncate(summary),
		})
		delete(m.runs, payload.RunID)
		m.rememberFinished(payload.RunID)
	case "aborted":
		events = append(events, taskstate.MobileEvent{
			TaskID:  m.taskID,
			Kind:    "interrupted",
			State:   taskstate.Interrupted,
			Summary: "Codex was interrupted",
		})
		delete(m.runs, payload.RunID)
		m.rememberFinished(payload.RunID)
	}

	return events
}

// fallbackMessageText decodes message as a plain JSON string, returning ""
// for anything else (missing, object, array, non-string scalar).
func fallbackMessageText(message json.RawMessage) string {
	if len(message) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(message, &text); err != nil {
		return ""
	}
	return text
}

// truncate keeps the front of value up to summaryLimit runes.
func truncate(value string) string {
	if utf8.RuneCountInString(value) <= summaryLimit {
		return value
	}
	runes := []rune(value)
	return string(runes[:summaryLimit])
}
