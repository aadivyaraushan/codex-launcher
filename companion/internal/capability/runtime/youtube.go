package runtime

import (
	"errors"
	"log/slog"
	"time"

	youtubeadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/youtube"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingYouTubeDependency = errors.New("capability runtime: YouTube API and routing model are required")

type YouTubeConfig struct {
	API    youtubeadapter.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewYouTube builds the companion-side flow for YouTube search/open: stage 1
// routing, local stage 2 selection under entertainment, preview, and the
// real YouTube adapter. No OAuth — the Data API v3 key is the only
// connection.
func NewYouTube(config YouTubeConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingYouTubeDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(youtubeadapter.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"entertainment": {Adapters: []string{youtubeadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] YouTube flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
