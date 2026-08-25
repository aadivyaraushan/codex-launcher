package phoneruntime_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
)

// Phase 2 proof: a tool call round-trips launcher HTTP → bridge → runner →
// credentialed adapter → (stub) Beeper Desktop API and back, with the
// bearer token minted on disk — the whole path the OpenClaw plugin will
// drive, minus the plugin.
func TestAgentToolsRoundTripThroughRunnerAndStubBroker(t *testing.T) {
	var brokerHits atomic.Int64
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		brokerHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/chats" || r.URL.Path == "/v1/chats/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id": "chat-1", "accountID": "instagramgo", "network": "Instagram",
					"title": "Maya", "type": "single", "unreadCount": 1,
					"lastActivity": "2026-08-06T12:00:00Z",
				}},
				"hasMore": false,
			})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id": "m1", "chatID": "chat-1", "senderName": "Maya",
					"timestamp": "2026-08-06T12:00:00Z", "text": "are you free?",
					"isSender": false, "isUnread": true,
				}},
				"hasMore": false,
			})
		case r.URL.Path == "/v1/accounts":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"accountID": "instagramgo", "network": "Instagram"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer stub.Close()
	t.Setenv("BEEPER_ACCESS_TOKEN", "stub-token")
	t.Setenv("BEEPER_DESKTOP_BASE_URL", stub.URL)

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
	var base string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if addr := rt.BoundAddress(); addr != "" {
			base = "https://" + addr
			client = rt.TestHTTPClient()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if client == nil {
		t.Fatal("runtime never bound")
	}

	rawToken, err := os.ReadFile(rt.AgentBridgeTokenPath())
	if err != nil {
		t.Fatalf("bridge token not minted on disk: %v", err)
	}
	token := strings.TrimSpace(string(rawToken))
	if token == "" {
		t.Fatal("minted bridge token is empty")
	}

	// Without the token the endpoints must refuse, even from loopback.
	resp, err := client.Get(base + "/v1/agent-tools/list")
	if err != nil {
		t.Fatalf("tokenless list: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tokenless list status = %d, want 401", resp.StatusCode)
	}

	listReq, _ := http.NewRequest(http.MethodGet, base+"/v1/agent-tools/list", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(listReq)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list agentbridge.ToolListResult
	err = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d decode err=%v", resp.StatusCode, err)
	}
	found := false
	for _, tool := range list.Tools {
		if tool.Name == "instagram" {
			found = true
		}
	}
	if !found {
		names := make([]string, 0, len(list.Tools))
		for _, tool := range list.Tools {
			names = append(names, tool.Name)
		}
		t.Fatalf("credentialed beeper adapter missing from tool list: %v", names)
	}

	body, _ := json.Marshal(agentbridge.ToolCallRequest{Adapter: "instagram", Verb: "read"})
	callReq, _ := http.NewRequest(http.MethodPost, base+"/v1/agent-tools/call", strings.NewReader(string(body)))
	callReq.Header.Set("Authorization", "Bearer "+token)
	callReq.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(callReq)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var result agentbridge.ToolCallResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("call status=%d decode err=%v result=%+v error=%+v", resp.StatusCode, err, result, result.Error)
	}
	if !result.OK {
		t.Fatalf("call failed: %+v", result)
	}
	if brokerHits.Load() == 0 {
		t.Fatal("call never reached the stub Beeper API — no broker round-trip")
	}
}
