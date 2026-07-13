package tasktranscript

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMapAppServerPagePreservesOrderedTranscriptContent(t *testing.T) {
	raw := json.RawMessage(`{"thread":{"id":"thread-1","turns":[{"id":"turn-1","status":"completed","items":[
		{"id":"user-1","type":"userMessage","content":[{"type":"text","text":"Fix the tests"},{"type":"mention","name":"AGENTS.md","path":"/private/AGENTS.md"}]},
		{"id":"reason-1","type":"reasoning","summary":["Checking the failing package"],"content":["hidden chain of thought"]},
		{"id":"command-1","type":"commandExecution","command":"go test ./...","cwd":"/private/project","status":"failed","aggregatedOutput":"FAIL package"},
		{"id":"file-1","type":"fileChange","status":"completed","changes":[{"path":"src/main.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@"}]},
		{"id":"agent-1","type":"agentMessage","text":"I fixed the failing test."}
	]}]}}`)

	page, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.TaskID != "thread-1" || page.EarlierCursor != "" || page.Truncated {
		t.Fatalf("page metadata = %#v", page)
	}
	if len(page.Entries) != 5 {
		t.Fatalf("entries = %#v", page.Entries)
	}
	if page.Entries[0].Kind != KindUser || page.Entries[0].Text != "Fix the tests\n@AGENTS.md" {
		t.Fatalf("user entry = %#v", page.Entries[0])
	}
	if page.Entries[1].Kind != KindReasoning || page.Entries[1].Text != "Checking the failing package" || strings.Contains(page.Entries[1].Text, "hidden") {
		t.Fatalf("reasoning entry = %#v", page.Entries[1])
	}
	if page.Entries[2].Kind != KindCommand || page.Entries[2].Command != "go test ./..." || page.Entries[2].Output != "FAIL package" || page.Entries[2].Status != "failed" {
		t.Fatalf("command entry = %#v", page.Entries[2])
	}
	if page.Entries[3].Kind != KindFileChange || len(page.Entries[3].Changes) != 1 || page.Entries[3].Changes[0].Path != "src/main.go" || page.Entries[3].Changes[0].Kind != "update" || page.Entries[3].Changes[0].Diff != "@@ -1 +1 @@" {
		t.Fatalf("file entry = %#v", page.Entries[3])
	}
	if page.Entries[4].Kind != KindAgent || page.Entries[4].Text != "I fixed the failing test." {
		t.Fatalf("agent entry = %#v", page.Entries[4])
	}
}

func TestMapDesktopPageSupportsLegacyTurnsWithoutInventingPrivatePaths(t *testing.T) {
	raw := json.RawMessage(`{"id":"thread-1","cwd":"/private/project","turns":[
		{"role":"user","text":"Check status"},
		{"role":"assistant","text":"Everything passes."}
	]}`)

	page, err := MapDesktopPage(raw, PageOptions{TaskID: "thread-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 || page.Entries[0].ID != "desktop-turn-0" || page.Entries[0].Kind != KindUser || page.Entries[1].Kind != KindAgent {
		t.Fatalf("legacy entries = %#v", page.Entries)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/private/project") {
		t.Fatalf("page leaked task cwd: %s", encoded)
	}
}

func TestTranscriptPageUsesExclusiveStableEarlierCursor(t *testing.T) {
	raw := json.RawMessage(`{"thread":{"id":"thread-1","turns":[{"id":"turn-1","status":"completed","items":[
		{"id":"user-1","type":"userMessage","content":[{"type":"text","text":"one"}]},
		{"id":"agent-1","type":"agentMessage","text":"two"},
		{"id":"user-2","type":"userMessage","content":[{"type":"text","text":"three"}]},
		{"id":"agent-2","type":"agentMessage","text":"four"}
	]}]}}`)

	latest, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{latest.Entries[0].ID, latest.Entries[1].ID}; got[0] != "user-2" || got[1] != "agent-2" || latest.EarlierCursor != "user-2" {
		t.Fatalf("latest page = %#v", latest)
	}
	earlier, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 2, BeforeEntryID: latest.EarlierCursor})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{earlier.Entries[0].ID, earlier.Entries[1].ID}; got[0] != "user-1" || got[1] != "agent-1" || earlier.EarlierCursor != "" {
		t.Fatalf("earlier page = %#v", earlier)
	}
}

func TestTranscriptPageRejectsMismatchesDuplicatesAndUnknownCursors(t *testing.T) {
	tests := []struct {
		name    string
		raw     json.RawMessage
		options PageOptions
		want    error
	}{
		{name: "task mismatch", raw: json.RawMessage(`{"thread":{"id":"other","turns":[]}}`), options: PageOptions{TaskID: "thread-1", Limit: 2}, want: ErrTaskMismatch},
		{name: "duplicate item", raw: json.RawMessage(`{"thread":{"id":"thread-1","turns":[{"id":"turn-1","status":"completed","items":[{"id":"same","type":"agentMessage","text":"one"},{"id":"same","type":"agentMessage","text":"two"}]}]}}`), options: PageOptions{TaskID: "thread-1", Limit: 2}, want: ErrInvalidTranscript},
		{name: "unknown cursor", raw: json.RawMessage(`{"thread":{"id":"thread-1","turns":[]}}`), options: PageOptions{TaskID: "thread-1", Limit: 2, BeforeEntryID: "missing"}, want: ErrUnknownCursor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := MapAppServerPage(test.raw, test.options); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestTranscriptPageBoundsSingleLargeEntryWithoutBreakingUTF8(t *testing.T) {
	text := strings.Repeat("🙂", MaxEntryRunes+10)
	raw, err := json.Marshal(map[string]any{
		"thread": map[string]any{
			"id": "thread-1",
			"turns": []any{map[string]any{
				"id":     "turn-1",
				"status": "completed",
				"items": []any{map[string]any{
					"id":   "agent-1",
					"type": "agentMessage",
					"text": text,
				}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Truncated || len([]rune(page.Entries[0].Text)) != MaxEntryRunes || !strings.HasSuffix(page.Entries[0].Text, "🙂") {
		t.Fatalf("bounded entry runes = %d, truncated = %v", len([]rune(page.Entries[0].Text)), page.Truncated)
	}
}

func TestTranscriptTruncationDescribesOnlyTheReturnedPage(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"thread": map[string]any{
			"id": "thread-1",
			"turns": []any{map[string]any{
				"id":     "turn-1",
				"status": "completed",
				"items": []any{
					map[string]any{"id": "agent-old", "type": "agentMessage", "text": strings.Repeat("old", MaxEntryRunes)},
					map[string]any{"id": "agent-new", "type": "agentMessage", "text": "current"},
				},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Truncated || len(page.Entries) != 1 || page.Entries[0].ID != "agent-new" {
		t.Fatalf("latest page inherited older truncation: %#v", page)
	}
}

func TestTranscriptPageCapsFileChangesAndEncodedSize(t *testing.T) {
	const maxExpectedFileChanges = 64
	changes := make([]any, maxExpectedFileChanges+100)
	for index := range changes {
		changes[index] = map[string]any{
			"path": "src/file.go",
			"kind": map[string]any{"type": "update"},
			"diff": strings.Repeat("diff", MaxEntryRunes),
		}
	}
	raw, err := json.Marshal(map[string]any{
		"thread": map[string]any{
			"id": "thread-1",
			"turns": []any{map[string]any{
				"id":     "turn-1",
				"status": "completed",
				"items": []any{map[string]any{
					"id": "file-1", "type": "fileChange", "status": "completed", "changes": changes,
				}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Truncated || len(page.Entries[0].Changes) > maxExpectedFileChanges || len(encoded) > maxPageBytes {
		t.Fatalf("bounded file page: truncated=%v changes=%d bytes=%d", page.Truncated, len(page.Entries[0].Changes), len(encoded))
	}
	for _, change := range page.Entries[0].Changes {
		if change.Path == "" || change.Kind == "" {
			t.Fatalf("bounded file page retained an empty change: %#v", change)
		}
	}
}

func TestEmptyReasoningSummaryMapsToContentFreeActivityLabel(t *testing.T) {
	raw := json.RawMessage(`{"thread":{"id":"thread-1","turns":[{"id":"turn-1","status":"completed","items":[
		{"id":"reason-1","type":"reasoning","summary":[],"content":["hidden chain of thought"]}
	]}]}}`)

	page, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Kind != KindReasoning || page.Entries[0].Text != "Reasoning activity" || strings.Contains(page.Entries[0].Text, "hidden") {
		t.Fatalf("reasoning entry = %#v", page.Entries)
	}
}

func TestIncompleteItemsMapToValidContentFreeActivityLabels(t *testing.T) {
	raw := json.RawMessage(`{"thread":{"id":"thread-1","turns":[{"id":"turn-1","status":"inProgress","items":[
		{"id":"plan-1","type":"plan","text":""},
		{"id":"file-1","type":"fileChange","status":"inProgress","changes":[]}
	]}]}}`)

	page, err := MapAppServerPage(raw, PageOptions{TaskID: "thread-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 || page.Entries[0].Kind != KindPlan || page.Entries[0].Text != "Plan activity" {
		t.Fatalf("incomplete plan entry = %#v", page.Entries)
	}
	if page.Entries[1].Kind != KindActivity || page.Entries[1].Text != "File activity" || page.Entries[1].Status != "" || len(page.Entries[1].Changes) != 0 {
		t.Fatalf("incomplete file entry = %#v", page.Entries[1])
	}
}
