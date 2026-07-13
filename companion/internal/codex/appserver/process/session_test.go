package process

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
)

func TestStartInitializesLocalStdioAndFeedsTheTaskCatalog(t *testing.T) {
	var commandArgs []string
	starter := testStarter(t, func(_ context.Context, _ string, args ...string) *exec.Cmd {
		commandArgs = append([]string(nil), args...)
		return helperCommand(t, "ready")
	})

	session, err := starter.start(context.Background(), Options{Binary: "/configured/codex"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !reflect.DeepEqual(commandArgs, []string{"app-server", "--stdio"}) {
		t.Fatalf("child arguments = %#v", commandArgs)
	}
	set, err := taskadapter.NewAppServerOnly(session.Client())
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := set.ListRecent(context.Background(), 1)
	if err != nil || len(tasks) != 1 || tasks[0].ID != "thread-1" || tasks[0].Title != "Build launcher" {
		t.Fatalf("catalog tasks = %#v, %v", tasks, err)
	}
}

func TestStartChecksTheConfiguredBinaryBeforeSpawning(t *testing.T) {
	spawned := false
	starter := testStarter(t, func(context.Context, string, ...string) *exec.Cmd {
		spawned = true
		return helperCommand(t, "ready")
	})
	starter.validate = func(context.Context, string) (string, error) { return "", errors.New("unsupported Codex") }

	if _, err := starter.start(context.Background(), Options{Binary: "/configured/codex"}); err == nil || spawned {
		t.Fatalf("start error = %v, spawned = %v", err, spawned)
	}
}

func TestInitializationFailureStopsAndReapsTheChild(t *testing.T) {
	var command *exec.Cmd
	starter := testStarter(t, func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		command = helperCommand(t, "invalid_initialize")
		return command
	})

	if _, err := starter.start(context.Background(), Options{Binary: "/configured/codex"}); err == nil {
		t.Fatal("invalid initialization was accepted")
	}
	if command == nil || command.ProcessState == nil {
		t.Fatal("failed child was not reaped")
	}
}

func TestCloseStopsTheOwnedChildAndIsRepeatable(t *testing.T) {
	starter := testStarter(t, func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return helperCommand(t, "ready")
	})
	session, err := starter.start(context.Background(), Options{Binary: "/configured/codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("session did not report shutdown")
	}
}

func TestContextCancellationReapsTheChildWithoutAnUnexpectedExitLog(t *testing.T) {
	var logs lockedBuffer
	ctx, cancel := context.WithCancel(context.Background())
	starter := testStarter(t, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return helperCommandContext(ctx, t, "ready")
	})
	starter.logger = slog.New(slog.NewTextHandler(&logs, nil))
	session, err := starter.start(ctx, Options{Binary: "/configured/codex"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("cancelled child was not reaped")
	}
	if session.command.ProcessState == nil {
		t.Fatal("cancelled child has no process state")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	<-session.Client().Done()
	if strings.Contains(logs.String(), "unexpected_exit") || !strings.Contains(logs.String(), "decision=context_cancelled") {
		t.Fatalf("cancellation logs = %s", logs.String())
	}
}

func TestUnexpectedCleanExitIsLogged(t *testing.T) {
	var logs lockedBuffer
	starter := testStarter(t, func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return helperCommand(t, "ready")
	})
	starter.logger = slog.New(slog.NewTextHandler(&logs, nil))
	session, err := starter.start(context.Background(), Options{Binary: "/configured/codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.stdin.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("clean child exit was not observed")
	}
	<-session.Client().Done()
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "branch_reason=unexpected_exit") || !strings.Contains(logs.String(), "error_class=clean_exit") {
		t.Fatalf("clean-exit logs = %s", logs.String())
	}
}

func TestLifecycleLogsExcludeConfiguredPathsAndTaskContent(t *testing.T) {
	var logs lockedBuffer
	starter := testStarter(t, func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return helperCommand(t, "ready")
	})
	starter.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	starter.validate = func(context.Context, string) (string, error) {
		return "codex-cli /private/version\nInjected-version-log", nil
	}
	session, err := starter.start(context.Background(), Options{Binary: "/configured/codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	<-session.Client().Done()
	for _, expected := range []string{"[codex-process] start requested", "binary_source=explicit", "[codex-process] initialized", "[codex-process] stopped"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs missing %q: %s", expected, logs.String())
		}
	}
	for _, private := range []string{"/configured/codex", "/private/version", "Injected-version-log", "Build launcher"} {
		if strings.Contains(logs.String(), private) {
			t.Fatalf("logs exposed %q: %s", private, logs.String())
		}
	}
}

func TestAppServerProcessHelper(t *testing.T) {
	if os.Getenv("CODEX_LAUNCHER_PROCESS_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		os.Exit(2)
	}
	var initialize map[string]any
	if json.Unmarshal(line, &initialize) != nil || initialize["method"] != "initialize" {
		os.Exit(3)
	}
	if mode == "invalid_initialize" {
		_ = encoder.Encode(map[string]any{"id": initialize["id"], "result": map[string]any{"bad": true}})
		for {
			if _, err := reader.ReadBytes('\n'); err != nil {
				os.Exit(0)
			}
		}
	}
	_ = encoder.Encode(map[string]any{
		"id": initialize["id"],
		"result": map[string]any{
			"codexHome": os.Getenv("CODEX_LAUNCHER_PROCESS_CODEX_HOME"), "platformFamily": "unix", "platformOs": "macos", "userAgent": "codex-test",
		},
	})
	for {
		line, err = reader.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		var message map[string]any
		if json.Unmarshal(line, &message) != nil {
			os.Exit(4)
		}
		if message["method"] != "thread/list" {
			continue
		}
		_ = encoder.Encode(map[string]any{
			"id": message["id"],
			"result": map[string]any{"data": []any{map[string]any{
				"id": "thread-1", "preview": "Build launcher", "cwd": "/work/launcher", "updatedAt": 42,
				"status": map[string]any{"type": "active", "activeFlags": []string{}},
				"turns":  []any{map[string]any{"status": "inProgress"}},
			}}},
		})
	}
}

func testStarter(t *testing.T, command func(context.Context, string, ...string) *exec.Cmd) starter {
	t.Helper()
	return starter{
		discover: func(binary string) (string, error) {
			if binary != "/configured/codex" {
				t.Fatalf("discovered binary input = %q", binary)
			}
			return binary, nil
		},
		validate: func(context.Context, string) (string, error) { return "codex-cli test", nil },
		command:  command,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "-test.run=TestAppServerProcessHelper", "--", mode)
	configureHelperCommand(t, command, mode)
	return command
}

func helperCommandContext(ctx context.Context, t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, "-test.run=TestAppServerProcessHelper", "--", mode)
	configureHelperCommand(t, command, mode)
	return command
}

func configureHelperCommand(t *testing.T, command *exec.Cmd, mode string) {
	t.Helper()
	command.Env = append(
		os.Environ(),
		"CODEX_LAUNCHER_PROCESS_HELPER=1",
		"CODEX_LAUNCHER_PROCESS_CODEX_HOME="+t.TempDir(),
	)
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (buffer *lockedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.Write(value)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.String()
}
