package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// APIBaseURL is the YouTube Data API v3 base URL.
const APIBaseURL = "https://www.googleapis.com/youtube/v3"

// SearchQuotaCostPerCall and DefaultDailyQuotaUnits document the quota
// budget for search.list: 100 units per call against a default project
// budget of 10,000 units/day, i.e. about 100 searches/day for the entire
// user base (not per user). See
// https://developers.google.com/youtube/v3/determine_quota_cost and the
// plan's own capacity note (consumer-app-implementation-plan.md).
const (
	SearchQuotaCostPerCall = 100
	DefaultDailyQuotaUnits = 10000
)

// Video is the part of a search.list result the adapter needs.
type Video struct {
	ID           string
	Title        string
	ChannelTitle string
}

type searchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			Title        string `json:"title"`
			ChannelTitle string `json:"channelTitle"`
		} `json:"snippet"`
	} `json:"items"`
}

// HTTPClient calls the real YouTube Data API v3 using a plain API key (no
// per-user OAuth step).
type HTTPClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
	logger  *slog.Logger
}

var _ API = (*HTTPClient)(nil)

func NewHTTPClient(baseURL, apiKey string, client *http.Client, logger *slog.Logger) *HTTPClient {
	if baseURL == "" {
		baseURL = APIBaseURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: client, logger: logger}
}

// Search calls search.list for videos matching query and returns the
// parsed results in rank order. It never logs or wraps the api key into an
// error message.
func (c *HTTPClient) Search(ctx context.Context, query string) ([]Video, error) {
	q := url.Values{}
	q.Set("part", "snippet")
	q.Set("q", query)
	q.Set("type", "video")
	q.Set("maxResults", "5")
	q.Set("key", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("youtube: build request: %w", err)
	}
	c.logger.Info("[youtube] request", "method", http.MethodGet, "path", "/search", "query_length", len(query))
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[youtube] request failed", "path", "/search")
		return nil, fmt.Errorf("youtube: search request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		c.logger.Error("[youtube] response rejected", "path", "/search", "status", resp.StatusCode)
		return nil, fmt.Errorf("youtube: search returned status %d", resp.StatusCode)
	}
	var parsed searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("youtube: decode search response: %w", err)
	}
	videos := make([]Video, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		if strings.TrimSpace(item.ID.VideoID) == "" {
			c.logger.Warn("[youtube] dropping unusable search result", "reason", "missing_video_id")
			continue
		}
		videos = append(videos, Video{
			ID: item.ID.VideoID, Title: item.Snippet.Title, ChannelTitle: item.Snippet.ChannelTitle,
		})
	}
	c.logger.Info("[youtube] search complete", "result_count", len(videos))
	return videos, nil
}
