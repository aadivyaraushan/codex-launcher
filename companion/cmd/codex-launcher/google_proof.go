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

	"github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore"
	oauthcredential "github.com/codex-launcher/codex-launcher/companion/internal/app/credentialstore/oauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gcalendar"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/gdrive"
	googleoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/google"
	googleproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/google"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// Callers: main.go liveDependencies.startGoogleProof / serve-google-proof.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-google-proof"

func startGoogleProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURI := os.Getenv("GOOGLE_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "http://127.0.0.1:9194/oauth/google/callback"
	}
	listenAddress := "127.0.0.1:9194"
	if parsed, err := url.Parse(redirectURI); err == nil && parsed.Host != "" {
		listenAddress = parsed.Host
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("google proof serve: stage 1 router: %w", err)
	}
	authorizationContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	connection, err := googleproof.Authorize(authorizationContext, googleproof.AuthorizationConfig{
		ListenAddress: listenAddress,
		RedirectURI:   redirectURI,
		Flow: googleoauth.New(googleoauth.Config{
			ClientID: clientID, ClientSecret: clientSecret, Logger: logger,
		}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	tokens := connection.Snapshot()
	record := oauthcredential.Record{
		Provider: "google", AccessToken: tokens.AccessToken,
		RefreshToken: tokens.RefreshToken, TokenType: tokens.TokenType, Scopes: tokens.Scopes, ExpiresAt: tokens.ExpiresAt,
		Metadata: map[string]string{"requested_account": strings.TrimSpace(os.Getenv("GOOGLE_ACCOUNT"))},
	}
	store := credentialstore.NewKeychain(logger)
	for _, name := range []string{"google_calendar_oauth", "google_drive_oauth"} {
		if err := oauthcredential.Save(ctx, store, name, record); err != nil {
			_ = connection.Close()
			return nil, nil, fmt.Errorf("google proof serve: persist OAuth: %w", err)
		}
	}
	if err := store.Delete(ctx, "google_oauth"); err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("google proof serve: remove legacy OAuth record: %w", err)
	}
	calendarAPI := gcalendar.NewHTTPClient("", connection, nil, logger)
	driveAPI := gdrive.NewHTTPClient("", connection, nil, logger)
	service, err := capabilityruntime.NewGoogle(capabilityruntime.GoogleConfig{
		CalendarAPI: calendarAPI, DriveAPI: driveAPI, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[google-proof-serve] capability flow ready",
		"token_storage", "macos_keychain", "adapter_count", 2, "user_oauth", true)
	return service, connection, nil
}
