package phoneruntime_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
)

// Workstream A step 5: phone-runtime's stage-1 router is the OpenAI broker
// client, never the keyword matcher. Health reports the router by probing the
// broker's status endpoint: keyed -> "openai_broker", no key ->
// "openai_broker:no_key", unreachable -> "openai_broker" (a per-ask failure,
// not a health-down state). "explicit_app" must never appear again.

func openRuntimeWithBroker(t *testing.T, brokerURL string) *phoneruntime.Runtime {
	t.Helper()
	t.Setenv("ANDROID_OPENAI_BROKER_URL", brokerURL)
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { rt.Close() })
	return rt
}

func brokerStatusServer(t *testing.T, keyed bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/broker/openai/status" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if keyed {
			_, _ = w.Write([]byte(`{"keyed":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"keyed":false}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHealthRouterIsOpenAiBrokerWhenBrokerHoldsAKey(t *testing.T) {
	srv := brokerStatusServer(t, true)
	rt := openRuntimeWithBroker(t, srv.URL)
	if got := rt.Health().Router; got != "openai_broker" {
		t.Fatalf("Router = %q, want openai_broker", got)
	}
}

func TestHealthRouterIsNoKeyWhenBrokerHasNoKey(t *testing.T) {
	srv := brokerStatusServer(t, false)
	rt := openRuntimeWithBroker(t, srv.URL)
	if got := rt.Health().Router; got != "openai_broker:no_key" {
		t.Fatalf("Router = %q, want openai_broker:no_key", got)
	}
}

func TestHealthRouterStaysOpenAiBrokerWhenBrokerUnreachable(t *testing.T) {
	// A server we close immediately: dialing it is a connection refused, the
	// "broker unreachable" row of the fail-closed table.
	srv := httptest.NewServer(http.NotFoundHandler())
	dead := srv.URL
	srv.Close()
	rt := openRuntimeWithBroker(t, dead)
	if got := rt.Health().Router; got != "openai_broker" {
		t.Fatalf("Router = %q, want openai_broker on an unreachable broker", got)
	}
}

func TestHealthRouterNeverReportsExplicitApp(t *testing.T) {
	srv := brokerStatusServer(t, true)
	rt := openRuntimeWithBroker(t, srv.URL)
	if got := rt.Health().Router; got == "explicit_app" {
		t.Fatalf("Router = %q, the keyword matcher must not be the phone router", got)
	}
}
