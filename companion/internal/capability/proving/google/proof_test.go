package google_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	googleoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/google"
	googleproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/google"
)

// Callers: proof package tests. Schemas: OAuth TokenSet + in-memory Connection.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-google-proof"

// Callers: proving/google tests. Captures OAuth verbs for least-privilege asserts.
// User: follow-up on Google judge — proof Authorize should request Read only.
type fakeFlow struct {
	mu       sync.Mutex
	redirect string
	state    string
	verbs    []manifest.Verb
	callback int
}

func (f *fakeFlow) Start(_ context.Context, redirect string, verbs []manifest.Verb) (googleoauth.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redirect = redirect
	f.verbs = append([]manifest.Verb(nil), verbs...)
	f.state = "expected-state"
	return googleoauth.Authorization{URL: "https://accounts.google.test/o/oauth2/v2/auth?state=expected-state", State: f.state}, nil
}

func (f *fakeFlow) Callback(_ context.Context, state, code string) (googleoauth.TokenSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callback++
	if state != f.state || code != "approved-code" {
		return googleoauth.TokenSet{}, googleoauth.ErrInvalidState
	}
	return googleoauth.TokenSet{AccessToken: "ya29.access-secret", TokenType: "Bearer"}, nil
}

func (f *fakeFlow) callbackURL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.redirect
}

type fakeCalendarAPI struct {
	events  []gcalendar.Event
	cleared int
}

func (f *fakeCalendarAPI) ListEvents(context.Context, string) ([]gcalendar.Event, error) {
	return f.events, nil
}
func (f *fakeCalendarAPI) CreateEvent(context.Context, gcalendar.CreateEvent) (gcalendar.Event, error) {
	return gcalendar.Event{}, errors.New("write should not run in read-safe proof")
}
func (f *fakeCalendarAPI) Clear(context.Context) error { f.cleared++; return nil }

type fakeDriveAPI struct {
	files   []gdrive.File
	cleared int
}

func (f *fakeDriveAPI) ListFiles(context.Context, string) ([]gdrive.File, error) { return f.files, nil }
func (f *fakeDriveAPI) CreateFile(context.Context, gdrive.CreateFile) (gdrive.File, error) {
	return gdrive.File{}, errors.New("write should not run in read-safe proof")
}
func (f *fakeDriveAPI) Clear(context.Context) error { f.cleared++; return nil }

func TestRunWaitsForOAuthThenProvesCalendarAndDriveReadAndRevoke(t *testing.T) {
	flow := &fakeFlow{}
	calendarAPI := &fakeCalendarAPI{events: []gcalendar.Event{{ID: "e1", Summary: "Standup"}}}
	driveAPI := &fakeDriveAPI{files: []gdrive.File{{ID: "f1", Name: "notes.txt"}}}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go completeOAuth(t, ctx, flow)
	err := googleproof.Run(ctx, googleproof.Config{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9194/oauth/google/callback",
		Flow:          flow,
		NewCalendarAPI: func(*googleproof.Connection) gcalendar.API { return calendarAPI },
		NewDriveAPI:    func(*googleproof.Connection) gdrive.API { return driveAPI },
		Output:         &output,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "AUTH_URL=") || !strings.Contains(text, "VERDICT:") {
		t.Fatalf("output missing markers: %s", text)
	}
	if !strings.Contains(text, "calendar_event_count=1") || !strings.Contains(text, "drive_file_count=1") {
		t.Fatalf("read counts missing: %s", text)
	}
	if calendarAPI.cleared != 1 || driveAPI.cleared != 1 {
		t.Fatalf("cleared calendar=%d drive=%d", calendarAPI.cleared, driveAPI.cleared)
	}
	if flow.callback != 1 {
		t.Fatalf("callback count=%d", flow.callback)
	}
	// Callers: proof_test only. Guard: transcript must not print access token.
	// User: follow-up on Google judge — add token-leak assertion.
	if strings.Contains(text, "ya29.access-secret") {
		t.Fatalf("transcript leaked token:\n%s", text)
	}
}

func TestAuthorizeUsesConfiguredHTTPRedirectPath(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	connection, err := googleproof.Authorize(ctx, googleproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9194/oauth/google/callback",
		Flow:          flow,
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	defer connection.Close()
	if !strings.Contains(flow.callbackURL(), "/oauth/google/callback") {
		t.Fatalf("redirect=%q", flow.callbackURL())
	}
	token, err := connection.AccessToken(ctx)
	if err != nil || token != "ya29.access-secret" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestAuthorizeRequestsReadOnlyVerbs(t *testing.T) {
	flow := &fakeFlow{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go completeOAuth(t, ctx, flow)
	connection, err := googleproof.Authorize(ctx, googleproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:0",
		RedirectURI:   "http://127.0.0.1:9194/oauth/google/callback",
		Flow:          flow,
		Output:        io.Discard,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	defer connection.Close()
	flow.mu.Lock()
	verbs := append([]manifest.Verb(nil), flow.verbs...)
	flow.mu.Unlock()
	if len(verbs) != 1 || verbs[0] != manifest.Read {
		t.Fatalf("Authorize verbs=%v, want [read] only", verbs)
	}
}

func completeOAuth(t *testing.T, ctx context.Context, flow *fakeFlow) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		redirect := flow.callbackURL()
		if redirect != "" {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect+"?state=expected-state&code=approved-code", nil)
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
