package discovery

import (
	"log/slog"

	beepermessage "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/phonerules"
	stage1explicit "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/explicit"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// NewPhoneRouter builds the stage 1 router the same way phoneruntime.Open
// and production.go's no-OpenAI-key fallback both do: one explicit-app rule
// per Wave1Specs entry, Beeper-aware via routing/phonerules so a moved app's
// rule and the class the resolver actually finds it in never drift apart.
func NewPhoneRouter(logger *slog.Logger, beeperOn bool) RouterFunc {
	return stage1explicit.New(phonerules.Build(beeperOn), logger).Route
}

// NewPhoneSweep builds a Sweep against the real phone router and both
// Beeper states' real inventories — the same capabilityruntime.NewProduction
// call production wiring makes, just with a NoOpBeeperAPI standing in for a
// live Beeper connection so building the Beeper-on inventory needs no
// network access. Each state gets its own router, built for that state.
func NewPhoneSweep(logger *slog.Logger) (Sweep, error) {
	routerOff := NewPhoneRouter(logger, false)
	routerOn := NewPhoneRouter(logger, true)
	off, err := buildEnvironment(logger, routerOff, nil)
	if err != nil {
		return Sweep{}, err
	}
	on, err := buildEnvironment(logger, routerOn, NoOpBeeperAPI{})
	if err != nil {
		return Sweep{}, err
	}
	return Sweep{BeeperOff: off, BeeperOn: on}, nil
}

func buildEnvironment(logger *slog.Logger, router RouterFunc, beeperAPI beepermessage.API) (Environment, error) {
	config := capabilityruntime.ProductionConfig{Model: router, Logger: logger, BeeperAPI: beeperAPI}
	_, inv, err := capabilityruntime.NewProduction(config)
	if err != nil {
		return Environment{}, err
	}
	return Environment{Router: router, Resolve: inv.Resolve, Manifest: inv.Manifest}, nil
}
