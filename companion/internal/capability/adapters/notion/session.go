package notion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	client *http.Client
	url    string
	tokens TokenSource

	initializeMu    sync.Mutex
	mu              sync.Mutex
	nextID          int
	initialized     bool
	sessionID       string
	protocolVersion string
}

var _ Session = (*HTTPSession)(nil)

// TokenSource supplies the current OAuth access token. Durable sources may
// refresh and store a rotated token before returning it.
type TokenSource interface {
	AccessToken(context.Context) (string, error)
}

type tokenClearer interface {
	Clear(context.Context) error
}

type staticToken string

func (t staticToken) AccessToken(context.Context) (string, error) { return string(t), nil }

// NewHTTPSession builds a session against serverURL (ServerURL in normal
// use), authenticating every request with accessToken — an OAuth access
// token obtained by a completed OAuth flow elsewhere. A nil client falls
// back to http.DefaultClient.
func NewHTTPSession(client *http.Client, serverURL, accessToken string) *HTTPSession {
	return NewOAuthHTTPSession(client, serverURL, staticToken(accessToken))
}

// NewOAuthHTTPSession builds a session that asks tokens for the current OAuth
// access token before every HTTP request, so an expired token can be refreshed
// without rebuilding the Notion adapter.
func NewOAuthHTTPSession(client *http.Client, serverURL string, tokens TokenSource) *HTTPSession {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPSession{client: client, url: serverURL, tokens: tokens}
}

// Clear removes a durable OAuth record when the token source supports it.
// Static proof tokens have nothing durable to remove.
func (s *HTTPSession) Clear(ctx context.Context) error {
	clearer, ok := s.tokens.(tokenClearer)
	if !ok {
		return nil
	}
	return clearer.Clear(ctx)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	ID     int             `json:"id"`
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
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("notion: initialize MCP session: %w", err)
	}
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
	if err := s.applyBaseHeaders(ctx, req); err != nil {
		return nil, fmt.Errorf("notion: %s: load OAuth token: %w", method, err)
	}
	s.applySessionHeaders(req)

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
	if err := decodeRPCResponse(resp, id, &out); err != nil {
		return nil, fmt.Errorf("notion: %s: decode response: %w", method, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("notion: %s: %s", method, out.Error.Message)
	}
	return out.Result, nil
}

func (s *HTTPSession) ensureInitialized(ctx context.Context) error {
	s.initializeMu.Lock()
	defer s.initializeMu.Unlock()
	s.mu.Lock()
	if s.initialized {
		s.mu.Unlock()
		return nil
	}
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	params := map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "Operator", "version": "1"},
	}
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: "initialize", Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if err := s.applyBaseHeaders(ctx, req); err != nil {
		return fmt.Errorf("load OAuth token for initialize: %w", err)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		return fmt.Errorf("initialize returned %s", response.Status)
	}
	var initialized rpcResponse
	if err := decodeRPCResponse(response, id, &initialized); err != nil {
		response.Body.Close()
		return err
	}
	response.Body.Close()
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(initialized.Result, &result); err != nil {
		return fmt.Errorf("decode initialize result: %w", err)
	}
	s.mu.Lock()
	s.sessionID = response.Header.Get("Mcp-Session-Id")
	s.protocolVersion = result.ProtocolVersion
	if s.protocolVersion == "" {
		s.protocolVersion = "2025-03-26"
	}
	s.mu.Unlock()

	notification, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	if err != nil {
		return err
	}
	notifyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(notification))
	if err != nil {
		return err
	}
	if err := s.applyBaseHeaders(ctx, notifyReq); err != nil {
		return fmt.Errorf("load OAuth token for initialized notification: %w", err)
	}
	s.applySessionHeaders(notifyReq)
	notifyResponse, err := s.client.Do(notifyReq)
	if err != nil {
		return err
	}
	defer notifyResponse.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(notifyResponse.Body, 1<<20))
	if notifyResponse.StatusCode < 200 || notifyResponse.StatusCode >= 300 {
		return fmt.Errorf("initialized notification returned %s", notifyResponse.Status)
	}
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()
	return nil
}

func (s *HTTPSession) applyBaseHeaders(ctx context.Context, req *http.Request) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if s.tokens == nil {
		return nil
	}
	token, err := s.tokens.AccessToken(ctx)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return nil
}

func (s *HTTPSession) applySessionHeaders(req *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", s.sessionID)
	}
	if s.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", s.protocolVersion)
	}
}

func decodeRPCResponse(response *http.Response, requestID int, out *rpcResponse) error {
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return json.NewDecoder(response.Body).Decode(out)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var candidate rpcResponse
		if err := json.Unmarshal([]byte(payload), &candidate); err != nil {
			continue
		}
		if candidate.ID == requestID {
			*out = candidate
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("event stream ended without response id %d", requestID)
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
