package runtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	notionadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
)

type connectedNotionSession struct{ listCalls int }

func (s *connectedNotionSession) ListTools(context.Context) ([]string, error) {
	s.listCalls++
	return []string{notionadapter.ToolSearch, notionadapter.ToolFetch, notionadapter.ToolCreatePages, notionadapter.ToolUpdatePage}, nil
}

func (*connectedNotionSession) Call(context.Context, string, map[string]any) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

func TestNotionFlowConnectsAuthenticatedSessionBeforeRouting(t *testing.T) {
	session := &connectedNotionSession{}
	service, err := NewNotion(t.Context(), NotionConfig{
		Session: session,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"notes","app_named":"Notion","subject":"Operator","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewNotion: %v", err)
	}
	if service == nil || session.listCalls != 1 {
		t.Fatalf("service=%v listCalls=%d", service, session.listCalls)
	}
}

func TestNotionFlowRejectsMissingDependencies(t *testing.T) {
	if _, err := NewNotion(t.Context(), NotionConfig{}); err == nil {
		t.Fatal("empty Notion runtime config was accepted")
	}
}
