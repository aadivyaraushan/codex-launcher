package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
)

func TestRealReadOnlyAppServerCompatibility(t *testing.T) {
	if os.Getenv("CODEX_APPSERVER_LIVE") != "1" {
		t.Skip("set CODEX_APPSERVER_LIVE=1 to run the installed read-only check")
	}
	command := exec.Command("codex", "app-server", "--stdio")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = command.Process.Kill(); _, _ = command.Process.Wait() })
	client := NewClient(stdout, stdin, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{ExperimentalQuestions: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := client.ListThreads(ctx, ListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Data []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(result, &page) != nil || page.Data == nil {
		t.Fatalf("unexpected thread/list result: %s", result)
	}
	threadID := os.Getenv("CODEX_APPSERVER_THREAD_ID")
	if threadID == "" && len(page.Data) > 0 {
		var listed struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(page.Data[0], &listed) != nil {
			t.Fatal("latest thread has an invalid shape")
		}
		threadID = listed.ID
	}
	if threadID == "" {
		t.Skip("the installed app-server has no task to read")
	}
	readResult, err := client.ReadThread(ctx, threadID, true)
	if err != nil {
		t.Fatal(err)
	}
	var read struct {
		Thread json.RawMessage `json:"thread"`
	}
	if json.Unmarshal(readResult, &read) != nil || len(read.Thread) == 0 {
		t.Fatal("thread/read result contains no thread")
	}
	var turnShape struct {
		Turns []struct {
			Status string `json:"status"`
		} `json:"turns"`
	}
	if json.Unmarshal(read.Thread, &turnShape) != nil {
		t.Fatal("thread/read turns have an invalid shape")
	}
	task, err := taskstate.MapAppServerThread(read.Thread)
	if err != nil || task.ID != threadID {
		t.Fatalf("thread/read task mapping failed: id_match=%v error=%v", task.ID == threadID, err)
	}
	statuses := make([]string, len(turnShape.Turns))
	for index, turn := range turnShape.Turns {
		statuses[index] = turn.Status
	}
	t.Logf("thread/read task state=%s has_active_turn=%v turn_statuses=%v", task.State, task.ActiveTurnID != "", statuses)
}

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
	if err := client.RespondCommandApproval(ctx, "thread-secret", request.ID, DecisionDecline); err != nil {
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
			return client.RespondCommandApproval(ctx, "thread-1", json.RawMessage(`"request-1"`), ApprovalDecision("always"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.run() == nil {
				t.Fatal("unsafe input was accepted")
			}
		})
	}
}

func TestTurnInputUsesCurrentCodexLocalImageAndMentionShapes(t *testing.T) {
	inputs, err := turnInput("inspect these", []AttachmentInput{
		{ID: "image-1", Path: "/private/attachment-1", MediaType: "image/png"},
		{ID: "document-1", Path: "/private/attachment-2", MediaType: "application/pdf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"type":"text","text":"inspect these"},{"type":"localImage","path":"/private/attachment-1"},{"type":"mention","name":"document-1","path":"/private/attachment-2"}]`
	if string(encoded) != want {
		t.Fatalf("turn input = %s, want %s", encoded, want)
	}
	for _, invalid := range [][]AttachmentInput{
		{{ID: "bad id!", Path: "/private/file", MediaType: "image/png"}},
		{{ID: "image-1", Path: "relative/file", MediaType: "image/png"}},
		{{ID: "image-1", Path: "/private/file", MediaType: ""}},
		{{ID: "same", Path: "/private/one", MediaType: "image/png"}, {ID: "same", Path: "/private/two", MediaType: "image/png"}},
	} {
		if _, err := turnInput("inspect", invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid attachments %#v error = %v", invalid, err)
		}
	}
}

func TestThreadNotLoadedClassificationIsExactAndSurvivesWrapping(t *testing.T) {
	threadID := "00000000-0000-4000-8000-000000000000"
	err := fmt.Errorf("read task: %w", &rpcError{Code: -32600, Message: "thread not loaded: " + threadID})
	if !IsThreadNotLoaded(err, threadID) || IsThreadNotLoaded(&rpcError{Code: -32600, Message: "internal failure"}, threadID) || IsThreadNotLoaded(context.DeadlineExceeded, threadID) {
		t.Fatal("thread-not-loaded classification was not exact")
	}
}

func TestMutatingTimeoutIsOutcomeUnknownAndClosesConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	go func() {
		reader := bufio.NewReader(serverConn)
		encoder := json.NewEncoder(serverConn)
		line, _ := reader.ReadBytes('\n')
		var initialize capturedRequest
		_ = json.Unmarshal(line, &initialize)
		_ = encoder.Encode(map[string]any{"id": json.RawMessage(initialize.ID), "result": fakeInitializeResult()})
		_, _ = reader.ReadBytes('\n')
		_, _ = reader.ReadBytes('\n')
		<-time.After(time.Second)
	}()
	client := NewClient(clientConn, clientConn, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	short, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err := client.StartTurn(short, TurnOptions{ThreadID: "thread-1", Text: "hello"})
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) || unknown.Method != "turn/start" {
		t.Fatalf("error = %T %v", err, err)
	}
	if _, err := client.ListModels(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("post-timeout error = %v", err)
	}
}

func TestUnknownServerRequestFailsClosed(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	sendUnknown := make(chan struct{})
	go func() {
		reader := bufio.NewReader(serverConn)
		encoder := json.NewEncoder(serverConn)
		line, _ := reader.ReadBytes('\n')
		var initialize capturedRequest
		_ = json.Unmarshal(line, &initialize)
		_ = encoder.Encode(map[string]any{"id": json.RawMessage(initialize.ID), "result": fakeInitializeResult()})
		_, _ = reader.ReadBytes('\n')
		<-sendUnknown
		_ = encoder.Encode(map[string]any{"id": "unknown-1", "method": "future/dangerous/request", "params": map[string]any{}})
	}()
	client := NewClient(clientConn, clientConn, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	close(sendUnknown)
	select {
	case <-client.done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := client.ListModels(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("error = %v", err)
	}
}

func TestPermissionResponseContainsOnlyGrantedSubsetAndScope(t *testing.T) {
	var output strings.Builder
	client := NewClient(strings.NewReader(""), &output, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})
	client.mu.Lock()
	client.ready = true
	client.pendingRequests[`"permission-1"`] = pendingServerRequest{kind: requestKindPermission, threadID: "thread-1", permissions: json.RawMessage(`{"network":{"enabled":true}}`)}
	client.mu.Unlock()
	granted := json.RawMessage(`{"network":{"enabled":true}}`)
	if err := client.RespondPermissions(context.Background(), "thread-1", json.RawMessage(`"permission-1"`), granted, "session"); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(output.String()), &response); err != nil {
		t.Fatal(err)
	}
	if string(response.Result["permissions"]) != string(granted) || string(response.Result["scope"]) != `"session"` {
		t.Fatalf("result = %s", output.String())
	}
}

func TestBoundedReaderRejectsBeforeUnboundedLineAllocation(t *testing.T) {
	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 40)+"\n"), 8)
	if _, err := readBoundedLine(reader, 32); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestWireDecoderRejectsTrailingAndAmbiguousMessages(t *testing.T) {
	for _, frame := range [][]byte{
		[]byte(`{"id":1,"result":{}} {}`),
		[]byte(`{"id":1,"method":"turn/completed","result":{},"params":{}}`),
		[]byte(`{"id":true,"result":{}}`),
		[]byte(`{"id":1}`),
	} {
		if _, err := decodeWire(frame); err == nil {
			t.Fatalf("accepted invalid frame: %s", frame)
		}
	}
}

func TestWritesFailAfterConnectionCloses(t *testing.T) {
	var output strings.Builder
	client := NewClient(strings.NewReader(""), &output, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})
	client.fail(io.EOF)
	if err := client.RespondCommandApproval(context.Background(), "thread-1", json.RawMessage(`"approval-1"`), DecisionDecline); !errors.Is(err, ErrClosed) {
		t.Fatalf("error = %v", err)
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
			result := any(map[string]any{"ok": true})
			if request.Method == "initialize" {
				result = fakeInitializeResult()
			}
			_ = encoder.Encode(map[string]any{"id": json.RawMessage(request.ID), "result": result})
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
			_ = encoder.Encode(map[string]any{"id": json.RawMessage(request.ID), "result": fakeInitializeResult()})
		}
		if request.Method == "thread/read" {
			_ = encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread-secret", "turn": map[string]any{"status": "completed"}}})
			_ = encoder.Encode(map[string]any{"id": "approval-1", "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "thread-secret", "turnId": "turn-1", "itemId": "item-1", "startedAtMs": 1, "command": "do not log this command"}})
			_ = encoder.Encode(map[string]any{"id": json.RawMessage(request.ID), "result": map[string]any{"thread": map[string]any{"id": "thread-secret"}}})
			_, _ = reader.ReadBytes('\n')
		}
	}
}

func fakeInitializeResult() map[string]any {
	return map[string]any{"codexHome": "/tmp/codex", "platformFamily": "unix", "platformOs": "macos", "userAgent": "fake"}
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

type captureLogHandler struct {
	mu      sync.Mutex
	entries []string
}

func (handler *captureLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (handler *captureLogHandler) Handle(_ context.Context, record slog.Record) error {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	handler.entries = append(handler.entries, record.Message)
	record.Attrs(func(attr slog.Attr) bool {
		handler.entries = append(handler.entries, attr.Value.String())
		return true
	})
	return nil
}
func (handler *captureLogHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }
func (handler *captureLogHandler) WithGroup(string) slog.Handler      { return handler }
func (handler *captureLogHandler) Contains(value string) bool {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	for _, entry := range handler.entries {
		if entry == value {
			return true
		}
	}
	return false
}
