// Package microsoft implements Microsoft identity platform user OAuth
// (authorization code) for Wave 1 personal Outlook mail and work/school
// Teams chat via Graph.
package microsoft

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
	defaultTenant = "consumers"
	// TenantOrganizations is the work/school tenant for Teams Graph chat.
	// Outlook personal mail proof stays on consumers.
	TenantOrganizations = "organizations"

	// ScopeMailRead is delegated mailbox read for the signed-in user.
	ScopeMailRead = "Mail.Read"
	// ScopeMailReadWrite is delegated mailbox read/write (drafts).
	ScopeMailReadWrite = "Mail.ReadWrite"
	// ScopeMailSend is delegated send-as-user.
	ScopeMailSend = "Mail.Send"
	// ScopeChatReadWrite is delegated Teams chat read/send for work/school.
	ScopeChatReadWrite = "Chat.ReadWrite"
	// ScopeOfflineAccess requests a refresh token.
	ScopeOfflineAccess = "offline_access"
	// ScopeUserRead is the basic signed-in profile scope Graph expects.
	ScopeUserRead = "User.Read"
)

var (
	ErrInvalidState            = errors.New("microsoft oauth: invalid or already used state")
	ErrNoScopes                = errors.New("microsoft oauth: the requested verbs need no supported scope")
	ErrMissingCredential       = errors.New("microsoft oauth: client id and client secret are required")
	ErrLoopbackRequired        = errors.New("microsoft oauth: redirect URI must be loopback (127.0.0.1 or localhost)")
	ErrConsumersTenantForChat  = errors.New("microsoft oauth: Teams chat authorize cannot use consumers tenant")
)

type Config struct {
	ClientID     string
	ClientSecret string
	Tenant       string
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
	tenant       string
	authorizeURL string
	tokenURL     string
	http         *http.Client
	logger       *slog.Logger
	random       io.Reader

	mu      sync.Mutex
	pending map[string]pendingAuthorization
}

func New(config Config) *Flow {
	tenant := strings.TrimSpace(config.Tenant)
	if tenant == "" {
		tenant = defaultTenant
	}
	if config.AuthorizeURL == "" {
		config.AuthorizeURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenant)
	}
	if config.TokenURL == "" {
		config.TokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenant)
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
		clientID: config.ClientID, clientSecret: config.ClientSecret, tenant: tenant,
		authorizeURL: config.AuthorizeURL, tokenURL: config.TokenURL,
		http: config.HTTPClient, logger: config.Logger, random: config.RandomBytes,
		pending: make(map[string]pendingAuthorization),
	}
}

// ScopesForVerbs returns the Wave 1 Outlook mail ceilings. Read stays
// Mail.Read-only; write/send add Mail.ReadWrite and Mail.Send.
func ScopesForVerbs(verbs []manifest.Verb) ([]string, error) {
	read := false
	write := false
	send := false
	for _, verb := range verbs {
		switch verb {
		case manifest.Read:
			read = true
		case manifest.Write:
			write = true
		case manifest.Send:
			send = true
		default:
			return nil, ErrNoScopes
		}
	}
	if !read && !write && !send {
		return nil, ErrNoScopes
	}
	scopes := []string{ScopeOfflineAccess, ScopeUserRead}
	switch {
	case write || send:
		scopes = append(scopes, ScopeMailReadWrite)
		if send {
			scopes = append(scopes, ScopeMailSend)
		}
	default:
		scopes = append(scopes, ScopeMailRead)
	}
	return scopes, nil
}

// ChatScopesForVerbs returns Wave 1 Teams work/school chat ceilings.
// Read+Send map to Chat.ReadWrite; mail ScopesForVerbs stays separate.
func ChatScopesForVerbs(verbs []manifest.Verb) ([]string, error) {
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
	return []string{ScopeOfflineAccess, ScopeUserRead, ScopeChatReadWrite}, nil
}

func (f *Flow) Start(ctx context.Context, redirectURI string, verbs []manifest.Verb) (Authorization, error) {
	scopes, err := ScopesForVerbs(verbs)
	if err != nil {
		return Authorization{}, err
	}
	return f.start(ctx, redirectURI, scopes)
}

// StartChat builds an authorize URL for Teams work/school Chat.ReadWrite.
// Callers must configure Tenant to organizations (or a specific work tenant),
// not consumers.
func (f *Flow) StartChat(ctx context.Context, redirectURI string, verbs []manifest.Verb) (Authorization, error) {
	if strings.EqualFold(strings.TrimSpace(f.tenant), defaultTenant) ||
		strings.Contains(strings.ToLower(f.authorizeURL), "/consumers/") {
		return Authorization{}, ErrConsumersTenantForChat
	}
	scopes, err := ChatScopesForVerbs(verbs)
	if err != nil {
		return Authorization{}, err
	}
	return f.start(ctx, redirectURI, scopes)
}

func (f *Flow) start(ctx context.Context, redirectURI string, scopes []string) (Authorization, error) {
	_ = ctx
	if strings.TrimSpace(f.clientID) == "" || strings.TrimSpace(f.clientSecret) == "" {
		return Authorization{}, ErrMissingCredential
	}
	parsedRedirect, err := url.Parse(redirectURI)
	if err != nil || (parsedRedirect.Scheme != "http" && parsedRedirect.Scheme != "https") {
		return Authorization{}, fmt.Errorf("%w: got %q", ErrLoopbackRequired, redirectURI)
	}
	host := strings.ToLower(parsedRedirect.Hostname())
	if host != "127.0.0.1" && host != "localhost" {
		return Authorization{}, fmt.Errorf("%w: host=%q", ErrLoopbackRequired, host)
	}
	state, err := f.randomString(32)
	if err != nil {
		return Authorization{}, fmt.Errorf("microsoft oauth: generate state: %w", err)
	}
	f.mu.Lock()
	f.pending[state] = pendingAuthorization{redirectURI: redirectURI, scopes: scopes}
	f.mu.Unlock()

	query := url.Values{
		"client_id":     {f.clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"response_mode": {"query"},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
	}
	f.logger.Info("[microsoft-oauth] authorization ready",
		"scope_count", len(scopes), "redirect_host", safeHost(redirectURI),
		"tenant", f.tenant, "user_oauth", true)
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
		f.logger.Warn("[microsoft-oauth] callback rejected", "reason", "invalid_state")
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
		return TokenSet{}, fmt.Errorf("microsoft oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := f.http.Do(req)
	if err != nil {
		f.logger.Error("[microsoft-oauth] token request failed", "error", err)
		return TokenSet{}, fmt.Errorf("microsoft oauth: token request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return TokenSet{}, fmt.Errorf("microsoft oauth: read token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		f.logger.Error("[microsoft-oauth] token rejected", "status", response.StatusCode)
		return TokenSet{}, fmt.Errorf("microsoft oauth: token endpoint returned status %d", response.StatusCode)
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
		return TokenSet{}, fmt.Errorf("microsoft oauth: decode token response: %w", err)
	}
	if raw.Error != "" || raw.AccessToken == "" {
		f.logger.Error("[microsoft-oauth] token response not ok", "microsoft_error", raw.Error)
		if raw.Error != "" {
			return TokenSet{}, fmt.Errorf("microsoft oauth: token endpoint error %s", raw.Error)
		}
		return TokenSet{}, errors.New("microsoft oauth: token response contained no access token")
	}
	if raw.Scope != "" {
		scopes = strings.Fields(raw.Scope)
	}
	expiresAt := time.Time{}
	if raw.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	f.logger.Info("[microsoft-oauth] user token accepted",
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
