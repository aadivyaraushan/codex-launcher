// Package notion implements Notion hosted MCP's OAuth 2.0 discovery,
// dynamic client registration, PKCE authorization, and token rotation.
package notion

import (
	"bytes"
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
)

const DefaultServerURL = "https://mcp.notion.com/mcp"

var ErrInvalidState = errors.New("notion oauth: invalid or already used state")

type Config struct {
	ServerURL   string
	HTTPClient  *http.Client
	RandomBytes io.Reader
	Logger      *slog.Logger
}

type ProtectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

type Metadata struct {
	Resource                      string   `json:"resource"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

type RegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type ClientRegistration struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

type Authorization struct {
	URL   string
	State string
}

type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresIn    int       `json:"expires_in,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	WorkspaceID  string    `json:"workspace_id,omitempty"`
	ExpiresAt    time.Time `json:"-"`
}

type pendingAuthorization struct {
	metadata    Metadata
	client      ClientRegistration
	redirectURI string
	verifier    string
}

type Flow struct {
	serverURL string
	http      *http.Client
	random    io.Reader
	logger    *slog.Logger

	mu      sync.Mutex
	pending map[string]pendingAuthorization
}

func New(config Config) *Flow {
	if strings.TrimSpace(config.ServerURL) == "" {
		config.ServerURL = DefaultServerURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.RandomBytes == nil {
		config.RandomBytes = rand.Reader
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Flow{serverURL: strings.TrimRight(config.ServerURL, "/"), http: config.HTTPClient, random: config.RandomBytes, logger: config.Logger, pending: map[string]pendingAuthorization{}}
}

func (f *Flow) Discover(ctx context.Context) (Metadata, error) {
	parsed, err := url.Parse(f.serverURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Metadata{}, errors.New("notion oauth: invalid MCP server URL")
	}
	origin := parsed.Scheme + "://" + parsed.Host
	candidates := []string{f.serverURL + "/.well-known/oauth-protected-resource", origin + "/.well-known/oauth-protected-resource"}
	var protected ProtectedResourceMetadata
	var discoveryErr error
	for _, endpoint := range candidates {
		if err := f.getJSON(ctx, endpoint, &protected); err == nil && len(protected.AuthorizationServers) > 0 {
			discoveryErr = nil
			break
		} else {
			discoveryErr = err
		}
	}
	if discoveryErr != nil || len(protected.AuthorizationServers) == 0 {
		return Metadata{}, fmt.Errorf("notion oauth: protected-resource discovery failed: %w", discoveryErr)
	}
	authServer := strings.TrimRight(protected.AuthorizationServers[0], "/")
	var metadata Metadata
	if err := f.getJSON(ctx, authServer+"/.well-known/oauth-authorization-server", &metadata); err != nil {
		return Metadata{}, fmt.Errorf("notion oauth: authorization-server discovery failed: %w", err)
	}
	metadata.Resource = protected.Resource
	if metadata.Resource == "" {
		metadata.Resource = origin
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" || metadata.RegistrationEndpoint == "" {
		return Metadata{}, errors.New("notion oauth: discovery omitted required endpoints")
	}
	f.logger.Info("[notion-oauth] discovery complete", "authorization_host", safeHost(metadata.AuthorizationEndpoint), "registration_present", true)
	return metadata, nil
}

func (f *Flow) Register(ctx context.Context, metadata Metadata, redirectURI string) (ClientRegistration, error) {
	request := RegistrationRequest{
		ClientName: "Operator", RedirectURIs: []string{redirectURI},
		GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
		TokenEndpointAuthMethod: "none",
	}
	var registration ClientRegistration
	if err := f.postJSON(ctx, metadata.RegistrationEndpoint, request, &registration); err != nil {
		return ClientRegistration{}, fmt.Errorf("notion oauth: dynamic registration failed: %w", err)
	}
	if strings.TrimSpace(registration.ClientID) == "" {
		return ClientRegistration{}, errors.New("notion oauth: registration returned no client id")
	}
	f.logger.Info("[notion-oauth] client registered", "client_id_present", true, "redirect_host", safeHost(redirectURI))
	return registration, nil
}

func (f *Flow) Start(_ context.Context, metadata Metadata, client ClientRegistration, redirectURI string) (Authorization, error) {
	state, err := f.randomString(32)
	if err != nil {
		return Authorization{}, err
	}
	verifier, err := f.randomString(32)
	if err != nil {
		return Authorization{}, err
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	values := url.Values{
		"client_id": {client.ClientID}, "redirect_uri": {redirectURI}, "response_type": {"code"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
		"resource": {metadata.Resource},
	}
	if len(metadata.Resource) > 0 {
		values.Set("scope", "default")
	}
	f.mu.Lock()
	f.pending[state] = pendingAuthorization{metadata: metadata, client: client, redirectURI: redirectURI, verifier: verifier}
	f.mu.Unlock()
	f.logger.Info("[notion-oauth] authorization ready", "redirect_host", safeHost(redirectURI), "pkce", "S256")
	return Authorization{URL: metadata.AuthorizationEndpoint + "?" + values.Encode(), State: state}, nil
}

func (f *Flow) Callback(ctx context.Context, state, code string) (TokenSet, error) {
	f.mu.Lock()
	pending, ok := f.pending[state]
	if ok {
		delete(f.pending, state)
	}
	f.mu.Unlock()
	if !ok {
		return TokenSet{}, ErrInvalidState
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {pending.client.ClientID},
		"redirect_uri": {pending.redirectURI}, "code_verifier": {pending.verifier}, "resource": {pending.metadata.Resource},
	}
	return f.exchange(ctx, pending.metadata.TokenEndpoint, pending.client, form)
}

func (f *Flow) Refresh(ctx context.Context, metadata Metadata, client ClientRegistration, refreshToken string) (TokenSet, error) {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}, "client_id": {client.ClientID}, "resource": {metadata.Resource}}
	return f.exchange(ctx, metadata.TokenEndpoint, client, form)
}

func (f *Flow) exchange(ctx context.Context, endpoint string, client ClientRegistration, form url.Values) (TokenSet, error) {
	if client.ClientSecret != "" {
		form.Set("client_secret", client.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := f.http.Do(req)
	if err != nil {
		return TokenSet{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return TokenSet{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var provider struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &provider)
		return TokenSet{}, fmt.Errorf("notion oauth: token endpoint status %d error %s", response.StatusCode, provider.Error)
	}
	var tokens TokenSet
	if err := json.Unmarshal(body, &tokens); err != nil {
		return TokenSet{}, fmt.Errorf("notion oauth: decode token response: %w", err)
	}
	if tokens.AccessToken == "" {
		return TokenSet{}, errors.New("notion oauth: token response contained no access token")
	}
	if tokens.ExpiresIn > 0 {
		tokens.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}
	f.logger.Info("[notion-oauth] token accepted", "refresh_present", tokens.RefreshToken != "", "workspace_id_present", tokens.WorkspaceID != "", "expires_in_seconds", tokens.ExpiresIn)
	return tokens, nil
}

func (f *Flow) getJSON(ctx context.Context, endpoint string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return f.doJSON(req, result)
}

func (f *Flow) postJSON(ctx context.Context, endpoint string, body, result any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return f.doJSON(req, result)
}

func (f *Flow) doJSON(req *http.Request, result any) error {
	response, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("%s returned status %d", req.URL.Host, response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result)
}

func (f *Flow) randomString(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(f.random, raw); err != nil {
		return "", fmt.Errorf("notion oauth: random value: %w", err)
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
