package turnproxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

// --- fake gateway ---

// wireFrame covers all three gateway frame shapes (req/res/event) so the
// fake can decode what the client sends and encode what it sends back.
type wireFrame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Payload any             `json:"payload,omitempty"`
	Event   string          `json:"event,omitempty"`
	Error   any             `json:"error,omitempty"`
}

type gatewayRequest struct {
	ID     string
	Method string
	Params json.RawMessage
}

// fakeGateway accepts one gateway connection, performs the v4 handshake
// (challenge event first, then the client's connect req), auto-acknowledges
// every later request with ok:true, and lets the test push chat events.
type fakeGateway struct {
	t          *testing.T
	server     *httptest.Server
	rejectAuth bool

	connectParams chan json.RawMessage
	requests      chan gatewayRequest

	mu               sync.Mutex
	conn             *websocket.Conn
	responsePayloads map[string]any
}

// respondWith makes the fake reply to future requests of the given method
// with this payload instead of the default empty object.
func (gateway *fakeGateway) respondWith(method string, payload any) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.responsePayloads[method] = payload
}

// closeConn drops the websocket from the server side, the way a crashed or
// restarted gateway process would.
func (gateway *fakeGateway) closeConn() {
	gateway.mu.Lock()
	conn := gateway.conn
	gateway.mu.Unlock()
	if conn != nil {
		_ = conn.Close(websocket.StatusGoingAway, "gateway restarting")
	}
}

func newFakeGateway(t *testing.T, rejectAuth bool) *fakeGateway {
	gateway := &fakeGateway{
		t:                t,
		rejectAuth:       rejectAuth,
		connectParams:    make(chan json.RawMessage, 1),
		requests:         make(chan gatewayRequest, 16),
		responsePayloads: make(map[string]any),
	}
	gateway.server = httptest.NewServer(http.HandlerFunc(gateway.handle))
	t.Cleanup(gateway.server.Close)
	return gateway
}

func (gateway *fakeGateway) url() string {
	return "ws://" + strings.TrimPrefix(gateway.server.URL, "http://")
}

func (gateway *fakeGateway) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ctx := r.Context()
	gateway.mu.Lock()
	gateway.conn = conn
	gateway.mu.Unlock()

	gateway.write(ctx, wireFrame{Type: "event", Event: "connect.challenge", Payload: map[string]any{"nonce": "nonce-1"}})

	first, err := gateway.read(ctx)
	if err != nil {
		return
	}
	if first.Type != "req" || first.Method != "connect" {
		conn.Close(websocket.StatusPolicyViolation, "first frame must be connect")
		return
	}
	gateway.connectParams <- first.Params
	if gateway.rejectAuth {
		gateway.write(ctx, wireFrame{Type: "res", ID: first.ID, OK: false, Error: map[string]any{"code": "unauthorized", "message": "bad token"}})
		conn.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	gateway.write(ctx, wireFrame{Type: "res", ID: first.ID, OK: true, Payload: map[string]any{
		"type":     "hello-ok",
		"protocol": 4,
		"server":   map[string]any{"version": "test", "connId": "conn-1"},
	}})

	for {
		frame, err := gateway.read(ctx)
		if err != nil {
			return
		}
		if frame.Type != "req" {
			continue
		}
		gateway.requests <- gatewayRequest{ID: frame.ID, Method: frame.Method, Params: frame.Params}
		gateway.mu.Lock()
		payload, hasPayload := gateway.responsePayloads[frame.Method]
		gateway.mu.Unlock()
		if !hasPayload {
			payload = map[string]any{}
		}
		gateway.write(ctx, wireFrame{Type: "res", ID: frame.ID, OK: true, Payload: payload})
	}
}

func (gateway *fakeGateway) read(ctx context.Context) (wireFrame, error) {
	_, data, err := gateway.conn.Read(ctx)
	if err != nil {
		return wireFrame{}, err
	}
	var frame wireFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return wireFrame{}, err
	}
	return frame, nil
}

func (gateway *fakeGateway) write(ctx context.Context, frame wireFrame) {
	data, err := json.Marshal(frame)
	if err != nil {
		gateway.t.Errorf("fake gateway encode: %v", err)
		return
	}
	gateway.mu.Lock()
	conn := gateway.conn
	gateway.mu.Unlock()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return
	}
}

// sendChat pushes a "chat" event down the stream, the way the real gateway
// streams a run.
func (gateway *fakeGateway) sendChat(payload ChatEventPayload) {
	gateway.write(context.Background(), wireFrame{Type: "event", Event: "chat", Payload: payload})
}

func (gateway *fakeGateway) nextRequest(t *testing.T) gatewayRequest {
	t.Helper()
	select {
	case request := <-gateway.requests:
		return request
	case <-time.After(5 * time.Second):
		t.Fatal("gateway saw no request in time")
		return gatewayRequest{}
	}
}

// --- recording publisher ---

type channelPublisher struct {
	events chan taskstate.MobileEvent
}

func newChannelPublisher() *channelPublisher {
	return &channelPublisher{events: make(chan taskstate.MobileEvent, 16)}
}

func (p *channelPublisher) PublishTaskEvent(_ context.Context, event taskstate.MobileEvent) error {
	p.events <- event
	return nil
}

func (p *channelPublisher) next(t *testing.T) taskstate.MobileEvent {
	t.Helper()
	select {
	case event := <-p.events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no phone event arrived in time")
		return taskstate.MobileEvent{}
	}
}

// --- helpers ---

func connectedSource(t *testing.T) (*Source, *fakeGateway, *channelPublisher) {
	t.Helper()
	gateway := newFakeGateway(t, false)
	publisher := newChannelPublisher()
	source, err := Connect(context.Background(), Config{
		URL:        gateway.url(),
		Token:      "test-token",
		TaskID:     testTaskID,
		SessionKey: testSessionKey,
		Publisher:  publisher,
		Logger:     slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { source.Close() })
	return source, gateway, publisher
}

// --- tests ---

func TestConnectPerformsOperatorHandshake(t *testing.T) {
	_, gateway, _ := connectedSource(t)

	var params struct {
		MinProtocol int    `json:"minProtocol"`
		MaxProtocol int    `json:"maxProtocol"`
		Role        string `json:"role"`
		Auth        struct {
			Token string `json:"token"`
		} `json:"auth"`
	}
	select {
	case raw := <-gateway.connectParams:
		if err := json.Unmarshal(raw, &params); err != nil {
			t.Fatalf("decode connect params: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gateway never saw a connect request")
	}
	if params.MinProtocol != 4 || params.MaxProtocol != 4 {
		t.Fatalf("protocol bounds = %d..%d, want exactly 4", params.MinProtocol, params.MaxProtocol)
	}
	if params.Role != "operator" {
		t.Fatalf("role = %q, want operator", params.Role)
	}
	if params.Auth.Token != "test-token" {
		t.Fatalf("auth token = %q, want the configured token", params.Auth.Token)
	}
}

// A fresh Source starts with a blank lastMessage, so every gateway redial
// used to blank the phone agent's Home preview until the next turn. The
// caller who redials knows what was last said; Connect must accept that
// memory and hand it straight back from CurrentTask.
func TestConnectSeedsTheLastMessageItWasGiven(t *testing.T) {
	gateway := newFakeGateway(t, false)
	seed := taskstate.LastMessage{From: taskstate.SpeakerAgent, Text: "Archived 41 conversations."}
	source, err := Connect(context.Background(), Config{
		URL:                gateway.url(),
		Token:              "test-token",
		TaskID:             testTaskID,
		SessionKey:         testSessionKey,
		Publisher:          newChannelPublisher(),
		Logger:             slog.New(slog.DiscardHandler),
		InitialLastMessage: seed,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { source.Close() })

	task, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask: %v", err)
	}
	if task.LastMessage != seed {
		t.Fatalf("last message after connect = %+v, want the seeded %+v", task.LastMessage, seed)
	}
}

func TestConnectRefusedAuthFailsClosed(t *testing.T) {
	gateway := newFakeGateway(t, true)
	_, err := Connect(context.Background(), Config{
		URL:        gateway.url(),
		Token:      "wrong-token",
		TaskID:     testTaskID,
		SessionKey: testSessionKey,
		Publisher:  newChannelPublisher(),
		Logger:     slog.New(slog.DiscardHandler),
	})
	if err == nil {
		t.Fatal("a refused connect must surface as an error, not a half-open source")
	}
}

func TestStartTurnForwardsChatSendAndStreamsReply(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello agent")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	if result.ThreadID != testTaskID {
		t.Fatalf("result thread = %q, want %q", result.ThreadID, testTaskID)
	}
	if result.TurnID == "" {
		t.Fatal("a started turn must have an id the launcher can track")
	}

	request := gateway.nextRequest(t)
	if request.Method != "chat.send" {
		t.Fatalf("gateway saw %q, want chat.send", request.Method)
	}
	var params struct {
		SessionKey     string `json:"sessionKey"`
		Message        string `json:"message"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("decode chat.send params: %v", err)
	}
	if params.SessionKey != testSessionKey {
		t.Fatalf("sessionKey = %q, want %q", params.SessionKey, testSessionKey)
	}
	if params.Message != "hello agent" {
		t.Fatalf("message = %q, want the prompt verbatim", params.Message)
	}
	if params.IdempotencyKey == "" || params.IdempotencyKey != result.TurnID {
		t.Fatalf("idempotencyKey = %q, must be set and equal the returned TurnID %q", params.IdempotencyKey, result.TurnID)
	}

	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "Hi there", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})

	working := publisher.next(t)
	if working.Kind != "activity" || working.State != taskstate.Working || !working.StartsTurn {
		t.Fatalf("first event = %+v, want a turn-starting Working activity", working)
	}
	reply := publisher.next(t)
	if reply.Kind != "reply" || reply.State != taskstate.IdleAfterReply || reply.Summary != "Hi there" {
		t.Fatalf("second event = %+v, want the assembled reply", reply)
	}
}

// A triggered turn (a Beeper message waking the agent) sends chat.send like
// any turn, but the Home preview must not claim the owner spoke — the
// stamped last message is a plain preview, not {user, prompt}.
func TestStartTriggeredTurnSendsChatAndStampsAPlainPreview(t *testing.T) {
	source, gateway, _ := connectedSource(t)

	result, err := source.StartTriggeredTurn(context.Background(), testTaskID, "[trigger] Maya: you around tonight?", "New message from Maya")
	if err != nil {
		t.Fatalf("StartTriggeredTurn: %v", err)
	}
	if result.ThreadID != testTaskID {
		t.Fatalf("result thread = %q, want %q", result.ThreadID, testTaskID)
	}

	request := gateway.nextRequest(t)
	if request.Method != "chat.send" {
		t.Fatalf("gateway saw %q, want chat.send", request.Method)
	}
	var params struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("decode chat.send params: %v", err)
	}
	if params.Message != "[trigger] Maya: you around tonight?" {
		t.Fatalf("message = %q, want the trigger prompt verbatim", params.Message)
	}

	task, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask: %v", err)
	}
	want := taskstate.LastMessage{From: taskstate.SpeakerPlain, Text: "New message from Maya"}
	if task.LastMessage != want {
		t.Fatalf("last message = %+v, want the plain preview %+v", task.LastMessage, want)
	}
}

// The protocol doc marks the chat.send ack shape as unknown: the gateway
// may assign its own runId rather than echoing the idempotencyKey. When the
// ack carries one, it is the id the stream will use, so it must win.
func TestStartTurnPrefersServerAssignedRunID(t *testing.T) {
	source, gateway, publisher := connectedSource(t)
	gateway.respondWith("chat.send", map[string]any{"runId": "srv-run-9"})

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello agent")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	if result.TurnID != "srv-run-9" {
		t.Fatalf("TurnID = %q, want the server-assigned runId srv-run-9", result.TurnID)
	}
	gateway.nextRequest(t)

	// The active turn tracked from the stream must be the same id the
	// caller was handed, or the launcher cannot match "the turn I started"
	// to "the turn that is running".
	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "working on it", RunID: "srv-run-9", SessionKey: testSessionKey, Seq: 1})
	publisher.next(t)
	task, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask: %v", err)
	}
	if task.ActiveTurnID != result.TurnID {
		t.Fatalf("ActiveTurnID = %q, want the started turn %q", task.ActiveTurnID, result.TurnID)
	}
}

// A gateway that drops the socket must be observable: Done unblocks so the
// runtime can clear task-capable state and dial again.
func TestDoneClosesWhenGatewayDrops(t *testing.T) {
	source, gateway, _ := connectedSource(t)

	select {
	case <-source.Done():
		t.Fatal("Done must stay open while the connection is alive")
	default:
	}

	gateway.closeConn()
	select {
	case <-source.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done never closed after the gateway dropped the socket")
	}
}

func TestStartTurnForUnknownTaskIsRefused(t *testing.T) {
	source, _, _ := connectedSource(t)
	if _, err := source.StartExistingTurn(context.Background(), "some-other-task", "hi"); err == nil {
		t.Fatal("the source only speaks for the phone agent's own task")
	}
}

func TestRedirectForwardsSteer(t *testing.T) {
	source, gateway, _ := connectedSource(t)

	if _, err := source.RedirectExistingTurn(context.Background(), testTaskID, "change of plan"); err != nil {
		t.Fatalf("RedirectExistingTurn: %v", err)
	}
	request := gateway.nextRequest(t)
	if request.Method != "sessions.steer" {
		t.Fatalf("gateway saw %q, want sessions.steer", request.Method)
	}
	var params struct {
		Key     string `json:"key"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("decode steer params: %v", err)
	}
	// sessions.steer takes "key", not "sessionKey" — the gateway validates
	// this strictly.
	if params.Key != testSessionKey {
		t.Fatalf("key = %q, want %q", params.Key, testSessionKey)
	}
	if params.Message != "change of plan" {
		t.Fatalf("message = %q, want the steer text verbatim", params.Message)
	}
}

func TestInterruptForwardsAbort(t *testing.T) {
	source, gateway, _ := connectedSource(t)

	if _, err := source.InterruptExistingTurn(context.Background(), testTaskID); err != nil {
		t.Fatalf("InterruptExistingTurn: %v", err)
	}
	request := gateway.nextRequest(t)
	if request.Method != "chat.abort" {
		t.Fatalf("gateway saw %q, want chat.abort", request.Method)
	}
	var params struct {
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("decode abort params: %v", err)
	}
	if params.SessionKey != testSessionKey {
		t.Fatalf("sessionKey = %q, want %q", params.SessionKey, testSessionKey)
	}
}

func TestListRecentReturnsThePhoneAgentTask(t *testing.T) {
	source, _, _ := connectedSource(t)

	tasks, err := source.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want exactly the phone agent's task", len(tasks))
	}
	task := tasks[0]
	if task.ID != testTaskID {
		t.Fatalf("task id = %q, want %q", task.ID, testTaskID)
	}
	if task.Title == "" {
		t.Fatal("the task needs a human-readable title for the launcher's list")
	}
	if task.UpdatedAtUnix <= 0 {
		t.Fatal("snapshot validation drops tasks without a positive UpdatedAtUnix")
	}
	if task.Source != taskstate.SourceAppServer {
		t.Fatalf("task source = %q, want %q", task.Source, taskstate.SourceAppServer)
	}

	current, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask: %v", err)
	}
	if current.ID != testTaskID {
		t.Fatalf("current task id = %q, want %q", current.ID, testTaskID)
	}
}

func TestCurrentTaskRemembersWhoSpokeLast(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello agent")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)

	sent, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after send: %v", err)
	}
	wantUser := taskstate.LastMessage{From: taskstate.SpeakerUser, Text: "hello agent"}
	if sent.LastMessage != wantUser {
		t.Fatalf("last message after send = %#v, want %#v", sent.LastMessage, wantUser)
	}

	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "Hi there", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	publisher.next(t)

	if _, err := source.RedirectExistingTurn(context.Background(), testTaskID, "change of plan"); err != nil {
		t.Fatalf("RedirectExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	steered, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after steer: %v", err)
	}
	wantSteer := taskstate.LastMessage{From: taskstate.SpeakerUser, Text: "change of plan"}
	if steered.LastMessage != wantSteer {
		t.Fatalf("last message after steer = %#v, want %#v", steered.LastMessage, wantSteer)
	}

	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})
	publisher.next(t)

	replied, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after reply: %v", err)
	}
	wantAgent := taskstate.LastMessage{From: taskstate.SpeakerAgent, Text: "Hi there"}
	if replied.LastMessage != wantAgent {
		t.Fatalf("last message after reply = %#v, want %#v", replied.LastMessage, wantAgent)
	}
}

func TestCurrentTaskTracksRunState(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)

	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "thinking", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	publisher.next(t)

	working, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask while working: %v", err)
	}
	if working.State != taskstate.Working {
		t.Fatalf("state mid-run = %q, want working", working.State)
	}
	if !working.CanRedirect {
		t.Fatal("a working run must be steerable")
	}

	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})
	publisher.next(t)

	idle, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after final: %v", err)
	}
	if idle.State != taskstate.IdleAfterReply {
		t.Fatalf("state after final = %q, want idle_after_reply", idle.State)
	}
	if idle.CanRedirect {
		t.Fatal("an idle task has no turn to steer")
	}
}
