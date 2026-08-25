// Package agenttrigger turns an inbound Beeper message into the prompt the
// on-phone agent sees, and owns the owner-editable rules file that governs
// what the agent is allowed to do without being asked first.
package agenttrigger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperwatch"
)

// rulesFileName is where the owner-editable rules live, relative to the
// phone runtime's root.
const rulesFileName = "agent-rules.md"

// defaultRules is written the first time EnsureRules runs against a root
// with no rules file yet. It is deliberately permissive about passive work
// (drafting, summarizing) and conservative about anything that leaves the
// phone on its own — the allow list starts empty, so nothing sends
// automatically until the owner adds someone.
const defaultRules = `# Agent rules

These rules govern what the agent may do on its own when it is triggered by
an incoming message, without the phone owner asking first.

- Writing a draft reply or a summary unprompted is always allowed.
- Sending a message autonomously is only allowed to people on the allow
  list below. The allow list starts empty, so nothing sends automatically
  until the owner adds someone here.
- Everything else — any action not covered above — needs the owner's
  approval before it happens.

## Allow list

(empty — add a name or handle per line to allow autonomous sends to them)
`

// EnsureRules returns the contents of <root>/agent-rules.md, writing the
// default rules file if none exists yet. An existing file is returned
// verbatim and is never overwritten, so an owner's edits always win.
func EnsureRules(root string) (string, error) {
	path := filepath.Join(root, rulesFileName)
	existing, err := os.ReadFile(path)
	if err == nil {
		return string(existing), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("agenttrigger: read rules: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultRules), 0o600); err != nil {
		return "", fmt.Errorf("agenttrigger: write default rules: %w", err)
	}
	return defaultRules, nil
}

// Prompt formats the message that triggers the agent's turn: the incoming
// message, followed by the rules that govern what it may do about it. It
// says up front that this is an automatic trigger from an incoming
// message, not the phone owner speaking, so the agent never confuses the
// two.
func Prompt(msg beeperwatch.Message, rules string) string {
	return fmt.Sprintf(
		"This is an automatic trigger from an incoming message, not the phone owner speaking.\n\n"+
			"From: %s\n"+
			"Chat: %s\n"+
			"Time: %s\n"+
			"Message: %s\n\n"+
			"Rules for what you may do on your own:\n%s\n",
		msg.SenderName, msg.ChatID, msg.Timestamp, msg.Text, rules,
	)
}

// Allowed reports whether recipient is on the rules file's allow list: the
// section under a markdown heading whose text contains "allow list",
// running until the next heading. Entries are one per line, an optional
// "-" or "*" bullet stripped, matched case-insensitively against recipient
// with both sides trimmed. A rules file with no allow list section, or an
// empty one, allows nobody — the default rules' promise that nothing sends
// automatically until the owner adds someone.
func Allowed(rules, recipient string) bool {
	want := strings.TrimSpace(recipient)
	inSection := false
	for _, line := range strings.Split(rules, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			inSection = strings.Contains(strings.ToLower(trimmed), "allow list")
			continue
		}
		if !inSection || trimmed == "" || strings.HasPrefix(trimmed, "(") {
			continue
		}
		entry := trimmed
		switch {
		case strings.HasPrefix(entry, "- "):
			entry = entry[len("- "):]
		case strings.HasPrefix(entry, "* "):
			entry = entry[len("* "):]
		case strings.HasPrefix(entry, "-"):
			entry = entry[len("-"):]
		case strings.HasPrefix(entry, "*"):
			entry = entry[len("*"):]
		}
		if strings.EqualFold(strings.TrimSpace(entry), want) {
			return true
		}
	}
	return false
}

// Preview is the short line the Home task list shows for a triggered turn,
// before the agent has produced a reply of its own.
func Preview(msg beeperwatch.Message) string {
	if msg.SenderName == "" {
		return "New message"
	}
	return "New message from " + msg.SenderName
}
