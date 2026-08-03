// Package todoist implements Todoist's public-client OAuth flow with dynamic
// client registration and PKCE. It never creates or stores a client secret.
package todoist

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	defaultAPIBaseURL = "https://api.todoist.com"
	defaultAppBaseURL = "https://app.todoist.com"
)

var (
	ErrInvalidState = errors.New("todoist oauth: invalid or already used state")
	ErrNoScopes     = errors.New("todoist oauth: the requested verbs need no supported scope")
)

type Config struct {
	APIBaseURL  string
	AppBaseURL  string
	ClientName  string
	HTTPClient  *http.Client
	Logger      *slog.Logger
	RandomBytes io.Reader
}

type Authorization struct {
	URL   string
	State string
}

type TokenSet struct {
	ClientID     string
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scopes       []string
	ExpiresAt    time.Time
}

type pendingAuthorization struct {
	clientID    string
	redirectURI string
	verifier    string
	scopes      []string
}

type Flow struct {
	apiBaseURL string
	appBaseURL string
	clientName string
	http       *http.Client
	logger     *slog.Logger
	random     io.Reader

	mu      sync.Mutex
	pending map[string]pendingAuthorization
}

func New(config Config) *Flow {
	if config.APIBaseURL == "" {
		config.APIBaseURL = defaultAPIBaseURL
	}
	if config.AppBaseURL == "" {
		config.AppBaseURL = defaultAppBaseURL
	}
	if config.ClientName == "" {
		config.ClientName = "Operator"
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
		apiBaseURL: strings.TrimRight(config.APIBaseURL, "/"),
		appBaseURL: strings.TrimRight(config.AppBaseURL, "/"),
		clientName: config.ClientName, http: config.HTTPClient, logger: config.Logger,
		random: config.RandomBytes, pending: make(map[string]pendingAuthorization),
	}
}

func ScopesForVerbs(verbs []manifest.Verb) ([]string, error) {
	write := false
	read := false
	for _, verb := range verbs {
		switch verb {
		case manifest.Write:
			write = true
		case manifest.Read:
			read = true
		}
	}
	if write {
		return []string{"data:read_write"}, nil
	}
	if read {
		return []string{"data:read"}, nil
	}
	return nil, ErrNoScopes
}

func (f *Flow) Start(ctx context.Context, redirectURI string, verbs []manifest.Verb) (Authorization, error) {
	scopes, err := ScopesForVerbs(verbs)
	if err != nil {
		return Authorization{}, err
	}
	clientID, err := f.register(ctx, redirectURI)
	if err != nil {
		return Authorization{}, err
	}
	state, err := f.randomString(32)
	if err != nil {
		return Authorization{}, fmt.Errorf("todoist oauth: generate state: %w", err)
	}
	verifier, err := f.randomString(64)
	if err != nil {
		return Authorization{}, fmt.Errorf("todoist oauth: generate PKCE verifier: %w", err)
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])

	f.mu.Lock()
	f.pending[state] = pendingAuthorization{clientID: clientID, redirectURI: redirectURI, verifier: verifier, scopes: scopes}
	f.mu.Unlock()

	query := url.Values{
		"client_id":             {clientID},
		"scope":                 {strings.Join(scopes, ",")},
		"state":                 {state},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	f.logger.Info("[todoist-oauth] authorization ready", "scope_count", len(scopes), "redirect_host", safeHost(redirectURI))
	return Authorization{URL: f.appBaseURL + "/oauth/authorize?" + query.Encode(), State: state}, nil
}

func (f *Flow) Callback(ctx context.Context, state, code string) (TokenSet, error) {
	f.mu.Lock()
	pending, ok := f.pending[state]
	if ok {
		delete(f.pending, state)
	}
	f.mu.Unlock()
	if !ok {
		f.logger.Warn("[todoist-oauth] callback rejected", "reason", "invalid_state")
		return TokenSet{}, ErrInvalidState
	}
	form := url.Values{
		"client_id":     {pending.clientID},
		"code":          {code},
		"redirect_uri":  {pending.redirectURI},
		"code_verifier": {pending.verifier},
		"grant_type":    {"authorization_code"},
	}
	return f.exchange(ctx, form, pending.clientID, pending.scopes, "authorization_code", "")
}

func (f *Flow) Refresh(ctx context.Context, clientID, refreshToken string) (TokenSet, error) {
	form := url.Values{
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}
	return f.exchange(ctx, form, clientID, nil, "refresh_token", refreshToken)
}

func (f *Flow) register(ctx context.Context, redirectURI string) (string, error) {
	payload := map[string]any{
		"client_name": f.clientName, "redirect_uris": []string{redirectURI},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	}
	var response struct {
		ClientID string `json:"client_id"`
	}
	if err := f.jsonRequest(ctx, http.MethodPost, "/oauth/register", payload, &response); err != nil {
		return "", err
	}
	if response.ClientID == "" {
		return "", errors.New("todoist oauth: registration returned no client id")
	}
	f.logger.Info("[todoist-oauth] public client registered", "redirect_host", safeHost(redirectURI))
	return response.ClientID, nil
}

func (f *Flow) exchange(ctx context.Context, form url.Values, clientID string, scopes []string, grantType, previousRefresh string) (TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.apiBaseURL+"/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, fmt.Errorf("todoist oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := f.http.Do(req)
	if err != nil {
		f.logger.Error("[todoist-oauth] token request failed", "grant_type", grantType, "error", err)
		return TokenSet{}, fmt.Errorf("todoist oauth: token request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		f.logger.Error("[todoist-oauth] token rejected", "grant_type", grantType, "status", response.StatusCode)
		return TokenSet{}, fmt.Errorf("todoist oauth: token endpoint returned status %d", response.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&raw); err != nil {
		return TokenSet{}, fmt.Errorf("todoist oauth: decode token response: %w", err)
	}
	if raw.AccessToken == "" {
		return TokenSet{}, errors.New("todoist oauth: token response contained no access token")
	}
	if raw.RefreshToken == "" {
		raw.RefreshToken = previousRefresh
	}
	if raw.Scope != "" {
		scopes = strings.Fields(strings.ReplaceAll(raw.Scope, ",", " "))
	}
	f.logger.Info("[todoist-oauth] token accepted", "grant_type", grantType, "scope_count", len(scopes), "expires_in_seconds", raw.ExpiresIn)
	return TokenSet{
		ClientID: clientID, AccessToken: raw.AccessToken, RefreshToken: raw.RefreshToken,
		TokenType: raw.TokenType, Scopes: scopes, ExpiresAt: time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}, nil
}

func (f *Flow) jsonRequest(ctx context.Context, method, path string, payload, result any) error {
	var encoded strings.Builder
	if err := json.NewEncoder(&encoded).Encode(payload); err != nil {
		return fmt.Errorf("todoist oauth: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, f.apiBaseURL+path, strings.NewReader(encoded.String()))
	if err != nil {
		return fmt.Errorf("todoist oauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := f.http.Do(req)
	if err != nil {
		return fmt.Errorf("todoist oauth: request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("todoist oauth: registration returned status %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("todoist oauth: decode registration: %w", err)
	}
	return nil
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
