package runtime

import (
	"errors"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// Callers: companion/cmd/codex-launcher/google_proof.go and google_test.go.
// User: "Implement least code for Calendar + Drive within Wave-1 ceilings."

var ErrMissingGoogleDependency = errors.New("capability runtime: Google Calendar API, Drive API, and routing model are required")

type GoogleConfig struct {
	CalendarAPI gcalendar.API
	DriveAPI    gdrive.API
	Model       stage1.ModelFunc
	Logger      *slog.Logger
}

// NewGoogle builds the companion-side flow used by the owner-only Google proof:
// stage 1 routing, local stage 2 selection, preview enforcement, and the real
// Calendar + Drive adapters under one user OAuth token.
func NewGoogle(config GoogleConfig) (*flow.Service, error) {
	if config.CalendarAPI == nil || config.DriveAPI == nil || config.Model == nil {
		return nil, ErrMissingGoogleDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	if err := reg.Register(gcalendar.New(config.CalendarAPI, config.Logger)); err != nil {
		return nil, err
	}
	if err := reg.Register(gdrive.New(config.DriveAPI, config.Logger)); err != nil {
		return nil, err
	}
	resolver := stage2.New(
		reg,
		contacts.NewGraph(time.Now),
		stage2.ClassMap{
			"calendar": {Adapters: []string{gcalendar.ID}, Addressing: stage2.ToAThing},
			"notes":    {Adapters: []string{gdrive.ID}, Addressing: stage2.ToAThing},
		},
		manifest.PlatformAndroid,
	)
	config.Logger.Info("[capability-runtime] Google flow ready", "adapter_count", 2, "class_count", 2, "platform", manifest.PlatformAndroid)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), ConsentGate(), config.Logger), nil
}
