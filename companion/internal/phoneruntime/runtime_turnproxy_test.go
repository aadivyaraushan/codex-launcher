package phoneruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/turnproxy"
)

// stubTurnSource satisfies TurnSource so tests can stand in for a connected
// gateway without any websocket.
type stubTurnSource struct {
	mu     sync.Mutex
	closed bool
}

func (s *stubTurnSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	return []taskstate.Task{{ID: "phone-agent", Title: "Phone agent", State: taskstate.IdleAfterReply, UpdatedAtUnix: 1, Source: taskstate.SourceAppServer}}, nil
}

func (s *stubTurnSource) CurrentTask(context.Context, string) (taskstate.Task, error) {
	return taskstate.Task{ID: "phone-agent", Title: "Phone agent", State: taskstate.IdleAfterReply, UpdatedAtUnix: 1, Source: taskstate.SourceAppServer}, nil
}

func (s *stubTurnSource) StartExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{ThreadID: "phone-agent", TurnID: "turn-1"}, nil
}

func (s *stubTurnSource) RedirectExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{ThreadID: "phone-agent", TurnID: "turn-1"}, nil
}

func (s *stubTurnSource) InterruptExistingTurn(context.Context, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{ThreadID: "phone-agent", TurnID: "turn-1"}, nil
}

func (s *stubTurnSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *stubTurnSource) wasClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func writeGatewayToken(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "gateway-token")
	if err := os.WriteFile(path, []byte("tok-123\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	return path
}

func gatewayConfig(root, tokenPath string) Config {
	return Config{
		Root:             root,
		DisplayName:      "Operator phone",
		ListenAddress:    "127.0.0.1:0",
		GatewayURL:       "ws://127.0.0.1:18789",
		GatewayTokenPath: tokenPath,
	}
}

func waitForTaskCapable(t *testing.T, runtime *Runtime) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.TaskCapable() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime never became task capable")
}

func TestGatewayConfigRequiresBothFields(t *testing.T) {
	root := t.TempDir()
	config := Config{Root: root, DisplayName: "Operator phone", ListenAddress: "127.0.0.1:0", GatewayURL: "ws://127.0.0.1:18789"}
	if _, err := Open(context.Background(), config, Dependencies{Random: rand.Reader}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("gateway URL without a token path opened anyway: %v", err)
	}
	config = Config{Root: root, DisplayName: "Operator phone", ListenAddress: "127.0.0.1:0", GatewayTokenPath: filepath.Join(root, "gateway-token")}
	if _, err := Open(context.Background(), config, Dependencies{Random: rand.Reader}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("token path without a gateway URL opened anyway: %v", err)
	}
}

func TestOpenWithGatewayBecomesTaskCapable(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := &stubTurnSource{}
	var mu sync.Mutex
	var seen []turnproxy.Config
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(_ context.Context, config turnproxy.Config) (TurnSource, error) {
			mu.Lock()
			seen = append(seen, config)
			mu.Unlock()
			return stub, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer runtime.Close()

	waitForTaskCapable(t, runtime)
	if !runtime.Health().TaskCapable {
		t.Fatal("health must report the runtime task capable once the gateway is connected")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("connect was never called")
	}
	config := seen[0]
	if config.URL != "ws://127.0.0.1:18789" {
		t.Fatalf("connect URL = %q, want the configured gateway", config.URL)
	}
	// The runtime config carries a token PATH; the value is read from disk
	// only at connect time and must arrive trimmed.
	if config.Token != "tok-123" {
		t.Fatalf("connect token = %q, want the trimmed file contents", config.Token)
	}
	if config.TaskID != "phone-agent" {
		t.Fatalf("task id = %q, want the agent's fixed thread", config.TaskID)
	}
	if config.SessionKey != "agent:main:main" {
		t.Fatalf("session key = %q, want the gateway's default main session", config.SessionKey)
	}
	if config.Publisher == nil {
		t.Fatal("the source must publish stream events into the runtime's handler")
	}
}

func TestGatewayConnectRetriesUntilSuccess(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := &stubTurnSource{}
	var mu sync.Mutex
	attempts := 0
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(context.Context, turnproxy.Config) (TurnSource, error) {
			mu.Lock()
			defer mu.Unlock()
			attempts++
			if attempts < 3 {
				return nil, errors.New("gateway not up yet")
			}
			return stub, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer runtime.Close()

	waitForTaskCapable(t, runtime)
	mu.Lock()
	defer mu.Unlock()
	if attempts < 3 {
		t.Fatalf("connect attempts = %d, want at least 3 (two failures then success)", attempts)
	}
}

func TestCloseClosesTheTurnSource(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := &stubTurnSource{}
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(context.Context, turnproxy.Config) (TurnSource, error) {
			return stub, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	waitForTaskCapable(t, runtime)
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !stub.wasClosed() {
		t.Fatal("closing the runtime must close the gateway connection too")
	}
}
