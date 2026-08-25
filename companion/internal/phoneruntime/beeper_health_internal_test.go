package phoneruntime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

// Workstream B5. Read-only mode used to make Beeper vanish entirely (the phone
// got a nil client and could neither read nor send). That defeats the vision:
// a cautious user who only wants to READ their messages got nothing. Read-only
// mode must now hand back a client that still answers reads and refuses writes.

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBeeperReadOnlyModeYieldsAReadingClientNotNil(t *testing.T) {
	t.Setenv("BEEPER_ACCESS_TOKEN", "tok-x")
	t.Setenv("BEEPER_READONLY", "1")

	api := beeperAPIFromEnv(discardLog())
	if api == nil {
		t.Fatal("read-only mode must still provide a Beeper client for reads, got nil")
	}
	// A write must refuse locally (the read-only guard returns before any
	// network call), proving this really is a read-only client.
	if _, err := api.Send(context.Background(), "c1", "hi"); !errors.Is(err, beeper.ErrReadOnly) {
		t.Fatalf("read-only client must refuse Send with ErrReadOnly, got %v", err)
	}
}

func TestNoTokenStillYieldsNoClient(t *testing.T) {
	t.Setenv("BEEPER_ACCESS_TOKEN", "")
	t.Setenv("BEEPER_READONLY", "1")
	t.Setenv("BEEPER_ACCOUNT_DB", "/nonexistent/account.db")

	if api := beeperAPIFromEnv(discardLog()); api != nil {
		t.Fatal("with no access token there is nothing to read, expected nil")
	}
}
