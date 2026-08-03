// Package instagram wires the Instagram draft-and-open adapter into the
// shared two-stage router and execution flow used by the mobile companion.
package instagram

import (
	"errors"
	"log/slog"
	"time"

	instagramadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/instagram"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

var ErrMissingDependency = errors.New("capability runtime: Instagram routing model is required")

type Config struct {
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// New builds the companion-side flow for Instagram draft-and-open: stage 1
// routing, local stage 2 selection under messaging, preview, and the
// hand-off adapter. No OAuth or account credentials are involved.
func New(config Config) (*flow.Service, error) {
	if config.Model == nil {
		return nil, ErrMissingDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(instagramadapter.New(config.Logger)); err != nil {
		return nil, err
	}
	// Instagram never reads contacts or account data, so messaging is not
	// addressed_to_person here. The subject is only a drafting hint.
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"messaging": {Adapters: []string{instagramadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Instagram flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), capabilityruntime.ConsentGate(), config.Logger), nil
}
