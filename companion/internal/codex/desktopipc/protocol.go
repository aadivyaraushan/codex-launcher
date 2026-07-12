package desktopipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

const MaxFrameBytes = 256 * 1024 * 1024

var (
	ErrActionNotAllowed    = errors.New("desktop action is not allowed")
	ErrFrameTooLarge       = errors.New("desktop IPC frame is too large")
	ErrIncompatibleVersion = errors.New("desktop IPC version is incompatible")
	ErrInvalidAction       = errors.New("desktop action is invalid")
	ErrInvalidFrame        = errors.New("desktop IPC frame is invalid")
	ErrRevisionOrder       = errors.New("desktop stream revision is out of order")
	ErrSnapshotRequired    = errors.New("desktop stream snapshot is required")
	ErrWrongConversation   = errors.New("desktop event belongs to another task")
)

type WireMessage struct {
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

func ReadFrame(reader io.Reader) (WireMessage, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return WireMessage{}, fmt.Errorf("read desktop IPC frame length: %w", err)
	}
	length := binary.LittleEndian.Uint32(header)
	if length == 0 {
		return WireMessage{}, ErrInvalidFrame
	}
	if length > MaxFrameBytes {
		return WireMessage{}, ErrFrameTooLarge
	}
	body := make([]byte, int(length))
	if _, err := io.ReadFull(reader, body); err != nil {
		return WireMessage{}, fmt.Errorf("read desktop IPC frame body: %w", err)
	}
	if len(body) < 2 || body[0] != '{' {
		return WireMessage{}, ErrInvalidFrame
	}
	var message WireMessage
	if err := json.Unmarshal(body, &message); err != nil {
		return WireMessage{}, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	return message, nil
}

func WriteFrame(writer io.Writer, message WireMessage) error {
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
	"thread-follower-steer-turn":                             1,
	"thread-follower-interrupt-turn":                         2,
	"thread-follower-command-approval-decision":              1,
	"thread-follower-file-approval-decision":                 1,
	"thread-follower-permissions-request-approval-response":  1,
	"thread-follower-submit-user-input":                      1,
	"thread-follower-submit-mcp-server-elicitation-response": 1,
}

func ValidateVersion(method string, version int) error {
	want, ok := supportedVersions[method]
	if !ok || version != want {
		return fmt.Errorf("%w: method=%s version=%d", ErrIncompatibleVersion, method, version)
	}
	return nil
}

type StreamEvent struct {
	ConversationID string
	ChangeType     string
	Revision       uint64
	RawChange      json.RawMessage
}

func ParseStreamEvent(message WireMessage) (StreamEvent, error) {
	if message.Type != "broadcast" || message.Method != "thread-stream-state-changed" {
		return StreamEvent{}, ErrInvalidFrame
	}
	if err := ValidateVersion(message.Method, message.Version); err != nil {
		return StreamEvent{}, err
	}
	var params struct {
		ConversationID string `json:"conversationId"`
		Change         struct {
			Type     string `json:"type"`
			Revision uint64 `json:"revision"`
		} `json:"change"`
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return StreamEvent{}, fmt.Errorf("%w: stream params: %v", ErrInvalidFrame, err)
	}
	if params.ConversationID == "" || params.Change.Type == "" || params.Change.Revision == 0 {
		return StreamEvent{}, ErrInvalidFrame
	}
	return StreamEvent{
		ConversationID: params.ConversationID,
		ChangeType:     params.Change.Type,
		Revision:       params.Change.Revision,
		RawChange:      append(json.RawMessage(nil), message.Params...),
	}, nil
}

type StreamState struct {
	conversationID string
	mu             sync.RWMutex
	hasSnapshot    bool
	revision       uint64
}

func NewStreamState(conversationID string) *StreamState {
	return &StreamState{conversationID: conversationID}
}

func (state *StreamState) Apply(event StreamEvent) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if event.ConversationID != state.conversationID {
		return ErrWrongConversation
	}
	if event.ChangeType == "snapshot" {
		if event.Revision == 0 {
			return ErrRevisionOrder
		}
		state.hasSnapshot = true
		state.revision = event.Revision
		return nil
	}
	if !state.hasSnapshot {
		return ErrSnapshotRequired
	}
	if event.Revision != state.revision+1 {
		return ErrRevisionOrder
	}
	state.revision = event.Revision
	return nil
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

func LogStreamEvent(logger *slog.Logger, event StreamEvent) {
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
	Kind            ActionKind
	ConversationID  string
	RequestID       string
	Decision        string
	Input           json.RawMessage
	TurnStartParams json.RawMessage
}

func BuildFollowerAction(action FollowerAction) (WireMessage, error) {
	if action.ConversationID == "" {
		return WireMessage{}, fmt.Errorf("%w: task ID is required", ErrInvalidAction)
	}
	params := map[string]any{"conversationId": action.ConversationID}
	method := ""
	switch action.Kind {
	case ActionStartTurn:
		if !validJSONObject(action.TurnStartParams) {
			return WireMessage{}, fmt.Errorf("%w: turnStartParams must be an object", ErrInvalidAction)
		}
		method = "thread-follower-start-turn"
		params["turnStartParams"] = action.TurnStartParams
	case ActionSteerTurn:
		if !json.Valid(action.Input) || len(action.Input) == 0 {
			return WireMessage{}, fmt.Errorf("%w: input is required", ErrInvalidAction)
		}
		method = "thread-follower-steer-turn"
		params["input"] = action.Input
	case ActionInterruptTurn:
		method = "thread-follower-interrupt-turn"
	case ActionCommandApproval:
		if action.RequestID == "" || !allowedApprovalDecision(action.Decision) {
			return WireMessage{}, fmt.Errorf("%w: approval request and decision are required", ErrInvalidAction)
		}
		method = "thread-follower-command-approval-decision"
		params["requestId"] = action.RequestID
		params["decision"] = action.Decision
	default:
		return WireMessage{}, fmt.Errorf("%w: %s", ErrActionNotAllowed, action.Kind)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return WireMessage{}, fmt.Errorf("encode follower action: %w", err)
	}
	return WireMessage{Type: "request", Method: method, Version: supportedVersions[method], Params: encoded}, nil
}

func validJSONObject(value json.RawMessage) bool {
	if len(value) < 2 || value[0] != '{' || !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil
}

func allowedApprovalDecision(decision string) bool {
	switch decision {
	case "accept", "acceptForSession", "decline", "cancel":
		return true
	default:
		return false
	}
}
