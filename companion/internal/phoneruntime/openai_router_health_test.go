// Fact-force:
// 1) Importers/callers: go test ./companion/internal/phoneruntime -run HealthRouter;
//    production Health() in runtime.go for /v1/health.
// 2) Affected API: Health.Router = "openai_broker" | "openai_broker:no_key"
//    (explicit_app deleted from phone-runtime).
// 3) No durable data files; httptest returns {"keyed":true|false} only.
// 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only
//    (survive timeouts by shipping a durable checkpoint)."
package phoneruntime_test

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
)

func TestHealthRouterReportsOpenAIBrokerWhenKeyed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/broker/openai/status" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"keyed":true}`)
	}))
	defer server.Close()
	t.Setenv("ANDROID_OPENAI_BROKER_URL", server.URL)

	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	if got := rt.Health().Router; got != "openai_broker" {
		t.Fatalf("Router=%q, want openai_broker", got)
	}
}

func TestHealthRouterReportsNoKeyWhenVaultEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"keyed":false}`)
	}))
	defer server.Close()
	t.Setenv("ANDROID_OPENAI_BROKER_URL", server.URL)

	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	if got := rt.Health().Router; got != "openai_broker:no_key" {
		t.Fatalf("Router=%q, want openai_broker:no_key", got)
	}
}
