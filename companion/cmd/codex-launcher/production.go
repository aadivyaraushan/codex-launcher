package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	beepermessage "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/beepermessage"
	deeplinkadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	notionadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	youtubeadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/youtube"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/killswitch"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	stage1explicit "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/explicit"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/verification/alerts"
)

// killListRefreshInterval is how often the background watcher re-checks the
// remote kill list once the companion is up and serving. It only needs to
// be frequent enough that a newly killed adapter stops being reachable
// within a reasonable window — this is not a per-request check.
const killListRefreshInterval = 15 * time.Minute

// alertSweepInterval is how often the background watcher re-checks
// Telemetry's per-adapter reports for a fresh rot signal. Unlike the kill
// list, this costs nothing to run more often — it only reads counters
// already sitting in memory, no network call and no third-party account —
// so it runs on a much shorter cadence than the kill list check, short
// enough that a quiet regression shows up the same day it starts rather
// than getting lost in the next morning's traffic.
const alertSweepInterval = 5 * time.Minute

// startProductionCapabilityFlow builds the capability flow plain `serve`
// actually runs with. Every other adapter in this repo was reachable only
// from its own owner-only `serve-<name>-proof` command, run by hand, once,
// for the person who wrote it — so an ordinary phone talking to an ordinary
// `serve` companion always got dependencies.capabilityFlow == nil, and
// handler.go refused every capability request outright. This is the seam
// that closes that gap: it reads the same environment variables the proof
// commands already read (OPENAI_API_KEY for the router, plus whichever of
// GOOGLE_MAPS_API_KEY, YOUTUBE_API_KEY, and PODCASTS_FEED_URL happen to be
// set) and hands them to capabilityruntime.NewProduction.
//
// `serve` never waits on an interactive consent screen. Instead it restores
// the restart-safe OAuth records created by the proof commands and registers
// the connections that are currently usable. Providers with no complete
// stored record stay absent, so startup remains unattended.
//
// Callers: main.go runWith's plain "serve" branch.
func startProductionCapabilityFlow(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, error) {
	_ = output
	logger := slog.Default()
	secrets := credentialstore.NewKeychain(logger)
	disconnectIDs := []string{youtubeadapter.ID}
	for _, spec := range beepermessage.ProductionSpecs() {
		disconnectIDs = append(disconnectIDs, spec.ID)
	}
	disconnected, err := productionDisconnects(ctx, secrets, disconnectIDs, logger)
	if err != nil {
		return nil, fmt.Errorf("production capability flow: load durable disconnects: %w", err)
	}
	oauthConnections := loadProductionOAuth(ctx, secrets, logger)
	routingModel, routingSource, err := productionRoutingModel(logger)
	if err != nil {
		return nil, fmt.Errorf("production capability flow: stage 1 router: %w", err)
	}
	logger.Info("[production-serve] stage 1 router ready", "source", routingSource)
	var connectedNotion *notionadapter.Adapter
	if oauthConnections.Notion != nil {
		candidate, buildErr := notionadapter.New(oauthConnections.Notion)
		if buildErr == nil {
			ceiling, connectErr := candidate.Connect(ctx)
			buildErr = connectErr
			if connectErr == nil {
				connectedNotion = candidate
				logger.Info("[production-serve] Notion enabled", "measured_ceiling", ceiling)
			}
		}
		if buildErr != nil {
			logger.Warn("[production-serve] Notion connection unavailable", "error", buildErr)
		}
	}
	var beeperAPI *beeper.Client
	beeperToken := strings.TrimSpace(os.Getenv("BEEPER_ACCESS_TOKEN"))
	beeperReadOnly := envEnabled(os.Getenv("BEEPER_READONLY"))
	if beeperToken != "" && !beeperReadOnly {
		beeperAPI = beeper.NewClient(os.Getenv("BEEPER_DESKTOP_BASE_URL"), beeper.StaticToken(beeperToken), nil, logger)
		logger.Info("[production-serve] Beeper messaging enabled", "write_enabled", true, "token_present", true)
	} else {
		logger.Info("[production-serve] Beeper messaging unavailable", "token_present", beeperToken != "", "read_only", beeperReadOnly)
	}
	service, inventory, err := capabilityruntime.NewProduction(capabilityruntime.ProductionConfig{
		Model:             routingModel,
		Logger:            logger,
		MapsAPIKey:        os.Getenv("GOOGLE_MAPS_API_KEY"),
		YouTubeAPIKey:     productionSecret(ctx, os.Getenv("YOUTUBE_API_KEY"), "youtube_api_key", secrets, logger),
		PodcastsFeedURL:   os.Getenv("PODCASTS_FEED_URL"),
		BeeperAPI:         beeperAPI,
		GoogleCalendarAPI: oauthConnections.GoogleCalendar,
		GoogleDriveAPI:    oauthConnections.GoogleDrive,
		SlackAPI:          oauthConnections.Slack,
		OutlookAPI:        oauthConnections.Outlook,
		SpotifyAPI:        oauthConnections.Spotify,
		NotionAdapter:     connectedNotion,
		Disconnected:      disconnected,
		PersistDisconnect: func(ctx context.Context, id string) error { return persistProductionDisconnect(ctx, secrets, id) },
	})
	if err != nil {
		return nil, fmt.Errorf("production capability flow: %w", err)
	}
	logger.Info("[production-serve] capability flow ready",
		"registered_count", len(inventory.Registered),
		"skipped_count", len(inventory.Skipped),
	)

	startKillSwitch(ctx, inventory, logger)
	startAlertWatcher(ctx, inventory, logger)

	return service, nil
}

func productionRoutingModel(logger *slog.Logger) (stage1.ModelFunc, string, error) {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		client, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
		if err != nil {
			return nil, "", err
		}
		return client.Model, "openai", nil
	}
	specs := deeplinkadapter.Wave1Specs()
	rules := make([]stage1explicit.Rule, 0, len(specs))
	for _, spec := range specs {
		rules = append(rules, stage1explicit.Rule{ID: spec.ID, Name: spec.AppName, AppClass: spec.AppClass, Verbs: spec.Verbs})
	}
	model := stage1explicit.New(rules, logger)
	return model.Route, "explicit_app", nil
}

type secretReader interface {
	Get(context.Context, string) ([]byte, error)
}

type disconnectStore interface {
	Get(context.Context, string) ([]byte, error)
	Put(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

func productionDisconnectName(id string) string {
	return credentialstore.AdapterDisconnectName(id)
}

func productionDisconnects(ctx context.Context, store disconnectStore, ids []string, logger *slog.Logger) (map[string]bool, error) {
	disconnected := map[string]bool{}
	for _, id := range ids {
		if _, err := store.Get(ctx, productionDisconnectName(id)); err == nil {
			disconnected[id] = true
			logger.Info("[production-serve] adapter remains disconnected", "adapter_id", id)
		} else if !errors.Is(err, credentialstore.ErrNotFound) {
			logger.Error("[production-serve] disconnect state unavailable", "adapter_id", id, "error", err)
			return nil, fmt.Errorf("read disconnect state for %s: %w", id, err)
		}
	}
	return disconnected, nil
}

func persistProductionDisconnect(ctx context.Context, store disconnectStore, id string) error {
	return store.Put(ctx, productionDisconnectName(id), []byte("1"))
}

func clearProductionDisconnect(ctx context.Context, store disconnectStore, id string) error {
	return store.Delete(ctx, productionDisconnectName(id))
}

// productionSecret keeps explicit process configuration as the override, then
// falls back to the logged-in user's Keychain for restart-safe local service.
// It logs only where the value came from, never the value itself.
func productionSecret(ctx context.Context, environmentValue, name string, store secretReader, logger *slog.Logger) string {
	if value := strings.TrimSpace(environmentValue); value != "" {
		logger.Info("[production-serve] credential loaded", "name", name, "source", "environment")
		return value
	}
	secret, err := store.Get(ctx, name)
	if err != nil {
		logger.Info("[production-serve] credential unavailable", "name", name, "source", "keychain")
		return ""
	}
	logger.Info("[production-serve] credential loaded", "name", name, "source", "keychain")
	return strings.TrimSpace(string(secret))
}

func envEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// startAlertWatcher wires the alert sweep to the registry and Telemetry
// this process is actually serving out of. Telemetry already folds every
// real production outcome back in and works out whether an adapter has
// quietly fallen short of what it claims — inventory satisfies
// alerts.Source directly, so the watcher reads that same live signal
// rather than a fresh, empty one. See
// internal/capability/verification/alerts for the no-repeat rule that
// keeps this from turning into a log line nobody reads.
func startAlertWatcher(ctx context.Context, inventory capabilityruntime.Inventory, logger *slog.Logger) {
	watcher := alerts.New(inventory, logger)
	go watcher.Run(ctx, alertSweepInterval)
}

// startKillSwitch wires the remote kill list into the registry this
// process is actually serving out of. With no URL configured this changes
// nothing — that is today's behaviour and stays that way. With a URL
// configured, it does one check before returning, so a killed adapter is
// never reachable even for the brief window before the background loop's
// first tick, and then keeps checking in the background for as long as the
// companion runs.
func startKillSwitch(ctx context.Context, inventory capabilityruntime.Inventory, logger *slog.Logger) {
	killListURL := strings.TrimSpace(os.Getenv("CAPABILITY_KILL_LIST_URL"))
	if killListURL == "" {
		logger.Info("[production-serve] no CAPABILITY_KILL_LIST_URL configured; kill switch is inactive")
		return
	}

	source := killswitch.NewHTTPSource(killListURL, nil, logger)
	watcher := killswitch.New(inventory, source, logger)

	if err := watcher.Refresh(ctx); err != nil {
		// A companion that refuses to start because a kill list was
		// unreachable is worse than one running yesterday's kills, so log
		// and keep serving rather than failing startup.
		logger.Warn("[production-serve] initial kill list refresh failed; continuing with whatever kills were already in place", "error", err.Error())
	}

	go watcher.Run(ctx, killListRefreshInterval)
}
