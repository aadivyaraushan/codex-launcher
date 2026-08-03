package todoist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	todoistoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/todoist"
)

type fakeFlow struct {
	mu       sync.Mutex
	redirect string
	state    string
	callback int
}

func (f *fakeFlow) Start(_ context.Context, redirect string, verbs []manifest.Verb) (todoistoauth.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redirect = redirect
	f.state = "expected-state"
	return todoistoauth.Authorization{URL: "https://app.todoist.test/oauth/authorize?state=expected-state", State: f.state}, nil
}

func (f *fakeFlow) Callback(_ context.Context, state, code string) (todoistoauth.TokenSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callback++
	if state != f.state || code != "approved-code" {
		return todoistoauth.TokenSet{}, todoistoauth.ErrInvalidState
	}
	return todoistoauth.TokenSet{AccessToken: "access-secret", RefreshToken: "refresh-secret"}, nil
}

func (f *fakeFlow) callbackURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.redirect
}

type fakeAPI struct {
	created []todoistadapter.CreateTask
	tasks   []todoistadapter.Task
	cleared int
}

func (f *fakeAPI) ListTasks(context.Context) ([]todoistadapter.Task, error) { return f.tasks, nil }

func (f *fakeAPI) CreateTask(_ context.Context, request todoistadapter.CreateTask) (todoistadapter.Task, error) {
	f.created = append(f.created, request)
	task := todoistadapter.Task{ID: "proof-task", Content: request.Content, Description: request.Description}
	f.tasks = append(f.tasks, task)
	return task, nil
}

func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return nil }

func TestRunWaitsForOAuthAndExplicitConfirmationThenProvesCreateReadAndRevoke(t *testing.T) {
	flow := &fakeFlow{}
	api := &fakeAPI{}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go completeOAuth(t, ctx, flow)
	err := Run(ctx, Config{
		ListenAddress: "127.0.0.1:0", Flow: flow,
		NewAPI: func(*Connection) todoistadapter.API { return api },
		Input:  strings.NewReader("yes\n"), Output: &output,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Content: "Operator Wave 1 proof", Description: "Created only after preview confirmation",
	})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, output.String())
	}
	if len(api.created) != 1 {
		t.Fatalf("created=%d, want 1", len(api.created))
	}
	if api.cleared != 1 {
		t.Fatalf("clear calls=%d, want 1", api.cleared)
	}
	transcript := output.String()
	for _, want := range []string{"AUTH_URL=", "Operator Wave 1 proof", "Type yes", "created Todoist task proof-task", "VERDICT: todoist RT-2 proven"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript lacks %q:\n%s", want, transcript)
		}
	}
	for _, secret := range []string{"access-secret", "refresh-secret", "approved-code"} {
		if strings.Contains(transcript, secret) {
			t.Fatalf("transcript leaked %q", secret)
		}
	}
}

func TestRunDoesNotCreateWhenPreviewIsDeclined(t *testing.T) {
	flow := &fakeFlow{}
	api := &fakeAPI{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	err := Run(ctx, Config{
		ListenAddress: "127.0.0.1:0", Flow: flow,
		NewAPI: func(*Connection) todoistadapter.API { return api },
		Input:  strings.NewReader("no\n"), Output: io.Discard,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Content: "must not be created",
	})
	if err == nil || len(api.created) != 0 {
		t.Fatalf("declined run err=%v created=%d", err, len(api.created))
	}
}

func TestRunRecoversAnExactTaskAfterAnUncertainWriteWithoutCreatingADuplicate(t *testing.T) {
	flow := &fakeFlow{}
	api := &fakeAPI{tasks: []todoistadapter.Task{{
		ID: "recovered-task", Content: "Operator Wave 1 proof unique", Description: "Confirmed preview",
	}}}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)

	err := Run(ctx, Config{
		ListenAddress: "127.0.0.1:0", Flow: flow,
		NewAPI: func(*Connection) todoistadapter.API { return api },
		Input:  strings.NewReader("yes\n"), Output: &output,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Content: "Operator Wave 1 proof unique", Description: "Confirmed preview",
	})

	if err != nil {
		t.Fatalf("Run: %v\n%s", err, output.String())
	}
	if len(api.created) != 0 {
		t.Fatalf("created=%d, want zero because the exact task already exists", len(api.created))
	}
	if !strings.Contains(output.String(), "RECOVERY: exact Todoist task already exists id=recovered-task") {
		t.Fatalf("recovery was not recorded:\n%s", output.String())
	}
	if api.cleared != 1 {
		t.Fatalf("clear calls=%d, want 1", api.cleared)
	}
}

func TestDefaultProofTaskTitleIsUniqueToTheRun(t *testing.T) {
	now := time.Date(2026, 7, 31, 20, 15, 16, 123456789, time.UTC)
	if got, want := defaultContent(now), "Operator Wave 1 Todoist proof 20260731T201516.123456789Z"; got != want {
		t.Fatalf("default content=%q, want %q", got, want)
	}
}

// A browser that navigates away mid-callback cancels the HTTP request
// context. Token exchange must keep using the Authorize parent context, or
// Safari/Chrome closing the tab kills a successful OAuth mid-flight.
func TestAuthorizeSurvivesCanceledHTTPRequestContext(t *testing.T) {
	flow := &slowCallbackFlow{fakeFlow: &fakeFlow{}, entered: make(chan struct{}), hold: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	connCh := make(chan *Connection, 1)
	go func() {
		connection, err := Authorize(ctx, AuthorizationConfig{
			ListenAddress: "127.0.0.1:0",
			Flow:          flow,
			Output:        io.Discard,
			Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		if err != nil {
			errCh <- err
			return
		}
		connCh <- connection
	}()

	var redirect string
	deadline := time.Now().Add(time.Second)
	for redirect == "" {
		if time.Now().After(deadline) {
			t.Fatal("Authorize never published a callback URL")
		}
		redirect = flow.callbackURL()
		time.Sleep(time.Millisecond)
	}

	u, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("parse callback URL: %v", err)
	}
	q := u.Query()
	q.Set("state", "expected-state")
	q.Set("code", "approved-code")
	u.RawQuery = q.Encode()

	conn, dialErr := net.Dial("tcp", u.Host)
	if dialErr != nil {
		t.Fatalf("dial callback host: %v", dialErr)
	}
	requestLine := "GET " + u.RequestURI() + " HTTP/1.1\r\nHost: " + u.Host + "\r\nConnection: close\r\n\r\n"
	if _, writeErr := io.WriteString(conn, requestLine); writeErr != nil {
		t.Fatalf("write callback request: %v", writeErr)
	}

	select {
	case <-flow.entered:
	case <-ctx.Done():
		t.Fatal("timed out waiting for Callback to start")
	}
	// Drop the browser connection while token exchange is still running.
	_ = conn.Close()
	time.Sleep(50 * time.Millisecond)
	close(flow.hold)

	select {
	case connection := <-connCh:
		if connection == nil {
			t.Fatal("Authorize returned a nil connection")
		}
		token, tokenErr := connection.AccessToken(context.Background())
		if tokenErr != nil || token != "access-secret" {
			t.Fatalf("token=%q err=%v", token, tokenErr)
		}
		_ = connection.Close()
	case err := <-errCh:
		t.Fatalf("Authorize failed after the browser dropped the callback connection: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for Authorize")
	}
}

type slowCallbackFlow struct {
	*fakeFlow
	entered chan struct{}
	hold    chan struct{}
	once    sync.Once
}

func (f *slowCallbackFlow) Callback(ctx context.Context, state, code string) (todoistoauth.TokenSet, error) {
	f.once.Do(func() { close(f.entered) })
	select {
	case <-f.hold:
	case <-time.After(2 * time.Second):
		return todoistoauth.TokenSet{}, errors.New("hold never released")
	}
	if err := ctx.Err(); err != nil {
		return todoistoauth.TokenSet{}, fmt.Errorf("callback context died while exchanging tokens: %w", err)
	}
	return f.fakeFlow.Callback(ctx, state, code)
}

func completeOAuth(t *testing.T, ctx context.Context, flow *fakeFlow) {
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
			return
		}
		q := u.Query()
		q.Set("state", "expected-state")
		q.Set("code", "approved-code")
		u.RawQuery = q.Encode()
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return
		}
		_ = response.Body.Close()
		return
	}
}

var _ capabilityadapter.Adapter = (*todoistadapter.Adapter)(nil)
