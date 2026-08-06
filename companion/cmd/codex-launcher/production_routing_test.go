package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	stage1explicit "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/explicit"
)

func TestProductionRoutingKeepsExplicitAppRequestsWorkingWithoutAnOpenAIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("BEEPER_ACCESS_TOKEN", "")
	t.Setenv("BEEPER_READONLY", "")
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

// B3: when Beeper owns messages/discord under beeper_messaging, the Mac keyword
// fallback must not still advertise those ids under class messaging — that is
// how "…Instagram messages" wrongly matched Google Messages (diagnosis 2026-08-06).
func TestProductionExplicitRulesOmitBeeperOwnedMessagingWhenBeeperOn(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("BEEPER_ACCESS_TOKEN", "beeper-token")
	t.Setenv("BEEPER_READONLY", "")

	model, source, err := productionRoutingModel(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if source != "explicit_app" {
		t.Fatalf("source = %q", source)
	}

	_, err = model(context.Background(), "what's my most recent unread Instagram messages")
	if !errors.Is(err, stage1explicit.ErrNoExplicitApp) {
		t.Fatalf("Instagram-messages ask with Beeper on: err=%v, want ErrNoExplicitApp (messages must not be advertised under messaging)", err)
	}

	_, err = model(context.Background(), "Draft a Discord message to Maya that I'm running late")
	if !errors.Is(err, stage1explicit.ErrNoExplicitApp) {
		t.Fatalf("Discord draft with Beeper on: err=%v, want ErrNoExplicitApp", err)
	}

	// Unrelated Wave1 apps stay available.
	raw, err := model(context.Background(), "Play Never Gonna Give You Up official video YouTube")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.AppNamed != "youtube" {
		t.Fatalf("youtube route broken when filtering Beeper ids: %+v", route)
	}
}

func TestProductionExplicitRulesKeepMessagesDiscordWhenBeeperOff(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("BEEPER_ACCESS_TOKEN", "")
	t.Setenv("BEEPER_READONLY", "")

	model, _, err := productionRoutingModel(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := model(context.Background(), "Draft a Discord message to Maya that I'm running late")
	if err != nil {
		t.Fatal(err)
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if route.AppNamed != "discord" || route.AppClass != "messaging" || route.Verb != manifest.Compose {
		t.Fatalf("without Beeper, discord should stay under messaging compose: %+v", route)
	}
}

// Fact-force (edit): callers=go test ./cmd/codex-launcher; schemas=none;
// user: "Continue OpenAI+Beeper — **SLICE 4: B4 + B5**."
func TestProductionExplicitRulesOmitBeeperOwnedWhenBeeperReadOnlyStillRegisters(t *testing.T) {
	// B5: BEEPER_READONLY=1 still registers Beeper with Verbs:[read], so Wave1
	// messages/discord must stay omitted under class messaging.
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("BEEPER_ACCESS_TOKEN", "beeper-token")
	t.Setenv("BEEPER_READONLY", "1")

	model, _, err := productionRoutingModel(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = model(context.Background(), "Draft a Discord message to Maya that I'm running late")
	if !errors.Is(err, stage1explicit.ErrNoExplicitApp) {
		t.Fatalf("readonly Beeper still registered: want ErrNoExplicitApp for messaging/discord, got err=%v", err)
	}
}
