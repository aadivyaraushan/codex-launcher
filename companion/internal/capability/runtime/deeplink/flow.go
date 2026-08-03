// Package deeplink wires Wave-1 money/food/media/messaging/rides/travel/
// services/finance/notes/shopping/tasks prepare-and-open adapters into the shared
// two-stage router used by the mobile companion.
//
// Callers: companion/cmd/codex-launcher/deeplink_proof.go (serve-deeplink-proof).
// API: New(Config{Model, Logger}) (*flow.Service, error). No OAuth schema.
// User ask (verbatim intent): overnight Group A Uber/Uber Eats/Resy/DoorDash
// hands_off prepare-and-open (no partner credentials).
package deeplink

import (
	"errors"
	"log/slog"
	"time"

	deeplinkadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

var ErrMissingDependency = errors.New("capability runtime: deeplink routing model is required")

type Config struct {
	Model  stage1.ModelFunc
	Logger *slog.Logger
}

// New builds the companion-side flow for Wave-1 prepare-and-open Specs
// (money/food/media/messaging/rides/travel/services/finance/notes/shopping/tasks).
// Stage 1/2 classes come from each Spec's AppClass. Do not use "payments" —
// stage1 rejects pay, and money is draft-only compose. Ceiling is always
// hands_off: no partner API, no OAuth, no completion claims.
func New(config Config) (*flow.Service, error) {
	if config.Model == nil {
		return nil, ErrMissingDependency
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	reg := registry.New()
	byClass := map[string][]string{}
	for _, spec := range deeplinkadapter.Wave1Specs() {
		if err := reg.Register(deeplinkadapter.New(spec, config.Logger)); err != nil {
			return nil, err
		}
		byClass[spec.AppClass] = append(byClass[spec.AppClass], spec.ID)
	}
	// None of these adapters ever resolve a handle: each one only prepares
	// a draft and opens the named app, and the person the message is for is
	// picked by hand once that app is open (see Resolve in adapter.go,
	// which carries Subject through as a "subject_hint" and never touches
	// the contact graph or a Handle). So every class here — including
	// messaging and money — has no person for stage 2 to resolve.
	classes := stage2.ClassMap{}
	for class, adapters := range byClass {
		classes[class] = stage2.Class{Adapters: adapters, Addressing: stage2.ToAThing}
	}
	resolver := stage2.New(reg, contacts.NewGraph(time.Now), classes, manifest.PlatformAndroid)
	config.Logger.Info("[capability-runtime] deeplink flow ready",
		"adapter_count", len(deeplinkadapter.Wave1Specs()),
		"money_adapters", len(byClass["money"]),
		"food_adapters", len(byClass["food"]),
		"media_adapters", len(byClass["media"]),
		"messaging_adapters", len(byClass["messaging"]),
		"rides_adapters", len(byClass["rides"]),
		"travel_adapters", len(byClass["travel"]),
		"services_adapters", len(byClass["services"]),
		"finance_adapters", len(byClass["finance"]),
		"notes_adapters", len(byClass["notes"]),
		"shopping_adapters", len(byClass["shopping"]),
		"tasks_adapters", len(byClass["tasks"]),
		"platform", manifest.PlatformAndroid,
	)
	return flow.New(stage1.New(config.Model), resolver, execution.New(reg), capabilityruntime.ConsentGate(), config.Logger), nil
}
