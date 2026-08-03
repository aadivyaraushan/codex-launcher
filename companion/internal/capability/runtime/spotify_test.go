package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	spotifyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// fakeSpotifyAPI stands in for the live Web API client, so this test proves
// the wiring rather than Spotify's contract (that is the adapter package's
// own client_test.go).
type fakeSpotifyAPI struct {
	track  spotifyadapter.Track
	device spotifyadapter.Device
	played []string
}

func (f *fakeSpotifyAPI) Search(context.Context, string) ([]spotifyadapter.Track, error) {
	return []spotifyadapter.Track{f.track}, nil
}

func (f *fakeSpotifyAPI) Devices(context.Context) ([]spotifyadapter.Device, error) {
	return []spotifyadapter.Device{f.device}, nil
}

func (f *fakeSpotifyAPI) Play(_ context.Context, deviceID, trackURI string) error {
	f.played = append(f.played, deviceID+" "+trackURI)
	return nil
}

func (f *fakeSpotifyAPI) Clear(context.Context) error { return nil }

// The adapter exists but nothing imports it, so nothing on the phone can
// reach it. This is the wiring the Pixel row depends on: an utterance goes
// through the same two-stage router the other runtimes use and comes back as
// a preview the session can render, then plays on confirm.
func TestSpotifyFlowPlaysThroughTheProductionRouter(t *testing.T) {
	api := &fakeSpotifyAPI{
		track:  spotifyadapter.Track{ID: "t-1", Name: "Bad Blood", Artist: "Bastille", URI: "spotify:track:t-1"},
		device: spotifyadapter.Device{ID: "device-1", Name: "Pixel 9", IsActive: true},
	}
	service, err := NewSpotify(SpotifyConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"play","app_class":"music","app_named":"Spotify","subject":"Bad Blood","body":"","confidence":0.96}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewSpotify: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Play Bad Blood on Spotify")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != spotifyadapter.ID || preview.Verb != manifest.Play || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done || len(api.played) != 1 {
		t.Fatalf("outcome=%+v played=%v", outcome, api.played)
	}
}

func TestSpotifyFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewSpotify(SpotifyConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}

var _ capabilityadapter.Adapter = (*spotifyadapter.Adapter)(nil)
