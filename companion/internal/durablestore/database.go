package durablestore

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	_ "modernc.org/sqlite"
)

var ErrUnsafeStatePath = errors.New("companion state database path is unsafe")

type Store struct {
	db     *sql.DB
	limits eventjournal.Limits
}

func Open(ctx context.Context, path string, limits eventjournal.Limits) (*Store, error) {
	if ctx == nil || limits.MaxEvents < 1 || limits.MaxBytes < 1 {
		return nil, eventjournal.ErrInvalidEvent
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, ErrUnsafeStatePath
	}
	if info, statErr := os.Lstat(absolute); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafeStatePath
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, ErrUnsafeStatePath
	}
	parent := filepath.Dir(absolute)
	if info, statErr := os.Lstat(parent); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return nil, ErrUnsafeStatePath
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, ErrUnsafeStatePath
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, ErrUnsafeStatePath
	}
	if info, err := os.Lstat(parent); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrUnsafeStatePath
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return nil, ErrUnsafeStatePath
	}
	databaseURL := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := databaseURL.Query()
	query.Set("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	databaseURL.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", databaseURL.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, limits: limits}
	if err := store.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(absolute, 0o600); err != nil {
		_ = db.Close()
		return nil, ErrUnsafeStatePath
	}
	return store, nil
}

func (store *Store) initialize(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS paired_devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    pairing_generation TEXT NOT NULL,
    current_public_key BLOB NOT NULL,
    pending_public_key BLOB NOT NULL,
    paired_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pairing_offers (
    secret_hash BLOB PRIMARY KEY,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    protocol INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    state TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS prompt_entries (
    action_id TEXT PRIMARY KEY,
    queue_key TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    prompt TEXT NOT NULL,
    model TEXT NOT NULL,
    effort TEXT NOT NULL,
    permission_mode TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    state TEXT NOT NULL,
    result_code TEXT NOT NULL,
    result_thread_id TEXT NOT NULL,
    result_turn_id TEXT NOT NULL,
    error_code TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS prompt_entries_queue_order
    ON prompt_entries(queue_key, created_at, action_id);
CREATE TABLE IF NOT EXISTS event_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    next_sequence INTEGER NOT NULL
);
INSERT OR IGNORE INTO event_state(singleton, next_sequence) VALUES (1, 0);
CREATE TABLE IF NOT EXISTS journal_events (
    sequence INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    body BLOB NOT NULL,
    created_at TEXT NOT NULL,
    retained_bytes INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS event_acks (
    device_id TEXT PRIMARY KEY,
    sequence INTEGER NOT NULL
);`
	_, err := store.db.ExecContext(ctx, schema)
	return err
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}
