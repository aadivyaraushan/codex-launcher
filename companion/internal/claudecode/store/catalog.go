package store

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
)

const (
	// maxSessionFiles bounds a catalog sweep. A long-lived computer accumulates
	// far more sessions than a phone will ever show, and the sweep runs on every
	// snapshot refresh, so it is capped rather than allowed to grow unbounded.
	maxSessionFiles = 512
	// maxProjectDirectories bounds how many encoded-cwd directories are walked.
	maxProjectDirectories = 256
	// headRecordsForSummary is how far into a file the sweep reads to find the
	// cwd and opening prompt. They are written near the top, so a whole-file
	// read would be wasted work on every refresh.
	headRecordsForSummary = 64
)

// ProjectApprover decides whether a session may be shown to the phone at all.
//
// This is the security boundary of the catalog: the launcher must never
// surface work from a folder the owner has not approved. It takes the cwd
// recorded inside the session file, never the encoded directory name, because
// that encoding collapses '/', '_', and '.' to '-' and cannot be reversed.
type ProjectApprover interface {
	// ApproveCWD returns the project label to display, and whether the cwd is
	// inside an approved project.
	ApproveCWD(cwd string) (string, bool)
}

// ApproverFunc adapts a plain function to ProjectApprover.
type ApproverFunc func(string) (string, bool)

func (approve ApproverFunc) ApproveCWD(cwd string) (string, bool) { return approve(cwd) }

// Summary is what the catalog knows about a session without reading all of it.
type Summary struct {
	SessionID     string
	Title         string
	ProjectLabel  string
	CWD           string
	Path          string
	UpdatedAtUnix int64
}

// Catalog lists Claude Code sessions as tasks.
type Catalog struct {
	root     string
	approver ProjectApprover
	logger   *slog.Logger

	mu     sync.Mutex
	cached map[string]cacheEntry
}

// cacheEntry avoids re-reading a session file whose contents cannot have
// changed. Sessions are append-only, and a catalog sweep only needs the header
// and title, so a file whose size and mtime are unchanged is reused.
type cacheEntry struct {
	modified time.Time
	size     int64
	summary  Summary
	approved bool
}

func NewCatalog(root string, approver ProjectApprover, logger *slog.Logger) (*Catalog, error) {
	if strings.TrimSpace(root) == "" || approver == nil {
		return nil, errors.New("Claude Code catalog dependency is missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Catalog{root: root, approver: approver, logger: logger, cached: make(map[string]cacheEntry)}, nil
}

// ListRecent returns the most recently active approved sessions, newest first.
func (catalog *Catalog) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	summaries, err := catalog.listSummaries(ctx, limit)
	if err != nil {
		return nil, err
	}
	tasks := make([]taskstate.Task, 0, len(summaries))
	for _, summary := range summaries {
		tasks = append(tasks, taskstate.Task{
			ID:            summary.SessionID,
			Title:         summary.Title,
			ProjectLabel:  summary.ProjectLabel,
			State:         taskstate.IdleAfterReply,
			UpdatedAtUnix: summary.UpdatedAtUnix,
			Source:        taskstate.SourceClaudeCode,
			// Claude Code queues a message sent mid-turn rather than merging it
			// into the running turn, so a task can never be redirected.
			CanRedirect: false,
		})
	}
	return tasks, nil
}

// Lookup finds one session by ID, subject to the same approval rule as the
// listing: an unapproved session is reported as missing.
func (catalog *Catalog) Lookup(ctx context.Context, sessionID string) (Summary, error) {
	if !validSessionID(sessionID) {
		return Summary{}, ErrSessionNotFound
	}
	summaries, err := catalog.listSummaries(ctx, 0)
	if err != nil {
		return Summary{}, err
	}
	for _, summary := range summaries {
		if summary.SessionID == sessionID {
			return summary, nil
		}
	}
	return Summary{}, ErrSessionNotFound
}

func (catalog *Catalog) listSummaries(ctx context.Context, limit int) ([]Summary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidates, err := catalog.candidateFiles()
	if err != nil {
		return nil, err
	}
	// Newest first, so a bounded sweep keeps the sessions the owner is most
	// likely to care about.
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].modified.After(candidates[right].modified)
	})
	if len(candidates) > maxSessionFiles {
		candidates = candidates[:maxSessionFiles]
	}

	summaries := make([]Summary, 0, len(candidates))
	skipped := 0
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		summary, approved, err := catalog.summarise(candidate)
		if err != nil || !approved {
			if !approved && err == nil {
				skipped++
			}
			continue
		}
		summaries = append(summaries, summary)
		if limit > 0 && len(summaries) >= limit {
			break
		}
	}
	catalog.logger.Debug("[claude-catalog] swept sessions",
		"scanned", len(candidates), "listed", len(summaries), "skipped_unapproved", skipped,
		"output_shape", "approved_sessions_newest_first")
	return summaries, nil
}

type candidate struct {
	path     string
	modified time.Time
	size     int64
}

func (catalog *Catalog) candidateFiles() ([]candidate, error) {
	directories, err := os.ReadDir(catalog.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No sessions yet is an empty catalog, not a failure: the owner may
			// simply not have run Claude Code on this computer.
			return nil, nil
		}
		return nil, errors.Join(ErrSessionsUnavailable, err)
	}
	candidates := make([]candidate, 0, len(directories))
	for index, directory := range directories {
		if index >= maxProjectDirectories {
			break
		}
		if !directory.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(catalog.root, directory.Name()))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{
				path:     filepath.Join(catalog.root, directory.Name(), entry.Name()),
				modified: info.ModTime(),
				size:     info.Size(),
			})
		}
	}
	return candidates, nil
}

// summarise reads only the head of a file plus its title records.
func (catalog *Catalog) summarise(file candidate) (Summary, bool, error) {
	catalog.mu.Lock()
	cached, found := catalog.cached[file.path]
	catalog.mu.Unlock()
	if found && cached.modified.Equal(file.modified) && cached.size == file.size {
		return cached.summary, cached.approved, nil
	}

	sessionID := strings.TrimSuffix(filepath.Base(file.path), ".jsonl")
	if !validSessionID(sessionID) {
		return Summary{}, false, nil
	}
	summary := Summary{SessionID: sessionID, Path: file.path, UpdatedAtUnix: file.modified.Unix()}
	firstPrompt := ""
	scanned := 0
	err := scanFile(file.path, func(entry record) bool {
		scanned++
		switch entry.Type {
		case recordAITitle:
			if entry.AITitle != "" {
				summary.Title = entry.AITitle
			}
		case recordUser, recordAssistant:
			if summary.CWD == "" && entry.CWD != "" {
				summary.CWD = entry.CWD
			}
			if firstPrompt == "" && entry.Type == recordUser && !entry.IsSidechain {
				firstPrompt = firstText(entry)
			}
		}
		// Keep going past the head only while still missing the cwd, which is
		// the one field approval depends on.
		return scanned < headRecordsForSummary || summary.CWD == ""
	})
	if err != nil {
		return Summary{}, false, err
	}
	if summary.Title == "" {
		summary.Title = titleFromPrompt(firstPrompt)
	}

	label, approved := "", false
	if summary.CWD != "" {
		label, approved = catalog.approver.ApproveCWD(summary.CWD)
	}
	summary.ProjectLabel = label
	if summary.Title == "" || summary.ProjectLabel == "" {
		approved = false
	}

	catalog.mu.Lock()
	if len(catalog.cached) > maxSessionFiles*2 {
		catalog.cached = make(map[string]cacheEntry)
	}
	catalog.cached[file.path] = cacheEntry{modified: file.modified, size: file.size, summary: summary, approved: approved}
	catalog.mu.Unlock()
	return summary, approved, nil
}

func firstText(entry record) string {
	for _, block := range entry.Message.blocks() {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return block.Text
		}
	}
	return ""
}

// titleFromPrompt builds a fallback title from the opening prompt when the CLI
// has not written one yet.
func titleFromPrompt(prompt string) string {
	cleaned := strings.Join(strings.Fields(prompt), " ")
	if cleaned == "" {
		return ""
	}
	const maximum = 80
	runes := []rune(cleaned)
	if len(runes) <= maximum {
		return cleaned
	}
	return strings.TrimSpace(string(runes[:maximum])) + "…"
}

// validSessionID accepts the CLI's session file names, which are UUIDs. It is
// deliberately strict: the name becomes a task ID on the wire and is joined
// into a filesystem path.
func validSessionID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		switch index {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			isDigit := char >= '0' && char <= '9'
			isLower := char >= 'a' && char <= 'f'
			isUpper := char >= 'A' && char <= 'F'
			if !isDigit && !isLower && !isUpper {
				return false
			}
		}
	}
	return true
}
