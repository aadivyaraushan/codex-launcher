package runtime

import (
	"errors"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// Callers: companion/cmd/codex-launcher/microsoft_proof.go and microsoft_test.go.
// User: "Implement least code for Wave-1 Outlook (mail) within plan ceilings."
// API: NewMicrosoft(MicrosoftConfig) -> *flow.Service. No data files.

var ErrMissingMicrosoftDependency = errors.New("capability runtime: Outlook API and routing model are required")

type MicrosoftConfig struct {
	API    outlook.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewMicrosoft builds the companion-side flow used by the owner-only Microsoft
// proof: stage 1 routing, local stage 2 selection, preview enforcement, and
// the real Outlook Graph user-OAuth adapter.
func NewMicrosoft(config MicrosoftConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingMicrosoftDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(outlook.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"email": {Adapters: []string{outlook.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Microsoft Outlook flow ready",
		"adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
