package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

func TestRuntimePumpsProjectedCodexEventIntoJournalSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan taskstate.MobileEvent)
	store := &signalingEventStore{
		MemoryStore: eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 16, MaxBytes: 64 * 1024}),
		appended:    make(chan eventjournal.Event, 1),
	}
	config := Config{
		Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443,
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}},
	}
	runtime, err := NewRuntime(ctx, config, Dependencies{
		PairingStore: pairing.NewMemoryStore(), PromptStore: promptqueue.NewMemoryStore(), EventStore: store, Random: rand.Reader,
		TaskSource: integrationTaskSource{}, TaskEvents: events,
	})
	if err != nil {
		t.Fatal(err)
	}
	events <- taskstate.MobileEvent{TaskID: "thread-1", Kind: "reply", State: taskstate.IdleAfterReply, Summary: "Codex replied"}
	appended := <-store.appended
	if appended.Name != "event" {
		t.Fatalf("appended event = %#v", appended)
	}
	snapshot, err := runtime.Journal.Snapshot(appNow)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Tasks []struct {
			TaskID, Title, State string
		} `json:"tasks"`
	}
	if err := json.Unmarshal(snapshot.Body, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].TaskID != "thread-1" || state.Tasks[0].Title != "Build launcher" || state.Tasks[0].State != "idle_after_reply" {
		t.Fatalf("snapshot tasks = %#v", state.Tasks)
	}
}

type integrationTaskSource struct{}

func (integrationTaskSource) ListRecent(context.Context, int) ([]taskstate.Task, error) {
	return []taskstate.Task{{
		ID: "thread-1", Title: "Build launcher", ProjectLabel: "uf-u", State: taskstate.Working,
		UpdatedAtUnix: appNow.Unix(), Source: taskstate.SourceAppServer,
	}}, nil
}

type signalingEventStore struct {
	*eventjournal.MemoryStore
	appended chan eventjournal.Event
}

func (store *signalingEventStore) Append(ctx context.Context, event eventjournal.Event) (eventjournal.Event, error) {
	appended, err := store.MemoryStore.Append(ctx, event)
	if err == nil {
		store.appended <- appended
	}
	return appended, err
}
