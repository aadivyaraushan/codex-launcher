package outlook

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

// Docs: Context7 /microsoftgraph/microsoft-graph-docs-contrib —
// GET /me/messages, POST /me/messages (draft), POST /me/sendMail.

const APIBaseURL = "https://graph.microsoft.com"

var ErrTokenCannotBeCleared = errors.New("outlook: token source cannot clear its token")

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

func (c *HTTPClient) ListMessages(ctx context.Context, query string) ([]Message, error) {
	values := url.Values{
		"$top":    {"50"},
		"$select": {"id,subject,from,bodyPreview"},
	}
	if strings.TrimSpace(query) != "" {
		// $search is preferred for free text; Graph requires quotes around the term.
		values.Set("$search", fmt.Sprintf("%q", query))
	}
	var page struct {
		Value []struct {
			ID          string `json:"id"`
			Subject     string `json:"subject"`
			BodyPreview string `json:"bodyPreview"`
			From        struct {
				EmailAddress struct {
					Address string `json:"address"`
					Name    string `json:"name"`
				} `json:"emailAddress"`
			} `json:"from"`
		} `json:"value"`
	}
	path := "/v1.0/me/messages?" + values.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(page.Value))
	for _, item := range page.Value {
		from := item.From.EmailAddress.Address
		if from == "" {
			from = item.From.EmailAddress.Name
		}
		messages = append(messages, Message{
			ID: item.ID, Subject: item.Subject, From: from, Preview: item.BodyPreview,
		})
	}
	c.logger.Info("[outlook] list complete", "message_count", len(messages), "query_length", len(query))
	return messages, nil
}

func (c *HTTPClient) CreateDraft(ctx context.Context, request CreateDraft) (Message, error) {
	payload := map[string]any{
		"subject": request.Subject,
		"body": map[string]string{
			"contentType": "Text",
			"content":     request.Body,
		},
	}
	if to := strings.TrimSpace(request.To); to != "" {
		payload["toRecipients"] = []map[string]any{
			{"emailAddress": map[string]string{"address": to}},
		}
	}
	var raw struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1.0/me/messages", payload, &raw); err != nil {
		return Message{}, err
	}
	return Message{ID: raw.ID, Subject: raw.Subject}, nil
}

func (c *HTTPClient) SendMail(ctx context.Context, request SendMail) error {
	payload := map[string]any{
		"message": map[string]any{
			"subject": request.Subject,
			"body": map[string]string{
				"contentType": "Text",
				"content":     request.Body,
			},
			"toRecipients": []map[string]any{
				{"emailAddress": map[string]string{"address": request.To}},
			},
		},
		"saveToSentItems": true,
	}
	return c.do(ctx, http.MethodPost, "/v1.0/me/sendMail", payload, nil)
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
		return fmt.Errorf("outlook: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return fmt.Errorf("outlook: encode request: %w", encodeErr)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("outlook: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// ConsistencyLevel: eventual is required by Graph when using $search.
	if strings.Contains(path, "$search=") || strings.Contains(path, "%24search=") {
		req.Header.Set("ConsistencyLevel", "eventual")
	}
	cleanPath := strings.Split(path, "?")[0]
	c.logger.Info("[outlook] request", "method", method, "path", cleanPath, "has_body", body != nil)
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[outlook] request failed", "method", method, "path", cleanPath, "error", err)
		wrapped := fmt.Errorf("outlook: %s request failed: %w", method, err)
		return adapter.ClassifyHTTPFailure(ID, method, wrapped)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[outlook] response rejected", "method", method, "path", cleanPath, "status", response.StatusCode)
		return fmt.Errorf("outlook: %s %s returned status %d", method, cleanPath, response.StatusCode)
	}
	if result == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("outlook: decode %s response: %w", method, err)
	}
	return nil
}
