package notion

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDiscoverRegisterAuthorizeExchangeAndRefresh(t *testing.T) {
	var server *httptest.Server
	var tokenForms []url.Values
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_servers": []string{server.URL}, "resource": server.URL, "scopes_supported": []string{"default"}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "registration_endpoint": server.URL + "/register",
				"code_challenge_methods_supported": []string{"S256"},
			})
		case "/register":
			var request RegistrationRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode registration: %v", err)
			}
			if len(request.RedirectURIs) != 1 || request.TokenEndpointAuthMethod != "none" {
				t.Fatalf("registration = %+v", request)
			}
			_ = json.NewEncoder(w).Encode(ClientRegistration{ClientID: "notion-client", RedirectURIs: request.RedirectURIs, TokenEndpointAuthMethod: "none"})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenForms = append(tokenForms, r.Form)
			if r.Form.Get("grant_type") == "authorization_code" {
				if r.Form.Get("code_verifier") == "" || r.Form.Get("code") != "approved-code" {
					t.Fatalf("authorization form = %v", r.Form)
				}
				_ = json.NewEncoder(w).Encode(TokenSet{AccessToken: "access-one", RefreshToken: "refresh-one", TokenType: "Bearer", ExpiresIn: 3600, Scope: "default", UserID: "user-1", WorkspaceID: "workspace-1"})
				return
			}
			_ = json.NewEncoder(w).Encode(TokenSet{AccessToken: "access-two", RefreshToken: "refresh-two", TokenType: "Bearer", ExpiresIn: 3600, Scope: "default"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	flow := New(Config{ServerURL: server.URL + "/mcp", HTTPClient: server.Client(), RandomBytes: bytes.NewReader(bytes.Repeat([]byte{7}, 128)), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	metadata, err := flow.Discover(t.Context())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	client, err := flow.Register(t.Context(), metadata, "http://127.0.0.1:9197/oauth/notion/callback")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	auth, err := flow.Start(t.Context(), metadata, client, "http://127.0.0.1:9197/oauth/notion/callback")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	parsed, _ := url.Parse(auth.URL)
	if parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("resource") != server.URL {
		t.Fatalf("authorization URL = %s", auth.URL)
	}
	tokens, err := flow.Callback(t.Context(), auth.State, "approved-code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if tokens.AccessToken != "access-one" || tokens.RefreshToken != "refresh-one" || tokens.WorkspaceID != "workspace-1" {
		t.Fatalf("tokens = %+v", tokens)
	}
	rotated, err := flow.Refresh(t.Context(), metadata, client, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.AccessToken != "access-two" || rotated.RefreshToken != "refresh-two" {
		t.Fatalf("rotated = %+v", rotated)
	}
	if len(tokenForms) != 2 || tokenForms[1].Get("client_id") != "notion-client" || tokenForms[1].Get("refresh_token") != "refresh-one" {
		t.Fatalf("token forms = %v", tokenForms)
	}
}

func TestCallbackRejectsWrongState(t *testing.T) {
	flow := New(Config{RandomBytes: strings.NewReader(strings.Repeat("x", 128))})
	if _, err := flow.Callback(t.Context(), "wrong", "code"); err == nil {
		t.Fatal("wrong state was accepted")
	}
}
