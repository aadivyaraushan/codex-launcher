package podcasts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Minimal RSS 2.0 podcast fixture per rssboard.org enclosure + Podcast Index
// episode conventions: item title/guid + enclosure url/length/type.
const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Sample Show</title>
    <link>https://example.com/show</link>
    <description>A public-shaped fixture feed</description>
    <item>
      <title>Episode Two: Nested Paths</title>
      <guid isPermaLink="false">ep-2</guid>
      <pubDate>Mon, 01 Jan 2024 12:00:00 GMT</pubDate>
      <enclosure url="https://cdn.example.com/ep2.mp3" length="2048" type="audio/mpeg"/>
      <link>https://example.com/ep2</link>
    </item>
    <item>
      <title>Episode One: Hello Feed</title>
      <guid isPermaLink="false">ep-1</guid>
      <pubDate>Sun, 31 Dec 2023 12:00:00 GMT</pubDate>
      <enclosure url="https://cdn.example.com/ep1.mp3" length="1024" type="audio/mpeg"/>
      <link>https://example.com/ep1</link>
    </item>
  </channel>
</rss>`

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestManifestIsFreeAndroidRT2PlainRSSRoute(t *testing.T) {
	a := New(FeedConfig{URL: "https://example.com/feed.xml"}, nil, discardLogger())
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthNone || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if m.Ceiling != manifest.Completes || m.Consent != manifest.ConsentA {
		t.Fatalf("ceiling/consent = %s/%s", m.Ceiling, m.Consent)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Play) {
		t.Fatalf("Podcasts must offer read and play: %v", m.Verbs)
	}
	if m.Platform != manifest.PlatformAndroid || m.ProvesCeiling == "" {
		t.Fatalf("platform=%s proves_ceiling=%q", m.Platform, m.ProvesCeiling)
	}
}

func TestParseFeedListsEpisodesFromEnclosureItems(t *testing.T) {
	episodes, err := ParseFeed(strings.NewReader(sampleFeed))
	if err != nil {
		t.Fatalf("ParseFeed: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episodes=%d, want 2", len(episodes))
	}
	if episodes[0].GUID != "ep-2" || episodes[0].Title != "Episode Two: Nested Paths" {
		t.Fatalf("first episode = %+v", episodes[0])
	}
	if episodes[0].EnclosureURL != "https://cdn.example.com/ep2.mp3" || episodes[0].EnclosureType != "audio/mpeg" {
		t.Fatalf("first enclosure = %+v", episodes[0])
	}
	if episodes[1].GUID != "ep-1" || episodes[1].EnclosureURL == "" {
		t.Fatalf("second episode = %+v", episodes[1])
	}
}

func TestHTTPClientFetchesAndParsesFeedWithoutLiveNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, sampleFeed)
	}))
	t.Cleanup(server.Close)

	client := NewHTTPClient(server.Client(), discardLogger())
	episodes, err := client.Fetch(context.Background(), server.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(episodes) != 2 || episodes[0].EnclosureURL == "" {
		t.Fatalf("episodes=%+v", episodes)
	}
}

func TestReadReturnsEpisodeListFromConfiguredFeed(t *testing.T) {
	feed := &fakeFeed{episodes: []Episode{
		{GUID: "ep-2", Title: "Episode Two: Nested Paths", EnclosureURL: "https://cdn.example.com/ep2.mp3", EnclosureType: "audio/mpeg"},
		{GUID: "ep-1", Title: "Episode One: Hello Feed", EnclosureURL: "https://cdn.example.com/ep1.mp3", EnclosureType: "audio/mpeg"},
	}}
	a := New(FeedConfig{URL: "https://example.com/feed.xml"}, feed, discardLogger())

	plan, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read})
	if err != nil {
		t.Fatalf("resolve read: %v", err)
	}
	if plan.Details["episode_count"] != "2" || !strings.Contains(plan.Details["episodes"], "Episode One: Hello Feed") {
		t.Fatalf("read plan details = %+v", plan.Details)
	}

	preview, err := a.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Episode Two") || !strings.Contains(shown, "2 episodes") {
		t.Fatalf("preview did not list episodes: %q", shown)
	}

	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute read: %v", err)
	}
	if !out.Done || out.Reached != manifest.Completes {
		t.Fatalf("read outcome = %+v", out)
	}
	if !strings.Contains(out.Detail, "ep-1") || !strings.Contains(out.Detail, "Episode One: Hello Feed") {
		t.Fatalf("read detail missing list: %q", out.Detail)
	}
	if feed.calls != 1 {
		t.Fatalf("feed fetch calls=%d, want 1", feed.calls)
	}
}

func TestPlayResolvesOneEpisodeEnclosureAndCompletes(t *testing.T) {
	feed := &fakeFeed{episodes: []Episode{
		{GUID: "ep-2", Title: "Episode Two: Nested Paths", EnclosureURL: "https://cdn.example.com/ep2.mp3", EnclosureType: "audio/mpeg"},
		{GUID: "ep-1", Title: "Episode One: Hello Feed", EnclosureURL: "https://cdn.example.com/ep1.mp3", EnclosureType: "audio/mpeg"},
	}}
	a := New(FeedConfig{URL: "https://example.com/feed.xml"}, feed, discardLogger())

	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Episode",
	})
	if !errors.Is(err, ErrAmbiguousEpisode) {
		t.Fatalf("ambiguous play returned %v", err)
	}
	var question *adapter.ClarificationError
	if !errors.As(err, &question) || question.Question == "" {
		t.Fatalf("ambiguous play is not a user question: %T %v", err, err)
	}

	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Hello Feed",
	})
	if err != nil {
		t.Fatalf("play resolve: %v", err)
	}
	if plan.Handle != "ep-1" || plan.Details["enclosure_url"] != "https://cdn.example.com/ep1.mp3" {
		t.Fatalf("play plan = %+v", plan)
	}

	preview, err := a.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("play preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Episode One: Hello Feed") || !strings.Contains(shown, "https://cdn.example.com/ep1.mp3") {
		t.Fatalf("play preview missing title/enclosure: %q", shown)
	}

	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("play execute: %v", err)
	}
	if !out.Done || out.Reached != manifest.Completes {
		t.Fatalf("play outcome = %+v", out)
	}
	if !strings.Contains(out.Detail, "https://cdn.example.com/ep1.mp3") {
		t.Fatalf("play detail must carry enclosure URL: %q", out.Detail)
	}
}

func TestPlayByGUIDAndMissingFeedURL(t *testing.T) {
	feed := &fakeFeed{episodes: []Episode{
		{GUID: "ep-1", Title: "Episode One: Hello Feed", EnclosureURL: "https://cdn.example.com/ep1.mp3", EnclosureType: "audio/mpeg"},
	}}
	a := New(FeedConfig{}, feed, discardLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{Verb: manifest.Play, Subject: "ep-1"})
	if !errors.Is(err, ErrMissingFeedURL) {
		t.Fatalf("missing feed url returned %v", err)
	}

	a = New(FeedConfig{URL: "https://example.com/feed.xml"}, feed, discardLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{Verb: manifest.Play, Subject: "ep-1"})
	if err != nil {
		t.Fatalf("guid play: %v", err)
	}
	if plan.Details["enclosure_url"] != "https://cdn.example.com/ep1.mp3" {
		t.Fatalf("guid plan = %+v", plan)
	}
}

func TestRevokeIsNoopWithoutCredentials(t *testing.T) {
	a := New(FeedConfig{URL: "https://example.com/feed.xml"}, &fakeFeed{}, discardLogger())
	if err := a.Revoke(context.Background()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
}

// Callers: podcasts unit tests only.
// Affected API: resolveFeedURL Fields["feed_url"]; ParseFeed skip/guid fallback; play ErrNoEnclosure.
// Schema: RSS item/enclosure. User: follow-up on Podcasts judge — add missing regression tests.
func TestReadUsesFieldsFeedURLOverride(t *testing.T) {
	feed := &fakeFeed{episodes: []Episode{
		{GUID: "ep-9", Title: "Override Episode", EnclosureURL: "https://cdn.example.com/ep9.mp3", EnclosureType: "audio/mpeg"},
	}}
	a := New(FeedConfig{URL: "https://example.com/default-feed.xml"}, feed, discardLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read,
		Fields: map[string]string{"feed_url": "https://example.com/override-feed.xml"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if feed.lastURL != "https://example.com/override-feed.xml" {
		t.Fatalf("fetched %q, want override feed", feed.lastURL)
	}
	if plan.Details["feed_url"] != "https://example.com/override-feed.xml" {
		t.Fatalf("plan feed_url=%q", plan.Details["feed_url"])
	}
}

func TestParseFeedSkipsItemsWithoutEnclosureAndFallsBackGUID(t *testing.T) {
	const feed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Mixed</title>
  <item>
    <title>Text only note</title>
    <guid>no-audio</guid>
  </item>
  <item>
    <title>Playable Without Guid</title>
    <enclosure url="https://cdn.example.com/noguid.mp3" length="100" type="audio/mpeg"/>
  </item>
</channel></rss>`
	episodes, err := ParseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("ParseFeed: %v", err)
	}
	if len(episodes) != 1 {
		t.Fatalf("episodes=%d, want 1 (no-enclosure skipped)", len(episodes))
	}
	if episodes[0].Title != "Playable Without Guid" {
		t.Fatalf("episode=%+v", episodes[0])
	}
	if episodes[0].GUID != "https://cdn.example.com/noguid.mp3" {
		t.Fatalf("GUID fallback=%q, want enclosure URL", episodes[0].GUID)
	}
}

func TestPlayRejectsMatchedEpisodeWithoutEnclosure(t *testing.T) {
	feed := &fakeFeed{episodes: []Episode{
		{GUID: "bare", Title: "Bare Title", EnclosureURL: ""},
	}}
	a := New(FeedConfig{URL: "https://example.com/feed.xml"}, feed, discardLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Bare Title",
	})
	if !errors.Is(err, ErrNoEnclosure) {
		t.Fatalf("err=%v, want ErrNoEnclosure", err)
	}
}

type fakeFeed struct {
	episodes []Episode
	err      error
	calls    int
	lastURL  string
}

func (f *fakeFeed) Fetch(_ context.Context, feedURL string) ([]Episode, error) {
	f.calls++
	f.lastURL = feedURL
	return f.episodes, f.err
}
