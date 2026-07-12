package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRealReadOnlyObservation(t *testing.T) {
	threadID := os.Getenv("CODEX_PROBE_THREAD_ID")
	if threadID == "" {
		t.Skip("set CODEX_PROBE_THREAD_ID to run the installed app-server probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	minimumEvents := 3
	if rawMinimum := os.Getenv("CODEX_PROBE_MIN_EVENTS"); rawMinimum != "" {
		parsed, parseErr := strconv.Atoi(rawMinimum)
		if parseErr != nil || parsed < 0 {
			t.Fatalf("invalid CODEX_PROBE_MIN_EVENTS %q", rawMinimum)
		}
		minimumEvents = parsed
	}
	var result Result
	var err error
	if os.Getenv("CODEX_PROBE_STRATEGY") == "proxy" {
		result, err = RunDaemonProxyEvents(ctx, os.Getenv("CODEX_PROBE_BINARY"), threadID, minimumEvents)
	} else {
		result, err = RunReadOnlyEvents(ctx, os.Getenv("CODEX_PROBE_BINARY"), threadID, minimumEvents)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("version=%s status=%s events=%v approvalDenied=%v", result.Version, result.Observation.ListedStatus, result.Observation.EventMethods, result.Observation.ApprovalDenied)
}

func TestDiscoverBinaryPrefersExplicitPath(t *testing.T) {
	lookupCalled := false
	got, err := discoverBinary("/Applications/ChatGPT.app/codex", func(string) (string, error) {
		lookupCalled = true
		return "/usr/local/bin/codex", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Applications/ChatGPT.app/codex" || lookupCalled {
		t.Fatalf("discoverBinary() = %q, lookupCalled=%v", got, lookupCalled)
	}
}

func TestDiscoverBinaryFallsBackToPath(t *testing.T) {
	got, err := discoverBinary("", func(name string) (string, error) {
		if name != "codex" {
			t.Fatalf("lookup name = %q", name)
		}
		return "/opt/homebrew/bin/codex", nil
	})
	if err != nil || got != "/opt/homebrew/bin/codex" {
		t.Fatalf("discoverBinary() = %q, %v", got, err)
	}
}

func TestAppServerArgsKeepDedicatedAndDaemonStrategiesDistinct(t *testing.T) {
	if got := appServerArgs(StrategyDedicated); !reflect.DeepEqual(got, []string{"app-server", "--stdio"}) {
		t.Fatalf("dedicated args = %#v", got)
	}
	if got := appServerArgs(StrategyDaemonProxy); !reflect.DeepEqual(got, []string{"app-server", "proxy"}) {
		t.Fatalf("proxy args = %#v", got)
	}
}

func TestReadOnlySequenceOrdersHandshakeObservationAndDecline(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	serverResult := make(chan fakeServerResult, 1)
	go runOrderedFakeServer(server, serverResult)

	observation, err := NewSession(client, client).readOnlySequence("thread-desktop", true)
	if err != nil {
		t.Fatal(err)
	}
	result := <-serverResult
	if result.err != nil {
		t.Fatal(result.err)
	}

	wantMethods := []string{"initialize", "initialized", "thread/list", "thread/read", "thread/resume"}
	if !reflect.DeepEqual(result.methods, wantMethods) {
		t.Fatalf("methods = %#v, want %#v", result.methods, wantMethods)
	}
	if result.approvalDecision != "decline" {
		t.Fatalf("approval decision = %q", result.approvalDecision)
	}
	if observation.ThreadID != "thread-desktop" || observation.ListedStatus != "active" || !reflect.DeepEqual(observation.EventMethods, []string{"turn/started", "item/commandExecution/requestApproval"}) {
		t.Fatalf("observation = %#v", observation)
	}
	if strings.Contains(strings.Join(result.methods, ","), "turn/start") {
		t.Fatal("read-only sequence must not start a model-backed turn")
	}
}

func TestReadOnlySequenceRejectsEventBeforeInitializeResponse(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	go func() {
		reader := bufio.NewReader(server)
		_, _ = reader.ReadBytes('\n')
		_ = json.NewEncoder(server).Encode(map[string]any{
			"method": "turn/started",
			"params": map[string]any{"threadId": "too-early"},
		})
	}()

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_, err := NewSession(client, client).ReadOnlySequence("thread-desktop")
	if err == nil || !strings.Contains(err.Error(), "initialize response") {
		t.Fatalf("error = %v", err)
	}
}

type fakeServerResult struct {
	methods          []string
	approvalDecision string
	err              error
}

func runOrderedFakeServer(conn net.Conn, result chan<- fakeServerResult) {
	defer close(result)
	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	seen := make([]string, 0, 5)

	read := func() (wireMessage, error) {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return wireMessage{}, err
		}
		var message wireMessage
		return message, json.Unmarshal(line, &message)
	}
	respond := func(id json.RawMessage, value any) error {
		return encoder.Encode(map[string]any{"id": json.RawMessage(id), "result": value})
	}

	for _, expected := range []string{"initialize", "initialized", "thread/list", "thread/read", "thread/resume"} {
		message, err := read()
		if err != nil {
			result <- fakeServerResult{err: err}
			return
		}
		seen = append(seen, message.Method)
		if message.Method != expected {
			result <- fakeServerResult{methods: seen, err: errors.New("unexpected method " + message.Method)}
			return
		}
		if expected != "initialized" {
			value := map[string]any{}
			switch expected {
			case "initialize":
				value = map[string]any{"userAgent": "fake", "platformFamily": "unix", "platformOs": "macos"}
			case "thread/list":
				_ = encoder.Encode(map[string]any{
					"method": "account/updated",
					"params": map[string]any{"authMode": "chatgpt"},
				})
				value = map[string]any{"data": []any{map[string]any{
					"id": "thread-desktop", "status": map[string]any{"type": "active", "activeFlags": []any{"waitingOnApproval"}},
				}}}
			case "thread/read", "thread/resume":
				value = map[string]any{"thread": map[string]any{"id": "thread-desktop"}}
			}
			if err := respond(message.ID, value); err != nil {
				result <- fakeServerResult{methods: seen, err: err}
				return
			}
		}
	}

	_ = encoder.Encode(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": "thread-desktop"}})
	_ = encoder.Encode(map[string]any{
		"id":     "approval-1",
		"method": "item/commandExecution/requestApproval",
		"params": map[string]any{"threadId": "thread-desktop"},
	})

	approval, err := read()
	if err != nil {
		result <- fakeServerResult{methods: seen, err: err}
		return
	}
	var response struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(approval.Result, &response); err != nil && !errors.Is(err, io.EOF) {
		result <- fakeServerResult{methods: seen, err: err}
		return
	}
	result <- fakeServerResult{methods: seen, approvalDecision: response.Decision}
}
