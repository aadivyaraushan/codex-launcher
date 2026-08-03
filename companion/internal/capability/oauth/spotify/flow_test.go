package spotify

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func TestScopesAreDerivedFromTheVerbsAndNeverAskForWrite(t *testing.T) {
	cases := []struct {
		verbs []manifest.Verb
		want  string
	}{
		{[]manifest.Verb{manifest.Read}, "user-read-playback-state"},
		{[]manifest.Verb{manifest.Play}, "user-read-playback-state user-modify-playback-state"},
		{[]manifest.Verb{manifest.Read, manifest.Play}, "user-read-playback-state user-modify-playback-state"},
	}
	for _, test := range cases {
		got, err := ScopesForVerbs(test.verbs)
		if err != nil || strings.Join(got, " ") != test.want {
			t.Fatalf("ScopesForVerbs(%v)=%v err=%v, want %q", test.verbs, got, err, test.want)
		}
	}
	if _, err := ScopesForVerbs([]manifest.Verb{manifest.Write}); err != ErrNoScopes {
		t.Fatalf("write verb returned %v, want ErrNoScopes", err)
	}
}

func TestStartRequiresAClientIDAndSecretAndBuildsTheAuthorizeURL(t *testing.T) {
	flow := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if _, err := flow.Start(context.Background(), "http://127.0.0.1:8888/callback", []manifest.Verb{manifest.Play}); err != ErrMissingCredential {
		t.Fatalf("Start without credentials returned %v, want ErrMissingCredential", err)
	}

	flow = New(Config{
		ClientID: "client-id", ClientSecret: "client-secret",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1:8888/callback", []manifest.Verb{manifest.Read, manifest.Play})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(auth.URL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	if u.Host != "accounts.spotify.com" || u.Path != "/authorize" {
		t.Fatalf("authorize URL=%s", auth.URL)
	}
	q := u.Query()
	if q.Get("client_id") != "client-id" || q.Get("response_type") != "code" || q.Get("redirect_uri") != "http://127.0.0.1:8888/callback" {
		t.Fatalf("authorize query=%v", q)
	}
	if q.Get("scope") != "user-read-playback-state user-modify-playback-state" {
		t.Fatalf("scope=%q", q.Get("scope"))
	}
	if q.Get("state") == "" || auth.State != q.Get("state") {
		t.Fatalf("authorize URL lacks state: %s", auth.URL)
	}
}

func TestCallbackExchangesTheCodeWithBasicAuthAndRefreshRotatesTheToken(t *testing.T) {
	var tokenCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
		if r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("authorization header = %q, want %q", r.Header.Get("Authorization"), wantAuth)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.Form.Get("client_secret") != "" {
			t.Fatal("client secret must go in the Authorization header, not the body")
		}
		tokenCalls++
		if r.Form.Get("grant_type") == "refresh_token" {
			if r.Form.Get("refresh_token") != "refresh-1" {
				t.Fatalf("refresh_token=%q", r.Form.Get("refresh_token"))
			}
			_, _ = io.WriteString(w, `{"access_token":"access-2","token_type":"Bearer","expires_in":3600,"scope":"user-read-playback-state user-modify-playback-state"}`)
			return
		}
		if r.Form.Get("code") != "auth-code" || r.Form.Get("redirect_uri") != "http://127.0.0.1:8888/callback" {
			t.Fatalf("unexpected authorization_code form: %v", r.Form)
		}
		_, _ = io.WriteString(w, `{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer","expires_in":3600,"scope":"user-read-playback-state user-modify-playback-state"}`)
	}))
	defer server.Close()

	flow := New(Config{
		ClientID: "client-id", ClientSecret: "client-secret", TokenURL: server.URL + "/api/token",
		HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{9}, 64)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1:8888/callback", []manifest.Verb{manifest.Play})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := flow.Callback(context.Background(), "wrong-state", "auth-code"); err != ErrInvalidState {
		t.Fatalf("wrong state returned %v, want ErrInvalidState", err)
	}
	tokens, err := flow.Callback(context.Background(), auth.State, "auth-code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if tokens.AccessToken != "access-1" || tokens.RefreshToken != "refresh-1" {
		t.Fatalf("tokens=%+v", tokens)
	}
	if _, err := flow.Callback(context.Background(), auth.State, "auth-code-again"); err != ErrInvalidState {
		t.Fatalf("reused state returned %v, want ErrInvalidState", err)
	}

	rotated, err := flow.Refresh(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	// Spotify may omit refresh_token on refresh; the flow must keep using the
	// previous one rather than dropping it.
	if rotated.AccessToken != "access-2" || rotated.RefreshToken != "refresh-1" {
		t.Fatalf("rotated tokens=%+v", rotated)
	}
	if tokenCalls != 2 {
		t.Fatalf("token calls=%d, want 2", tokenCalls)
	}
}

func TestLogsAndErrorsDoNotLeakCodesOrTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "access-secret refresh-secret", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	flow := New(Config{
		ClientID: "client-id", ClientSecret: "client-secret", TokenURL: server.URL,
		HTTPClient: server.Client(), Logger: slog.New(slog.NewTextHandler(&logs, nil)),
		RandomBytes: bytes.NewReader(bytes.Repeat([]byte{11}, 64)),
	})
	auth, err := flow.Start(context.Background(), "http://127.0.0.1:8888/callback", []manifest.Verb{manifest.Play})
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
