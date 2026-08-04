package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	youtubeadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/youtube"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// startYouTubeProof builds the owner-only YouTube search/open capability
// flow for Pixel proof, following the Maps no-OAuth pattern: YOUTUBE_API_KEY
// plus OPENAI_API_KEY for routing.
//
// Callers: companion/cmd/codex-launcher/main.go serve-youtube-proof.
// User ask: close the YouTube search/open Pixel row — wire the real Data
// API v3 answer into an Operator session preview like Maps and Podcasts
// already do, instead of only proving it via adb.
func startYouTubeProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, error) {
	_ = output
	logger := slog.Default()
	secrets := credentialstore.NewKeychain(logger)
	apiKey := strings.TrimSpace(productionSecret(ctx, os.Getenv("YOUTUBE_API_KEY"), "youtube_api_key", secrets, logger))
	if apiKey == "" {
		return nil, fmt.Errorf("youtube proof serve: YOUTUBE_API_KEY or Keychain youtube_api_key is required")
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("youtube proof serve: stage 1 router: %w", err)
	}
	client := youtubeadapter.NewHTTPClient("", apiKey, nil, logger)
	service, err := capabilityruntime.NewYouTube(capabilityruntime.YouTubeConfig{
		API:    client,
		Model:  router.Model,
		Logger: logger,
	})
	if err != nil {
		return nil, err
	}
	if err := clearProductionDisconnect(ctx, secrets, youtubeadapter.ID); err != nil {
		return nil, fmt.Errorf("youtube proof serve: clear prior disconnect: %w", err)
	}
	logger.Info("[youtube-proof-serve] capability flow ready",
		"adapter_count", 1,
		"oauth", "none",
		"api_key_len", len(apiKey),
	)
	return service, nil
}
