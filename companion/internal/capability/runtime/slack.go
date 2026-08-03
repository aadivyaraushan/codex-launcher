package runtime

import (
	"errors"
	"log/slog"
	"time"

	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingSlackDependency = errors.New("capability runtime: Slack API and routing model are required")

type SlackConfig struct {
	API    slackadapter.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewSlack builds the companion-side flow used by the owner-only Slack proof:
// stage 1 routing, local stage 2 selection, preview enforcement, and the real
// Slack user-OAuth adapter.
func NewSlack(config SlackConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingSlackDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(slackadapter.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	// Slack's own adapter resolves Subject against channel names, not
	// people — resolveChannel in the adapter, backed by ListChannels, which
	// only lists public_channel/private_channel and never DMs. So this
	// class has no person for the contact graph to resolve.
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"messaging": {Adapters: []string{slackadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Slack flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
