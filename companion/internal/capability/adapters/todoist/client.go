package todoist

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

const APIBaseURL = "https://api.todoist.com"

var ErrTokenCannotBeCleared = errors.New("todoist: token source cannot clear its token")

type Task struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	Description string `json:"description,omitempty"`
}

type CreateTask struct {
	Content     string `json:"content"`
	Description string `json:"description,omitempty"`
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

func (c *HTTPClient) ListTasks(ctx context.Context) ([]Task, error) {
	var tasks []Task
	cursor := ""
	seen := map[string]bool{}
	for {
		query := url.Values{}
		if cursor != "" {
			if seen[cursor] {
				return nil, errors.New("todoist: repeated pagination cursor")
			}
			seen[cursor] = true
			query.Set("cursor", cursor)
		}
		path := "/api/v1/tasks"
		if encoded := query.Encode(); encoded != "" {
			path += "?" + encoded
		}
		var page struct {
			Results    []Task  `json:"results"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		tasks = append(tasks, page.Results...)
		if page.NextCursor == nil || *page.NextCursor == "" {
			c.logger.Info("[todoist] list complete", "task_count", len(tasks), "page_count", len(seen)+1)
			return tasks, nil
		}
		cursor = *page.NextCursor
	}
}

func (c *HTTPClient) CreateTask(ctx context.Context, request CreateTask) (Task, error) {
	var task Task
	if err := c.do(ctx, http.MethodPost, "/api/v1/tasks", request, &task); err != nil {
		return Task{}, err
	}
	return task, nil
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
		return fmt.Errorf("todoist: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		var encoded strings.Builder
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return fmt.Errorf("todoist: encode request: %w", err)
		}
		reader = strings.NewReader(encoded.String())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("todoist: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.logger.Info("[todoist] request", "method", method, "path", strings.Split(path, "?")[0], "has_body", body != nil)
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[todoist] request failed", "method", method, "path", strings.Split(path, "?")[0], "error", err)
		wrapped := fmt.Errorf("todoist: %s request failed: %w", method, err)
		return adapter.ClassifyHTTPFailure(ID, method, wrapped)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[todoist] response rejected", "method", method, "path", strings.Split(path, "?")[0], "status", response.StatusCode)
		return fmt.Errorf("todoist: %s %s returned status %d", method, strings.Split(path, "?")[0], response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("todoist: decode %s response: %w", method, err)
	}
	return nil
}
