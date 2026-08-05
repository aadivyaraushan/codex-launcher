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
	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	slackoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/slack"
	slackproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/slack"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

// Callers: main serve-slack-proof. Default http loopback avoids self-signed TLS browser blocks. No schema.
// User: "Change Slack OAuth redirect to http://127.0.0.1:PORT/oauth/slack/callback (or http://localhost) so no cert warning"
func startSlackProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("SLACK_CLIENT_ID")
	clientSecret := os.Getenv("SLACK_CLIENT_SECRET")
	redirectURI := os.Getenv("SLACK_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "http://127.0.0.1:9192/oauth/slack/callback"
	}
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("slack proof serve: stage 1 router: %w", err)
	}
	authorizationContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	connection, err := slackproof.Authorize(authorizationContext, slackproof.AuthorizationConfig{
		ListenAddress: "127.0.0.1:9192",
		RedirectURI:   redirectURI,
		Verbs:         []manifest.Verb{manifest.Read, manifest.Send},
		Flow: slackoauth.New(slackoauth.Config{
			ClientID: clientID, ClientSecret: clientSecret, Logger: logger,
		}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	tokens := connection.Snapshot()
	api := slackadapter.NewHTTPClient("", connection, nil, logger)
	identity, err := api.Identity(ctx)
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("slack proof serve: verify authenticated workspace: %w", err)
	}
	if err := validateSlackWorkspace(identity, os.Getenv("SLACK_WORKSPACE_HOST")); err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	if err := oauthcredential.Save(ctx, credentialstore.NewKeychain(logger), "slack_oauth", oauthcredential.Record{
		Provider: "slack", Account: identity.Team, AccessToken: tokens.AccessToken,
		TokenType: tokens.TokenType, Scopes: tokens.Scopes, ExpiresAt: tokens.ExpiresAt,
		Metadata: map[string]string{"team_id": identity.TeamID, "user_id": identity.UserID, "workspace_url": identity.URL},
	}); err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("slack proof serve: persist OAuth: %w", err)
	}
	service, err := capabilityruntime.NewSlack(capabilityruntime.SlackConfig{
		API: api, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[slack-proof-serve] capability flow ready", "token_storage", "macos_keychain", "adapter_count", 1, "user_oauth", true)
	return service, connection, nil
}

func validateSlackWorkspace(identity slackadapter.WorkspaceIdentity, expectedHost string) error {
	expectedHost = strings.TrimSpace(strings.ToLower(expectedHost))
	if expectedHost == "" {
		return nil
	}
	parsed, err := url.Parse(identity.URL)
	if err != nil || parsed.Hostname() == "" {
		return fmt.Errorf("slack proof serve: authenticated workspace returned an invalid URL")
	}
	if !strings.EqualFold(parsed.Hostname(), expectedHost) {
		return fmt.Errorf("slack proof serve: authenticated workspace %q does not match selected workspace %q", parsed.Hostname(), expectedHost)
	}
	return nil
}
