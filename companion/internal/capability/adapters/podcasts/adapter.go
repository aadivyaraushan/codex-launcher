package podcasts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "podcasts"

var (
	ErrAmbiguousEpisode = errors.New("podcasts: more than one episode matches; ask the user which one")
	ErrNoEpisode        = errors.New("podcasts: no episode matches")
	ErrMissingFeedURL   = errors.New("podcasts: feed URL is required")
	ErrNoEnclosure      = errors.New("podcasts: episode has no enclosure URL")
	ErrNotConnected     = errors.New("podcasts: feed client is not connected")
)

// FeedConfig names the default RSS URL for this adapter instance.
type FeedConfig struct {
	URL string
}

type Adapter struct {
	feedURL string
	feed    Feed
	logger  *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(config FeedConfig, feed Feed, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{feedURL: strings.TrimSpace(config.URL), feed: feed, logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read, manifest.Play},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthNone, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "podcasts_rss_enclosure_play_smoke",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.feed == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	feedURL := a.resolveFeedURL(in)
	a.logger.Info("[podcasts] resolve", "verb", in.Verb, "feed_url_len", len(feedURL), "subject_length", len(in.Subject))
	if feedURL == "" {
		return adapter.Plan{}, ErrMissingFeedURL
	}
	switch in.Verb {
	case manifest.Read:
		return a.resolveRead(ctx, feedURL)
	case manifest.Play:
		return a.resolvePlay(ctx, feedURL, in.Subject)
	default:
		return adapter.Plan{}, fmt.Errorf("podcasts: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveFeedURL(in adapter.Intent) string {
	if in.Fields != nil {
		if override := strings.TrimSpace(in.Fields["feed_url"]); override != "" {
			return override
		}
	}
	return a.feedURL
}

func (a *Adapter) resolveRead(ctx context.Context, feedURL string) (adapter.Plan, error) {
	episodes, err := a.feed.Fetch(ctx, feedURL)
	if err != nil {
		a.logger.Error("[podcasts] list episodes failed", "error", err)
		return adapter.Plan{}, err
	}
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Summary: "List podcast episodes",
		Details: map[string]string{
			"feed_url":      feedURL,
			"episode_count": strconv.Itoa(len(episodes)),
			"episodes":      formatEpisodeList(episodes),
		},
	}, nil
}

func (a *Adapter) resolvePlay(ctx context.Context, feedURL, query string) (adapter.Plan, error) {
	episodes, err := a.feed.Fetch(ctx, feedURL)
	if err != nil {
		a.logger.Error("[podcasts] play fetch failed", "error", err)
		return adapter.Plan{}, err
	}
	ep, err := matchEpisode(episodes, query)
	if err != nil {
		return adapter.Plan{}, err
	}
	if strings.TrimSpace(ep.EnclosureURL) == "" {
		return adapter.Plan{}, ErrNoEnclosure
	}
	a.logger.Info("[podcasts] play resolved", "guid", ep.GUID, "enclosure_type", ep.EnclosureType)
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Play, Handle: ep.GUID,
		Summary: "Play podcast episode",
		Details: map[string]string{
			"feed_url":         feedURL,
			"guid":             ep.GUID,
			"title":            ep.Title,
			"enclosure_url":    ep.EnclosureURL,
			"enclosure_type":   ep.EnclosureType,
			"enclosure_length": ep.EnclosureLength,
			"pub_date":         ep.PubDate,
			"link":             ep.Link,
		},
	}, nil
}

func matchEpisode(episodes []Episode, query string) (Episode, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		if len(episodes) == 0 {
			return Episode{}, ErrNoEpisode
		}
		if len(episodes) > 1 {
			return Episode{}, &adapter.ClarificationError{Question: "Which matching podcast episode did you mean?", Cause: ErrAmbiguousEpisode}
		}
		return episodes[0], nil
	}
	var exact, partial []Episode
	for _, ep := range episodes {
		title := strings.ToLower(ep.Title)
		guid := strings.ToLower(ep.GUID)
		if title == needle || guid == needle {
			exact = append(exact, ep)
		} else if strings.Contains(title, needle) || strings.Contains(guid, needle) {
			partial = append(partial, ep)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return Episode{}, ErrNoEpisode
	}
	if len(matches) > 1 {
		return Episode{}, &adapter.ClarificationError{Question: "Which matching podcast episode did you mean?", Cause: ErrAmbiguousEpisode}
	}
	return matches[0], nil
}

func formatEpisodeList(episodes []Episode) string {
	var b strings.Builder
	for i, ep := range episodes {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\t%s\t%s", ep.GUID, ep.Title, ep.EnclosureURL)
	}
	return b.String()
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	switch plan.Verb {
	case manifest.Read:
		count := plan.Details["episode_count"]
		lines := []string{count + " episodes"}
		for _, line := range strings.Split(plan.Details["episodes"], "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) >= 2 {
				lines = append(lines, parts[1])
			} else {
				lines = append(lines, line)
			}
		}
		return adapter.Preview{
			Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: "Show episodes",
		}, nil
	case manifest.Play:
		return adapter.Preview{
			Plan:     plan,
			Headline: plan.Summary,
			Lines:    []string{plan.Details["title"], plan.Details["enclosure_url"]},
			Confirm:  "Play episode",
		}, nil
	default:
		return adapter.Preview{}, fmt.Errorf("podcasts: verb %q is not supported", plan.Verb)
	}
}

func (a *Adapter) Execute(_ context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	a.logger.Info("[podcasts] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		detail := plan.Details["episodes"]
		a.logger.Info("[podcasts] execute complete", "verb", "read", "episode_count", plan.Details["episode_count"])
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: detail}, nil
	case manifest.Play:
		url := plan.Details["enclosure_url"]
		if strings.TrimSpace(url) == "" {
			return adapter.Outcome{}, ErrNoEnclosure
		}
		detail := fmt.Sprintf("play %s via %s", plan.Details["title"], url)
		a.logger.Info("[podcasts] execute complete", "verb", "play", "guid", plan.Handle, "done", true)
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: detail}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("podcasts: verb %q is not supported", plan.Verb)
	}
}

func (a *Adapter) Revoke(context.Context) error {
	a.logger.Info("[podcasts] revoke", "decision", "noop_no_credentials")
	return nil
}
