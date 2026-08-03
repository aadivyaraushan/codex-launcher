// Package runtime composes capability adapters with the shared two-stage
// router and execution flow used by the mobile companion.
package runtime

import (
	"errors"
	"log/slog"
	"time"

	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingTodoistDependency = errors.New("capability runtime: Todoist API and routing model are required")

type TodoistConfig struct {
	API    todoistadapter.API
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// NewTodoist builds the exact companion-side flow used by the owner-only
// Pixel proof: stage 1 routing, local stage 2 selection, preview enforcement,
// and the real Todoist adapter. Credentials stay behind the supplied API.
func NewTodoist(config TodoistConfig) (*flow.Service, error) {
	if config.API == nil || config.Model == nil {
		return nil, ErrMissingTodoistDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(todoistadapter.New(config.API, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"tasks": {Adapters: []string{todoistadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Todoist flow ready", "adapter_count", 1, "class_count", 1, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
