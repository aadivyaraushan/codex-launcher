package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"

	todoistadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/todoist"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Plan Wave 1 step 4: every registered in-scope proving adapter must be
// reachable by at least one ordinary prompt through the production flow.
func TestInScopeTodoistPromptReachesRegisteredAdapter(t *testing.T) {
	api := &fakeTodoistAPI{}
	service, err := NewTodoist(TodoistConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"write","app_class":"tasks","app_named":"Todoist","subject":"create a test task in Todoist","body":"","confidence":0.99}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewTodoist: %v", err)
	}
	preview, err := service.Prepare(context.Background(), "s", "r", "create a test task in Todoist")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if preview.AdapterID != todoistadapter.ID {
		t.Fatalf("adapter=%q want %q", preview.AdapterID, todoistadapter.ID)
	}
	if preview.Verb != manifest.Write {
		t.Fatalf("verb=%q", preview.Verb)
	}
}
