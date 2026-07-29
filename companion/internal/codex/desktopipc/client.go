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
	"sync/atomic"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
)

var (
	ErrDisconnected           = errors.New("desktop IPC disconnected")
	ErrIncompatibleBuild      = errors.New("ChatGPT Desktop build is incompatible")
	ErrOwnerUnavailable       = errors.New("desktop task owner is unavailable")
	ErrPlatformUnsupported    = errors.New("desktop IPC platform is unsupported")
	ErrRemote                 = errors.New("desktop IPC request failed")
	ErrPendingRequestMismatch = errors.New("desktop pending request does not match the action")
	ErrPermissionEscalation   = errors.New("desktop permission response exceeds the request")
	ErrUnsafeEndpoint         = errors.New("desktop IPC endpoint is unsafe")
	ErrWriteNotSent           = errors.New("desktop action was not sent")
	ErrWriteOutcomeUnknown    = errors.New("desktop action outcome is unknown")
	errStoppedBeforeWrite     = errors.New("desktop IPC stopped before write")
)

const PinnedDesktopBuild = "26.707.72221"

const (
	darwinDesktopExecutable  = "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"
	darwinDesktopInfoPlist   = "/Applications/ChatGPT.app/Contents/Info.plist"
	desktopDecisionQueueSize = 64
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

	requestMu        sync.Mutex
	writeMu          sync.Mutex
	stateMu          sync.RWMutex
	pendingMu        sync.Mutex
	readerOnce       sync.Once
	doneOnce         sync.Once
	clientID         string
	streams          map[string]*StreamState
	pending          map[string]chan wireMessage
	done             chan struct{}
	terminalErr      error
	closeErr         error
	buildErr         error
	pendingActions   map[string]map[string]desktopPendingAction
	consumedActions  map[string]map[string]ActionKind
	mobileEventMu    sync.Mutex
	mobileEventOnce  sync.Once
	mobileVerified   map[string]bool
	mobileAuth       map[string]*taskstate.EventAuthorization
	mobilePending    map[string]taskstate.MobileEvent
	mobileOrder      []string
	mobileSignal     chan struct{}
	mobileEvents     chan taskstate.MobileEvent
	decisionRequests chan appserver.ServerRequest
}

type desktopPendingAction struct {
	kind        ActionKind
	permissions json.RawMessage
	questionIDs map[string]bool
	decisions   map[string]bool
	inFlight    bool
}

var _ taskstate.TaskAdapter = (*Client)(nil)

func (*Client) TaskSource() taskstate.Source { return taskstate.SourceDesktop }

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

func (connector *SessionConnector) Done() <-chan struct{} {
	if connector == nil {
		return closedSignal()
	}
	connector.mu.Lock()
	defer connector.mu.Unlock()
	if connector.current == nil {
		return closedSignal()
	}
	return connector.current.done
}

func (connector *SessionConnector) Close() error {
	if connector == nil {
		return nil
	}
	connector.mu.Lock()
	defer connector.mu.Unlock()
	if connector.current == nil {
		return nil
	}
	err := connector.current.Close()
	connector.current = nil
	connector.logger.Info("[desktop-ipc] connector stopped", "decision", "owner_closed")
	return err
}

func closedSignal() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func newClient(connection io.ReadWriteCloser, desktopBuild string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	client := &Client{
		connection:       connection,
		logger:           logger,
		streams:          make(map[string]*StreamState),
		pending:          make(map[string]chan wireMessage),
		pendingActions:   make(map[string]map[string]desktopPendingAction),
		consumedActions:  make(map[string]map[string]ActionKind),
		mobileVerified:   make(map[string]bool),
		mobileAuth:       make(map[string]*taskstate.EventAuthorization),
		mobilePending:    make(map[string]taskstate.MobileEvent),
		mobileSignal:     make(chan struct{}, 1),
		mobileEvents:     make(chan taskstate.MobileEvent),
		decisionRequests: make(chan appserver.ServerRequest, desktopDecisionQueueSize),
		done:             make(chan struct{}),
		buildErr:         verifyDesktopBuild(desktopBuild),
	}
	return client
}

func (client *Client) DecisionRequests() <-chan appserver.ServerRequest {
	if client == nil || client.decisionRequests == nil {
		closed := make(chan appserver.ServerRequest)
		close(closed)
		return closed
	}
	return client.decisionRequests
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
	client.markMobileStreamUnverified(conversationID)
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

func (client *Client) AuthorizeMobileEvents(conversationID string) error {
	if conversationID == "" {
		return errors.New("desktop task ID is required")
	}
	stream := client.Stream(conversationID)
	task, err := taskstate.MapDesktopConversationState(stream.State().Materialized)
	if err != nil || task.ID != conversationID {
		client.markMobileStreamUnverified(conversationID)
		return ErrInvalidFrame
	}
	client.markMobileStreamVerified(conversationID, stream)
	return nil
}

func (client *Client) RevokeMobileEvents(conversationID string) {
	client.markMobileStreamUnverified(conversationID)
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
	err := client.executeConfirmedAction(ctx, FollowerAction{
		Kind: ActionCommandApproval, ConversationID: conversationID,
		RequestID: requestID, Decision: decision,
	})
	if err != nil {
		return fmt.Errorf("route desktop approval: %w", err)
	}
	return nil
}

func (client *Client) StartTurn(ctx context.Context, conversationID, text string) (json.RawMessage, error) {
	return client.StartTurnWithAttachments(ctx, conversationID, text, nil)
}

func (client *Client) StartTurnWithAttachments(ctx context.Context, conversationID, text string, attachments []AttachmentInput) (json.RawMessage, error) {
	return client.executeFollowerAction(ctx, FollowerAction{Kind: ActionStartTurn, ConversationID: conversationID, Text: text, Attachments: attachments})
}

func (client *Client) StartTurnWithSettings(ctx context.Context, conversationID, text string, settings ThreadSettings) (json.RawMessage, error) {
	if err := client.UpdateSettings(ctx, conversationID, settings); err != nil {
		return nil, fmt.Errorf("update desktop settings before turn: %w", err)
	}
	return client.StartTurn(ctx, conversationID, text)
}

func (client *Client) SteerTurn(ctx context.Context, conversationID, text string) (json.RawMessage, error) {
	return client.SteerTurnWithAttachments(ctx, conversationID, text, nil)
}

func (client *Client) SteerTurnWithAttachments(ctx context.Context, conversationID, text string, attachments []AttachmentInput) (json.RawMessage, error) {
	return client.executeFollowerAction(ctx, FollowerAction{Kind: ActionSteerTurn, ConversationID: conversationID, Text: text, Attachments: attachments})
}

func (client *Client) InterruptTurn(ctx context.Context, conversationID string) (json.RawMessage, error) {
	return client.executeFollowerAction(ctx, FollowerAction{Kind: ActionInterruptTurn, ConversationID: conversationID})
}

func (client *Client) Compact(ctx context.Context, conversationID string) error {
	return client.executeConfirmedAction(ctx, FollowerAction{Kind: ActionCompact, ConversationID: conversationID})
}

func (client *Client) UpdateSettings(ctx context.Context, conversationID string, settings ThreadSettings) error {
	return client.executeConfirmedAction(ctx, FollowerAction{Kind: ActionUpdateSettings, ConversationID: conversationID, Settings: &settings})
}

func (client *Client) RouteFileApprovalDecision(ctx context.Context, conversationID, requestID, decision string) error {
	return client.executeConfirmedAction(ctx, FollowerAction{Kind: ActionFileApproval, ConversationID: conversationID, RequestID: requestID, Decision: decision})
}

func (client *Client) RespondPermissionRequest(ctx context.Context, conversationID, requestID string, response PermissionResponse) error {
	return client.executeConfirmedAction(ctx, FollowerAction{Kind: ActionPermissionApproval, ConversationID: conversationID, RequestID: requestID, PermissionResponse: &response})
}

func (client *Client) SubmitUserInput(ctx context.Context, conversationID, requestID string, response UserInputResponse) error {
	return client.executeConfirmedAction(ctx, FollowerAction{Kind: ActionSubmitUserInput, ConversationID: conversationID, RequestID: requestID, UserInputResponse: &response})
}

func (client *Client) SubmitMCP(ctx context.Context, conversationID, requestID string, response MCPResponse) error {
	return client.executeConfirmedAction(ctx, FollowerAction{Kind: ActionSubmitMCP, ConversationID: conversationID, RequestID: requestID, MCPResponse: &response})
}

func (client *Client) EditLastTurn(ctx context.Context, conversationID, turnID, text, agentMode string, shouldSendPermissionOverrides bool, serviceTier string) error {
	return client.executeConfirmedAction(ctx, FollowerAction{
		Kind: ActionEditLastTurn, ConversationID: conversationID, TurnID: turnID, Text: text, AgentMode: agentMode,
		ShouldSendPermissionOverrides: shouldSendPermissionOverrides, ServiceTier: serviceTier,
	})
}

func (client *Client) executeConfirmedAction(ctx context.Context, action FollowerAction) error {
	if _, err := buildFollowerAction(action); err != nil {
		return err
	}
	if requiresPendingDesktopAction(action.Kind) {
		if client.ClientID() == "" {
			return &ActionRequestError{Outcome: WriteNotSent, Cause: ErrDisconnected}
		}
		if err := client.authorizePendingAction(action); err != nil {
			return err
		}
	}
	if _, err := client.executeFollowerAction(ctx, action); err != nil {
		if requiresPendingDesktopAction(action.Kind) {
			client.finishPendingAction(action, !errors.Is(err, ErrWriteNotSent))
		}
		return fmt.Errorf("route desktop %s: %w", action.Kind, err)
	}
	if requiresPendingDesktopAction(action.Kind) {
		client.finishPendingAction(action, true)
	}
	return nil
}

func (client *Client) executeFollowerAction(ctx context.Context, action FollowerAction) (json.RawMessage, error) {
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
	case ActionCommandApproval, ActionCompact, ActionUpdateSettings, ActionFileApproval, ActionPermissionApproval, ActionSubmitUserInput, ActionSubmitMCP, ActionEditLastTurn:
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
	select {
	case <-client.done:
		if sideEffect {
			return nil, client.actionDeliveryError(message.Method, 0, client.terminalError())
		}
		return nil, client.terminalError()
	default:
	}
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
	writeResult := make(chan error, 1)
	go func() {
		client.writeMu.Lock()
		select {
		case <-client.done:
			client.writeMu.Unlock()
			writeResult <- errStoppedBeforeWrite
			return
		default:
		}
		tracker.started.Store(true)
		writeErr := writeFrame(tracker, message)
		client.writeMu.Unlock()
		writeResult <- writeErr
	}()
	select {
	case err = <-writeResult:
	case <-ctx.Done():
		if sideEffect {
			deliveryErr := client.actionDeliveryError(message.Method, 1, ctx.Err())
			client.fail(ctx.Err())
			return nil, deliveryErr
		}
		client.fail(ctx.Err())
		return nil, ctx.Err()
	case <-client.done:
		if sideEffect {
			bytesWritten := 1
			if !tracker.started.Load() {
				bytesWritten = 0
			}
			return nil, client.actionDeliveryError(message.Method, bytesWritten, client.terminalError())
		}
		return nil, client.terminalError()
	}
	if errors.Is(err, errStoppedBeforeWrite) {
		if sideEffect {
			return nil, client.actionDeliveryError(message.Method, 0, client.terminalError())
		}
		return nil, client.terminalError()
	}
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
	started atomic.Bool
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
	_ = client.stop(err, false)
}

func (client *Client) Close() error {
	if client == nil {
		return nil
	}
	return client.stop(ErrDisconnected, true)
}

func (client *Client) stop(err error, planned bool) error {
	client.doneOnce.Do(func() {
		client.stateMu.Lock()
		client.terminalErr = err
		client.stateMu.Unlock()
		if planned {
			client.logger.Info("[desktop-ipc] connection stopped", "decision", "owner_closed")
		} else {
			client.logger.Error("[desktop-ipc] connection stopped", "error", err)
		}
		closeErr := client.connection.Close()
		client.stateMu.Lock()
		client.closeErr = closeErr
		client.stateMu.Unlock()
		close(client.done)
	})
	client.stateMu.RLock()
	defer client.stateMu.RUnlock()
	return client.closeErr
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
	retained := stream.State()
	if err := client.registerPendingState(event.ConversationID, retained.Materialized); err != nil {
		return err
	}
	client.queueMobileState(event.ConversationID, retained.Materialized)
	stream.signalUpdate()
	logStreamEvent(client.logger, event)
	return nil
}

func requiresPendingDesktopAction(kind ActionKind) bool {
	switch kind {
	case ActionCommandApproval, ActionFileApproval, ActionPermissionApproval, ActionSubmitUserInput, ActionSubmitMCP:
		return true
	default:
		return false
	}
}

func (client *Client) registerPendingRequests(conversationID string, raw json.RawMessage) error {
	var change struct {
		Type              string          `json:"type"`
		ConversationState json.RawMessage `json:"conversationState"`
	}
	if json.Unmarshal(raw, &change) != nil || change.Type != "snapshot" || len(change.ConversationState) == 0 {
		return ErrInvalidFrame
	}
	return client.registerPendingState(conversationID, change.ConversationState)
}

func (client *Client) registerPendingState(conversationID string, raw json.RawMessage) error {
	var state struct {
		Requests []struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		} `json:"requests"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return ErrInvalidFrame
	}
	registered := make(map[string]desktopPendingAction)
	projected := make(map[string]appserver.ServerRequest)
	seenKinds := make(map[string]ActionKind)
	for _, request := range state.Requests {
		requestID, ok := desktopRequestID(request.ID)
		if !ok {
			continue
		}
		kind := desktopActionKind(request.Method)
		if kind == "" {
			continue
		}
		seenKinds[requestID] = kind
		var params struct {
			ThreadID           string          `json:"threadId"`
			TurnID             string          `json:"turnId"`
			ItemID             string          `json:"itemId"`
			StartedAtMs        int64           `json:"startedAtMs"`
			Command            string          `json:"command"`
			CWD                string          `json:"cwd"`
			Reason             string          `json:"reason"`
			GrantRoot          string          `json:"grantRoot"`
			ServerName         string          `json:"serverName"`
			Mode               string          `json:"mode"`
			Message            string          `json:"message"`
			Permissions        json.RawMessage `json:"permissions"`
			AvailableDecisions []string        `json:"availableDecisions"`
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
		if json.Unmarshal(request.Params, &params) != nil || params.ThreadID != conversationID {
			return ErrInvalidFrame
		}
		pending := desktopPendingAction{kind: kind, permissions: append(json.RawMessage(nil), params.Permissions...), questionIDs: map[string]bool{}, decisions: map[string]bool{}}
		projectedRequest := appserver.ServerRequest{ID: append(json.RawMessage(nil), request.ID...), Method: request.Method, Params: append(json.RawMessage(nil), request.Params...), ThreadID: params.ThreadID, TurnID: params.TurnID, ItemID: params.ItemID, StartedAtMs: params.StartedAtMs, Command: params.Command, CWD: params.CWD, Reason: params.Reason, GrantRoot: params.GrantRoot, ServerName: params.ServerName, MCPMode: params.Mode, MCPMessage: params.Message, Permissions: append(json.RawMessage(nil), params.Permissions...)}
		for _, decision := range params.AvailableDecisions {
			if !allowedApprovalDecision(decision) {
				return ErrInvalidFrame
			}
			pending.decisions[decision] = true
			projectedRequest.AllowedDecisions = append(projectedRequest.AllowedDecisions, appserver.ApprovalDecision(decision))
		}
		if (kind == ActionCommandApproval || kind == ActionFileApproval) && len(pending.decisions) == 0 {
			for _, decision := range []string{"accept", "acceptForSession", "decline", "cancel"} {
				pending.decisions[decision] = true
				projectedRequest.AllowedDecisions = append(projectedRequest.AllowedDecisions, appserver.ApprovalDecision(decision))
			}
		}
		for _, question := range params.Questions {
			if !validDesktopID(question.ID) || pending.questionIDs[question.ID] {
				return ErrInvalidFrame
			}
			pending.questionIDs[question.ID] = true
			options := make([]string, 0, len(question.Options))
			for _, option := range question.Options {
				options = append(options, option.Label)
			}
			projectedRequest.Questions = append(projectedRequest.Questions, appserver.ServerQuestion{ID: question.ID, Header: question.Header, Prompt: question.Question, Options: options, Secret: question.IsSecret})
		}
		registered[requestID] = pending
		projected[requestID] = projectedRequest
	}
	client.stateMu.Lock()
	old := client.pendingActions[conversationID]
	tombstones := client.consumedActions[conversationID]
	if tombstones == nil {
		tombstones = make(map[string]ActionKind)
		client.consumedActions[conversationID] = tombstones
	}
	for requestID, pending := range registered {
		if tombstones[requestID] == pending.kind {
			delete(registered, requestID)
			continue
		}
		if prior, exists := old[requestID]; exists && prior.kind == pending.kind && prior.inFlight {
			pending.inFlight = true
			registered[requestID] = pending
		}
	}
	for requestID, kind := range tombstones {
		if seenKinds[requestID] != kind {
			delete(tombstones, requestID)
		}
	}
	newRequests := make([]appserver.ServerRequest, 0)
	for requestID, pending := range registered {
		if prior, exists := old[requestID]; !exists || prior.kind != pending.kind {
			newRequests = append(newRequests, projected[requestID])
		}
	}
	client.pendingActions[conversationID] = registered
	client.stateMu.Unlock()
	for _, request := range newRequests {
		select {
		case client.decisionRequests <- request:
			client.logger.Info("[desktop-ipc] decision request", "thread_id", request.ThreadID, "request_method", request.Method, "decision", "publish_safe_typed_fields")
		default:
			return fmt.Errorf("%w: desktop decision queue full", ErrInvalidFrame)
		}
	}
	return nil
}

func (client *Client) authorizePendingAction(action FollowerAction) error {
	client.stateMu.Lock()
	defer client.stateMu.Unlock()
	requests := client.pendingActions[action.ConversationID]
	pending, ok := requests[action.RequestID]
	if !ok || pending.kind != action.Kind || pending.inFlight {
		return ErrPendingRequestMismatch
	}
	switch action.Kind {
	case ActionCommandApproval, ActionFileApproval:
		if !pending.decisions[action.Decision] {
			return ErrPendingRequestMismatch
		}
	case ActionPermissionApproval:
		if action.PermissionResponse == nil || !taskstate.PermissionSubset(action.PermissionResponse.Permissions, pending.permissions) {
			return ErrPermissionEscalation
		}
	case ActionSubmitUserInput:
		if action.UserInputResponse == nil || len(action.UserInputResponse.Answers) != len(pending.questionIDs) {
			return ErrPendingRequestMismatch
		}
		for questionID := range action.UserInputResponse.Answers {
			if !pending.questionIDs[questionID] {
				return ErrPendingRequestMismatch
			}
		}
	}
	pending.inFlight = true
	requests[action.RequestID] = pending
	return nil
}

func (client *Client) finishPendingAction(action FollowerAction, consume bool) {
	client.stateMu.Lock()
	defer client.stateMu.Unlock()
	requests := client.pendingActions[action.ConversationID]
	pending, ok := requests[action.RequestID]
	if !ok {
		return
	}
	if consume {
		delete(requests, action.RequestID)
		tombstones := client.consumedActions[action.ConversationID]
		if tombstones == nil {
			tombstones = make(map[string]ActionKind)
			client.consumedActions[action.ConversationID] = tombstones
		}
		tombstones[action.RequestID] = action.Kind
		return
	}
	pending.inFlight = false
	requests[action.RequestID] = pending
}

func desktopRequestID(raw json.RawMessage) (string, bool) {
	var value string
	if json.Unmarshal(raw, &value) == nil && validDesktopID(value) {
		return value, true
	}
	return "", false
}

func desktopActionKind(method string) ActionKind {
	switch method {
	case "item/commandExecution/requestApproval":
		return ActionCommandApproval
	case "item/fileChange/requestApproval":
		return ActionFileApproval
	case "item/permissions/requestApproval":
		return ActionPermissionApproval
	case "item/tool/requestUserInput":
		return ActionSubmitUserInput
	case "mcpServer/elicitation/request":
		return ActionSubmitMCP
	default:
		return ""
	}
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
	select {
	case <-client.done:
		return client.terminalError()
	default:
	}
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
