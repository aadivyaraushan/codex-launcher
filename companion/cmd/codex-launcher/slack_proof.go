package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	slackoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/slack"
	slackproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/slack"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

func startSlackProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	clientID := os.Getenv("SLACK_CLIENT_ID")
	clientSecret := os.Getenv("SLACK_CLIENT_SECRET")
	redirectURI := os.Getenv("SLACK_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "https://127.0.0.1:9192/oauth/slack/callback"
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
		Flow: slackoauth.New(slackoauth.Config{
			ClientID: clientID, ClientSecret: clientSecret, Logger: logger,
		}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	api := slackadapter.NewHTTPClient("", connection, nil, logger)
	service, err := capabilityruntime.NewSlack(capabilityruntime.SlackConfig{
		API: api, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[slack-proof-serve] ephemeral capability flow ready", "token_storage", "memory_only", "adapter_count", 1, "user_oauth", true)
	return service, connection, nil
}
