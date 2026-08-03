package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
)

const APIBaseURL = "https://www.googleapis.com"

var ErrTokenCannotBeCleared = errors.New("gdrive: token source cannot clear its token")

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

func (c *HTTPClient) ListFiles(ctx context.Context, query string) ([]File, error) {
	values := url.Values{
		"pageSize": {"50"},
		"fields":   {"files(id,name,mimeType)"},
		"spaces":   {"drive"},
	}
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		escaped := strings.ReplaceAll(trimmed, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `'`, `\'`)
		values.Set("q", fmt.Sprintf("name contains '%s' and trashed = false", escaped))
	} else {
		values.Set("q", "trashed = false")
	}
	var page struct {
		Files []File `json:"files"`
	}
	path := "/drive/v3/files?" + values.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, "", &page); err != nil {
		return nil, err
	}
	c.logger.Info("[gdrive] list complete", "file_count", len(page.Files), "query_length", len(query))
	return page.Files, nil
}

func (c *HTTPClient) CreateFile(ctx context.Context, request CreateFile) (File, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadataHeader := textproto.MIMEHeader{}
	metadataHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return File{}, fmt.Errorf("gdrive: metadata part: %w", err)
	}
	meta := map[string]string{"name": request.Name, "mimeType": "text/plain"}
	if err := json.NewEncoder(metadataPart).Encode(meta); err != nil {
		return File{}, fmt.Errorf("gdrive: encode metadata: %w", err)
	}
	mediaHeader := textproto.MIMEHeader{}
	mediaHeader.Set("Content-Type", "text/plain")
	mediaPart, err := writer.CreatePart(mediaHeader)
	if err != nil {
		return File{}, fmt.Errorf("gdrive: media part: %w", err)
	}
	if _, err := io.WriteString(mediaPart, request.Content); err != nil {
		return File{}, fmt.Errorf("gdrive: write content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return File{}, fmt.Errorf("gdrive: close multipart: %w", err)
	}

	var raw File
	path := "/upload/drive/v3/files?uploadType=multipart&fields=id,name,mimeType"
	if err := c.do(ctx, http.MethodPost, path, &body, writer.FormDataContentType(), &raw); err != nil {
		return File{}, err
	}
	return raw, nil
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
		c.logger.Warn("[gdrive] remote revoke request failed", "error", err)
		return adapter.ClassifyHTTPFailure(ID, "revoke", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		c.logger.Warn("[gdrive] remote revoke rejected", "status", response.StatusCode)
		return fmt.Errorf("gdrive: revoke returned status %d", response.StatusCode)
	}
	return nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body io.Reader, contentType string, result any) error {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("gdrive: load access token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("gdrive: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	cleanPath := strings.Split(path, "?")[0]
	c.logger.Info("[gdrive] request", "method", method, "path", cleanPath, "has_body", body != nil)
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[gdrive] request failed", "method", method, "path", cleanPath, "error", err)
		wrapped := fmt.Errorf("gdrive: %s request failed: %w", method, err)
		return adapter.ClassifyHTTPFailure(ID, method, wrapped)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[gdrive] response rejected", "method", method, "path", cleanPath, "status", response.StatusCode)
		return fmt.Errorf("gdrive: %s %s returned status %d", method, cleanPath, response.StatusCode)
	}
	if result == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("gdrive: decode %s response: %w", method, err)
	}
	return nil
}
