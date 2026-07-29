package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/tasktranscript"
	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/turn"
)

// maxDiffBytes bounds a rendered file diff. Entries are bounded again by
// tasktranscript before they reach the phone; this stops a huge patch from
// being built in the first place.
const maxDiffBytes = 32 * 1024

// ReadTranscript returns a page of a session's transcript, newest page first,
// paging backwards through the file the way the phone reads it.
//
// The session must be approved: an unapproved one is reported as missing, the
// same as an unknown one.
func (catalog *Catalog) ReadTranscript(ctx context.Context, sessionID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	summary, err := catalog.Lookup(ctx, sessionID)
	if err != nil {
		return tasktranscript.Page{}, err
	}
	if err := ctx.Err(); err != nil {
		return tasktranscript.Page{}, err
	}
	entries, err := readEntries(summary.Path)
	if err != nil {
		return tasktranscript.Page{}, err
	}
	return pageOf(sessionID, entries, options)
}

// pageOf slices the assembled entries into the page the phone asked for.
func pageOf(sessionID string, entries []tasktranscript.Entry, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	limit := options.Limit
	if limit <= 0 || limit > tasktranscript.MaxPageEntries {
		limit = tasktranscript.MaxPageEntries
	}
	end := len(entries)
	if cursor := strings.TrimSpace(options.BeforeEntryID); cursor != "" {
		found := -1
		for index, entry := range entries {
			if entry.ID == cursor {
				found = index
				break
			}
		}
		if found < 0 {
			// The phone is holding a cursor from a transcript that has since been
			// rewritten or removed; say so rather than silently returning the
			// newest page and looking like the history jumped.
			return tasktranscript.Page{}, tasktranscript.ErrUnknownCursor
		}
		end = found
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := tasktranscript.Page{TaskID: sessionID, Entries: entries[start:end]}
	if start > 0 {
		page.EarlierCursor = entries[start].ID
	}
	return page, nil
}

// readEntries assembles the whole session into transcript entries, oldest
// first. Tool results are folded into the tool call they answer, so the phone
// shows one command entry with its output rather than two disconnected rows.
func readEntries(path string) ([]tasktranscript.Entry, error) {
	var entries []tasktranscript.Entry
	// toolEntry maps a tool_use ID to the entry awaiting its result.
	toolEntry := make(map[string]int)

	err := scanFile(path, func(entry record) bool {
		// Subagent transcripts are a second conversation the phone has no way to
		// show, so they are folded to a single activity row instead of being
		// interleaved into the owner's transcript.
		if entry.IsSidechain {
			return true
		}
		switch entry.Type {
		case recordUser:
			entries = appendUserRecord(entries, toolEntry, entry)
		case recordAssistant:
			entries = appendAssistantRecord(entries, toolEntry, entry)
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func appendUserRecord(entries []tasktranscript.Entry, toolEntry map[string]int, entry record) []tasktranscript.Entry {
	prompt := make([]string, 0, 2)
	for _, block := range entry.Message.blocks() {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				prompt = append(prompt, block.Text)
			}
		case "tool_result":
			// Attach the output to the call it answers. A result whose call was
			// never seen is dropped: an output with no command is not something
			// the owner can interpret.
			index, found := toolEntry[block.ToolUseID]
			if !found {
				continue
			}
			delete(toolEntry, block.ToolUseID)
			entries[index].Output = block.resultText()
			// Overwrites the in_progress the call was created with; a result is
			// the only thing that settles a tool entry's status.
			if block.IsError {
				entries[index].Status = "failed"
			} else {
				entries[index].Status = "completed"
			}
			if entries[index].Kind == tasktranscript.KindFileChange && !block.IsError {
				applyStructuredPatch(&entries[index], entry.ToolResult)
			}
		}
	}
	if len(prompt) == 0 {
		return entries
	}
	return append(entries, tasktranscript.Entry{
		ID:   entryID(entry, len(entries)),
		Kind: tasktranscript.KindUser,
		Text: strings.Join(prompt, "\n"),
	})
}

func appendAssistantRecord(entries []tasktranscript.Entry, toolEntry map[string]int, entry record) []tasktranscript.Entry {
	for _, block := range entry.Message.blocks() {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			entries = append(entries, tasktranscript.Entry{
				ID:   entryID(entry, len(entries)),
				Kind: tasktranscript.KindAgent,
				Text: block.Text,
			})
		case "thinking":
			if strings.TrimSpace(block.Thinking) == "" {
				continue
			}
			entries = append(entries, tasktranscript.Entry{
				ID:   entryID(entry, len(entries)),
				Kind: tasktranscript.KindReasoning,
				Text: block.Thinking,
			})
		case "tool_use":
			entries = append(entries, toolUseEntry(entry, block, len(entries)))
			if block.ID != "" {
				toolEntry[block.ID] = len(entries) - 1
			}
		}
	}
	return entries
}

func toolUseEntry(entry record, block contentBlock, position int) tasktranscript.Entry {
	built := tasktranscript.Entry{ID: entryID(entry, position), Status: "in_progress"}
	switch turn.ActivityForTool(block.Name) {
	case "command":
		built.Kind = tasktranscript.KindCommand
		built.Command = bashCommand(block.Input)
	case "file":
		built.Kind = tasktranscript.KindFileChange
		if path := turn.ToolInputPath(block.Name, block.Input); path != "" {
			built.Changes = []tasktranscript.FileChange{{Path: path, Kind: fileChangeKind(block.Name)}}
		}
	case "plan":
		built.Kind = tasktranscript.KindPlan
		built.Text = planText(block.Input)
	default:
		// Reads, searches, MCP calls, and anything the CLI adds later. The tool
		// name is shown because it is the only honest description available, and
		// the input is withheld because it can quote file contents.
		built.Kind = tasktranscript.KindActivity
		built.Text = activityLabel(block.Name)
	}
	return built
}

// fileChangeKind distinguishes a new file from an edit to an existing one,
// matching the vocabulary the Codex mapper already sends to the phone.
func fileChangeKind(tool string) string {
	if tool == turn.ToolWrite {
		return "add"
	}
	return "update"
}

func bashCommand(input json.RawMessage) string {
	var value struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(input, &value) != nil {
		return ""
	}
	return value.Command
}

// planText renders a TodoWrite input as the plan the owner sees.
func planText(input json.RawMessage) string {
	var value struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	if json.Unmarshal(input, &value) != nil || len(value.Todos) == 0 {
		return "Updated the plan"
	}
	lines := make([]string, 0, len(value.Todos))
	for _, todo := range value.Todos {
		if strings.TrimSpace(todo.Content) == "" {
			continue
		}
		marker := "[ ]"
		switch todo.Status {
		case "completed":
			marker = "[x]"
		case "in_progress":
			marker = "[~]"
		}
		lines = append(lines, marker+" "+todo.Content)
	}
	if len(lines) == 0 {
		return "Updated the plan"
	}
	return strings.Join(lines, "\n")
}

func activityLabel(tool string) string {
	switch tool {
	case turn.ToolRead:
		return "Read a file"
	case turn.ToolGrep, turn.ToolGlob:
		return "Searched the project"
	case turn.ToolTask:
		return "Ran a subagent"
	case "":
		return "Used a tool"
	case turn.ToolAskUserQuestion:
		return "Asked a question"
	}
	if strings.HasPrefix(tool, "mcp__") {
		return "Used a connected tool"
	}
	return "Used " + tool
}

// applyStructuredPatch turns the CLI's recorded patch hunks into a unified diff
// for the file-change entry.
func applyStructuredPatch(entry *tasktranscript.Entry, raw json.RawMessage) {
	if len(raw) == 0 || len(entry.Changes) == 0 {
		return
	}
	var value struct {
		FilePath        string `json:"filePath"`
		StructuredPatch []struct {
			OldStart int      `json:"oldStart"`
			OldLines int      `json:"oldLines"`
			NewStart int      `json:"newStart"`
			NewLines int      `json:"newLines"`
			Lines    []string `json:"lines"`
		} `json:"structuredPatch"`
	}
	if json.Unmarshal(raw, &value) != nil || len(value.StructuredPatch) == 0 {
		return
	}
	var builder strings.Builder
	for _, hunk := range value.StructuredPatch {
		header := "@@ -" + hunkRange(hunk.OldStart, hunk.OldLines) + " +" + hunkRange(hunk.NewStart, hunk.NewLines) + " @@\n"
		if builder.Len()+len(header) > maxDiffBytes {
			break
		}
		builder.WriteString(header)
		for _, line := range hunk.Lines {
			if builder.Len()+len(line)+1 > maxDiffBytes {
				break
			}
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
	}
	entry.Changes[0].Diff = builder.String()
	if value.FilePath != "" && entry.Changes[0].Path == "" {
		entry.Changes[0].Path = value.FilePath
	}
}

func hunkRange(start, lines int) string {
	return strconv.Itoa(start) + "," + strconv.Itoa(lines)
}

// entryID names an entry stably across reads so the phone's paging cursor
// stays valid. The record UUID is stable and unique; the position suffix
// separates several entries produced by one record.
func entryID(entry record, position int) string {
	base := entry.UUID
	if base == "" {
		base = "entry"
	}
	return fmt.Sprintf("%s.%d", base, position)
}
