// Package slack drives the owner-only Slack user OAuth stop checkpoint:
// HTTPS callback, in-memory token, channel read, and revoke.
package slack

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	slackoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

var (
	ErrMissingDependencies = errors.New("slack proof: flow and API factory are required")
	ErrHTTPSRequired       = errors.New("slack proof: Slack requires an HTTPS redirect URI")
)

type OAuthFlow interface {
	Start(context.Context, string, []manifest.Verb) (slackoauth.Authorization, error)
	Callback(context.Context, string, string) (slackoauth.TokenSet, error)
}

type Config struct {
	ListenAddress string
	RedirectURI   string
	Flow          OAuthFlow
	NewAPI        func(*Connection) slackadapter.API
	Output        io.Writer
	Logger        *slog.Logger
	Channel       string
}

type AuthorizationConfig struct {
	ListenAddress string
	RedirectURI   string
	Flow          OAuthFlow
	Output        io.Writer
	Logger        *slog.Logger
	TLSConfig     *tls.Config
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
		return "", slackadapter.ErrNotConnected
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
		return ErrMissingDependencies
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if strings.TrimSpace(config.Channel) == "" {
		config.Channel = "general"
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
	api := config.NewAPI(connection)
	a := slackadapter.New(api, config.Logger)
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("slack proof: register adapter: %w", err)
	}
	runner := execution.New(reg)

	readPlan, err := runner.Resolve(ctx, adapter.Intent{
		AdapterID: slackadapter.ID, Verb: manifest.Read, Subject: config.Channel,
	})
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("slack proof: resolve read: %w", err)
	}
	readOutcome, err := runner.Execute(ctx, readPlan, execution.Confirmation{})
	if err != nil {
		_ = connection.Clear(ctx)
		return fmt.Errorf("slack proof: execute read: %w", err)
	}
	fmt.Fprintf(config.Output, "READ: reached=%s done=%t detail=%s\n", readOutcome.Reached, readOutcome.Done, readOutcome.Detail)
	if err := runner.Revoke(ctx, slackadapter.ID); err != nil {
		return fmt.Errorf("slack proof: revoke: %w", err)
	}
	fmt.Fprintln(config.Output, "REVOKE: in-memory access token cleared and adapter removed from the registry")
	fmt.Fprintln(config.Output, "VERDICT: slack user OAuth proven by channel read and revoke (send left gated behind preview)")
	return nil
}

// Authorize completes the owner-only OAuth callback over HTTPS and returns an
// in-memory token source. Callers must close it when their serving process stops.
func Authorize(ctx context.Context, config AuthorizationConfig) (*Connection, error) {
	if config.Flow == nil {
		return nil, errors.New("slack proof: OAuth flow is required")
	}
	if config.ListenAddress == "" {
		config.ListenAddress = "127.0.0.1:9192"
	}
	if config.RedirectURI == "" {
		config.RedirectURI = "https://127.0.0.1:9192/oauth/slack/callback"
	}
	if config.Output == nil {
		config.Output = io.Discard
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	parsed, err := url.Parse(config.RedirectURI)
	if err != nil || parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: got %q", ErrHTTPSRequired, config.RedirectURI)
	}

	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("slack proof: listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	if strings.Contains(config.RedirectURI, ":0/") || strings.HasSuffix(config.ListenAddress, ":0") {
		// Tests bind an ephemeral port; rebuild the HTTPS redirect to match it.
		config.RedirectURI = "https://" + listener.Addr().String() + "/oauth/slack/callback"
		parsed, err = url.Parse(config.RedirectURI)
		if err != nil {
			return nil, fmt.Errorf("slack proof: rebuild redirect URI: %w", err)
		}
	}

	tlsConfig := config.TLSConfig
	if tlsConfig == nil {
		tlsConfig, err = selfSignedTLSConfig(parsed.Hostname())
		if err != nil {
			return nil, fmt.Errorf("slack proof: build local TLS cert: %w", err)
		}
	}
	tlsListener := tls.NewListener(listener, tlsConfig)

	authorization, err := config.Flow.Start(ctx, config.RedirectURI, []manifest.Verb{manifest.Read})
	if err != nil {
		return nil, fmt.Errorf("slack proof: start OAuth: %w", err)
	}

	tokens := make(chan slackoauth.TokenSet, 1)
	callbackErrors := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/slack/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		set, callbackErr := config.Flow.Callback(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if callbackErr != nil {
			config.Logger.Warn("[slack-proof] OAuth callback rejected", "error", callbackErr)
			http.Error(w, "Slack sign-in could not be accepted. Return to Operator and try again.", http.StatusBadRequest)
			select {
			case callbackErrors <- callbackErr:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "Slack is connected. You can return to Operator.")
		select {
		case tokens <- set:
		default:
		}
	})
	server := &http.Server{Handler: mux}
	serveErrors := make(chan error, 1)
	go func() {
		if serveErr := server.Serve(tlsListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serveErrors <- serveErr
		}
	}()
	defer server.Shutdown(context.Background())

	fmt.Fprintf(config.Output, "AUTH_URL=%s\n", authorization.URL)
	fmt.Fprintf(config.Output, "REDIRECT_URI=%s\n", config.RedirectURI)
	fmt.Fprintln(config.Output, "Open that URL, approve Slack as yourself (user OAuth), and return here after the browser says it is connected.")
	fmt.Fprintln(config.Output, "NOTE: callback uses a local self-signed HTTPS cert; browsers may warn before continuing.")
	config.Logger.Info("[slack-proof] waiting for OAuth", "callback_host", listener.Addr().String(), "https", true)

	var tokenSet slackoauth.TokenSet
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("slack proof: wait for OAuth: %w", ctx.Err())
	case err := <-serveErrors:
		return nil, fmt.Errorf("slack proof: callback server: %w", err)
	case err := <-callbackErrors:
		return nil, fmt.Errorf("slack proof: OAuth callback: %w", err)
	case tokenSet = <-tokens:
	}
	return &Connection{access: tokenSet.AccessToken}, nil
}

func selfSignedTLSConfig(host string) (*tls.Config, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Operator Slack OAuth"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
		template.DNSNames = nil
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}, nil
}
