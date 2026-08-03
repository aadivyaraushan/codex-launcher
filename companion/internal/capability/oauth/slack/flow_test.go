package slack

import (
	"bytes"
	"context"
	"errors"
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

func TestScopesAreUserTokenScopesDerivedFromVerbs(t *testing.T) {
	cases := []struct {
		verbs []manifest.Verb
		want  string
	}{
		{[]manifest.Verb{manifest.Read}, "channels:read,groups:read"},
		{[]manifest.Verb{manifest.Send}, "channels:read,groups:read,chat:write"},
		{[]manifest.Verb{manifest.Read, manifest.Send}, "channels:read,groups:read,chat:write"},
	}
	for _, test := range cases {
		got, err := ScopesForVerbs(test.verbs)
		if err != nil || strings.Join(got, ",") != test.want {
			t.Fatalf("ScopesForVerbs(%v)=%v err=%v, want %q", test.verbs, got, err, test.want)
		}
	}
	if _, err := ScopesForVerbs([]manifest.Verb{manifest.Order}); err == nil {
		t.Fatal("unsupported verb returned scopes")
	}
}

func TestStartBuildsUserOnlyAuthorizeURLWithoutBotScope(t *testing.T) {
	flow := New(Config{
		ClientID: "slack-client-id", ClientSecret: "slack-client-secret",
		AuthorizeURL: "https://slack.test/oauth/v2_user/authorize",
		TokenURL:     "https://slack.test/api/oauth.v2.user.access",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
	})
	auth, err := flow.Start(context.Background(), "https://127.0.0.1:9192/oauth/slack/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	q := u.Query()
	if u.Path != "/oauth/v2_user/authorize" {
		t.Fatalf("authorize path=%s, want /oauth/v2_user/authorize", u.Path)
	}
	if q.Get("client_id") != "slack-client-id" || q.Get("response_type") != "code" {
		t.Fatalf("authorize query=%v", q)
	}
	if q.Get("scope") != "channels:read,groups:read" {
		t.Fatalf("scope=%q", q.Get("scope"))
	}
	if q.Get("user_scope") != "" || q.Get("bot_scope") != "" {
		t.Fatalf("user-only flow must not send bot/user_scope params: %v", q)
	}
	if q.Get("redirect_uri") != "https://127.0.0.1:9192/oauth/slack/callback" {
		t.Fatalf("redirect_uri=%q", q.Get("redirect_uri"))
	}
	if q.Get("state") == "" || auth.State != q.Get("state") {
		t.Fatalf("state mismatch auth=%+v query=%v", auth, q)
	}
}

func TestCallbackRejectsWrongStateAndReturnsUserAccessToken(t *testing.T) {
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth.v2.user.access" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		tokenForm = r.Form
		_, _ = io.WriteString(w, `{"ok":true,"access_token":"xoxp-user-token","token_type":"user","team":{"id":"T1","name":"Ops"},"authed_user":{"id":"U1"}}`)
	}))
	defer server.Close()

	flow := New(Config{
		ClientID: "slack-client-id", ClientSecret: "slack-client-secret",
		AuthorizeURL: server.URL + "/oauth/v2_user/authorize",
		TokenURL:     server.URL + "/api/oauth.v2.user.access",
		HTTPClient:   server.Client(),
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{9}, 64)),
	})
	auth, err := flow.Start(context.Background(), "https://127.0.0.1:9192/oauth/slack/callback", []manifest.Verb{manifest.Read, manifest.Send})
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
	if tokens.AccessToken != "xoxp-user-token" || tokens.TokenType != "user" || tokens.TeamID != "T1" || tokens.UserID != "U1" {
		t.Fatalf("tokens=%+v", tokens)
	}
	if tokenForm.Get("client_id") != "slack-client-id" || tokenForm.Get("client_secret") != "slack-client-secret" {
		t.Fatalf("token form credentials missing: %v", tokenForm)
	}
	if tokenForm.Get("code") != "auth-code" || tokenForm.Get("redirect_uri") != "https://127.0.0.1:9192/oauth/slack/callback" {
		t.Fatalf("token form=%v", tokenForm)
	}
	if tokenForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type=%q", tokenForm.Get("grant_type"))
	}
	if _, err := flow.Callback(context.Background(), auth.State, "auth-code-again"); err == nil {
		t.Fatal("reused state was accepted")
	}
}

func TestCallbackRejectsBotTokenResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"access_token":"xoxb-bot-token","token_type":"bot"}`)
	}))
	defer server.Close()

	flow := New(Config{
		ClientID: "slack-client-id", ClientSecret: "slack-client-secret",
		AuthorizeURL: server.URL + "/oauth/v2_user/authorize",
		TokenURL:     server.URL + "/api/oauth.v2.user.access",
		HTTPClient:   server.Client(),
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{3}, 64)),
	})
	auth, err := flow.Start(context.Background(), "https://127.0.0.1/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.Callback(context.Background(), auth.State, "code"); err == nil {
		t.Fatal("bot token response was accepted")
	}
}

func TestLogsAndErrorsDoNotLeakCodesOrTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "xoxp-leaked-token authorization-secret", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	flow := New(Config{
		ClientID: "slack-client-id", ClientSecret: "slack-client-secret",
		AuthorizeURL: server.URL + "/oauth/v2_user/authorize",
		TokenURL:     server.URL + "/api/oauth.v2.user.access",
		HTTPClient:   server.Client(),
		Logger:       slog.New(slog.NewTextHandler(&logs, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{11}, 64)),
	})
	auth, err := flow.Start(context.Background(), "https://127.0.0.1/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err = flow.Callback(context.Background(), auth.State, "authorization-secret")
	if err == nil {
		t.Fatal("failed token exchange returned no error")
	}
	combined := err.Error() + logs.String()
	for _, secret := range []string{"authorization-secret", "xoxp-leaked-token", "slack-client-secret"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("OAuth output leaked %q: %s", secret, combined)
		}
	}
}

func TestStartRequiresConfiguredCredentials(t *testing.T) {
	flow := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := flow.Start(context.Background(), "https://127.0.0.1/callback", []manifest.Verb{manifest.Read}); err == nil {
		t.Fatal("missing credentials were accepted")
	}
}

// Callers of Flow.Start: proving/slack.Authorize, live probe test, unit tests.
// Affected API: Start must reject non-https redirectURI (Slack docs). No new schema.
// User: follow-up on Slack judge — enforce HTTPS in Flow.Start.
func TestStartRejectsHTTPRedirectURI(t *testing.T) {
	flow := New(Config{
		ClientID: "slack-client-id", ClientSecret: "slack-client-secret",
		AuthorizeURL: "https://slack.test/oauth/v2_user/authorize",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{5}, 64)),
	})
	_, err := flow.Start(context.Background(), "http://127.0.0.1:9192/oauth/slack/callback", []manifest.Verb{manifest.Read})
	if !errors.Is(err, ErrHTTPSRequired) {
		t.Fatalf("err=%v, want ErrHTTPSRequired", err)
	}
}

func TestLiveOAuthStartAgainstSlackWhenEnvPresent(t *testing.T) {
	clientID := os.Getenv("SLACK_CLIENT_ID")
	clientSecret := os.Getenv("SLACK_CLIENT_SECRET")
	redirect := os.Getenv("SLACK_REDIRECT_URI")
	if clientID == "" || clientSecret == "" || redirect == "" {
		t.Skip("live Slack credentials not present in env")
	}
	if !strings.HasPrefix(redirect, "https://") {
		t.Fatalf("Slack redirect must be https, got %q", redirect)
	}
	flow := New(Config{
		ClientID: clientID, ClientSecret: clientSecret,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host != "slack.com" || u.Path != "/oauth/v2_user/authorize" {
		t.Fatalf("authorize URL=%s", auth.URL)
	}
	if u.Query().Get("user_scope") != "" {
		t.Fatalf("user-only flow leaked user_scope param: %v", u.Query())
	}
	t.Logf("authorize_ok path=%s scope=%s state_len=%d", u.Path, u.Query().Get("scope"), len(auth.State))

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
	for _, needle := range []string{"bad_redirect_uri", "invalid_client", "oauth_authorization_url_mismatch"} {
		if strings.Contains(combined, needle) {
			t.Fatalf("BLOCKER_SIGNAL=%s status=%d", needle, resp.StatusCode)
		}
	}
}
