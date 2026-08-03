package msteams

// Docs: Context7 /websites/learn_microsoft_en-us_graph —
// GET /me/chats, POST /chats/{chat-id}/messages.
// Personal Microsoft accounts are not supported for these chat APIs.

import (
	"bytes"
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

const APIBaseURL = "https://graph.microsoft.com"

var ErrTokenCannotBeCleared = errors.New("msteams: token source cannot clear its token")

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

func (c *HTTPClient) ListChats(ctx context.Context) ([]Chat, error) {
	values := url.Values{
		"$top":    {"50"},
		"$select": {"id,topic,chatType"},
	}
	var page struct {
		Value []struct {
			ID       string `json:"id"`
			Topic    string `json:"topic"`
			ChatType string `json:"chatType"`
		} `json:"value"`
	}
	path := "/v1.0/me/chats?" + values.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	chats := make([]Chat, 0, len(page.Value))
	for _, item := range page.Value {
		chats = append(chats, Chat{ID: item.ID, Topic: item.Topic})
	}
	c.logger.Info("[msteams] list complete", "chat_count", len(chats))
	return chats, nil
}

func (c *HTTPClient) SendMessage(ctx context.Context, request SendMessage) (SentMessage, error) {
	chatID := strings.TrimSpace(request.ChatID)
	if chatID == "" {
		return SentMessage{}, ErrEmptyChat
	}
	payload := map[string]any{
		"body": map[string]string{
			"content": request.Body,
		},
	}
	var raw struct {
		ID     string `json:"id"`
		ChatID string `json:"chatId"`
	}
	path := "/v1.0/chats/" + url.PathEscape(chatID) + "/messages"
	if err := c.do(ctx, http.MethodPost, path, payload, &raw); err != nil {
		return SentMessage{}, err
	}
	if raw.ChatID == "" {
		raw.ChatID = chatID
	}
	return SentMessage{ID: raw.ID, ChatID: raw.ChatID}, nil
}

func (c *HTTPClient) Clear(ctx context.Context) error {
	clearer, ok := c.tokens.(tokenClearer)
	if !ok {
		return ErrTokenCannotBeCleared
	}
	return clearer.Clear(ctx)
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body, result any) error {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("msteams: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return fmt.Errorf("msteams: encode request: %w", encodeErr)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("msteams: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cleanPath := strings.Split(path, "?")[0]
	c.logger.Info("[msteams] request", "method", method, "path", cleanPath, "has_body", body != nil)
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[msteams] request failed", "method", method, "path", cleanPath, "error", err)
		wrapped := fmt.Errorf("msteams: %s request failed: %w", method, err)
		return adapter.ClassifyHTTPFailure(ID, method, wrapped)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[msteams] response rejected", "method", method, "path", cleanPath, "status", response.StatusCode)
		return fmt.Errorf("msteams: %s %s returned status %d", method, cleanPath, response.StatusCode)
	}
	if result == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("msteams: decode %s response: %w", method, err)
	}
	return nil
}
