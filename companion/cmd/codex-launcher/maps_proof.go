package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	mapsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// startMapsProof builds the owner-only Google Maps places/directions
// completes capability flow for Pixel proof, following the Podcasts
// no-OAuth pattern: GOOGLE_MAPS_API_KEY plus OPENAI_API_KEY for routing.
//
// Callers: companion/cmd/codex-launcher/main.go serve-maps-proof.
// User ask: close the Maps places/directions Pixel row — wire the real
// Places API/Routes API answer into an Operator session preview like
// Podcasts and Todoist already do, instead of only proving it via adb.
func startMapsProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, error) {
	_ = ctx
	_ = output
	logger := slog.Default()
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("maps proof serve: GOOGLE_MAPS_API_KEY is required")
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("maps proof serve: stage 1 router: %w", err)
	}
	client := mapsadapter.NewHTTPClient("", "", apiKey, nil, logger)
	service, err := capabilityruntime.NewMaps(capabilityruntime.MapsConfig{
		API:    client,
		Model:  router.Model,
		Logger: logger,
	})
	if err != nil {
		return nil, err
	}
	logger.Info("[maps-proof-serve] capability flow ready",
		"adapter_count", 1,
		"oauth", "none",
		"api_key_len", len(apiKey),
	)
	return service, nil
}
