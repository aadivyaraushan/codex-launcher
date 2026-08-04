package microsoft_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	msoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
	msproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/microsoft"
)

// Callers: proof package tests. Schemas: OAuth TokenSet + in-memory Connection.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-microsoft-proof"
// Confirmed absent: microsoft_proof_absent=yes. No data files — synthetic tokens only.

type fakeFlow struct {
	mu        sync.Mutex
	redirect  string
	state     string
	callback  int
	chatStart bool
	mailVerbs []manifest.Verb
	chatVerbs []manifest.Verb
}

func (f *fakeFlow) Start(_ context.Context, redirect string, verbs []manifest.Verb) (msoauth.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redirect = redirect
	f.state = "expected-state"
	f.mailVerbs = append([]manifest.Verb(nil), verbs...)
	return msoauth.Authorization{URL: "https://login.microsoft.test/consumers/oauth2/v2.0/authorize?state=expected-state", State: f.state}, nil
}

func TestAuthorizeRequestsConfiguredMailScopes(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	connection, err := msproof.Authorize(ctx, msproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9195/oauth/microsoft/callback",
		Flow:          flow,
		Verbs:         []manifest.Verb{manifest.Read, manifest.Write, manifest.Send},
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	defer connection.Close()
	if len(flow.mailVerbs) != 3 || flow.mailVerbs[0] != manifest.Read || flow.mailVerbs[1] != manifest.Write || flow.mailVerbs[2] != manifest.Send {
		t.Fatalf("mail verbs=%v, want [read write send]", flow.mailVerbs)
	}
}

func (f *fakeFlow) StartChat(_ context.Context, redirect string, verbs []manifest.Verb) (msoauth.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redirect = redirect
	f.state = "expected-chat-state"
	f.chatStart = true
	f.chatVerbs = append([]manifest.Verb(nil), verbs...)
	return msoauth.Authorization{
		URL:   "https://login.microsoft.test/organizations/oauth2/v2.0/authorize?state=expected-chat-state",
		State: f.state,
	}, nil
}

func (f *fakeFlow) Callback(_ context.Context, state, code string) (msoauth.TokenSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callback++
	if state != f.state || code != "approved-code" {
		return msoauth.TokenSet{}, msoauth.ErrInvalidState
	}
	return msoauth.TokenSet{AccessToken: "eyJaccess-secret", TokenType: "Bearer"}, nil
}

func (f *fakeFlow) callbackURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.redirect
}

type fakeMailAPI struct {
	messages []outlook.Message
	cleared  int
}

func (f *fakeMailAPI) ListMessages(context.Context, string) ([]outlook.Message, error) {
	return f.messages, nil
}
func (f *fakeMailAPI) CreateDraft(context.Context, outlook.CreateDraft) (outlook.Message, error) {
	return outlook.Message{}, errors.New("write should not run in read-safe proof")
}
func (f *fakeMailAPI) SendMail(context.Context, outlook.SendMail) error {
	return errors.New("send should not run in read-safe proof")
}
func (f *fakeMailAPI) Clear(context.Context) error { f.cleared++; return nil }

func TestRunWaitsForOAuthThenProvesMailReadAndRevoke(t *testing.T) {
	flow := &fakeFlow{}
	mailAPI := &fakeMailAPI{messages: []outlook.Message{{ID: "m1", Subject: "Hello"}}}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go completeOAuth(t, ctx, flow)
	err := msproof.Run(ctx, msproof.Config{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9195/oauth/microsoft/callback",
		Flow:          flow,
		NewMailAPI:    func(*msproof.Connection) outlook.API { return mailAPI },
		Output:        &output,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "AUTH_URL=") || !strings.Contains(text, "VERDICT:") {
		t.Fatalf("output missing markers: %s", text)
	}
	if !strings.Contains(text, "mail_message_count=1") {
		t.Fatalf("read count missing: %s", text)
	}
	if mailAPI.cleared != 1 {
		t.Fatalf("cleared=%d", mailAPI.cleared)
	}
	if flow.callback != 1 {
		t.Fatalf("callback count=%d", flow.callback)
	}
	// Callers: proof_test only. Guard: transcript must not print access token.
	// User: follow-up on Microsoft judge — add token-leak assertion.
	if strings.Contains(text, "eyJaccess-secret") {
		t.Fatalf("transcript leaked token:\n%s", text)
	}
}

func TestAuthorizeUsesConfiguredHTTPRedirectPath(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	connection, err := msproof.Authorize(ctx, msproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9195/oauth/microsoft/callback",
		Flow:          flow,
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	defer connection.Close()
	if !strings.Contains(flow.callbackURL(), "/oauth/microsoft/callback") {
		t.Fatalf("redirect=%q", flow.callbackURL())
	}
	token, err := connection.AccessToken(ctx)
	if err != nil || token != "eyJaccess-secret" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestAuthorizeSurfacesProviderCallbackErrorWithoutTokenExchange(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuthError(t, ctx, flow)
	_, err := msproof.Authorize(ctx, msproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9195/oauth/microsoft/callback",
		Flow:          flow,
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid_request") || !strings.Contains(err.Error(), "AADSTS900144") {
		t.Fatalf("Authorize error = %v, want safe provider callback error", err)
	}
	if strings.Contains(err.Error(), "private account detail") {
		t.Fatalf("Authorize leaked provider description: %v", err)
	}
	if flow.callback != 0 {
		t.Fatalf("token exchange ran %d times after provider callback error", flow.callback)
	}
}

// Callers: proof_test. Covers portless localhost rebuild before Start.
// User: follow-up on Microsoft judge — unit-test portless redirect rebuild.
func TestAuthorizeRebuildsPortlessLocalhostRedirect(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	connection, err := msproof.Authorize(ctx, msproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://localhost/oauth/microsoft/callback",
		Flow:          flow,
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	defer connection.Close()
	got := flow.callbackURL()
	if !strings.HasPrefix(got, "http://localhost:") || !strings.HasSuffix(got, "/oauth/microsoft/callback") {
		t.Fatalf("rebuilt redirect=%q", got)
	}
	if strings.HasPrefix(got, "http://localhost/") && !strings.Contains(got, "localhost:") {
		t.Fatalf("portless redirect was not rebuilt: %q", got)
	}
}

// Callers: proof_test. Teams work/school authorize start uses StartChat
// (Chat.ReadWrite + organizations), not mail Start/consumers.
func TestAuthorizeChatStartsWithChatScopes(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	var output bytes.Buffer
	connection, err := msproof.AuthorizeChat(ctx, msproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9196/oauth/microsoft/callback",
		Flow:          flow,
		Output:        &output,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("AuthorizeChat: %v", err)
	}
	defer connection.Close()
	if !flow.chatStart {
		t.Fatal("AuthorizeChat did not call StartChat")
	}
	if len(flow.chatVerbs) != 2 || flow.chatVerbs[0] != manifest.Read || flow.chatVerbs[1] != manifest.Send {
		t.Fatalf("chat verbs=%v", flow.chatVerbs)
	}
	text := output.String()
	if !strings.Contains(text, "AUTH_URL=") || !strings.Contains(text, "/organizations/") {
		t.Fatalf("output missing Teams organizations AUTH_URL: %s", text)
	}
	if strings.Contains(text, "eyJaccess-secret") {
		t.Fatalf("transcript leaked token:\n%s", text)
	}
}

// Callers: proof_test. Teams AuthorizeChat must fail closed on consumers
// (work/school Graph chat is not supported for personal Microsoft accounts).
func TestAuthorizeChatRejectsConsumersTenant(t *testing.T) {
	flow := msoauth.New(msoauth.Config{
		ClientID: "ms-client-id", Tenant: "consumers",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := msproof.AuthorizeChat(ctx, msproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9196/oauth/microsoft/callback",
		Flow:          flow,
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if !errors.Is(err, msoauth.ErrConsumersTenantForChat) {
		t.Fatalf("AuthorizeChat consumers err = %v, want ErrConsumersTenantForChat", err)
	}
}

func completeOAuth(t *testing.T, ctx context.Context, flow *fakeFlow) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		flow.mu.Lock()
		redirect := flow.redirect
		state := flow.state
		flow.mu.Unlock()
		if redirect != "" && state != "" {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect+"?state="+state+"&code=approved-code", nil)
			if err != nil {
				t.Errorf("build callback: %v", err)
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("oauth callback never became ready")
}

func completeOAuthError(t *testing.T, ctx context.Context, flow *fakeFlow) {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		redirect := flow.callbackURL()
		if redirect == "" {
			time.Sleep(time.Millisecond)
			continue
		}
		u, err := url.Parse(redirect)
		if err != nil {
			t.Errorf("parse redirect: %v", err)
			return
		}
		q := u.Query()
		q.Set("state", "expected-state")
		q.Set("error", "invalid_request")
		q.Set("error_description", "AADSTS900144: private account detail")
		u.RawQuery = q.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			t.Errorf("build callback: %v", err)
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		_ = response.Body.Close()
		return
	}
}
