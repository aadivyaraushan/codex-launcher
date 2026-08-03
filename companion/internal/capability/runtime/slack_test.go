package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	slackadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/slack"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

type fakeSlackAPI struct {
	channels []slackadapter.Channel
	posted   []slackadapter.PostMessage
}

func (f *fakeSlackAPI) ListChannels(context.Context) ([]slackadapter.Channel, error) {
	return f.channels, nil
}
func (f *fakeSlackAPI) PostMessage(_ context.Context, request slackadapter.PostMessage) (slackadapter.PostedMessage, error) {
	f.posted = append(f.posted, request)
	return slackadapter.PostedMessage{Channel: request.Channel, Timestamp: "1.2"}, nil
}
func (f *fakeSlackAPI) Clear(context.Context) error { return nil }

func TestSlackFlowComposesTheProductionRouterAndAdapter(t *testing.T) {
	api := &fakeSlackAPI{channels: []slackadapter.Channel{{ID: "C9", Name: "wave1"}}}
	service, err := NewSlack(SlackConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"send","app_class":"messaging","app_named":"Slack","subject":"wave1","body":"hello from Operator","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewSlack: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Post hello to #wave1 on Slack")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != slackadapter.ID || preview.Verb != manifest.Send || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done || len(api.posted) != 1 {
		t.Fatalf("outcome=%+v posted=%d", outcome, len(api.posted))
	}
}

func TestSlackFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewSlack(SlackConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}
