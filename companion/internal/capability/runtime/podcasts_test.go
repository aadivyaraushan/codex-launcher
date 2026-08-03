package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	podcastsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/podcasts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

type fakePodcastsFeed struct {
	episodes []podcastsadapter.Episode
	calls    int
}

func (f *fakePodcastsFeed) Fetch(context.Context, string) ([]podcastsadapter.Episode, error) {
	f.calls++
	return f.episodes, nil
}

func TestPodcastsFlowComposesRouterAndPlainRSSAdapter(t *testing.T) {
	feed := &fakePodcastsFeed{episodes: []podcastsadapter.Episode{
		{GUID: "ep-1", Title: "Episode One: Hello Feed", EnclosureURL: "https://cdn.example.com/ep1.mp3", EnclosureType: "audio/mpeg"},
	}}
	service, err := NewPodcasts(PodcastsConfig{
		FeedURL: "https://example.com/feed.xml",
		Feed:    feed,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"media","app_named":"Podcasts","subject":"Hello Feed","body":"","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewPodcasts: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Play Hello Feed from the podcast feed")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != podcastsadapter.ID || preview.Verb != manifest.Play || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	if len(preview.Lines) < 2 || preview.Lines[1] != "https://cdn.example.com/ep1.mp3" {
		t.Fatalf("preview lines=%v", preview.Lines)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done || outcome.Reached != manifest.Completes {
		t.Fatalf("outcome=%+v", outcome)
	}
	if feed.calls != 1 {
		t.Fatalf("feed calls=%d, want 1 at resolve", feed.calls)
	}
}

func TestPodcastsFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewPodcasts(PodcastsConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}

var _ capabilityadapter.Adapter = (*podcastsadapter.Adapter)(nil)
