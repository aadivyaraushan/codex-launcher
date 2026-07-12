package desktopipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrDisconnected        = errors.New("desktop IPC disconnected")
	ErrIncompatibleBuild   = errors.New("ChatGPT Desktop build is incompatible")
	ErrOwnerUnavailable    = errors.New("desktop task owner is unavailable")
	ErrPlatformUnsupported = errors.New("desktop IPC platform is unsupported")
	ErrRemote              = errors.New("desktop IPC request failed")
	ErrUnsafeEndpoint      = errors.New("desktop IPC endpoint is unsafe")
	ErrWriteNotSent        = errors.New("desktop action was not sent")
	ErrWriteOutcomeUnknown = errors.New("desktop action outcome is unknown")
)

const PinnedDesktopBuild = "26.707.51957"

const (
	darwinDesktopExecutable = "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"
	darwinDesktopInfoPlist  = "/Applications/ChatGPT.app/Contents/Info.plist"
)

type WriteOutcome string

const (
	WriteNotSent        WriteOutcome = "not_sent"
	WriteOutcomeUnknown WriteOutcome = "outcome_unknown"
)

type ActionRequestError struct {
	Outcome WriteOutcome
	Cause   error
}

func (err *ActionRequestError) Error() string {
	return fmt.Sprintf("desktop action %s: %v", err.Outcome, err.Cause)
}

func (err *ActionRequestError) Unwrap() error {
	return err.Cause
}

func (err *ActionRequestError) Is(target error) bool {
	return err.Outcome == WriteNotSent && target == ErrWriteNotSent ||
		err.Outcome == WriteOutcomeUnknown && target == ErrWriteOutcomeUnknown
}

type deadlineConn interface {
	SetDeadline(time.Time) error
}

func discoverEndpoint(goos, tempDir string, uid int) (network string, address string, err error) {
	switch goos {
	case "darwin":
		if tempDir == "" || uid < 0 {
			return "", "", fmt.Errorf("%w: missing macOS user boundary", ErrUnsafeEndpoint)
		}
		return "unix", filepath.Join(tempDir, "codex-ipc", fmt.Sprintf("ipc-%d.sock", uid)), nil
	case "windows":
		return "npipe", `\\.\pipe\codex-ipc`, nil
	default:
		return "", "", fmt.Errorf("%w: %s", ErrPlatformUnsupported, goos)
	}
}

func verifyUnixEndpoint(privateTempRoot, socketPath string, expectedUID int) error {
	if expectedUID < 0 {
		return fmt.Errorf("%w: invalid expected user", ErrUnsafeEndpoint)
	}
	root, err := filepath.Abs(privateTempRoot)
	if err != nil {
		return fmt.Errorf("%w: resolve private temp root", ErrUnsafeEndpoint)
	}
	endpoint, err := filepath.Abs(socketPath)
	if err != nil {
		return fmt.Errorf("%w: resolve socket", ErrUnsafeEndpoint)
	}
	relative, err := filepath.Rel(root, endpoint)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: socket is outside private temp root", ErrUnsafeEndpoint)
	}
	rootInfo, err := os.Lstat(root)
	rootUID, rootHasUID := fileOwnerUID(rootInfo)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() || rootInfo.Mode().Perm()&0o077 != 0 || !rootHasUID || rootUID != uint64(expectedUID) {
		return fmt.Errorf("%w: temp root is not owner-only", ErrUnsafeEndpoint)
	}
	current := root
	parts := strings.Split(relative, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: socket parent is not a real directory", ErrUnsafeEndpoint)
		}
	}
	endpointInfo, err := os.Lstat(endpoint)
	endpointUID, endpointHasUID := fileOwnerUID(endpointInfo)
	if err != nil || endpointInfo.Mode()&os.ModeSocket == 0 || !endpointHasUID || endpointUID != uint64(expectedUID) {
		return fmt.Errorf("%w: endpoint is not a Unix socket", ErrUnsafeEndpoint)
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint64, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	uid := value.FieldByName("Uid")
	if !uid.IsValid() || !uid.CanUint() {
		return 0, false
	}
	return uid.Uint(), true
}

func verifyDarwinSocketOwner(ctx context.Context, socketPath string, expectedUID int) error {
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-U", "-Fpcun").Output()
	if err != nil {
		return fmt.Errorf("%w: cannot inspect Unix socket owner", ErrUnsafeEndpoint)
	}
	pid, err := darwinSocketOwnerPID(output, socketPath, expectedUID)
	if err != nil {
		return err
	}
	executable, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return fmt.Errorf("%w: cannot inspect ChatGPT executable", ErrUnsafeEndpoint)
	}
	return verifyDarwinExecutablePath(executable, darwinDesktopExecutable)
}

func verifyDarwinSocketOwnerOutput(output []byte, socketPath string, expectedUID int) error {
	_, err := darwinSocketOwnerPID(output, socketPath, expectedUID)
	return err
}

func darwinSocketOwnerPID(output []byte, socketPath string, expectedUID int) (int, error) {
	command := ""
	uid := -1
	pid := -1
	for _, field := range strings.Split(string(output), "\n") {
		if field == "" {
			continue
		}
		switch field[0] {
		case 'p':
			command = ""
			uid = -1
			pid, _ = strconv.Atoi(field[1:])
		case 'c':
			command = field[1:]
		case 'u':
			parsed, err := strconv.Atoi(field[1:])
			if err == nil {
				uid = parsed
			}
		case 'n':
			if field[1:] == socketPath && command == "ChatGPT" && uid == expectedUID && pid > 0 {
				return pid, nil
			}
		}
	}
	return 0, fmt.Errorf("%w: socket is not owned by the current ChatGPT process", ErrUnsafeEndpoint)
}

func verifyDarwinExecutablePath(output []byte, expectedPath string) error {
	if strings.TrimSpace(string(output)) != expectedPath {
		return fmt.Errorf("%w: socket owner is not the pinned ChatGPT bundle", ErrUnsafeEndpoint)
	}
	return nil
}

type Client struct {
	connection io.ReadWriteCloser
	logger     *slog.Logger

	requestMu   sync.Mutex
	writeMu     sync.Mutex
	stateMu     sync.RWMutex
	pendingMu   sync.Mutex
	readerOnce  sync.Once
	doneOnce    sync.Once
	clientID    string
	streams     map[string]*StreamState
	pending     map[string]chan wireMessage
	done        chan struct{}
	terminalErr error
	buildErr    error
}

type sessionDialer func(context.Context) (io.ReadWriteCloser, string, error)

type SessionConnector struct {
	clientType string
	logger     *slog.Logger
	dial       sessionDialer
	mu         sync.Mutex
	current    *Client
}

func NewDarwinSessionConnector(clientType string, logger *slog.Logger) *SessionConnector {
	return newSessionConnector(clientType, logger, dialVerifiedDarwinDesktop)
}

func newTestSessionConnector(clientType string, logger *slog.Logger, dial sessionDialer) *SessionConnector {
	return newSessionConnector(clientType, logger, dial)
}

func newSessionConnector(clientType string, logger *slog.Logger, dial sessionDialer) *SessionConnector {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &SessionConnector{clientType: clientType, logger: logger, dial: dial}
}

func dialVerifiedDarwinDesktop(ctx context.Context) (io.ReadWriteCloser, string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, "", fmt.Errorf("read current macOS user: %w", err)
	}
	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return nil, "", fmt.Errorf("parse current macOS user ID: %w", err)
	}
	network, socketPath, err := discoverEndpoint("darwin", os.TempDir(), uid)
	if err != nil {
		return nil, "", err
	}
	if err := verifyUnixEndpoint(os.TempDir(), socketPath, uid); err != nil {
		return nil, "", err
	}
	if err := verifyDarwinSocketOwner(ctx, socketPath, uid); err != nil {
		return nil, "", err
	}
	build, err := readDarwinDesktopBuild(darwinDesktopInfoPlist)
	if err != nil {
		return nil, "", err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, network, socketPath)
	if err != nil {
		return nil, "", fmt.Errorf("dial verified ChatGPT Desktop socket: %w", err)
	}
	return connection, build, nil
}

func (connector *SessionConnector) Connect(ctx context.Context) (*Client, error) {
	connector.mu.Lock()
	defer connector.mu.Unlock()
	if connector.current != nil {
		select {
		case <-connector.current.done:
			connector.logger.Info("[desktop-ipc] reconnecting", "branch_reason", "previous_connection_stopped")
		default:
			return connector.current, nil
		}
	}
	if connector.dial == nil {
		return nil, errors.New("desktop IPC dialer is required")
	}
	connection, desktopBuild, err := connector.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect desktop IPC: %w", err)
	}
	client := newClient(connection, desktopBuild, connector.logger)
	if err := client.initialize(ctx, connector.clientType); err != nil {
		_ = connection.Close()
		return nil, err
	}
	connector.current = client
	return client, nil
}

func newClient(connection io.ReadWriteCloser, desktopBuild string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		connection: connection,
		logger:     logger,
		streams:    make(map[string]*StreamState),
		pending:    make(map[string]chan wireMessage),
		done:       make(chan struct{}),
		buildErr:   verifyDesktopBuild(desktopBuild),
	}
}

func verifyDesktopBuild(desktopBuild string) error {
	if desktopBuild != PinnedDesktopBuild {
		return fmt.Errorf("%w: got %q, want %q", ErrIncompatibleBuild, desktopBuild, PinnedDesktopBuild)
	}
	return nil
}

func readDarwinDesktopBuild(infoPlistPath string) (string, error) {
	file, err := os.Open(infoPlistPath)
	if err != nil {
		return "", fmt.Errorf("read ChatGPT Info.plist: %w", err)
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	wantValue := false
	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return "", fmt.Errorf("parse ChatGPT Info.plist: %w", tokenErr)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var key string
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return "", fmt.Errorf("parse ChatGPT Info.plist key: %w", err)
			}
			wantValue = key == "CFBundleShortVersionString"
		case "string":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", fmt.Errorf("parse ChatGPT Info.plist value: %w", err)
			}
			if wantValue {
				if value == "" {
					break
				}
				return value, nil
			}
			wantValue = false
		}
	}
	return "", fmt.Errorf("%w: CFBundleShortVersionString is missing", ErrIncompatibleBuild)
}

func (client *Client) initialize(ctx context.Context, clientType string) error {
	if client.buildErr != nil {
		return client.buildErr
	}
	if clientType == "" {
		return errors.New("desktop IPC client type is required")
	}
	params, err := json.Marshal(map[string]string{"clientType": clientType})
	if err != nil {
		return err
	}
	result, err := client.syncRequest(ctx, wireMessage{Type: "request", Method: "initialize", Params: params})
	if err != nil {
		return fmt.Errorf("initialize desktop IPC: %w", err)
	}
	var initialized struct {
		ClientID string `json:"clientId"`
	}
	if err := json.Unmarshal(result, &initialized); err != nil || initialized.ClientID == "" {
		return fmt.Errorf("initialize desktop IPC: %w", ErrInvalidFrame)
	}
	client.stateMu.Lock()
	client.clientID = initialized.ClientID
	client.stateMu.Unlock()
	client.logger.Info("[desktop-ipc] initialized", "client_type", clientType)
	client.startReader()
	return nil
}

func (client *Client) ClientID() string {
	client.stateMu.RLock()
	defer client.stateMu.RUnlock()
	return client.clientID
}

func (client *Client) Stream(conversationID string) *StreamState {
	client.stateMu.Lock()
	defer client.stateMu.Unlock()
	stream := client.streams[conversationID]
	if stream == nil {
		stream = newStreamState(conversationID)
		client.streams[conversationID] = stream
	}
	return stream
}

func (client *Client) LoadCompleteHistory(ctx context.Context, conversationID string) (uint64, error) {
	if conversationID == "" {
		return 0, errors.New("desktop task ID is required")
	}
	stream := client.Stream(conversationID)
	previousSnapshotGeneration := stream.State().SnapshotGeneration
	params, err := json.Marshal(map[string]string{"conversationId": conversationID})
	if err != nil {
		return 0, err
	}
	result, err := client.request(ctx, wireMessage{
		Type: "request", Method: "thread-follower-load-complete-history",
		Version: supportedVersions["thread-follower-load-complete-history"], Params: params,
	}, false)
	if err != nil {
		return 0, fmt.Errorf("load desktop task history: %w", err)
	}
	var loaded struct {
		Revision uint64 `json:"revision"`
	}
	if err := json.Unmarshal(result, &loaded); err != nil || loaded.Revision == 0 {
		return 0, fmt.Errorf("load desktop task history: %w", ErrInvalidFrame)
	}
	if err := client.waitForFreshSnapshot(ctx, stream, loaded.Revision, previousSnapshotGeneration); err != nil {
		return 0, fmt.Errorf("wait for desktop task snapshot: %w", err)
	}
	return loaded.Revision, nil
}

func (client *Client) WaitForRevision(ctx context.Context, conversationID string, revision uint64) error {
	stream := client.Stream(conversationID)
	for !stream.HasSnapshot() || stream.Revision() < revision {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stream.Updates():
		case <-client.done:
			client.stateMu.RLock()
			err := client.terminalErr
			client.stateMu.RUnlock()
			if err == nil {
				err = ErrDisconnected
			}
			return err
		}
	}
	return nil
}

func (client *Client) waitForFreshSnapshot(ctx context.Context, stream *StreamState, revision, previousGeneration uint64) error {
	for {
		state := stream.State()
		if state.SnapshotGeneration > previousGeneration && state.Revision >= revision {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrSnapshotRequired, ctx.Err())
		case <-stream.Updates():
		case <-client.done:
			return client.terminalError()
		}
	}
}

func (client *Client) RouteApprovalDecision(ctx context.Context, conversationID, requestID, decision string) error {
	_, err := client.ExecuteFollowerAction(ctx, FollowerAction{
		Kind: ActionCommandApproval, ConversationID: conversationID,
		RequestID: requestID, Decision: decision,
	})
	if err != nil {
		return fmt.Errorf("route desktop approval: %w", err)
	}
	return nil
}

func (client *Client) ExecuteFollowerAction(ctx context.Context, action FollowerAction) (json.RawMessage, error) {
	message, err := buildFollowerAction(action)
	if err != nil {
		return nil, err
	}
	result, err := client.request(ctx, message, true)
	if err != nil {
		return nil, err
	}
	if err := validateFollowerActionResult(action.Kind, result); err != nil {
		client.fail(err)
		return nil, client.actionDeliveryError(message.Method, 1, err)
	}
	return result, nil
}

func validateFollowerActionResult(kind ActionKind, result json.RawMessage) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(result, &object); err != nil {
		return fmt.Errorf("%w: changed action response", ErrInvalidFrame)
	}
	switch kind {
	case ActionStartTurn:
		value, ok := object["result"]
		if len(object) != 1 || !ok || !validStartTurnResult(value) {
			return fmt.Errorf("%w: changed start result", ErrInvalidFrame)
		}
	case ActionSteerTurn:
		value, ok := object["result"]
		if len(object) != 1 || !ok || !validSteerTurnResult(value) {
			return fmt.Errorf("%w: changed steer result", ErrInvalidFrame)
		}
	case ActionInterruptTurn:
		value, ok := object["ok"]
		turnID, hasTurnID := object["interruptedTurnId"]
		var parsedTurnID string
		if len(object) != 2 || !ok || string(value) != "true" || !hasTurnID ||
			json.Unmarshal(turnID, &parsedTurnID) != nil || parsedTurnID == "" {
			return fmt.Errorf("%w: interrupt was not confirmed", ErrInvalidFrame)
		}
	case ActionCommandApproval:
		value, ok := object["ok"]
		if len(object) != 1 || !ok || string(value) != "true" {
			return fmt.Errorf("%w: action was not confirmed", ErrInvalidFrame)
		}
	default:
		return fmt.Errorf("%w: unknown action response", ErrInvalidFrame)
	}
	return nil
}

func validStartTurnResult(value json.RawMessage) bool {
	var result map[string]json.RawMessage
	if json.Unmarshal(value, &result) != nil || len(result) != 1 {
		return false
	}
	var turn map[string]json.RawMessage
	if json.Unmarshal(result["turn"], &turn) != nil {
		return false
	}
	allowed := map[string]bool{
		"completedAt": true, "durationMs": true, "error": true, "id": true,
		"items": true, "itemsView": true, "startedAt": true, "status": true,
	}
	for key := range turn {
		if !allowed[key] {
			return false
		}
	}
	var id, status string
	var items []json.RawMessage
	if json.Unmarshal(turn["id"], &id) != nil || id == "" ||
		json.Unmarshal(turn["status"], &status) != nil || !validTurnStatus(status) ||
		json.Unmarshal(turn["items"], &items) != nil {
		return false
	}
	return true
}

func validSteerTurnResult(value json.RawMessage) bool {
	var result map[string]json.RawMessage
	if json.Unmarshal(value, &result) != nil || len(result) != 1 {
		return false
	}
	var turnID string
	return json.Unmarshal(result["turnId"], &turnID) == nil && turnID != ""
}

func validTurnStatus(status string) bool {
	switch status {
	case "completed", "interrupted", "failed", "inProgress":
		return true
	default:
		return false
	}
}

func (client *Client) request(ctx context.Context, message wireMessage, sideEffect bool) (json.RawMessage, error) {
	client.requestMu.Lock()
	defer client.requestMu.Unlock()
	if client.ClientID() == "" {
		return nil, errors.New("desktop IPC is not initialized")
	}
	requestID, err := randomRequestID()
	if err != nil {
		return nil, err
	}
	message.RequestID = requestID
	message.SourceClientID = client.ClientID()
	message.TimeoutMS = 10_000
	response := make(chan wireMessage, 1)
	client.pendingMu.Lock()
	client.pending[requestID] = response
	client.pendingMu.Unlock()
	defer func() {
		client.pendingMu.Lock()
		delete(client.pending, requestID)
		client.pendingMu.Unlock()
	}()
	client.logger.Info("[desktop-ipc] request",
		"method", message.Method,
		"version", message.Version,
		"input_shape", "typed params",
	)
	tracker := &writeTracker{writer: client.connection}
	client.writeMu.Lock()
	err = writeFrame(tracker, message)
	client.writeMu.Unlock()
	if err != nil {
		if sideEffect {
			deliveryErr := client.actionDeliveryError(message.Method, tracker.written, err)
			client.fail(err)
			return nil, deliveryErr
		}
		return nil, fmt.Errorf("%w: %v", ErrDisconnected, err)
	}
	select {
	case <-ctx.Done():
		if sideEffect {
			deliveryErr := client.actionDeliveryError(message.Method, tracker.written, ctx.Err())
			client.fail(ctx.Err())
			return nil, deliveryErr
		}
		return nil, ctx.Err()
	case <-client.done:
		if sideEffect {
			return nil, client.actionDeliveryError(message.Method, tracker.written, client.terminalError())
		}
		return nil, client.terminalError()
	case incoming := <-response:
		result, responseErr := client.responseResult(message, incoming)
		if responseErr != nil && sideEffect && !errors.Is(responseErr, ErrOwnerUnavailable) {
			deliveryErr := client.actionDeliveryError(message.Method, tracker.written, responseErr)
			client.fail(responseErr)
			return nil, deliveryErr
		}
		return result, responseErr
	}
}

type writeTracker struct {
	writer  io.Writer
	written int
}

func (tracker *writeTracker) Write(value []byte) (int, error) {
	written, err := tracker.writer.Write(value)
	tracker.written += written
	return written, err
}

func (client *Client) actionDeliveryError(method string, bytesWritten int, cause error) error {
	outcome := WriteNotSent
	if bytesWritten > 0 {
		outcome = WriteOutcomeUnknown
	}
	client.logger.Warn("[desktop-ipc] action delivery failed",
		"method", method,
		"write_outcome", outcome,
		"error", cause,
	)
	return &ActionRequestError{Outcome: outcome, Cause: cause}
}

func (client *Client) syncRequest(ctx context.Context, message wireMessage) (json.RawMessage, error) {
	requestID, err := randomRequestID()
	if err != nil {
		return nil, err
	}
	message.RequestID = requestID
	if err := client.setDeadline(ctx); err != nil {
		return nil, err
	}
	defer client.clearDeadline()
	client.writeMu.Lock()
	err = writeFrame(client.connection, message)
	client.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	for {
		incoming, readErr := readFrame(client.connection)
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, readErr
		}
		if incoming.Type == "broadcast" {
			if err := client.handleInbound(incoming); err != nil {
				return nil, err
			}
			continue
		}
		if incoming.Type != "response" || incoming.RequestID != requestID {
			return nil, fmt.Errorf("%w: mismatched response", ErrInvalidFrame)
		}
		return client.responseResult(message, incoming)
	}
}

func (client *Client) startReader() {
	client.readerOnce.Do(func() { go client.readLoop() })
}

func (client *Client) readLoop() {
	for {
		message, err := readFrame(client.connection)
		if err != nil {
			client.fail(fmt.Errorf("%w: %v", ErrDisconnected, err))
			return
		}
		switch message.Type {
		case "broadcast":
			if err := client.handleInbound(message); err != nil {
				client.fail(err)
				return
			}
		case "client-discovery-request":
			if err := client.rejectDiscovery(message); err != nil {
				client.fail(err)
				return
			}
		case "response":
			client.pendingMu.Lock()
			response := client.pending[message.RequestID]
			client.pendingMu.Unlock()
			if response == nil {
				client.fail(fmt.Errorf("%w: response has no pending request", ErrInvalidFrame))
				return
			}
			response <- message
		default:
			client.fail(fmt.Errorf("%w: unknown message type %q", ErrInvalidFrame, message.Type))
			return
		}
	}

}

func (client *Client) responseResult(request wireMessage, response wireMessage) (json.RawMessage, error) {
	if response.ResultType == "error" {
		if response.Error == "no-client-found" || response.Error == "client-cannot-handle-request" {
			client.logger.Warn("[desktop-ipc] response failed",
				"method", request.Method,
				"branch_reason", "owner_unavailable",
			)
			return nil, ErrOwnerUnavailable
		}
		client.logger.Warn("[desktop-ipc] response failed",
			"method", request.Method,
			"branch_reason", "remote_error",
		)
		return nil, fmt.Errorf("%w: %s", ErrRemote, response.Error)
	}
	if response.ResultType != "success" || response.Method != request.Method {
		client.logger.Error("[desktop-ipc] invalid response",
			"method", request.Method,
			"response_method", response.Method,
			"result_type", response.ResultType,
		)
		return nil, fmt.Errorf("%w: response method or result mismatch", ErrInvalidFrame)
	}
	client.logger.Info("[desktop-ipc] response", "method", request.Method, "output_shape", "success")
	return append(json.RawMessage(nil), response.Result...), nil

}

func (client *Client) fail(err error) {
	client.doneOnce.Do(func() {
		client.stateMu.Lock()
		client.terminalErr = err
		client.stateMu.Unlock()
		client.logger.Error("[desktop-ipc] connection stopped", "error", err)
		_ = client.connection.Close()
		close(client.done)
	})
}

func (client *Client) terminalError() error {
	client.stateMu.RLock()
	defer client.stateMu.RUnlock()
	if client.terminalErr == nil {
		return ErrDisconnected
	}
	return client.terminalErr
}

func (client *Client) handleInbound(message wireMessage) error {
	if message.Type != "broadcast" {
		return fmt.Errorf("%w: inbound message is not a broadcast", ErrInvalidFrame)
	}
	if message.Method != "thread-stream-state-changed" {
		if err := validateIgnoredBroadcast(message.Method, message.Version); err != nil {
			client.logger.Error("[desktop-ipc] incompatible broadcast",
				"method", message.Method,
				"version", message.Version,
			)
			return err
		}
		client.logger.Debug("[desktop-ipc] ignored compatible broadcast",
			"method", message.Method,
			"version", message.Version,
			"branch_reason", "mobile_companion_does_not_use_event",
		)
		return nil
	}
	event, err := parseStreamEvent(message)
	if err != nil {
		return err
	}
	stream, tracked := client.trackedStream(event.ConversationID)
	if !tracked {
		client.logger.Debug("[desktop-ipc] ignored untracked task event",
			"thread_id", event.ConversationID,
			"revision", event.Revision,
			"branch_reason", "task_not_requested",
		)
		return nil
	}
	if err := stream.apply(event); err != nil {
		return fmt.Errorf("apply desktop stream event: %w", err)
	}
	logStreamEvent(client.logger, event)
	return nil
}

func (client *Client) trackedStream(conversationID string) (*StreamState, bool) {
	client.stateMu.RLock()
	defer client.stateMu.RUnlock()
	stream, ok := client.streams[conversationID]
	return stream, ok
}

func (client *Client) rejectDiscovery(message wireMessage) error {
	response, err := json.Marshal(map[string]bool{"canHandle": false})
	if err != nil {
		return err
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return writeFrame(client.connection, wireMessage{
		Type: "client-discovery-response", RequestID: message.RequestID, Response: response,
	})
}

func (client *Client) setDeadline(ctx context.Context) error {
	connection, ok := client.connection.(deadlineConn)
	if !ok {
		return nil
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}
	return connection.SetDeadline(deadline)
}

func (client *Client) clearDeadline() {
	if connection, ok := client.connection.(deadlineConn); ok {
		_ = connection.SetDeadline(time.Time{})
	}
}

func randomRequestID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create desktop IPC request ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}
