package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

func TestProductionRoutingKeepsExplicitAppRequestsWorkingWithoutAnOpenAIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	model, source, err := productionRoutingModel(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if source != "explicit_app" {
		t.Fatalf("source = %q", source)
	}

	raw, err := model(context.Background(), "Play Never Gonna Give You Up official video YouTube")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.AppNamed != "youtube" || route.Verb != manifest.Play || route.Subject != "Never Gonna Give You Up official video" {
		t.Fatalf("route = %+v", route)
	}
}
