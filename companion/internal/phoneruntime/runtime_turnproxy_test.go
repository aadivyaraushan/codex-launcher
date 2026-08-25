package phoneruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/turnproxy"
)

// stubTurnSource satisfies TurnSource so tests can stand in for a connected
// gateway without any websocket. Closing done simulates the gateway
// dropping the connection.
type stubTurnSource struct {
	done chan struct{}
	// onDone, when set, runs on every Done() query. The connect goroutine
	// only queries Done() after installing the source, so tests can use it
	// as an "installed" signal.
	onDone func()
	// lastMessage, when set, rides on the task this stub reports — the
	// memory a redial is expected to carry into the next connection.
	lastMessage taskstate.LastMessage
	mu          sync.Mutex
	closed      bool
	triggered   []triggeredTurn
}

// triggeredTurn records one StartTriggeredTurn call so tests can assert what
// the Beeper watcher delivered.
type triggeredTurn struct {
	taskID  string
	prompt  string
	preview string
}

func newStubTurnSource() *stubTurnSource {
	return &stubTurnSource{done: make(chan struct{})}
}

func (s *stubTurnSource) Done() <-chan struct{} {
	if s.onDone != nil {
		s.onDone()
	}
	return s.done
}

func (s *stubTurnSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	return []taskstate.Task{{ID: "phone-agent", Title: "Phone agent", State: taskstate.IdleAfterReply, UpdatedAtUnix: 1, Source: taskstate.SourceAppServer, LastMessage: s.lastMessage}}, nil
}

func (s *stubTurnSource) CurrentTask(context.Context, string) (taskstate.Task, error) {
	return taskstate.Task{ID: "phone-agent", Title: "Phone agent", State: taskstate.IdleAfterReply, UpdatedAtUnix: 1, Source: taskstate.SourceAppServer, LastMessage: s.lastMessage}, nil
}

func (s *stubTurnSource) StartExistingTurn(context.Context, string, string) (taskadapter.ExistingTaskResult, error) {
	return taskadapter.ExistingTaskResult{ThreadID: "phone-agent", TurnID: "turn-1"}, nil
}

func (s *stubTurnSource) StartTriggeredTurn(_ context.Context, taskID, prompt, preview string) (taskadapter.ExistingTaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggered = append(s.triggered, triggeredTurn{taskID: taskID, prompt: prompt, preview: preview})
	return taskadapter.ExistingTaskResult{ThreadID: taskID, TurnID: "turn-trigger-1"}, nil
}

func (s *stubTurnSource) triggeredTurns() []triggeredTurn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]triggeredTurn(nil), s.triggered...)
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
	stub := newStubTurnSource()
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
	stub := newStubTurnSource()
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

// An event published for the phone agent right after connect must land even
// though the handler's task snapshot was seeded empty at Open (the deferred
// source had nothing to list yet). The desktop path refreshes the snapshot
// and retries on ErrUnknownTaskEvent (eventpump.go); the gateway path must
// give its publisher the same guarantee or the first chat events of a run
// are silently dropped.
func TestGatewayPublishAfterConnectReachesTheHandler(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := newStubTurnSource()
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

	mu.Lock()
	publisher := seen[0].Publisher
	mu.Unlock()
	event := taskstate.MobileEvent{
		TaskID:     "phone-agent",
		Kind:       "activity",
		State:      taskstate.Working,
		Summary:    "Codex is working",
		StartsTurn: true,
	}
	if err := publisher.PublishTaskEvent(context.Background(), event); err != nil {
		t.Fatalf("an event for the phone agent's task was dropped after connect: %v", err)
	}
}

// A dropped gateway connection must flip TaskCapable off and be redialed;
// a runtime that stays "task capable" on a dead socket is lying to the
// phone until the whole process restarts.
func TestGatewayReconnectsAfterDrop(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	first := newStubTurnSource()
	second := newStubTurnSource()
	// The redial blocks until the test has observed the incapable state, so
	// the "TaskCapable must go false" window cannot be raced away by an
	// instant reconnect.
	release := make(chan struct{})
	var mu sync.Mutex
	attempts := 0
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(ctx context.Context, _ turnproxy.Config) (TurnSource, error) {
			mu.Lock()
			attempts++
			attempt := attempts
			mu.Unlock()
			if attempt == 1 {
				return first, nil
			}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return second, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer runtime.Close()
	waitForTaskCapable(t, runtime)

	close(first.done)

	deadline := time.Now().Add(5 * time.Second)
	sawIncapable := false
	for time.Now().Before(deadline) {
		if !runtime.TaskCapable() {
			sawIncapable = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawIncapable {
		t.Fatal("TaskCapable stayed true on a dead gateway connection")
	}
	if !first.wasClosed() {
		t.Fatal("the dropped source must be closed before redialing")
	}
	close(release)

	waitForTaskCapable(t, runtime)
	mu.Lock()
	defer mu.Unlock()
	if attempts < 2 {
		t.Fatalf("connect attempts = %d, want a redial after the drop", attempts)
	}
}

// A redial builds a fresh Source, and a fresh Source remembers nothing —
// so without help, every gateway drop silently blanks the phone agent's
// last-message preview on Home until the next turn. The runtime is the
// only party who still holds the dropped connection; it must read the
// last message off that source before closing it and seed the next dial's
// config with it.
func TestRedialCarriesTheLastMessageForward(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	spoke := taskstate.LastMessage{From: taskstate.SpeakerAgent, Text: "Archived 41 conversations."}
	first := newStubTurnSource()
	first.lastMessage = spoke
	second := newStubTurnSource()
	var mu sync.Mutex
	attempts := 0
	var redialSeed taskstate.LastMessage
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(_ context.Context, config turnproxy.Config) (TurnSource, error) {
			mu.Lock()
			defer mu.Unlock()
			attempts++
			if attempts == 1 {
				return first, nil
			}
			redialSeed = config.InitialLastMessage
			return second, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer runtime.Close()
	waitForTaskCapable(t, runtime)

	close(first.done)

	deadline := time.Now().Add(5 * time.Second)
	redialed := false
	for time.Now().Before(deadline) {
		mu.Lock()
		redialed = attempts >= 2
		mu.Unlock()
		if redialed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !redialed {
		t.Fatal("the dropped connection was never redialed")
	}

	mu.Lock()
	defer mu.Unlock()
	if redialSeed != spoke {
		t.Fatalf("redial seed = %+v, want the dropped source's last message %+v", redialSeed, spoke)
	}
}

// A dial that resolves only after Close() has canceled it must not be
// installed: the runtime is already torn down, so the late source has to
// be closed instead of registered, and TaskCapable must stay false. The
// assertions run immediately after Close returns — Close must not come
// back while the connect goroutine can still touch the closed runtime.
func TestCloseWhileConnectInFlightDoesNotInstallTheLateSource(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := newStubTurnSource()
	entered := make(chan struct{})
	var once sync.Once
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(ctx context.Context, _ turnproxy.Config) (TurnSource, error) {
			once.Do(func() { close(entered) })
			// A dial whose transport already succeeded: cancellation arrives,
			// but the connected source still comes back rather than an error.
			<-ctx.Done()
			return stub, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	<-entered
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if !stub.wasClosed() {
		t.Fatal("a source that connected after Close was left open — leaked socket on a closed runtime")
	}
	if runtime.TaskCapable() {
		t.Fatal("TaskCapable reports true on a closed runtime")
	}
}

// Open's own error paths must tear down like Close does: when the connect
// goroutine installs a source and a later Open step then fails, the failed
// Open must close that source before returning — otherwise it leaks a live
// gateway socket (and its read goroutine) that no caller can ever reach.
func TestOpenFailureAfterConnectClosesTheInstalledSource(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	// A directory squatting on the cert path makes the cert-export step —
	// the last of Open's early-error paths, after the connect goroutine has
	// launched — fail deterministically.
	if err := os.Mkdir(filepath.Join(root, "agentbridge-cert.pem"), 0o755); err != nil {
		t.Fatalf("mkdir cert path: %v", err)
	}

	stub := newStubTurnSource()
	installed := make(chan struct{})
	var once sync.Once
	stub.onDone = func() { once.Do(func() { close(installed) }) }
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		// Open's own single Now call sits at the TLS-certificate step,
		// between launching the connect goroutine and the failing cert
		// export. Session-handler construction also calls Now, earlier,
		// before the goroutine exists — so gate on the direct caller and
		// block only Open's call until the goroutine has installed the
		// source. That pins the interleaving under test: install first,
		// then the Open failure.
		Now: func() time.Time {
			if pc, _, _, ok := stdruntime.Caller(1); ok &&
				strings.HasSuffix(stdruntime.FuncForPC(pc).Name(), "phoneruntime.Open") {
				<-installed
			}
			return time.Now()
		},
		TurnProxyConnect: func(context.Context, turnproxy.Config) (TurnSource, error) {
			return stub, nil
		},
	})
	if err == nil {
		runtime.Close()
		t.Fatal("Open must fail when the cert path is unwritable")
	}
	if !stub.wasClosed() {
		t.Fatal("a source installed during a failed Open was left open — leaked socket nothing can reach")
	}
}

func TestCloseClosesTheTurnSource(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := newStubTurnSource()
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
