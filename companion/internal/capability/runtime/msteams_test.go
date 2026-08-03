package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/msteams"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Callers: go test ./companion/internal/capability/runtime/ -run MSTeams
// User: "Look at how capabilityruntime.NewMicrosoft works — only add NewMSTeams if cheap."
// Synthetic Stage-1 JSON only; no data files.

type fakeMSTeamsAPI struct {
	chats []msteams.Chat
	sent  []msteams.SendMessage
}

func (f *fakeMSTeamsAPI) ListChats(context.Context) ([]msteams.Chat, error) {
	return f.chats, nil
}
func (f *fakeMSTeamsAPI) SendMessage(_ context.Context, request msteams.SendMessage) (msteams.SentMessage, error) {
	f.sent = append(f.sent, request)
	return msteams.SentMessage{ID: "msg-1", ChatID: request.ChatID}, nil
}
func (f *fakeMSTeamsAPI) Clear(context.Context) error { return nil }

func TestMSTeamsFlowComposesTheProductionRouterAndAdapter(t *testing.T) {
	api := &fakeMSTeamsAPI{chats: []msteams.Chat{{ID: "chat-1", Topic: "Wave1 standup"}}}
	service, err := NewMSTeams(MSTeamsConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"messaging","app_named":"msteams","subject":"Wave1 standup","body":"","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewMSTeams: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Read Wave1 standup in Teams")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != msteams.ID || preview.Verb != manifest.Read || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestMSTeamsFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewMSTeams(MSTeamsConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}
