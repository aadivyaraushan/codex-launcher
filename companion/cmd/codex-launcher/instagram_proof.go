package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	instagramruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime/instagram"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
)

// startInstagramProof builds the owner-only Instagram draft-and-open capability
// flow for Pixel proof. No OAuth or tokens — only OPENAI_API_KEY for routing.
//
// Callers: companion/cmd/codex-launcher/main.go serve-instagram-proof.
// User ask: judge follow-up — live Instagram path before Pixel proof.
func startInstagramProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, error) {
	_ = ctx
	_ = output
	logger := slog.Default()
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("instagram proof serve: stage 1 router: %w", err)
	}
	service, err := instagramruntime.New(instagramruntime.Config{Model: router.Model, Logger: logger})
	if err != nil {
		return nil, err
	}
	logger.Info("[instagram-proof-serve] capability flow ready", "adapter_count", 1, "oauth", "none")
	return service, nil
}
