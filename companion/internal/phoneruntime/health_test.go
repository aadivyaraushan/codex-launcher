package phoneruntime_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
)

// Gate facts for this edit (existing file, not new):
// 1) Callers: `go test ./internal/phoneruntime -run TestHealth` exercises this file;
//    production callers of Health() are runtime.go Serve /v1/health and cmd/operator-phone-runtime.
// 2) Grep: health_test.go already owns Health JSON contract tests; no second beeper-health test file.
// 3) No data files; BeeperAccountStatus is in-memory {ID,Network,Status}.
// 4) User: "Wire Operator health beeper= if a valid path exists (Mac token or phone)."

func TestHealthReportsReadinessWithoutSecrets(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: phoneruntime.ListenAddress,
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	report := rt.Health()
	if report.Mode != "standalone_phone" {
		t.Fatalf("Mode = %q, want standalone_phone", report.Mode)
	}
	if report.Process != "ready" {
		t.Fatalf("Process = %q, want ready", report.Process)
	}
	if report.Router == "" {
		t.Fatal("Router status missing")
	}
	if report.ListenAddress != phoneruntime.ListenAddress {
		t.Fatalf("ListenAddress = %q, want %s", report.ListenAddress, phoneruntime.ListenAddress)
	}
	// Without a Beeper API client, deeplink Instagram/Discord/Messages must not
	// pretend Beeper is present-but-disconnected.
	if report.Beeper != "unavailable" {
		t.Fatalf("Beeper = %q, want unavailable when no Beeper client is wired", report.Beeper)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"sk-", "bearer ", "refresh_token", "client_secret", "api_key\":", "keychain", "macos"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("health payload leaked secret material containing %q: %s", banned, raw)
		}
	}
}

func TestHealthBeeperConnectedWhenAccountsProbeSucceeds(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{
		Random: rand.Reader,
		BeeperAccounts: func(context.Context) ([]phoneruntime.BeeperAccountStatus, error) {
			return []phoneruntime.BeeperAccountStatus{
				{ID: "instagramgo", Network: "Instagram", Status: "connected"},
				{ID: "discordgo", Network: "Discord", Status: "connected"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()
	if got := rt.Health().Beeper; got != "connected" {
		t.Fatalf("Beeper = %q, want connected", got)
	}
}

func TestHealthBeeperNotConnectedWhenAccountsProbeEmpty(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{
		Random: rand.Reader,
		BeeperAccounts: func(context.Context) ([]phoneruntime.BeeperAccountStatus, error) {
			return []phoneruntime.BeeperAccountStatus{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()
	if got := rt.Health().Beeper; got != "not_connected" {
		t.Fatalf("Beeper = %q, want not_connected", got)
	}
}

func TestServeFailsWhenPortAlreadyHeld(t *testing.T) {
	root := t.TempDir()
	first, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          filepath.Join(root, "a"),
		DisplayName:   "first",
		ListenAddress: phoneruntime.ListenAddress,
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	defer first.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- first.Serve(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if first.Health().Process == "serving" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if first.Health().Process != "serving" {
		t.Fatal("first runtime never reached serving")
	}

	second, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          filepath.Join(root, "b"),
		DisplayName:   "second",
		ListenAddress: phoneruntime.ListenAddress,
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer second.Close()

	serveErr := second.Serve(context.Background())
	if serveErr == nil {
		t.Fatal("second Serve succeeded; want listen conflict on fixed 9443")
	}
	if !strings.Contains(serveErr.Error(), "9443") && !strings.Contains(strings.ToLower(serveErr.Error()), "address already in use") {
		t.Fatalf("Serve error = %v, want fixed-port conflict", serveErr)
	}
	cancel()
	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("first Serve did not stop")
	}
}

func TestCapabilityOnlyWelcomeOmitsDesktopTasks(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	if rt.TaskCapable() {
		t.Fatal("phone runtime must not advertise desktop tasks")
	}
	if !rt.CapabilityCapable() {
		t.Fatal("phone runtime must expose capability actions")
	}
}

func TestHealthHTTPEndpoint(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = rt.Serve(ctx) }()

	var client *http.Client
	var healthURL string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr := rt.BoundAddress()
		if addr != "" {
			healthURL = "https://" + addr + "/v1/health"
			client = rt.TestHTTPClient()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if client == nil {
		t.Fatal("runtime never bound an address")
	}

	resp, err := client.Get(healthURL)
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var report phoneruntime.Health
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Mode != "standalone_phone" {
		t.Fatalf("Mode = %q", report.Mode)
	}
}
