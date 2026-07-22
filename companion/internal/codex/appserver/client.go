package appserver

import (
	"bufio"
	"bytes"
	"context"
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

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

const (
	maxTurnTextBytes       = 128 * 1024
	maxWireMessageBytes    = 16 * 1024 * 1024
	maxNotificationBytes   = 1024 * 1024
	maxServerRequestBytes  = 256 * 1024
	notificationQueueSize  = 64
	serverRequestQueueSize = 8
)

var (
	ErrAlreadyInitialized = errors.New("app-server client is already initialized")
	ErrClosed             = errors.New("app-server client is closed")
	ErrInvalidInput       = errors.New("app-server input is invalid")
	ErrMessageTooLarge    = errors.New("app-server message is too large")
	ErrNotInitialized     = errors.New("app-server client is not initialized")
	ErrQueueFull          = errors.New("app-server event queue is full")
	ErrRequestMismatch    = errors.New("app-server request does not match the pending action")
	ErrDecisionNotOffered = errors.New("app-server approval decision was not offered")
)

type Options struct{ ExperimentalQuestions bool }
type ListOptions struct{ Limit int }
type SandboxMode string

const (
	SandboxReadOnly         SandboxMode = "read-only"
	SandboxWorkspaceWrite   SandboxMode = "workspace-write"
	SandboxDangerFullAccess SandboxMode = "danger-full-access"
)

type ThreadOptions struct {
	CWD, Model     string
	ApprovalPolicy json.RawMessage
	Sandbox        SandboxMode
}
type TurnOptions struct {
	ThreadID, Text, CWD, Model, Effort string
	ApprovalPolicy                     json.RawMessage
	SandboxPolicy                      json.RawMessage
	Attachments                        []AttachmentInput
}

type AttachmentInput struct {
	ID        string
	Path      string
	MediaType string
}

type userInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type Notification struct {
	Method string
	Params json.RawMessage
}
type ServerRequest struct {
	ID               json.RawMessage
	Method           string
	Params           json.RawMessage
	ThreadID         string
	TurnID           string
	ItemID           string
	StartedAtMs      int64
	Command          string
	CWD              string
	Reason           string
	GrantRoot        string
	ServerName       string
	MCPMode          string
	MCPMessage       string
	AllowedDecisions []ApprovalDecision
	Permissions      json.RawMessage
	Questions        []ServerQuestion
}

type ServerQuestion struct {
	ID      string
	Header  string
	Prompt  string
	Options []string
	Secret  bool
}

type ApprovalDecision string

const (
	DecisionAccept           ApprovalDecision = "accept"
	DecisionAcceptForSession ApprovalDecision = "acceptForSession"
	DecisionDecline          ApprovalDecision = "decline"
	DecisionCancel           ApprovalDecision = "cancel"
)

type wireMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type OutcomeUnknownError struct {
	Method string
	Cause  error
}

func (err *OutcomeUnknownError) Error() string {
	return fmt.Sprintf("%s outcome is unknown: %v", err.Method, err.Cause)
}
func (err *OutcomeUnknownError) Unwrap() error { return err.Cause }

func (err *rpcError) Error() string {
	return fmt.Sprintf("app-server error %d: %s", err.Code, err.Message)
}

func IsThreadNotLoaded(err error, threadID string) bool {
	var remote *rpcError
	return validID(threadID) && errors.As(err, &remote) && remote.Code == -32600 && remote.Message == "thread not loaded: "+threadID
}

type response struct {
	result json.RawMessage
	err    error
}

type serverRequestKind string

const (
	requestKindCommand    serverRequestKind = "command"
	requestKindFile       serverRequestKind = "file"
	requestKindPermission serverRequestKind = "permission"
	requestKindQuestion   serverRequestKind = "question"
	requestKindMCP        serverRequestKind = "mcp"
)

type pendingServerRequest struct {
	kind        serverRequestKind
	threadID    string
	allowed     map[ApprovalDecision]bool
	permissions json.RawMessage
	questionIDs map[string]bool
}

type Client struct {
	reader                 *bufio.Reader
	writer                 io.Writer
	logger                 *slog.Logger
	options                Options
	writeGate              chan struct{}
	mu                     sync.Mutex
	nextID                 uint64
	pending                map[string]chan response
	pendingRequests        map[string]pendingServerRequest
	started, ready, closed bool
	done                   chan struct{}
	events                 chan Notification
	requests               chan ServerRequest
	closers                []io.Closer
	outputsOnce            sync.Once
}

var _ taskstate.TaskAdapter = (*Client)(nil)

func (*Client) TaskSource() taskstate.Source { return taskstate.SourceAppServer }

func NewClient(reader io.Reader, writer io.Writer, logger *slog.Logger, options Options) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	closers := make([]io.Closer, 0, 2)
	if closer, ok := reader.(io.Closer); ok {
		closers = append(closers, closer)
	}
	if closer, ok := writer.(io.Closer); ok {
		closers = append(closers, closer)
	}
	writeGate := make(chan struct{}, 1)
	writeGate <- struct{}{}
	return &Client{reader: bufio.NewReader(reader), writer: writer, writeGate: writeGate, logger: logger, options: options,
		pending: make(map[string]chan response), pendingRequests: make(map[string]pendingServerRequest), done: make(chan struct{}),
		events: make(chan Notification, notificationQueueSize), requests: make(chan ServerRequest, serverRequestQueueSize), closers: closers}
}

func (client *Client) Initialize(ctx context.Context) error {
	client.mu.Lock()
	if client.started {
		client.mu.Unlock()
		return ErrAlreadyInitialized
	}
	client.started = true
	client.mu.Unlock()
	go client.readLoop()
	result, err := client.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "codex_launcher", "title": "Codex Launcher", "version": "0.1.0-alpha.1"},
		"capabilities": map[string]bool{"experimentalApi": client.options.ExperimentalQuestions},
	}, true)
	if err != nil {
		return fmt.Errorf("initialize app-server: %w", err)
	}
	if len(result) == 0 || string(result) == "null" {
		return fmt.Errorf("initialize app-server: empty result")
	}
	if err := validateInitializeResult(result); err != nil {
		client.fail(err)
		return fmt.Errorf("initialize app-server: %w", err)
	}
	if err := client.notify(ctx, "initialized", map[string]any{}); err != nil {
		return fmt.Errorf("acknowledge app-server initialization: %w", err)
	}
	client.mu.Lock()
	client.ready = true
	client.mu.Unlock()
	client.logger.Info("[codex-adapter] initialized", "experimental_questions", client.options.ExperimentalQuestions)
	return nil
}

func validateInitializeResult(result json.RawMessage) error {
	var value struct {
		CodexHome      string `json:"codexHome"`
		PlatformFamily string `json:"platformFamily"`
		PlatformOS     string `json:"platformOs"`
		UserAgent      string `json:"userAgent"`
	}
	if json.Unmarshal(result, &value) != nil || !validBoundedText(value.CodexHome, 4096) || !filepath.IsAbs(value.CodexHome) || !validBoundedText(value.UserAgent, 4096) {
		return ErrInvalidInput
	}
	if value.PlatformFamily != "unix" && value.PlatformFamily != "windows" {
		return ErrInvalidInput
	}
	if value.PlatformOS != "macos" && value.PlatformOS != "linux" && value.PlatformOS != "windows" {
		return ErrInvalidInput
	}
	return nil
}

func (client *Client) Events() <-chan Notification    { return client.events }
func (client *Client) Requests() <-chan ServerRequest { return client.requests }
func (client *Client) Done() <-chan struct{}          { return client.done }

func (client *Client) ListThreads(ctx context.Context, options ListOptions) (json.RawMessage, error) {
	if options.Limit < 0 {
		return nil, ErrInvalidInput
	}
	params := map[string]any{}
	if options.Limit > 0 {
		params["limit"] = options.Limit
	}
	return client.call(ctx, "thread/list", params, false)
}
func (client *Client) ReadThread(ctx context.Context, threadID string, includeTurns bool) (json.RawMessage, error) {
	if !validID(threadID) {
		return nil, ErrInvalidInput
	}
	return client.call(ctx, "thread/read", map[string]any{"threadId": threadID, "includeTurns": includeTurns}, false)
}
func (client *Client) StartThread(ctx context.Context, options ThreadOptions) (json.RawMessage, error) {
	if !validThreadOptions(options) {
		return nil, ErrInvalidInput
	}
	return client.call(ctx, "thread/start", threadParams(options), false)
}
func (client *Client) ResumeThread(ctx context.Context, threadID string, options ThreadOptions) (json.RawMessage, error) {
	if !validID(threadID) {
		return nil, ErrInvalidInput
	}
	if !validThreadOptions(options) {
		return nil, ErrInvalidInput
	}
	params := threadParams(options)
	params["threadId"] = threadID
	return client.call(ctx, "thread/resume", params, false)
}
func (client *Client) ForkThread(ctx context.Context, threadID, lastTurnID string) (json.RawMessage, error) {
	if !validID(threadID) || lastTurnID != "" && !validID(lastTurnID) {
		return nil, ErrInvalidInput
	}
	params := map[string]any{"threadId": threadID}
	if lastTurnID != "" {
		params["lastTurnId"] = lastTurnID
	}
	return client.call(ctx, "thread/fork", params, false)
}
func (client *Client) ArchiveThread(ctx context.Context, threadID string) (json.RawMessage, error) {
	if !validID(threadID) {
		return nil, ErrInvalidInput
	}
	return client.call(ctx, "thread/archive", map[string]any{"threadId": threadID}, false)
}
func (client *Client) SetThreadName(ctx context.Context, threadID, name string) (json.RawMessage, error) {
	if !validID(threadID) || strings.TrimSpace(name) == "" || len(name) > 256 {
		return nil, ErrInvalidInput
	}
	return client.call(ctx, "thread/name/set", map[string]any{"threadId": threadID, "name": name}, false)
}
func (client *Client) ListModels(ctx context.Context) (json.RawMessage, error) {
	return client.call(ctx, "model/list", map[string]any{}, false)
}
func (client *Client) StartTurn(ctx context.Context, options TurnOptions) (json.RawMessage, error) {
	if !validID(options.ThreadID) || !validText(options.Text) || !validEffort(options.Effort) ||
		!validOptional(options.CWD, 4096) || !validOptional(options.Model, 256) || !validApprovalPolicy(options.ApprovalPolicy) || !validSandboxPolicy(options.SandboxPolicy) {
		return nil, ErrInvalidInput
	}
	input, err := turnInput(options.Text, options.Attachments)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"threadId": options.ThreadID, "input": input}
	putString(params, "cwd", options.CWD)
	putString(params, "model", options.Model)
	putString(params, "effort", options.Effort)
	putRaw(params, "approvalPolicy", options.ApprovalPolicy)
	if len(options.SandboxPolicy) != 0 {
		params["sandboxPolicy"] = options.SandboxPolicy
	}
	return client.call(ctx, "turn/start", params, false)
}
func (client *Client) SteerTurn(ctx context.Context, threadID, expectedTurnID, text string) (json.RawMessage, error) {
	return client.SteerTurnWithAttachments(ctx, threadID, expectedTurnID, text, nil)
}

func (client *Client) SteerTurnWithAttachments(ctx context.Context, threadID, expectedTurnID, text string, attachments []AttachmentInput) (json.RawMessage, error) {
	if !validID(threadID) || !validID(expectedTurnID) || !validText(text) {
		return nil, ErrInvalidInput
	}
	input, err := turnInput(text, attachments)
	if err != nil {
		return nil, err
	}
	return client.call(ctx, "turn/steer", map[string]any{"threadId": threadID, "expectedTurnId": expectedTurnID, "input": input}, false)
}
func (client *Client) InterruptTurn(ctx context.Context, threadID, turnID string) (json.RawMessage, error) {
	if !validID(threadID) || !validID(turnID) {
		return nil, ErrInvalidInput
	}
	client.logger.Info("[codex-adapter] turn interrupt requested", "task_id", threadID, "turn_id", turnID)
	result, err := client.call(ctx, "turn/interrupt", map[string]any{"threadId": threadID, "turnId": turnID}, false)
	if err != nil {
		remoteCode := 0
		var remote *rpcError
		if errors.As(err, &remote) {
			remoteCode = remote.Code
		}
		client.logger.Error("[codex-adapter] turn interrupt rejected", "task_id", threadID, "turn_id", turnID, "remote_code", remoteCode, "error_class", fmt.Sprintf("%T", err))
	}
	return result, err
}

func (client *Client) RespondCommandApproval(ctx context.Context, threadID string, id json.RawMessage, decision ApprovalDecision) error {
	return client.respondApproval(ctx, threadID, id, decision, requestKindCommand)
}

func (client *Client) RespondFileApproval(ctx context.Context, threadID string, id json.RawMessage, decision ApprovalDecision) error {
	return client.respondApproval(ctx, threadID, id, decision, requestKindFile)
}

func (client *Client) respondApproval(ctx context.Context, threadID string, id json.RawMessage, decision ApprovalDecision, kind serverRequestKind) error {
	if !validID(threadID) || !validRequestID(id) || !validDecision(decision) {
		return ErrInvalidInput
	}
	if err := client.takeApprovalRequest(threadID, id, decision, kind); err != nil {
		return err
	}
	return client.respond(ctx, id, map[string]any{"decision": decision})
}
func (client *Client) RespondUserInput(ctx context.Context, threadID string, id json.RawMessage, answers map[string][]string) error {
	if !validID(threadID) || !validRequestID(id) || !client.options.ExperimentalQuestions || len(answers) == 0 {
		return ErrInvalidInput
	}
	wrapped := make(map[string]map[string][]string, len(answers))
	totalBytes := 0
	for questionID, values := range answers {
		if !validID(questionID) || len(values) == 0 || len(values) > 32 {
			return ErrInvalidInput
		}
		for _, value := range values {
			totalBytes += len(value)
			if len(value) > maxTurnTextBytes || totalBytes > maxTurnTextBytes {
				return ErrInvalidInput
			}
		}
		wrapped[questionID] = map[string][]string{"answers": values}
	}
	if err := client.takeQuestionRequest(threadID, id, wrapped); err != nil {
		return err
	}
	return client.respond(ctx, id, map[string]any{"answers": wrapped})
}
func (client *Client) RespondMCP(ctx context.Context, threadID string, id json.RawMessage, action string, content any) error {
	if !validID(threadID) || !validRequestID(id) || (action != "accept" && action != "decline" && action != "cancel") || action != "accept" && content != nil || !validMCPContent(content) {
		return ErrInvalidInput
	}
	if _, err := client.takePendingRequest(threadID, id, requestKindMCP); err != nil {
		return err
	}
	result := map[string]any{"action": action}
	if content != nil {
		result["content"] = content
	}
	return client.respond(ctx, id, result)
}

func (client *Client) RespondPermissions(ctx context.Context, threadID string, id json.RawMessage, granted json.RawMessage, scope string) error {
	if !validID(threadID) || !validRequestID(id) || (scope != "turn" && scope != "session") || len(granted) == 0 || len(granted) > 64*1024 {
		return ErrInvalidInput
	}
	var permissions map[string]json.RawMessage
	if json.Unmarshal(granted, &permissions) != nil || permissions == nil {
		return ErrInvalidInput
	}
	if err := client.takePermissionRequest(threadID, id, granted); err != nil {
		return err
	}
	return client.respond(ctx, id, map[string]any{"permissions": permissions, "scope": scope})
}

func (client *Client) call(ctx context.Context, method string, params any, beforeReady bool) (json.RawMessage, error) {
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return nil, ErrClosed
	}
	if !beforeReady && !client.ready {
		client.mu.Unlock()
		return nil, ErrNotInitialized
	}
	client.nextID++
	id := client.nextID
	key := fmt.Sprintf("%d", id)
	responseChannel := make(chan response, 1)
	client.pending[key] = responseChannel
	client.mu.Unlock()
	if err := client.write(ctx, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		client.removePending(key)
		client.fail(err)
		if mutatingMethod(method) {
			return nil, &OutcomeUnknownError{Method: method, Cause: err}
		}
		return nil, err
	}
	client.logger.Debug("[codex-adapter] request sent", "method", method, "request_id", id)
	select {
	case response := <-responseChannel:
		if response.err != nil {
			return nil, response.err
		}
		client.logger.Debug("[codex-adapter] response received", "method", method, "request_id", id)
		return response.result, nil
	case <-ctx.Done():
		client.fail(ctx.Err())
		if mutatingMethod(method) {
			return nil, &OutcomeUnknownError{Method: method, Cause: ctx.Err()}
		}
		return nil, ctx.Err()
	case <-client.done:
		if mutatingMethod(method) {
			return nil, &OutcomeUnknownError{Method: method, Cause: ErrClosed}
		}
		return nil, ErrClosed
	}
}
func (client *Client) notify(ctx context.Context, method string, params any) error {
	return client.write(ctx, map[string]any{"method": method, "params": params})
}
func (client *Client) respond(ctx context.Context, id json.RawMessage, result any) error {
	if err := client.write(ctx, map[string]any{"id": id, "result": result}); err != nil {
		client.fail(err)
		return &OutcomeUnknownError{Method: "server-request response", Cause: err}
	}
	return nil
}
func (client *Client) write(ctx context.Context, message any) error {
	client.mu.Lock()
	closed := client.closed
	client.mu.Unlock()
	if closed {
		return ErrClosed
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode app-server message: %w", err)
	}
	encoded = append(encoded, '\n')
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-client.done:
		return ErrClosed
	case <-client.writeGate:
	}
	result := make(chan error, 1)
	go func() { result <- writeAll(client.writer, encoded) }()
	select {
	case err := <-result:
		client.writeGate <- struct{}{}
		if err != nil {
			client.fail(err)
			return fmt.Errorf("write app-server message: %w", err)
		}
		return nil
	case <-ctx.Done():
		client.fail(ctx.Err())
		return ctx.Err()
	case <-client.done:
		return ErrClosed
	}
}

func writeAll(writer io.Writer, value []byte) error {
	for len(value) != 0 {
		written, err := writer.Write(value)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}

func (client *Client) readLoop() {
	defer client.closeOutputs()
	for {
		line, err := readBoundedLine(client.reader, maxWireMessageBytes)
		if err != nil {
			client.fail(fmt.Errorf("read app-server message: %w", err))
			return
		}
		message, err := decodeWire(line)
		if err != nil {
			client.fail(fmt.Errorf("decode app-server message: %w", err))
			return
		}
		if len(message.ID) != 0 && message.Method == "" {
			client.routeResponse(message)
			continue
		}
		if message.Method == "" {
			client.fail(ErrInvalidInput)
			return
		}
		if len(message.ID) != 0 {
			if !client.allowedServerRequest(message.Method) {
				client.fail(fmt.Errorf("unsupported app-server request %s", message.Method))
				return
			}
			if len(message.Params) > maxServerRequestBytes {
				client.fail(ErrMessageTooLarge)
				return
			}
			request, err := client.registerServerRequest(message)
			if err != nil {
				client.fail(err)
				return
			}
			select {
			case client.requests <- request:
				client.logger.Info("[codex-adapter] server request", "method", message.Method)
			default:
				client.fail(ErrQueueFull)
				return
			}
			continue
		}
		if len(message.Params) > maxNotificationBytes {
			client.fail(ErrMessageTooLarge)
			return
		}
		event := Notification{Method: message.Method, Params: append(json.RawMessage(nil), message.Params...)}
		select {
		case client.events <- event:
			client.logger.Debug("[codex-adapter] event", "method", message.Method)
		default:
			client.fail(ErrQueueFull)
			return
		}
	}
}

func decodeWire(frame []byte) (wireMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	var message wireMessage
	if err := decoder.Decode(&message); err != nil {
		return wireMessage{}, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return wireMessage{}, ErrInvalidInput
	}
	hasID := len(message.ID) != 0 && string(message.ID) != "null"
	if hasID && !validRequestID(message.ID) {
		return wireMessage{}, ErrInvalidInput
	}
	hasResult := len(message.Result) != 0
	hasError := message.Error != nil
	if message.Method == "" {
		if !hasID || len(message.Params) != 0 || hasResult == hasError || hasError && message.Error.Message == "" {
			return wireMessage{}, ErrInvalidInput
		}
		return message, nil
	}
	if hasResult || hasError || len(message.Params) == 0 {
		return wireMessage{}, ErrInvalidInput
	}
	return message, nil
}

func readBoundedLine(reader *bufio.Reader, maximum int) ([]byte, error) {
	line := make([]byte, 0, min(maximum, 4096))
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > maximum-len(line) {
			return nil, ErrMessageTooLarge
		}
		line = append(line, fragment...)
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}
func (client *Client) routeResponse(message wireMessage) {
	key := string(message.ID)
	client.mu.Lock()
	channel := client.pending[key]
	delete(client.pending, key)
	client.mu.Unlock()
	if channel == nil {
		client.fail(fmt.Errorf("response for unknown request id %s", key))
		return
	}
	if message.Error != nil {
		channel <- response{err: message.Error}
		return
	}
	if len(message.Result) == 0 {
		channel <- response{err: ErrInvalidInput}
		return
	}
	channel <- response{result: append(json.RawMessage(nil), message.Result...)}
}
func (client *Client) fail(err error) {
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return
	}
	client.closed = true
	pending := client.pending
	client.pending = make(map[string]chan response)
	close(client.done)
	closers := append([]io.Closer(nil), client.closers...)
	client.mu.Unlock()
	for _, closer := range closers {
		_ = closer.Close()
	}
	client.logger.Error("[codex-adapter] connection closed", "error", err)
	for _, channel := range pending {
		channel <- response{err: err}
	}
}

func (client *Client) closeOutputs() {
	client.outputsOnce.Do(func() {
		close(client.events)
		close(client.requests)
	})
}
func (client *Client) removePending(key string) {
	client.mu.Lock()
	delete(client.pending, key)
	client.mu.Unlock()
}

func (client *Client) registerServerRequest(message wireMessage) (ServerRequest, error) {
	var params struct {
		ThreadID           string             `json:"threadId"`
		TurnID             string             `json:"turnId"`
		ItemID             string             `json:"itemId"`
		StartedAtMs        *int64             `json:"startedAtMs"`
		CWD                string             `json:"cwd"`
		Command            string             `json:"command"`
		Reason             string             `json:"reason"`
		GrantRoot          string             `json:"grantRoot"`
		ServerName         string             `json:"serverName"`
		Mode               string             `json:"mode"`
		Message            string             `json:"message"`
		AvailableDecisions []ApprovalDecision `json:"availableDecisions"`
		Permissions        json.RawMessage    `json:"permissions"`
		Questions          []struct {
			ID       string `json:"id"`
			Header   string `json:"header"`
			Question string `json:"question"`
			IsSecret bool   `json:"isSecret"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(message.Params, &params) != nil || !validID(params.ThreadID) {
		return ServerRequest{}, ErrInvalidInput
	}
	kind := requestKindForMethod(message.Method)
	if kind == "" {
		return ServerRequest{}, ErrInvalidInput
	}
	if kind == requestKindCommand || kind == requestKindFile || kind == requestKindPermission || kind == requestKindQuestion {
		if !validID(params.TurnID) || !validID(params.ItemID) {
			return ServerRequest{}, ErrInvalidInput
		}
	}
	if kind == requestKindCommand || kind == requestKindFile || kind == requestKindPermission {
		if params.StartedAtMs == nil || *params.StartedAtMs < 0 {
			return ServerRequest{}, ErrInvalidInput
		}
	}
	if kind == requestKindPermission && !validBoundedText(params.CWD, 4096) {
		return ServerRequest{}, ErrInvalidInput
	}
	if params.Command != "" && !validBoundedText(params.Command, 4096) || params.CWD != "" && !validBoundedText(params.CWD, 4096) ||
		params.Reason != "" && !validBoundedText(params.Reason, 4096) || params.GrantRoot != "" && !validBoundedText(params.GrantRoot, 4096) {
		return ServerRequest{}, ErrInvalidInput
	}
	if kind == requestKindMCP && !validBoundedText(params.ServerName, 256) {
		return ServerRequest{}, ErrInvalidInput
	}
	if kind == requestKindMCP && (params.Mode != "form" && params.Mode != "openai/form" && params.Mode != "url" || !validBoundedText(params.Message, 4096)) {
		return ServerRequest{}, ErrInvalidInput
	}
	allowed := make(map[ApprovalDecision]bool)
	if kind == requestKindCommand || kind == requestKindFile {
		decisions := params.AvailableDecisions
		if len(decisions) == 0 {
			decisions = []ApprovalDecision{DecisionAccept, DecisionAcceptForSession, DecisionDecline, DecisionCancel}
		}
		for _, decision := range decisions {
			if !validDecision(decision) {
				return ServerRequest{}, ErrInvalidInput
			}
			allowed[decision] = true
		}
	}
	if kind == requestKindPermission {
		var permissions map[string]json.RawMessage
		if len(params.Permissions) == 0 || json.Unmarshal(params.Permissions, &permissions) != nil || permissions == nil {
			return ServerRequest{}, ErrInvalidInput
		}
	}
	questionIDs := make(map[string]bool)
	questions := make([]ServerQuestion, 0, len(params.Questions))
	if kind == requestKindQuestion {
		if len(params.Questions) == 0 || len(params.Questions) > 32 {
			return ServerRequest{}, ErrInvalidInput
		}
		for _, question := range params.Questions {
			if !validID(question.ID) || !validBoundedText(question.Header, 256) || !validBoundedText(question.Question, 4096) || questionIDs[question.ID] {
				return ServerRequest{}, ErrInvalidInput
			}
			options := make([]string, 0, len(question.Options))
			if len(question.Options) > 32 {
				return ServerRequest{}, ErrInvalidInput
			}
			for _, option := range question.Options {
				if !validBoundedText(option.Label, 512) {
					return ServerRequest{}, ErrInvalidInput
				}
				options = append(options, option.Label)
			}
			questionIDs[question.ID] = true
			questions = append(questions, ServerQuestion{ID: question.ID, Header: question.Header, Prompt: question.Question, Options: options, Secret: question.IsSecret})
		}
	}
	key := string(message.ID)
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return ServerRequest{}, ErrClosed
	}
	if _, exists := client.pendingRequests[key]; exists {
		return ServerRequest{}, ErrInvalidInput
	}
	client.pendingRequests[key] = pendingServerRequest{kind: kind, threadID: params.ThreadID, allowed: allowed, permissions: append(json.RawMessage(nil), params.Permissions...), questionIDs: questionIDs}
	decisions := make([]ApprovalDecision, 0, len(allowed))
	for _, decision := range []ApprovalDecision{DecisionAccept, DecisionAcceptForSession, DecisionDecline, DecisionCancel} {
		if allowed[decision] {
			decisions = append(decisions, decision)
		}
	}
	startedAtMs := int64(0)
	if params.StartedAtMs != nil {
		startedAtMs = *params.StartedAtMs
	}
	return ServerRequest{
		ID: append(json.RawMessage(nil), message.ID...), Method: message.Method, Params: append(json.RawMessage(nil), message.Params...),
		ThreadID: params.ThreadID, TurnID: params.TurnID, ItemID: params.ItemID, StartedAtMs: startedAtMs, Command: params.Command, CWD: params.CWD, Reason: params.Reason,
		GrantRoot: params.GrantRoot, ServerName: params.ServerName, MCPMode: params.Mode, MCPMessage: params.Message,
		AllowedDecisions: decisions, Permissions: append(json.RawMessage(nil), params.Permissions...), Questions: questions,
	}, nil
}

func (client *Client) takeApprovalRequest(threadID string, id json.RawMessage, decision ApprovalDecision, kind serverRequestKind) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return ErrClosed
	}
	if !client.ready {
		return ErrNotInitialized
	}
	key := string(id)
	pending, exists := client.pendingRequests[key]
	if !exists || pending.kind != kind || pending.threadID != threadID {
		return ErrRequestMismatch
	}
	if !pending.allowed[decision] {
		return ErrDecisionNotOffered
	}
	delete(client.pendingRequests, key)
	return nil
}

func (client *Client) takePermissionRequest(threadID string, id, granted json.RawMessage) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return ErrClosed
	}
	if !client.ready {
		return ErrNotInitialized
	}
	key := string(id)
	pending, exists := client.pendingRequests[key]
	if !exists || pending.kind != requestKindPermission || pending.threadID != threadID {
		return ErrRequestMismatch
	}
	if !permissionSubset(granted, pending.permissions) {
		return ErrDecisionNotOffered
	}
	delete(client.pendingRequests, key)
	return nil
}

func (client *Client) takeQuestionRequest(threadID string, id json.RawMessage, answers map[string]map[string][]string) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return ErrClosed
	}
	if !client.ready {
		return ErrNotInitialized
	}
	key := string(id)
	pending, exists := client.pendingRequests[key]
	if !exists || pending.kind != requestKindQuestion || pending.threadID != threadID {
		return ErrRequestMismatch
	}
	if len(answers) != len(pending.questionIDs) {
		return ErrRequestMismatch
	}
	for questionID := range answers {
		if !pending.questionIDs[questionID] {
			return ErrRequestMismatch
		}
	}
	delete(client.pendingRequests, key)
	return nil
}

func permissionSubset(granted, requested json.RawMessage) bool {
	return taskstate.PermissionSubset(granted, requested)
}

func (client *Client) takePendingRequest(threadID string, id json.RawMessage, accepted ...serverRequestKind) (pendingServerRequest, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return pendingServerRequest{}, ErrClosed
	}
	if !client.ready {
		return pendingServerRequest{}, ErrNotInitialized
	}
	key := string(id)
	pending, exists := client.pendingRequests[key]
	if !exists {
		return pendingServerRequest{}, ErrRequestMismatch
	}
	if pending.threadID != threadID {
		return pendingServerRequest{}, ErrRequestMismatch
	}
	matched := false
	for _, kind := range accepted {
		matched = matched || pending.kind == kind
	}
	if !matched {
		return pendingServerRequest{}, ErrRequestMismatch
	}
	delete(client.pendingRequests, key)
	return pending, nil
}

func threadParams(options ThreadOptions) map[string]any {
	params := map[string]any{}
	putString(params, "cwd", options.CWD)
	putString(params, "model", options.Model)
	putRaw(params, "approvalPolicy", options.ApprovalPolicy)
	if options.Sandbox != "" {
		params["sandbox"] = options.Sandbox
	}
	return params
}

var attachmentIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)

func turnInput(text string, attachments []AttachmentInput) ([]userInput, error) {
	if !validText(text) || len(attachments) > 16 {
		return nil, ErrInvalidInput
	}
	input := make([]userInput, 0, len(attachments)+1)
	input = append(input, userInput{Type: "text", Text: text})
	seen := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		mediaType, _, err := mime.ParseMediaType(attachment.MediaType)
		if err != nil || !attachmentIDPattern.MatchString(attachment.ID) || !filepath.IsAbs(attachment.Path) || filepath.Clean(attachment.Path) != attachment.Path || len(attachment.Path) > 4096 {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[attachment.ID]; duplicate {
			return nil, ErrInvalidInput
		}
		seen[attachment.ID] = struct{}{}
		if strings.HasPrefix(strings.ToLower(mediaType), "image/") {
			input = append(input, userInput{Type: "localImage", Path: attachment.Path})
		} else {
			input = append(input, userInput{Type: "mention", Name: attachment.ID, Path: attachment.Path})
		}
	}
	return input, nil
}
func putString(values map[string]any, key, value string) {
	if value != "" {
		values[key] = value
	}
}
func putRaw(values map[string]any, key string, value json.RawMessage) {
	if len(value) != 0 {
		values[key] = value
	}
}
func validID(value string) bool                    { return strings.TrimSpace(value) != "" && len(value) <= 256 }
func validOptional(value string, maximum int) bool { return value == "" || len(value) <= maximum }
func validBoundedText(value string, maximum int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maximum
}
func validText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxTurnTextBytes
}
func validEffort(value string) bool {
	return value == "" || validBoundedText(value, 256)
}

func validThreadOptions(options ThreadOptions) bool {
	return validOptional(options.CWD, 4096) && validOptional(options.Model, 256) && validApprovalPolicy(options.ApprovalPolicy) && validSandboxMode(options.Sandbox)
}

func validJSONObject(raw json.RawMessage, maximum int) bool {
	if len(raw) == 0 {
		return true
	}
	if len(raw) > maximum {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func validMCPContent(content any) bool {
	if content == nil {
		return true
	}
	encoded, err := json.Marshal(content)
	return err == nil && len(encoded) <= 64*1024
}

func validSandboxMode(mode SandboxMode) bool {
	return mode == "" || mode == SandboxReadOnly || mode == SandboxWorkspaceWrite || mode == SandboxDangerFullAccess
}

func validApprovalPolicy(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var simple string
	if json.Unmarshal(raw, &simple) == nil {
		return simple == "untrusted" || simple == "on-request" || simple == "never"
	}
	var outer map[string]json.RawMessage
	if json.Unmarshal(raw, &outer) != nil || len(outer) != 1 {
		return false
	}
	var granular map[string]json.RawMessage
	if json.Unmarshal(outer["granular"], &granular) != nil {
		return false
	}
	required := map[string]bool{"mcp_elicitations": false, "rules": false, "sandbox_approval": false}
	for key, value := range granular {
		if _, needed := required[key]; needed {
			required[key] = true
		} else if key != "request_permissions" && key != "skill_approval" {
			return false
		}
		var flag bool
		if json.Unmarshal(value, &flag) != nil {
			return false
		}
	}
	for _, present := range required {
		if !present {
			return false
		}
	}
	return true
}

func validSandboxPolicy(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	if len(raw) > 64*1024 {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	var kind string
	if json.Unmarshal(object["type"], &kind) != nil {
		return false
	}
	allowedByKind := map[string]map[string]bool{
		"dangerFullAccess": {"type": true},
		"readOnly":         {"type": true, "networkAccess": true},
		"externalSandbox":  {"type": true, "networkAccess": true},
		"workspaceWrite":   {"type": true, "networkAccess": true, "excludeSlashTmp": true, "excludeTmpdirEnvVar": true, "writableRoots": true},
	}
	allowed := allowedByKind[kind]
	if allowed == nil {
		return false
	}
	for key := range object {
		if !allowed[key] {
			return false
		}
	}
	for _, key := range []string{"networkAccess", "excludeSlashTmp", "excludeTmpdirEnvVar"} {
		value := object[key]
		if len(value) == 0 {
			continue
		}
		if kind == "externalSandbox" && key == "networkAccess" {
			var network string
			if json.Unmarshal(value, &network) != nil || network != "restricted" && network != "enabled" {
				return false
			}
			continue
		}
		var flag bool
		if json.Unmarshal(value, &flag) != nil {
			return false
		}
	}
	if value := object["writableRoots"]; len(value) != 0 {
		var roots []string
		if json.Unmarshal(value, &roots) != nil || len(roots) > 256 {
			return false
		}
		for _, root := range roots {
			if !validBoundedText(root, 4096) {
				return false
			}
		}
	}
	return true
}
func validDecision(value ApprovalDecision) bool {
	return value == DecisionAccept || value == DecisionAcceptForSession || value == DecisionDecline || value == DecisionCancel
}
func validRequestID(id json.RawMessage) bool {
	var stringID string
	if json.Unmarshal(id, &stringID) == nil {
		return stringID != ""
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	if decoder.Decode(&number) != nil {
		return false
	}
	_, err := strconv.ParseInt(number.String(), 10, 64)
	return err == nil
}

func mutatingMethod(method string) bool {
	switch method {
	case "thread/start", "thread/resume", "thread/fork", "thread/archive", "thread/name/set", "turn/start", "turn/steer", "turn/interrupt":
		return true
	default:
		return false
	}
}

func (client *Client) allowedServerRequest(method string) bool {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval", "mcpServer/elicitation/request":
		return true
	case "item/tool/requestUserInput":
		return client.options.ExperimentalQuestions
	default:
		return false
	}
}

func requestKindForMethod(method string) serverRequestKind {
	switch method {
	case "item/commandExecution/requestApproval":
		return requestKindCommand
	case "item/fileChange/requestApproval":
		return requestKindFile
	case "item/permissions/requestApproval":
		return requestKindPermission
	case "item/tool/requestUserInput":
		return requestKindQuestion
	case "mcpServer/elicitation/request":
		return requestKindMCP
	default:
		return ""
	}
}
