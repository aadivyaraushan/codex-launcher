package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	podcastsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/podcasts"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
)

// startPodcastsProof builds the owner-only Podcasts plain-RSS capability flow
// for Pixel proof. No OAuth — PODCASTS_FEED_URL plus OPENAI_API_KEY for routing.
//
// Callers: companion/cmd/codex-launcher/main.go serve-podcasts-proof.
// User ask: wire serve-podcasts-proof like Instagram (no OAuth; feed URL required).
func startPodcastsProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, error) {
	_ = ctx
	_ = output
	logger := slog.Default()
	feedURL := strings.TrimSpace(os.Getenv("PODCASTS_FEED_URL"))
	if feedURL == "" {
		return nil, fmt.Errorf("podcasts proof serve: PODCASTS_FEED_URL is required")
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("podcasts proof serve: stage 1 router: %w", err)
	}
	service, err := capabilityruntime.NewPodcasts(capabilityruntime.PodcastsConfig{
		FeedURL: feedURL,
		Feed:    podcastsadapter.NewHTTPClient(nil, logger),
		Model:   router.Model,
		Logger:  logger,
	})
	if err != nil {
		return nil, err
	}
	logger.Info("[podcasts-proof-serve] capability flow ready",
		"adapter_count", 1,
		"oauth", "none",
		"feed_url_len", len(feedURL),
	)
	return service, nil
}
