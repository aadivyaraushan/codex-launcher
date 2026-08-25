package tasktranscript

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxPageEntries = 64
	MaxEntryRunes  = 8192
	MaxFileChanges = 64
	maxPageBytes   = 192 * 1024
)

var (
	ErrInvalidTranscript = errors.New("invalid Codex task transcript")
	ErrTaskMismatch      = errors.New("Codex transcript task does not match request")
	ErrUnknownCursor     = errors.New("Codex transcript cursor is unknown")
)

type Kind string

const (
	KindUser       Kind = "user"
	KindAgent      Kind = "agent"
	KindReasoning  Kind = "reasoning"
	KindPlan       Kind = "plan"
	KindCommand    Kind = "command"
	KindFileChange Kind = "file_change"
	KindActivity   Kind = "activity"
	// KindMessage renders one received chat message as its own row: sender,
	// body text, and when it was sent. The wire requires exactly
	// {id, turnId, kind, sender, text, sentAt} for this kind
	// (contract/validation.go:880-887).
	KindMessage Kind = "message"
)

type PageOptions struct {
	TaskID        string
	BeforeEntryID string
	Limit         int
}

type Page struct {
	TaskID        string  `json:"taskId"`
	Entries       []Entry `json:"entries"`
	EarlierCursor string  `json:"earlierCursor,omitempty"`
	Truncated     bool    `json:"truncated"`
}

type Entry struct {
	ID      string       `json:"id"`
	TurnID  string       `json:"turnId,omitempty"`
	Kind    Kind         `json:"kind"`
	Text    string       `json:"text,omitempty"`
	Status  string       `json:"status,omitempty"`
	Command string       `json:"command,omitempty"`
	Output  string       `json:"output,omitempty"`
	Changes []FileChange `json:"changes,omitempty"`
	// Sender and SentAt are set only on KindMessage entries: the display name
	// of who sent the message, and when, as an RFC3339 string
	// (contract/validation.go:884-887 requires both, present only for "message").
	Sender string `json:"sender,omitempty"`
	SentAt string `json:"sentAt,omitempty"`
}

type FileChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Diff string `json:"diff,omitempty"`
}

type rawThread struct {
	ID    string    `json:"id"`
	Turns []rawTurn `json:"turns"`
}

type rawTurn struct {
	ID     string            `json:"id"`
	Role   string            `json:"role"`
	Text   string            `json:"text"`
	Status string            `json:"status"`
	Items  []json.RawMessage `json:"items"`
}

func MapAppServerPage(raw json.RawMessage, options PageOptions) (Page, error) {
	var response struct {
		Thread rawThread `json:"thread"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return Page{}, ErrInvalidTranscript
	}
	return mapPage(response.Thread, options, false)
}

func MapDesktopPage(raw json.RawMessage, options PageOptions) (Page, error) {
	var thread rawThread
	if json.Unmarshal(raw, &thread) != nil {
		return Page{}, ErrInvalidTranscript
	}
	return mapPage(thread, options, true)
}

func mapPage(thread rawThread, options PageOptions, allowLegacyTurns bool) (Page, error) {
	if !validID(options.TaskID) || thread.ID != options.TaskID {
		return Page{}, ErrTaskMismatch
	}
	if options.Limit < 1 || options.Limit > MaxPageEntries || options.BeforeEntryID != "" && !validID(options.BeforeEntryID) {
		return Page{}, ErrInvalidTranscript
	}
	entries := make([]Entry, 0)
	seen := make(map[string]struct{})
	for turnIndex, turn := range thread.Turns {
		turnID := turn.ID
		if turnID == "" && allowLegacyTurns && len(turn.Items) == 0 {
			turnID = fmt.Sprintf("desktop-turn-%d", turnIndex)
		}
		if !validID(turnID) {
			return Page{}, ErrInvalidTranscript
		}
		if len(turn.Items) == 0 && allowLegacyTurns && turn.Role != "" {
			kind, okay := legacyRoleKind(turn.Role)
			if !okay || turn.Text == "" {
				return Page{}, ErrInvalidTranscript
			}
			entry := Entry{ID: turnID, TurnID: turnID, Kind: kind, Text: turn.Text}
			if err := appendUnique(&entries, seen, entry); err != nil {
				return Page{}, err
			}
			continue
		}
		for _, rawItem := range turn.Items {
			entry, err := mapItem(rawItem, turnID)
			if err != nil {
				return Page{}, err
			}
			if err := appendUnique(&entries, seen, entry); err != nil {
				return Page{}, err
			}
		}
	}
	end := len(entries)
	if options.BeforeEntryID != "" {
		end = -1
		for index := range entries {
			if entries[index].ID == options.BeforeEntryID {
				end = index
				break
			}
		}
		if end < 0 {
			return Page{}, ErrUnknownCursor
		}
	}
	start := end - options.Limit
	if start < 0 {
		start = 0
	}
	page := Page{TaskID: options.TaskID, Entries: append([]Entry(nil), entries[start:end]...)}
	if start > 0 && len(page.Entries) != 0 {
		page.EarlierCursor = page.Entries[0].ID
	}
	return BoundPageForWire(page), nil
}

// BoundPageForWire enforces the two wire limits contract/validation.go and
// EncodeText impose on a task_page frame: no entry's text/command/output may
// exceed MaxEntryRunes, and the encoded frame may not exceed maxPageBytes.
// Every source of transcript pages (the desktop/app-server mapper here, and
// the phone-side thread store) must run its page through this before
// returning it, or an oversized entry or an oversized page becomes
// unencodable and the whole page fails to reach the client.
//
// Individual entries are clipped first via boundEntry. If the page as a
// whole is still too big, the oldest entries are dropped one at a time
// (newest entries are what a resumed conversation needs first) until it
// fits, and EarlierCursor is kept pointing at the earliest entry still
// present so the dropped entries stay reachable by paging further back.
// Either kind of clipping sets Truncated so the client knows the page is
// incomplete. The input page's Entries slice is consumed and returned as
// part of the result, not aliased by the caller afterward.
func BoundPageForWire(page Page) Page {
	for index := range page.Entries {
		bounded, wasTruncated := boundEntry(page.Entries[index])
		page.Entries[index] = bounded
		page.Truncated = page.Truncated || wasTruncated
	}
	for encodedPageBytes(page) > maxPageBytes && len(page.Entries) > 1 {
		page.Entries = page.Entries[1:]
		page.EarlierCursor = page.Entries[0].ID
		page.Truncated = true
	}
	return page
}

func mapItem(raw json.RawMessage, turnID string) (Entry, error) {
	var header struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &header) != nil || !validID(header.ID) || header.Type == "" {
		return Entry{}, ErrInvalidTranscript
	}
	entry := Entry{ID: header.ID, TurnID: turnID}
	switch header.Type {
	case "userMessage":
		var item struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
				Name string `json:"name"`
			} `json:"content"`
		}
		if json.Unmarshal(raw, &item) != nil || len(item.Content) == 0 {
			return Entry{}, ErrInvalidTranscript
		}
		parts := make([]string, 0, len(item.Content))
		for _, content := range item.Content {
			switch content.Type {
			case "text":
				if content.Text == "" {
					return Entry{}, ErrInvalidTranscript
				}
				parts = append(parts, content.Text)
			case "mention":
				if content.Name == "" {
					return Entry{}, ErrInvalidTranscript
				}
				parts = append(parts, "@"+content.Name)
			case "skill":
				if content.Name == "" {
					return Entry{}, ErrInvalidTranscript
				}
				parts = append(parts, "$"+content.Name)
			case "image", "localImage":
				parts = append(parts, "Image attached")
			default:
				return Entry{}, ErrInvalidTranscript
			}
		}
		entry.Kind, entry.Text = KindUser, strings.Join(parts, "\n")
	case "agentMessage":
		var item struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return Entry{}, ErrInvalidTranscript
		}
		if item.Text == "" {
			entry.Kind, entry.Text = KindActivity, "Agent response in progress"
		} else {
			entry.Kind, entry.Text = KindAgent, item.Text
		}
	case "reasoning":
		var item struct {
			Summary []string `json:"summary"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return Entry{}, ErrInvalidTranscript
		}
		entry.Kind, entry.Text = KindReasoning, strings.Join(item.Summary, "\n")
		if entry.Text == "" {
			entry.Text = "Reasoning activity"
		}
	case "plan":
		var item struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return Entry{}, ErrInvalidTranscript
		}
		entry.Kind, entry.Text = KindPlan, item.Text
		if entry.Text == "" {
			entry.Text = "Plan activity"
		}
	case "commandExecution":
		var item struct {
			Command string  `json:"command"`
			Output  *string `json:"aggregatedOutput"`
			Status  string  `json:"status"`
		}
		if json.Unmarshal(raw, &item) != nil || item.Command == "" || !validStatus(item.Status) {
			return Entry{}, ErrInvalidTranscript
		}
		entry.Kind, entry.Command, entry.Status = KindCommand, item.Command, item.Status
		if item.Output != nil {
			entry.Output = *item.Output
		}
	case "fileChange":
		var item struct {
			Status  string `json:"status"`
			Changes []struct {
				Path string          `json:"path"`
				Kind json.RawMessage `json:"kind"`
				Diff string          `json:"diff"`
			} `json:"changes"`
		}
		if json.Unmarshal(raw, &item) != nil || !validStatus(item.Status) {
			return Entry{}, ErrInvalidTranscript
		}
		entry.Kind, entry.Status = KindFileChange, item.Status
		for _, change := range item.Changes {
			kind := decodeChangeKind(change.Kind)
			if change.Path == "" || kind == "" {
				return Entry{}, ErrInvalidTranscript
			}
			entry.Changes = append(entry.Changes, FileChange{Path: change.Path, Kind: kind, Diff: change.Diff})
		}
		if len(entry.Changes) == 0 {
			entry.Kind, entry.Text, entry.Status = KindActivity, "File activity", ""
		}
	default:
		entry.Kind, entry.Text = KindActivity, activityLabel(header.Type)
	}
	return entry, nil
}

func decodeChangeKind(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var object struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.Type
	}
	return ""
}

func boundEntry(entry Entry) (Entry, bool) {
	remaining := MaxEntryRunes
	truncated := false
	bound := func(value string) string {
		if value == "" {
			return ""
		}
		runes := []rune(value)
		if len(runes) > remaining {
			runes = runes[:remaining]
			truncated = true
		}
		remaining -= len(runes)
		return string(runes)
	}
	entry.Text = bound(entry.Text)
	entry.Command = bound(entry.Command)
	entry.Output = bound(entry.Output)
	changes := entry.Changes
	if len(changes) > MaxFileChanges {
		changes = changes[:MaxFileChanges]
		truncated = true
	}
	entry.Changes = make([]FileChange, 0, len(changes))
	for _, change := range changes {
		change.Path = bound(change.Path)
		change.Kind = bound(change.Kind)
		if change.Path == "" || change.Kind == "" {
			truncated = true
			break
		}
		change.Diff = bound(change.Diff)
		entry.Changes = append(entry.Changes, change)
	}
	if entry.Kind == KindFileChange && len(entry.Changes) == 0 {
		entry.Kind, entry.Text, entry.Status = KindActivity, "File activity", ""
	}
	return entry, truncated
}

func appendUnique(entries *[]Entry, seen map[string]struct{}, entry Entry) error {
	if _, duplicate := seen[entry.ID]; duplicate {
		return ErrInvalidTranscript
	}
	seen[entry.ID] = struct{}{}
	*entries = append(*entries, entry)
	return nil
}

func legacyRoleKind(role string) (Kind, bool) {
	switch role {
	case "user":
		return KindUser, true
	case "assistant", "agent":
		return KindAgent, true
	default:
		return "", false
	}
}

func validID(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func validStatus(status string) bool {
	switch status {
	case "inProgress", "completed", "failed", "declined":
		return true
	default:
		return false
	}
}

func activityLabel(itemType string) string {
	switch itemType {
	case "mcpToolCall", "dynamicToolCall":
		return "Tool activity"
	case "webSearch":
		return "Web search"
	case "imageView":
		return "Image viewed"
	case "collabAgentToolCall", "subAgentActivity":
		return "Agent activity"
	default:
		return "Codex activity"
	}
}

func encodedPageBytes(page Page) int {
	encoded, err := json.Marshal(page)
	if err != nil {
		return maxPageBytes + 1
	}
	return len(encoded)
}
