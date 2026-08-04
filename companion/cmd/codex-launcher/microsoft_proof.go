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
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	msoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/microsoft"
	msproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/microsoft"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// Callers: main.go liveDependencies.startMicrosoftProof / serve-microsoft-proof.
// User: "Prefer mirroring Todoist/Slack proving shape: serve-microsoft-proof"
// Env schemas (never logged): MICROSOFT_CLIENT_ID, MICROSOFT_REDIRECT_URI,
// MICROSOFT_TENANT. The public-client PKCE flow does not use a client secret.

func startMicrosoftProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("MICROSOFT_CLIENT_ID")
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
		Verbs:         []manifest.Verb{manifest.Read, manifest.Write, manifest.Send},
		Flow: msoauth.New(msoauth.Config{
			ClientID: clientID, Tenant: tenant, Logger: logger,
		}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	tokens := connection.Snapshot()
	mailAPI := outlook.NewHTTPClient("", connection, nil, logger)
	identity, err := mailAPI.Identity(ctx)
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("microsoft proof serve: verify authenticated account: %w", err)
	}
	expectedAccount := strings.TrimSpace(os.Getenv("MICROSOFT_ACCOUNT"))
	if err := validateMicrosoftAccount(identity, expectedAccount); err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	account := expectedAccount
	if account == "" {
		account = strings.TrimSpace(identity.Mail)
	}
	if account == "" {
		account = strings.TrimSpace(identity.UserPrincipalName)
	}
	if err := oauthcredential.Save(ctx, credentialstore.NewKeychain(logger), "microsoft_oauth", oauthcredential.Record{
		Provider: "microsoft", Account: account, AccessToken: tokens.AccessToken,
		RefreshToken: tokens.RefreshToken, TokenType: tokens.TokenType, Scopes: tokens.Scopes, ExpiresAt: tokens.ExpiresAt,
		Metadata: map[string]string{"tenant": tenant, "account_id": identity.ID},
	}); err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("microsoft proof serve: persist OAuth: %w", err)
	}
	service, err := capabilityruntime.NewMicrosoft(capabilityruntime.MicrosoftConfig{
		API: mailAPI, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[microsoft-proof-serve] capability flow ready",
		"token_storage", "macos_keychain", "adapter_count", 1, "user_oauth", true, "tenant", tenant)
	return service, connection, nil
}

func validateMicrosoftAccount(identity outlook.AccountIdentity, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(identity.Mail), expected) || strings.EqualFold(strings.TrimSpace(identity.UserPrincipalName), expected) {
		return nil
	}
	return fmt.Errorf("microsoft proof serve: authenticated account does not match selected account %q", expected)
}
