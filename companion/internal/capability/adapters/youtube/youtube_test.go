package youtube

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	// Evidence: saved-results/wave1-youtube-maps-complete.md. Both verbs go
	// through search.list, and the live API key returns 403 because its Google
	// Cloud project is restricted, so nothing has ever been carried to the end
	// through this adapter. A ceiling is a measured fact, so it reads
	// hands_off until a real run says otherwise.
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Ceiling != manifest.HandsOff {
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
	// The adapter's own Read case reports completes, and on its own terms
	// that is honest — it returns a real answer and hands nothing off. But
	// the manifest ships hands_off, and a manifest is a cap: the runner
	// pulls the outcome down to it. That is the point of the clamp. If the
	// API key's restriction is ever lifted and a real run proves the search
	// works, raise the manifest and this expectation moves with it.
	if out.Reached != manifest.HandsOff || out.Done {
		t.Fatalf("outcome = %+v, want the runner to clamp it to the manifest's hands_off", out)
	}
}

// Requirement: search + open video (Play verb) opens the YouTube app.
// Opening an app is a hand-off, so the outcome says so.
func TestPlayResolvesAndHandsOffToYouTube(t *testing.T) {
	api := &fakeAPI{videos: []Video{
		{ID: "abc123", Title: "Some Song", ChannelTitle: "Some Artist"},
	}}
	a := New(api, newLogger())
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Play, Subject: "some song"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["watch_url"] != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("watch_url = %q", plan.Details["watch_url"])
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Opening the app is a hand-off: the video is playing in YouTube, and
	// what happens next is between the user and YouTube. An outcome that
	// names an app it handed to may not also claim it finished the job.
	if out.Reached != manifest.HandsOff || !out.Done || out.HandedOffTo != "YouTube" {
		t.Fatalf("outcome = %+v, want hands_off, done, handed off to YouTube", out)
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
