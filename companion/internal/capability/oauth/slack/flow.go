// Package slack implements Slack's user-only OAuth flow (v2_user authorize +
// oauth.v2.user.access). It never requests bot scopes or accepts bot tokens.
package slack

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const (
	defaultAuthorizeURL = "https://slack.com/oauth/v2_user/authorize"
	defaultTokenURL     = "https://slack.com/api/oauth.v2.user.access"
)

var (
	// Callers: Flow.Start, proving/slack.Authorize, serve-slack-proof. API: redirect validation.
	// User: "Change Slack OAuth redirect to http://127.0.0.1:PORT/oauth/slack/callback ... so no cert warning"
	ErrInvalidState       = errors.New("slack oauth: invalid or already used state")
	ErrNoScopes           = errors.New("slack oauth: the requested verbs need no supported scope")
	ErrMissingCredential  = errors.New("slack oauth: client id and client secret are required")
	ErrBotToken           = errors.New("slack oauth: refused a bot token; user OAuth is required")
	ErrInvalidRedirectURI = errors.New("slack oauth: redirect URI must be https, or http on 127.0.0.1/localhost")
)

type Config struct {
	ClientID     string
	ClientSecret string
	AuthorizeURL string
	TokenURL     string
	HTTPClient   *http.Client
	Logger       *slog.Logger
	RandomBytes  io.Reader
}

type Authorization struct {
	URL   string
	State string
}

type TokenSet struct {
	AccessToken string
	TokenType   string
	Scopes      []string
	TeamID      string
	TeamName    string
	UserID      string
	ExpiresAt   time.Time
}

type pendingAuthorization struct {
	redirectURI string
	scopes      []string
}

type Flow struct {
	clientID     string
	clientSecret string
	authorizeURL string
	tokenURL     string
	http         *http.Client
	logger       *slog.Logger
	random       io.Reader

	mu      sync.Mutex
	pending map[string]pendingAuthorization
}

func New(config Config) *Flow {
	if config.AuthorizeURL == "" {
		config.AuthorizeURL = defaultAuthorizeURL
	}
	if config.TokenURL == "" {
		config.TokenURL = defaultTokenURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.RandomBytes == nil {
		config.RandomBytes = rand.Reader
	}
	return &Flow{
		clientID: config.ClientID, clientSecret: config.ClientSecret,
		authorizeURL: config.AuthorizeURL, tokenURL: config.TokenURL,
		http: config.HTTPClient, logger: config.Logger, random: config.RandomBytes,
		pending: make(map[string]pendingAuthorization),
	}
}

func ScopesForVerbs(verbs []manifest.Verb) ([]string, error) {
	read := false
	send := false
	for _, verb := range verbs {
		switch verb {
		case manifest.Read:
			read = true
		case manifest.Send:
			send = true
		default:
			return nil, ErrNoScopes
		}
	}
	if !read && !send {
		return nil, ErrNoScopes
	}
	scopes := []string{"channels:read", "groups:read"}
	if send {
		scopes = append(scopes, "chat:write")
	}
	return scopes, nil
}

func validateRedirectURI(redirectURI string) (*url.URL, error) {
	parsedRedirect, err := url.Parse(redirectURI)
	if err != nil || parsedRedirect.Scheme == "" || parsedRedirect.Host == "" {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidRedirectURI, redirectURI)
	}
	host := strings.ToLower(parsedRedirect.Hostname())
	switch parsedRedirect.Scheme {
	case "https":
		return parsedRedirect, nil
	case "http":
		if host == "127.0.0.1" || host == "localhost" {
			return parsedRedirect, nil
		}
		return nil, fmt.Errorf("%w: http requires loopback host, got %q", ErrInvalidRedirectURI, host)
	default:
		return nil, fmt.Errorf("%w: got %q", ErrInvalidRedirectURI, redirectURI)
	}
}

func (f *Flow) Start(ctx context.Context, redirectURI string, verbs []manifest.Verb) (Authorization, error) {
	_ = ctx
	if strings.TrimSpace(f.clientID) == "" || strings.TrimSpace(f.clientSecret) == "" {
		return Authorization{}, ErrMissingCredential
	}
	if _, err := validateRedirectURI(redirectURI); err != nil {
		return Authorization{}, err
	}
	scopes, err := ScopesForVerbs(verbs)
	if err != nil {
		return Authorization{}, err
	}
	state, err := f.randomString(32)
	if err != nil {
		return Authorization{}, fmt.Errorf("slack oauth: generate state: %w", err)
	}
	f.mu.Lock()
	f.pending[state] = pendingAuthorization{redirectURI: redirectURI, scopes: scopes}
	f.mu.Unlock()

	query := url.Values{
		"client_id":     {f.clientID},
		"scope":         {strings.Join(scopes, ",")},
		"state":         {state},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
	}
	f.logger.Info("[slack-oauth] authorization ready",
		"scope_count", len(scopes), "redirect_host", safeHost(redirectURI), "user_only", true)
	return Authorization{URL: f.authorizeURL + "?" + query.Encode(), State: state}, nil
}

func (f *Flow) Callback(ctx context.Context, state, code string) (TokenSet, error) {
	f.mu.Lock()
	pending, ok := f.pending[state]
	if ok {
		delete(f.pending, state)
	}
	f.mu.Unlock()
	if !ok {
		f.logger.Warn("[slack-oauth] callback rejected", "reason", "invalid_state")
		return TokenSet{}, ErrInvalidState
	}
	form := url.Values{
		"client_id":     {f.clientID},
		"client_secret": {f.clientSecret},
		"code":          {code},
		"redirect_uri":  {pending.redirectURI},
		"grant_type":    {"authorization_code"},
	}
	return f.exchange(ctx, form, pending.scopes)
}

func (f *Flow) exchange(ctx context.Context, form url.Values, scopes []string) (TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, fmt.Errorf("slack oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := f.http.Do(req)
	if err != nil {
		f.logger.Error("[slack-oauth] token request failed", "error", err)
		return TokenSet{}, fmt.Errorf("slack oauth: token request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return TokenSet{}, fmt.Errorf("slack oauth: read token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		f.logger.Error("[slack-oauth] token rejected", "status", response.StatusCode)
		return TokenSet{}, fmt.Errorf("slack oauth: token endpoint returned status %d", response.StatusCode)
	}
	var raw struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		ExpiresIn   int    `json:"expires_in"`
		Team        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
		AuthedUser struct {
			ID string `json:"id"`
		} `json:"authed_user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return TokenSet{}, fmt.Errorf("slack oauth: decode token response: %w", err)
	}
	if !raw.OK || raw.AccessToken == "" {
		f.logger.Error("[slack-oauth] token response not ok", "slack_error", raw.Error)
		if raw.Error != "" {
			return TokenSet{}, fmt.Errorf("slack oauth: token endpoint error %s", raw.Error)
		}
		return TokenSet{}, errors.New("slack oauth: token response contained no access token")
	}
	tokenType := strings.ToLower(strings.TrimSpace(raw.TokenType))
	if tokenType == "bot" || strings.HasPrefix(raw.AccessToken, "xoxb-") {
		f.logger.Error("[slack-oauth] refused bot token", "token_type", tokenType)
		return TokenSet{}, ErrBotToken
	}
	if raw.Scope != "" {
		scopes = strings.Fields(strings.ReplaceAll(raw.Scope, ",", " "))
	}
	expiresAt := time.Time{}
	if raw.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	f.logger.Info("[slack-oauth] user token accepted",
		"scope_count", len(scopes), "team_id_present", raw.Team.ID != "", "user_id_present", raw.AuthedUser.ID != "")
	return TokenSet{
		AccessToken: raw.AccessToken, TokenType: raw.TokenType, Scopes: scopes,
		TeamID: raw.Team.ID, TeamName: raw.Team.Name, UserID: raw.AuthedUser.ID, ExpiresAt: expiresAt,
	}, nil
}

func (f *Flow) randomString(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(f.random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func safeHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "invalid"
	}
	return parsed.Host
}
