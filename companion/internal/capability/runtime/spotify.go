package runtime

import (
	"errors"
	"log/slog"
	"time"

	spotifyadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/spotify"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingSpotifyDependency = errors.New("capability runtime: Spotify API and routing model are required")

type SpotifyConfig struct {
	API    spotifyadapter.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewSpotify builds the companion-side flow for Spotify search/play: stage 1
// routing, local stage 2 selection under music, preview, and the real
// Spotify adapter. Credentials stay behind the supplied API.
func NewSpotify(config SpotifyConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingSpotifyDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(spotifyadapter.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"music": {Adapters: []string{spotifyadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Spotify flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
