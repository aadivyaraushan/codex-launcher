package youtube

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

type fakeAPI struct {
	videos []Video
	err    error
	calls  []string
}

func (f *fakeAPI) Search(_ context.Context, query string) ([]Video, error) {
	f.calls = append(f.calls, query)
	return f.videos, f.err
}

func newLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestManifestIsTheCappedAndroidRT2YouTubeRoute(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	// Measured 2026-08-04 against the Operator project: the exact HTTPClient
	// search.list path returned five usable videos. Read therefore completes;
	// Play still returns its own hands_off outcome after resolving a real URL.
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Ceiling != manifest.Completes {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Play) {
		t.Fatalf("YouTube must offer read and play: %v", m.Verbs)
	}
	// Requirement: record the YouTube capacity limit (10,000 quota
	// units/day ~= 100 searches for the entire user base) in the
	// manifest's capacity field.
	if m.Capacity.Kind != manifest.CapacityCapped || m.Capacity.Limit != 100 {
		t.Fatalf("capacity = %+v, want capped:100", m.Capacity)
	}
	if m.ProvesCeiling == "" {
		t.Fatalf("proves_ceiling must be set")
	}
}

// Requirement: YouTube search returns an id and the plan previews the title.
func TestSearchReturnsAVideoIDAndPreviewShowsTheTitle(t *testing.T) {
	api := &fakeAPI{videos: []Video{
		{ID: "dQw4w9WgXcQ", Title: "Never Gonna Give You Up", ChannelTitle: "Rick Astley"},
	}}
	a := New(api, newLogger())
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "never gonna give you up",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["video_id"] != "dQw4w9WgXcQ" {
		t.Fatalf("plan video_id = %q, want dQw4w9WgXcQ", plan.Details["video_id"])
	}

	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Never Gonna Give You Up") {
		t.Fatalf("preview did not show the title: %q", shown)
	}

	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done {
		t.Fatalf("outcome = %+v, want the measured read result to complete", out)
	}
}

// Requirement: search + play keeps the resolved video URL until the phone can
// open that exact video. The device action is deliberately not an ordinary
// outcome: the Mac cannot claim success before the Pixel answers.
func TestPlayResolvesAndHandsTheExactVideoToThePixel(t *testing.T) {
	api := &fakeAPI{videos: []Video{
		{ID: "dQw4w9WgXcQ", Title: "Never Gonna Give You Up", ChannelTitle: "Rick Astley"},
	}}
	a := New(api, newLogger())
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Play, Subject: "never gonna give you up"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["watch_url"] != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("watch_url = %q", plan.Details["watch_url"])
	}
	out, err := a.Execute(ctx, plan)
	var deviceWork *adapter.DeviceWorkError
	if !errors.As(err, &deviceWork) {
		t.Fatalf("execute error = %v, want DeviceWorkError", err)
	}
	if !reflect.DeepEqual(out, adapter.Outcome{}) {
		t.Fatalf("execute claimed an outcome before the phone answered: %+v", out)
	}
	if deviceWork.AdapterID != ID || deviceWork.Kind != "youtube_play" {
		t.Fatalf("device work route = %+v, want youtube/youtube_play", deviceWork)
	}
	if deviceWork.Handle != plan.Details["watch_url"] || deviceWork.Text != plan.Details["title"] {
		t.Fatalf("device work lost the selected video: %+v", deviceWork)
	}
	if deviceWork.Ceiling != manifest.HandsOff {
		t.Fatalf("device work ceiling = %q, want hands_off until Android verifies playback", deviceWork.Ceiling)
	}
}

func TestPlayRefusesAWatchURLThatWasChangedAfterResolve(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	plan := adapter.Plan{
		AdapterID: ID,
		Verb:      manifest.Play,
		Details: map[string]string{
			"watch_url": "https://example.com/watch?v=dQw4w9WgXcQ",
			"title":     "Changed target",
		},
	}

	_, err := a.Execute(context.Background(), plan)
	if !errors.Is(err, ErrInvalidVideoURL) {
		t.Fatalf("execute error = %v, want ErrInvalidVideoURL", err)
	}
}

func TestSearchWithNoResultsFailsClosed(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "asdkjaskldj"})
	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("empty results returned %v, want ErrNoResults", err)
	}
}

func TestEmptyQueryIsRejected(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "   "})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("blank query returned %v, want ErrEmptyQuery", err)
	}
}

func TestPlayCannotOutrunSearchBecauseItIsTheSameCall(t *testing.T) {
	// The manifest used to claim play completes while read did not. It cannot:
	// Resolve sends both verbs through the same search.list request, so a key
	// that 403s takes play down with it. This test is why that claim is gone.
	broken := errors.New("youtube: search failed with status 403")
	a := New(&fakeAPI{err: broken}, newLogger())

	for _, verb := range []manifest.Verb{manifest.Read, manifest.Play} {
		_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: verb, Subject: "bicycle repair"})
		if !errors.Is(err, broken) {
			t.Errorf("Resolve(%s) error = %v, want the search failure — no verb survives a broken search", verb, err)
		}
	}
}

func TestRevokePersistsTheDisconnectBeforeClearingTheLiveClient(t *testing.T) {
	revokes := 0
	a := NewWithRevoke(&fakeAPI{}, func(context.Context) error {
		revokes++
		return nil
	}, newLogger())
	if err := a.Revoke(t.Context()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revokes != 1 {
		t.Fatalf("persistent revoke calls = %d, want 1", revokes)
	}
	if _, err := a.Resolve(t.Context(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "query"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Resolve after revoke = %v, want ErrNotConnected", err)
	}
}
