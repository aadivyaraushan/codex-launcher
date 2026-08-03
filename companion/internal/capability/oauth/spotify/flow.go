// Package spotify implements Spotify's confidential-client Authorization
// Code flow: https://developer.spotify.com/documentation/web-api/tutorials/code-flow.
// Unlike Todoist's public PKCE client, Spotify issues a client secret and
// requires it on every token request via HTTP Basic auth — never in the
// request body — and Spotify's redirect URI is pre-registered on the
// developer dashboard rather than dynamically registered per flow.
package spotify

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
	defaultAuthorizeURL = "https://accounts.spotify.com/authorize"
	defaultTokenURL     = "https://accounts.spotify.com/api/token"
)

var (
	ErrInvalidState      = errors.New("spotify oauth: invalid or already used state")
	ErrNoScopes          = errors.New("spotify oauth: the requested verbs need no supported scope")
	ErrMissingCredential = errors.New("spotify oauth: client id and secret are required")
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

// ScopesForVerbs derives the minimum Spotify scopes for the requested verbs.
// Play needs both playback-state scopes; Read alone needs only the
// read-only one. Write is never requested — playlist and library writes
// stay out of v1.
func ScopesForVerbs(verbs []manifest.Verb) ([]string, error) {
	read := false
	play := false
	for _, verb := range verbs {
		switch verb {
		case manifest.Read:
			read = true
		case manifest.Play:
			read = true
			play = true
		}
	}
	if play {
		return []string{"user-read-playback-state", "user-modify-playback-state"}, nil
	}
	if read {
		return []string{"user-read-playback-state"}, nil
	}
	return nil, ErrNoScopes
}

func (f *Flow) Start(_ context.Context, redirectURI string, verbs []manifest.Verb) (Authorization, error) {
	if f.clientID == "" || f.clientSecret == "" {
		return Authorization{}, ErrMissingCredential
	}
	scopes, err := ScopesForVerbs(verbs)
	if err != nil {
		return Authorization{}, err
	}
	state, err := f.randomString(32)
	if err != nil {
		return Authorization{}, fmt.Errorf("spotify oauth: generate state: %w", err)
	}

	f.mu.Lock()
	f.pending[state] = pendingAuthorization{redirectURI: redirectURI, scopes: scopes}
	f.mu.Unlock()

	query := url.Values{
		"client_id":     {f.clientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
	}
	f.logger.Info("[spotify-oauth] authorization ready", "scope_count", len(scopes), "redirect_host", safeHost(redirectURI))
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
		f.logger.Warn("[spotify-oauth] callback rejected", "reason", "invalid_state")
		return TokenSet{}, ErrInvalidState
	}
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {pending.redirectURI},
	}
	return f.exchange(ctx, form, pending.scopes, "authorization_code", "")
}

// Refresh rotates the access token. Spotify's client id is fixed on the
// Flow (there is one pre-registered developer app), so unlike Todoist's
// per-registration client, Refresh only needs the refresh token.
func (f *Flow) Refresh(ctx context.Context, refreshToken string) (TokenSet, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return f.exchange(ctx, form, nil, "refresh_token", refreshToken)
}

func (f *Flow) exchange(ctx context.Context, form url.Values, scopes []string, grantType, previousRefresh string) (TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, fmt.Errorf("spotify oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(f.clientID+":"+f.clientSecret)))
	response, err := f.http.Do(req)
	if err != nil {
		f.logger.Error("[spotify-oauth] token request failed", "grant_type", grantType, "error", err)
		return TokenSet{}, fmt.Errorf("spotify oauth: token request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		f.logger.Error("[spotify-oauth] token rejected", "grant_type", grantType, "status", response.StatusCode)
		return TokenSet{}, fmt.Errorf("spotify oauth: token endpoint returned status %d", response.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&raw); err != nil {
		return TokenSet{}, fmt.Errorf("spotify oauth: decode token response: %w", err)
	}
	if raw.AccessToken == "" {
		return TokenSet{}, errors.New("spotify oauth: token response contained no access token")
	}
	if raw.RefreshToken == "" {
		// Spotify's refresh response may omit refresh_token; the previous
		// one stays valid, so keep using it.
		raw.RefreshToken = previousRefresh
	}
	if raw.Scope != "" {
		scopes = strings.Fields(raw.Scope)
	}
	f.logger.Info("[spotify-oauth] token accepted", "grant_type", grantType, "scope_count", len(scopes), "expires_in_seconds", raw.ExpiresIn)
	return TokenSet{
		AccessToken: raw.AccessToken, RefreshToken: raw.RefreshToken,
		TokenType: raw.TokenType, Scopes: scopes, ExpiresAt: time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
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
