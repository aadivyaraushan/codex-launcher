// Package threads is the phone-side thread store for capability requests. A
// capability request today lives and dies inside one dialog: the preview
// appears, the result flashes, and nothing survives dismissal. This package
// turns every capability request into a persistent thread task, keyed by the
// actionId the phone already generated, and serves it back through the same
// TaskSource/TaskTranscriptSource/ExistingTaskSource method sets the desktop
// task list uses (mobilesession/handler.go:39-75). It defines its own Flow
// interface rather than importing mobilesession, and satisfies the handler's
// interfaces structurally.
package threads

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	_ "modernc.org/sqlite"
)

// SourcePhoneCapability marks a task as a phone-local capability thread. It
// is not one of taskstate's three wire-known sources (desktop, app_server,
// catalog_candidate) because none of them fit: this task never touches a
// coding-agent turn engine. Source is not on the wire (absent from the
// snapshot task validator, contract/validation.go:824) and the optional
// recovery interfaces that switch on it (handler.go:77-92) are not
// implemented here, so an unrecognized value is safe.
const SourcePhoneCapability taskstate.Source = "phone_capability"

// ErrUnknownThread means the caller asked about a thread id this store has
// never seen. Callers must fail instead of inventing a task for it.
var ErrUnknownThread = errors.New("threads: unknown thread")

// Store is the sqlite-backed thread store. All access goes through a single
// connection (like durablestore.Store), so sqlite serializes writes and no
// extra locking is needed around the database itself; previewSink and flow
// are set once at startup but read from goroutines handling live requests,
// so those two are guarded by mu.
type Store struct {
	db     *sql.DB
	now    func() time.Time
	logger *slog.Logger

	mu          sync.Mutex
	flow        Flow
	previewSink func(threadID string, preview capabilityflow.Preview)
}

// Open creates or reopens the thread store at path. now is injected so tests
// control time; logger may be nil.
func Open(path string, now func() time.Time, logger *slog.Logger) (*Store, error) {
	if now == nil {
		return nil, errors.New("threads: now is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+absolute+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, now: now, logger: logger}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) initialize(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS threads (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    title TEXT NOT NULL,
    updated_at_unix INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS thread_entries (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    entry_id TEXT NOT NULL UNIQUE,
    thread_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    text TEXT NOT NULL,
    sender TEXT NOT NULL DEFAULT '',
    sent_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS thread_entries_thread_seq ON thread_entries(thread_id, seq);
CREATE TABLE IF NOT EXISTS turn_threads (
    turn_id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL
);`
	_, err := store.db.ExecContext(ctx, schema)
	return err
}

// Close releases the underlying database connection.
func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

// ListRecent implements mobilesession.TaskSource: the phone's task list, most
// recently active thread first.
func (store *Store) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := store.db.QueryContext(ctx,
		`SELECT id, title, updated_at_unix FROM threads ORDER BY updated_at_unix DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]taskstate.Task, 0, limit)
	for rows.Next() {
		var id, title string
		var updatedAt int64
		if err := rows.Scan(&id, &title, &updatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, taskFromRow(id, title, updatedAt))
	}
	return tasks, rows.Err()
}

// CurrentTask implements mobilesession.ExistingTaskSource: startExistingTask
// preflights with this before routing a follow-up (handler.go:1391-1401), so
// an unknown thread must be a real error, not a fabricated task.
func (store *Store) CurrentTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	var title string
	var updatedAt int64
	err := store.db.QueryRowContext(ctx, `SELECT title, updated_at_unix FROM threads WHERE id = ?`, taskID).
		Scan(&title, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return taskstate.Task{}, fmt.Errorf("%w: %s", ErrUnknownThread, taskID)
	}
	if err != nil {
		return taskstate.Task{}, err
	}
	return taskFromRow(taskID, title, updatedAt), nil
}

// taskFromRow builds the wire-facing Task for a thread row. A capability
// thread always reports idle_after_reply and no redirectable turn: the
// handler's startExistingTask queues follow-ups behind any busy state
// (handler.go:1416-1418), and a capability thread has no long-running turn
// worth queuing behind (store_test.go:126-134). ProjectLabel has no natural
// analogue for a capability thread, so it carries a fixed, wire-safe label.
func taskFromRow(id, title string, updatedAt int64) taskstate.Task {
	return taskstate.Task{
		ID: id, Title: title, ProjectLabel: "Capability",
		State: taskstate.IdleAfterReply, CanRedirect: false,
		UpdatedAtUnix: updatedAt, Source: SourcePhoneCapability,
	}
}

func (store *Store) threadExists(ctx context.Context, taskID string) (bool, error) {
	var id string
	err := store.db.QueryRowContext(ctx, `SELECT id FROM threads WHERE id = ?`, taskID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// newRandomID mints a wire-safe id: 128 bits of randomness, hex-encoded (the
// same shape as relaybox's token minting, relaybox/tokens.go:41-45), which
// stays inside the wire's identifier alphabet (contract/validation.go:31).
func newRandomID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

// sanitizeLine folds text to a single control-free line and clips it to at
// most maxRunes, mirroring taskstate's own safeDisplay (mapper.go:205-217,
// unexported) since this package cannot import it.
func sanitizeLine(s string, maxRunes int) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsControl(r) {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	runes := []rune(out)
	if len(runes) > maxRunes {
		out = string(runes[:maxRunes])
	}
	return out
}

func deriveTitle(utterance string) string {
	title := sanitizeLine(utterance, 80)
	if title == "" {
		title = "Capability request"
	}
	return title
}
