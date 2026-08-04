package runtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	notionadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var ErrMissingNotionDependency = errors.New("capability runtime: Notion session and routing model are required")

type NotionConfig struct {
	Session notionadapter.Session
	Model   stage1.ModelFunc
	Logger  *slog.Logger
}

// NewNotion measures the authenticated workspace's live MCP tool ceiling,
// then exposes the connected adapter through the same preview-enforced flow
// used by production.
func NewNotion(ctx context.Context, config NotionConfig) (*flow.Service, error) {
	if config.Session == nil || config.Model == nil {
		return nil, ErrMissingNotionDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	adapter, err := notionadapter.New(config.Session)
	if err != nil {
		return nil, err
	}
	ceiling, err := adapter.Connect(ctx)
	if err != nil {
		return nil, err
	}
	reg := registry.New()
	if err := reg.Register(adapter); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{"notes": {Adapters: []string{notionadapter.ID}, Addressing: stage2.ToAThing}},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Notion flow ready", "adapter_count", 1, "class_count", 1, "measured_ceiling", ceiling, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
