package agentbridge

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// adapterLister is the one thing the bridge needs from a registry: the ids
// it should describe in the tool list. Both *registry.Registry and
// capability/runtime's Inventory satisfy it, so production wiring can hand
// the bridge either the raw registry (tests) or the read-only Inventory
// view (production.go) without this package importing more than it uses.
type adapterLister interface {
	AdapterIDs() []string
}

// Bridge serves the two agent-tool endpoints for one registry and runner.
// Every request must carry the bearer token; the caller (phoneruntime)
// additionally restricts the routes to loopback.
type Bridge struct {
	ids    adapterLister
	runner *execution.Runner
	token  string
	logger *slog.Logger
}

// New returns a Bridge. token must be non-empty; a Bridge with an empty
// token refuses every request rather than serving unauthenticated.
func New(ids adapterLister, runner *execution.Runner, token string, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bridge{ids: ids, runner: runner, token: token, logger: logger}
}

// Handler serves GET /v1/agent-tools/list and POST /v1/agent-tools/call.
func (b *Bridge) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/agent-tools/list":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			b.handleList(w, r)
		case "/v1/agent-tools/call":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			b.handleCall(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// authorized checks the bearer token in constant time. An empty configured
// token is a misconfiguration, not open access, so it refuses everything —
// including a request presenting an empty bearer of its own.
func (b *Bridge) authorized(r *http.Request) bool {
	if b.token == "" {
		return false
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) < len(prefix) || header[:len(prefix)] != prefix {
		return false
	}
	given := header[len(prefix):]
	return given != "" && subtle.ConstantTimeCompare([]byte(given), []byte(b.token)) == 1
}

func (b *Bridge) handleList(w http.ResponseWriter, _ *http.Request) {
	var tools []ToolDescriptor
	for _, id := range b.ids.AdapterIDs() {
		m, err := b.runner.Describe(id)
		if err != nil {
			// A disabled or otherwise unreachable adapter is left off the
			// list rather than failing the whole listing.
			continue
		}
		tools = append(tools, describeTool(m))
	}
	writeJSON(w, http.StatusOK, ToolListResult{Tools: tools})
}

// describeTool turns one adapter's manifest into the wire descriptor the
// agent plans against. The manifest carries no parameter schema of its
// own, so InputSchema is invented here from the one intent shape every
// adapter accepts.
func describeTool(m manifest.Manifest) ToolDescriptor {
	verbs := make([]VerbDescriptor, 0, len(m.Verbs))
	verbNames := make([]any, 0, len(m.Verbs))
	for _, v := range m.Verbs {
		verbs = append(verbs, VerbDescriptor{Name: string(v), RequiresPreview: v.RequiresPreview()})
		verbNames = append(verbNames, string(v))
	}
	description := "Adapter " + m.ID + "; verbs: "
	for i, v := range m.Verbs {
		if i > 0 {
			description += ", "
		}
		description += string(v)
	}
	return ToolDescriptor{
		Name:        m.ID,
		Description: description,
		Verbs:       verbs,
		Ceiling:     string(m.Ceiling),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"verb": map[string]any{
					"type":        "string",
					"description": "Which verb to invoke on " + m.ID + ".",
					"enum":        verbNames,
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Unresolved subject (a contact name, a place, a track title).",
				},
				"handle": map[string]any{
					"type":        "string",
					"description": "Device-resolved handle (phone number, place id, URI).",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Free-text body for the verb, when it takes one.",
				},
				"fields": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
					"description":          "Any additional named fields the verb needs.",
				},
			},
			"required": []any{"verb"},
		},
	}
}

func (b *Bridge) handleCall(w http.ResponseWriter, r *http.Request) {
	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.logCall(req.Adapter, req.Verb, "bad_request")
		writeJSON(w, http.StatusBadRequest, ToolCallResult{OK: false, Error: &CallError{Code: "bad_request", Message: "body is not valid JSON"}})
		return
	}

	verb, err := manifest.ParseVerb(req.Verb)
	if err != nil {
		b.logCall(req.Adapter, req.Verb, "bad_request")
		writeJSON(w, http.StatusBadRequest, ToolCallResult{OK: false, Error: &CallError{Code: "bad_request", Message: "unknown verb"}})
		return
	}

	intent := adapter.Intent{
		AdapterID: req.Adapter,
		Verb:      verb,
		Subject:   req.Subject,
		Handle:    req.Handle,
		Body:      req.Body,
		Fields:    req.Fields,
	}

	plan, err := b.runner.Resolve(r.Context(), intent)
	if err != nil {
		b.respondCallError(w, req.Adapter, req.Verb, err)
		return
	}

	preview, err := b.runner.Preview(r.Context(), plan)
	if err != nil {
		b.respondCallError(w, req.Adapter, req.Verb, err)
		return
	}

	// Every call runs the preview step and self-confirms it — the agent-side
	// gates (Phase 3) decide what may be called at all, not a second
	// confirmation round-trip through this bridge.
	out, err := b.runner.Execute(r.Context(), plan, preview.Confirmed())
	if err != nil {
		b.respondCallError(w, req.Adapter, req.Verb, err)
		return
	}

	b.logCall(req.Adapter, req.Verb, "ok")
	writeJSON(w, http.StatusOK, ToolCallResult{
		OK:          true,
		Reached:     string(out.Reached),
		Done:        out.Done,
		HandedOffTo: out.HandedOffTo,
		Detail:      out.Detail,
		Preview: &PreviewSummary{
			Headline: preview.Headline,
			Lines:    preview.Lines,
			Confirm:  preview.Confirm,
		},
	})
}

// respondCallError maps a resolve/preview/execute error onto the closed set
// of CallError codes and writes the ToolCallResult envelope even on
// failure, since the OpenClaw plugin decodes that envelope on every path.
func (b *Bridge) respondCallError(w http.ResponseWriter, adapterID, verb string, err error) {
	var status int
	var code string
	switch {
	case errors.Is(err, registry.ErrUnknownAdapter), errors.Is(err, registry.ErrAdapterDisabled):
		status, code = http.StatusNotFound, "unknown_adapter"
	case errors.Is(err, execution.ErrVerbNotOffered):
		status, code = http.StatusBadRequest, "verb_not_offered"
	default:
		status, code = http.StatusBadGateway, "adapter_failed"
	}
	b.logCall(adapterID, verb, code)
	writeJSON(w, status, ToolCallResult{OK: false, Error: &CallError{Code: code, Message: err.Error()}})
}

// logCall records one line per call — adapter, verb, and outcome only.
// Never the token, the call body, or a handle: those can carry a phone
// number or message text, exactly what this line must not leak into logs.
func (b *Bridge) logCall(adapterID, verb, outcome string) {
	b.logger.Info("[agent-bridge] call", "adapter", adapterID, "verb", verb, "outcome", outcome)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
