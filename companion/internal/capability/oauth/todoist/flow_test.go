package todoist

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func TestScopesAreDerivedFromTheVerbsAndNeverAskForDelete(t *testing.T) {
	cases := []struct {
		verbs []manifest.Verb
		want  string
	}{
		{[]manifest.Verb{manifest.Read}, "data:read"},
		{[]manifest.Verb{manifest.Write}, "data:read_write"},
		{[]manifest.Verb{manifest.Read, manifest.Write}, "data:read_write"},
	}
	for _, test := range cases {
		got, err := ScopesForVerbs(test.verbs)
		if err != nil || strings.Join(got, " ") != test.want {
			t.Fatalf("ScopesForVerbs(%v)=%v err=%v, want %q", test.verbs, got, err, test.want)
		}
		if strings.Contains(strings.Join(got, " "), "delete") {
			t.Fatalf("verbs %v asked for delete: %v", test.verbs, got)
		}
	}
}

func TestStartRegistersAPublicPKCEClientAndBuildsTheAuthorizeURL(t *testing.T) {
	var registered map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/register" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&registered); err != nil {
			t.Fatalf("decode registration: %v", err)
		}
		_, _ = io.WriteString(w, `{"client_id":"dynamic-client"}`)
	}))
	defer server.Close()

	flow := New(Config{
		APIBaseURL:  server.URL,
		AppBaseURL:  server.URL,
		ClientName:  "Operator",
		HTTPClient:  server.Client(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{7}, 128)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1:9191/oauth/todoist/callback", []manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if registered["token_endpoint_auth_method"] != "none" {
		t.Fatalf("registration auth method=%v, want none", registered["token_endpoint_auth_method"])
	}
	if strings.Contains(mustJSON(t, registered), "client_secret") {
		t.Fatalf("public registration included a client secret: %v", registered)
	}

	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	q := u.Query()
	if u.Path != "/oauth/authorize" || q.Get("client_id") != "dynamic-client" || q.Get("response_type") != "code" {
		t.Fatalf("authorize URL=%s", auth.URL)
	}
	if q.Get("state") == "" || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL lacks state or PKCE: %s", auth.URL)
	}
	if q.Get("scope") != "data:read_write" || auth.State != q.Get("state") {
		t.Fatalf("scope/state mismatch: auth=%+v query=%v", auth, q)
	}
}

func TestStartUsesSecureRandomnessWhenNoTestReaderIsSupplied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"client_id":"dynamic-client"}`)
	}))
	defer server.Close()

	flow := New(Config{
		APIBaseURL: server.URL, AppBaseURL: server.URL,
		HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start with production defaults: %v", err)
	}
	if auth.State == "" {
		t.Fatal("Start produced no state")
	}
}

func TestCallbackRejectsWrongAndReusedStateAndRotatesRefreshTokens(t *testing.T) {
	var tokenCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/register":
			_, _ = io.WriteString(w, `{"client_id":"dynamic-client"}`)
		case "/oauth/access_token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("client_secret") != "" {
				t.Fatal("public client sent a client secret")
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-2","token_type":"Bearer","expires_in":3600}`)
				return
			}
			if r.Form.Get("code_verifier") == "" {
				t.Fatal("authorization exchange omitted PKCE verifier")
			}
			_, _ = io.WriteString(w, `{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer","expires_in":3600}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	flow := New(Config{
		APIBaseURL: server.URL, AppBaseURL: server.URL, ClientName: "Operator",
		HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{9}, 128)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1/callback", []manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.Callback(context.Background(), "wrong-state", "code"); err == nil {
		t.Fatal("wrong state was accepted")
	}
	tokens, err := flow.Callback(context.Background(), auth.State, "code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if tokens.AccessToken != "access-1" || tokens.RefreshToken != "refresh-1" {
		t.Fatalf("tokens=%+v", tokens)
	}
	if _, err := flow.Callback(context.Background(), auth.State, "code-again"); err == nil {
		t.Fatal("reused state was accepted")
	}
	rotated, err := flow.Refresh(context.Background(), tokens.ClientID, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.AccessToken != "access-2" || rotated.RefreshToken != "refresh-2" {
		t.Fatalf("rotated tokens=%+v", rotated)
	}
	if tokenCalls != 2 {
		t.Fatalf("token calls=%d, want 2", tokenCalls)
	}
}

func TestLogsAndErrorsDoNotLeakCodesOrTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/register" {
			_, _ = io.WriteString(w, `{"client_id":"dynamic-client"}`)
			return
		}
		http.Error(w, "access-secret refresh-secret", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	flow := New(Config{
		APIBaseURL: server.URL, AppBaseURL: server.URL, ClientName: "Operator",
		HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(&logs, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{11}, 128)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err = flow.Callback(context.Background(), auth.State, "authorization-secret")
	if err == nil {
		t.Fatal("failed token exchange returned no error")
	}
	combined := err.Error() + logs.String()
	for _, secret := range []string{"authorization-secret", "access-secret", "refresh-secret"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("OAuth output leaked %q: %s", secret, combined)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}
