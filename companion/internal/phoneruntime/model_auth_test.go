package phoneruntime_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/device"
)

func TestHealthHTTPDoesNotBlockWhenListHangs(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			return nil, errors.New("list still hung")
		},
	})
	defer close(release)
	defer rt.Close()

	client, base := serveRuntime(t, rt)
	client.Timeout = 400 * time.Millisecond

	start := time.Now()
	resp, err := client.Get(base + "/v1/health")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GET /v1/health blocked on hung list: %v (elapsed %s)", err, elapsed)
	}
	defer resp.Body.Close()
	if elapsed > 300*time.Millisecond {
		t.Fatalf("GET /v1/health took %s, want immediate cached modelAuth", elapsed)
	}
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(payload["modelAuth"]) != `"missing"` {
		t.Fatalf("modelAuth = %s, want missing until first successful list", payload["modelAuth"])
	}
	raw, _ := json.Marshal(payload)
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"sk-", "access_token", "refresh_token"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("hung-list health leaked %q: %s", banned, raw)
		}
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background list never started")
	}
}

func TestHealthKeepsOauthReadyWhenLaterListTimesOut(t *testing.T) {
	var mu sync.Mutex
	ready := []modelauth.Profile{
		{ID: "openai:default", Type: "oauth", Provider: "openai"},
	}
	var listErr error
	failed := make(chan struct{}, 1)
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			mu.Lock()
			profiles := append([]modelauth.Profile(nil), ready...)
			err := listErr
			mu.Unlock()
			if err != nil {
				select {
				case failed <- struct{}{}:
				default:
				}
			}
			return profiles, err
		},
	})
	defer rt.Close()
	waitModelAuth(t, rt, "oauth_ready")

	mu.Lock()
	ready = nil
	listErr = context.DeadlineExceeded
	mu.Unlock()

	deadline := time.Now().Add(2 * time.Second)
	sawFail := false
	for time.Now().Before(deadline) {
		_ = rt.Health()
		select {
		case <-failed:
			sawFail = true
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if sawFail {
			break
		}
	}
	if !sawFail {
		t.Fatal("list never returned a timeout after oauth_ready")
	}
	for i := 0; i < 10; i++ {
		if rt.Health().ModelAuth != "oauth_ready" {
			t.Fatalf("ModelAuth = %q, want oauth_ready kept across list timeout", rt.Health().ModelAuth)
		}
	}
}

func TestHealthModelAuthMissingByDefault(t *testing.T) {
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			return nil, nil
		},
	})
	defer rt.Close()
	if rt.Health().ModelAuth != "missing" {
		t.Fatalf("ModelAuth = %q, want missing", rt.Health().ModelAuth)
	}
}

func TestHealthModelAuthOauthReadyFromListTypeNotKeyed(t *testing.T) {
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			return []modelauth.Profile{
				{ID: "openai:manual", Type: "api_key", Provider: "openai"},
				{ID: "openai:default", Type: "oauth", Provider: "openai"},
			}, nil
		},
	})
	defer rt.Close()
	waitModelAuth(t, rt, "oauth_ready")
}

func TestHealthModelAuthApiKeyOnlyStaysMissing(t *testing.T) {
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			return []modelauth.Profile{
				{ID: "openai:manual", Type: "api_key", Provider: "openai"},
			}, nil
		},
	})
	defer rt.Close()
	if rt.Health().ModelAuth != "missing" {
		t.Fatalf("api_key ModelAuth = %q, want missing", rt.Health().ModelAuth)
	}
}

func TestHealthHTTPIncludesModelAuthWithoutSecrets(t *testing.T) {
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			return []modelauth.Profile{
				{ID: "openai:default", Type: "oauth", Provider: "openai"},
			}, nil
		},
	})
	defer rt.Close()
	waitModelAuth(t, rt, "oauth_ready")
	client, base := serveRuntime(t, rt)
	resp, err := client.Get(base + "/v1/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(payload["modelAuth"]) != `"oauth_ready"` {
		t.Fatalf("modelAuth = %s, want oauth_ready", payload["modelAuth"])
	}
	raw, _ := json.Marshal(payload)
	lower := strings.ToLower(string(raw))
	for _, banned := range []string{"sk-", "access_token", "refresh_token"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("health leaked %q: %s", banned, raw)
		}
	}
}

func TestModelAuthStartReturnsOnlyUserCodeAndVerificationUrl(t *testing.T) {
	dirty := `
https://auth.openai.com/codex/device
Code: AB12-CD34
access_token=sk-secret-login
refresh_token=rt-secret-login
`
	var logs bytes.Buffer
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{
		Random: rand.Reader,
		Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})),
		ModelAuth: phoneruntime.ModelAuthHooks{
			List: func(context.Context) ([]modelauth.Profile, error) {
				return nil, nil
			},
			StartLogin: func(context.Context) (device.Prompt, error) {
				prompt, ok := device.ParseOutput(dirty)
				if !ok {
					t.Fatal("fixture must parse")
				}
				return prompt, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	client, base := serveRuntime(t, rt)
	resp, err := client.Post(base+"/v1/model-auth/start", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("keys = %v, want only userCode and verificationUrl", keysOf(payload))
	}
	if string(payload["userCode"]) != `"AB12-CD34"` {
		t.Fatalf("userCode = %s", payload["userCode"])
	}
	if string(payload["verificationUrl"]) != `"https://auth.openai.com/codex/device"` {
		t.Fatalf("verificationUrl = %s", payload["verificationUrl"])
	}
	lower := strings.ToLower(logs.String())
	for _, banned := range []string{"sk-secret-login", "rt-secret-login", "access_token=", "refresh_token="} {
		if strings.Contains(lower, banned) {
			t.Fatalf("start logs leaked %q: %s", banned, logs.String())
		}
	}
	if rt.Health().ModelAuth != "pending" {
		t.Fatalf("after start ModelAuth = %q, want pending", rt.Health().ModelAuth)
	}
}

func TestHealthPendingWhenListHangsDuringDeviceLogin(t *testing.T) {
	release := make(chan struct{})
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			<-release
			return nil, errors.New("list still hung")
		},
		StartLogin: func(context.Context) (device.Prompt, error) {
			return device.Prompt{UserCode: "AB12-CD34", VerificationURL: "https://auth.openai.com/codex/device"}, nil
		},
	})
	defer close(release)
	defer rt.Close()

	client, base := serveRuntime(t, rt)
	client.Timeout = 400 * time.Millisecond
	resp, err := client.Post(base+"/v1/model-auth/start", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	start := time.Now()
	health, err := client.Get(base + "/v1/health")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GET /v1/health blocked during pending login: %v (elapsed %s)", err, elapsed)
	}
	defer health.Body.Close()
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(health.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(payload["modelAuth"]) != `"pending"` {
		t.Fatalf("modelAuth = %s, want pending", payload["modelAuth"])
	}
}

func TestHealthOauthReadyFromDiskStoreWithoutCLI(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("OPENCLAW_STATE_DIR", stateDir)
	agentDir := filepath.Join(stateDir, "agents", "main", "agent")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
		"version": 1,
		"profiles": {
			"openai:default": {
				"type": "oauth",
				"provider": "openai",
				"access": "sk-secret-access",
				"refresh": "rt-secret-refresh"
			}
		}
	}`)
	if err := os.WriteFile(filepath.Join(agentDir, "auth-profiles.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{
		Random: rand.Reader,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()
	if rt.Health().ModelAuth != "oauth_ready" {
		t.Fatalf("seeded ModelAuth = %q, want oauth_ready from disk without CLI", rt.Health().ModelAuth)
	}
	report, err := json.Marshal(rt.Health())
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(report))
	for _, banned := range []string{"sk-", "access_token", "refresh_token", "rt-secret"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("health leaked %q: %s", banned, report)
		}
	}
}

func TestModelAuthStartPrefersOauthOrderAfterReady(t *testing.T) {
	var order []string
	var restarted bool
	listed := []modelauth.Profile{
		{ID: "openai:manual", Type: "api_key", Provider: "openai"},
	}
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			return listed, nil
		},
		StartLogin: func(context.Context) (device.Prompt, error) {
			listed = []modelauth.Profile{
				{ID: "openai:manual", Type: "api_key", Provider: "openai"},
				{ID: "openai:default", Type: "oauth", Provider: "openai"},
			}
			return device.Prompt{UserCode: "AB12-CD34", VerificationURL: "https://auth.openai.com/codex/device"}, nil
		},
		SetAuthOrder: func(_ context.Context, ids []string) error {
			order = append([]string(nil), ids...)
			return nil
		},
		RestartGateway: func(context.Context) error {
			restarted = true
			return nil
		},
	})
	defer rt.Close()
	client, base := serveRuntime(t, rt)
	resp, err := client.Post(base+"/v1/model-auth/start", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rt.Health().ModelAuth == "oauth_ready" && len(order) > 0 && restarted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rt.Health().ModelAuth != "oauth_ready" {
		t.Fatalf("ModelAuth = %q, want oauth_ready after login", rt.Health().ModelAuth)
	}
	if len(order) == 0 || order[0] != "openai:default" {
		t.Fatalf("auth order = %v, want oauth id first", order)
	}
	if !restarted {
		t.Fatal("gateway was not restarted after oauth profile became ready")
	}
}

func TestHealthOauthReadyPrefersOrderWithoutStart(t *testing.T) {
	var order []string
	var restarted bool
	rt := openModelAuthRuntime(t, phoneruntime.ModelAuthHooks{
		List: func(context.Context) ([]modelauth.Profile, error) {
			return []modelauth.Profile{
				{ID: "openai:manual", Type: "api_key", Provider: "openai"},
				{ID: "openai:default", Type: "oauth", Provider: "openai"},
			}, nil
		},
		SetAuthOrder: func(_ context.Context, ids []string) error {
			order = append([]string(nil), ids...)
			return nil
		},
		RestartGateway: func(context.Context) error {
			restarted = true
			return nil
		},
	})
	defer rt.Close()
	waitModelAuth(t, rt, "oauth_ready")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(order) > 0 && restarted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(order) == 0 || order[0] != "openai:default" {
		t.Fatalf("auth order = %v, want oauth id first after health saw oauth_ready", order)
	}
	if !restarted {
		t.Fatal("gateway was not restarted when oauth was already present")
	}
}

func openModelAuthRuntime(t *testing.T, hooks phoneruntime.ModelAuthHooks) *phoneruntime.Runtime {
	t.Helper()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{
		Random:    rand.Reader,
		Logger:    slog.New(slog.DiscardHandler),
		ModelAuth: hooks,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return rt
}

func serveRuntime(t *testing.T, rt *phoneruntime.Runtime) (*http.Client, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = rt.Serve(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if addr := rt.BoundAddress(); addr != "" {
			return rt.TestHTTPClient(), "https://" + addr
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("runtime never bound")
	return nil, ""
}

func waitModelAuth(t *testing.T, rt *phoneruntime.Runtime, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		last = rt.Health().ModelAuth
		if last == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ModelAuth = %q, want %q", last, want)
}

func keysOf(payload map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	return keys
}
