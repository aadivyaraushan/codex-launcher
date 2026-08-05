package phoneruntime_test

// Callers: `go test ./companion/internal/phoneruntime` — exercises Runtime.MarkBrokerReady
// and POST /v1/credentials/broker-status handled in runtime.go Serve().
// Existing search: Grep android_broker_pending / broker-status — only runtime.go remap;
// no broker_status*.go yet.
// Data: broker-status.json under runtime Root — {"todoist":"ready"} string map, RFC3339 optional.
// User instruction: Fix android_broker_pending on phone-runtime health despite Todoist token in credential_broker — bridge/import so health reflects connected Todoist.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
)

func TestBrokerStatusMarksTodoistReadyInHealth(t *testing.T) {
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

	if err := rt.MarkBrokerReady("todoist"); err != nil {
		t.Fatalf("MarkBrokerReady: %v", err)
	}
	after := rt.Health().Credentials["todoist"]
	if after != "ready" {
		t.Fatalf("todoist after = %q, want ready", after)
	}

	rt.Close()
	rt2, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer rt2.Close()
	if rt2.Health().Credentials["todoist"] != "ready" {
		t.Fatalf("todoist after reopen = %q, want ready", rt2.Health().Credentials["todoist"])
	}
}

func TestBrokerStatusHTTPRejectsSecretsAndMarksReady(t *testing.T) {
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

	bad, _ := json.Marshal(map[string]string{
		"provider":     "todoist",
		"status":       "ready",
		"access_token": "should-not-be-accepted",
	})
	resp, err := client.Post(base+"/v1/credentials/broker-status", "application/json", bytes.NewReader(bad))
	if err != nil {
		t.Fatalf("bad post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected rejection when access_token present")
	}

	good, _ := json.Marshal(map[string]string{"provider": "todoist", "status": "ready"})
	resp, err = client.Post(base+"/v1/credentials/broker-status", "application/json", bytes.NewReader(good))
	if err != nil {
		t.Fatalf("good post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if rt.Health().Credentials["todoist"] != "ready" {
		t.Fatalf("health todoist = %q", rt.Health().Credentials["todoist"])
	}
}
