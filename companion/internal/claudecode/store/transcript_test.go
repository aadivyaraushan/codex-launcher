package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/tasktranscript"
)

const transcriptCWD = "/Users/owner/work/launcher"

func assistantLine(uuid, blocks string) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":%q,"sessionId":%q,"cwd":%q,"isSidechain":false,"timestamp":"2026-07-29T12:00:01.000Z","message":{"role":"assistant","content":[%s]}}`, uuid, sessionA, transcriptCWD, blocks)
}

func toolResultLine(uuid, toolUseID, content string, isError bool, toolUseResult string) string {
	if toolUseResult == "" {
		toolUseResult = "null"
	}
	return fmt.Sprintf(`{"type":"user","uuid":%q,"sessionId":%q,"cwd":%q,"isSidechain":false,"timestamp":"2026-07-29T12:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":%q,"is_error":%t}]},"toolUseResult":%s}`,
		uuid, sessionA, transcriptCWD, toolUseID, content, isError, toolUseResult)
}

func readTranscript(t *testing.T, lines []string, options tasktranscript.PageOptions) tasktranscript.Page {
	t.Helper()
	root := t.TempDir()
	writeSession(t, root, sessionA, transcriptCWD, lines...)
	page, err := newCatalog(t, root, approveAll).ReadTranscript(context.Background(), sessionA, options)
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	return page
}

func TestTranscriptMapsEachBlockKindTheOwnerCanRecognise(t *testing.T) {
	lines := []string{
		titleLine(sessionA, "Fix the build"),
		userLine(sessionA, transcriptCWD, "Fix the build please"),
		assistantLine("a-1", `{"type":"thinking","thinking":"I should look at the failure first."}`),
		assistantLine("a-2", `{"type":"text","text":"Looking at it now."}`),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	want := []struct {
		kind tasktranscript.Kind
		text string
	}{
		{tasktranscript.KindUser, "Fix the build please"},
		{tasktranscript.KindReasoning, "I should look at the failure first."},
		{tasktranscript.KindAgent, "Looking at it now."},
	}
	if len(page.Entries) != len(want) {
		t.Fatalf("entries = %#v", page.Entries)
	}
	for index, expected := range want {
		got := page.Entries[index]
		if got.Kind != expected.kind || got.Text != expected.text {
			t.Fatalf("entry %d = %#v; want %q %q", index, got, expected.kind, expected.text)
		}
	}
}

// A command and its output belong in one row; the owner cannot interpret an
// output that is detached from the command that produced it.
func TestCommandOutputIsFoldedIntoTheCommandEntry(t *testing.T) {
	lines := []string{
		titleLine(sessionA, "Run tests"),
		userLine(sessionA, transcriptCWD, "Run the tests"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}}`),
		toolResultLine("u-2", "toolu_1", "ok  everything passed", false, ""),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	if len(page.Entries) != 2 {
		t.Fatalf("entries = %#v", page.Entries)
	}
	command := page.Entries[1]
	if command.Kind != tasktranscript.KindCommand || command.Command != "go test ./..." {
		t.Fatalf("command entry = %#v", command)
	}
	if command.Output != "ok  everything passed" || command.Status != "completed" {
		t.Fatalf("output = %q status = %q", command.Output, command.Status)
	}
}

func TestFailedToolResultMarksTheEntryFailed(t *testing.T) {
	lines := []string{
		titleLine(sessionA, "Run tests"),
		userLine(sessionA, transcriptCWD, "Run the tests"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"false"}}`),
		toolResultLine("u-2", "toolu_1", "exit status 1", true, ""),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	command := page.Entries[len(page.Entries)-1]
	if command.Status != "failed" || command.Output != "exit status 1" {
		t.Fatalf("entry = %#v", command)
	}
}

// A tool call still waiting for its result must not look finished.
func TestUnansweredToolCallStaysInProgress(t *testing.T) {
	lines := []string{
		titleLine(sessionA, "Run tests"),
		userLine(sessionA, transcriptCWD, "Run the tests"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"sleep 100"}}`),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	command := page.Entries[len(page.Entries)-1]
	if command.Status != "in_progress" || command.Output != "" {
		t.Fatalf("entry = %#v", command)
	}
}

func TestFileEditBecomesAFileChangeWithADiff(t *testing.T) {
	// structuredPatch captured verbatim from a real Edit tool result.
	toolUseResult := `{"filePath":"/Users/owner/work/launcher/sample.txt","structuredPatch":[{"oldStart":1,"oldLines":3,"newStart":1,"newLines":3,"lines":[" alpha","-beta","+BETA"," gamma"]}]}`
	lines := []string{
		titleLine(sessionA, "Rename"),
		userLine(sessionA, transcriptCWD, "Rename beta"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"Edit","input":{"file_path":"/Users/owner/work/launcher/sample.txt","old_string":"beta","new_string":"BETA"}}`),
		toolResultLine("u-2", "toolu_1", "The file has been updated.", false, toolUseResult),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	change := page.Entries[len(page.Entries)-1]
	if change.Kind != tasktranscript.KindFileChange || len(change.Changes) != 1 {
		t.Fatalf("entry = %#v", change)
	}
	if change.Changes[0].Path != "/Users/owner/work/launcher/sample.txt" || change.Changes[0].Kind != "update" {
		t.Fatalf("change = %#v", change.Changes[0])
	}
	diff := change.Changes[0].Diff
	for _, want := range []string{"@@ -1,3 +1,3 @@", "-beta", "+BETA", " alpha"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
}

func TestWriteIsReportedAsAnAddedFile(t *testing.T) {
	lines := []string{
		titleLine(sessionA, "New file"),
		userLine(sessionA, transcriptCWD, "Create it"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"Write","input":{"file_path":"/Users/owner/work/launcher/new.go"}}`),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	change := page.Entries[len(page.Entries)-1]
	if len(change.Changes) != 1 || change.Changes[0].Kind != "add" {
		t.Fatalf("entry = %#v", change)
	}
}

func TestTodoWriteBecomesThePlan(t *testing.T) {
	input := `{"todos":[{"content":"Read the failure","status":"completed"},{"content":"Fix it","status":"in_progress"},{"content":"Run tests","status":"pending"}]}`
	lines := []string{
		titleLine(sessionA, "Plan"),
		userLine(sessionA, transcriptCWD, "Make a plan"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"TodoWrite","input":`+input+`}`),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	plan := page.Entries[len(page.Entries)-1]
	if plan.Kind != tasktranscript.KindPlan {
		t.Fatalf("entry = %#v", plan)
	}
	for _, want := range []string{"[x] Read the failure", "[~] Fix it", "[ ] Run tests"} {
		if !strings.Contains(plan.Text, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan.Text)
		}
	}
}

// A tool the companion cannot describe precisely gets a plain label, and its
// input is withheld because tool inputs can quote file contents.
func TestUnrecognisedToolsBecomeActivityWithoutLeakingTheirInput(t *testing.T) {
	tests := []struct {
		tool string
		want string
	}{
		{"Read", "Read a file"},
		{"Grep", "Searched the project"},
		{"Task", "Ran a subagent"},
		{"mcp__playwright__browser_click", "Used a connected tool"},
		{"SomeFutureTool", "Used SomeFutureTool"},
	}
	for _, test := range tests {
		lines := []string{
			titleLine(sessionA, "Work"),
			userLine(sessionA, transcriptCWD, "Do it"),
			assistantLine("a-1", fmt.Sprintf(`{"type":"tool_use","id":"toolu_1","name":%q,"input":{"file_path":"/etc/shadow","secret":"private content"}}`, test.tool)),
		}
		page := readTranscript(t, lines, tasktranscript.PageOptions{})
		activity := page.Entries[len(page.Entries)-1]
		if activity.Kind != tasktranscript.KindActivity || activity.Text != test.want {
			t.Fatalf("%s entry = %#v; want %q", test.tool, activity, test.want)
		}
		if strings.Contains(activity.Text, "private content") || strings.Contains(activity.Command, "private content") {
			t.Fatalf("%s leaked its input: %#v", test.tool, activity)
		}
	}
}

// A subagent transcript is a second conversation the phone cannot show, so it
// must not be interleaved into the owner's.
func TestSubagentRecordsAreNotInterleavedIntoTheTranscript(t *testing.T) {
	sidechain := fmt.Sprintf(`{"type":"assistant","uuid":"s-1","sessionId":%q,"cwd":%q,"isSidechain":true,"timestamp":"2026-07-29T12:00:03.000Z","message":{"role":"assistant","content":[{"type":"text","text":"subagent chatter"}]}}`, sessionA, transcriptCWD)
	lines := []string{
		titleLine(sessionA, "Work"),
		userLine(sessionA, transcriptCWD, "Do it"),
		sidechain,
		assistantLine("a-1", `{"type":"text","text":"Done."}`),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	for _, entry := range page.Entries {
		if strings.Contains(entry.Text, "subagent chatter") {
			t.Fatalf("subagent record reached the transcript: %#v", page.Entries)
		}
	}
	if len(page.Entries) != 2 {
		t.Fatalf("entries = %#v", page.Entries)
	}
}

// An output with no matching call is not something the owner can interpret.
func TestOrphanToolResultIsDropped(t *testing.T) {
	lines := []string{
		titleLine(sessionA, "Work"),
		userLine(sessionA, transcriptCWD, "Do it"),
		toolResultLine("u-2", "toolu_missing", "orphan output", false, ""),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	if len(page.Entries) != 1 {
		t.Fatalf("orphan result produced entries: %#v", page.Entries)
	}
}

func TestBookkeepingRecordsNeverReachThePhone(t *testing.T) {
	lines := []string{
		`{"type":"queue-operation","operation":"enqueue","sessionId":"` + sessionA + `"}`,
		`{"type":"file-history-snapshot","messageId":"m-1","snapshot":{"secret":"private"}}`,
		`{"type":"last-prompt","lastPrompt":"private prompt","sessionId":"` + sessionA + `"}`,
		`{"type":"attachment","attachment":{"type":"deferred_tools_delta"},"sessionId":"` + sessionA + `"}`,
		titleLine(sessionA, "Work"),
		userLine(sessionA, transcriptCWD, "Do it"),
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	if len(page.Entries) != 1 || page.Entries[0].Kind != tasktranscript.KindUser {
		t.Fatalf("bookkeeping leaked: %#v", page.Entries)
	}
}

func TestTranscriptPagesBackwardsWithAStableCursor(t *testing.T) {
	lines := []string{titleLine(sessionA, "Long"), userLine(sessionA, transcriptCWD, "Start")}
	for index := range 10 {
		lines = append(lines, assistantLine(fmt.Sprintf("a-%d", index), fmt.Sprintf(`{"type":"text","text":"reply %d"}`, index)))
	}
	root := t.TempDir()
	writeSession(t, root, sessionA, transcriptCWD, lines...)
	catalog := newCatalog(t, root, approveAll)

	newest, err := catalog.ReadTranscript(context.Background(), sessionA, tasktranscript.PageOptions{Limit: 4})
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	if len(newest.Entries) != 4 || newest.Entries[3].Text != "reply 9" {
		t.Fatalf("newest page = %#v", newest.Entries)
	}
	if newest.EarlierCursor == "" {
		t.Fatal("newest page offered no earlier cursor")
	}

	earlier, err := catalog.ReadTranscript(context.Background(), sessionA, tasktranscript.PageOptions{Limit: 4, BeforeEntryID: newest.EarlierCursor})
	if err != nil {
		t.Fatalf("earlier page error = %v", err)
	}
	if len(earlier.Entries) != 4 || earlier.Entries[3].Text != "reply 5" {
		t.Fatalf("earlier page = %#v", earlier.Entries)
	}
	// Entry IDs must be stable across reads or the cursor would drift.
	repeat, err := catalog.ReadTranscript(context.Background(), sessionA, tasktranscript.PageOptions{Limit: 4})
	if err != nil || repeat.Entries[0].ID != newest.Entries[0].ID {
		t.Fatalf("entry IDs are not stable across reads: %v", err)
	}
}

func TestFirstPageStopsOfferingACursorAtTheStart(t *testing.T) {
	lines := []string{titleLine(sessionA, "Short"), userLine(sessionA, transcriptCWD, "Only prompt")}
	page := readTranscript(t, lines, tasktranscript.PageOptions{Limit: 10})
	if page.EarlierCursor != "" {
		t.Fatalf("a complete transcript offered an earlier cursor: %q", page.EarlierCursor)
	}
}

// A stale cursor must be reported, not silently answered with the newest page,
// or the phone would look like the history jumped.
func TestUnknownCursorIsReported(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, sessionA, transcriptCWD, titleLine(sessionA, "Work"), userLine(sessionA, transcriptCWD, "Do it"))
	_, err := newCatalog(t, root, approveAll).ReadTranscript(context.Background(), sessionA, tasktranscript.PageOptions{BeforeEntryID: "no-such-entry.0"})
	if err != tasktranscript.ErrUnknownCursor {
		t.Fatalf("error = %v; want ErrUnknownCursor", err)
	}
}

func TestTranscriptOfAnUnapprovedSessionIsReportedAsMissing(t *testing.T) {
	root := t.TempDir()
	secret := "/Users/owner/private/taxes"
	writeSession(t, root, sessionB, secret, titleLine(sessionB, "Taxes"), userLine(sessionB, secret, "Do my taxes"))
	catalog := newCatalog(t, root, ApproverFunc(func(string) (string, bool) { return "", false }))
	if _, err := catalog.ReadTranscript(context.Background(), sessionB, tasktranscript.PageOptions{}); err != ErrSessionNotFound {
		t.Fatalf("error = %v; want ErrSessionNotFound", err)
	}
}

func TestPageLimitIsCappedAtTheProtocolMaximum(t *testing.T) {
	lines := []string{titleLine(sessionA, "Long"), userLine(sessionA, transcriptCWD, "Start")}
	for index := range tasktranscript.MaxPageEntries + 20 {
		lines = append(lines, assistantLine(fmt.Sprintf("a-%d", index), fmt.Sprintf(`{"type":"text","text":"reply %d"}`, index)))
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{Limit: 10000})
	if len(page.Entries) > tasktranscript.MaxPageEntries {
		t.Fatalf("page returned %d entries; cap is %d", len(page.Entries), tasktranscript.MaxPageEntries)
	}
}

func TestToolResultBlockListContentIsRendered(t *testing.T) {
	// Some tools return content as a block list rather than a bare string.
	result := fmt.Sprintf(`{"type":"user","uuid":"u-2","sessionId":%q,"cwd":%q,"isSidechain":false,"timestamp":"2026-07-29T12:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}],"is_error":false}]}}`, sessionA, transcriptCWD)
	lines := []string{
		titleLine(sessionA, "Work"),
		userLine(sessionA, transcriptCWD, "Do it"),
		assistantLine("a-1", `{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"echo"}}`),
		result,
	}
	page := readTranscript(t, lines, tasktranscript.PageOptions{})
	command := page.Entries[len(page.Entries)-1]
	if command.Output != "first\nsecond" {
		t.Fatalf("output = %q", command.Output)
	}
}
