package microsoft

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

func TestWave1ScopesStayInsideMailCeiling(t *testing.T) {
	got, err := ScopesForVerbs([]manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("ScopesForVerbs read: %v", err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, ScopeMailRead) || !strings.Contains(joined, ScopeOfflineAccess) {
		t.Fatalf("read scopes=%v, want Mail.Read + offline_access", got)
	}
	if strings.Contains(joined, ScopeMailSend) || strings.Contains(joined, ScopeMailReadWrite) {
		t.Fatalf("read-only must not request write/send: %v", got)
	}

	got, err = ScopesForVerbs([]manifest.Verb{manifest.Read, manifest.Write, manifest.Send})
	if err != nil {
		t.Fatalf("ScopesForVerbs full: %v", err)
	}
	joined = strings.Join(got, " ")
	for _, need := range []string{ScopeMailReadWrite, ScopeMailSend, ScopeOfflineAccess, ScopeUserRead} {
		if !strings.Contains(joined, need) {
			t.Fatalf("full scopes missing %s: %v", need, got)
		}
	}
	if _, err := ScopesForVerbs([]manifest.Verb{manifest.Compose}); err == nil {
		t.Fatal("unsupported verb returned scopes")
	}
}

func TestStartBuildsMicrosoftAuthorizeURL(t *testing.T) {
	flow := New(Config{
		ClientID: "ms-client-id", Tenant: "consumers",
		AuthorizeURL: "https://login.microsoft.test/consumers/oauth2/v2.0/authorize",
		TokenURL:     "https://login.microsoft.test/consumers/oauth2/v2.0/token",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
	})
	redirect := "http://127.0.0.1:9195/oauth/microsoft/callback"
	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if !strings.HasSuffix(u.Path, "/oauth2/v2.0/authorize") {
		t.Fatalf("authorize path=%s", u.Path)
	}
	if q.Get("client_id") != "ms-client-id" || q.Get("response_type") != "code" {
		t.Fatalf("query=%v", q)
	}
	if q.Get("redirect_uri") != redirect {
		t.Fatalf("redirect_uri=%q", q.Get("redirect_uri"))
	}
	if q.Get("response_mode") != "query" {
		t.Fatalf("response_mode=%q", q.Get("response_mode"))
	}
	scope := q.Get("scope")
	if !strings.Contains(scope, ScopeMailRead) || !strings.Contains(scope, ScopeOfflineAccess) {
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
		_, _ = io.WriteString(w, `{"access_token":"eyJaccess","refresh_token":"0.Refresh","expires_in":3600,"token_type":"Bearer","scope":"Mail.Read offline_access User.Read"}`)
	}))
	defer server.Close()

	flow := New(Config{
		ClientID: "ms-client-id", Tenant: "consumers",
		AuthorizeURL: server.URL + "/oauth2/v2.0/authorize",
		TokenURL:     server.URL + "/token",
		HTTPClient:   server.Client(),
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes:  bytes.NewReader(bytes.Repeat([]byte{9}, 64)),
	})
	redirect := "http://127.0.0.1:9195/oauth/microsoft/callback"
	auth, err := flow.Start(context.Background(), redirect, []manifest.Verb{manifest.Read})
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
	if tokens.AccessToken != "eyJaccess" || tokens.RefreshToken != "0.Refresh" || tokens.TokenType != "Bearer" {
		t.Fatalf("tokens=%+v", tokens)
	}
	if tokenForm.Get("client_id") != "ms-client-id" || tokenForm.Get("client_secret") != "" || tokenForm.Get("code_verifier") == "" {
		t.Fatalf("public-client token form is wrong: %v", tokenForm)
	}
	if tokenForm.Get("code") != "auth-code" || tokenForm.Get("redirect_uri") != redirect {
		t.Fatalf("token form=%v", tokenForm)
	}
	if tokenForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type=%q", tokenForm.Get("grant_type"))
	}
	if tokenForm.Get("scope") != strings.Join([]string{ScopeOfflineAccess, ScopeUserRead, ScopeMailRead}, " ") {
		t.Fatalf("scope=%q", tokenForm.Get("scope"))
	}
}

// Personal Outlook uses Microsoft's public-client authorization-code flow.
// PKCE proves that the callback belongs to the process that started sign-in,
// so no client secret is stored or sent by Operator.
func TestPublicClientUsesPKCEWithoutAClientSecret(t *testing.T) {
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		tokenForm = r.Form
		_, _ = io.WriteString(w, `{"access_token":"eyJaccess","refresh_token":"0.Refresh","expires_in":3600,"token_type":"Bearer","scope":"Mail.Read offline_access User.Read"}`)
	}))
	defer server.Close()

	flow := New(Config{
		ClientID: "ms-public-client", Tenant: "consumers",
		AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token",
		HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{12}, 96)),
	})
	auth, err := flow.Start(t.Context(), "http://localhost:9195/oauth/microsoft/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	authorizeURL, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	if authorizeURL.Query().Get("code_challenge_method") != "S256" || authorizeURL.Query().Get("code_challenge") == "" {
		t.Fatalf("authorize query is missing PKCE: %v", authorizeURL.Query())
	}
	if _, err := flow.Callback(t.Context(), auth.State, "approved-code"); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if tokenForm.Get("code_verifier") == "" {
		t.Fatalf("token form is missing PKCE verifier: %v", tokenForm)
	}
	if tokenForm.Get("client_secret") != "" {
		t.Fatalf("public client sent a client secret: %v", tokenForm)
	}
}

func TestCallbackReportsSafeMicrosoftErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_request","error_description":"AADSTS900144: The request body must contain the following parameter: 'scope'. Private account detail","error_codes":[900144],"correlation_id":"safe-correlation"}`)
	}))
	defer server.Close()
	flow := New(Config{
		ClientID: "ms-client-id", Tenant: "organizations",
		AuthorizeURL: server.URL, TokenURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), RandomBytes: bytes.NewReader(bytes.Repeat([]byte{4}, 64)),
	})
	auth, err := flow.Start(t.Context(), "http://127.0.0.1:9195/oauth/microsoft/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err = flow.Callback(t.Context(), auth.State, "rejected-code")
	if err == nil || !strings.Contains(err.Error(), "invalid_request") || !strings.Contains(err.Error(), "900144") || !strings.Contains(err.Error(), "missing_parameter=scope") {
		t.Fatalf("Callback error = %v, want safe provider error and numeric code", err)
	}
	if strings.Contains(err.Error(), "private account detail") {
		t.Fatalf("Callback leaked provider description: %v", err)
	}
}

func TestStartRequiresConfiguredCredentials(t *testing.T) {
	flow := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := flow.Start(context.Background(), "http://127.0.0.1:9195/oauth/microsoft/callback", []manifest.Verb{manifest.Read}); err == nil {
		t.Fatal("missing credentials were accepted")
	}
}

func TestRefreshUsesOfflineGrantScopesAndKeepsRotatedRefreshToken(t *testing.T) {
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		tokenForm = r.Form
		_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"token_type":"Bearer","scope":"Mail.Read offline_access User.Read"}`)
	}))
	defer server.Close()
	flow := New(Config{
		ClientID: "ms-client-id", Tenant: "consumers",
		TokenURL: server.URL, HTTPClient: server.Client(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	scopes := []string{ScopeMailRead, ScopeOfflineAccess, ScopeUserRead}

	tokens, err := flow.Refresh(t.Context(), "existing-refresh", scopes)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tokenForm.Get("grant_type") != "refresh_token" || tokenForm.Get("refresh_token") != "existing-refresh" {
		t.Fatalf("refresh form=%v", tokenForm)
	}
	if tokenForm.Get("scope") != strings.Join(scopes, " ") {
		t.Fatalf("scope=%q", tokenForm.Get("scope"))
	}
	if tokenForm.Get("client_id") != "ms-client-id" || tokenForm.Get("client_secret") != "" {
		t.Fatalf("public-client refresh form is wrong: %v", tokenForm)
	}
	if tokens.AccessToken != "new-access" || tokens.RefreshToken != "new-refresh" {
		t.Fatalf("tokens=%+v", tokens)
	}
}

// Callers: oauth/microsoft tests. Affected API: Flow.Start loopback gate.
// User: follow-up on Microsoft judge — enforce loopback in Flow.Start.
func TestStartRejectsNonLoopbackRedirect(t *testing.T) {
	flow := New(Config{
		ClientID:    "ms-client-id",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{5}, 64)),
	})
	_, err := flow.Start(context.Background(), "http://example.com/oauth/microsoft/callback", []manifest.Verb{manifest.Read})
	if !errors.Is(err, ErrLoopbackRequired) {
		t.Fatalf("err=%v, want ErrLoopbackRequired", err)
	}
}

func TestStartDefaultsTenantToConsumers(t *testing.T) {
	flow := New(Config{
		ClientID:    "ms-client-id",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{3}, 64)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1:9195/oauth/microsoft/callback", []manifest.Verb{manifest.Read})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(u.Path, "/consumers/") {
		t.Fatalf("expected consumers tenant in path, got %s", u.Path)
	}
}

// Callers: oauth/microsoft tests. Teams work/school chat scopes must not
// change mail-only ScopesForVerbs used by Outlook personal proof.
func TestChatScopesForTeamsVerbsLeaveMailScopesUnchanged(t *testing.T) {
	mail, err := ScopesForVerbs([]manifest.Verb{manifest.Read, manifest.Write, manifest.Send})
	if err != nil {
		t.Fatalf("mail scopes: %v", err)
	}
	mailJoined := strings.Join(mail, " ")
	for _, need := range []string{ScopeMailReadWrite, ScopeMailSend, ScopeOfflineAccess, ScopeUserRead} {
		if !strings.Contains(mailJoined, need) {
			t.Fatalf("mail path missing %s: %v", need, mail)
		}
	}
	if strings.Contains(mailJoined, ScopeChatReadWrite) {
		t.Fatalf("mail-only ScopesForVerbs must not request Chat.ReadWrite: %v", mail)
	}

	chat, err := ChatScopesForVerbs([]manifest.Verb{manifest.Read, manifest.Send})
	if err != nil {
		t.Fatalf("chat scopes: %v", err)
	}
	chatJoined := strings.Join(chat, " ")
	for _, need := range []string{ScopeChatReadWrite, ScopeOfflineAccess, ScopeUserRead} {
		if !strings.Contains(chatJoined, need) {
			t.Fatalf("chat scopes missing %s: %v", need, chat)
		}
	}
	if strings.Contains(chatJoined, ScopeMailRead) || strings.Contains(chatJoined, ScopeMailSend) {
		t.Fatalf("chat scopes must not request mail: %v", chat)
	}
	if _, err := ChatScopesForVerbs([]manifest.Verb{manifest.Write}); err == nil {
		t.Fatal("unsupported chat verb returned scopes")
	}
}

func TestStartChatUsesOrganizationsTenantAndChatScopes(t *testing.T) {
	flow := New(Config{
		ClientID: "ms-client-id", Tenant: TenantOrganizations,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{11}, 64)),
	})
	redirect := "http://127.0.0.1:9196/oauth/microsoft/callback"
	auth, err := flow.StartChat(context.Background(), redirect, []manifest.Verb{manifest.Read, manifest.Send})
	if err != nil {
		t.Fatalf("StartChat: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(u.Path, "/organizations/") {
		t.Fatalf("Teams authorize must use organizations tenant, got %s", u.Path)
	}
	if strings.Contains(u.Path, "/consumers/") {
		t.Fatalf("Teams authorize must not use consumers: %s", u.Path)
	}
	scope := u.Query().Get("scope")
	if !strings.Contains(scope, ScopeChatReadWrite) || !strings.Contains(scope, ScopeOfflineAccess) {
		t.Fatalf("StartChat scope=%q", scope)
	}
	if strings.Contains(scope, ScopeMailRead) {
		t.Fatalf("StartChat must not request mail scopes: %q", scope)
	}
}

func TestStartChatRejectsConsumersTenant(t *testing.T) {
	flow := New(Config{
		ClientID: "ms-client-id", Tenant: "consumers",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{11}, 64)),
	})
	_, err := flow.StartChat(context.Background(), "http://127.0.0.1:9196/oauth/microsoft/callback", []manifest.Verb{manifest.Read, manifest.Send})
	if !errors.Is(err, ErrConsumersTenantForChat) {
		t.Fatalf("StartChat consumers err = %v, want ErrConsumersTenantForChat", err)
	}
}

func TestLiveChatOAuthStartAgainstMicrosoftWhenEnvPresent(t *testing.T) {
	clientID := os.Getenv("MICROSOFT_CLIENT_ID")
	redirect := os.Getenv("MICROSOFT_REDIRECT_URI")
	if clientID == "" || redirect == "" {
		t.Skip("live Microsoft credentials not present in env")
	}
	tenant := os.Getenv("MICROSOFT_TEAMS_TENANT")
	if tenant == "" {
		tenant = TenantOrganizations
	}
	parsed, err := url.Parse(redirect)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatalf("expected loopback redirect host, got invalid or non-loopback (value not logged)")
	}
	t.Logf("teams_env_present True True True tenant=%s redirect_scheme_host_path %s %s %s",
		tenant, parsed.Scheme, parsed.Host, safePath(redirect))

	flow := New(Config{
		ClientID: clientID, Tenant: tenant,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	auth, err := flow.StartChat(context.Background(), redirect, []manifest.Verb{manifest.Read, manifest.Send})
	if err != nil {
		t.Fatalf("StartChat: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host != "login.microsoftonline.com" || !strings.Contains(u.Path, "/"+tenant+"/") {
		t.Fatalf("authorize URL host/path unexpected: host=%s path=%s", u.Host, u.Path)
	}
	if strings.Contains(u.Path, "/consumers/") {
		t.Fatalf("Teams live start must not use consumers: %s", u.Path)
	}
	scope := u.Query().Get("scope")
	if !strings.Contains(scope, ScopeChatReadWrite) {
		t.Fatalf("live StartChat scope missing Chat.ReadWrite: %q", scope)
	}
	t.Logf("teams_authorize_ok path=%s scope_count=%d state_len=%d",
		u.Path, len(strings.Fields(scope)), len(auth.State))

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
	t.Logf("teams_authorize_http_status=%d location_present=%t", resp.StatusCode, loc != "")
	for _, needle := range []string{"redirect_uri_mismatch", "invalid_client", "access_denied", "error=invalid_request"} {
		if strings.Contains(combined, needle) {
			if errCode := extractAADSTS(combined); errCode != "" {
				t.Fatalf("BLOCKER_SIGNAL=%s aadsts=%s status=%d", needle, errCode, resp.StatusCode)
			}
			t.Fatalf("BLOCKER_SIGNAL=%s status=%d", needle, resp.StatusCode)
		}
	}
	if errCode := extractAADSTS(combined); errCode != "" {
		t.Fatalf("BLOCKER_SIGNAL=aadsts aadsts=%s status=%d", errCode, resp.StatusCode)
	}
}

func TestLiveOAuthStartAgainstMicrosoftWhenEnvPresent(t *testing.T) {
	clientID := os.Getenv("MICROSOFT_CLIENT_ID")
	redirect := os.Getenv("MICROSOFT_REDIRECT_URI")
	tenant := os.Getenv("MICROSOFT_TENANT")
	if clientID == "" || redirect == "" {
		t.Skip("live Microsoft credentials not present in env")
	}
	if tenant == "" {
		tenant = "consumers"
	}
	parsed, err := url.Parse(redirect)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatalf("expected loopback redirect host, got invalid or non-loopback (value not logged)")
	}
	t.Logf("env_present True True True tenant=%s redirect_scheme_host_path %s %s %s",
		tenant, parsed.Scheme, parsed.Host, safePath(redirect))

	flow := New(Config{
		ClientID: clientID, Tenant: tenant,
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
	if u.Host != "login.microsoftonline.com" || !strings.HasSuffix(u.Path, "/oauth2/v2.0/authorize") {
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
			if errCode := extractAADSTS(combined); errCode != "" {
				t.Fatalf("BLOCKER_SIGNAL=%s aadsts=%s status=%d", needle, errCode, resp.StatusCode)
			}
			t.Fatalf("BLOCKER_SIGNAL=%s status=%d", needle, resp.StatusCode)
		}
	}
	if errCode := extractAADSTS(combined); errCode != "" {
		t.Fatalf("BLOCKER_SIGNAL=aadsts aadsts=%s status=%d", errCode, resp.StatusCode)
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

func extractAADSTS(combined string) string {
	upper := strings.ToUpper(combined)
	idx := strings.Index(upper, "AADSTS")
	if idx < 0 {
		return ""
	}
	end := idx
	for end < len(upper) && ((upper[end] >= 'A' && upper[end] <= 'Z') || (upper[end] >= '0' && upper[end] <= '9')) {
		end++
	}
	return upper[idx:end]
}
