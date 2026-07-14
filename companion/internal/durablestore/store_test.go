package durablestore

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

var storeNow = time.Date(2026, 7, 14, 10, 30, 0, 0, time.UTC)

func TestStorePreservesPairingQueueAndJournalAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := Open(ctx, path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	_, identity, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	device := pairing.DeviceRecord{
		ID: "pixel-9", Name: "Pixel 9", PairingGeneration: "generation-1",
		CurrentPublicKey: []byte("current-key"), PendingPublicKey: []byte("pending-key"), PairedAt: storeNow,
	}
	if err := store.SaveHostIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	entry := promptqueue.Entry{
		ActionID: "action-1", QueueKey: "new:action-1", ActionKind: "start_turn", ProjectID: "main", Prompt: "Build it",
		Model: "gpt-5", Effort: "high", PermissionMode: "workspace-write", State: promptqueue.StatePrepared,
		CreatedAt: storeNow, UpdatedAt: storeNow,
	}
	if err := store.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	entry.State = promptqueue.StateSentUnknown
	entry.UpdatedAt = storeNow.Add(time.Second)
	if err := store.CompareAndSwap(ctx, promptqueue.StatePrepared, entry); err != nil {
		t.Fatal(err)
	}
	firstEvent, err := store.Append(ctx, eventjournal.Event{Name: "task.updated", Body: []byte(`{"id":"task-1"}`), CreatedAt: storeNow})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Acknowledge(ctx, "pixel-9", firstEvent.Sequence); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	gotIdentity, err := reopened.HostIdentity(ctx)
	if err != nil || !gotIdentity.Equal(identity) {
		t.Fatalf("host identity equal = %v, error = %v", gotIdentity.Equal(identity), err)
	}
	gotDevice, err := reopened.Device(ctx, "pixel-9")
	if err != nil || gotDevice.ID != device.ID || string(gotDevice.PendingPublicKey) != string(device.PendingPublicKey) {
		t.Fatalf("device = %#v, error = %v", gotDevice, err)
	}
	gotEntry, err := reopened.NextPending(ctx, "new:action-1")
	if err != nil || gotEntry.State != promptqueue.StateSentUnknown || gotEntry.Prompt != "Build it" || gotEntry.ActionKind != "start_turn" {
		t.Fatalf("pending entry = %#v, error = %v", gotEntry, err)
	}
	events, err := reopened.ReplayAfter(ctx, 0)
	if err != nil || len(events) != 1 || events[0].Sequence != firstEvent.Sequence {
		t.Fatalf("events = %#v, error = %v", events, err)
	}
	acknowledged, err := reopened.Acknowledged(ctx, "pixel-9")
	if err != nil || acknowledged != firstEvent.Sequence {
		t.Fatalf("acknowledged = %d, error = %v", acknowledged, err)
	}
}

func TestStoreKeepsCompareAndSwapAtomicAndMapsMissingRows(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite3"), eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	entry := promptqueue.Entry{
		ActionID: "action-1", QueueKey: "thread-1", ActionKind: "start_turn", OwnerSource: "app_server", ThreadID: "thread-1", ProjectID: "main",
		Prompt: "Continue", State: promptqueue.StatePrepared, CreatedAt: storeNow, UpdatedAt: storeNow,
	}
	if err := store.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Entry(ctx, "action-1")
	if err != nil || stored.OwnerSource != "app_server" {
		t.Fatalf("stored owner source = %q, %v", stored.OwnerSource, err)
	}
	if err := store.Create(ctx, entry); !errors.Is(err, promptqueue.ErrDuplicateAction) {
		t.Fatalf("duplicate error = %v", err)
	}
	entry.State = promptqueue.StateConfirmed
	if err := store.CompareAndSwap(ctx, promptqueue.StateSentUnknown, entry); !errors.Is(err, promptqueue.ErrStateConflict) {
		t.Fatalf("state conflict error = %v", err)
	}
	if _, err := store.Entry(ctx, "missing"); !errors.Is(err, promptqueue.ErrActionNotFound) {
		t.Fatalf("missing prompt error = %v", err)
	}
	if _, err := store.Device(ctx, "missing"); !errors.Is(err, pairing.ErrDeviceNotFound) {
		t.Fatalf("missing device error = %v", err)
	}
}

func TestSQLiteEnumeratesPendingExistingTaskQueuesOnly(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite3"), eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, entry := range []promptqueue.Entry{
		{ActionID: "z", QueueKey: "thread-z", ThreadID: "thread-z", ActionKind: "start_turn", Prompt: "Later", State: promptqueue.StatePrepared, CreatedAt: storeNow, UpdatedAt: storeNow},
		{ActionID: "a", QueueKey: "thread-a", ThreadID: "thread-a", ActionKind: "steer_turn", Prompt: "Maybe", State: promptqueue.StateSentUnknown, CreatedAt: storeNow, UpdatedAt: storeNow},
		{ActionID: "new", QueueKey: "new:new", ProjectID: "main", ActionKind: "start_turn", Prompt: "New", State: promptqueue.StatePrepared, CreatedAt: storeNow, UpdatedAt: storeNow},
	} {
		if err := store.Create(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.PendingThreadIDs(ctx)
	if err != nil || !slices.Equal(ids, []string{"thread-a", "thread-z"}) {
		t.Fatalf("pending thread IDs = %#v, %v", ids, err)
	}
}

func TestOpenAddsActionKindToAnExistingPromptDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE prompt_entries (
action_id TEXT PRIMARY KEY, queue_key TEXT NOT NULL, thread_id TEXT NOT NULL, project_id TEXT NOT NULL, prompt TEXT NOT NULL,
model TEXT NOT NULL, effort TEXT NOT NULL, permission_mode TEXT NOT NULL, request_hash TEXT NOT NULL, state TEXT NOT NULL,
result_code TEXT NOT NULL, result_thread_id TEXT NOT NULL, result_turn_id TEXT NOT NULL, error_code TEXT NOT NULL,
created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	queue := promptqueue.New(store, nil)
	if err := queue.Enqueue(context.Background(), promptqueue.Entry{
		ActionID: "stop-legacy", QueueKey: "action:stop-legacy", ActionKind: "interrupt_turn", ThreadID: "thread-1", CreatedAt: storeNow,
	}); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Entry(context.Background(), "stop-legacy")
	if err != nil || entry.ActionKind != "interrupt_turn" {
		t.Fatalf("migrated entry = %#v, %v", entry, err)
	}
}

func TestPromptStoreMigratesAndRoundTripsAttachmentOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	legacy, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE prompt_entries (
action_id TEXT PRIMARY KEY, queue_key TEXT NOT NULL, action_kind TEXT NOT NULL DEFAULT 'start_turn', owner_source TEXT NOT NULL DEFAULT '',
thread_id TEXT NOT NULL, project_id TEXT NOT NULL, prompt TEXT NOT NULL, model TEXT NOT NULL, effort TEXT NOT NULL,
permission_mode TEXT NOT NULL, request_hash TEXT NOT NULL, state TEXT NOT NULL, result_code TEXT NOT NULL,
result_thread_id TEXT NOT NULL, result_turn_id TEXT NOT NULL, error_code TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	want := promptqueue.Entry{
		ActionID: "with-files", QueueKey: "thread-1", ThreadID: "thread-1", Prompt: "inspect", DeviceID: "phone-1",
		AttachmentIDs: []string{"upload-1", "upload-2"}, State: promptqueue.StatePrepared, CreatedAt: storeNow, UpdatedAt: storeNow,
	}
	if err := store.Create(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Entry(context.Background(), want.ActionID)
	if err != nil || got.DeviceID != want.DeviceID || !slices.Equal(got.AttachmentIDs, want.AttachmentIDs) {
		t.Fatalf("round-tripped attachment ownership = %#v, %v", got, err)
	}
}

func TestStoreCompactsEventsWithoutReusingSequenceNumbers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := Open(ctx, path, eventjournal.Limits{MaxEvents: 2, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"task.one", "task.two", "task.three"} {
		if _, err := store.Append(ctx, eventjournal.Event{Name: name, Body: []byte(`{"ok":true}`), CreatedAt: storeNow.Add(time.Duration(index) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path, eventjournal.Limits{MaxEvents: 2, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	bounds, err := reopened.Bounds(ctx)
	if err != nil || bounds != (eventjournal.Bounds{Earliest: 2, Latest: 3, Count: 2}) {
		t.Fatalf("bounds = %#v, error = %v", bounds, err)
	}
	next, err := reopened.Append(ctx, eventjournal.Event{Name: "task.four", Body: []byte(`{"ok":true}`), CreatedAt: storeNow.Add(4 * time.Second)})
	if err != nil || next.Sequence != 4 {
		t.Fatalf("next event = %#v, error = %v", next, err)
	}
}

func TestOpenRejectsSymlinkedStateFolderAndUsesOwnerOnlyModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL inheritance has a separate planned native test")
	}
	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(root, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(context.Background(), filepath.Join(linkedParent, "state.sqlite3"), eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096}); !errors.Is(err, ErrUnsafeStatePath) {
		if store != nil {
			_ = store.Close()
		}
		t.Fatalf("symlinked state folder error = %v", err)
	}

	secureParent := filepath.Join(root, "secure")
	statePath := filepath.Join(secureParent, "state.sqlite3")
	store, err := Open(context.Background(), statePath, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	parentInfo, err := os.Stat(secureParent)
	if err != nil {
		t.Fatal(err)
	}
	stateInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if parentInfo.Mode().Perm() != 0o700 || stateInfo.Mode().Perm() != 0o600 {
		t.Fatalf("state modes = folder %o, file %o", parentInfo.Mode().Perm(), stateInfo.Mode().Perm())
	}
}
