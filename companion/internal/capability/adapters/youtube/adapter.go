// Package youtube is Operator's RT-2 adapter for YouTube search and open.
// It uses the YouTube Data API v3 (search.list) with a plain project API
// key — there is no per-user OAuth step. Both the read (search) verb and the
// play (open a video) verb resolve through that same search.list call. The
// Operator project key completed that exact call on 2026-08-04, returning
// five usable video rows, so read now has a measured completes ceiling.
// Play still returns hands_off because it opens the resolved URL on Android.
// Like/subscribe and in-app chrome remote control are explicitly deferred.
package youtube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "youtube"

const androidPackage = "com.google.android.youtube"

var (
	ErrNotConnected = errors.New("youtube: adapter is not connected")
	ErrEmptyQuery   = errors.New("youtube: search query must not be empty")
	ErrNoResults    = errors.New("youtube: no video matches the search")
)

// API is the small part of YouTube the adapter needs. HTTPClient is the
// real implementation; tests use a recording fake.
type API interface {
	Search(ctx context.Context, query string) ([]Video, error)
}

type Adapter struct {
	api    API
	revoke func(context.Context) error
	logger *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(api API, logger *slog.Logger) *Adapter {
	return NewWithRevoke(api, nil, logger)
}

func NewWithRevoke(api API, revoke func(context.Context) error, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{api: api, revoke: revoke, logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read, manifest.Play},
		Ceiling: manifest.Completes,
		Consent: manifest.ConsentA,
		Auth:    manifest.AuthNone, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityCapped, Limit: 100},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "youtube_search_open_smoke",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[youtube] resolve", "verb", in.Verb, "subject_length", len(in.Subject))
	switch in.Verb {
	case manifest.Read, manifest.Play:
		return a.resolveSearch(ctx, in.Verb, in.Subject)
	default:
		return adapter.Plan{}, fmt.Errorf("youtube: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveSearch(ctx context.Context, verb manifest.Verb, query string) (adapter.Plan, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return adapter.Plan{}, ErrEmptyQuery
	}
	videos, err := a.api.Search(ctx, query)
	if err != nil {
		a.logger.Error("[youtube] search failed", "error", err)
		return adapter.Plan{}, err
	}
	if len(videos) == 0 {
		return adapter.Plan{}, ErrNoResults
	}
	v := videos[0]
	summary := "Search YouTube"
	if verb == manifest.Play {
		summary = "Play a video on YouTube"
	}
	return adapter.Plan{
		AdapterID: ID, Verb: verb, Handle: v.ID, Summary: summary,
		Details: map[string]string{
			"video_id": v.ID, "title": v.Title, "channel": v.ChannelTitle,
			"watch_url":       "https://www.youtube.com/watch?v=" + v.ID,
			"android_package": androidPackage,
		},
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	lines := []string{plan.Details["title"]}
	if channel := plan.Details["channel"]; channel != "" {
		lines = append(lines, channel)
	}
	confirm := "Show result"
	if plan.Verb == manifest.Play {
		confirm = "Open in YouTube"
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(_ context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[youtube] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("Found %q by %s on YouTube.", plan.Details["title"], plan.Details["channel"]),
		}, nil
	case manifest.Play:
		return adapter.Outcome{
			Reached: manifest.HandsOff, Done: true, HandedOffTo: "YouTube",
			Detail: fmt.Sprintf("Found %q by %s. Open YouTube to choose and play it; Operator cannot deep-link this result yet.", plan.Details["title"], plan.Details["channel"]),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("youtube: verb %q is not supported", plan.Verb)
	}
}

// Revoke of an already-disconnected adapter reports success, not
// ErrNotConnected — see the comment on todoist's Revoke for why (a retried
// revoke should never look like a failed disconnect).
func (a *Adapter) Revoke(ctx context.Context) error {
	if a.api == nil {
		return nil
	}
	if a.revoke != nil {
		if err := a.revoke(ctx); err != nil {
			a.logger.Error("[youtube] persistent revoke failed", "error", err)
			return err
		}
	}
	a.api = nil
	a.logger.Info("[youtube] revoked")
	return nil
}
