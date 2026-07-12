package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestClientInitializesThenUsesCurrentStableMethodsAndShapes(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	serverResult := make(chan []capturedRequest, 1)
	go captureStableSequence(serverConn, serverResult)

	client := NewClient(clientConn, clientConn, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{ExperimentalQuestions: true})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListThreads(ctx, ListOptions{Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadThread(ctx, "thread-1", true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartThread(ctx, ThreadOptions{CWD: "/work", Model: "model-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ResumeThread(ctx, "thread-1", ThreadOptions{CWD: "/work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ForkThread(ctx, "thread-1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetThreadName(ctx, "thread-1", "Launcher task"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListModels(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartTurn(ctx, TurnOptions{ThreadID: "thread-1", Text: "hello", CWD: "/work", Model: "model-1", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SteerTurn(ctx, "thread-1", "turn-1", "more"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.InterruptTurn(ctx, "thread-1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ArchiveThread(ctx, "thread-1"); err != nil {
		t.Fatal(err)
	}

	requests := <-serverResult
	wantMethods := []string{"initialize", "initialized", "thread/list", "thread/read", "thread/start", "thread/resume", "thread/fork", "thread/name/set", "model/list", "turn/start", "turn/steer", "turn/interrupt", "thread/archive"}
	gotMethods := make([]string, len(requests))
	for index := range requests {
		gotMethods[index] = requests[index].Method
	}
	if !reflect.DeepEqual(gotMethods, wantMethods) {
		t.Fatalf("methods = %#v", gotMethods)
	}
	assertJSONFields(t, requests[0].Params, "clientInfo", "capabilities")
	assertJSONFields(t, requests[1].Params)
	assertJSONFields(t, requests[3].Params, "threadId", "includeTurns")
	assertJSONFields(t, requests[9].Params, "threadId", "input", "cwd", "model", "effort")
	assertJSONFields(t, requests[10].Params, "threadId", "expectedTurnId", "input")
	assertJSONFields(t, requests[11].Params, "threadId", "turnId")
}

func TestReaderRoutesNotificationsServerRequestsAndResponsesWithoutContentLogs(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go serveInterleavedRead(serverConn)

	var logs captureLogHandler
	client := NewClient(clientConn, clientConn, slog.New(&logs), Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadThread(ctx, "thread-secret", true); err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-client.Events():
		if event.Method != "turn/completed" {
			t.Fatalf("event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var request ServerRequest
	select {
	case request = <-client.Requests():
		if request.Method != "item/commandExecution/requestApproval" {
			t.Fatalf("request = %#v", request)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := client.RespondApproval(request.ID, DecisionDecline); err != nil {
		t.Fatal(err)
	}
	if logs.Contains("do not log this command") {
		t.Fatal("diagnostic log leaked task content")
	}
}

func TestClientRejectsUnsafeInputsAndUnsupportedApprovalDecisions(t *testing.T) {
	client := NewClient(bufio.NewReader(nil), io.Discard, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})
	ctx := context.Background()
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{name: "blank thread", run: func() error { _, err := client.ReadThread(ctx, "", true); return err }},
		{name: "blank turn text", run: func() error { _, err := client.StartTurn(ctx, TurnOptions{ThreadID: "thread-1"}); return err }},
		{name: "bad approval", run: func() error {
			return client.RespondApproval(json.RawMessage(`"request-1"`), ApprovalDecision("always"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.run() == nil {
				t.Fatal("unsafe input was accepted")
			}
		})
	}
}

type capturedRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func captureStableSequence(conn net.Conn, result chan<- []capturedRequest) {
	defer close(result)
	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	requests := make([]capturedRequest, 0, 13)
	for index := 0; index < 13; index++ {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request capturedRequest
		if json.Unmarshal(line, &request) != nil {
			return
		}
		requests = append(requests, request)
		if len(request.ID) != 0 {
			_ = encoder.Encode(map[string]any{"id": json.RawMessage(request.ID), "result": map[string]any{"ok": true}})
		}
	}
	result <- requests
}

func serveInterleavedRead(conn net.Conn) {
	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	for index := 0; index < 3; index++ {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request capturedRequest
		if json.Unmarshal(line, &request) != nil {
			return
		}
		if request.Method == "initialize" {
			_ = encoder.Encode(map[string]any{"id": json.RawMessage(request.ID), "result": map[string]any{"userAgent": "fake"}})
		}
		if request.Method == "thread/read" {
			_ = encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread-secret", "turn": map[string]any{"status": "completed"}}})
			_ = encoder.Encode(map[string]any{"id": "approval-1", "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "thread-secret", "command": "do not log this command"}})
			_ = encoder.Encode(map[string]any{"id": json.RawMessage(request.ID), "result": map[string]any{"thread": map[string]any{"id": "thread-secret"}}})
		}
	}
}

func assertJSONFields(t *testing.T, raw json.RawMessage, fields ...string) {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != len(fields) {
		t.Fatalf("fields = %#v, want %#v", value, fields)
	}
	for _, field := range fields {
		if value[field] == nil {
			t.Fatalf("missing field %s in %s", field, raw)
		}
	}
}

type captureLogHandler struct{ entries []string }

func (handler *captureLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (handler *captureLogHandler) Handle(_ context.Context, record slog.Record) error {
	handler.entries = append(handler.entries, record.Message)
	return nil
}
func (handler *captureLogHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }
func (handler *captureLogHandler) WithGroup(string) slog.Handler      { return handler }
func (handler *captureLogHandler) Contains(value string) bool {
	for _, entry := range handler.entries {
		if entry == value {
			return true
		}
	}
	return false
}
