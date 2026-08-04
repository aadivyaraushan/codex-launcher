// Package spotify drives the real Spotify adapter through a live search and
// play attempt: OAuth, preview (track + target device), confirmed play, and
// revoke. Unlike Todoist's proof, the OAuth callback listener binds to a
// fixed address rather than an ephemeral port, because Spotify requires the
// redirect URI to be pre-registered on the developer dashboard
// (SPOTIFY_REDIRECT_URI in .env is http://127.0.0.1:8888/callback).
package spotify

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	spotifyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	spotifyoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/spotify"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

var ErrPreviewDeclined = errors.New("spotify proof: preview was not confirmed")

// defaultListenAddress and defaultCallbackPath together must match
// SPOTIFY_REDIRECT_URI exactly, since Spotify only accepts a pre-registered
// redirect URI — the proof cannot ask for an ephemeral port the way
// Todoist's dynamically-registered client can.
const (
	defaultListenAddress = "127.0.0.1:8888"
	defaultCallbackPath  = "/callback"
)

type OAuthFlow interface {
	Start(context.Context, string, []manifest.Verb) (spotifyoauth.Authorization, error)
	Callback(context.Context, string, string) (spotifyoauth.TokenSet, error)
}

type Config struct {
	ListenAddress string
	CallbackPath  string
	Flow          OAuthFlow
	NewAPI        func(*Connection) spotifyadapter.API
	Query         string
	Input         io.Reader
	Output        io.Writer
	Logger        *slog.Logger
}

type AuthorizationConfig struct {
	ListenAddress string
	CallbackPath  string
	Flow          OAuthFlow
	Output        io.Writer
	Logger        *slog.Logger
}

// Connection is the proof run's in-memory token source. The token disappears
// when Clear runs and is never written to disk or printed.
type Connection struct {
	mu     sync.RWMutex
	tokens spotifyoauth.TokenSet
}

func (c *Connection) AccessToken(context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tokens.AccessToken == "" {
		return "", spotifyadapter.ErrNotConnected
	}
	return c.tokens.AccessToken, nil
}

func (c *Connection) Snapshot() spotifyoauth.TokenSet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snapshot := c.tokens
	snapshot.Scopes = append([]string(nil), c.tokens.Scopes...)
	return snapshot
}

func (c *Connection) Clear(context.Context) error {
	c.mu.Lock()
	c.tokens = spotifyoauth.TokenSet{}
	c.mu.Unlock()
	return nil
}

func (c *Connection) Close() error {
	return c.Clear(context.Background())
}

func Run(ctx context.Context, config Config) error {
	if config.Flow == nil || config.NewAPI == nil {
		return errors.New("spotify proof: flow and API factory are required")
	}
	if config.Input == nil {
		config.Input = strings.NewReader("")
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if strings.TrimSpace(config.Query) == "" {
		return errors.New("spotify proof: a search query is required")
	}

	connection, err := Authorize(ctx, AuthorizationConfig{
		ListenAddress: config.ListenAddress,
		CallbackPath:  config.CallbackPath,
		Flow:          config.Flow,
		Output:        config.Output,
		Logger:        config.Logger,
	})
	if err != nil {
		return err
	}
	api := config.NewAPI(connection)
	a := spotifyadapter.New(api, config.Logger)
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("spotify proof: register adapter: %w", err)
	}
	runner := execution.New(reg)

	plan, err := runner.Resolve(ctx, adapter.Intent{
		AdapterID: spotifyadapter.ID, Verb: manifest.Play, Subject: config.Query,
	})
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("spotify proof: resolve play: %w", err)
	}
	preview, err := runner.Preview(ctx, plan)
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("spotify proof: preview play: %w", err)
	}
	fmt.Fprintln(config.Output, "\nPreview — nothing has played yet:")
	fmt.Fprintf(config.Output, "  %s\n", preview.Headline)
	for _, line := range preview.Lines {
		fmt.Fprintf(config.Output, "  - %s\n", line)
	}
	fmt.Fprintln(config.Output, "Type yes to start playback on the device shown above:")
	answer, readErr := bufio.NewReader(config.Input).ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = connection.Clear(ctx)
		return fmt.Errorf("spotify proof: read confirmation: %w", readErr)
	}
	if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
		_ = connection.Clear(ctx)
		return ErrPreviewDeclined
	}

	outcome, executeErr := runner.Execute(ctx, plan, preview.Confirmed())
	if executeErr != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("spotify proof: execute play: %w", executeErr)
	}
	fmt.Fprintf(config.Output, "PLAY: reached=%s done=%t handed_off_to=%q detail=%s\n", outcome.Reached, outcome.Done, outcome.HandedOffTo, outcome.Detail)
	if outcome.Reached == manifest.Completes {
		fmt.Fprintln(config.Output, "Now check the Pixel: adb shell dumpsys media_session (or dumpsys audio) to confirm audio is actually playing.")
	} else {
		fmt.Fprintln(config.Output, "VERDICT-NOTE: demoted to hands_off — this is a fail per the plan's Pixel table, not a pass.")
	}

	if err := runner.Revoke(ctx, spotifyadapter.ID); err != nil {
		return fmt.Errorf("spotify proof: revoke: %w", err)
	}
	fmt.Fprintln(config.Output, "REVOKE: in-memory access token cleared and adapter removed from the registry")
	return nil
}

// Authorize completes the owner-only OAuth callback and returns an in-memory
// token source. Callers must close it when their serving process stops.
func Authorize(ctx context.Context, config AuthorizationConfig) (*Connection, error) {
	if config.Flow == nil {
		return nil, errors.New("spotify proof: OAuth flow is required")
	}
	if config.ListenAddress == "" {
		config.ListenAddress = defaultListenAddress
	}
	if config.CallbackPath == "" {
		config.CallbackPath = defaultCallbackPath
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("spotify proof: listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	redirectURI := "http://" + config.ListenAddress + config.CallbackPath
	authorization, err := config.Flow.Start(ctx, redirectURI, []manifest.Verb{manifest.Read, manifest.Play})
	if err != nil {
		return nil, fmt.Errorf("spotify proof: start OAuth: %w", err)
	}

	tokens := make(chan spotifyoauth.TokenSet, 1)
	callbackErrors := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(config.CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Use the Authorize parent context, not r.Context(): browsers often
		// drop the callback connection as soon as the redirect lands, which
		// cancels the request context and would abort a live token exchange.
		set, callbackErr := config.Flow.Callback(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if callbackErr != nil {
			config.Logger.Warn("[spotify-proof] OAuth callback rejected", "error", callbackErr)
			http.Error(w, "Spotify sign-in could not be accepted. Return to Operator and try again.", http.StatusBadRequest)
			select {
			case callbackErrors <- callbackErr:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "Spotify is connected. You can return to Operator.")
		select {
		case tokens <- set:
		default:
		}
	})
	server := &http.Server{Handler: mux}
	serveErrors := make(chan error, 1)
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serveErrors <- serveErr
		}
	}()
	defer server.Shutdown(context.Background())

	fmt.Fprintf(config.Output, "AUTH_URL=%s\n", authorization.URL)
	fmt.Fprintln(config.Output, "Open that URL, approve Spotify, and return here after the browser says it is connected.")
	config.Logger.Info("[spotify-proof] waiting for OAuth", "callback_host", config.ListenAddress)

	var tokenSet spotifyoauth.TokenSet
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("spotify proof: wait for OAuth: %w", ctx.Err())
	case err := <-serveErrors:
		return nil, fmt.Errorf("spotify proof: callback server: %w", err)
	case err := <-callbackErrors:
		return nil, fmt.Errorf("spotify proof: OAuth callback: %w", err)
	case tokenSet = <-tokens:
	}
	return &Connection{tokens: tokenSet}, nil
}
