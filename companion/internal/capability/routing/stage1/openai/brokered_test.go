// Fact-force:
// 1) Callers: this test file; production = phoneruntime/runtime.go Open → NewBrokered;
//    mobilesession/handler.go errors.Is(ErrRouterNotProvisioned|ErrRouterUnreachable).
// 2) find companion -iname '*brokered*' → 0; rg NewBrokered|ErrRouterNotProvisioned → none.
// 3) No data files; httptest fakes /v1/broker/openai/{status,responses} JSON only.
// 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only"
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

func TestBrokeredModelSendsNoAuthorizationHeader(t *testing.T) {
	var sawAuth string
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		sawAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"read\",\"app_class\":\"beeper_messaging\",\"app_named\":\"instagram\",\"subject\":\"\",\"body\":\"\",\"confidence\":0.9}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	client, err := NewBrokered(server.URL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	raw, err := client.Model(context.Background(), "unread Instagram")
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	if sawAuth != "" {
		t.Fatalf("Authorization must be empty on brokered path, got %q", sawAuth)
	}
	if path != "/v1/broker/openai/responses" {
		t.Fatalf("path=%q, want /v1/broker/openai/responses", path)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if route.Verb != manifest.Read || route.AppClass != "beeper_messaging" || route.AppNamed != "instagram" {
		t.Fatalf("route=%+v", route)
	}
}

func TestBrokeredStatusNoKeySurfacesTypedFailClosedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/broker/openai/status":
			_, _ = io.WriteString(w, `{"keyed":false}`)
		case "/v1/broker/openai/responses":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"no_key"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewBrokered(server.URL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	keyed, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if keyed {
		t.Fatal("Status keyed=true, want false")
	}
	_, err = client.Model(context.Background(), "anything")
	if !errors.Is(err, ErrRouterNotProvisioned) {
		t.Fatalf("Model err=%v, want ErrRouterNotProvisioned", err)
	}
}

func TestBrokeredStatusKeyedReportsTrue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/broker/openai/status" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"keyed":true}`)
	}))
	defer server.Close()

	client, err := NewBrokered(server.URL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	keyed, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !keyed {
		t.Fatal("Status keyed=false, want true")
	}
}

func TestBrokeredUnreachableSurfacesTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := server.URL
	server.Close()

	client, err := NewBrokered(base, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	_, err = client.Model(context.Background(), "hi")
	if !errors.Is(err, ErrRouterUnreachable) {
		t.Fatalf("Model err=%v, want ErrRouterUnreachable", err)
	}
	_, err = client.Status(context.Background())
	if !errors.Is(err, ErrRouterUnreachable) {
		t.Fatalf("Status err=%v, want ErrRouterUnreachable", err)
	}
}

func TestBrokeredRoundTripUsesSameRouteSchemaAsDirectClient(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verb\":\"write\",\"app_class\":\"tasks\",\"app_named\":\"todoist\",\"subject\":\"Buy oat milk\",\"body\":\"\",\"confidence\":0.98}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	client, err := NewBrokered(server.URL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	if _, err := client.Model(context.Background(), "add buy oat milk"); err != nil {
		t.Fatalf("Model: %v", err)
	}
	text, _ := request["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	if format["name"] != "operator_route" || format["strict"] != true {
		t.Fatalf("format=%v", format)
	}
	if !strings.Contains(request["instructions"].(string), "Route the user's request.") {
		t.Fatalf("instructions missing coaching")
	}
}
