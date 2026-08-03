package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	todoistoauth "github.com/codex-launcher/codex-launcher/companion/internal/capability/oauth/todoist"
	todoistproof "github.com/codex-launcher/codex-launcher/companion/internal/capability/proving/todoist"
)

func runTodoist(args []string) error {
	fs := flag.NewFlagSet("todoist", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:9191", "local address for the Todoist OAuth callback")
	content := fs.String("content", "", "task title shown in the preview; blank creates a unique proof title")
	description := fs.String("description", "Created by the confirmed Wave 1 RT-2 proving run", "task description shown in the preview")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger := slog.Default()
	flow := todoistoauth.New(todoistoauth.Config{ClientName: "Operator", Logger: logger})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return todoistproof.Run(ctx, todoistproof.Config{
		ListenAddress: *listen,
		Flow:          flow,
		NewAPI: func(connection *todoistproof.Connection) todoistadapter.API {
			return todoistadapter.NewHTTPClient("", connection, nil, logger)
		},
		Input: os.Stdin, Output: os.Stdout, Logger: logger,
		Content: *content, Description: *description,
	})
}
