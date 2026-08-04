package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
)

const APIBaseURL = "https://slack.com"

var ErrTokenCannotBeCleared = errors.New("slack: token source cannot clear its token")

type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"is_archived"`
}

type PostMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type PostedMessage struct {
	Channel   string `json:"channel"`
	Timestamp string `json:"ts"`
}

// WorkspaceIdentity is Slack's authenticated account and workspace result.
type WorkspaceIdentity struct {
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
}

type TokenSource interface {
	AccessToken(context.Context) (string, error)
}

type tokenClearer interface {
	Clear(context.Context) error
}

type HTTPClient struct {
	baseURL string
	tokens  TokenSource
	http    *http.Client
	logger  *slog.Logger
}

var _ API = (*HTTPClient)(nil)

func NewHTTPClient(baseURL string, tokens TokenSource, client *http.Client, logger *slog.Logger) *HTTPClient {
	if baseURL == "" {
		baseURL = APIBaseURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPClient{baseURL: strings.TrimRight(baseURL, "/"), tokens: tokens, http: client, logger: logger}
}

func (c *HTTPClient) ListChannels(ctx context.Context) ([]Channel, error) {
	var channels []Channel
	cursor := ""
	seen := map[string]bool{}
	for {
		query := url.Values{
			"exclude_archived": {"true"},
			"limit":            {"200"},
			"types":            {"public_channel,private_channel"},
		}
		if cursor != "" {
			if seen[cursor] {
				return nil, errors.New("slack: repeated pagination cursor")
			}
			seen[cursor] = true
			query.Set("cursor", cursor)
		}
		var page struct {
			OK               bool      `json:"ok"`
			Error            string    `json:"error"`
			Channels         []Channel `json:"channels"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := c.do(ctx, http.MethodGet, "/api/conversations.list?"+query.Encode(), nil, &page); err != nil {
			return nil, err
		}
		if !page.OK {
			return nil, fmt.Errorf("slack: conversations.list error %s", page.Error)
		}
		channels = append(channels, page.Channels...)
		if page.ResponseMetadata.NextCursor == "" {
			c.logger.Info("[slack] list complete", "channel_count", len(channels), "page_count", len(seen)+1)
			return channels, nil
		}
		cursor = page.ResponseMetadata.NextCursor
	}
}

func (c *HTTPClient) PostMessage(ctx context.Context, request PostMessage) (PostedMessage, error) {
	var response struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/chat.postMessage", request, &response); err != nil {
		return PostedMessage{}, err
	}
	if !response.OK {
		return PostedMessage{}, fmt.Errorf("slack: chat.postMessage error %s", response.Error)
	}
	return PostedMessage{Channel: response.Channel, Timestamp: response.TS}, nil
}

// Identity uses Slack's no-extra-scope auth.test endpoint to verify which
// workspace and user the OAuth token actually represents.
func (c *HTTPClient) Identity(ctx context.Context) (WorkspaceIdentity, error) {
	var response struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		WorkspaceIdentity
	}
	if err := c.do(ctx, http.MethodPost, "/api/auth.test", map[string]any{}, &response); err != nil {
		return WorkspaceIdentity{}, err
	}
	if !response.OK {
		return WorkspaceIdentity{}, fmt.Errorf("slack: auth.test error %s", response.Error)
	}
	c.logger.Info("[slack] identity verified", "team_id_present", response.TeamID != "", "user_id_present", response.UserID != "")
	return response.WorkspaceIdentity, nil
}

func (c *HTTPClient) Clear(ctx context.Context) error {
	clearer, ok := c.tokens.(tokenClearer)
	if !ok {
		return ErrTokenCannotBeCleared
	}
	// Best-effort Slack-side revoke; local clear still happens either way.
	_ = c.revokeRemote(ctx)
	return clearer.Clear(ctx)
}

func (c *HTTPClient) revokeRemote(ctx context.Context) error {
	var response struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Revoked bool   `json:"revoked"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/auth.revoke", map[string]any{}, &response); err != nil {
		c.logger.Warn("[slack] remote revoke request failed", "error", err)
		return err
	}
	if !response.OK {
		c.logger.Warn("[slack] remote revoke rejected", "slack_error", response.Error)
		return fmt.Errorf("slack: auth.revoke error %s", response.Error)
	}
	return nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body, result any) error {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("slack: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		var encoded strings.Builder
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return fmt.Errorf("slack: encode request: %w", err)
		}
		reader = strings.NewReader(encoded.String())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cleanPath := strings.Split(path, "?")[0]
	c.logger.Info("[slack] request", "method", method, "path", cleanPath, "has_body", body != nil)
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[slack] request failed", "method", method, "path", cleanPath, "error", err)
		wrapped := fmt.Errorf("slack: %s request failed: %w", method, err)
		return adapter.ClassifyHTTPFailure(ID, method, wrapped)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[slack] response rejected", "method", method, "path", cleanPath, "status", response.StatusCode)
		return fmt.Errorf("slack: %s %s returned status %d", method, cleanPath, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("slack: decode %s response: %w", method, err)
	}
	return nil
}
