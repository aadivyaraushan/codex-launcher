package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
)

// mutatingTools names the notion-* tools that change something in Notion
// when called, as opposed to notion-search/notion-fetch, which only read. A
// lost reply to one of these is ambiguous in a way a lost reply to a read
// never is.
var mutatingTools = map[string]bool{
	ToolCreatePages: true,
	ToolUpdatePage:  true,
}

// HTTPSession is the real MCP session: JSON-RPC 2.0 over HTTP to Notion's
// hosted MCP server, authenticated with a live OAuth access token supplied
// by the caller. It never accepts or stores a static API key — Notion's own
// server refuses bearer tokens, and NewWithAPIKey in notion.go is the place
// that turns that refusal into a compile-time-checked error rather than a
// runtime surprise.
//
// This type is deliberately thin: it knows how to send a JSON-RPC request
// and unwrap MCP's envelope, nothing more. It is not covered by
// notion_test.go, which exercises the adapter entirely through the fake
// Session there.
type HTTPSession struct {
	client      *http.Client
	url         string
	accessToken string

	mu     sync.Mutex
	nextID int
}

var _ Session = (*HTTPSession)(nil)

// NewHTTPSession builds a session against serverURL (ServerURL in normal
// use), authenticating every request with accessToken — an OAuth access
// token obtained by a completed OAuth flow elsewhere. A nil client falls
// back to http.DefaultClient.
func NewHTTPSession(client *http.Client, serverURL, accessToken string) *HTTPSession {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPSession{client: client, url: serverURL, accessToken: accessToken}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// invoke sends one JSON-RPC request over HTTP and returns its result field,
// or an error built from either a transport failure or an RPC-level error.
// mutating and verb tell invoke whether a lost reply to THIS call is
// ambiguous (a write whose request may have already landed) or an ordinary
// failure (a read, which changed nothing) — the caller knows which tool it
// is calling and invoke does not, so it must be told.
func (s *HTTPSession) invoke(ctx context.Context, method string, params any, mutating bool, verb string) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("notion: encode %s request: %w", method, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("notion: build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.accessToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		wrapped := fmt.Errorf("notion: %s: %w", method, err)
		httpMethod := http.MethodGet
		if mutating {
			httpMethod = http.MethodPost
		}
		classified := adapter.ClassifyHTTPFailure(ID, httpMethod, wrapped)
		if unknown, ok := classified.(*adapter.OutcomeUnknownError); ok {
			// The transport call only knows GET vs. POST; the caller knows
			// which MCP tool this was, so restore that as the Verb.
			unknown.Verb = verb
		}
		return nil, classified
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion: %s: server returned %s", method, resp.Status)
	}

	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("notion: %s: decode response: %w", method, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("notion: %s: %s", method, out.Error.Message)
	}
	return out.Result, nil
}

// ListTools calls MCP's tools/list and returns the tool names the connected
// account's server exposes.
func (s *HTTPSession) ListTools(ctx context.Context) ([]string, error) {
	raw, err := s.invoke(ctx, "tools/list", map[string]any{}, false, "tools/list")
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("notion: tools/list: unexpected reply shape: %w", err)
	}
	names := make([]string, 0, len(parsed.Tools))
	for _, t := range parsed.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

// Call invokes one MCP tool by name via tools/call and returns its raw
// result payload.
func (s *HTTPSession) Call(ctx context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	raw, err := s.invoke(ctx, "tools/call", map[string]any{"name": tool, "arguments": args}, mutatingTools[tool], tool)
	if err != nil {
		return nil, err
	}

	// MCP wraps a tool's result in a content envelope; unwrap it when
	// present, but fall back to the raw result for tools that don't use it.
	var parsed struct {
		Content json.RawMessage `json:"content"`
		IsError bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return raw, nil
	}
	if parsed.IsError {
		return nil, fmt.Errorf("notion: tool %s reported an error", tool)
	}
	if len(parsed.Content) > 0 {
		return parsed.Content, nil
	}
	return raw, nil
}
