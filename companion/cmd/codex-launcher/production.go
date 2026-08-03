package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/killswitch"
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
// Unlike the proof commands, nothing here waits on a person clicking
// through an OAuth consent screen: `serve` starts unattended, so Spotify,
// Todoist, Slack, Google Calendar/Drive, Outlook, and Teams are never
// registered by this path — see the oauthReason comment next to
// ProductionConfig in internal/capability/runtime/production.go for why.
//
// Callers: main.go runWith's plain "serve" branch.
func startProductionCapabilityFlow(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, error) {
	_ = output
	logger := slog.Default()
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("production capability flow: stage 1 router: %w", err)
	}
	service, inventory, err := capabilityruntime.NewProduction(capabilityruntime.ProductionConfig{
		Model:           router.Model,
		Logger:          logger,
		MapsAPIKey:      os.Getenv("GOOGLE_MAPS_API_KEY"),
		YouTubeAPIKey:   os.Getenv("YOUTUBE_API_KEY"),
		PodcastsFeedURL: os.Getenv("PODCASTS_FEED_URL"),
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
