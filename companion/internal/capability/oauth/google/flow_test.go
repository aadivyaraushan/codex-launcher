package google

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func TestWave1ScopesStayInsideCeiling(t *testing.T) {
	got, err := ScopesForVerbs([]manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		t.Fatalf("ScopesForVerbs: %v", err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, ScopeCalendarEvents) || !strings.Contains(joined, ScopeDriveFile) {
		t.Fatalf("scopes=%v, want calendar.events and drive.file", got)
	}
	for _, scope := range got {
		if strings.Contains(scope, "gmail") || scope == "https://www.googleapis.com/auth/drive" ||
			scope == "https://www.googleapis.com/auth/drive.readonly" ||
			scope == "https://www.googleapis.com/auth/calendar" {
			t.Fatalf("restricted/broad scope leaked: %s", scope)
		}
	}
	if _, err := ScopesForVerbs([]manifest.Verb{manifest.Send}); err == nil {
		t.Fatal("unsupported verb returned scopes")
	}
}

func TestStartBuildsGoogleAuthorizeURL(t *testing.T) {
	flow := New(Config{
		ClientID: "google-client-id", ClientSecret: "google-client-secret",
		AuthorizeURL: "https://accounts.google.test/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.google.test/token",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
	})
	redirect := "http://127.0.0.1:9194/oauth/google/callback"
	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if u.Path != "/o/oauth2/v2/auth" {
		t.Fatalf("authorize path=%s", u.Path)
	}
	if q.Get("client_id") != "google-client-id" || q.Get("response_type") != "code" {
		t.Fatalf("query=%v", q)
	}
	if q.Get("redirect_uri") != redirect {
		t.Fatalf("redirect_uri=%q", q.Get("redirect_uri"))
	}
	if q.Get("access_type") != "offline" {
		t.Fatalf("access_type=%q", q.Get("access_type"))
	}
	scope := q.Get("scope")
	if !strings.Contains(scope, ScopeCalendarEvents) || !strings.Contains(scope, ScopeDriveFile) {
		t.Fatalf("scope=%q", scope)
	}
	if q.Get("state") == "" || auth.State != q.Get("state") {
		t.Fatalf("state mismatch auth=%+v query=%v", auth, q)
	}
}

func TestCallbackRejectsWrongStateAndReturnsTokens(t *testing.T) {
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		tokenForm = r.Form
		_, _ = io.WriteString(w, `{"access_token":"ya29.access","refresh_token":"1//refresh","expires_in":3600,"token_type":"Bearer","scope":"https://www.googleapis.com/auth/calendar.events https://www.googleapis.com/auth/drive.file"}`)
	}))
	defer server.Close()

	flow := New(Config{
		ClientID: "google-client-id", ClientSecret: "google-client-secret",
		AuthorizeURL: server.URL + "/o/oauth2/v2/auth",
		TokenURL:     server.URL + "/token",
		HTTPClient:   server.Client(),
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{9}, 64)),
	})
	redirect := "http://127.0.0.1:9194/oauth/google/callback"
	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.Callback(context.Background(), "wrong-state", "code"); err == nil {
		t.Fatal("wrong state was accepted")
	}
	tokens, err := flow.Callback(context.Background(), auth.State, "auth-code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if tokens.AccessToken != "ya29.access" || tokens.RefreshToken != "1//refresh" || tokens.TokenType != "Bearer" {
		t.Fatalf("tokens=%+v", tokens)
	}
	if tokenForm.Get("client_id") != "google-client-id" || tokenForm.Get("client_secret") != "google-client-secret" {
		t.Fatalf("token form credentials missing: %v", tokenForm)
	}
	if tokenForm.Get("code") != "auth-code" || tokenForm.Get("redirect_uri") != redirect {
		t.Fatalf("token form=%v", tokenForm)
	}
	if tokenForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type=%q", tokenForm.Get("grant_type"))
	}
}

func TestStartRequiresConfiguredCredentials(t *testing.T) {
	flow := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := flow.Start(context.Background(), "http://127.0.0.1:9194/oauth/google/callback", []manifest.Verb{manifest.Read}); err == nil {
		t.Fatal("missing credentials were accepted")
	}
}

func TestRefreshUsesOfflineGrantAndPreservesRefreshTokenWhenGoogleOmitsIt(t *testing.T) {
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		tokenForm = r.Form
		_, _ = io.WriteString(w, `{"access_token":"new-access","expires_in":3600,"token_type":"Bearer","scope":"scope-a scope-b"}`)
	}))
	defer server.Close()
	flow := New(Config{
		ClientID: "google-client-id", ClientSecret: "google-client-secret",
		TokenURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	tokens, err := flow.Refresh(t.Context(), "existing-refresh")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tokenForm.Get("grant_type") != "refresh_token" || tokenForm.Get("refresh_token") != "existing-refresh" {
		t.Fatalf("refresh form=%v", tokenForm)
	}
	if tokenForm.Get("client_id") != "google-client-id" || tokenForm.Get("client_secret") != "google-client-secret" {
		t.Fatalf("refresh credentials missing: %v", tokenForm)
	}
	if tokens.AccessToken != "new-access" || tokens.RefreshToken != "existing-refresh" {
		t.Fatalf("tokens=%+v", tokens)
	}
}

func TestLiveOAuthStartAgainstGoogleWhenEnvPresent(t *testing.T) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirect := os.Getenv("GOOGLE_REDIRECT_URI")
	if clientID == "" || clientSecret == "" || redirect == "" {
		t.Skip("live Google credentials not present in env")
	}
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") && !strings.HasPrefix(redirect, "https://127.0.0.1:") {
		t.Fatalf("expected loopback redirect, got host from GOOGLE_REDIRECT_URI (value not logged)")
	}
	flow := New(Config{
		ClientID: clientID, ClientSecret: clientSecret,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host != "accounts.google.com" || u.Path != "/o/oauth2/v2/auth" {
		t.Fatalf("authorize URL host/path unexpected: host=%s path=%s", u.Host, u.Path)
	}
	t.Logf("authorize_ok path=%s scope_count=%d state_len=%d redirect_path=%s",
		u.Path, len(strings.Fields(u.Query().Get("scope"))), len(auth.State), safePath(redirect))

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(auth.URL)
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	loc := resp.Header.Get("Location")
	combined := strings.ToLower(string(body) + " " + loc)
	t.Logf("authorize_http_status=%d location_present=%t", resp.StatusCode, loc != "")
	for _, needle := range []string{"redirect_uri_mismatch", "invalid_client", "access_denied", "error=invalid_request"} {
		if strings.Contains(combined, needle) {
			t.Fatalf("BLOCKER_SIGNAL=%s status=%d", needle, resp.StatusCode)
		}
	}
}

func safePath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "invalid"
	}
	if parsed.Path == "" {
		return "/"
	}
	return parsed.Path
}
