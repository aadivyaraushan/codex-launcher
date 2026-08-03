package instagram

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	instagramadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/instagram"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func TestInstagramFlowComposesTheProductionRouterAndAdapter(t *testing.T) {
	service, err := New(Config{
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"compose","app_class":"messaging","app_named":"instagram","subject":"Maya","body":"Running ten minutes late","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Draft an Instagram message to Maya")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != instagramadapter.ID || preview.Verb != manifest.Compose || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	shown := strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Running ten minutes late") {
		t.Fatalf("preview lines missing draft: %v", preview.Lines)
	}

	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if outcome.Reached != manifest.HandsOff || !outcome.Done || outcome.HandedOffTo != "Instagram" {
		t.Fatalf("outcome=%+v", outcome)
	}
	if strings.Contains(strings.ToLower(outcome.Detail), "sent") {
		t.Fatalf("outcome claims send: %q", outcome.Detail)
	}
}

func TestInstagramFlowRejectsMissingModel(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}
