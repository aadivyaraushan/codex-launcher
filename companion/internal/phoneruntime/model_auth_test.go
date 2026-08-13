package phoneruntime_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/device"
)

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
	if rt.Health().ModelAuth != "oauth_ready" {
		t.Fatalf("ModelAuth = %q, want oauth_ready", rt.Health().ModelAuth)
	}
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

func keysOf(payload map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	return keys
}
