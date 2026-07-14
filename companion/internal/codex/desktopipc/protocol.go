package desktopipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path/filepath"
	"regexp"
	"strconv"
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
	BaseRevision   uint64
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
		Type         string `json:"type"`
		BaseRevision uint64 `json:"baseRevision"`
		Revision     uint64 `json:"revision"`
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
		BaseRevision:   change.BaseRevision,
		Revision:       change.Revision,
		RawChange:      append(json.RawMessage(nil), params.Change...),
	}, nil
}

type RetainedState struct {
	Revision           uint64
	SnapshotGeneration uint64
	Snapshot           json.RawMessage
	Deltas             []json.RawMessage
	Materialized       json.RawMessage
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
	materialized       json.RawMessage
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
		var change struct {
			ConversationState json.RawMessage `json:"conversationState"`
		}
		if json.Unmarshal(event.RawChange, &change) != nil || len(change.ConversationState) == 0 {
			return ErrInvalidFrame
		}
		if len(event.RawChange)+len(change.ConversationState) > MaxRetainedStateBytes {
			return ErrStateTooLarge
		}
		state.hasSnapshot = true
		state.revision = event.Revision
		state.snapshotGeneration++
		state.snapshot = append(state.snapshot[:0], event.RawChange...)
		state.materialized = append(state.materialized[:0], change.ConversationState...)
		state.deltas = nil
		state.retainedBytes = len(state.snapshot) + len(state.materialized)
		return nil
	}
	if !state.hasSnapshot {
		return ErrSnapshotRequired
	}
	if event.ChangeType != "patches" || event.BaseRevision != state.revision || event.Revision != state.revision+1 {
		return ErrRevisionOrder
	}
	materialized, err := applyPinnedPatches(state.materialized, event.RawChange)
	if err != nil {
		return err
	}
	newRetainedBytes := state.retainedBytes - len(state.materialized) + len(materialized) + len(event.RawChange)
	if newRetainedBytes > MaxRetainedStateBytes {
		return ErrStateTooLarge
	}
	state.revision = event.Revision
	state.materialized = materialized
	state.deltas = append(state.deltas, append(json.RawMessage(nil), event.RawChange...))
	state.retainedBytes = newRetainedBytes
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
		Materialized:       append(json.RawMessage(nil), state.materialized...),
	}
}

func applyPinnedPatches(materialized, rawChange json.RawMessage) (json.RawMessage, error) {
	var change struct {
		Patches []struct {
			Op    string          `json:"op"`
			Path  []any           `json:"path"`
			Value json.RawMessage `json:"value"`
		} `json:"patches"`
	}
	decoder := json.NewDecoder(bytes.NewReader(rawChange))
	decoder.UseNumber()
	if decoder.Decode(&change) != nil || len(change.Patches) == 0 || len(change.Patches) > 4096 {
		return nil, ErrInvalidFrame
	}
	var root any
	decoder = json.NewDecoder(bytes.NewReader(materialized))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil {
		return nil, ErrInvalidFrame
	}
	for _, patch := range change.Patches {
		if len(patch.Path) > 64 || (patch.Op != "add" && patch.Op != "replace" && patch.Op != "remove") {
			return nil, ErrInvalidFrame
		}
		var value any
		if patch.Op != "remove" {
			valueDecoder := json.NewDecoder(bytes.NewReader(patch.Value))
			valueDecoder.UseNumber()
			if valueDecoder.Decode(&value) != nil {
				return nil, ErrInvalidFrame
			}
		}
		var err error
		root, err = applyPatchAt(root, patch.Path, patch.Op, value)
		if err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(root)
	if err != nil || len(encoded) > MaxRetainedStateBytes {
		return nil, ErrStateTooLarge
	}
	return encoded, nil
}

func applyPatchAt(node any, path []any, operation string, value any) (any, error) {
	if len(path) == 0 {
		if operation == "remove" {
			return nil, ErrInvalidFrame
		}
		return value, nil
	}
	segment := path[0]
	if object, ok := node.(map[string]any); ok {
		key, ok := segment.(string)
		if !ok || key == "" || len(key) > 256 || unsafePatchKey(key) {
			return nil, ErrInvalidFrame
		}
		if len(path) == 1 {
			_, exists := object[key]
			if operation == "replace" && !exists || operation == "remove" && !exists {
				return nil, ErrInvalidFrame
			}
			if operation == "remove" {
				delete(object, key)
			} else {
				object[key] = value
			}
			return object, nil
		}
		child, exists := object[key]
		if !exists {
			return nil, ErrInvalidFrame
		}
		updated, err := applyPatchAt(child, path[1:], operation, value)
		if err != nil {
			return nil, err
		}
		object[key] = updated
		return object, nil
	}
	if list, ok := node.([]any); ok {
		index, ok := patchIndex(segment)
		if !ok {
			return nil, ErrInvalidFrame
		}
		if len(path) == 1 {
			switch operation {
			case "add":
				if index < 0 || index > len(list) {
					return nil, ErrInvalidFrame
				}
				list = append(list, nil)
				copy(list[index+1:], list[index:])
				list[index] = value
			case "replace":
				if index < 0 || index >= len(list) {
					return nil, ErrInvalidFrame
				}
				list[index] = value
			case "remove":
				if index < 0 || index >= len(list) {
					return nil, ErrInvalidFrame
				}
				list = append(list[:index], list[index+1:]...)
			}
			return list, nil
		}
		if index < 0 || index >= len(list) {
			return nil, ErrInvalidFrame
		}
		updated, err := applyPatchAt(list[index], path[1:], operation, value)
		if err != nil {
			return nil, err
		}
		list[index] = updated
		return list, nil
	}
	return nil, ErrInvalidFrame
}

func unsafePatchKey(key string) bool {
	return key == "__proto__" || key == "prototype" || key == "constructor"
}

func patchIndex(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	case string:
		parsed, err := strconv.Atoi(typed)
		return parsed, err == nil
	default:
		return 0, false
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
	ActionStartTurn          ActionKind = "start_turn"
	ActionSteerTurn          ActionKind = "steer_turn"
	ActionInterruptTurn      ActionKind = "interrupt_turn"
	ActionCommandApproval    ActionKind = "command_approval"
	ActionCompact            ActionKind = "compact"
	ActionUpdateSettings     ActionKind = "update_settings"
	ActionFileApproval       ActionKind = "file_approval"
	ActionPermissionApproval ActionKind = "permission_approval"
	ActionSubmitUserInput    ActionKind = "submit_user_input"
	ActionSubmitMCP          ActionKind = "submit_mcp"
	ActionEditLastTurn       ActionKind = "edit_last_turn"
)

type ThreadSettings struct {
	Model          string `json:"model,omitempty"`
	Effort         string `json:"effort,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Permissions    string `json:"permissions,omitempty"`
}

type PermissionResponse struct {
	Permissions json.RawMessage `json:"permissions"`
	Scope       string          `json:"scope"`
}

type UserInputAnswer struct {
	Answers []string `json:"answers"`
}

type UserInputResponse struct {
	Answers map[string]UserInputAnswer `json:"answers"`
}

type MCPResponse struct {
	Action  string          `json:"action"`
	Content json.RawMessage `json:"content,omitempty"`
}

type FollowerAction struct {
	Kind                          ActionKind
	ConversationID                string
	RequestID                     string
	Decision                      string
	Text                          string
	Attachments                   []AttachmentInput
	TurnID                        string
	AgentMode                     string
	ServiceTier                   string
	ShouldSendPermissionOverrides bool
	Settings                      *ThreadSettings
	PermissionResponse            *PermissionResponse
	UserInputResponse             *UserInputResponse
	MCPResponse                   *MCPResponse
}

type AttachmentInput struct {
	ID        string
	Path      string
	MediaType string
}

type followerInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

func buildFollowerAction(action FollowerAction) (wireMessage, error) {
	if !validDesktopID(action.ConversationID) {
		return wireMessage{}, fmt.Errorf("%w: task ID is required", ErrInvalidAction)
	}
	params := map[string]any{"conversationId": action.ConversationID}
	method := ""
	switch action.Kind {
	case ActionStartTurn:
		if !validActionText(action.Text) || action.hasFieldsExceptText() {
			return wireMessage{}, fmt.Errorf("%w: start text is required", ErrInvalidAction)
		}
		input, err := followerInputItems(action.Text, action.Attachments)
		if err != nil {
			return wireMessage{}, err
		}
		method = "thread-follower-start-turn"
		params["turnStartParams"] = map[string]any{"input": input}
	case ActionSteerTurn:
		if !validActionText(action.Text) || action.hasFieldsExceptText() {
			return wireMessage{}, fmt.Errorf("%w: steer text is required", ErrInvalidAction)
		}
		input, err := followerInputItems(action.Text, action.Attachments)
		if err != nil {
			return wireMessage{}, err
		}
		method = "thread-follower-steer-turn"
		params["input"] = input
	case ActionInterruptTurn:
		if action.hasPayloadFields() {
			return wireMessage{}, fmt.Errorf("%w: interrupt has unexpected fields", ErrInvalidAction)
		}
		method = "thread-follower-interrupt-turn"
	case ActionCommandApproval:
		if !validDesktopID(action.RequestID) || !allowedApprovalDecision(action.Decision) || action.hasFieldsExceptApproval() {
			return wireMessage{}, fmt.Errorf("%w: approval request and decision are required", ErrInvalidAction)
		}
		method = "thread-follower-command-approval-decision"
		params["requestId"] = action.RequestID
		params["decision"] = action.Decision
	case ActionCompact:
		if action.hasPayloadFields() {
			return wireMessage{}, fmt.Errorf("%w: compact has unexpected fields", ErrInvalidAction)
		}
		method = "thread-follower-compact-thread"
	case ActionUpdateSettings:
		if action.Settings == nil || !validThreadSettings(*action.Settings) || action.hasFieldsExceptSettings() {
			return wireMessage{}, fmt.Errorf("%w: valid thread settings are required", ErrInvalidAction)
		}
		method = "thread-follower-update-thread-settings"
		params["threadSettings"] = action.Settings
	case ActionFileApproval:
		if !validDesktopID(action.RequestID) || !allowedApprovalDecision(action.Decision) || action.hasFieldsExceptApproval() {
			return wireMessage{}, fmt.Errorf("%w: file approval request and decision are required", ErrInvalidAction)
		}
		method = "thread-follower-file-approval-decision"
		params["requestId"] = action.RequestID
		params["decision"] = action.Decision
	case ActionPermissionApproval:
		if !validDesktopID(action.RequestID) || action.PermissionResponse == nil || !validPermissionResponse(*action.PermissionResponse) || action.hasFieldsExceptPermissionResponse() {
			return wireMessage{}, fmt.Errorf("%w: valid permission response is required", ErrInvalidAction)
		}
		method = "thread-follower-permissions-request-approval-response"
		params["requestId"] = action.RequestID
		params["response"] = action.PermissionResponse
	case ActionSubmitUserInput:
		if !validDesktopID(action.RequestID) || action.UserInputResponse == nil || !validUserInputResponse(*action.UserInputResponse) || action.hasFieldsExceptUserInputResponse() {
			return wireMessage{}, fmt.Errorf("%w: valid user input response is required", ErrInvalidAction)
		}
		method = "thread-follower-submit-user-input"
		params["requestId"] = action.RequestID
		params["response"] = action.UserInputResponse
	case ActionSubmitMCP:
		if !validDesktopID(action.RequestID) || action.MCPResponse == nil || !validMCPResponse(*action.MCPResponse) || action.hasFieldsExceptMCPResponse() {
			return wireMessage{}, fmt.Errorf("%w: valid MCP response is required", ErrInvalidAction)
		}
		method = "thread-follower-submit-mcp-server-elicitation-response"
		params["requestId"] = action.RequestID
		params["response"] = action.MCPResponse
	case ActionEditLastTurn:
		if !validActionText(action.Text) || !validDesktopID(action.TurnID) || !validAgentMode(action.AgentMode) || !validDesktopID(action.ServiceTier) || action.hasFieldsExceptEdit() {
			return wireMessage{}, fmt.Errorf("%w: valid last-turn edit is required", ErrInvalidAction)
		}
		method = "thread-follower-edit-last-user-turn"
		params["turnId"] = action.TurnID
		params["message"] = action.Text
		params["agentMode"] = action.AgentMode
		params["shouldSendPermissionOverrides"] = action.ShouldSendPermissionOverrides
		params["serviceTier"] = action.ServiceTier
	default:
		return wireMessage{}, fmt.Errorf("%w: %s", ErrActionNotAllowed, action.Kind)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return wireMessage{}, fmt.Errorf("encode follower action: %w", err)
	}
	return wireMessage{Type: "request", Method: method, Version: supportedVersions[method], Params: encoded}, nil
}

func (action FollowerAction) hasPayloadFields() bool {
	return action.RequestID != "" || action.Decision != "" || action.Text != "" || action.TurnID != "" || action.AgentMode != "" || action.ServiceTier != "" ||
		len(action.Attachments) != 0 || action.ShouldSendPermissionOverrides || action.Settings != nil || action.PermissionResponse != nil || action.UserInputResponse != nil || action.MCPResponse != nil
}

func (action FollowerAction) hasFieldsExceptSettings() bool {
	copy := action
	copy.Settings = nil
	return copy.hasPayloadFields()
}

func (action FollowerAction) hasFieldsExceptText() bool {
	copy := action
	copy.Text = ""
	copy.Attachments = nil
	return copy.hasPayloadFields()
}

var followerAttachmentIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)

func followerInputItems(text string, attachments []AttachmentInput) ([]followerInput, error) {
	if len(attachments) > 16 {
		return nil, fmt.Errorf("%w: too many attachments", ErrInvalidAction)
	}
	input := make([]followerInput, 0, len(attachments)+1)
	input = append(input, followerInput{Type: "text", Text: text})
	seen := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		mediaType, _, err := mime.ParseMediaType(attachment.MediaType)
		if err != nil || !followerAttachmentIDPattern.MatchString(attachment.ID) || !filepath.IsAbs(attachment.Path) || filepath.Clean(attachment.Path) != attachment.Path || len(attachment.Path) > 4096 {
			return nil, fmt.Errorf("%w: invalid attachment", ErrInvalidAction)
		}
		if _, duplicate := seen[attachment.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate attachment", ErrInvalidAction)
		}
		seen[attachment.ID] = struct{}{}
		if strings.HasPrefix(strings.ToLower(mediaType), "image/") {
			input = append(input, followerInput{Type: "localImage", Path: attachment.Path})
		} else {
			input = append(input, followerInput{Type: "mention", Name: attachment.ID, Path: attachment.Path})
		}
	}
	return input, nil
}

func (action FollowerAction) hasFieldsExceptApproval() bool {
	copy := action
	copy.RequestID, copy.Decision = "", ""
	return copy.hasPayloadFields()
}

func (action FollowerAction) hasFieldsExceptPermissionResponse() bool {
	copy := action
	copy.RequestID, copy.PermissionResponse = "", nil
	return copy.hasPayloadFields()
}

func (action FollowerAction) hasFieldsExceptUserInputResponse() bool {
	copy := action
	copy.RequestID, copy.UserInputResponse = "", nil
	return copy.hasPayloadFields()
}

func (action FollowerAction) hasFieldsExceptMCPResponse() bool {
	copy := action
	copy.RequestID, copy.MCPResponse = "", nil
	return copy.hasPayloadFields()
}

func (action FollowerAction) hasFieldsExceptEdit() bool {
	copy := action
	copy.Text, copy.TurnID, copy.AgentMode, copy.ServiceTier = "", "", "", ""
	copy.ShouldSendPermissionOverrides = false
	return copy.hasPayloadFields()
}

func validThreadSettings(settings ThreadSettings) bool {
	if settings.Model == "" && settings.Effort == "" && settings.ApprovalPolicy == "" && settings.Permissions == "" {
		return false
	}
	return len(settings.Model) <= 256 && validEffort(settings.Effort) && validDesktopApprovalPolicy(settings.ApprovalPolicy) && (settings.Permissions == "" || validDesktopID(settings.Permissions))
}

func validEffort(effort string) bool {
	return effort == "" || strings.TrimSpace(effort) != "" && len(effort) <= 256
}

func validPermissionResponse(response PermissionResponse) bool {
	if response.Scope != "turn" && response.Scope != "session" {
		return false
	}
	var permissions map[string]json.RawMessage
	return len(response.Permissions) > 0 && len(response.Permissions) <= 64*1024 && json.Unmarshal(response.Permissions, &permissions) == nil && permissions != nil
}

func validUserInputResponse(response UserInputResponse) bool {
	if len(response.Answers) == 0 || len(response.Answers) > 32 {
		return false
	}
	totalBytes := 0
	for id, answer := range response.Answers {
		if strings.TrimSpace(id) == "" || len(id) > 256 || len(answer.Answers) == 0 || len(answer.Answers) > 32 {
			return false
		}
		for _, value := range answer.Answers {
			totalBytes += len(value)
			if len(value) > MaxActionTextBytes || totalBytes > MaxActionTextBytes {
				return false
			}
		}
	}
	return true
}

func validMCPResponse(response MCPResponse) bool {
	if response.Action != "accept" && response.Action != "decline" && response.Action != "cancel" {
		return false
	}
	if response.Action != "accept" && len(response.Content) != 0 {
		return false
	}
	if len(response.Content) > 64*1024 {
		return false
	}
	if len(response.Content) != 0 && !json.Valid(response.Content) {
		return false
	}
	return true
}

func validAgentMode(mode string) bool {
	switch mode {
	case "auto", "read-only", "granular", "full-access", "custom", "guardian-approvals":
		return true
	default:
		return false
	}
}

func validDesktopID(value string) bool { return strings.TrimSpace(value) != "" && len(value) <= 256 }

func validDesktopApprovalPolicy(value string) bool {
	return value == "" || value == "untrusted" || value == "on-request" || value == "never"
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
