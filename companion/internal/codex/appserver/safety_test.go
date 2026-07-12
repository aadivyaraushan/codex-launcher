package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestQuestionResponseUsesCurrentSchemaAndPendingKind(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	response := make(chan map[string]json.RawMessage, 1)
	go func() {
		reader := bufio.NewReader(serverConn)
		encoder := json.NewEncoder(serverConn)
		line, _ := reader.ReadBytes('\n')
		var initialize capturedRequest
		_ = json.Unmarshal(line, &initialize)
		_ = encoder.Encode(map[string]any{"id": json.RawMessage(initialize.ID), "result": fakeInitializeResult()})
		_, _ = reader.ReadBytes('\n')
		_ = encoder.Encode(map[string]any{"id": "question-1", "method": "item/tool/requestUserInput", "params": map[string]any{
			"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-1", "questions": []map[string]any{{"id": "choice", "header": "Choice", "question": "Pick one"}},
		}})
		line, _ = reader.ReadBytes('\n')
		var got map[string]json.RawMessage
		_ = json.Unmarshal(line, &got)
		response <- got
	}()
	client := NewClient(clientConn, clientConn, discardLogger(), Options{ExperimentalQuestions: true})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	request := <-client.Requests()
	if request.Method != "item/tool/requestUserInput" || request.ThreadID != "thread-1" {
		t.Fatalf("request = %#v", request)
	}
	if err := client.RespondCommandApproval(ctx, "thread-1", request.ID, DecisionDecline); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("cross-kind error = %v", err)
	}
	if err := client.RespondUserInput(ctx, "thread-1", request.ID, map[string][]string{"other": {"A"}}); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("wrong question error = %v", err)
	}
	if err := client.RespondUserInput(ctx, "thread-2", request.ID, map[string][]string{"choice": {"A"}}); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("cross-thread question error = %v", err)
	}
	if err := client.RespondUserInput(ctx, "thread-1", request.ID, map[string][]string{"choice": {strings.Repeat("x", maxTurnTextBytes+1)}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized answer error = %v", err)
	}
	if err := client.RespondUserInput(ctx, "thread-1", request.ID, map[string][]string{"choice": {"A"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.RespondUserInput(ctx, "thread-1", request.ID, map[string][]string{"choice": {"B"}}); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("duplicate error = %v", err)
	}
	wire := <-response
	if string(wire["result"]) != `{"answers":{"choice":{"answers":["A"]}}}` {
		t.Fatalf("result = %s", wire["result"])
	}
}

func TestInitializeRejectsIncompleteCurrentSchemaResponse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		reader := bufio.NewReader(serverConn)
		line, _ := reader.ReadBytes('\n')
		var initialize capturedRequest
		_ = json.Unmarshal(line, &initialize)
		_ = json.NewEncoder(serverConn).Encode(map[string]any{"id": json.RawMessage(initialize.ID), "result": map[string]any{"userAgent": "fake"}})
		_ = serverConn.SetReadDeadline(time.Now().Add(time.Second))
		_, _ = reader.ReadBytes('\n')
		_ = serverConn.Close()
	}()
	client := NewClient(clientConn, clientConn, discardLogger(), Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err == nil {
		t.Fatal("accepted incomplete initialize response")
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("incompatible initialization left client open")
	}
}

func TestPartialInitializedAcknowledgementIsTerminal(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	wrapped := &failOnSecondWriteConn{Conn: clientConn}
	t.Cleanup(func() { _ = wrapped.Close(); _ = serverConn.Close() })
	go func() {
		reader := bufio.NewReader(serverConn)
		line, _ := reader.ReadBytes('\n')
		var initialize capturedRequest
		_ = json.Unmarshal(line, &initialize)
		_ = json.NewEncoder(serverConn).Encode(map[string]any{"id": json.RawMessage(initialize.ID), "result": fakeInitializeResult()})
	}()
	client := NewClient(wrapped, wrapped, discardLogger(), Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err == nil {
		t.Fatal("partial initialized acknowledgement succeeded")
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("partial acknowledgement left client open")
	}
}

func TestApprovalRegistryEnforcesOfferedDecisionAndPermissionShape(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go serveApprovalRequests(serverConn)
	client := NewClient(clientConn, clientConn, discardLogger(), Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	command := <-client.Requests()
	if command.ThreadID != "thread-1" || len(command.AllowedDecisions) != 1 || command.AllowedDecisions[0] != DecisionDecline {
		t.Fatalf("command request = %#v", command)
	}
	if err := client.RespondCommandApproval(ctx, "thread-1", command.ID, DecisionAcceptForSession); !errors.Is(err, ErrDecisionNotOffered) {
		t.Fatalf("unoffered decision error = %v", err)
	}
	if err := client.RespondCommandApproval(ctx, "thread-2", command.ID, DecisionDecline); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("cross-thread command error = %v", err)
	}
	if err := client.RespondFileApproval(ctx, "thread-1", command.ID, DecisionDecline); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("cross-kind approval error = %v", err)
	}
	if err := client.RespondCommandApproval(ctx, "thread-1", command.ID, DecisionDecline); err != nil {
		t.Fatal(err)
	}
	permission := <-client.Requests()
	if permission.ThreadID != "thread-1" || string(permission.Permissions) != `{"network":{"enabled":true}}` {
		t.Fatalf("permission request = %#v", permission)
	}
	if err := client.RespondPermissions(ctx, "thread-1", permission.ID, json.RawMessage(`{"fileSystem":{"write":["/private"]}}`), "session"); !errors.Is(err, ErrDecisionNotOffered) {
		t.Fatalf("permission escalation error = %v", err)
	}
	if err := client.RespondPermissions(ctx, "thread-1", permission.ID, json.RawMessage(`{"network":{"enabled":true}}`), "session"); err != nil {
		t.Fatal(err)
	}
}

func TestResponseWriteFailureIsUnknownAndTerminal(t *testing.T) {
	transport := &failingReadWriteCloser{read: strings.NewReader(""), failAfter: 7}
	client := NewClient(transport, transport, discardLogger(), Options{})
	client.mu.Lock()
	client.ready = true
	client.pendingRequests["\"approval-1\""] = pendingServerRequest{kind: requestKindCommand, threadID: "thread-1", allowed: map[ApprovalDecision]bool{DecisionDecline: true}}
	client.mu.Unlock()
	err := client.RespondCommandApproval(context.Background(), "thread-1", json.RawMessage(`"approval-1"`), DecisionDecline)
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) || !transport.Closed() {
		t.Fatalf("response failure = %T %v closed=%v", err, err, transport.Closed())
	}
	if err := client.RespondCommandApproval(context.Background(), "thread-1", json.RawMessage(`"approval-1"`), DecisionDecline); !errors.Is(err, ErrClosed) {
		t.Fatalf("reuse error = %v", err)
	}
}

func TestBlockedResponseWriteStopsAtContextDeadlineAsUnknown(t *testing.T) {
	writer := &blockingWriteCloser{closed: make(chan struct{})}
	client := NewClient(strings.NewReader(""), writer, discardLogger(), Options{})
	client.mu.Lock()
	client.ready = true
	client.pendingRequests[`"approval-1"`] = pendingServerRequest{kind: requestKindCommand, threadID: "thread-1", allowed: map[ApprovalDecision]bool{DecisionDecline: true}}
	client.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := client.RespondCommandApproval(ctx, "thread-1", json.RawMessage(`"approval-1"`), DecisionDecline)
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked response error = %T %v", err, err)
	}
	select {
	case <-writer.closed:
	case <-time.After(time.Second):
		t.Fatal("blocked writer was not closed")
	}
}

func TestQueuesHaveExplicitRetainedMemoryBudgets(t *testing.T) {
	if maxNotificationBytes*notificationQueueSize > 64*1024*1024 {
		t.Fatalf("notification queue budget = %d", maxNotificationBytes*notificationQueueSize)
	}
	if maxServerRequestBytes*serverRequestQueueSize > 2*1024*1024 {
		t.Fatalf("request queue budget = %d", maxServerRequestBytes*serverRequestQueueSize)
	}
}

func TestOutboundOptionsAreBoundedAndSchemaShapedBeforeWriting(t *testing.T) {
	client := NewClient(strings.NewReader(""), io.Discard, discardLogger(), Options{})
	ctx := context.Background()
	for _, run := range []func() error{
		func() error { _, err := client.ListThreads(ctx, ListOptions{Limit: -1}); return err },
		func() error {
			_, err := client.StartThread(ctx, ThreadOptions{Model: strings.Repeat("x", 257)})
			return err
		},
		func() error {
			_, err := client.StartThread(ctx, ThreadOptions{Sandbox: SandboxMode("future")})
			return err
		},
		func() error {
			_, err := client.StartTurn(ctx, TurnOptions{ThreadID: "thread-1", Text: "hello", SandboxPolicy: json.RawMessage(`{"broken":`)})
			return err
		},
	} {
		if err := run(); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("unsafe outbound input error = %v", err)
		}
	}
}

func TestCurrentPolicyUnionsAndHostAdvertisedEffortPassValidation(t *testing.T) {
	client := NewClient(strings.NewReader(""), io.Discard, discardLogger(), Options{})
	ctx := context.Background()
	for _, options := range []ThreadOptions{
		{Sandbox: SandboxReadOnly, ApprovalPolicy: json.RawMessage(`"on-request"`)},
		{Sandbox: SandboxWorkspaceWrite, ApprovalPolicy: json.RawMessage(`{"granular":{"mcp_elicitations":true,"rules":true,"sandbox_approval":true,"request_permissions":true}}`)},
	} {
		if _, err := client.StartThread(ctx, options); errors.Is(err, ErrInvalidInput) {
			t.Fatalf("valid thread policy rejected: %v", err)
		}
	}
	turn := TurnOptions{ThreadID: "thread-1", Text: "hello", Effort: "host-advertised-future-effort", ApprovalPolicy: json.RawMessage(`"untrusted"`), SandboxPolicy: json.RawMessage(`{"type":"readOnly","networkAccess":false}`)}
	if _, err := client.StartTurn(ctx, turn); errors.Is(err, ErrInvalidInput) {
		t.Fatalf("valid turn policy rejected: %v", err)
	}
}

func TestServerRequestsRequireEveryCurrentSchemaField(t *testing.T) {
	client := NewClient(strings.NewReader(""), io.Discard, discardLogger(), Options{ExperimentalQuestions: true})
	for index, test := range []struct{ method, params string }{
		{method: "item/commandExecution/requestApproval", params: `{"threadId":"thread-1","itemId":"item-1","startedAtMs":1}`},
		{method: "item/fileChange/requestApproval", params: `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1"}`},
		{method: "item/permissions/requestApproval", params: `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","startedAtMs":1,"permissions":{}}`},
		{method: "item/tool/requestUserInput", params: `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","questions":[{"id":"q","header":"","question":"Pick"}]}`},
		{method: "mcpServer/elicitation/request", params: `{"threadId":"thread-1"}`},
	} {
		message := wireMessage{ID: json.RawMessage(string(rune('1' + index))), Method: test.method, Params: json.RawMessage(test.params)}
		if request, err := client.registerServerRequest(message); err == nil {
			t.Fatalf("accepted invalid request %#v", request)
		}
	}
}

func TestMCPResponseRejectsContentUnlessAcceptedAndBoundsAcceptedContent(t *testing.T) {
	client := NewClient(strings.NewReader(""), io.Discard, discardLogger(), Options{})
	for _, test := range []struct {
		action  string
		content any
	}{
		{action: "decline", content: map[string]any{"value": "unexpected"}},
		{action: "cancel", content: "unexpected"},
		{action: "accept", content: strings.Repeat("x", 64*1024+1)},
	} {
		if err := client.RespondMCP(context.Background(), "thread-1", json.RawMessage(`"mcp-1"`), test.action, test.content); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("unsafe MCP response error = %v", err)
		}
	}
}

func TestTerminalFailureClosesTransportAndOutputChannels(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := NewClient(clientConn, clientConn, discardLogger(), Options{})
	client.mu.Lock()
	client.started = true
	client.mu.Unlock()
	go client.readLoop()
	_ = serverConn.Close()
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("done stayed open")
	}
	select {
	case _, ok := <-client.Events():
		if ok {
			t.Fatal("events stayed open")
		}
	case <-time.After(time.Second):
		t.Fatal("events did not close")
	}
	select {
	case _, ok := <-client.Requests():
		if ok {
			t.Fatal("requests stayed open")
		}
	case <-time.After(time.Second):
		t.Fatal("requests did not close")
	}
}

func serveApprovalRequests(conn net.Conn) {
	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	line, _ := reader.ReadBytes('\n')
	var initialize capturedRequest
	_ = json.Unmarshal(line, &initialize)
	_ = encoder.Encode(map[string]any{"id": json.RawMessage(initialize.ID), "result": fakeInitializeResult()})
	_, _ = reader.ReadBytes('\n')
	_ = encoder.Encode(map[string]any{"id": "command-1", "method": "item/commandExecution/requestApproval", "params": map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-1", "startedAtMs": 1, "availableDecisions": []string{"decline"},
	}})
	_, _ = reader.ReadBytes('\n')
	_ = encoder.Encode(map[string]any{"id": "permission-1", "method": "item/permissions/requestApproval", "params": map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-2", "startedAtMs": 1, "cwd": "/work", "permissions": map[string]any{"network": map[string]any{"enabled": true}},
	}})
	_, _ = reader.ReadBytes('\n')
}

type failingReadWriteCloser struct {
	mu        sync.Mutex
	read      io.Reader
	written   int
	failAfter int
	closed    bool
}

type blockingWriteCloser struct {
	once   sync.Once
	closed chan struct{}
}

type failOnSecondWriteConn struct {
	net.Conn
	mu     sync.Mutex
	writes int
}

func (conn *failOnSecondWriteConn) Write(value []byte) (int, error) {
	conn.mu.Lock()
	conn.writes++
	writeNumber := conn.writes
	conn.mu.Unlock()
	if writeNumber == 2 {
		if len(value) > 4 {
			return 4, io.ErrClosedPipe
		}
		return 0, io.ErrClosedPipe
	}
	return conn.Conn.Write(value)
}

func (writer *blockingWriteCloser) Write([]byte) (int, error) {
	<-writer.closed
	return 0, io.ErrClosedPipe
}
func (writer *blockingWriteCloser) Close() error {
	writer.once.Do(func() { close(writer.closed) })
	return nil
}

func (value *failingReadWriteCloser) Read(p []byte) (int, error) { return value.read.Read(p) }
func (value *failingReadWriteCloser) Write(p []byte) (int, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.written >= value.failAfter {
		return 0, io.ErrClosedPipe
	}
	remaining := value.failAfter - value.written
	if remaining < len(p) {
		value.written += remaining
		return remaining, io.ErrClosedPipe
	}
	value.written += len(p)
	return len(p), nil
}
func (value *failingReadWriteCloser) Close() error {
	value.mu.Lock()
	value.closed = true
	value.mu.Unlock()
	return nil
}
func (value *failingReadWriteCloser) Closed() bool {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.closed
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
