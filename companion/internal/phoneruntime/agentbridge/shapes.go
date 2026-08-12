// Package agentbridge exposes the capability registry to the on-phone
// OpenClaw agent as plain HTTP tools: GET /v1/agent-tools/list translates
// every registered adapter's manifest into a tool descriptor, and
// POST /v1/agent-tools/call drives one intent through the execution runner
// (Resolve → Preview → Execute, previews self-confirmed at this layer —
// the agent-side gates decide what may be called at all).
//
// This file is the wire contract. The OpenClaw plugin (Phase 3) mirrors
// these shapes verbatim; change them in both places or not at all.
package agentbridge

// ToolDescriptor is one adapter presented as an agent tool. The manifest
// has no parameter schema of its own, so InputSchema is invented here from
// the one intent shape every adapter accepts (verb, subject, handle, body,
// fields).
type ToolDescriptor struct {
	// Name is the adapter id, verbatim — it is what ToolCallRequest.Adapter
	// must echo back.
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Verbs       []VerbDescriptor `json:"verbs"`
	// Ceiling is the manifest's declared ceiling ("completes", "one_tap",
	// "hands_off"), so the agent can tell the user how far a call can go.
	Ceiling string `json:"ceiling"`
	// InputSchema is a JSON Schema object describing the call arguments.
	InputSchema map[string]any `json:"inputSchema"`
}

// VerbDescriptor is one verb an adapter offers, with the fact the agent
// must plan around: whether the runner demands a confirmed preview before
// it will execute.
type VerbDescriptor struct {
	Name            string `json:"name"`
	RequiresPreview bool   `json:"requiresPreview"`
}

// ToolListResult is the body of GET /v1/agent-tools/list.
type ToolListResult struct {
	Tools []ToolDescriptor `json:"tools"`
}

// ToolCallRequest is the body of POST /v1/agent-tools/call — an intent in
// wire form.
type ToolCallRequest struct {
	Adapter   string            `json:"adapter"`
	Verb      string            `json:"verb"`
	Subject   string            `json:"subject,omitempty"`
	Handle    string            `json:"handle,omitempty"`
	To        string            `json:"to,omitempty"`
	Recipient string            `json:"recipient,omitempty"`
	ChatID    string            `json:"chat_id,omitempty"`
	Body      string            `json:"body,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	// TurnKey identifies the agent turn this call belongs to, so the gate
	// policy can tell a read-then-send within one turn (exfiltration risk)
	// from the same sequence spread across turns.
	TurnKey string `json:"turnKey,omitempty"`
}

// ToolCallResult is the body of every /call response, success or failure.
// OK true means the adapter executed; the outcome fields mirror
// adapter.Outcome. OK false means nothing irreversible happened and Error
// says why.
type ToolCallResult struct {
	OK          bool            `json:"ok"`
	Reached     string          `json:"reached,omitempty"`
	Done        bool            `json:"done,omitempty"`
	HandedOffTo string          `json:"handedOffTo,omitempty"`
	Detail      string          `json:"detail,omitempty"`
	Preview     *PreviewSummary `json:"preview,omitempty"`
	Error       *CallError      `json:"error,omitempty"`
	// GateID is set only alongside error code approval_required: the id the
	// owner's approve/deny decision must reference.
	GateID string `json:"gateId,omitempty"`
}

// PreviewSummary is what the runner's preview showed before this layer
// confirmed it, echoed back so the turn transcript records what was agreed
// to on the caller's behalf.
type PreviewSummary struct {
	Headline string   `json:"headline"`
	Lines    []string `json:"lines,omitempty"`
	Confirm  string   `json:"confirm,omitempty"`
}

// CallError codes, closed set:
//
//	unauthorized       – bearer token missing or wrong (HTTP 401)
//	bad_request        – body not valid JSON or verb outside the closed set (HTTP 400)
//	unknown_adapter    – no registered adapter under that id (HTTP 404)
//	verb_not_offered   – adapter's manifest does not declare the verb (HTTP 400)
//	adapter_failed     – resolve/preview/execute returned an error (HTTP 502)
//	approval_required  – the hard-gate policy stopped this call for the
//	                     owner's explicit OK (HTTP 200: this is an answer,
//	                     not a transport failure — nothing executed, and
//	                     GateID plus Preview are set so the caller can show
//	                     the owner what it would have done)
type CallError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
