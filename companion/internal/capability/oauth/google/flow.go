// Package google implements Google's user OAuth authorization-code flow for
// Wave 1 Calendar (calendar.events) and Drive (drive.file only).
package google

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
	defaultAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURL     = "https://oauth2.googleapis.com/token"

	// ScopeCalendarEvents is the non-restricted Calendar events scope
	// (view and edit events). It is not the broad `calendar` scope.
	ScopeCalendarEvents = "https://www.googleapis.com/auth/calendar.events"
	// ScopeDriveFile is the non-restricted Drive scope for files the user
	// opens with this app or that the app creates.
	ScopeDriveFile = "https://www.googleapis.com/auth/drive.file"
)

var (
	ErrInvalidState      = errors.New("google oauth: invalid or already used state")
	ErrNoScopes          = errors.New("google oauth: the requested verbs need no supported scope")
	ErrMissingCredential = errors.New("google oauth: client id and client secret are required")
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
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scopes       []string
	ExpiresAt    time.Time
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

// ScopesForVerbs returns the Wave 1 Google ceilings for read/write. Send and
// other verbs are rejected so we never widen past calendar.events + drive.file.
func ScopesForVerbs(verbs []manifest.Verb) ([]string, error) {
	allowed := false
	for _, verb := range verbs {
		switch verb {
		case manifest.Read, manifest.Write:
			allowed = true
		default:
			return nil, ErrNoScopes
		}
	}
	if !allowed {
		return nil, ErrNoScopes
	}
	return []string{ScopeCalendarEvents, ScopeDriveFile}, nil
}

func (f *Flow) Start(ctx context.Context, redirectURI string, verbs []manifest.Verb) (Authorization, error) {
	_ = ctx
	if strings.TrimSpace(f.clientID) == "" || strings.TrimSpace(f.clientSecret) == "" {
		return Authorization{}, ErrMissingCredential
	}
	scopes, err := ScopesForVerbs(verbs)
	if err != nil {
		return Authorization{}, err
	}
	state, err := f.randomString(32)
	if err != nil {
		return Authorization{}, fmt.Errorf("google oauth: generate state: %w", err)
	}
	f.mu.Lock()
	f.pending[state] = pendingAuthorization{redirectURI: redirectURI, scopes: scopes}
	f.mu.Unlock()

	query := url.Values{
		"client_id":              {f.clientID},
		"redirect_uri":           {redirectURI},
		"response_type":          {"code"},
		"scope":                  {strings.Join(scopes, " ")},
		"state":                  {state},
		"access_type":            {"offline"},
		"include_granted_scopes": {"true"},
		"prompt":                 {"consent"},
	}
	f.logger.Info("[google-oauth] authorization ready",
		"scope_count", len(scopes), "redirect_host", safeHost(redirectURI), "user_oauth", true)
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
		f.logger.Warn("[google-oauth] callback rejected", "reason", "invalid_state")
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

// Refresh exchanges the long-lived offline token for a new access token.
// Google normally omits refresh_token from this response, so the existing
// value is carried forward unless Google explicitly rotates it.
func (f *Flow) Refresh(ctx context.Context, refreshToken string) (TokenSet, error) {
	if strings.TrimSpace(f.clientID) == "" || strings.TrimSpace(f.clientSecret) == "" {
		return TokenSet{}, ErrMissingCredential
	}
	if strings.TrimSpace(refreshToken) == "" {
		return TokenSet{}, errors.New("google oauth: refresh token is required")
	}
	form := url.Values{
		"client_id":     {f.clientID},
		"client_secret": {f.clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}
	tokens, err := f.exchange(ctx, form, nil)
	if err != nil {
		return TokenSet{}, err
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}
	return tokens, nil
}

func (f *Flow) exchange(ctx context.Context, form url.Values, scopes []string) (TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, fmt.Errorf("google oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := f.http.Do(req)
	if err != nil {
		f.logger.Error("[google-oauth] token request failed", "error", err)
		return TokenSet{}, fmt.Errorf("google oauth: token request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return TokenSet{}, fmt.Errorf("google oauth: read token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		f.logger.Error("[google-oauth] token rejected", "status", response.StatusCode)
		return TokenSet{}, fmt.Errorf("google oauth: token endpoint returned status %d", response.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return TokenSet{}, fmt.Errorf("google oauth: decode token response: %w", err)
	}
	if raw.Error != "" || raw.AccessToken == "" {
		f.logger.Error("[google-oauth] token response not ok", "google_error", raw.Error)
		if raw.Error != "" {
			return TokenSet{}, fmt.Errorf("google oauth: token endpoint error %s", raw.Error)
		}
		return TokenSet{}, errors.New("google oauth: token response contained no access token")
	}
	if raw.Scope != "" {
		scopes = strings.Fields(raw.Scope)
	}
	expiresAt := time.Time{}
	if raw.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	f.logger.Info("[google-oauth] user token accepted",
		"scope_count", len(scopes), "refresh_present", raw.RefreshToken != "", "expires_in_seconds", raw.ExpiresIn)
	return TokenSet{
		AccessToken: raw.AccessToken, RefreshToken: raw.RefreshToken,
		TokenType: raw.TokenType, Scopes: scopes, ExpiresAt: expiresAt,
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
