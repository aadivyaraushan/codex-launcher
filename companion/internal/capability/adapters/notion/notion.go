// Package notion is Operator's RT-1 proving adapter for Notion's hosted MCP
// server. It never holds a static API key — Notion's own documentation says
// its MCP server "requires user-based OAuth authentication and does not
// support bearer token authentication" — so every session here carries a
// live OAuth access token instead.
//
// The adapter's ceiling is measured per connected account, not claimed once
// for everyone: notion-search needs Notion AI, so an account without it can
// still be found and opened, but it cannot be searched, and this adapter
// says so rather than pretending otherwise.
package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ID identifies this adapter in the registry.
const ID = "notion"

// ServerURL is Notion's hosted MCP endpoint.
const ServerURL = "https://mcp.notion.com/mcp"

// ContainerTitle is the title of the page this adapter creates and owns.
// Every unattended write lands underneath this page, never into a page the
// user made themselves.
const ContainerTitle = "Operator"

// Tool names this adapter calls, as exposed by Notion's hosted MCP server.
const (
	ToolSearch      = "notion-search"
	ToolFetch       = "notion-fetch"
	ToolCreatePages = "notion-create-pages"
	ToolUpdatePage  = "notion-update-page"
)

// Errors this adapter returns.
var (
	// ErrBearerNotSupported is returned by NewWithAPIKey: Notion's hosted
	// MCP server does not accept a static bearer token, so there is no key
	// to hold and nothing worth trying.
	ErrBearerNotSupported = errors.New("notion: bearer/API-key auth is not supported by Notion's MCP server; use OAuth")

	// ErrNotConnected is returned by Resolve, Preview and Execute before
	// Connect has run. The tool list Connect fetches is what tells us which
	// half of the adapter exists on this account; acting before we have it
	// is guessing.
	ErrNotConnected = errors.New("notion: adapter is not connected; call Connect first")

	// ErrAmbiguousPage is returned when a search matches two or more pages.
	// Similar page titles are exactly what makes this hard, and picking one
	// is a wrong-page failure the user never sees coming.
	ErrAmbiguousPage = errors.New("notion: more than one page matches; ask the user which one")

	// ErrForeignPage is returned when a write names a page other than this
	// adapter's own container. This adapter only ever writes into a page it
	// created and owns.
	ErrForeignPage = errors.New("notion: refusing to write into a page this adapter does not own")
)

// Session is what this adapter needs from an MCP connection: the tool names
// the connected account's server exposes, and the ability to call one of
// them. A real connection is HTTPSession, in session.go; tests use a fake.
type Session interface {
	ListTools(context.Context) ([]string, error)
	Call(context.Context, string, map[string]any) (json.RawMessage, error)
}

// Adapter is Operator's Notion capability adapter.
type Adapter struct {
	session Session

	connected   bool
	tools       map[string]bool
	containerID string
}

var _ adapter.Adapter = (*Adapter)(nil)

// New builds an adapter over an already-authenticated session. Nothing is
// called yet — Connect measures the account's actual ceiling.
func New(s Session) (*Adapter, error) {
	if s == nil {
		return nil, errors.New("notion: session must not be nil")
	}
	return &Adapter{session: s}, nil
}

// NewWithAPIKey always refuses: Notion's MCP server does not support bearer
// token authentication, so accepting a key here would quietly create a
// secret with no benefit to hold it for.
func NewWithAPIKey(key string) (*Adapter, error) {
	return nil, ErrBearerNotSupported
}

// Describe returns this adapter's manifest. The ceiling here is the
// adapter's best case; the per-account truth comes from Connect.
func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID:       ID,
		Runtime:  manifest.RT1,
		Verbs:    []manifest.Verb{manifest.Read, manifest.Write},
		Ceiling:  manifest.Completes,
		Consent:  manifest.ConsentA,
		Auth:     manifest.AuthOAuth,
		Cost:     manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"},
		// android, not both: no iPhone has run any part of this yet.
		Platform:      manifest.PlatformAndroid,
		ProvesCeiling: "notion_read_roundtrip_smoke",
	}
}

// Connect fetches the connected account's tool list and measures the
// ceiling for THAT account: with notion-search present the account can
// discover pages on its own and reaches Completes; without it the adapter
// can still open Notion with the query but cannot find the page itself, so
// it reports HandsOff instead of pretending otherwise. The result is
// clamped to the manifest's declared ceiling so a server advertising extra
// tools can never promote the adapter above what it claims.
func (a *Adapter) Connect(ctx context.Context) (manifest.Ceiling, error) {
	names, err := a.session.ListTools(ctx)
	if err != nil {
		return "", fmt.Errorf("notion: %w", err)
	}

	tools := make(map[string]bool, len(names))
	for _, n := range names {
		tools[n] = true
	}

	measured := manifest.HandsOff
	if tools[ToolSearch] {
		measured = manifest.Completes
	}

	a.tools = tools
	a.connected = true
	return a.Describe().Ceiling.AtMost(measured), nil
}

// Resolve turns an intent into a plan, dispatching on verb.
func (a *Adapter) Resolve(ctx context.Context, intent adapter.Intent) (adapter.Plan, error) {
	if !a.connected {
		return adapter.Plan{}, ErrNotConnected
	}
	switch intent.Verb {
	case manifest.Read:
		return a.resolveRead(ctx, intent)
	case manifest.Write:
		return a.resolveWrite(ctx, intent)
	default:
		return adapter.Plan{}, fmt.Errorf("notion: verb %q is not supported", intent.Verb)
	}
}

// resolveRead always goes through notion-search with the user's own words;
// an id is never hardcoded. Zero results is an honest failure, not an empty
// read. Two or more results is ErrAmbiguousPage: real data is messier than a
// test page, and similar titles are exactly what makes this hard.
func (a *Adapter) resolveRead(ctx context.Context, intent adapter.Intent) (adapter.Plan, error) {
	hits, err := a.search(ctx, intent.Subject)
	if err != nil {
		return adapter.Plan{}, err
	}
	switch len(hits) {
	case 0:
		return adapter.Plan{}, fmt.Errorf("notion: no page matches %q", intent.Subject)
	case 1:
		h := hits[0]
		return adapter.Plan{
			AdapterID: ID,
			Verb:      manifest.Read,
			Handle:    h.ID,
			Summary:   fmt.Sprintf("Read %q", h.Title),
			Details:   map[string]string{"page_id": h.ID, "title": h.Title},
		}, nil
	default:
		return adapter.Plan{}, &adapter.ClarificationError{Question: "Which matching Notion page did you mean?", Cause: ErrAmbiguousPage}
	}
}

// resolveWrite refuses to target any page but this adapter's own container.
// With no explicit page named, the write targets the container: find it by
// title, or create it if this is the first write ever made. An explicit
// page_id is only ever legitimate if it names the container this adapter
// already resolved — anything else is a write into a page the user made,
// and gets no write call at all.
func (a *Adapter) resolveWrite(ctx context.Context, intent adapter.Intent) (adapter.Plan, error) {
	if explicit, ok := intent.Fields["page_id"]; ok && explicit != "" {
		if a.containerID == "" || explicit != a.containerID {
			return adapter.Plan{}, ErrForeignPage
		}
	} else {
		containerID, err := a.findOrCreateContainer(ctx)
		if err != nil {
			return adapter.Plan{}, err
		}
		a.containerID = containerID
	}

	title := intent.Subject
	if title == "" {
		title = "Note"
	}

	return adapter.Plan{
		AdapterID: ID,
		Verb:      manifest.Write,
		Handle:    a.containerID,
		Summary:   fmt.Sprintf("Write %q under %s", title, ContainerTitle),
		Details: map[string]string{
			"container_id": a.containerID,
			"title":        title,
			"body":         intent.Body,
		},
	}, nil
}

// findOrCreateContainer searches for the adapter's own container page by
// title, and creates it the first time there isn't one yet.
func (a *Adapter) findOrCreateContainer(ctx context.Context) (string, error) {
	hits, err := a.search(ctx, ContainerTitle)
	if err != nil {
		return "", err
	}
	for _, h := range hits {
		if h.Title == ContainerTitle {
			return h.ID, nil
		}
	}

	raw, err := a.session.Call(ctx, ToolCreatePages, map[string]any{"title": ContainerTitle})
	if err != nil {
		return "", fmt.Errorf("notion: %s: %w", ToolCreatePages, err)
	}
	page, err := parseCreatedPage(raw)
	if err != nil {
		return "", err
	}
	return page.ID, nil
}

// search wraps a notion-search call and its reply parsing in one place,
// since both a read and an unattended write need it.
func (a *Adapter) search(ctx context.Context, query string) ([]hit, error) {
	raw, err := a.session.Call(ctx, ToolSearch, map[string]any{"query": query})
	if err != nil {
		return nil, fmt.Errorf("notion: %s: %w", ToolSearch, err)
	}
	return parseSearchResults(raw)
}

// Preview renders a plan for the user before an irreversible verb runs. It
// never calls a tool that writes.
func (a *Adapter) Preview(ctx context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if !a.connected {
		return adapter.Preview{}, ErrNotConnected
	}
	switch plan.Verb {
	case manifest.Write:
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Write to %s in Notion", ContainerTitle),
			Lines:    []string{plan.Details["body"]},
			Confirm:  "Write to Notion",
		}, nil
	case manifest.Read:
		return adapter.Preview{
			Plan:     plan,
			Headline: plan.Summary,
			Lines:    []string{plan.Summary},
			Confirm:  "Read from Notion",
		}, nil
	default:
		return adapter.Preview{}, fmt.Errorf("notion: verb %q is not supported", plan.Verb)
	}
}

// Execute carries out a plan, dispatching on verb. Any call error is
// returned as an error alongside a Done-false, no-hand-off outcome — a
// failure is never dressed up as success or as a hand-off to another app.
func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if !a.connected {
		return adapter.Outcome{}, ErrNotConnected
	}
	switch plan.Verb {
	case manifest.Read:
		return a.executeRead(ctx, plan)
	case manifest.Write:
		return a.executeWrite(ctx, plan)
	default:
		return adapter.Outcome{}, fmt.Errorf("notion: verb %q is not supported", plan.Verb)
	}
}

func (a *Adapter) executeRead(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	raw, err := a.session.Call(ctx, ToolFetch, map[string]any{"page_id": plan.Details["page_id"]})
	if err != nil {
		return adapter.Outcome{Done: false}, fmt.Errorf("notion: %s: %w", ToolFetch, err)
	}
	fetched, err := parseFetchedPage(raw)
	if err != nil {
		return adapter.Outcome{Done: false}, err
	}
	return adapter.Outcome{
		Reached: manifest.Completes,
		Done:    true,
		Detail:  fetched.Content,
	}, nil
}

func (a *Adapter) executeWrite(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	args := map[string]any{
		"parent":  map[string]any{"page_id": plan.Details["container_id"]},
		"title":   plan.Details["title"],
		"content": plan.Details["body"],
	}
	raw, err := a.session.Call(ctx, ToolCreatePages, args)
	if err != nil {
		return adapter.Outcome{Done: false}, fmt.Errorf("notion: %s: %w", ToolCreatePages, err)
	}
	page, err := parseCreatedPage(raw)
	if err != nil {
		return adapter.Outcome{Done: false}, err
	}
	return adapter.Outcome{
		Reached: manifest.Completes,
		Done:    true,
		Detail:  fmt.Sprintf("wrote %q to %s", plan.Details["title"], page.Title),
	}, nil
}

// Revoke drops the session, so every call made afterwards is refused with
// ErrNotConnected rather than reaching a now-unauthorized server.
func (a *Adapter) Revoke(ctx context.Context) error {
	if clearer, ok := a.session.(interface{ Clear(context.Context) error }); ok {
		if err := clearer.Clear(ctx); err != nil {
			return err
		}
	}
	a.session = nil
	a.connected = false
	a.tools = nil
	a.containerID = ""
	return nil
}

// ---- reply shapes ----------------------------------------------------------
//
// Parsing here is deliberately tolerant: a missing or unparseable reply
// produces a clear error rather than a panic.

// hit is one result from notion-search.
type hit struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func parseSearchResults(raw json.RawMessage) ([]hit, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("notion: %s: empty reply", ToolSearch)
	}
	var parsed struct {
		Results []hit `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("notion: %s: unexpected reply shape: %w", ToolSearch, err)
	}
	return parsed.Results, nil
}

// createdPage is the reply shape of notion-create-pages.
type createdPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func parseCreatedPage(raw json.RawMessage) (createdPage, error) {
	if len(raw) == 0 {
		return createdPage{}, fmt.Errorf("notion: %s: empty reply", ToolCreatePages)
	}
	var page createdPage
	if err := json.Unmarshal(raw, &page); err != nil {
		return createdPage{}, fmt.Errorf("notion: %s: unexpected reply shape: %w", ToolCreatePages, err)
	}
	if page.ID == "" {
		return createdPage{}, fmt.Errorf("notion: %s: reply named no page id", ToolCreatePages)
	}
	return page, nil
}

// fetchedPage is the reply shape of notion-fetch.
type fetchedPage struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

func parseFetchedPage(raw json.RawMessage) (fetchedPage, error) {
	if len(raw) == 0 {
		return fetchedPage{}, fmt.Errorf("notion: %s: empty reply", ToolFetch)
	}
	var page fetchedPage
	if err := json.Unmarshal(raw, &page); err != nil {
		return fetchedPage{}, fmt.Errorf("notion: %s: unexpected reply shape: %w", ToolFetch, err)
	}
	return page, nil
}
