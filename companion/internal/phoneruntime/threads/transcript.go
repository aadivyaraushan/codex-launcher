package threads

import (
	"context"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
)

// ReadTranscript implements mobilesession.TaskTranscriptSource. It pages
// backwards from the newest entry the same way tasktranscript's own
// mapPage does for desktop/app-server tasks (mapper.go:107-177): the window
// ends just before BeforeEntryID (or at the end of the thread when there is
// no cursor), and EarlierCursor names the earliest entry still in the page
// so the next call can ask for what came before it.
func (store *Store) ReadTranscript(ctx context.Context, taskID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	if options.TaskID != taskID {
		return tasktranscript.Page{}, tasktranscript.ErrTaskMismatch
	}
	exists, err := store.threadExists(ctx, taskID)
	if err != nil {
		return tasktranscript.Page{}, err
	}
	if !exists {
		return tasktranscript.Page{}, tasktranscript.ErrTaskMismatch
	}

	all, err := store.allEntries(ctx, taskID)
	if err != nil {
		return tasktranscript.Page{}, err
	}

	limit := options.Limit
	if limit < 1 {
		limit = 1
	}
	if limit > tasktranscript.MaxPageEntries {
		limit = tasktranscript.MaxPageEntries
	}

	end := len(all)
	if options.BeforeEntryID != "" {
		end = -1
		for index, entry := range all {
			if entry.ID == options.BeforeEntryID {
				end = index
				break
			}
		}
		if end < 0 {
			return tasktranscript.Page{}, tasktranscript.ErrUnknownCursor
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}

	page := tasktranscript.Page{TaskID: taskID, Entries: append([]tasktranscript.Entry(nil), all[start:end]...)}
	if start > 0 && len(page.Entries) != 0 {
		page.EarlierCursor = page.Entries[0].ID
	}
	// The stored entries are unbounded (SQLite has no wire limits), so every
	// page must pass through the same clipping tasktranscript.mapPage applies
	// to desktop/app-server pages before it can be encoded onto the wire.
	return tasktranscript.BoundPageForWire(page), nil
}

func (store *Store) allEntries(ctx context.Context, taskID string) ([]tasktranscript.Entry, error) {
	rows, err := store.db.QueryContext(ctx,
		`SELECT entry_id, turn_id, kind, text, sender, sent_at FROM thread_entries WHERE thread_id = ? ORDER BY seq ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]tasktranscript.Entry, 0)
	for rows.Next() {
		var entry tasktranscript.Entry
		var kind string
		if err := rows.Scan(&entry.ID, &entry.TurnID, &kind, &entry.Text, &entry.Sender, &entry.SentAt); err != nil {
			return nil, err
		}
		entry.Kind = tasktranscript.Kind(kind)
		if entry.Kind != tasktranscript.KindMessage {
			// Sender/SentAt are wire-present only on message rows
			// (contract/validation.go:878-887); every other kind's row
			// carries empty defaults from the schema, so this is a no-op in
			// practice, kept explicit for clarity.
			entry.Sender, entry.SentAt = "", ""
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
