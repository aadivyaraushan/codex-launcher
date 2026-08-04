package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sequenceTokenSource struct {
	tokens []string
	calls  int
}

func (s *sequenceTokenSource) AccessToken(context.Context) (string, error) {
	token := s.tokens[s.calls]
	s.calls++
	return token, nil
}

func newInitializedHTTPSession(client *http.Client, serverURL, token string) *HTTPSession {
	session := NewHTTPSession(client, serverURL, token)
	session.initialized = true
	session.protocolVersion = "2025-03-26"
	return session
}

func TestHTTPSessionAdvertisesStreamableHTTPResponseTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"notion-fetch"}]}}`)
	}))
	defer server.Close()
	session := newInitializedHTTPSession(server.Client(), server.URL, "oauth-access")
	tools, err := session.ListTools(t.Context())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0] != "notion-fetch" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestHTTPSessionInitializesAndCarriesMCPStreamSession(t *testing.T) {
	initialized := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"Notion","version":"1"}}}`)
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != "session-1" {
				t.Fatalf("initialized notification session id = %q", r.Header.Get("Mcp-Session-Id"))
			}
			initialized = true
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if !initialized || r.Header.Get("Mcp-Session-Id") != "session-1" || r.Header.Get("MCP-Protocol-Version") != "2025-03-26" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"notion-fetch"}]}}`)
		default:
			t.Fatalf("method = %q", request.Method)
		}
	}))
	defer server.Close()
	session := NewHTTPSession(server.Client(), server.URL, "oauth-access")
	tools, err := session.ListTools(t.Context())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0] != "notion-fetch" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestHTTPSessionDecodesEventStreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		var request rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":"+fmt.Sprint(request.ID)+",\"result\":{\"tools\":[{\"name\":\"notion-search\"}]}}\n\n")
	}))
	defer server.Close()
	session := newInitializedHTTPSession(server.Client(), server.URL, "oauth-access")
	tools, err := session.ListTools(t.Context())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0] != "notion-search" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestHTTPSessionDecodesLargeToolListEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "text/event-stream")
		payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"tools":[{"name":"notion-search","description":"%s"}]}}`, request.ID, strings.Repeat("x", 100_000))
		_, _ = io.WriteString(w, "event: message\ndata: "+payload+"\n\n")
	}))
	defer server.Close()
	session := newInitializedHTTPSession(server.Client(), server.URL, "oauth-access")
	tools, err := session.ListTools(t.Context())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0] != "notion-search" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestOAuthHTTPSessionReadsTheCurrentTokenForEveryRequest(t *testing.T) {
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		var request rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[]}}`, request.ID)
	}))
	defer server.Close()
	tokens := &sequenceTokenSource{tokens: []string{"first", "rotated"}}
	session := NewOAuthHTTPSession(server.Client(), server.URL, tokens)
	session.initialized = true
	if _, err := session.ListTools(t.Context()); err != nil {
		t.Fatalf("first ListTools: %v", err)
	}
	if _, err := session.ListTools(t.Context()); err != nil {
		t.Fatalf("second ListTools: %v", err)
	}
	if got, want := strings.Join(authorizations, ","), "Bearer first,Bearer rotated"; got != want {
		t.Fatalf("authorization sequence = %q, want %q", got, want)
	}
}
