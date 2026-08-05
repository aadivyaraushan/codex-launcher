package spotify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

type playCall struct {
	deviceID string
	uri      string
}

type fakeAPI struct {
	tracks     []Track
	devices    []Device
	searchErr  error
	devicesErr error
	playErr    error
	playCalls  []playCall
	cleared    int
}

func (f *fakeAPI) Search(context.Context, string) ([]Track, error) {
	return f.tracks, f.searchErr
}

func (f *fakeAPI) Devices(context.Context) ([]Device, error) {
	return f.devices, f.devicesErr
}

func (f *fakeAPI) Play(_ context.Context, deviceID, trackURI string) error {
	f.playCalls = append(f.playCalls, playCall{deviceID: deviceID, uri: trackURI})
	return f.playErr
}

func (f *fakeAPI) Clear(context.Context) error {
	f.cleared++
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestManifestIsTheOAuthAndroidRT2SpotifyRoute(t *testing.T) {
	a := New(&fakeAPI{}, testLogger())
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Ceiling != manifest.Completes {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Play) {
		t.Fatalf("Spotify must offer read and play: %v", m.Verbs)
	}
	if m.Allows(manifest.Write) {
		t.Fatalf("Spotify v1 must not offer write (playlist/library writes are out of scope): %v", m.Verbs)
	}
	if m.Platform != manifest.PlatformAndroid || m.ProvesCeiling == "" {
		t.Fatalf("platform=%s proves_ceiling=%q", m.Platform, m.ProvesCeiling)
	}
}

// Search returns a track id: resolving a play intent finds the track and
// carries its id/uri forward into the plan.
func TestResolvePlaySearchesAndFindsATrackID(t *testing.T) {
	api := &fakeAPI{
		tracks: []Track{{ID: "6l8GvAyoUZwWDgF1e4822w", Name: "Bohemian Rhapsody", Artist: "Queen", URI: "spotify:track:6l8GvAyoUZwWDgF1e4822w"}},
	}
	a := New(api, testLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Bohemian Rhapsody Queen",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["track_id"] != "6l8GvAyoUZwWDgF1e4822w" || plan.Details["track_uri"] != "spotify:track:6l8GvAyoUZwWDgF1e4822w" {
		t.Fatalf("plan did not carry the resolved track id/uri: %+v", plan.Details)
	}
}

// A play plan previews the track and target device.
func TestPreviewPlayShowsTheTrackAndTheActiveDevice(t *testing.T) {
	api := &fakeAPI{
		tracks: []Track{{ID: "t1", Name: "Bohemian Rhapsody", Artist: "Queen", URI: "spotify:track:t1"}},
		devices: []Device{
			{ID: "d1", Name: "Kitchen Speaker", IsActive: false},
			{ID: "d2", Name: "Pixel 9", IsActive: true},
		},
	}
	a := New(api, testLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Bohemian Rhapsody Queen",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	preview, err := a.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Bohemian Rhapsody") || !strings.Contains(shown, "Queen") {
		t.Fatalf("preview did not show the track: %q", shown)
	}
	if !strings.Contains(shown, "Pixel 9") {
		t.Fatalf("preview did not name the active device: %q", shown)
	}
	if strings.Contains(shown, "Kitchen Speaker") {
		t.Fatalf("preview named the inactive device instead of the active one: %q", shown)
	}
}

// Execute against a stubbed Spotify (playback starts) returns completes.
func TestExecutePlayReachesCompletesWhenSpotifyStartsPlayback(t *testing.T) {
	api := &fakeAPI{
		tracks:  []Track{{ID: "t1", Name: "Bohemian Rhapsody", Artist: "Queen", URI: "spotify:track:t1"}},
		devices: []Device{{ID: "d2", Name: "Pixel 9", IsActive: true}},
	}
	a := New(api, testLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Bohemian Rhapsody Queen",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done {
		t.Fatalf("outcome=%+v, want reached=completes done=true", out)
	}
	if len(api.playCalls) != 1 || api.playCalls[0].deviceID != "d2" || api.playCalls[0].uri != "spotify:track:t1" {
		t.Fatalf("play was not called with the resolved device and track: %+v", api.playCalls)
	}
}

// finish-consumer plan: Spotify play cannot demote to a hand-off. No active
// device is a hard failure of the play verb.
func TestExecutePlayFailsClosedOnNoActiveDevice(t *testing.T) {
	api := &fakeAPI{
		tracks:  []Track{{ID: "t1", Name: "Bohemian Rhapsody", Artist: "Queen", URI: "spotify:track:t1"}},
		devices: nil,
		playErr: ErrNoActiveDevice,
	}
	a := New(api, testLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Play, Subject: "Bohemian Rhapsody Queen",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	out, err := a.Execute(context.Background(), plan)
	if !errors.Is(err, ErrNoActiveDevice) {
		t.Fatalf("execute err=%v out=%+v, want ErrNoActiveDevice", err, out)
	}
	if out.HandedOffTo != "" || out.Reached == manifest.HandsOff {
		t.Fatalf("must not demote play to hand-off: %+v", out)
	}
}

func TestResolveRejectsAnEmptyQuery(t *testing.T) {
	a := New(&fakeAPI{}, testLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Play, Subject: "   "})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("empty query returned %v, want ErrEmptyQuery", err)
	}
}

func TestResolveReturnsErrNoTrackWhenSearchIsEmpty(t *testing.T) {
	a := New(&fakeAPI{tracks: nil}, testLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Play, Subject: "no such song"})
	if !errors.Is(err, ErrNoTrack) {
		t.Fatalf("no matching track returned %v, want ErrNoTrack", err)
	}
}

func TestReadVerbSearchesWithoutAttemptingPlayback(t *testing.T) {
	api := &fakeAPI{tracks: []Track{{ID: "t1", Name: "Bohemian Rhapsody", Artist: "Queen", URI: "spotify:track:t1"}}}
	a := New(api, testLogger())
	plan, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "Bohemian Rhapsody"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done {
		t.Fatalf("outcome=%+v", out)
	}
	if len(api.playCalls) != 0 {
		t.Fatalf("read verb must never call Play: %+v", api.playCalls)
	}
}

func TestRevokeClearsTheConnectionAndStopsFutureCalls(t *testing.T) {
	api := &fakeAPI{tracks: []Track{{ID: "t1", Name: "Song", Artist: "Artist", URI: "spotify:track:t1"}}}
	a := New(api, testLogger())
	if err := a.Revoke(context.Background()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if api.cleared != 1 {
		t.Fatalf("clear calls=%d, want 1", api.cleared)
	}
	if _, err := a.Resolve(context.Background(), adapter.Intent{Verb: manifest.Play, Subject: "Song"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("resolve after revoke returned %v", err)
	}
}
