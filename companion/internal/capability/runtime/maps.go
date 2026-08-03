package runtime

import (
	"errors"
	"log/slog"
	"time"

	mapsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingMapsDependency = errors.New("capability runtime: Maps API client and routing model are required")

type MapsConfig struct {
	API    mapsadapter.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewMaps builds the companion-side flow for Google Maps places/directions
// (RT-2, completes): stage 1 routing, local stage 2 selection under travel,
// preview, and the RT-2 adapter. No OAuth — the Places/Routes API key is
// the only connection. Registers only the completes places/directions
// adapter (mapsadapter.ID "maps"), not the RT-4 saved-places hand-off
// (mapsadapter.SavedPlacesID "maps_saved_places"), which the deep-link pack
// already covers under app_named "googlemaps".
func NewMaps(config MapsConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingMapsDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(mapsadapter.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"travel": {Adapters: []string{mapsadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Maps flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
