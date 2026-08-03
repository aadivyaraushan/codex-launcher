package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	msoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
	msproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/microsoft"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// Callers: main.go liveDependencies.startMicrosoftProof / serve-microsoft-proof.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-microsoft-proof"
// Env schemas (never logged): MICROSOFT_CLIENT_ID, MICROSOFT_CLIENT_SECRET,
// MICROSOFT_REDIRECT_URI, MICROSOFT_TENANT. No data files.

func startMicrosoftProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("MICROSOFT_CLIENT_ID")
	clientSecret := os.Getenv("MICROSOFT_CLIENT_SECRET")
	tenant := os.Getenv("MICROSOFT_TENANT")
	if tenant == "" {
		tenant = "consumers"
	}
	redirectURI := os.Getenv("MICROSOFT_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "http://127.0.0.1:9195/oauth/microsoft/callback"
	}
	listenAddress := "127.0.0.1:9195"
	if parsed, err := url.Parse(redirectURI); err == nil && parsed.Hostname() != "" {
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "" {
			port = "9195"
		}
		if host == "localhost" {
			host = "127.0.0.1"
		}
		listenAddress = host + ":" + port
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("microsoft proof serve: stage 1 router: %w", err)
	}
	authorizationContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	connection, err := msproof.Authorize(authorizationContext, msproof.AuthorizationConfig{
		ListenAddress: listenAddress,
		RedirectURI:   redirectURI,
		Flow: msoauth.New(msoauth.Config{
			ClientID: clientID, ClientSecret: clientSecret, Tenant: tenant, Logger: logger,
		}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	mailAPI := outlook.NewHTTPClient("", connection, nil, logger)
	service, err := capabilityruntime.NewMicrosoft(capabilityruntime.MicrosoftConfig{
		API: mailAPI, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[microsoft-proof-serve] ephemeral capability flow ready",
		"token_storage", "memory_only", "adapter_count", 1, "user_oauth", true, "tenant", tenant)
	return service, connection, nil
}
