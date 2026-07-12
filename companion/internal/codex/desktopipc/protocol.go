package desktopipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

const MaxFrameBytes = 256 * 1024 * 1024
const MaxRetainedStateBytes = 64 * 1024 * 1024
const MaxActionTextBytes = 128 * 1024

var (
	ErrActionNotAllowed    = errors.New("desktop action is not allowed")
	ErrFrameTooLarge       = errors.New("desktop IPC frame is too large")
	ErrIncompatibleVersion = errors.New("desktop IPC version is incompatible")
	ErrInvalidAction       = errors.New("desktop action is invalid")
	ErrInvalidFrame        = errors.New("desktop IPC frame is invalid")
	ErrRevisionOrder       = errors.New("desktop stream revision is out of order")
	ErrSnapshotRequired    = errors.New("desktop stream snapshot is required")
	ErrStateTooLarge       = errors.New("desktop retained task state is too large")
	ErrWrongConversation   = errors.New("desktop event belongs to another task")
)

type wireMessage struct {
	Type           string          `json:"type,omitempty"`
	RequestID      string          `json:"requestId,omitempty"`
	SourceClientID string          `json:"sourceClientId,omitempty"`
	ResultType     string          `json:"resultType,omitempty"`
	Method         string          `json:"method,omitempty"`
	Version        int             `json:"version,omitempty"`
	TimeoutMS      int             `json:"timeoutMs,omitempty"`
	Params         json.RawMessage `json:"params,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	Response       json.RawMessage `json:"response,omitempty"`
}

func readFrame(reader io.Reader) (wireMessage, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return wireMessage{}, fmt.Errorf("read desktop IPC frame length: %w", err)
	}
	length := binary.LittleEndian.Uint32(header)
	if length == 0 {
		return wireMessage{}, ErrInvalidFrame
	}
	if length > MaxFrameBytes {
		return wireMessage{}, ErrFrameTooLarge
	}
	body := make([]byte, int(length))
	if _, err := io.ReadFull(reader, body); err != nil {
		return wireMessage{}, fmt.Errorf("read desktop IPC frame body: %w", err)
	}
	if len(body) < 2 || body[0] != '{' {
		return wireMessage{}, ErrInvalidFrame
	}
	var message wireMessage
	if err := json.Unmarshal(body, &message); err != nil {
		return wireMessage{}, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	return message, nil
}

func writeFrame(writer io.Writer, message wireMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode desktop IPC frame: %w", err)
	}
	if len(body) == 0 {
		return ErrInvalidFrame
	}
	if len(body) > MaxFrameBytes {
		return ErrFrameTooLarge
	}
	frame := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(frame, uint32(len(body)))
	copy(frame[4:], body)
	for len(frame) > 0 {
		written, writeErr := writer.Write(frame)
		if writeErr != nil {
			return fmt.Errorf("write desktop IPC frame: %w", writeErr)
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

var supportedVersions = map[string]int{
	"thread-stream-state-changed":                            11,
	"thread-follower-start-turn":                             1,
	"thread-follower-load-complete-history":                  1,
	"thread-follower-compact-thread":                         1,
	"thread-follower-steer-turn":                             1,
	"thread-follower-interrupt-turn":                         2,
	"thread-follower-update-thread-settings":                 1,
	"thread-follower-edit-last-user-turn":                    2,
	"thread-follower-command-approval-decision":              1,
	"thread-follower-file-approval-decision":                 1,
	"thread-follower-permissions-request-approval-response":  1,
	"thread-follower-submit-user-input":                      1,
	"thread-follower-submit-mcp-server-elicitation-response": 1,
}

var ignoredBroadcastVersions = map[string]int{
	"client-status-changed":                    0,
	"ipc-connection-reset":                     1,
	"query-cache-invalidate":                   0,
	"thread-archived":                          2,
	"thread-queued-followups-changed":          1,
	"thread-read-state-changed":                1,
	"thread-stream-following-changed":          1,
	"thread-stream-following-status-requested": 1,
	"thread-unarchived":                        1,
}

func validateVersion(method string, version int) error {
	want, ok := supportedVersions[method]
	if !ok || version != want {
		return fmt.Errorf("%w: method=%s version=%d", ErrIncompatibleVersion, method, version)
	}
	return nil
}

func validateIgnoredBroadcast(method string, version int) error {
	want, ok := ignoredBroadcastVersions[method]
	if !ok || version != want {
		return fmt.Errorf("%w: method=%s version=%d", ErrIncompatibleVersion, method, version)
	}
	return nil
}

type streamEvent struct {
	ConversationID string
	ChangeType     string
	Revision       uint64
	RawChange      json.RawMessage
}

func parseStreamEvent(message wireMessage) (streamEvent, error) {
	if message.Type != "broadcast" || message.Method != "thread-stream-state-changed" {
		return streamEvent{}, ErrInvalidFrame
	}
	if err := validateVersion(message.Method, message.Version); err != nil {
		return streamEvent{}, err
	}
	var params struct {
		ConversationID string          `json:"conversationId"`
		Change         json.RawMessage `json:"change"`
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return streamEvent{}, fmt.Errorf("%w: stream params: %v", ErrInvalidFrame, err)
	}
	var change struct {
		Type     string `json:"type"`
		Revision uint64 `json:"revision"`
	}
	if err := json.Unmarshal(params.Change, &change); err != nil {
		return streamEvent{}, fmt.Errorf("%w: stream change: %v", ErrInvalidFrame, err)
	}
	if params.ConversationID == "" || change.Type == "" || change.Revision == 0 {
		return streamEvent{}, ErrInvalidFrame
	}
	return streamEvent{
		ConversationID: params.ConversationID,
		ChangeType:     change.Type,
		Revision:       change.Revision,
		RawChange:      append(json.RawMessage(nil), params.Change...),
	}, nil
}

type RetainedState struct {
	Revision           uint64
	SnapshotGeneration uint64
	Snapshot           json.RawMessage
	Deltas             []json.RawMessage
}

type StreamState struct {
	conversationID     string
	mu                 sync.RWMutex
	hasSnapshot        bool
	revision           uint64
	snapshotGeneration uint64
	snapshot           json.RawMessage
	deltas             []json.RawMessage
	retainedBytes      int
	updated            chan struct{}
}

func newStreamState(conversationID string) *StreamState {
	return &StreamState{conversationID: conversationID, updated: make(chan struct{}, 1)}
}

func (state *StreamState) apply(event streamEvent) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if event.ConversationID != state.conversationID {
		return ErrWrongConversation
	}
	if event.ChangeType == "snapshot" {
		if event.Revision == 0 || state.hasSnapshot && event.Revision <= state.revision {
			return ErrRevisionOrder
		}
		if len(event.RawChange) > MaxRetainedStateBytes {
			return ErrStateTooLarge
		}
		state.hasSnapshot = true
		state.revision = event.Revision
		state.snapshotGeneration++
		state.snapshot = append(state.snapshot[:0], event.RawChange...)
		state.deltas = nil
		state.retainedBytes = len(state.snapshot)
		state.signalUpdate()
		return nil
	}
	if !state.hasSnapshot {
		return ErrSnapshotRequired
	}
	if event.Revision != state.revision+1 {
		return ErrRevisionOrder
	}
	if state.retainedBytes+len(event.RawChange) > MaxRetainedStateBytes {
		return ErrStateTooLarge
	}
	state.revision = event.Revision
	state.deltas = append(state.deltas, append(json.RawMessage(nil), event.RawChange...))
	state.retainedBytes += len(event.RawChange)
	state.signalUpdate()
	return nil
}

func (state *StreamState) signalUpdate() {
	select {
	case state.updated <- struct{}{}:
	default:
	}
}

func (state *StreamState) Updates() <-chan struct{} {
	return state.updated
}

func (state *StreamState) HasSnapshot() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.hasSnapshot
}

func (state *StreamState) Revision() uint64 {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.revision
}

func (state *StreamState) State() RetainedState {
	state.mu.RLock()
	defer state.mu.RUnlock()
	deltas := make([]json.RawMessage, len(state.deltas))
	for index, delta := range state.deltas {
		deltas[index] = append(json.RawMessage(nil), delta...)
	}
	return RetainedState{
		Revision:           state.revision,
		SnapshotGeneration: state.snapshotGeneration,
		Snapshot:           append(json.RawMessage(nil), state.snapshot...),
		Deltas:             deltas,
	}
}

func logStreamEvent(logger *slog.Logger, event streamEvent) {
	logger.Info("[desktop-ipc] stream event",
		"thread_id", event.ConversationID,
		"change_type", event.ChangeType,
		"revision", event.Revision,
	)
}

type ActionKind string

const (
	ActionStartTurn       ActionKind = "start_turn"
	ActionSteerTurn       ActionKind = "steer_turn"
	ActionInterruptTurn   ActionKind = "interrupt_turn"
	ActionCommandApproval ActionKind = "command_approval"
)

type FollowerAction struct {
	Kind           ActionKind
	ConversationID string
	RequestID      string
	Decision       string
	Text           string
}

type textInput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func buildFollowerAction(action FollowerAction) (wireMessage, error) {
	if action.ConversationID == "" {
		return wireMessage{}, fmt.Errorf("%w: task ID is required", ErrInvalidAction)
	}
	params := map[string]any{"conversationId": action.ConversationID}
	method := ""
	switch action.Kind {
	case ActionStartTurn:
		if !validActionText(action.Text) || action.RequestID != "" || action.Decision != "" {
			return wireMessage{}, fmt.Errorf("%w: start text is required", ErrInvalidAction)
		}
		method = "thread-follower-start-turn"
		params["turnStartParams"] = map[string]any{"input": []textInput{{Type: "text", Text: action.Text}}}
	case ActionSteerTurn:
		if !validActionText(action.Text) || action.RequestID != "" || action.Decision != "" {
			return wireMessage{}, fmt.Errorf("%w: steer text is required", ErrInvalidAction)
		}
		method = "thread-follower-steer-turn"
		params["input"] = []textInput{{Type: "text", Text: action.Text}}
	case ActionInterruptTurn:
		if action.Text != "" || action.RequestID != "" || action.Decision != "" {
			return wireMessage{}, fmt.Errorf("%w: interrupt has unexpected fields", ErrInvalidAction)
		}
		method = "thread-follower-interrupt-turn"
	case ActionCommandApproval:
		if action.RequestID == "" || !allowedApprovalDecision(action.Decision) || action.Text != "" {
			return wireMessage{}, fmt.Errorf("%w: approval request and decision are required", ErrInvalidAction)
		}
		method = "thread-follower-command-approval-decision"
		params["requestId"] = action.RequestID
		params["decision"] = action.Decision
	default:
		return wireMessage{}, fmt.Errorf("%w: %s", ErrActionNotAllowed, action.Kind)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return wireMessage{}, fmt.Errorf("encode follower action: %w", err)
	}
	return wireMessage{Type: "request", Method: method, Version: supportedVersions[method], Params: encoded}, nil
}

func validActionText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= MaxActionTextBytes
}

func allowedApprovalDecision(decision string) bool {
	switch decision {
	case "accept", "acceptForSession", "decline", "cancel":
		return true
	default:
		return false
	}
}
