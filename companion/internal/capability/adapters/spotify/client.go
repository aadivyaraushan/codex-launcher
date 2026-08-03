// Package spotify is Operator's Spotify adapter. It uses the official
// Spotify Web API with user OAuth to search for a track and start playback
// on a device the user already has Spotify open on. It never touches
// playlists or the user's library — those stay out of v1 — and it never
// claims playback started when Spotify reports no active device; that case
// demotes to a hand-off that opens the Spotify app instead.
package spotify

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
)

// APIBaseURL is the Spotify Web API host. See
// https://developer.spotify.com/documentation/web-api.
const APIBaseURL = "https://api.spotify.com"

// ErrNoActiveDevice is returned by Play when Spotify's documented no-active-
// device failure is reported: a 404 whose error body carries
// reason "NO_ACTIVE_DEVICE" (https://developer.spotify.com/documentation/web-api/reference/start-a-users-playback).
// This is the signal the adapter uses to demote a play attempt to a
// hand-off instead of claiming playback started.
var (
	ErrNoActiveDevice       = errors.New("spotify: no active device")
	ErrTokenCannotBeCleared = errors.New("spotify: token source cannot clear its token")
)

// Track is one search result.
type Track struct {
	ID     string
	Name   string
	Artist string
	URI    string
}

// Device is one entry from GET /v1/me/player/devices.
type Device struct {
	ID       string
	Name     string
	IsActive bool
}

// TokenSource supplies the bearer token for every request. The real
// implementation is backed by a stored, refreshable OAuth token; tests use
// a static string.
type TokenSource interface {
	AccessToken(context.Context) (string, error)
}

type tokenClearer interface {
	Clear(context.Context) error
}

// HTTPClient is the real Spotify Web API implementation. It is intentionally
// small: search, list devices, start playback — the only three calls the
// adapter needs.
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

// Search calls GET /v1/search with type=track. No scope is required for
// this call; it works with any valid user access token.
func (c *HTTPClient) Search(ctx context.Context, query string) ([]Track, error) {
	values := url.Values{"q": {query}, "type": {"track"}, "limit": {"5"}}
	path := "/v1/search?" + values.Encode()
	var response struct {
		Tracks struct {
			Items []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				URI     string `json:"uri"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	tracks := make([]Track, 0, len(response.Tracks.Items))
	for _, item := range response.Tracks.Items {
		artist := ""
		if len(item.Artists) > 0 {
			artist = item.Artists[0].Name
		}
		tracks = append(tracks, Track{ID: item.ID, Name: item.Name, Artist: artist, URI: item.URI})
	}
	c.logger.Info("[spotify] search complete", "result_count", len(tracks))
	return tracks, nil
}

// Devices calls GET /v1/me/player/devices. Requires the
// user-read-playback-state scope.
func (c *HTTPClient) Devices(ctx context.Context) ([]Device, error) {
	var response struct {
		Devices []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"devices"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/me/player/devices", nil, &response); err != nil {
		return nil, err
	}
	devices := make([]Device, 0, len(response.Devices))
	for _, d := range response.Devices {
		devices = append(devices, Device{ID: d.ID, Name: d.Name, IsActive: d.IsActive})
	}
	c.logger.Info("[spotify] devices listed", "device_count", len(devices))
	return devices, nil
}

// Play calls PUT /v1/me/player/play with the given track uri, targeting
// deviceID when non-empty. Requires the user-modify-playback-state scope.
// A 204 means playback started; a 404 with reason NO_ACTIVE_DEVICE returns
// ErrNoActiveDevice so the caller can demote instead of failing outright.
func (c *HTTPClient) Play(ctx context.Context, deviceID, trackURI string) error {
	path := "/v1/me/player/play"
	if deviceID != "" {
		path += "?" + url.Values{"device_id": {deviceID}}.Encode()
	}
	body := map[string]any{"uris": []string{trackURI}}
	return c.doPlay(ctx, path, body)
}

// Clear delegates to the token source's Clear method, when it has one — the
// same pattern used by the Todoist adapter's client.
func (c *HTTPClient) Clear(ctx context.Context) error {
	clearer, ok := c.tokens.(tokenClearer)
	if !ok {
		return ErrTokenCannotBeCleared
	}
	return clearer.Clear(ctx)
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, body, result any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[spotify] request failed", "method", method, "path", strings.Split(path, "?")[0], "error", err)
		return fmt.Errorf("spotify: %s request failed: %w", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[spotify] response rejected", "method", method, "path", strings.Split(path, "?")[0], "status", response.StatusCode)
		return fmt.Errorf("spotify: %s %s returned status %d", method, strings.Split(path, "?")[0], response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("spotify: decode %s response: %w", method, err)
	}
	return nil
}

// doPlay is separate from doJSON because Play's success response (204) has
// no body to decode, and its failure body carries a "reason" field that has
// to be inspected to tell "no active device" apart from every other error.
func (c *HTTPClient) doPlay(ctx context.Context, path string, body any) error {
	req, err := c.newRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	response, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[spotify] play request failed", "error", err)
		return fmt.Errorf("spotify: play request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		c.logger.Info("[spotify] play accepted", "status", response.StatusCode)
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	var errorBody struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &errorBody)
	if errorBody.Error.Reason == "NO_ACTIVE_DEVICE" {
		c.logger.Warn("[spotify] play refused", "status", response.StatusCode, "reason", errorBody.Error.Reason)
		return ErrNoActiveDevice
	}
	c.logger.Error("[spotify] play refused", "status", response.StatusCode, "reason", errorBody.Error.Reason)
	return fmt.Errorf("spotify: play request returned status %d reason %q", response.StatusCode, errorBody.Error.Reason)
}

func (c *HTTPClient) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("spotify: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		var encoded strings.Builder
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return nil, fmt.Errorf("spotify: encode request: %w", err)
		}
		reader = strings.NewReader(encoded.String())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("spotify: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.logger.Info("[spotify] request", "method", method, "path", strings.Split(path, "?")[0], "has_body", body != nil)
	return req, nil
}
