// Package todoist drives the real Todoist adapter through the Wave 1 stop
// checkpoint: OAuth, preview, confirmed create, read-back, and revoke.
package todoist

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
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	todoistoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

var ErrPreviewDeclined = errors.New("todoist proof: preview was not confirmed")

type OAuthFlow interface {
	Start(context.Context, string, []manifest.Verb) (todoistoauth.Authorization, error)
	Callback(context.Context, string, string) (todoistoauth.TokenSet, error)
}

type Config struct {
	ListenAddress string
	Flow          OAuthFlow
	NewAPI        func(*Connection) todoistadapter.API
	Input         io.Reader
	Output        io.Writer
	Logger        *slog.Logger
	Content       string
	Description   string
}

type AuthorizationConfig struct {
	ListenAddress string
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
		return "", todoistadapter.ErrNotConnected
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
	if config.Flow == nil || config.NewAPI == nil {
		return errors.New("todoist proof: flow and API factory are required")
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
	if strings.TrimSpace(config.Content) == "" {
		config.Content = defaultContent(time.Now())
	}

	connection, err := Authorize(ctx, AuthorizationConfig{
		ListenAddress: config.ListenAddress,
		Flow:          config.Flow,
		Output:        config.Output,
		Logger:        config.Logger,
	})
	if err != nil {
		return err
	}
	api := config.NewAPI(connection)
	a := todoistadapter.New(api, config.Logger)
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: register adapter: %w", err)
	}
	runner := execution.New(reg)

	plan, err := runner.Resolve(ctx, adapter.Intent{
		AdapterID: todoistadapter.ID, Verb: manifest.Write,
		Subject: config.Content, Body: config.Description,
	})
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: resolve write: %w", err)
	}
	preview, err := runner.Preview(ctx, plan)
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: preview write: %w", err)
	}
	fmt.Fprintln(config.Output, "\nPreview — nothing has been created yet:")
	fmt.Fprintf(config.Output, "  %s\n", preview.Headline)
	for _, line := range preview.Lines {
		fmt.Fprintf(config.Output, "  - %s\n", line)
	}
	fmt.Fprintln(config.Output, "Type yes to create exactly this task:")
	answer, readErr := bufio.NewReader(config.Input).ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: read confirmation: %w", readErr)
	}
	if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
		_ = connection.Clear(ctx)
		return ErrPreviewDeclined
	}

	existing, err := exactTasks(ctx, api, config.Content)
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: check for an uncertain earlier write: %w", err)
	}
	if len(existing) > 1 {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: %d exact tasks already exist; refusing to create another", len(existing))
	}
	if len(existing) == 1 {
		fmt.Fprintf(config.Output, "RECOVERY: exact Todoist task already exists id=%s\n", existing[0].ID)
	} else {
		outcome, executeErr := runner.Execute(ctx, plan, preview.Confirmed())
		if executeErr != nil {
			_ = connection.Clear(ctx)
			return fmt.Errorf("todoist proof: execute write: %w", executeErr)
		}
		fmt.Fprintf(config.Output, "WRITE: reached=%s done=%t detail=%s\n", outcome.Reached, outcome.Done, outcome.Detail)
	}
	readPlan, err := runner.Resolve(ctx, adapter.Intent{AdapterID: todoistadapter.ID, Verb: manifest.Read, Subject: config.Content})
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: resolve read-back: %w", err)
	}
	readOutcome, err := runner.Execute(ctx, readPlan, execution.Confirmation{})
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("todoist proof: execute read-back: %w", err)
	}
	fmt.Fprintf(config.Output, "READ-BACK: reached=%s done=%t\n", readOutcome.Reached, readOutcome.Done)
	if err := runner.Revoke(ctx, todoistadapter.ID); err != nil {
		return fmt.Errorf("todoist proof: revoke: %w", err)
	}
	fmt.Fprintln(config.Output, "REVOKE: in-memory access token cleared and adapter removed from the registry")
	fmt.Fprintln(config.Output, "VERDICT: todoist RT-2 proven by confirmed create, read-back, and revoke")
	return nil
}

// Authorize completes the owner-only OAuth callback and returns an in-memory
// token source. Callers must close it when their serving process stops.
func Authorize(ctx context.Context, config AuthorizationConfig) (*Connection, error) {
	if config.Flow == nil {
		return nil, errors.New("todoist proof: OAuth flow is required")
	}
	if config.ListenAddress == "" {
		config.ListenAddress = "127.0.0.1:9191"
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("todoist proof: listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	redirectURI := "http://" + listener.Addr().String() + "/oauth/todoist/callback"
	authorization, err := config.Flow.Start(ctx, redirectURI, []manifest.Verb{manifest.Read, manifest.Write})
	if err != nil {
		return nil, fmt.Errorf("todoist proof: start OAuth: %w", err)
	}

	tokens := make(chan todoistoauth.TokenSet, 1)
	callbackErrors := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/todoist/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Use the Authorize parent context, not r.Context(): browsers often
		// drop the callback connection as soon as the redirect lands, which
		// cancels the request context and would abort a live token exchange.
		set, callbackErr := config.Flow.Callback(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if callbackErr != nil {
			config.Logger.Warn("[todoist-proof] OAuth callback rejected", "error", callbackErr)
			http.Error(w, "Todoist sign-in could not be accepted. Return to Operator and try again.", http.StatusBadRequest)
			select {
			case callbackErrors <- callbackErr:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "Todoist is connected. You can return to Operator.")
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
	fmt.Fprintln(config.Output, "Open that URL, approve Todoist, and return here after the browser says it is connected.")
	config.Logger.Info("[todoist-proof] waiting for OAuth", "callback_host", listener.Addr().String())

	var tokenSet todoistoauth.TokenSet
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("todoist proof: wait for OAuth: %w", ctx.Err())
	case err := <-serveErrors:
		return nil, fmt.Errorf("todoist proof: callback server: %w", err)
	case err := <-callbackErrors:
		return nil, fmt.Errorf("todoist proof: OAuth callback: %w", err)
	case tokenSet = <-tokens:
	}
	return &Connection{access: tokenSet.AccessToken}, nil
}

func exactTasks(ctx context.Context, api todoistadapter.API, content string) ([]todoistadapter.Task, error) {
	tasks, err := api.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	want := strings.TrimSpace(content)
	matches := make([]todoistadapter.Task, 0, 1)
	for _, task := range tasks {
		if strings.TrimSpace(task.Content) == want {
			matches = append(matches, task)
		}
	}
	return matches, nil
}

func defaultContent(now time.Time) string {
	return "Operator Wave 1 Todoist proof " + now.UTC().Format("20060102T150405.000000000Z")
}
