package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	youtubeadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/youtube"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// fakeYouTubeAPI stands in for the Data API v3 client. The live key currently
// returns 403 because its Google Cloud project is restricted; that is a
// credential problem, so it must not be what this wiring test measures.
type fakeYouTubeAPI struct {
	videos  []youtubeadapter.Video
	queries []string
}

func (f *fakeYouTubeAPI) Search(_ context.Context, query string) ([]youtubeadapter.Video, error) {
	f.queries = append(f.queries, query)
	return f.videos, nil
}

func TestYouTubeFlowOpensAVideoThroughTheProductionRouter(t *testing.T) {
	api := &fakeYouTubeAPI{videos: []youtubeadapter.Video{
		{ID: "dQw4w9WgXcQ", Title: "Bicycle Repair Basics", ChannelTitle: "Park Tool"},
	}}
	service, err := NewYouTube(YouTubeConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"media","app_named":"YouTube","subject":"bicycle repair basics","body":"","confidence":0.94}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewYouTube: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Play bicycle repair basics on YouTube")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != youtubeadapter.ID || preview.Verb != manifest.Play || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	// The title has to reach the phone, not just an "open YouTube" line.
	if len(preview.Lines) == 0 || preview.Lines[0] != "Bicycle Repair Basics" {
		t.Fatalf("preview lines=%v, want the video title first", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	var deviceWork *capabilityadapter.DeviceWorkError
	if !errors.As(err, &deviceWork) {
		t.Fatalf("Confirm error = %v, want DeviceWorkError", err)
	}
	if deviceWork.Kind != "youtube_play" || deviceWork.Handle != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" || deviceWork.Text != "Bicycle Repair Basics" {
		t.Fatalf("device work = %+v", deviceWork)
	}
	if !reflect.DeepEqual(outcome, capabilityadapter.Outcome{}) {
		t.Fatalf("outcome=%+v, want zero outcome before phone playback", outcome)
	}
	if len(api.queries) != 1 {
		t.Fatalf("search calls=%d, want 1 at resolve", len(api.queries))
	}
}

func TestYouTubeFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewYouTube(YouTubeConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}

var _ capabilityadapter.Adapter = (*youtubeadapter.Adapter)(nil)
