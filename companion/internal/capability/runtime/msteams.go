package runtime

import (
	"errors"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/msteams"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// Callers: companion/cmd/codex-launcher/msteams_proof.go and msteams_test.go.
// User: "Look at how capabilityruntime.NewMicrosoft works — only add NewMSTeams if cheap."
// Cheap mirror of NewMicrosoft / NewSlack for work/school Teams Graph chat.
// API: NewMSTeams(MSTeamsConfig) -> *flow.Service. No data files.

var ErrMissingMSTeamsDependency = errors.New("capability runtime: Teams API and routing model are required")

type MSTeamsConfig struct {
	API    msteams.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewMSTeams builds the companion-side flow used by the owner-only Teams work
// proof: stage 1 routing, local stage 2 selection, preview enforcement, and
// the real Graph Chat.ReadWrite adapter (organizations tenant).
func NewMSTeams(config MSTeamsConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingMSTeamsDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(msteams.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	// Teams' own adapter resolves Subject against a chat's topic (findChat
	// in the adapter, backed by ListChats), not against a person, so this
	// class has no person for the contact graph to resolve.
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"messaging": {Adapters: []string{msteams.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Microsoft Teams work flow ready",
		"adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
