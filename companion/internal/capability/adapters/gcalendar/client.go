package gcalendar

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

const APIBaseURL = "https://www.googleapis.com"

var ErrTokenCannotBeCleared = errors.New("gcalendar: token source cannot clear its token")

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

func (c *HTTPClient) ListEvents(ctx context.Context, query string) ([]Event, error) {
	values := url.Values{"singleEvents": {"true"}, "orderBy": {"startTime"}, "maxResults": {"50"}}
	if strings.TrimSpace(query) != "" {
		values.Set("q", query)
	}
	var page struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
		} `json:"items"`
	}
	path := "/calendar/v3/calendars/primary/events?" + values.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, "", &page); err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(page.Items))
	for _, item := range page.Items {
		start := item.Start.DateTime
		if start == "" {
			start = item.Start.Date
		}
		end := item.End.DateTime
		if end == "" {
			end = item.End.Date
		}
		events = append(events, Event{
			ID: item.ID, Summary: item.Summary, Description: item.Description, Start: start, End: end,
		})
	}
	c.logger.Info("[gcalendar] list complete", "event_count", len(events), "query_length", len(query))
	return events, nil
}

func (c *HTTPClient) CreateEvent(ctx context.Context, request CreateEvent) (Event, error) {
	payload := map[string]any{
		"summary":     request.Summary,
		"description": request.Description,
		"start":       map[string]string{"dateTime": request.Start},
		"end":         map[string]string{"dateTime": request.End},
	}
	var raw struct {
		ID          string `json:"id"`
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Start       struct {
			DateTime string `json:"dateTime"`
		} `json:"start"`
		End struct {
			DateTime string `json:"dateTime"`
		} `json:"end"`
	}
	if err := c.do(ctx, http.MethodPost, "/calendar/v3/calendars/primary/events", payload, "application/json", &raw); err != nil {
		return Event{}, err
	}
	return Event{
		ID: raw.ID, Summary: raw.Summary, Description: raw.Description,
		Start: raw.Start.DateTime, End: raw.End.DateTime,
	}, nil
}

func (c *HTTPClient) Clear(ctx context.Context) error {
	clearer, ok := c.tokens.(tokenClearer)
	if !ok {
		return ErrTokenCannotBeCleared
	}
	_ = c.revokeRemote(ctx)
	return clearer.Clear(ctx)
}

func (c *HTTPClient) revokeRemote(ctx context.Context) error {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return err
	}
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Warn("[gcalendar] remote revoke request failed", "error", err)
		return adapter.ClassifyHTTPFailure(ID, "revoke", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		c.logger.Warn("[gcalendar] remote revoke rejected", "status", response.StatusCode)
		return fmt.Errorf("gcalendar: revoke returned status %d", response.StatusCode)
	}
	return nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, contentType string, result any) error {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("gcalendar: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return fmt.Errorf("gcalendar: encode request: %w", encodeErr)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("gcalendar: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	cleanPath := strings.Split(path, "?")[0]
	c.logger.Info("[gcalendar] request", "method", method, "path", cleanPath, "has_body", body != nil)
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[gcalendar] request failed", "method", method, "path", cleanPath, "error", err)
		wrapped := fmt.Errorf("gcalendar: %s request failed: %w", method, err)
		return adapter.ClassifyHTTPFailure(ID, method, wrapped)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[gcalendar] response rejected", "method", method, "path", cleanPath, "status", response.StatusCode)
		return fmt.Errorf("gcalendar: %s %s returned status %d", method, cleanPath, response.StatusCode)
	}
	if result == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("gcalendar: decode %s response: %w", method, err)
	}
	return nil
}
