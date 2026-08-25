// Package turnproxy speaks the OpenClaw Gateway's operator wire protocol:
// one websocket carrying JSON req/res/event frames (loopback-only, see
// saved-results/openclaw-gateway-protocol.md). client.go owns the raw
// connection — dialing, the connect handshake, and request/response
// correlation. It never logs the auth token or message bodies, only frame
// ids, methods, and event names.
package turnproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// protocolVersion is the OpenClaw Gateway wire protocol version this client
// speaks, both as the floor and ceiling it will negotiate.
const protocolVersion = 4

// gatewayFrame is the gateway's frame envelope: requests, responses, and
// events all share this shape (docs/gateway/protocol.md, frame shapes).
type gatewayFrame struct {
	Type    string             `json:"type"`
	ID      string             `json:"id,omitempty"`
	Method  string             `json:"method,omitempty"`
	Params  json.RawMessage    `json:"params,omitempty"`
	OK      bool               `json:"ok,omitempty"`
	Payload json.RawMessage    `json:"payload,omitempty"`
	Event   string             `json:"event,omitempty"`
	Error   *gatewayFrameError `json:"error,omitempty"`
}

type gatewayFrameError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type connectParams struct {
	MinProtocol int               `json:"minProtocol"`
	MaxProtocol int               `json:"maxProtocol"`
	Client      connectClientInfo `json:"client"`
	Role        string            `json:"role"`
	Scopes      []string          `json:"scopes"`
	Auth        connectAuth       `json:"auth"`
}

type connectClientInfo struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

type connectAuth struct {
	Token string `json:"token"`
}

// gatewayClient owns one websocket connection to the gateway. A single
// reader goroutine (readLoop) owns all reads: it routes "res" frames to the
// pending request that's waiting on their id and hands "event" frames to
// onEvent. Writes are serialized independently under writeMu.
type gatewayClient struct {
	conn    *websocket.Conn
	logger  *slog.Logger
	onEvent func(event string, payload json.RawMessage)

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan gatewayFrame

	challengeOnce sync.Once
	challengeCh   chan struct{}

	closeOnce sync.Once
	done      chan struct{}
}

// connectClient dials the gateway, performs the v4 operator handshake, and
// starts the reader goroutine. It fails closed: any error along the way
// closes the socket and returns before a *gatewayClient escapes, so callers
// never observe a half-open connection.
func connectClient(ctx context.Context, url, token string, logger *slog.Logger, onEvent func(event string, payload json.RawMessage)) (*gatewayClient, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("turnproxy: dial gateway: %w", err)
	}
	client := &gatewayClient{
		conn:        conn,
		logger:      logger,
		onEvent:     onEvent,
		pending:     make(map[string]chan gatewayFrame),
		challengeCh: make(chan struct{}),
		done:        make(chan struct{}),
	}
	go client.readLoop()

	if err := client.awaitChallenge(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("turnproxy: awaiting connect challenge: %w", err)
	}

	params := connectParams{
		MinProtocol: protocolVersion,
		MaxProtocol: protocolVersion,
		Client:      connectClientInfo{ID: "gateway-client", Version: "dev", Platform: "go", Mode: "backend"},
		Role:        "operator",
		Scopes:      []string{"operator.read", "operator.write"},
		Auth:        connectAuth{Token: token},
	}
	if _, err := client.request(ctx, "connect", params); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("turnproxy: connect handshake: %w", err)
	}
	logger.Info("[turnproxy] gateway connected")
	return client, nil
}

func (client *gatewayClient) awaitChallenge(ctx context.Context) error {
	select {
	case <-client.challengeCh:
		return nil
	case <-client.done:
		return fmt.Errorf("turnproxy: gateway closed before sending connect.challenge")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// readLoop is the connection's sole reader. It runs until the socket
// closes, at which point it closes done, unblocking every in-flight
// request and awaitChallenge call.
func (client *gatewayClient) readLoop() {
	defer close(client.done)
	for {
		_, data, err := client.conn.Read(context.Background())
		if err != nil {
			return
		}
		var frame gatewayFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			client.logger.Warn("[turnproxy] dropped unparsable frame")
			continue
		}
		switch frame.Type {
		case "res":
			client.deliver(frame)
		case "event":
			if frame.Event == "connect.challenge" {
				client.challengeOnce.Do(func() { close(client.challengeCh) })
			}
			if client.onEvent != nil {
				client.onEvent(frame.Event, frame.Payload)
			}
		}
	}
}

func (client *gatewayClient) deliver(frame gatewayFrame) {
	client.pendingMu.Lock()
	respCh, ok := client.pending[frame.ID]
	if ok {
		delete(client.pending, frame.ID)
	}
	client.pendingMu.Unlock()
	if ok {
		respCh <- frame
	}
}

// request sends a req frame and waits for its matching res, the connection
// closing, or ctx expiring — whichever comes first.
func (client *gatewayClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := newRequestID()
	respCh := make(chan gatewayFrame, 1)
	client.pendingMu.Lock()
	client.pending[id] = respCh
	client.pendingMu.Unlock()
	defer func() {
		client.pendingMu.Lock()
		delete(client.pending, id)
		client.pendingMu.Unlock()
	}()

	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("turnproxy: encode %s params: %w", method, err)
	}
	data, err := json.Marshal(gatewayFrame{Type: "req", ID: id, Method: method, Params: body})
	if err != nil {
		return nil, fmt.Errorf("turnproxy: encode %s frame: %w", method, err)
	}

	client.writeMu.Lock()
	writeErr := client.conn.Write(ctx, websocket.MessageText, data)
	client.writeMu.Unlock()
	if writeErr != nil {
		return nil, fmt.Errorf("turnproxy: send %s: %w", method, writeErr)
	}

	select {
	case frame := <-respCh:
		if !frame.OK {
			return nil, fmt.Errorf("turnproxy: %s refused: %s", method, errMessage(frame.Error))
		}
		return frame.Payload, nil
	case <-client.done:
		return nil, fmt.Errorf("turnproxy: gateway connection closed while awaiting %s", method)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the underlying connection and waits for readLoop to exit.
// Safe to call more than once.
func (client *gatewayClient) Close() error {
	var closeErr error
	client.closeOnce.Do(func() {
		closeErr = client.conn.Close(websocket.StatusNormalClosure, "turnproxy closing")
	})
	<-client.done
	return closeErr
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func errMessage(err *gatewayFrameError) string {
	if err == nil {
		return "unknown error"
	}
	return err.Message
}
