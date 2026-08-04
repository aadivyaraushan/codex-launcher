package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/msteams"
	msoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
	msproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/microsoft"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// Callers: main.go liveDependencies.startMSTeamsProof / serve-msteams-proof.
// User: "Mirror serve-microsoft-proof / microsoft_proof.go for work Teams chat"
// Env schemas (never logged): MICROSOFT_CLIENT_ID,
// MICROSOFT_TEAMS_REDIRECT_URI or MICROSOFT_REDIRECT_URI, MICROSOFT_TEAMS_TENANT
// (default organizations). Port 9196 (distinct from Outlook 9195). No data files.
//
// Overnight shape: AuthorizeChat (StartChat + Chat.ReadWrite) → list chats read
// smoke → NewMSTeams capability flow (cheap mirror of NewMicrosoft). Live chat
// send stays owner-gated behind preview. Token stays in memory until serve stops.

func startMSTeamsProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("MICROSOFT_CLIENT_ID")
	tenant := strings.TrimSpace(os.Getenv("MICROSOFT_TEAMS_TENANT"))
	if tenant == "" {
		tenant = msoauth.TenantOrganizations
	}
	redirectURI := os.Getenv("MICROSOFT_TEAMS_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = os.Getenv("MICROSOFT_REDIRECT_URI")
	}
	if redirectURI == "" {
		redirectURI = "http://127.0.0.1:9196/oauth/microsoft/callback"
	}
	listenAddress := "127.0.0.1:9196"
	if parsed, err := url.Parse(redirectURI); err == nil && parsed.Hostname() != "" {
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "" {
			port = "9196"
		}
		if host == "localhost" {
			host = "127.0.0.1"
		}
		listenAddress = host + ":" + port
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("msteams proof serve: stage 1 router: %w", err)
	}
	authorizationContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	connection, err := msproof.AuthorizeChat(authorizationContext, msproof.AuthorizationConfig{
		ListenAddress: listenAddress,
		RedirectURI:   redirectURI,
		Flow: msoauth.New(msoauth.Config{
			ClientID: clientID, Tenant: tenant, Logger: logger,
		}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	chatAPI := msteams.NewHTTPClient("", connection, nil, logger)
	chats, listErr := chatAPI.ListChats(authorizationContext)
	if listErr != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("msteams proof serve: list chats: %w", listErr)
	}
	fmt.Fprintf(output, "READ: chat_count=%d\n", len(chats))
	logger.Info("[msteams-proof-serve] chat list read ok", "chat_count", len(chats), "tenant", tenant)

	service, err := capabilityruntime.NewMSTeams(capabilityruntime.MSTeamsConfig{
		API: chatAPI, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[msteams-proof-serve] ephemeral capability flow ready",
		"token_storage", "memory_only", "adapter_count", 1, "user_oauth", true,
		"tenant", tenant, "listen", listenAddress, "send", "preview_gated")
	return service, connection, nil
}
