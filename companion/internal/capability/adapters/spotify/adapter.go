package spotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const (
	ID      = "spotify"
	AppName = "Spotify"
)

var (
	ErrEmptyQuery   = errors.New("spotify: search query must not be empty")
	ErrNoTrack      = errors.New("spotify: no track matches the search")
	ErrNotConnected = errors.New("spotify: adapter is not connected")
)

// API is the small part of the Spotify Web API the adapter needs.
// HTTPClient is the real implementation; tests use a recording fake.
type API interface {
	Search(context.Context, string) ([]Track, error)
	Devices(context.Context) ([]Device, error)
	Play(ctx context.Context, deviceID, trackURI string) error
	Clear(context.Context) error
}

type Adapter struct {
	api    API
	logger *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(api API, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{api: api, logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read, manifest.Play},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates: []manifest.Gate{manifest.GateNone},
		// Spotify's developer apps start in self-serve Development Mode,
		// capped at 25 registered users, until the owner applies for
		// extended quota mode. See planning/consumer-app-implementation-plan.md
		// ("Spotify Web API is self-serve OAuth").
		Capacity: manifest.Capacity{Kind: manifest.CapacityCapped, Limit: 25},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "spotify_search_play_smoke",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[spotify] resolve", "verb", in.Verb, "subject_length", len(in.Subject))
	query := strings.TrimSpace(in.Subject)
	if query == "" {
		return adapter.Plan{}, ErrEmptyQuery
	}
	switch in.Verb {
	case manifest.Read:
		return a.resolveSearch(ctx, query)
	case manifest.Play:
		return a.resolvePlay(ctx, query)
	default:
		return adapter.Plan{}, fmt.Errorf("spotify: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveSearch(ctx context.Context, query string) (adapter.Plan, error) {
	track, err := a.searchOne(ctx, query)
	if err != nil {
		return adapter.Plan{}, err
	}
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Summary: "Search Spotify",
		Details: map[string]string{
			"track_id": track.ID, "track_name": track.Name,
			"track_artist": track.Artist, "track_uri": track.URI,
		},
	}, nil
}

func (a *Adapter) resolvePlay(ctx context.Context, query string) (adapter.Plan, error) {
	track, err := a.searchOne(ctx, query)
	if err != nil {
		return adapter.Plan{}, err
	}
	devices, err := a.api.Devices(ctx)
	if err != nil {
		a.logger.Error("[spotify] list devices failed", "error", err)
		return adapter.Plan{}, err
	}
	deviceID, deviceName := selectDevice(devices)
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Play, Summary: "Play on Spotify",
		Details: map[string]string{
			"track_id": track.ID, "track_name": track.Name,
			"track_artist": track.Artist, "track_uri": track.URI,
			"device_id": deviceID, "device_name": deviceName,
		},
	}, nil
}

func (a *Adapter) searchOne(ctx context.Context, query string) (Track, error) {
	tracks, err := a.api.Search(ctx, query)
	if err != nil {
		a.logger.Error("[spotify] search failed", "error", err)
		return Track{}, err
	}
	if len(tracks) == 0 {
		return Track{}, ErrNoTrack
	}
	return tracks[0], nil
}

// selectDevice prefers the active device; when none is active it targets
// the first device Spotify reports, on the theory that PUT .../play with an
// explicit device_id can still transfer playback there. When there are no
// devices at all it returns an empty id, and Execute still makes the real
// call — Spotify's own response is what decides completes vs. hands_off.
func selectDevice(devices []Device) (id, name string) {
	for _, d := range devices {
		if d.IsActive {
			return d.ID, d.Name
		}
	}
	if len(devices) > 0 {
		return devices[0].ID, devices[0].Name
	}
	return "", ""
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	track := trackLine(plan.Details)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Preview{
			Plan: plan, Headline: plan.Summary,
			Lines: []string{track}, Confirm: "Show track",
		}, nil
	case manifest.Play:
		lines := []string{track}
		if deviceName := plan.Details["device_name"]; deviceName != "" {
			lines = append(lines, "Device: "+deviceName)
		} else {
			lines = append(lines, "No Spotify device found — Operator will try to start playback and open Spotify if it can't.")
		}
		return adapter.Preview{
			Plan: plan, Headline: plan.Summary,
			Lines: lines, Confirm: "Play",
		}, nil
	default:
		return adapter.Preview{}, fmt.Errorf("spotify: verb %q is not supported", plan.Verb)
	}
}

func trackLine(details map[string]string) string {
	name, artist := details["track_name"], details["track_artist"]
	if artist == "" {
		return name
	}
	return fmt.Sprintf("%s — %s", name, artist)
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[spotify] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: trackLine(plan.Details)}, nil
	case manifest.Play:
		return a.executePlay(ctx, plan)
	default:
		return adapter.Outcome{}, fmt.Errorf("spotify: verb %q is not supported", plan.Verb)
	}
}

func (a *Adapter) executePlay(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	err := a.api.Play(ctx, plan.Details["device_id"], plan.Details["track_uri"])
	if err == nil {
		a.logger.Info("[spotify] execute complete", "reached", manifest.Completes, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("started playback of %s", trackLine(plan.Details)),
		}, nil
	}
	if errors.Is(err, ErrNoActiveDevice) {
		a.logger.Warn("[spotify] no active device; demoting to hand-off", "track", plan.Details["track_name"])
		out := noActiveDeviceOutcome(trackLine(plan.Details))
		a.logger.Info("[spotify] execute complete", "reached", out.Reached, "done", out.Done, "handed_off_to", out.HandedOffTo)
		return out, nil
	}
	a.logger.Error("[spotify] play failed", "error", err)
	return adapter.Outcome{}, err
}

// noActiveDeviceOutcome is the demoted outcome for a play attempt that
// failed because Spotify reported no active device — the plan's fail
// condition, per planning/consumer-app-implementation-plan.md's Pixel 9
// self-verification table: "Clear 'no active device' = fail, not COMPLETE".
// It never claims playback started, matching the wire contract that
// handoff.DraftOutcome uses elsewhere: hands_off, done=true, a named
// HandedOffTo, and an honest "cannot know" detail.
func noActiveDeviceOutcome(track string) adapter.Outcome {
	return adapter.Outcome{
		Reached:     manifest.HandsOff,
		Done:        true,
		HandedOffTo: AppName,
		Detail: fmt.Sprintf(
			"Spotify reported no active device for %s. Operator opened Spotify so you can pick a device and press play yourself — it cannot know whether that worked.",
			track,
		),
	}
}

// Revoke of an already-disconnected adapter reports success, not
// ErrNotConnected — see the comment on todoist's Revoke for why (a retried
// revoke should never look like a failed disconnect).
func (a *Adapter) Revoke(ctx context.Context) error {
	if a.api == nil {
		return nil
	}
	if err := a.api.Clear(ctx); err != nil {
		a.logger.Error("[spotify] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[spotify] revoked")
	return nil
}
