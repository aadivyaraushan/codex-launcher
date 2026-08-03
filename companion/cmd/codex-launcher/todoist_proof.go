package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	todoistoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/todoist"
	todoistproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/todoist"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
	capabilityruntime "github.com/codex-launcher/codex-launcher/companion/internal/capability/runtime"
)

func startTodoistProof(ctx context.Context, output io.Writer) (mobilesession.CapabilityFlow, io.Closer, error) {
	logger := slog.Default()
	router, err := stage1openai.New(stage1openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Logger: logger})
	if err != nil {
		return nil, nil, fmt.Errorf("todoist proof serve: stage 1 router: %w", err)
	}
	authorizationContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	connection, err := todoistproof.Authorize(authorizationContext, todoistproof.AuthorizationConfig{
		Flow:   todoistoauth.New(todoistoauth.Config{ClientName: "Operator", Logger: logger}),
		Output: output,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	api := todoistadapter.NewHTTPClient("", connection, nil, logger)
	service, err := capabilityruntime.NewTodoist(capabilityruntime.TodoistConfig{
		API: api, Model: router.Model, Logger: logger,
	})
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	logger.Info("[todoist-proof-serve] ephemeral capability flow ready", "token_storage", "memory_only", "adapter_count", 1)
	return service, connection, nil
}
