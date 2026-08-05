package notion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// A recorded MCP session. Every test runs offline against replies captured
// from the real server's shape, so the only thing that costs a live call is
// changing what we send.
type fakeSession struct {
	tools   []string
	replies map[string]json.RawMessage
	errs    map[string]error
	calls   []call
	clears  int
}

type call struct {
	tool string
	args map[string]any
}

func newSession(tools ...string) *fakeSession {
	return &fakeSession{
		tools:   tools,
		replies: map[string]json.RawMessage{},
		errs:    map[string]error{},
	}
}

func (f *fakeSession) ListTools(context.Context) ([]string, error) { return f.tools, nil }

func (f *fakeSession) Clear(context.Context) error { f.clears++; return nil }

func (f *fakeSession) Call(_ context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	f.calls = append(f.calls, call{tool: tool, args: args})
	if err := f.errs[tool]; err != nil {
		return nil, err
	}
	return f.replies[tool], nil
}

func (f *fakeSession) called(tool string) bool {
	for _, c := range f.calls {
		if c.tool == tool {
			return true
		}
	}
	return false
}

func (f *fakeSession) lastCall(tool string) (call, bool) {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].tool == tool {
			return f.calls[i], true
		}
	}
	return call{}, false
}

// The full tool list a Notion account with AI access exposes, checked
// against Notion's supported-tools documentation on 2026-07-31.
func fullToolset() []string {
	return []string{
		ToolSearch, ToolFetch, ToolCreatePages, ToolUpdatePage,
		"notion-move-pages", "notion-duplicate-page", "notion-create-database",
		"notion-get-users",
	}
}

func searchHit(id, title string) json.RawMessage {
	return json.RawMessage(`{"results":[{"id":"` + id + `","title":"` + title + `"}]}`)
}

func connected(t *testing.T, s *fakeSession) *Adapter {
	t.Helper()
	a, err := New(s)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, err := a.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	return a
}

// ---- what this adapter is -----------------------------------------------

func TestTheManifestDescribesAnOfficialRemoteConnector(t *testing.T) {
	a, err := New(newSession(fullToolset()...))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	m := a.Describe()

	if err := m.Validate(); err != nil {
		t.Fatalf("the adapter's own manifest is invalid: %v", err)
	}
	if m.Runtime != manifest.RT1 {
		t.Errorf("runtime = %s, want RT-1", m.Runtime)
	}
	if m.Consent != manifest.ConsentA {
		t.Errorf("consent = %s, want A; this is Notion's own published connector", m.Consent)
	}
	if m.Auth != manifest.AuthOAuth {
		t.Errorf("auth = %s, want oauth", m.Auth)
	}
	if m.Ceiling != manifest.Completes {
		t.Errorf("ceiling = %s; the manifest carries the adapter's BEST, and the "+
			"per-account truth comes from Connect", m.Ceiling)
	}
	if m.ProvesCeiling == "" {
		t.Error("the manifest names no smoke test, so its ceiling can never be proven")
	}
	if m.Platform != manifest.PlatformAndroid {
		// This is a cloud call made by the companion, so nothing about it is
		// Android-specific. It still says android, because no iPhone has ever
		// run any part of this — there is no iOS client and both iOS probes
		// are deferred. "both" would promise an iOS user a route nobody has
		// walked. Platform stays android until a real iPhone says otherwise.
		t.Errorf("platform = %s, want android until an iPhone has run this", m.Platform)
	}
}

func TestTheServerAddressIsNotionsHostedOne(t *testing.T) {
	if ServerURL != "https://mcp.notion.com/mcp" {
		t.Errorf("ServerURL = %q, want Notion's hosted MCP endpoint", ServerURL)
	}
}

// ---- OAuth only, and holding an API key is the bug ----------------------

func TestAStaticAPIKeyIsRefusedRatherThanTried(t *testing.T) {
	// Checked against Notion's own documentation on 2026-07-31: "Notion MCP
	// requires user-based OAuth authentication and does not support bearer
	// token authentication." So there is no key to hold and nothing to put in
	// .env, and code that accepts one would quietly create a secret we then
	// have to custody for no benefit.
	if _, err := NewWithAPIKey("secret_abc123"); !errors.Is(err, ErrBearerNotSupported) {
		t.Fatalf("an API key was accepted: %v", err)
	}
}

// ---- the ceiling is per connected account, not per adapter --------------

func TestAnAccountWithSearchCanDiscoverAndSoReachesCompletes(t *testing.T) {
	s := newSession(fullToolset()...)
	a, _ := New(s)

	ceiling, err := a.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if ceiling != manifest.Completes {
		t.Errorf("ceiling = %s, want completes", ceiling)
	}
}

func TestAnAccountWithoutSearchCannotDiscoverAndSaysSoInsteadOfPretending(t *testing.T) {
	// notion-search needs Notion AI. Two users on this one adapter, one free
	// and one on Business, genuinely have different ceilings, and the free one
	// must not be shown the Business one's promise.
	s := newSession(ToolFetch, ToolCreatePages, ToolUpdatePage)
	a, _ := New(s)

	ceiling, err := a.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if ceiling.Rank() >= manifest.Completes.Rank() {
		t.Fatalf("an account with no search still claims %s", ceiling)
	}
}

func TestTheMeasuredCeilingCanOnlyLowerTheDeclaredOne(t *testing.T) {
	// A server that advertised everything plus something we do not use must
	// not push the ceiling above what the manifest declares.
	s := newSession(append(fullToolset(), "notion-query-data-sources", "notion-invent-anything")...)
	a, _ := New(s)

	ceiling, _ := a.Connect(context.Background())
	if ceiling.Rank() > a.Describe().Ceiling.Rank() {
		t.Fatalf("the measurement promoted the adapter to %s", ceiling)
	}
}

func TestNothingWorksBeforeConnect(t *testing.T) {
	// The tool list is what tells us which half of the adapter exists on this
	// account. Acting before we have it is guessing.
	a, _ := New(newSession(fullToolset()...))

	if _, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "meeting notes",
	}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Resolve before Connect returned %v", err)
	}
}

// ---- discovery, never a hardcoded id ------------------------------------

func TestFindingAPageGoesThroughSearchRatherThanAKnownID(t *testing.T) {
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = searchHit("page-123", "Weekly meeting notes")
	a := connected(t, s)

	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "meeting notes",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !s.called(ToolSearch) {
		t.Fatal("the adapter produced a plan without searching; that means an id is hardcoded somewhere")
	}
	if plan.Details["page_id"] != "page-123" {
		t.Errorf("the plan points at %q, want the id search returned", plan.Details["page_id"])
	}
	c, _ := s.lastCall(ToolSearch)
	if !strings.Contains(strings.ToLower(c.args["query"].(string)), "meeting notes") {
		t.Errorf("the search query was %v, want the user's words", c.args["query"])
	}
}

func TestSearchFindingNothingIsAnHonestFailureNotAnEmptyRead(t *testing.T) {
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = json.RawMessage(`{"results":[]}`)
	a := connected(t, s)

	if _, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "a page that does not exist",
	}); err == nil {
		t.Fatal("a search with no results still produced a plan")
	}
}

func TestSeveralPagesWithSimilarNamesIsAnAskNotAPick(t *testing.T) {
	// Real data is messier than a test page, and similar titles are exactly
	// what makes this hard. Picking one is a wrong-page failure the user
	// never sees coming.
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = json.RawMessage(
		`{"results":[{"id":"a","title":"Meeting notes"},{"id":"b","title":"Meeting notes 2026"}]}`)
	a := connected(t, s)

	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "meeting notes",
	})
	if !errors.Is(err, ErrAmbiguousPage) {
		t.Fatalf("the adapter picked between two similar pages: %v", err)
	}
	var question *adapter.ClarificationError
	if !errors.As(err, &question) || question.Question == "" {
		t.Fatalf("ambiguous page is not a user question: %T %v", err, err)
	}
}

func TestExactTitleAmongFuzzySearchHitsIsNotAmbiguous(t *testing.T) {
	// Callers: resolveRead / live write→search→fetch proof.
	// Affected API: notion-search may return fuzzy neighbors; exact title wins.
	// User: "Run live Notion read/write proof per plan with durable evidence."
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = json.RawMessage(
		`{"results":[{"id":"near","title":"OP-NOTION-old"},{"id":"exact","title":"OP-NOTION-20260805T222820Z"},{"id":"other","title":"OP notes"}]}`)
	a := connected(t, s)

	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "OP-NOTION-20260805T222820Z",
	})
	if err != nil {
		t.Fatalf("exact title should resolve: %v", err)
	}
	if plan.Details["page_id"] != "exact" {
		t.Fatalf("page_id = %q, want exact", plan.Details["page_id"])
	}
}

// ---- writes go only where the adapter itself made room ------------------

func TestAnUnattendedWriteGoesIntoTheAdaptersOwnPage(t *testing.T) {
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = json.RawMessage(`{"results":[]}`)
	s.replies[ToolCreatePages] = json.RawMessage(`{"id":"own-page","title":"` + ContainerTitle + `"}`)
	a := connected(t, s)
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "verification", Body: "nightly check ok",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, err := a.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Callers: Adapter.Execute write path / proveNotionReadWrite.
	// Affected API: Notion MCP notion-create-pages (parent + pages[]).
	// Schema: pages[].properties.title, pages[].content; reply pages[].id.
	// User: "Run live Notion read/write proof per plan with durable evidence."
	var writeCall call
	var found bool
	for _, c := range s.calls {
		if c.tool != ToolCreatePages {
			continue
		}
		parent, _ := c.args["parent"].(map[string]any)
		if parent != nil && parent["page_id"] == "own-page" {
			writeCall = c
			found = true
		}
	}
	if !found {
		t.Fatal("nothing was written under the adapter's own page")
	}
	pages, ok := writeCall.args["pages"].([]map[string]any)
	if !ok || len(pages) != 1 {
		t.Fatalf("create-pages pages = %#v, want one page object", writeCall.args["pages"])
	}
	props, _ := pages[0]["properties"].(map[string]any)
	if props["title"] != "verification" {
		t.Errorf("write title = %v, want verification", props["title"])
	}
	if pages[0]["content"] != "nightly check ok" {
		t.Errorf("write content = %v", pages[0]["content"])
	}
}

func TestWriteOutcomeNamesTheCreatedPageID(t *testing.T) {
	// Callers: proveNotionReadWrite fetches by page_id (search index lag).
	// User: "Run live Notion read/write proof per plan with durable evidence."
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = json.RawMessage(`{"results":[{"id":"own-page","title":"` + ContainerTitle + `"}]}`)
	s.replies[ToolCreatePages] = json.RawMessage(`{"pages":[{"id":"child-1","title":"verification"}]}`)
	a := connected(t, s)
	ctx := context.Background()
	plan, err := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "verification", Body: "nightly check ok",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.Detail, "page_id=child-1") {
		t.Fatalf("write detail missing page_id: %q", out.Detail)
	}
}

func TestParseCreatedPageAcceptsPagesArrayReply(t *testing.T) {
	page, err := parseCreatedPage(json.RawMessage(
		`[{"type":"text","text":"{\"pages\":[{\"id\":\"pg-1\",\"url\":\"https://notion.so/p/pg-1\",\"properties\":{\"title\":\"Operator\"}}]}"}]`,
	))
	if err != nil {
		t.Fatalf("parseCreatedPage: %v", err)
	}
	if page.ID != "pg-1" {
		t.Errorf("id = %q, want pg-1", page.ID)
	}
}

func TestAWriteAimedAtAPageTheUserMadeIsRefused(t *testing.T) {
	s := newSession(fullToolset()...)
	a := connected(t, s)

	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "notes", Body: "x",
		Fields: map[string]string{"page_id": "someone-elses-page"},
	})
	if !errors.Is(err, ErrForeignPage) {
		t.Fatalf("a write into a user's own page returned %v", err)
	}
	if s.called(ToolCreatePages) || s.called(ToolUpdatePage) {
		t.Fatal("the write went out anyway")
	}
}

// ---- preview and outcome ------------------------------------------------

func TestThePreviewNamesThePageAndTheTextBeforeAnythingIsWritten(t *testing.T) {
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = json.RawMessage(`{"results":[]}`)
	s.replies[ToolCreatePages] = json.RawMessage(`{"id":"own-page","title":"` + ContainerTitle + `"}`)
	a := connected(t, s)
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "router", Body: "two stages",
	})
	before := len(s.calls)

	p, err := a.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	joined := p.Headline + "\n" + strings.Join(p.Lines, "\n")
	if !strings.Contains(joined, "two stages") {
		t.Errorf("the preview does not show what will be written:\n%s", joined)
	}
	if p.Confirm == "" {
		t.Error("the preview has no confirm label")
	}
	for _, c := range s.calls[before:] {
		if c.tool == ToolCreatePages || c.tool == ToolUpdatePage {
			t.Fatalf("previewing wrote something: %s", c.tool)
		}
	}
}

func TestAReadReportsWhatItRead(t *testing.T) {
	// Callers: executeRead. Notion MCP notion-fetch takes `id` (not page_id).
	// Docs: developers.notion.com/guides/mcp (2026-08-06).
	// User: "Run live Notion read/write proof per plan with durable evidence."
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = searchHit("page-123", "Weekly meeting notes")
	s.replies[ToolFetch] = json.RawMessage(`{"id":"page-123","content":"ship Wave 0 on Friday"}`)
	a := connected(t, s)
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "meeting notes"})
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !out.Done || out.Reached != manifest.Completes {
		t.Errorf("outcome = %+v, want completes and done", out)
	}
	if !strings.Contains(out.Detail, "ship Wave 0 on Friday") {
		t.Errorf("the outcome does not carry what was read: %q", out.Detail)
	}
	c, ok := s.lastCall(ToolFetch)
	if !ok {
		t.Fatal("fetch was not called")
	}
	if c.args["id"] != "page-123" {
		t.Fatalf("fetch args = %#v, want id=page-123", c.args)
	}
	if _, has := c.args["page_id"]; has {
		t.Fatalf("fetch still sends page_id: %#v", c.args)
	}
}

func TestParseFetchedPageAcceptsLiveTextTitleShape(t *testing.T) {
	// Live MCP returns title+text (not content). Captured 2026-08-05.
	page, err := parseFetchedPage(json.RawMessage(
		`[{"type":"text","text":"{\"metadata\":{\"type\":\"page\"},\"title\":\"OP-NOTION-X\",\"url\":\"https://app.notion.com/p/x\",\"text\":\"<content>\\nOP-NOTION-X live proof\\n</content>\"}"}]`,
	))
	if err != nil {
		t.Fatalf("parseFetchedPage: %v", err)
	}
	if !strings.Contains(page.Content, "OP-NOTION-X live proof") {
		t.Fatalf("content = %q", page.Content)
	}
	if page.Title != "OP-NOTION-X" {
		t.Fatalf("title = %q", page.Title)
	}
}

func TestAServerErrorIsAFailureNotAHandOff(t *testing.T) {
	s := newSession(fullToolset()...)
	s.replies[ToolSearch] = searchHit("page-123", "Weekly meeting notes")
	s.errs[ToolFetch] = errors.New("rate limited")
	a := connected(t, s)
	ctx := context.Background()

	plan, _ := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "meeting notes"})
	out, err := a.Execute(ctx, plan)
	if err == nil {
		t.Fatal("a server error reported success")
	}
	if out.Done || out.HandedOffTo != "" {
		t.Errorf("a failure was dressed up as %+v", out)
	}
}

// ---- revoke -------------------------------------------------------------

func TestRevokeDropsTheSessionSoNothingElseCanBeCalled(t *testing.T) {
	s := newSession(fullToolset()...)
	a := connected(t, s)
	ctx := context.Background()

	if err := a.Revoke(ctx); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	if s.clears != 1 {
		t.Fatalf("durable credential clears = %d, want 1", s.clears)
	}
	if _, err := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "anything"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("the adapter still works after a revoke: %v", err)
	}
}
