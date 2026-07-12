package desktopipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrDisconnected        = errors.New("desktop IPC disconnected")
	ErrOwnerUnavailable    = errors.New("desktop task owner is unavailable")
	ErrPlatformUnsupported = errors.New("desktop IPC platform is unsupported")
	ErrRemote              = errors.New("desktop IPC request failed")
	ErrUnsafeEndpoint      = errors.New("desktop IPC endpoint is unsafe")
)

type deadlineConn interface {
	SetDeadline(time.Time) error
}

func DiscoverEndpoint(goos, tempDir string, uid int) (network string, address string, err error) {
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

func VerifyUnixEndpoint(privateTempRoot, socketPath string) error {
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
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: temp root is not owner-only", ErrUnsafeEndpoint)
	}
	endpointInfo, err := os.Lstat(endpoint)
	if err != nil || endpointInfo.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: endpoint is not a Unix socket", ErrUnsafeEndpoint)
	}
	return nil
}

type Client struct {
	connection io.ReadWriteCloser
	logger     *slog.Logger

	requestMu sync.Mutex
	stateMu   sync.RWMutex
	clientID  string
	streams   map[string]*StreamState
}

func NewClient(connection io.ReadWriteCloser, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{connection: connection, logger: logger, streams: make(map[string]*StreamState)}
}

func (client *Client) Initialize(ctx context.Context, clientType string) error {
	if clientType == "" {
		return errors.New("desktop IPC client type is required")
	}
	params, err := json.Marshal(map[string]string{"clientType": clientType})
	if err != nil {
		return err
	}
	result, err := client.request(ctx, WireMessage{Type: "request", Method: "initialize", Params: params})
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
		stream = NewStreamState(conversationID)
		client.streams[conversationID] = stream
	}
	return stream
}

func (client *Client) LoadCompleteHistory(ctx context.Context, conversationID string) (uint64, error) {
	if conversationID == "" {
		return 0, errors.New("desktop task ID is required")
	}
	stream := client.Stream(conversationID)
	params, err := json.Marshal(map[string]string{"conversationId": conversationID})
	if err != nil {
		return 0, err
	}
	result, err := client.request(ctx, WireMessage{
		Type: "request", Method: "thread-follower-load-complete-history",
		Version: supportedVersions["thread-follower-load-complete-history"], Params: params,
	})
	if err != nil {
		return 0, fmt.Errorf("load desktop task history: %w", err)
	}
	var loaded struct {
		Revision uint64 `json:"revision"`
	}
	if err := json.Unmarshal(result, &loaded); err != nil || loaded.Revision == 0 {
		return 0, fmt.Errorf("load desktop task history: %w", ErrInvalidFrame)
	}
	for !stream.HasSnapshot() || stream.Revision() < loaded.Revision {
		message, readErr := client.read(ctx)
		if readErr != nil {
			return 0, fmt.Errorf("wait for desktop task snapshot: %w", readErr)
		}
		if err := client.handleInbound(message); err != nil {
			return 0, err
		}
	}
	return loaded.Revision, nil
}

func (client *Client) RouteApprovalDecision(ctx context.Context, conversationID, requestID, decision string) error {
	message, err := BuildFollowerAction(FollowerAction{
		Kind: ActionCommandApproval, ConversationID: conversationID,
		RequestID: requestID, Decision: decision,
	})
	if err != nil {
		return err
	}
	_, err = client.request(ctx, message)
	if err != nil {
		return fmt.Errorf("route desktop approval: %w", err)
	}
	return nil
}

func (client *Client) request(ctx context.Context, message WireMessage) (json.RawMessage, error) {
	client.requestMu.Lock()
	defer client.requestMu.Unlock()
	if message.Method != "initialize" && client.ClientID() == "" {
		return nil, errors.New("desktop IPC is not initialized")
	}
	requestID, err := randomRequestID()
	if err != nil {
		return nil, err
	}
	message.RequestID = requestID
	if message.Method != "initialize" {
		message.SourceClientID = client.ClientID()
		message.TimeoutMS = 10_000
	}
	if err := client.setDeadline(ctx); err != nil {
		return nil, err
	}
	defer client.clearDeadline()
	client.logger.Info("[desktop-ipc] request",
		"method", message.Method,
		"version", message.Version,
		"input_shape", "typed params",
	)
	if err := WriteFrame(client.connection, message); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDisconnected, err)
	}
	for {
		incoming, readErr := ReadFrame(client.connection)
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %v", ErrDisconnected, readErr)
		}
		if incoming.Type == "client-discovery-request" {
			if err := client.rejectDiscovery(incoming); err != nil {
				return nil, err
			}
			continue
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
		if incoming.ResultType == "error" {
			if incoming.Error == "no-client-found" || incoming.Error == "client-cannot-handle-request" {
				return nil, ErrOwnerUnavailable
			}
			return nil, fmt.Errorf("%w: %s", ErrRemote, incoming.Error)
		}
		if incoming.ResultType != "success" {
			return nil, fmt.Errorf("%w: missing success result", ErrInvalidFrame)
		}
		client.logger.Info("[desktop-ipc] response", "method", message.Method, "output_shape", "success")
		return append(json.RawMessage(nil), incoming.Result...), nil
	}
}

func (client *Client) read(ctx context.Context) (WireMessage, error) {
	if err := client.setDeadline(ctx); err != nil {
		return WireMessage{}, err
	}
	defer client.clearDeadline()
	message, err := ReadFrame(client.connection)
	if err != nil && ctx.Err() != nil {
		return WireMessage{}, ctx.Err()
	}
	return message, err
}

func (client *Client) handleInbound(message WireMessage) error {
	if message.Method != "thread-stream-state-changed" {
		return nil
	}
	event, err := ParseStreamEvent(message)
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
	if err := stream.Apply(event); err != nil {
		return fmt.Errorf("apply desktop stream event: %w", err)
	}
	LogStreamEvent(client.logger, event)
	return nil
}

func (client *Client) trackedStream(conversationID string) (*StreamState, bool) {
	client.stateMu.RLock()
	defer client.stateMu.RUnlock()
	stream, ok := client.streams[conversationID]
	return stream, ok
}

func (client *Client) rejectDiscovery(message WireMessage) error {
	response, err := json.Marshal(map[string]bool{"canHandle": false})
	if err != nil {
		return err
	}
	return WriteFrame(client.connection, WireMessage{
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
