// Package store reads what Claude Code has already written to disk.
//
// The Codex app-server answers thread/list and thread/read over RPC. Claude
// Code has no equivalent: its history lives as newline-delimited JSON under
// ~/.claude/projects/<encoded-cwd>/<session-id>.jsonl, and the companion reads
// it directly.
//
// Two rules govern everything here. The files are read, never written: session
// state belongs to the CLI, and the companion keeps its own overlay in its own
// database. And a session is only ever surfaced to the phone when the cwd
// recorded inside the file is inside a project the owner approved -- the
// directory name is a lossy encoding and must never be used to make that
// decision.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// maxLineBytes bounds one transcript line. Tool results embed file and
	// command output, so this is large, but a corrupt or hostile file must not
	// be able to exhaust memory.
	maxLineBytes = 8 * 1024 * 1024
	// maxRecordsScanned bounds how much of one session file is walked.
	maxRecordsScanned = 100000
)

var (
	ErrSessionsUnavailable = errors.New("Claude Code session directory is unavailable")
	ErrSessionNotFound     = errors.New("Claude Code session was not found")
	ErrSessionUnreadable   = errors.New("Claude Code session file could not be read")
)

// Record types written to a session file. Only user and assistant records carry
// conversation; the rest is CLI bookkeeping that must not reach the phone.
const (
	recordUser      = "user"
	recordAssistant = "assistant"
	recordAITitle   = "ai-title"
)

// record is one decoded transcript line. Fields absent from a given record type
// stay zero, so callers check what they need rather than assuming a shape.
type record struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	IsSidechain bool            `json:"isSidechain"`
	AITitle     string          `json:"aiTitle"`
	Message     recordMessage   `json:"message"`
	ToolResult  json.RawMessage `json:"toolUseResult"`
}

type recordMessage struct {
	Role string `json:"role"`
	// Content is a list of blocks for the records the companion cares about,
	// but the CLI also writes plain strings, so it is decoded lazily.
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// blocks decodes message content, tolerating both the block-list form and the
// bare-string form the CLI uses for simple messages.
func (message recordMessage) blocks() []contentBlock {
	if len(message.Content) == 0 {
		return nil
	}
	var list []contentBlock
	if json.Unmarshal(message.Content, &list) == nil {
		return list
	}
	var text string
	if json.Unmarshal(message.Content, &text) == nil && text != "" {
		return []contentBlock{{Type: "text", Text: text}}
	}
	return nil
}

// resultText renders a tool result's content, which is a string for most tools
// and a block list for some.
func (block contentBlock) resultText() string {
	if len(block.Content) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(block.Content, &text) == nil {
		return text
	}
	var list []contentBlock
	if json.Unmarshal(block.Content, &list) == nil {
		parts := make([]string, 0, len(list))
		for _, inner := range list {
			if inner.Type == "text" && inner.Text != "" {
				parts = append(parts, inner.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func (entry record) timestamp() time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// Root locates the session directory, honouring CLAUDE_CONFIG_DIR the same way
// the CLI does so a non-default configuration is still readable.
func Root() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); configured != "" {
		return filepath.Join(configured, "projects"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Join(ErrSessionsUnavailable, err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// scanFile walks a session file line by line, calling visit for each decodable
// record. Undecodable lines are skipped: one bad line must not hide a whole
// task. Returning false from visit stops the walk early.
func scanFile(path string, visit func(record) bool) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrSessionNotFound
		}
		return errors.Join(ErrSessionUnreadable, err)
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64*1024)
	for scanned := 0; scanned < maxRecordsScanned; scanned++ {
		line, err := readBoundedLine(reader)
		if len(line) > 0 {
			var entry record
			if json.Unmarshal(line, &entry) == nil && entry.Type != "" {
				if !visit(entry) {
					return nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.Join(ErrSessionUnreadable, err)
		}
	}
	return nil
}

// readBoundedLine returns one line, discarding without buffering any line that
// exceeds the limit so a single huge record cannot exhaust memory.
func readBoundedLine(reader *bufio.Reader) ([]byte, error) {
	var collected []byte
	oversized := false
	for {
		chunk, err := reader.ReadSlice('\n')
		if !oversized {
			if len(collected)+len(chunk) > maxLineBytes {
				oversized = true
				collected = nil
			} else {
				collected = append(collected, chunk...)
			}
		}
		if err == nil {
			if oversized {
				return nil, nil
			}
			return trimEnd(collected), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if oversized {
			return nil, err
		}
		return trimEnd(collected), err
	}
}

func trimEnd(line []byte) []byte {
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	return line
}
