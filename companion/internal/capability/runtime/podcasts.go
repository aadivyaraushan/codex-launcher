package runtime

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	podcastsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/podcasts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingPodcastsDependency = errors.New("capability runtime: Podcasts feed URL, feed client, and routing model are required")

type PodcastsConfig struct {
	FeedURL string
	Feed    podcastsadapter.Feed
	Model   stage1.ModelFunc
	Logger  *slog.Logger
}

// NewPodcasts builds the companion-side flow for plain-RSS Podcasts: stage 1
// routing, local stage 2 selection under media, preview, and the RT-2
// adapter. No OAuth — the feed URL is the only connection.
func NewPodcasts(config PodcastsConfig) (*flow.Service, error) {
	if strings.TrimSpace(config.FeedURL) == "" || config.Feed == nil || config.Model == nil {
		return nil, ErrMissingPodcastsDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(podcastsadapter.New(podcastsadapter.FeedConfig{URL: config.FeedURL}, config.Feed, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"media": {Adapters: []string{podcastsadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Podcasts flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
