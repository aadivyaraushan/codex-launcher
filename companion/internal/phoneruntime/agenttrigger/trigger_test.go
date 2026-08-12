package agenttrigger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperwatch"
)

func TestMissingRulesFileMaterializesTheDefault(t *testing.T) {
	root := t.TempDir()

	rules, err := EnsureRules(root)
	if err != nil {
		t.Fatalf("EnsureRules: %v", err)
	}
	if !strings.Contains(rules, "draft") {
		t.Fatalf("default rules must allow drafts, got: %q", rules)
	}
	if !strings.Contains(rules, "summar") {
		t.Fatalf("default rules must allow summaries, got: %q", rules)
	}
	if !strings.Contains(strings.ToLower(rules), "allow list") {
		t.Fatalf("default rules must restrict autonomous sends to an allow list, got: %q", rules)
	}

	// The default is written to disk so the owner can find and edit it.
	onDisk, err := os.ReadFile(filepath.Join(root, "agent-rules.md"))
	if err != nil {
		t.Fatalf("default rules were not materialized: %v", err)
	}
	if string(onDisk) != rules {
		t.Fatal("materialized file must match the returned rules exactly")
	}
}

func TestAnEditedRulesFileLoadsVerbatimAndIsNotOverwritten(t *testing.T) {
	root := t.TempDir()
	custom := "# My rules\nReply to Maya immediately.\n"
	path := filepath.Join(root, "agent-rules.md")
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatalf("write custom rules: %v", err)
	}

	rules, err := EnsureRules(root)
	if err != nil {
		t.Fatalf("EnsureRules: %v", err)
	}
	if rules != custom {
		t.Fatalf("rules = %q, want the owner's file verbatim %q", rules, custom)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != custom {
		t.Fatal("EnsureRules must never overwrite an existing rules file")
	}
}

func TestPromptCarriesTheMessageAndTheRules(t *testing.T) {
	msg := beeperwatch.Message{
		ID:         "msg-1",
		ChatID:     "chat-1",
		SenderName: "Maya",
		Text:       "you around tonight?",
		Timestamp:  "2026-08-12T09:00:00Z",
	}

	prompt := Prompt(msg, "RULES-SENTINEL")

	for _, want := range []string{"Maya", "chat-1", "you around tonight?", "2026-08-12T09:00:00Z", "RULES-SENTINEL"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt is missing %q:\n%s", want, prompt)
		}
	}
	// The agent must be able to tell this apart from the owner speaking.
	if !strings.Contains(strings.ToLower(prompt), "automatic trigger") {
		t.Fatalf("prompt must say it is an automatic trigger, not the owner:\n%s", prompt)
	}
}

func TestPreviewNamesTheSender(t *testing.T) {
	preview := Preview(beeperwatch.Message{SenderName: "Maya", Text: "hi"})
	if preview != "New message from Maya" {
		t.Fatalf("preview = %q, want %q", preview, "New message from Maya")
	}
	anonymous := Preview(beeperwatch.Message{Text: "hi"})
	if anonymous != "New message" {
		t.Fatalf("preview without a sender name = %q, want %q", anonymous, "New message")
	}
}
