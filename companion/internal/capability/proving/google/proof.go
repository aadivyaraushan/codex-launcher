// Package google drives the owner-only Google OAuth stop checkpoint for
// Calendar (calendar.events) and Drive (drive.file): HTTP callback, in-memory
// token, read-safe list checks, and revoke. Writes stay behind preview.
package google

import (
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

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	googleoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/google"
)

// Callers: google_proof.go serve command + proof_test.go.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-google-proof"

var ErrMissingDependencies = errors.New("google proof: flow and API factories are required")

const defaultRedirectURI = "http://127.0.0.1:9194/oauth/google/callback"

type OAuthFlow interface {
	Start(context.Context, string, []manifest.Verb) (googleoauth.Authorization, error)
	Callback(context.Context, string, string) (googleoauth.TokenSet, error)
}

type Config struct {
	ListenAddress  string
	RedirectURI    string
	Flow           OAuthFlow
	NewCalendarAPI func(*Connection) gcalendar.API
	NewDriveAPI    func(*Connection) gdrive.API
	Output         io.Writer
	Logger         *slog.Logger
}

type AuthorizationConfig struct {
	ListenAddress string
	RedirectURI   string
	Flow          OAuthFlow
	Output        io.Writer
	Logger        *slog.Logger
}

// Connection is the proof run's in-memory token source. The token disappears
// when Clear runs and is never written to disk or printed.
type Connection struct {
	mu     sync.RWMutex
	access string
}

func (c *Connection) AccessToken(context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.access == "" {
		return "", gcalendar.ErrNotConnected
	}
	return c.access, nil
}

func (c *Connection) Clear(context.Context) error {
	c.mu.Lock()
	c.access = ""
	c.mu.Unlock()
	return nil
}

func (c *Connection) Close() error {
	return c.Clear(context.Background())
}

func Run(ctx context.Context, config Config) error {
	if config.Flow == nil || config.NewCalendarAPI == nil || config.NewDriveAPI == nil {
		return ErrMissingDependencies
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	connection, err := Authorize(ctx, AuthorizationConfig{
		ListenAddress: config.ListenAddress,
		RedirectURI:   config.RedirectURI,
		Flow:          config.Flow,
		Output:        config.Output,
		Logger:        config.Logger,
	})
	if err != nil {
		return err
	}
	calendarAPI := config.NewCalendarAPI(connection)
	driveAPI := config.NewDriveAPI(connection)

	events, err := calendarAPI.ListEvents(ctx, "")
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("google proof: list calendar events: %w", err)
	}
	fmt.Fprintf(config.Output, "READ: calendar_event_count=%d\n", len(events))

	files, err := driveAPI.ListFiles(ctx, "")
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("google proof: list drive files: %w", err)
	}
	fmt.Fprintf(config.Output, "READ: drive_file_count=%d\n", len(files))

	_ = calendarAPI.Clear(ctx)
	_ = driveAPI.Clear(ctx)
	_ = connection.Clear(ctx)
	fmt.Fprintln(config.Output, "REVOKE: in-memory access token cleared")
	fmt.Fprintln(config.Output, "VERDICT: google user OAuth proven by calendar+drive read and revoke (writes left gated behind preview)")
	return nil
}

// Authorize completes the owner-only OAuth callback over HTTP loopback and
// returns an in-memory token source. Callers must close it when serving stops.
func Authorize(ctx context.Context, config AuthorizationConfig) (*Connection, error) {
	if config.Flow == nil {
		return nil, errors.New("google proof: OAuth flow is required")
	}
	if config.ListenAddress == "" {
		config.ListenAddress = "127.0.0.1:9194"
	}
	if config.RedirectURI == "" {
		config.RedirectURI = defaultRedirectURI
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	parsed, err := url.Parse(config.RedirectURI)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("google proof: redirect URI must be http(s) loopback: %q", config.RedirectURI)
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return nil, fmt.Errorf("google proof: redirect URI must be loopback (127.0.0.1 or localhost): host=%q", host)
	}

	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("google proof: listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	if strings.Contains(config.RedirectURI, ":0/") || strings.HasSuffix(config.ListenAddress, ":0") {
		path := parsed.Path
		if path == "" {
			path = "/oauth/google/callback"
		}
		config.RedirectURI = parsed.Scheme + "://" + listener.Addr().String() + path
		parsed, err = url.Parse(config.RedirectURI)
		if err != nil {
			return nil, fmt.Errorf("google proof: rebuild redirect URI: %w", err)
		}
	}

	authorization, err := config.Flow.Start(ctx, config.RedirectURI, []manifest.Verb{manifest.Read})
	if err != nil {
		return nil, fmt.Errorf("google proof: start OAuth: %w", err)
	}

	tokens := make(chan googleoauth.TokenSet, 1)
	callbackErrors := make(chan error, 1)
	callbackPath := parsed.Path
	if callbackPath == "" {
		callbackPath = "/"
	}
	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		set, callbackErr := config.Flow.Callback(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if callbackErr != nil {
			config.Logger.Warn("[google-proof] OAuth callback rejected", "error", callbackErr)
			http.Error(w, "Google sign-in could not be accepted. Return to Operator and try again.", http.StatusBadRequest)
			select {
			case callbackErrors <- callbackErr:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "Google is connected. You can return to Operator.")
		select {
		case tokens <- set:
		default:
		}
	}
	mux.HandleFunc(callbackPath, handler)
	if callbackPath != "/oauth/google/callback" {
		mux.HandleFunc("/oauth/google/callback", handler)
	}
	server := &http.Server{Handler: mux}
	serveErrors := make(chan error, 1)
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serveErrors <- serveErr
		}
	}()
	defer server.Shutdown(context.Background())

	fmt.Fprintf(config.Output, "AUTH_URL=%s\n", authorization.URL)
	fmt.Fprintf(config.Output, "REDIRECT_URI=%s\n", config.RedirectURI)
	fmt.Fprintln(config.Output, "Open that URL, approve Google as yourself (user OAuth), and return here after the browser says it is connected.")
	config.Logger.Info("[google-proof] waiting for OAuth", "callback_host", listener.Addr().String(), "https", parsed.Scheme == "https")

	var tokenSet googleoauth.TokenSet
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("google proof: wait for OAuth: %w", ctx.Err())
	case err := <-serveErrors:
		return nil, fmt.Errorf("google proof: callback server: %w", err)
	case err := <-callbackErrors:
		return nil, fmt.Errorf("google proof: OAuth callback: %w", err)
	case tokenSet = <-tokens:
	}
	return &Connection{access: tokenSet.AccessToken}, nil
}
