// Package turnproxy turns OpenClaw Gateway chat-run event payloads into the
// launcher's taskstate.MobileEvents, so the phone sees the same activity /
// reply / failure / interrupted vocabulary it already gets from Codex.
package turnproxy

import (
	"encoding/json"

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

// AgentEventPayload is one "agent" event from the gateway. Thinking tokens
// arrive as stream:"thinking" with data.text (accumulated) and/or data.delta
// (incremental). OpenClaw 2026.7.1-2 may omit sessionKey, nest the body under
// payload, or send type:"thinking" with top-level text. Other streams are
// ignored here.
type AgentEventPayload struct {
	RunID      string         `json:"runId"`
	SessionKey string         `json:"sessionKey"`
	Stream     string         `json:"stream"`
	Data       AgentEventData `json:"data"`
}

// AgentEventData is the stream-specific body of an agent event.
type AgentEventData struct {
	Text  string `json:"text"`
	Delta string `json:"delta"`
}

// summaryLimit matches the mobile contract's cap on display strings
// (internal/mobileapi/contract/validation.go safeDisplayString): a longer
// or control-character summary would fail validation and the whole event
// would be dropped, so it must arrive already trimmed and scrubbed.
const summaryLimit = 512

const genericWorkingSummary = "Codex is working"

// runState is the reply and reasoning text accumulated so far for one
// in-flight run.
type runState struct {
	text      string
	reasoning string
	started   bool
}

// finishedRunsLimit bounds how many terminated run ids the mapper
// remembers, FIFO, so a long-lived session's memory does not grow without
// bound.
const finishedRunsLimit = 128

// TurnMapper converts one gateway session's chat and thinking events into
// MobileEvents for one task. It accumulates reply and reasoning text per run
// and remembers which runs have already opened with a Working event. It is
// meant to be driven by a single goroutine per session, so it takes no lock.
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
// Chat payloads for any other session are ignored by Apply. Thinking with a
// missing sessionKey, or a different key on an already in-flight run, still
// attaches via ApplyAgent.
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

func (m *TurnMapper) runFor(runID string) *runState {
	run, exists := m.runs[runID]
	if exists {
		return run
	}
	run = &runState{}
	m.runs[runID] = run
	return run
}

// KnowsRun reports whether this mapper already has in-flight state for runID.
func (m *TurnMapper) KnowsRun(runID string) bool {
	if runID == "" {
		return false
	}
	_, exists := m.runs[runID]
	return exists
}

func (m *TurnMapper) acceptsThinking(payload AgentEventPayload) bool {
	if payload.Stream != "thinking" || payload.RunID == "" {
		return false
	}
	if payload.SessionKey == "" || payload.SessionKey == m.sessionKey {
		return true
	}
	return m.KnowsRun(payload.RunID)
}

func (m *TurnMapper) workingEvent(run *runState, summary string) taskstate.MobileEvent {
	startsTurn := !run.started
	run.started = true
	if summary == "" {
		summary = genericWorkingSummary
	}
	return taskstate.MobileEvent{
		TaskID:     m.taskID,
		Kind:       "activity",
		State:      taskstate.Working,
		Summary:    summary,
		StartsTurn: startsTurn,
	}
}

func (m *TurnMapper) applyReasoning(run *runState, text, delta string) (taskstate.MobileEvent, bool) {
	previous := taskstate.SafeDisplay(run.reasoning, "", summaryLimit)
	if text != "" {
		run.reasoning = text
	} else if delta != "" {
		run.reasoning += delta
	} else {
		return taskstate.MobileEvent{}, false
	}
	summary := taskstate.SafeDisplay(run.reasoning, "", summaryLimit)
	if summary == "" || summary == previous {
		return taskstate.MobileEvent{}, false
	}
	return m.workingEvent(run, summary), true
}

// ReasoningDisplay returns the safe display text of the in-flight reasoning
// buffer for runID, or "" if this run has none. It never returns raw control
// characters.
func (m *TurnMapper) ReasoningDisplay(runID string) string {
	run, exists := m.runs[runID]
	if !exists {
		return ""
	}
	return taskstate.SafeDisplay(run.reasoning, "", summaryLimit)
}

// ApplyAgent folds one gateway agent event into the mapper's run state.
// stream:"thinking" for this session produces MobileEvents. A missing
// sessionKey, or a different key on an already in-flight run, still attaches.
func (m *TurnMapper) ApplyAgent(payload AgentEventPayload) []taskstate.MobileEvent {
	if !m.acceptsThinking(payload) {
		return nil
	}
	if _, finished := m.finishedRuns[payload.RunID]; finished {
		return nil
	}
	run := m.runFor(payload.RunID)
	event, ok := m.applyReasoning(run, payload.Data.Text, payload.Data.Delta)
	if !ok {
		return nil
	}
	return []taskstate.MobileEvent{event}
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
	}

	thinkingText, thinkingDelta := thinkingFromMessage(payload.Message)
	if payload.State == "delta" {
		if event, ok := m.applyReasoning(run, thinkingText, thinkingDelta); ok {
			events = append(events, event)
		}
	} else if thinkingText != "" && run.reasoning == "" {
		run.reasoning = thinkingText
	}

	switch payload.State {
	case "delta":
		if payload.Replace {
			run.text = payload.DeltaText
		} else {
			run.text += payload.DeltaText
		}
		if !run.started {
			// StartsTurn matters beyond this one flag: the event pump drops a
			// Working event that follows a terminal state unless it starts a
			// new turn (internal/app/eventpump.go:39), so every run's first
			// activity event must carry it.
			events = append(events, m.workingEvent(run, genericWorkingSummary))
		}
	case "final":
		// Collapse newlines/control chars before publish: safeDisplayString
		// rejects them and a dropped reply leaves Operator stuck on Working.
		summary := taskstate.SafeDisplay(run.text, "", summaryLimit)
		if summary == "" {
			summary = taskstate.SafeDisplay(fallbackMessageText(payload.Message), "Codex replied", summaryLimit)
		}
		events = append(events, taskstate.MobileEvent{
			TaskID:  m.taskID,
			Kind:    "reply",
			State:   taskstate.IdleAfterReply,
			Summary: summary,
		})
		delete(m.runs, payload.RunID)
		m.rememberFinished(payload.RunID)
	case "error":
		events = append(events, taskstate.MobileEvent{
			TaskID:  m.taskID,
			Kind:    "failure",
			State:   taskstate.Failed,
			Summary: taskstate.SafeDisplay(payload.ErrorMessage, "Codex hit an error", summaryLimit),
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

// fallbackMessageText decodes message as a plain JSON string, or as a
// chat-message object whose content array has type:"text" parts. Anything
// else (missing, unknown shape) returns "".
func fallbackMessageText(message json.RawMessage) string {
	if len(message) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(message, &text); err == nil {
		return text
	}
	return textFromMessageContent(message, "text")
}

func thinkingFromMessage(message json.RawMessage) (text, delta string) {
	text = textFromMessageContent(message, "thinking")
	if text == "" {
		text = textFromMessageContent(message, "reasoning")
	}
	if text != "" {
		return text, ""
	}
	if len(message) == 0 {
		return "", ""
	}
	var body struct {
		Thinking  string `json:"thinking"`
		Reasoning string `json:"reasoning"`
	}
	if json.Unmarshal(message, &body) != nil {
		return "", ""
	}
	if body.Thinking != "" {
		return body.Thinking, ""
	}
	return body.Reasoning, ""
}

func textFromMessageContent(message json.RawMessage, wantType string) string {
	if len(message) == 0 {
		return ""
	}
	var body struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
	}
	if json.Unmarshal(message, &body) != nil {
		return ""
	}
	var parts []string
	for _, part := range body.Content {
		if part.Type != wantType {
			continue
		}
		chunk := part.Text
		if chunk == "" {
			chunk = part.Thinking
		}
		if chunk != "" {
			parts = append(parts, chunk)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += "\n" + part
	}
	return out
}

// NormalizeAgentEvent unwraps the OpenClaw 2026.7.1-2 agent envelopes into
// AgentEventPayload. It never returns thinking text to callers beyond the
// struct fields; callers must not log those fields.
func NormalizeAgentEvent(raw json.RawMessage) (AgentEventPayload, bool) {
	return decodeAgentEvent(raw, 0)
}

func decodeAgentEvent(raw json.RawMessage, depth int) (AgentEventPayload, bool) {
	if len(raw) == 0 || depth > 3 {
		return AgentEventPayload{}, false
	}
	var envelope struct {
		RunID      string          `json:"runId"`
		SessionKey string          `json:"sessionKey"`
		Stream     string          `json:"stream"`
		Type       string          `json:"type"`
		Text       string          `json:"text"`
		Delta      string          `json:"delta"`
		Thinking   string          `json:"thinking"`
		Data       json.RawMessage `json:"data"`
		Payload    json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return AgentEventPayload{}, false
	}
	if len(envelope.Payload) > 0 && envelope.Payload[0] == '{' {
		nested, ok := decodeAgentEvent(envelope.Payload, depth+1)
		if ok {
			if nested.RunID == "" {
				nested.RunID = envelope.RunID
			}
			if nested.SessionKey == "" {
				nested.SessionKey = envelope.SessionKey
			}
			return nested, true
		}
	}

	stream := envelope.Stream
	if stream == "" && (envelope.Type == "thinking" || envelope.Type == "reasoning") {
		stream = "thinking"
	}
	data := AgentEventData{Text: envelope.Text, Delta: envelope.Delta}
	if data.Text == "" {
		data.Text = envelope.Thinking
	}
	if len(envelope.Data) > 0 {
		mergeAgentData(&stream, &data, envelope.Data, depth)
	}
	if stream == "" && data.Text == "" && data.Delta == "" && envelope.RunID == "" && envelope.SessionKey == "" {
		return AgentEventPayload{}, false
	}
	return AgentEventPayload{
		RunID:      envelope.RunID,
		SessionKey: envelope.SessionKey,
		Stream:     stream,
		Data:       data,
	}, true
}

func mergeAgentData(stream *string, data *AgentEventData, raw json.RawMessage, depth int) {
	var inner struct {
		Stream   string `json:"stream"`
		Type     string `json:"type"`
		Text     string `json:"text"`
		Delta    string `json:"delta"`
		Thinking string `json:"thinking"`
	}
	if json.Unmarshal(raw, &inner) == nil && (inner.Stream != "" || inner.Type != "" || inner.Text != "" || inner.Delta != "" || inner.Thinking != "") {
		if data.Text == "" {
			data.Text = inner.Text
		}
		if data.Delta == "" {
			data.Delta = inner.Delta
		}
		if data.Text == "" {
			data.Text = inner.Thinking
		}
		if *stream == "" {
			if inner.Stream != "" {
				*stream = inner.Stream
			} else if inner.Type == "thinking" || inner.Type == "reasoning" {
				*stream = "thinking"
			}
		}
		return
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil && data.Text == "" {
		data.Text = asString
		return
	}
	if raw[0] == '{' {
		if nested, ok := decodeAgentEvent(raw, depth+1); ok {
			if data.Text == "" {
				data.Text = nested.Data.Text
			}
			if data.Delta == "" {
				data.Delta = nested.Data.Delta
			}
			if *stream == "" {
				*stream = nested.Stream
			}
		}
	}
}
