// Package podcasts is Operator's Wave-1 plain-RSS podcast adapter (RT-2).
// It lists episodes from a feed URL and resolves play to an enclosure URL.
// No partner API, no OAuth — RSS 2.0 <enclosure> is the play path.
package podcasts

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Feed loads episodes from one RSS URL. HTTPClient is the real
// implementation; tests use a recording fake.
type Feed interface {
	Fetch(ctx context.Context, feedURL string) ([]Episode, error)
}

// Episode is one podcast item with a media enclosure.
type Episode struct {
	GUID            string
	Title           string
	Link            string
	PubDate         string
	EnclosureURL    string
	EnclosureType   string
	EnclosureLength string
}

type HTTPClient struct {
	http   *http.Client
	logger *slog.Logger
}

func NewHTTPClient(client *http.Client, logger *slog.Logger) *HTTPClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPClient{http: client, logger: logger}
}

func (c *HTTPClient) Fetch(ctx context.Context, feedURL string) ([]Episode, error) {
	feedURL = strings.TrimSpace(feedURL)
	c.logger.Info("[podcasts] fetch", "feed_url_host_len", hostLen(feedURL), "feed_url_len", len(feedURL))
	if feedURL == "" {
		return nil, ErrMissingFeedURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[podcasts] fetch failed", "error", err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Error("[podcasts] fetch bad status", "status", resp.StatusCode)
		return nil, fmt.Errorf("podcasts: feed HTTP %d", resp.StatusCode)
	}
	episodes, err := ParseFeed(resp.Body)
	if err != nil {
		c.logger.Error("[podcasts] parse failed", "error", err)
		return nil, err
	}
	c.logger.Info("[podcasts] fetch complete", "episode_count", len(episodes))
	return episodes, nil
}

func hostLen(raw string) int {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			return j
		}
		return len(rest)
	}
	return 0
}

// ParseFeed reads an RSS 2.0 document and returns channel items that carry
// a media enclosure (url + type). Spec: https://www.rssboard.org/rss-specification
// (<enclosure> url/length/type). Podcast Index treats enclosure as required
// for playable episodes.
func ParseFeed(r io.Reader) ([]Episode, error) {
	var doc rssDocument
	decoder := xml.NewDecoder(r)
	decoder.Strict = false
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("podcasts: parse feed: %w", err)
	}
	out := make([]Episode, 0, len(doc.Channel.Items))
	for _, item := range doc.Channel.Items {
		url := strings.TrimSpace(item.Enclosure.URL)
		if url == "" {
			continue
		}
		guid := strings.TrimSpace(item.GUID.Value)
		if guid == "" {
			guid = url
		}
		out = append(out, Episode{
			GUID:            guid,
			Title:           strings.TrimSpace(item.Title),
			Link:            strings.TrimSpace(item.Link),
			PubDate:         strings.TrimSpace(item.PubDate),
			EnclosureURL:    url,
			EnclosureType:   strings.TrimSpace(item.Enclosure.Type),
			EnclosureLength: strings.TrimSpace(item.Enclosure.Length),
		})
	}
	return out, nil
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string       `xml:"title"`
	Link      string       `xml:"link"`
	PubDate   string       `xml:"pubDate"`
	GUID      rssGUID      `xml:"guid"`
	Enclosure rssEnclosure `xml:"enclosure"`
}

type rssGUID struct {
	Value string `xml:",chardata"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}
