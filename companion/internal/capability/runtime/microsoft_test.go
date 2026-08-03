package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/outlook"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Callers: go test ./companion/internal/capability/runtime/ -run Microsoft
// User: "Tests red→green." Synthetic Stage-1 JSON only; no data files.

type fakeOutlookAPI struct {
	messages []outlook.Message
	drafts   []outlook.CreateDraft
	sent     []outlook.SendMail
}

func (f *fakeOutlookAPI) ListMessages(context.Context, string) ([]outlook.Message, error) {
	return f.messages, nil
}
func (f *fakeOutlookAPI) CreateDraft(_ context.Context, request outlook.CreateDraft) (outlook.Message, error) {
	f.drafts = append(f.drafts, request)
	return outlook.Message{ID: "draft-1", Subject: request.Subject}, nil
}
func (f *fakeOutlookAPI) SendMail(_ context.Context, request outlook.SendMail) error {
	f.sent = append(f.sent, request)
	return nil
}
func (f *fakeOutlookAPI) Clear(context.Context) error { return nil }

func TestMicrosoftFlowComposesTheProductionRouterAndAdapter(t *testing.T) {
	api := &fakeOutlookAPI{messages: []outlook.Message{{ID: "m1", Subject: "Wave1 invoice"}}}
	service, err := NewMicrosoft(MicrosoftConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"email","app_named":"Outlook","subject":"Wave1 invoice","body":"","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewMicrosoft: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Read Wave1 invoice in Outlook")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != outlook.ID || preview.Verb != manifest.Read || preview.Fingerprint == "" {
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

func TestMicrosoftFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewMicrosoft(MicrosoftConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}
