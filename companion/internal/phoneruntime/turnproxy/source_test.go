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
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
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
	chatSendHold     chan struct{}
}

func (gateway *fakeGateway) holdChatSend() {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.chatSendHold = make(chan struct{})
}

func (gateway *fakeGateway) releaseChatSend() {
	gateway.mu.Lock()
	hold := gateway.chatSendHold
	gateway.chatSendHold = nil
	gateway.mu.Unlock()
	if hold != nil {
		close(hold)
	}
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
		hold := gateway.chatSendHold
		payload, hasPayload := gateway.responsePayloads[frame.Method]
		gateway.mu.Unlock()
		if frame.Method == "chat.send" && hold != nil {
			<-hold
		}
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

// sendAgent pushes an "agent" event down the stream (thinking tokens, tools).
func (gateway *fakeGateway) sendAgent(payload AgentEventPayload) {
	gateway.write(context.Background(), wireFrame{Type: "event", Event: "agent", Payload: payload})
}

func (gateway *fakeGateway) sendRaw(event string, payload any) {
	gateway.write(context.Background(), wireFrame{Type: "event", Event: event, Payload: payload})
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

type safeLogBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func newSafeLogBuffer() *safeLogBuffer {
	return &safeLogBuffer{}
}

func (logs *safeLogBuffer) Write(p []byte) (int, error) {
	logs.mu.Lock()
	defer logs.mu.Unlock()
	return logs.buf.Write(p)
}

func (logs *safeLogBuffer) String() string {
	logs.mu.Lock()
	defer logs.mu.Unlock()
	return logs.buf.String()
}

// --- helpers ---

func connectedSource(t *testing.T) (*Source, *fakeGateway, *channelPublisher) {
	t.Helper()
	source, gateway, publisher, _ := connectedSourceLogging(t, slog.New(slog.DiscardHandler))
	return source, gateway, publisher
}

func connectedSourceLogging(t *testing.T, logger *slog.Logger) (*Source, *fakeGateway, *channelPublisher, *safeLogBuffer) {
	t.Helper()
	logs := newSafeLogBuffer()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	gateway := newFakeGateway(t, false)
	publisher := newChannelPublisher()
	source, err := Connect(context.Background(), Config{
		URL:        gateway.url(),
		Token:      "test-token",
		TaskID:     testTaskID,
		SessionKey: testSessionKey,
		Publisher:  publisher,
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { source.Close() })
	return source, gateway, publisher, logs
}

func waitLogsContain(t *testing.T, logs *safeLogBuffer, needle string) {
	t.Helper()
	waitLogsCount(t, logs, needle, 1)
}

func waitLogsCount(t *testing.T, logs *safeLogBuffer, needle string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(logs.String(), needle) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("logs have %d of %q, want >= %d:\n%s", strings.Count(logs.String(), needle), needle, want, logs.String())
}

func waitLogLineContains(t *testing.T, logs *safeLogBuffer, needles ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range strings.Split(logs.String(), "\n") {
			matched := true
			for _, needle := range needles {
				if !strings.Contains(line, needle) {
					matched = false
					break
				}
			}
			if matched {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no log line contains %q:\n%s", needles, logs.String())
}

func waitActiveTurn(t *testing.T, source *Source, taskID, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		task, err := source.CurrentTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("CurrentTask: %v", err)
		}
		last = task.ActiveTurnID
		if last == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ActiveTurnID = %q, want %q", last, want)
}

func (p *channelPublisher) expectEmpty(t *testing.T) {
	t.Helper()
	select {
	case event := <-p.events:
		t.Fatalf("unexpected phone event %+v", event)
	default:
	}
}

// --- tests ---

func TestConnectPerformsOperatorHandshake(t *testing.T) {
	_, gateway, _ := connectedSource(t)

	var params struct {
		MinProtocol int      `json:"minProtocol"`
		MaxProtocol int      `json:"maxProtocol"`
		Role        string   `json:"role"`
		Caps        []string `json:"caps"`
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
	if !containsString(params.Caps, thinkingEventsCap) || !containsString(params.Caps, sessionScopedEventsCap) {
		t.Fatalf("caps = %q, want %s and %s so this OpenClaw version can stream thinking", params.Caps, thinkingEventsCap, sessionScopedEventsCap)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

	expectSendWorking(t, publisher, testTaskID)
	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "Hi there", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})

	working := publisher.next(t)
	if working.Kind != "activity" || working.State != taskstate.Working || working.StartsTurn || working.Summary != "Hi there" {
		t.Fatalf("delta event = %+v, want updating Working with the live reply", working)
	}
	reply := publisher.next(t)
	if reply.Kind != "reply" || reply.State != taskstate.IdleAfterReply || reply.Summary != "Hi there" {
		t.Fatalf("final event = %+v, want the assembled reply", reply)
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
	if result.TurnID == "srv-run-9" {
		t.Fatal("StartExistingTurn must return before the chat.send ack, so the TurnID is the idempotency key")
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)
	waitActiveTurn(t, source, testTaskID, "srv-run-9")

	// The stream uses the server-assigned id. After the ack aliases it onto
	// the turn StartRun opened, those events must still attach.
	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "working on it", RunID: "srv-run-9", SessionKey: testSessionKey, Seq: 1})
	delta := publisher.next(t)
	if delta.Kind != "activity" || delta.Summary != "working on it" {
		t.Fatalf("server-run delta = %+v, want it attached to the open turn", delta)
	}
	task, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask: %v", err)
	}
	if task.ActiveTurnID != "srv-run-9" {
		t.Fatalf("ActiveTurnID = %q, want the server-assigned runId", task.ActiveTurnID)
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

	expectSendWorking(t, publisher, testTaskID)
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

// A pretty-printed agent reply must land in lastMessage without control
// characters, or the next snapshot refresh fails invalid_safe_projection
// and Operator stays on Working.
func TestCurrentTaskSanitizesMultilineAgentReply(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "print json")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "{\n  \"ok\": true\n}", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	working := publisher.next(t)
	if working.Kind != "activity" {
		t.Fatalf("delta event = %#v, want working activity", working)
	}

	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})
	reply := publisher.next(t)
	if reply.Kind != "reply" || reply.Summary != "{ \"ok\": true }" {
		t.Fatalf("published reply = %#v", reply)
	}

	replied, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after reply: %v", err)
	}
	want := taskstate.LastMessage{From: taskstate.SpeakerAgent, Text: "{ \"ok\": true }"}
	if replied.LastMessage != want {
		t.Fatalf("last message = %#v, want %#v", replied.LastMessage, want)
	}
}

func TestCurrentTaskSanitizesMultilineUserPrompt(t *testing.T) {
	source, gateway, _ := connectedSource(t)

	if _, err := source.StartExistingTurn(context.Background(), testTaskID, "line one\nline two"); err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)

	sent, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after send: %v", err)
	}
	want := taskstate.LastMessage{From: taskstate.SpeakerUser, Text: "line one line two"}
	if sent.LastMessage != want {
		t.Fatalf("last message after send = %#v, want %#v", sent.LastMessage, want)
	}
}

func TestCurrentTaskTracksRunState(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

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

func TestReadTranscriptReturnsUserAndAgentLinesFromThisSession(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("empty ReadTranscript: %v", err)
	}
	if page.TaskID != testTaskID || page.Entries == nil {
		t.Fatalf("empty page = %+v, want task %q with a non-nil entries slice", page, testTaskID)
	}
	if len(page.Entries) != 0 {
		t.Fatalf("fresh source entries = %d, want none", len(page.Entries))
	}

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "Say only: pong")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)
	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "pong", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})
	publisher.next(t)
	publisher.next(t)

	page, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after turn: %v", err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("entries = %d, want user then agent", len(page.Entries))
	}
	if page.Entries[0].Kind != tasktranscript.KindUser || page.Entries[0].Text != "Say only: pong" {
		t.Fatalf("first entry = %+v, want the owner's prompt", page.Entries[0])
	}
	if page.Entries[1].Kind != tasktranscript.KindAgent || page.Entries[1].Text != "pong" {
		t.Fatalf("second entry = %+v, want the agent reply", page.Entries[1])
	}
	if page.Entries[0].ID == "" || page.Entries[0].TurnID == "" || page.Entries[1].ID == "" || page.Entries[1].TurnID == "" {
		t.Fatal("every transcript line needs an id and turnId for the mobile contract")
	}

	if _, err := source.ReadTranscript(context.Background(), "some-other-task", tasktranscript.PageOptions{TaskID: "some-other-task", Limit: 32}); err == nil {
		t.Fatal("ReadTranscript must refuse a task this source does not speak for")
	}
}

func TestReadTranscriptIncludesTheSeededLastMessage(t *testing.T) {
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

	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Kind != tasktranscript.KindAgent || page.Entries[0].Text != seed.Text {
		t.Fatalf("seeded page = %+v, want the last known agent line", page.Entries)
	}
}

func TestThinkingAgentEventsFillReasoningTranscriptAndUpdateWorking(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendAgent(AgentEventPayload{
		Stream: "thinking", RunID: result.TurnID, SessionKey: testSessionKey,
		Data: AgentEventData{Text: "Checking the calendar\nthen drafting"},
	})
	working := publisher.next(t)
	if working.Kind != "activity" || working.State != taskstate.Working || working.StartsTurn {
		t.Fatalf("first thinking = %+v, want Working without StartsTurn after send", working)
	}
	if working.Summary != "Checking the calendar then drafting" {
		t.Fatalf("working summary = %q, want collapsed reasoning", working.Summary)
	}

	gateway.sendAgent(AgentEventPayload{
		Stream: "thinking", RunID: result.TurnID, SessionKey: testSessionKey,
		Data: AgentEventData{Text: "Checking the calendar then drafting a reply"},
	})
	updated := publisher.next(t)
	if updated.Kind != "activity" || updated.StartsTurn || updated.Summary == working.Summary {
		t.Fatalf("later thinking = %+v, want a changed Working summary without StartsTurn", updated)
	}

	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("entries = %+v, want user then one updating reasoning row", page.Entries)
	}
	if page.Entries[0].Kind != tasktranscript.KindUser {
		t.Fatalf("first entry = %+v, want the prompt", page.Entries[0])
	}
	if page.Entries[1].Kind != tasktranscript.KindReasoning || page.Entries[1].Text != updated.Summary {
		t.Fatalf("reasoning entry = %+v, want the latest safe summary", page.Entries[1])
	}

	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Message: json.RawMessage(`"pong"`), Seq: 3})
	reply := publisher.next(t)
	if reply.Kind != "reply" || reply.Summary != "pong" {
		t.Fatalf("final = %+v, want reply", reply)
	}
	page, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after reply: %v", err)
	}
	if len(page.Entries) != 3 || page.Entries[2].Kind != tasktranscript.KindAgent {
		t.Fatalf("after reply entries = %+v, want user, reasoning, agent", page.Entries)
	}
}

func TestThinkingWithAlternateSessionKeyOrNestedPayloadAttachesToTheActiveRun(t *testing.T) {
	source, gateway, publisher := connectedSource(t)
	home, err := source.StartExistingTurn(context.Background(), HomeComposeTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("home compose: %v", err)
	}
	homeSend := gateway.nextRequest(t)
	var homeParams struct {
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(homeSend.Params, &homeParams); err != nil {
		t.Fatalf("decode home chat.send: %v", err)
	}
	expectSendWorking(t, publisher, home.ThreadID)

	gateway.sendRaw("agent", map[string]any{
		"payload": map[string]any{
			"runId":  home.TurnID,
			"stream": "thinking",
			"data":   map[string]any{"text": "Checking the calendar"},
		},
	})
	first := publisher.next(t)
	if first.Kind != "activity" || first.State != taskstate.Working || first.Summary != "Checking the calendar" || first.TaskID != home.ThreadID {
		t.Fatalf("nested thinking = %+v, want Working on the home chat", first)
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      home.TurnID,
		"sessionKey": "agent:main:main",
		"stream":     "thinking",
		"data":       map[string]any{"text": "Checking the calendar then drafting"},
	})
	updated := publisher.next(t)
	if updated.TaskID != home.ThreadID || updated.Summary != "Checking the calendar then drafting" {
		t.Fatalf("mismatched sessionKey thinking = %+v, want it on the home chat not inbound", updated)
	}

	homePage, err := source.ReadTranscript(context.Background(), home.ThreadID, tasktranscript.PageOptions{TaskID: home.ThreadID, Limit: 32})
	if err != nil {
		t.Fatalf("home transcript: %v", err)
	}
	if len(homePage.Entries) != 2 || homePage.Entries[1].Kind != tasktranscript.KindReasoning || homePage.Entries[1].Text != updated.Summary {
		t.Fatalf("home transcript = %+v, want user then updating KindReasoning", homePage.Entries)
	}
	inboundPage, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("inbound transcript: %v", err)
	}
	for _, entry := range inboundPage.Entries {
		if entry.Kind == tasktranscript.KindReasoning {
			t.Fatalf("home thinking leaked into inbound: %+v", entry)
		}
	}

	gateway.sendRaw("agent", map[string]any{
		"type":       "thinking",
		"runId":      home.TurnID,
		"text":       "Need a shorter path",
		"sessionKey": "agent:main:main",
	})
	topLevel := publisher.next(t)
	if topLevel.TaskID != home.ThreadID || topLevel.Summary != "Need a shorter path" {
		t.Fatalf("top-level type thinking = %+v, want it on the home chat", topLevel)
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      "run-unrelated",
		"sessionKey": "agent:main:main",
		"stream":     "thinking",
		"data":       map[string]any{"text": "should not land on inbound"},
	})
	gateway.sendRaw("agent", map[string]any{
		"runId":  home.TurnID,
		"stream": "thinking",
		"data":   map[string]any{"text": "Still drafting on the home chat"},
	})
	afterUnrelated := publisher.next(t)
	if afterUnrelated.Summary == "should not land on inbound" || afterUnrelated.TaskID != home.ThreadID {
		t.Fatalf("unrelated thinking with inbound sessionKey leaked: %+v", afterUnrelated)
	}
	if afterUnrelated.Summary != "Still drafting on the home chat" {
		t.Fatalf("next event after unrelated thinking = %+v, want the home chat update", afterUnrelated)
	}
	inboundPage, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("inbound transcript after unrelated: %v", err)
	}
	for _, entry := range inboundPage.Entries {
		if entry.Kind == tasktranscript.KindReasoning {
			t.Fatalf("unrelated thinking leaked into inbound: %+v", entry)
		}
	}
}

func TestPixelItemReasoningFillsKindReasoningAndUpdatesWorking(t *testing.T) {
	source, gateway, publisher, logs := connectedSourceLogging(t, nil)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "item",
		"data": map[string]any{
			"itemId": "rsn-1",
			"phase":  "start",
			"kind":   "analysis",
			"title":  "Reasoning",
			"status": "running",
		},
	})
	waitLogsContain(t, logs, "drop_reason=no_text")
	if !strings.Contains(logs.String(), "item_type=reasoning") {
		t.Fatalf("title-only item must log item_type=reasoning, logs:\n%s", logs.String())
	}
	publisher.expectEmpty(t)
	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after title-only: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Kind != tasktranscript.KindUser {
		t.Fatalf("after title-only entries = %+v, want only the user line (Chrome Working is enough)", page.Entries)
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "item",
		"data": map[string]any{
			"item": map[string]any{
				"id":      "rsn-1",
				"type":    "reasoning",
				"summary": []string{"Checking the calendar"},
				"content": []string{"hidden chain of thought"},
			},
		},
	})
	summarized := publisher.next(t)
	if summarized.Summary != "Checking the calendar" || summarized.StartsTurn {
		t.Fatalf("nested summary = %+v, want an updating Working summary", summarized)
	}

	gateway.sendRaw("agent", map[string]any{
		"payload": map[string]any{
			"runId":  result.TurnID,
			"stream": "codex_app_server.item",
			"data": map[string]any{
				"phase": "completed",
				"type":  "reasoning",
				"item": map[string]any{
					"type": "reasoning",
					"summary": []map[string]string{
						{"type": "summary_text", "text": "Checking the calendar then drafting"},
					},
					"content": []map[string]string{
						{"type": "reasoning_text", "text": "hidden chain of thought"},
					},
				},
			},
		},
	})
	updated := publisher.next(t)
	if updated.Kind != "activity" || updated.StartsTurn || updated.Summary == summarized.Summary {
		t.Fatalf("later item reasoning = %+v, want a changed Working summary without StartsTurn", updated)
	}
	if updated.Summary != "Checking the calendar then drafting" {
		t.Fatalf("updated summary = %q", updated.Summary)
	}

	page, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("entries = %+v, want user then one updating reasoning row", page.Entries)
	}
	if page.Entries[1].Kind != tasktranscript.KindReasoning || page.Entries[1].Text != updated.Summary {
		t.Fatalf("reasoning entry = %+v, want the latest safe summary", page.Entries[1])
	}
	if strings.Contains(page.Entries[1].Text, "hidden") {
		t.Fatal("hidden CoT leaked into the transcript")
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "assistant",
		"data": map[string]any{
			"text": "pong",
			"content": []map[string]string{
				{"type": "thinking", "text": "Need a shorter path"},
				{"type": "text", "text": "pong"},
			},
		},
	})
	assistantThinking := publisher.next(t)
	if assistantThinking.Summary != "Need a shorter path" {
		t.Fatalf("assistant thinking = %+v, want reasoning from content type, not reply text", assistantThinking)
	}
}

func TestUnmatchedItemReasoningDoesNotAttachToInboundPhoneAgent(t *testing.T) {
	source, gateway, publisher := connectedSource(t)
	home, err := source.StartExistingTurn(context.Background(), HomeComposeTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("home compose: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, home.ThreadID)

	gateway.sendRaw("agent", map[string]any{
		"runId":      "run-unrelated",
		"sessionKey": "agent:main:main",
		"stream":     "item",
		"data": map[string]any{
			"item": map[string]any{"type": "reasoning", "summary": []string{"should not land on inbound"}},
		},
	})
	gateway.sendRaw("agent", map[string]any{
		"runId":  home.TurnID,
		"stream": "codex_app_server.item",
		"data": map[string]any{
			"type": "reasoning",
			"item": map[string]any{"type": "reasoning", "summary": []string{"Home chat reasoning"}},
		},
	})
	applied := publisher.next(t)
	if applied.Summary == "should not land on inbound" || applied.TaskID != home.ThreadID {
		t.Fatalf("unrelated item reasoning leaked: %+v", applied)
	}
	if applied.Summary != "Home chat reasoning" {
		t.Fatalf("home item reasoning = %+v", applied)
	}
	inboundPage, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("inbound transcript: %v", err)
	}
	for _, entry := range inboundPage.Entries {
		if entry.Kind == tasktranscript.KindReasoning {
			t.Fatalf("unrelated item reasoning leaked into inbound: %+v", entry)
		}
	}
}

func TestAgentEventLogsApplicationWithoutThinkingText(t *testing.T) {
	logs := newSafeLogBuffer()
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	gateway := newFakeGateway(t, false)
	publisher := newChannelPublisher()
	source, err := Connect(context.Background(), Config{
		URL:        gateway.url(),
		Token:      "test-token",
		TaskID:     testTaskID,
		SessionKey: testSessionKey,
		Publisher:  publisher,
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { source.Close() })

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendRaw("agent", map[string]any{
		"runId":  result.TurnID,
		"stream": "lifecycle",
		"data":   map[string]any{"phase": "start"},
	})
	gateway.sendAgent(AgentEventPayload{
		Stream: "thinking", RunID: result.TurnID, SessionKey: testSessionKey,
		Data: AgentEventData{Text: "secret chain of thought"},
	})
	_ = publisher.next(t)

	logsText := logs.String()
	if strings.Contains(logsText, "secret chain of thought") {
		t.Fatal("thinking text must not appear in logs")
	}
	if !strings.Contains(logsText, "received agent event") || !strings.Contains(logsText, "drop_reason=not_reasoning") {
		t.Fatalf("lifecycle agent event was not logged as dropped, logs:\n%s", logsText)
	}
	if !strings.Contains(logsText, "applied=true") || !strings.Contains(logsText, "session_key_present=true") {
		t.Fatalf("applied thinking was not logged, logs:\n%s", logsText)
	}
	if !strings.Contains(logsText, "reasoning_runes=") {
		t.Fatalf("applied thinking must log a rune count, logs:\n%s", logsText)
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":  result.TurnID,
		"stream": "item",
		"data": map[string]any{
			"item": map[string]any{
				"type":    "reasoning",
				"summary": []string{"item summary without logging the words"},
			},
		},
	})
	_ = publisher.next(t)
	logsText = logs.String()
	if strings.Contains(logsText, "item summary without logging the words") {
		t.Fatal("item reasoning text must not appear in logs")
	}
	if !strings.Contains(logsText, "stream=item") || !strings.Contains(logsText, "item_type=reasoning") {
		t.Fatalf("item reasoning was not logged with enum type, logs:\n%s", logsText)
	}

	gateway.sendAgent(AgentEventPayload{
		Stream: "thinking", RunID: result.TurnID, SessionKey: testSessionKey,
	})
	gateway.sendAgent(AgentEventPayload{
		Stream: "thinking", RunID: result.TurnID, SessionKey: testSessionKey,
		Data: AgentEventData{Text: "later thought"},
	})
	_ = publisher.next(t)
	logsText = logs.String()
	if !strings.Contains(logsText, "applied=false") || !strings.Contains(logsText, "drop_reason=unchanged") {
		t.Fatalf("empty thinking must log applied=false, logs:\n%s", logsText)
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "assistant",
		"data":       map[string]any{"text": "secret assistant tokens", "delta": "secret assistant tokens"},
	})
	waitLogLineContains(t, logs, "stream=assistant", "drop_reason=not_reasoning")
	if strings.Contains(logs.String(), "secret assistant tokens") {
		t.Fatal("assistant reply text must not appear in logs")
	}
	publisher.expectEmpty(t)
}

func TestThinkingAgentEventsFromOtherSessionsAreIgnoredBySource(t *testing.T) {
	source, gateway, publisher := connectedSource(t)
	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendAgent(AgentEventPayload{
		Stream: "thinking", RunID: "run-z", SessionKey: "agent:other:main",
		Data: AgentEventData{Text: "secret chain of thought"},
	})
	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "Hi", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	working := publisher.next(t)
	if working.Summary == "secret chain of thought" {
		t.Fatal("foreign thinking must not become this session's Working summary")
	}
	if working.Kind != "activity" || working.Summary != "Hi" {
		t.Fatalf("expected the chat delta Working, got %+v", working)
	}
	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	for _, entry := range page.Entries {
		if entry.Kind == tasktranscript.KindReasoning {
			t.Fatalf("foreign thinking leaked into the transcript: %+v", entry)
		}
	}
}

func TestHomeComposeStartsANewConversationInsteadOfAppendingPhoneAgent(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	first, err := source.StartExistingTurn(context.Background(), testTaskID, "old inbound prompt")
	if err != nil {
		t.Fatalf("inbound StartExistingTurn: %v", err)
	}
	inboundSend := gateway.nextRequest(t)
	if inboundSend.Method != "chat.send" {
		t.Fatalf("inbound method = %q", inboundSend.Method)
	}
	var inboundParams struct {
		SessionKey string `json:"sessionKey"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal(inboundSend.Params, &inboundParams); err != nil {
		t.Fatalf("decode inbound chat.send: %v", err)
	}
	if inboundParams.SessionKey != testSessionKey {
		t.Fatalf("inbound session = %q, want the stable agent session", inboundParams.SessionKey)
	}

	expectSendWorking(t, publisher, testTaskID)
	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "old reply", RunID: first.TurnID, SessionKey: testSessionKey, Seq: 1})
	gateway.sendChat(ChatEventPayload{State: "final", RunID: first.TurnID, SessionKey: testSessionKey, Seq: 2})
	publisher.next(t)
	publisher.next(t)

	home, err := source.StartExistingTurn(context.Background(), HomeComposeTaskID, "new home prompt")
	if err != nil {
		t.Fatalf("home compose: %v", err)
	}
	if home.ThreadID == "" || home.ThreadID == testTaskID || home.ThreadID == HomeComposeTaskID {
		t.Fatalf("home thread = %q, want a fresh phone-chat id", home.ThreadID)
	}

	homeSend := gateway.nextRequest(t)
	if homeSend.Method != "chat.send" {
		t.Fatalf("home method = %q", homeSend.Method)
	}
	var homeParams struct {
		SessionKey string `json:"sessionKey"`
		Message    string `json:"message"`
		Thinking   string `json:"thinking"`
	}
	if err := json.Unmarshal(homeSend.Params, &homeParams); err != nil {
		t.Fatalf("decode home chat.send: %v", err)
	}
	if homeParams.SessionKey == testSessionKey || homeParams.SessionKey == "" {
		t.Fatalf("home session = %q, want a new session not the inbound one", homeParams.SessionKey)
	}
	if homeParams.Message != "new home prompt" {
		t.Fatalf("home message = %q", homeParams.Message)
	}
	if homeParams.Thinking != "max" {
		t.Fatalf("home thinking = %q, want high so the gateway streams thinking events", homeParams.Thinking)
	}

	tasks, err := source.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(tasks) < 2 {
		t.Fatalf("ListRecent = %+v, want inbound task plus the new home chat", tasks)
	}
	var sawInbound, sawHome bool
	for _, task := range tasks {
		if task.ID == testTaskID {
			sawInbound = true
		}
		if task.ID == home.ThreadID {
			sawHome = true
		}
		if task.ID == HomeComposeTaskID {
			t.Fatal("the compose inbox must not appear as a home row")
		}
	}
	if !sawInbound || !sawHome {
		t.Fatalf("ListRecent = %+v, missing inbound or home chat", tasks)
	}

	inboundPage, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("inbound transcript: %v", err)
	}
	if len(inboundPage.Entries) < 2 {
		t.Fatalf("inbound transcript = %+v, want the old prompt and reply kept", inboundPage.Entries)
	}
	homePage, err := source.ReadTranscript(context.Background(), home.ThreadID, tasktranscript.PageOptions{TaskID: home.ThreadID, Limit: 32})
	if err != nil {
		t.Fatalf("home transcript: %v", err)
	}
	if len(homePage.Entries) != 1 || homePage.Entries[0].Kind != tasktranscript.KindUser || homePage.Entries[0].Text != "new home prompt" {
		t.Fatalf("home transcript = %+v, want only the new prompt", homePage.Entries)
	}

	follow, err := source.StartExistingTurn(context.Background(), home.ThreadID, "follow up in the new thread")
	if err != nil {
		t.Fatalf("follow-up: %v", err)
	}
	if follow.ThreadID != home.ThreadID {
		t.Fatalf("follow-up thread = %q, want the same home chat %q", follow.ThreadID, home.ThreadID)
	}
	followSend := gateway.nextRequest(t)
	var followParams struct {
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(followSend.Params, &followParams); err != nil {
		t.Fatalf("decode follow-up chat.send: %v", err)
	}
	if followParams.SessionKey != homeParams.SessionKey {
		t.Fatalf("follow-up session = %q, want the home chat session %q", followParams.SessionKey, homeParams.SessionKey)
	}
}

func expectSendWorking(t *testing.T, publisher *channelPublisher, taskID string) taskstate.MobileEvent {
	t.Helper()
	event := publisher.next(t)
	if event.TaskID != taskID || event.Kind != "activity" || event.State != taskstate.Working || !event.StartsTurn || event.Summary != "Codex is working" {
		t.Fatalf("send Working = %+v, want StartsTurn activity on %q", event, taskID)
	}
	return event
}

func TestSendPublishesWorkingBeforeAnyChatDelta(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "hello agent")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)

	working := expectSendWorking(t, publisher, testTaskID)
	if working.Summary != "Codex is working" {
		t.Fatalf("send Working summary = %q", working.Summary)
	}

	task, err := source.CurrentTask(context.Background(), testTaskID)
	if err != nil {
		t.Fatalf("CurrentTask after send: %v", err)
	}
	if task.State != taskstate.Working {
		t.Fatalf("state after send = %q, want working before any chat delta", task.State)
	}
	if task.ActiveTurnID != result.TurnID {
		t.Fatalf("ActiveTurnID = %q, want the accepted turn %q", task.ActiveTurnID, result.TurnID)
	}
}

func TestHomeComposePublishesWorkingOnSend(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	home, err := source.StartExistingTurn(context.Background(), HomeComposeTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("home compose: %v", err)
	}
	gateway.nextRequest(t)

	expectSendWorking(t, publisher, home.ThreadID)

	task, err := source.CurrentTask(context.Background(), home.ThreadID)
	if err != nil {
		t.Fatalf("CurrentTask after home send: %v", err)
	}
	if task.State != taskstate.Working || task.ActiveTurnID != home.TurnID {
		t.Fatalf("home task after send = %+v, want Working on the accepted turn", task)
	}
}

func TestWorkingIsVisibleBeforeChatSendAck(t *testing.T) {
	source, gateway, publisher := connectedSource(t)
	gateway.holdChatSend()
	defer gateway.releaseChatSend()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	home, err := source.StartExistingTurn(ctx, HomeComposeTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("StartExistingTurn blocked on chat.send ack or failed: %v", err)
	}
	if !strings.HasPrefix(home.ThreadID, homeChatPrefix) {
		t.Fatalf("ThreadID = %q, want a phone-chat-* fork so the phone can open the thread before ack", home.ThreadID)
	}

	working := expectSendWorking(t, publisher, home.ThreadID)
	if working.Summary != "Codex is working" {
		t.Fatalf("Working before ack = %+v", working)
	}
	task, err := source.CurrentTask(context.Background(), home.ThreadID)
	if err != nil {
		t.Fatalf("CurrentTask before ack: %v", err)
	}
	if task.State != taskstate.Working || task.ActiveTurnID != home.TurnID {
		t.Fatalf("task before ack = %+v, want Working on the new thread immediately", task)
	}
	request := gateway.nextRequest(t)
	if request.Method != "chat.send" {
		t.Fatalf("gateway saw %q, want chat.send still waiting for ack", request.Method)
	}
	publisher.expectEmpty(t)
}

func TestPreambleAndCommandShowLiveProgress(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "item",
		"data": map[string]any{
			"item": map[string]any{"type": "preamble", "title": "Considering the request"},
		},
	})
	preamble := publisher.next(t)
	if preamble.Kind != "activity" || preamble.StartsTurn || preamble.Summary != "Considering the request" {
		t.Fatalf("preamble Working = %+v, want live progress", preamble)
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "codex_app_server.item",
		"data": map[string]any{
			"type": "commandExecution",
			"item": map[string]any{"type": "commandExecution", "command": "ls"},
		},
	})
	command := publisher.next(t)
	if command.Kind != "activity" || command.StartsTurn || command.Summary != "ls" {
		t.Fatalf("command Working = %+v, want the command text", command)
	}

	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if len(page.Entries) != 3 {
		t.Fatalf("entries = %+v, want user, KindActivity, KindCommand", page.Entries)
	}
	if page.Entries[1].Kind != tasktranscript.KindActivity || page.Entries[1].Text != "Considering the request" {
		t.Fatalf("activity entry = %+v", page.Entries[1])
	}
	if page.Entries[2].Kind != tasktranscript.KindCommand || page.Entries[2].Command != "ls" || page.Entries[2].Status != "inProgress" {
		t.Fatalf("command entry = %+v, want KindCommand with inProgress status", page.Entries[2])
	}
	for _, entry := range page.Entries {
		if entry.Kind == tasktranscript.KindReasoning {
			t.Fatalf("progress items leaked into KindReasoning: %+v", entry)
		}
	}
}

func TestChatDeltasUpdateAgentTranscriptBeforeFinal(t *testing.T) {
	source, gateway, publisher := connectedSource(t)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "Say only: pong")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "po", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 1})
	first := publisher.next(t)
	if first.Kind != "activity" || first.State != taskstate.Working || first.StartsTurn || first.Summary != "po" {
		t.Fatalf("first delta Working = %+v, want updating summary po", first)
	}

	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after first delta: %v", err)
	}
	if len(page.Entries) != 2 || page.Entries[0].Kind != tasktranscript.KindUser || page.Entries[1].Kind != tasktranscript.KindAgent || page.Entries[1].Text != "po" {
		t.Fatalf("after first delta entries = %+v, want user then live KindAgent", page.Entries)
	}

	gateway.sendChat(ChatEventPayload{State: "delta", DeltaText: "ng", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 2})
	second := publisher.next(t)
	if second.Kind != "activity" || second.Summary != "pong" || second.Summary == first.Summary {
		t.Fatalf("second delta Working = %+v, want changed summary pong", second)
	}

	page, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after second delta: %v", err)
	}
	if len(page.Entries) != 2 || page.Entries[1].Kind != tasktranscript.KindAgent || page.Entries[1].Text != "pong" {
		t.Fatalf("after second delta entries = %+v, want one updating KindAgent", page.Entries)
	}

	gateway.sendChat(ChatEventPayload{State: "final", RunID: result.TurnID, SessionKey: testSessionKey, Seq: 3})
	reply := publisher.next(t)
	if reply.Kind != "reply" || reply.Summary != "pong" {
		t.Fatalf("final = %+v, want the assembled reply", reply)
	}
	page, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after final: %v", err)
	}
	if len(page.Entries) != 2 || page.Entries[1].Kind != tasktranscript.KindAgent || page.Entries[1].Text != "pong" {
		t.Fatalf("after final entries = %+v, want the same KindAgent row", page.Entries)
	}
}

func TestAssistantReplyAndTitleOnlyDoNotBecomeReasoning(t *testing.T) {
	source, gateway, publisher, logs := connectedSourceLogging(t, nil)

	result, err := source.StartExistingTurn(context.Background(), testTaskID, "plan tonight")
	if err != nil {
		t.Fatalf("StartExistingTurn: %v", err)
	}
	gateway.nextRequest(t)
	expectSendWorking(t, publisher, testTaskID)

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "item",
		"data": map[string]any{
			"itemId": "rsn-1",
			"phase":  "start",
			"kind":   "analysis",
			"title":  "Reasoning",
			"status": "running",
		},
	})
	waitLogsContain(t, logs, "drop_reason=no_text")
	publisher.expectEmpty(t)

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "assistant",
		"data":       map[string]any{"text": "Checking", "delta": "Checking"},
	})
	waitLogLineContains(t, logs, "stream=assistant", "drop_reason=not_reasoning")
	publisher.expectEmpty(t)

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "item",
		"data": map[string]any{
			"item": map[string]any{
				"id":      "rsn-1",
				"type":    "reasoning",
				"summary": []string{"Checking the calendar then drafting"},
				"content": []string{"hidden chain of thought"},
			},
		},
	})
	summarized := publisher.next(t)
	if summarized.Summary != "Checking the calendar then drafting" {
		t.Fatalf("item summary = %+v, want KindReasoning text from the real summary", summarized)
	}

	page, err := source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if len(page.Entries) != 2 || page.Entries[1].Kind != tasktranscript.KindReasoning || page.Entries[1].Text != summarized.Summary {
		t.Fatalf("entries = %+v, want user then KindReasoning from the summary", page.Entries)
	}
	if strings.Contains(page.Entries[1].Text, "hidden") {
		t.Fatal("hidden CoT leaked into the transcript")
	}

	gateway.sendRaw("agent", map[string]any{
		"runId":      result.TurnID,
		"sessionKey": testSessionKey,
		"stream":     "item",
		"data": map[string]any{
			"itemId": "rsn-1",
			"phase":  "start",
			"kind":   "analysis",
			"title":  "Reasoning",
		},
	})
	waitLogsContain(t, logs, "drop_reason=unchanged")
	page, err = source.ReadTranscript(context.Background(), testTaskID, tasktranscript.PageOptions{TaskID: testTaskID, Limit: 32})
	if err != nil {
		t.Fatalf("ReadTranscript after title replay: %v", err)
	}
	if page.Entries[1].Kind != tasktranscript.KindReasoning || page.Entries[1].Text != summarized.Summary {
		t.Fatalf("title-only replay overwrote live reasoning: %+v", page.Entries[1])
	}
}

func TestTriggeredTurnStaysOnTheInboundSession(t *testing.T) {
	source, gateway, _ := connectedSource(t)
	if _, err := source.StartTriggeredTurn(context.Background(), testTaskID, "[trigger] Maya: hi", "New message from Maya"); err != nil {
		t.Fatalf("StartTriggeredTurn: %v", err)
	}
	request := gateway.nextRequest(t)
	var params struct {
		SessionKey string `json:"sessionKey"`
		Thinking   string `json:"thinking"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if params.SessionKey != testSessionKey {
		t.Fatalf("triggered session = %q, want the stable inbound session", params.SessionKey)
	}
	if params.Thinking != "" {
		t.Fatalf("triggered thinking = %q, want empty so inbound stays on the stable agent session without a user thinking hint", params.Thinking)
	}
}
