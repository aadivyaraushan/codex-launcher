// Package microsoft drives the owner-only Microsoft Graph OAuth stop checkpoint
// for personal Outlook mail: HTTP callback, in-memory token, read-safe list
// check, and revoke. Writes/sends stay behind preview.
package microsoft

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

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	msoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
)

// Callers: microsoft_proof.go serve command + proof_test.go.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-microsoft-proof"

var ErrMissingDependencies = errors.New("microsoft proof: flow and mail API factory are required")

const defaultRedirectURI = "http://127.0.0.1:9195/oauth/microsoft/callback"

type OAuthFlow interface {
	Start(context.Context, string, []manifest.Verb) (msoauth.Authorization, error)
	StartChat(context.Context, string, []manifest.Verb) (msoauth.Authorization, error)
	Callback(context.Context, string, string) (msoauth.TokenSet, error)
}

type Config struct {
	ListenAddress string
	RedirectURI   string
	Flow          OAuthFlow
	NewMailAPI    func(*Connection) outlook.API
	Output        io.Writer
	Logger        *slog.Logger
}

type AuthorizationConfig struct {
	ListenAddress string
	RedirectURI   string
	Flow          OAuthFlow
	Verbs         []manifest.Verb
	Output        io.Writer
	Logger        *slog.Logger
}

// Connection is the proof run's in-memory token source. The token disappears
// when Clear runs and is never written to disk or printed.
type Connection struct {
	mu     sync.RWMutex
	tokens msoauth.TokenSet
}

func (c *Connection) AccessToken(context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tokens.AccessToken == "" {
		return "", outlook.ErrNotConnected
	}
	return c.tokens.AccessToken, nil
}

func (c *Connection) Snapshot() msoauth.TokenSet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snapshot := c.tokens
	snapshot.Scopes = append([]string(nil), c.tokens.Scopes...)
	return snapshot
}

func (c *Connection) Clear(context.Context) error {
	c.mu.Lock()
	c.tokens = msoauth.TokenSet{}
	c.mu.Unlock()
	return nil
}

func (c *Connection) Close() error {
	return c.Clear(context.Background())
}

func Run(ctx context.Context, config Config) error {
	if config.Flow == nil || config.NewMailAPI == nil {
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
	mailAPI := config.NewMailAPI(connection)

	messages, err := mailAPI.ListMessages(ctx, "")
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("microsoft proof: list mail messages: %w", err)
	}
	fmt.Fprintf(config.Output, "READ: mail_message_count=%d\n", len(messages))

	_ = mailAPI.Clear(ctx)
	_ = connection.Clear(ctx)
	fmt.Fprintln(config.Output, "REVOKE: in-memory access token cleared")
	fmt.Fprintln(config.Output, "VERDICT: microsoft user OAuth proven by outlook mail read and revoke (writes/sends left gated behind preview)")
	return nil
}

// Authorize completes the owner-only OAuth callback over HTTP loopback and
// returns an in-memory token source. Callers must close it when serving stops.
// Mail path: Start with Mail.Read on the configured tenant (usually consumers).
func Authorize(ctx context.Context, config AuthorizationConfig) (*Connection, error) {
	return authorize(ctx, config, func(ctx context.Context, redirect string) (msoauth.Authorization, error) {
		verbs := config.Verbs
		if len(verbs) == 0 {
			verbs = []manifest.Verb{manifest.Read}
		}
		return config.Flow.Start(ctx, redirect, verbs)
	})
}

// AuthorizeChat starts work/school Teams Chat.ReadWrite authorize (StartChat)
// and waits for the loopback callback. Live chat send stays owner-gated.
func AuthorizeChat(ctx context.Context, config AuthorizationConfig) (*Connection, error) {
	return authorize(ctx, config, func(ctx context.Context, redirect string) (msoauth.Authorization, error) {
		return config.Flow.StartChat(ctx, redirect, []manifest.Verb{manifest.Read, manifest.Send})
	})
}

func authorize(ctx context.Context, config AuthorizationConfig, start func(context.Context, string) (msoauth.Authorization, error)) (*Connection, error) {
	if config.Flow == nil {
		return nil, errors.New("microsoft proof: OAuth flow is required")
	}
	if config.ListenAddress == "" {
		config.ListenAddress = "127.0.0.1:9195"
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
		return nil, fmt.Errorf("microsoft proof: redirect URI must be http(s) loopback: %q", config.RedirectURI)
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return nil, fmt.Errorf("microsoft proof: redirect URI must be loopback (127.0.0.1 or localhost): host=%q", host)
	}

	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("microsoft proof: listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	if parsed.Port() == "" && (host == "localhost" || host == "127.0.0.1") {
		// Env may register host-only localhost (port 80). Bind the configured
		// listen port and rebuild the redirect so the callback can land, while
		// keeping the registered hostname (Microsoft treats localhost specially).
		path := parsed.Path
		if path == "" {
			path = "/oauth/microsoft/callback"
		}
		port := ""
		if tcp, ok := listener.Addr().(*net.TCPAddr); ok {
			port = fmt.Sprintf("%d", tcp.Port)
		}
		if port == "" {
			return nil, fmt.Errorf("microsoft proof: listen address has no TCP port")
		}
		config.RedirectURI = fmt.Sprintf("%s://%s:%s%s", parsed.Scheme, host, port, path)
		parsed, err = url.Parse(config.RedirectURI)
		if err != nil {
			return nil, fmt.Errorf("microsoft proof: rebuild portless redirect URI: %w", err)
		}
		config.Logger.Warn("[microsoft-proof] rebuilt portless redirect to listen port",
			"callback_host", host+":"+port, "redirect_path", path)
	} else if strings.Contains(config.RedirectURI, ":0/") || strings.HasSuffix(config.ListenAddress, ":0") {
		path := parsed.Path
		if path == "" {
			path = "/oauth/microsoft/callback"
		}
		config.RedirectURI = parsed.Scheme + "://" + listener.Addr().String() + path
		parsed, err = url.Parse(config.RedirectURI)
		if err != nil {
			return nil, fmt.Errorf("microsoft proof: rebuild redirect URI: %w", err)
		}
	}

	authorization, err := start(ctx, config.RedirectURI)
	if err != nil {
		return nil, fmt.Errorf("microsoft proof: start OAuth: %w", err)
	}

	tokens := make(chan msoauth.TokenSet, 1)
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
		if providerErr := microsoftCallbackError(r.URL.Query()); providerErr != nil {
			config.Logger.Warn("[microsoft-proof] OAuth provider callback rejected", "error", providerErr)
			http.Error(w, "Microsoft sign-in could not be accepted. Return to Operator and try again.", http.StatusBadRequest)
			select {
			case callbackErrors <- providerErr:
			default:
			}
			return
		}
		set, callbackErr := config.Flow.Callback(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if callbackErr != nil {
			config.Logger.Warn("[microsoft-proof] OAuth callback rejected", "error", callbackErr)
			http.Error(w, "Microsoft sign-in could not be accepted. Return to Operator and try again.", http.StatusBadRequest)
			select {
			case callbackErrors <- callbackErr:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "Microsoft is connected. You can return to Operator.")
		select {
		case tokens <- set:
		default:
		}
	}
	mux.HandleFunc(callbackPath, handler)
	if callbackPath != "/oauth/microsoft/callback" {
		mux.HandleFunc("/oauth/microsoft/callback", handler)
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
	fmt.Fprintln(config.Output, "Open that URL, approve Microsoft as yourself (user OAuth), and return here after the browser says it is connected.")
	config.Logger.Info("[microsoft-proof] waiting for OAuth", "callback_host", listener.Addr().String(), "https", parsed.Scheme == "https")

	var tokenSet msoauth.TokenSet
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("microsoft proof: wait for OAuth: %w", ctx.Err())
	case err := <-serveErrors:
		return nil, fmt.Errorf("microsoft proof: callback server: %w", err)
	case err := <-callbackErrors:
		return nil, fmt.Errorf("microsoft proof: OAuth callback: %w", err)
	case tokenSet = <-tokens:
	}
	return &Connection{tokens: tokenSet}, nil
}

func microsoftCallbackError(query url.Values) error {
	kind := strings.TrimSpace(query.Get("error"))
	if kind == "" {
		return nil
	}
	for _, r := range kind {
		if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			kind = "provider_error"
			break
		}
	}
	code := microsoftAADSTSCode(query.Get("error_description"))
	if code == "" {
		return fmt.Errorf("microsoft proof: authorization callback error %s", kind)
	}
	return fmt.Errorf("microsoft proof: authorization callback error %s code %s", kind, code)
}

func microsoftAADSTSCode(description string) string {
	upper := strings.ToUpper(description)
	index := strings.Index(upper, "AADSTS")
	if index < 0 {
		return ""
	}
	end := index + len("AADSTS")
	for end < len(upper) && upper[end] >= '0' && upper[end] <= '9' {
		end++
	}
	if end == index+len("AADSTS") {
		return ""
	}
	return upper[index:end]
}
