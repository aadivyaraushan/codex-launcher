package phoneruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperwatch"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/turnproxy"
)

func TestBeeperConfigRequiresAGateway(t *testing.T) {
	root := t.TempDir()
	config := Config{Root: root, DisplayName: "Operator phone", ListenAddress: "127.0.0.1:0", BeeperBaseURL: "http://127.0.0.1:23373"}
	if _, err := Open(context.Background(), config, Dependencies{Random: rand.Reader}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("a Beeper watcher with no gateway to deliver to opened anyway: %v", err)
	}
}

func TestABeeperMessageStartsATriggeredTurn(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	t.Setenv("BEEPER_ACCESS_TOKEN", "beeper-tok")
	stub := newStubTurnSource()

	config := gatewayConfig(root, tokenPath)
	config.BeeperBaseURL = "http://127.0.0.1:1"

	watchConfigs := make(chan beeperwatch.Config, 1)
	watchCtxDone := make(chan struct{})
	var watchOnce sync.Once
	runtime, err := Open(context.Background(), config, Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(context.Context, turnproxy.Config) (TurnSource, error) {
			return stub, nil
		},
		BeeperWatch: func(ctx context.Context, cfg beeperwatch.Config) {
			watchOnce.Do(func() { watchConfigs <- cfg })
			<-ctx.Done()
			close(watchCtxDone)
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer runtime.Close()
	waitForTaskCapable(t, runtime)

	var watchConfig beeperwatch.Config
	select {
	case watchConfig = <-watchConfigs:
	case <-time.After(5 * time.Second):
		t.Fatal("the Beeper watcher was never started")
	}
	if watchConfig.BaseURL != "http://127.0.0.1:1" {
		t.Fatalf("watcher base URL = %q, want the configured one", watchConfig.BaseURL)
	}
	if watchConfig.Token != "beeper-tok" {
		t.Fatalf("watcher token = %q, want the one from BEEPER_ACCESS_TOKEN", watchConfig.Token)
	}
	if watchConfig.Notify == nil {
		t.Fatal("the watcher must be given a Notify that reaches the agent session")
	}

	watchConfig.Notify(context.Background(), beeperwatch.Message{
		ID:         "msg-1",
		ChatID:     "chat-1",
		SenderName: "Maya",
		Text:       "you around tonight?",
		Timestamp:  "2026-08-12T09:00:00Z",
	})

	deadline := time.Now().Add(5 * time.Second)
	var turns []triggeredTurn
	for time.Now().Before(deadline) {
		turns = stub.triggeredTurns()
		if len(turns) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(turns) != 1 {
		t.Fatalf("triggered turns = %d, want exactly 1", len(turns))
	}
	turn := turns[0]
	if turn.taskID != "phone-agent" {
		t.Fatalf("trigger went to task %q, want the phone agent's thread", turn.taskID)
	}
	if !strings.Contains(turn.prompt, "you around tonight?") || !strings.Contains(turn.prompt, "Maya") {
		t.Fatalf("trigger prompt is missing the message:\n%s", turn.prompt)
	}
	if turn.preview != "New message from Maya" {
		t.Fatalf("preview = %q, want %q", turn.preview, "New message from Maya")
	}

	// The rules governing unprompted action ride inside the prompt, and the
	// default file is materialized where the owner can edit it.
	rulesOnDisk, err := os.ReadFile(filepath.Join(root, "agent-rules.md"))
	if err != nil {
		t.Fatalf("default rules file was not materialized in the runtime root: %v", err)
	}
	if !strings.Contains(turn.prompt, strings.TrimSpace(string(rulesOnDisk))) {
		t.Fatalf("trigger prompt does not carry the rules file contents:\n%s", turn.prompt)
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-watchCtxDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close must stop the Beeper watcher")
	}
}

func TestNoBeeperConfigMeansNoWatcher(t *testing.T) {
	previousDelay := turnProxyRetryDelay
	turnProxyRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { turnProxyRetryDelay = previousDelay })

	root := t.TempDir()
	tokenPath := writeGatewayToken(t, root)
	stub := newStubTurnSource()
	started := make(chan struct{}, 1)
	runtime, err := Open(context.Background(), gatewayConfig(root, tokenPath), Dependencies{
		Random: rand.Reader,
		TurnProxyConnect: func(context.Context, turnproxy.Config) (TurnSource, error) {
			return stub, nil
		},
		BeeperWatch: func(context.Context, beeperwatch.Config) {
			started <- struct{}{}
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer runtime.Close()
	waitForTaskCapable(t, runtime)

	select {
	case <-started:
		t.Fatal("no BeeperBaseURL configured, yet the watcher was started")
	case <-time.After(200 * time.Millisecond):
	}
}
