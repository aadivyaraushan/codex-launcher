package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

type fakeTodoistAPI struct{ created []todoistadapter.CreateTask }

func (f *fakeTodoistAPI) ListTasks(context.Context) ([]todoistadapter.Task, error) { return nil, nil }
func (f *fakeTodoistAPI) CreateTask(_ context.Context, request todoistadapter.CreateTask) (todoistadapter.Task, error) {
	f.created = append(f.created, request)
	return todoistadapter.Task{ID: "task-1", Content: request.Content}, nil
}
func (f *fakeTodoistAPI) Clear(context.Context) error { return nil }

func TestTodoistFlowComposesTheProductionRouterAndAdapter(t *testing.T) {
	api := &fakeTodoistAPI{}
	service, err := NewTodoist(TodoistConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"write","app_class":"tasks","app_named":"Todoist","subject":"Buy oat milk","body":"Tomorrow","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewTodoist: %v", err)
	}

	preview, err := service.Prepare(context.Background(), "pixel/session/1", "request-1", "Add milk to Todoist")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != todoistadapter.ID || preview.Verb != manifest.Write || preview.Fingerprint == "" {
		t.Fatalf("preview=%+v", preview)
	}
	outcome, err := service.Confirm(context.Background(), "pixel/session/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !outcome.Done || len(api.created) != 1 {
		t.Fatalf("outcome=%+v created=%d", outcome, len(api.created))
	}
}

func TestTodoistFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewTodoist(TodoistConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}

var _ capabilityadapter.Adapter = (*todoistadapter.Adapter)(nil)
