package slack

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	slackoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/slack"
)

// Callers: proof_test only. Captures OAuth verbs so read-safe Authorize can be asserted.
// User: follow-up on Slack judge — read path must not request chat:write.
type fakeFlow struct {
	mu       sync.Mutex
	redirect string
	state    string
	verbs    []manifest.Verb
	callback int
}

func (f *fakeFlow) Start(_ context.Context, redirect string, verbs []manifest.Verb) (slackoauth.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redirect = redirect
	f.verbs = append([]manifest.Verb(nil), verbs...)
	f.state = "expected-state"
	return slackoauth.Authorization{URL: "https://slack.test/oauth/v2_user/authorize?state=expected-state", State: f.state}, nil
}

func (f *fakeFlow) Callback(_ context.Context, state, code string) (slackoauth.TokenSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callback++
	if state != f.state || code != "approved-code" {
		return slackoauth.TokenSet{}, slackoauth.ErrInvalidState
	}
	return slackoauth.TokenSet{AccessToken: "xoxp-access-secret", TokenType: "user"}, nil
}

func (f *fakeFlow) callbackURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.redirect
}

type fakeAPI struct {
	channels []slackadapter.Channel
	cleared  int
}

func (f *fakeAPI) ListChannels(context.Context) ([]slackadapter.Channel, error) {
	return f.channels, nil
}
func (f *fakeAPI) PostMessage(context.Context, slackadapter.PostMessage) (slackadapter.PostedMessage, error) {
	return slackadapter.PostedMessage{}, errors.New("send should not run in read-safe proof")
}
func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return nil }

func TestRunWaitsForOAuthThenProvesChannelReadAndRevoke(t *testing.T) {
	flow := &fakeFlow{}
	api := &fakeAPI{channels: []slackadapter.Channel{{ID: "C1", Name: "general"}}}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go completeOAuth(t, ctx, flow)
	err := Run(ctx, Config{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "https://127.0.0.1:9192/oauth/slack/callback",
		Flow:          flow,
		NewAPI:        func(*Connection) slackadapter.API { return api },
		Output:        &output,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Channel:       "general",
	})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, output.String())
	}
	if api.cleared != 1 {
		t.Fatalf("clear calls=%d, want 1", api.cleared)
	}
	transcript := output.String()
	for _, want := range []string{"AUTH_URL=", "REDIRECT_URI=https://", "READ:", "VERDICT: slack user OAuth proven"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript lacks %q:\n%s", want, transcript)
		}
	}
	if strings.Contains(transcript, "xoxp-access-secret") {
		t.Fatalf("transcript leaked token:\n%s", transcript)
	}
}

func TestAuthorizeRejectsHTTPRedirectURI(t *testing.T) {
	_, err := Authorize(context.Background(), AuthorizationConfig{
		Flow:          &fakeFlow{},
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9192/oauth/slack/callback",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if !errors.Is(err, ErrHTTPSRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestAuthorizeRequestsReadOnlyScopes(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go completeOAuth(t, ctx, flow)
	conn, err := Authorize(ctx, AuthorizationConfig{
		Flow:          flow,
		Verbs:         []manifest.Verb{manifest.Read},
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "https://127.0.0.1:0/oauth/slack/callback",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Output:        io.Discard,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	flow.mu.Lock()
	verbs := append([]manifest.Verb(nil), flow.verbs...)
	flow.mu.Unlock()
	if len(verbs) != 1 || verbs[0] != manifest.Read {
		t.Fatalf("Authorize verbs=%v, want [read] only (no chat:write)", verbs)
	}
}

func TestAuthorizeRequestsConfiguredSendScope(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go completeOAuth(t, ctx, flow)
	conn, err := Authorize(ctx, AuthorizationConfig{
		Flow:          flow,
		Verbs:         []manifest.Verb{manifest.Read, manifest.Send},
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "https://127.0.0.1:0/oauth/slack/callback",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Output:        io.Discard,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	flow.mu.Lock()
	verbs := append([]manifest.Verb(nil), flow.verbs...)
	flow.mu.Unlock()
	if len(verbs) != 2 || verbs[0] != manifest.Read || verbs[1] != manifest.Send {
		t.Fatalf("Authorize verbs=%v, want [read send]", verbs)
	}
}

func completeOAuth(t *testing.T, ctx context.Context, flow *fakeFlow) {
	t.Helper()
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
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
		q.Set("code", "approved-code")
		u.RawQuery = q.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			t.Errorf("build callback: %v", err)
			return
		}
		response, err := client.Do(request)
		if err != nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		_ = response.Body.Close()
		return
	}
}
