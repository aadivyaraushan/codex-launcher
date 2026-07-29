package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
)

const (
	sessionA = "11111111-2222-3333-4444-555555555555"
	sessionB = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
)

// approveAll stands in for a companion whose owner approved everything.
var approveAll = ApproverFunc(func(cwd string) (string, bool) { return filepath.Base(cwd), true })

// writeSession writes a session file under the encoded-cwd directory layout the
// CLI uses. The directory name is deliberately a lossy encoding of cwd, which
// is exactly why the catalog must not trust it.
func writeSession(t *testing.T, root, sessionID, cwd string, lines ...string) string {
	t.Helper()
	encoded := strings.NewReplacer("/", "-", "_", "-", ".", "-").Replace(cwd)
	directory := filepath.Join(root, encoded)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func userLine(sessionID, cwd, text string) string {
	return fmt.Sprintf(`{"type":"user","uuid":"u-1","sessionId":%q,"cwd":%q,"isSidechain":false,"timestamp":"2026-07-29T12:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":%q}]}}`, sessionID, cwd, text)
}

func titleLine(sessionID, title string) string {
	return fmt.Sprintf(`{"type":"ai-title","sessionId":%q,"aiTitle":%q}`, sessionID, title)
}

func newCatalog(t *testing.T, root string, approver ProjectApprover) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(root, approver, nil)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestListRecentReportsApprovedSessionsAsTasks(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Fix the build"), titleLine(sessionA, "Fix the build"))

	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
	task := tasks[0]
	if task.ID != sessionA || task.Title != "Fix the build" || task.ProjectLabel != "launcher" {
		t.Fatalf("task = %#v", task)
	}
	if task.Source != taskstate.SourceClaudeCode {
		t.Fatalf("source = %q", task.Source)
	}
	// Claude Code queues a mid-turn message instead of merging it, so a task
	// must never advertise redirect support to the phone.
	if task.CanRedirect {
		t.Fatal("task claimed it could be redirected")
	}
}

// The security rule of the catalog: the launcher must never surface work from a
// folder the owner did not approve.
func TestListRecentHidesSessionsOutsideApprovedProjects(t *testing.T) {
	root := t.TempDir()
	approved := "/Users/owner/work/launcher"
	secret := "/Users/owner/private/taxes"
	writeSession(t, root, sessionA, approved, userLine(sessionA, approved, "Fix the build"), titleLine(sessionA, "Fix the build"))
	writeSession(t, root, sessionB, secret, userLine(sessionB, secret, "Do my taxes"), titleLine(sessionB, "Do my taxes"))

	approver := ApproverFunc(func(cwd string) (string, bool) {
		if cwd == approved {
			return "launcher", true
		}
		return "", false
	})
	tasks, err := newCatalog(t, root, approver).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != sessionA {
		t.Fatalf("unapproved session leaked: %#v", tasks)
	}
}

// The directory name collapses '/', '_', and '.' all to '-', so it cannot be
// decoded back to a path. Approval must use the cwd recorded inside the file.
func TestApprovalUsesTheRecordedCWDNotTheEncodedDirectoryName(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/my_work/site.v2"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Ship it"), titleLine(sessionA, "Ship it"))

	var seen []string
	approver := ApproverFunc(func(candidate string) (string, bool) {
		seen = append(seen, candidate)
		return "site", candidate == cwd
	})
	tasks, err := newCatalog(t, root, approver).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(seen) != 1 || seen[0] != cwd {
		t.Fatalf("approver was asked about %#v; want the recorded cwd %q", seen, cwd)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
}

// A file with no cwd cannot be checked against the approved projects, so it
// must be withheld rather than shown on the strength of its directory name.
func TestSessionWithoutARecordedCWDIsWithheld(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "-Users-owner-work-launcher")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"ai-title","sessionId":%q,"aiTitle":"Mystery"}`, sessionA)
	if err := os.WriteFile(filepath.Join(directory, sessionA+".jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("session without a cwd was listed: %#v", tasks)
	}
}

func TestListRecentOrdersNewestFirstAndHonoursTheLimit(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	ids := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
	}
	base := time.Now().Add(-time.Hour)
	for index, id := range ids {
		path := writeSession(t, root, id, cwd, userLine(id, cwd, "task "+id), titleLine(id, "task "+id))
		stamp := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("limit ignored: %#v", tasks)
	}
	if tasks[0].ID != ids[2] || tasks[1].ID != ids[1] {
		t.Fatalf("order = %q, %q; want newest first", tasks[0].ID, tasks[1].ID)
	}
}

func TestTitleFallsBackToTheOpeningPromptWhenTheCLIHasNotNamedItYet(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "  Investigate   the   flaky test  "))

	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Investigate the flaky test" {
		t.Fatalf("title = %#v", tasks)
	}
}

func TestLongPromptTitleIsTruncated(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, strings.Repeat("word ", 200)))

	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 10)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %#v, %v", tasks, err)
	}
	if runes := []rune(tasks[0].Title); len(runes) > 81 {
		t.Fatalf("title is %d runes: %q", len(runes), tasks[0].Title)
	}
}

// A truncated or corrupt line must not hide the rest of a session.
func TestCorruptLinesAreSkippedWithoutLosingTheSession(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd,
		`{"type":"user","uuid":"u-0","truncated`,
		"not json at all",
		userLine(sessionA, cwd, "Still here"),
		titleLine(sessionA, "Still here"),
	)
	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Still here" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestFilesThatAreNotSessionsAreIgnored(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Real"), titleLine(sessionA, "Real"))
	directory := filepath.Join(root, strings.NewReplacer("/", "-", "_", "-", ".", "-").Replace(cwd))
	for _, name := range []string{"notes.txt", "not-a-uuid.jsonl", ".DS_Store"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(directory, "memory"), 0o700); err != nil {
		t.Fatal(err)
	}
	tasks, err := newCatalog(t, root, approveAll).ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != sessionA {
		t.Fatalf("tasks = %#v", tasks)
	}
}

// A computer with no Claude Code history is an empty catalog, not an error:
// the owner may simply not have run it here yet.
func TestMissingSessionDirectoryIsAnEmptyCatalog(t *testing.T) {
	catalog := newCatalog(t, filepath.Join(t.TempDir(), "never-created"), approveAll)
	tasks, err := catalog.ListRecent(context.Background(), 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("ListRecent() = %#v, %v", tasks, err)
	}
}

func TestLookupFindsAnApprovedSessionAndHidesAnUnapprovedOne(t *testing.T) {
	root := t.TempDir()
	approved := "/Users/owner/work/launcher"
	secret := "/Users/owner/private/taxes"
	writeSession(t, root, sessionA, approved, userLine(sessionA, approved, "Fix"), titleLine(sessionA, "Fix"))
	writeSession(t, root, sessionB, secret, userLine(sessionB, secret, "Taxes"), titleLine(sessionB, "Taxes"))

	catalog := newCatalog(t, root, ApproverFunc(func(cwd string) (string, bool) {
		return "launcher", cwd == approved
	}))
	summary, err := catalog.Lookup(context.Background(), sessionA)
	if err != nil || summary.SessionID != sessionA || summary.CWD != approved {
		t.Fatalf("Lookup() = %#v, %v", summary, err)
	}
	// An unapproved session is reported as missing rather than as forbidden,
	// which keeps its existence from being probeable from the phone.
	if _, err := catalog.Lookup(context.Background(), sessionB); err != ErrSessionNotFound {
		t.Fatalf("Lookup(unapproved) error = %v; want ErrSessionNotFound", err)
	}
	if _, err := catalog.Lookup(context.Background(), "../../etc/passwd"); err != ErrSessionNotFound {
		t.Fatalf("Lookup(traversal) error = %v; want ErrSessionNotFound", err)
	}
}

func TestValidSessionIDRejectsAnythingThatIsNotAUUID(t *testing.T) {
	valid := []string{sessionA, sessionB, "ABCDEF01-2345-6789-ABCD-EF0123456789"}
	for _, value := range valid {
		if !validSessionID(value) {
			t.Fatalf("validSessionID(%q) = false", value)
		}
	}
	// The session ID becomes a task ID on the wire and is joined into a
	// filesystem path, so it is checked strictly.
	invalid := []string{
		"", "short", "../../etc/passwd",
		"11111111-2222-3333-4444-55555555555", // one short
		"11111111-2222-3333-4444-5555555555555",
		"11111111_2222_3333_4444_555555555555",
		"gggggggg-2222-3333-4444-555555555555",
		"11111111-2222-3333-4444-55555555555/",
	}
	for _, value := range invalid {
		if validSessionID(value) {
			t.Fatalf("validSessionID(%q) = true", value)
		}
	}
}

// The sweep runs on every snapshot refresh, so an unchanged file must not be
// re-read each time.
func TestUnchangedSessionsAreNotReReadOnEverySweep(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Fix"), titleLine(sessionA, "Fix"))

	asked := 0
	catalog := newCatalog(t, root, ApproverFunc(func(string) (string, bool) {
		asked++
		return "launcher", true
	}))
	for range 5 {
		if _, err := catalog.ListRecent(context.Background(), 10); err != nil {
			t.Fatalf("ListRecent() error = %v", err)
		}
	}
	if asked != 1 {
		t.Fatalf("session was re-read %d times across five sweeps", asked)
	}
}

func TestAChangedSessionIsReRead(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	path := writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Fix"), titleLine(sessionA, "First"))
	catalog := newCatalog(t, root, approveAll)
	first, err := catalog.ListRecent(context.Background(), 10)
	if err != nil || len(first) != 1 {
		t.Fatalf("first sweep = %#v, %v", first, err)
	}

	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Fix"), titleLine(sessionA, "Renamed"))
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second, err := catalog.ListRecent(context.Background(), 10)
	if err != nil || len(second) != 1 {
		t.Fatalf("second sweep = %#v, %v", second, err)
	}
	if second[0].Title != "Renamed" {
		t.Fatalf("title = %q; want the updated one", second[0].Title)
	}
}

func TestListRecentStopsWhenTheContextIsCancelled(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/owner/work/launcher"
	writeSession(t, root, sessionA, cwd, userLine(sessionA, cwd, "Fix"), titleLine(sessionA, "Fix"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newCatalog(t, root, approveAll).ListRecent(ctx, 10); err == nil {
		t.Fatal("cancelled sweep returned successfully")
	}
}

func TestNewCatalogRequiresItsDependencies(t *testing.T) {
	if _, err := NewCatalog("", approveAll, nil); err == nil {
		t.Fatal("empty root was accepted")
	}
	if _, err := NewCatalog(t.TempDir(), nil, nil); err == nil {
		t.Fatal("missing approver was accepted")
	}
}

func TestRootHonoursTheConfiguredDirectory(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")
	root, err := Root()
	if err != nil || root != filepath.Join("/custom/claude", "projects") {
		t.Fatalf("Root() = %q, %v", root, err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	root, err = Root()
	if err != nil || root != filepath.Join(home, ".claude", "projects") {
		t.Fatalf("Root() = %q, %v", root, err)
	}
}
