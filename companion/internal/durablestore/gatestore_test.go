package durablestore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
)

func TestGateStateSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := Open(ctx, path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.MarkRecipientMessaged(ctx, "instagram", "maya"); err != nil {
		t.Fatalf("MarkRecipientMessaged: %v", err)
	}
	// Marking twice must not error — every successful send re-marks.
	if err := store.MarkRecipientMessaged(ctx, "instagram", "maya"); err != nil {
		t.Fatalf("MarkRecipientMessaged again: %v", err)
	}
	if err := store.RecordGateDenial(ctx, "gate-1"); err != nil {
		t.Fatalf("RecordGateDenial: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, path, eventjournal.Limits{MaxEvents: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	known, err := reopened.KnownRecipient(ctx, "instagram", "maya")
	if err != nil {
		t.Fatalf("KnownRecipient: %v", err)
	}
	if !known {
		t.Fatal("a messaged recipient must still be known after reopen")
	}
	stranger, err := reopened.KnownRecipient(ctx, "instagram", "someone-new")
	if err != nil {
		t.Fatalf("KnownRecipient: %v", err)
	}
	if stranger {
		t.Fatal("a never-messaged recipient must not be known")
	}
	otherAdapter, err := reopened.KnownRecipient(ctx, "discord", "maya")
	if err != nil {
		t.Fatalf("KnownRecipient: %v", err)
	}
	if otherAdapter {
		t.Fatal("known-ness is per adapter; the same name on another adapter is a stranger")
	}

	denied, err := reopened.GateDenied(ctx, "gate-1")
	if err != nil {
		t.Fatalf("GateDenied: %v", err)
	}
	if !denied {
		t.Fatal("a recorded denial must survive reopen")
	}
	fresh, err := reopened.GateDenied(ctx, "gate-2")
	if err != nil {
		t.Fatalf("GateDenied: %v", err)
	}
	if fresh {
		t.Fatal("an id that was never denied must not read as denied")
	}
}
