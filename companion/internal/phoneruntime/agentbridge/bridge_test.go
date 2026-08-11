package agentbridge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

const testToken = "agentbridge-test-token-0123456789abcdef"

// ungatedStore is the fake gates.Store used by these tests: every recipient
// they exercise is pre-marked known, so the calls under test reach the
// adapter directly and this file's assertions about resolve/preview/execute
// stay unchanged by the gate policy this bridge now always runs.
type ungatedStore struct {
	known map[string]bool
}

func newUngatedStore() *ungatedStore {
	return &ungatedStore{known: map[string]bool{}}
}

func (s *ungatedStore) KnownRecipient(adapter, recipient string) (bool, error) {
	return s.known[adapter+"\x00"+recipient], nil
}

func (s *ungatedStore) MarkRecipientMessaged(adapter, recipient string) error {
	s.known[adapter+"\x00"+recipient] = true
	return nil
}

func (s *ungatedStore) RecordDenial(string) error      { return nil }
func (s *ungatedStore) WasDenied(string) (bool, error) { return false, nil }

// testGateDeps builds a GateDeps whose store already knows every recipient
// these bridge tests send to, so none of them observe gating.
func testGateDeps() agentbridge.GateDeps {
	store := newUngatedStore()
	store.known["beeper.message\x00+15550000000"] = true
	serial := 0
	return agentbridge.GateDeps{
		Policy: gates.New(store, func() string {
			serial++
			return "gate-" + string(rune('0'+serial))
		}),
		Store: store,
	}
}

// fake is a recording adapter (same shape as execution's runner_test
// recorder) so a test can assert what the bridge actually drove.
type fake struct {
	m     manifest.Manifest
	calls []string
}

func (f *fake) Describe() manifest.Manifest { return f.m }

func (f *fake) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	f.calls = append(f.calls, "resolve")
	return adapter.Plan{
		AdapterID: f.m.ID,
		Verb:      in.Verb,
		Handle:    in.Handle,
		Summary:   "send \"" + in.Body + "\" to " + in.Handle,
	}, nil
}

func (f *fake) Preview(_ context.Context, p adapter.Plan) (adapter.Preview, error) {
	f.calls = append(f.calls, "preview")
	return adapter.Preview{Plan: p, Headline: p.Summary, Lines: []string{"to: " + p.Handle}, Confirm: "Send"}, nil
}

func (f *fake) Execute(_ context.Context, _ adapter.Plan) (adapter.Outcome, error) {
	f.calls = append(f.calls, "execute")
	return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: "sent"}, nil
}

func (f *fake) Revoke(context.Context) error { return nil }

func newFake(id string, verbs ...manifest.Verb) *fake {
	return &fake{m: manifest.Manifest{
		ID: id, Runtime: manifest.RT4, Verbs: verbs,
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthDevice, Cost: manifest.CostFree,
		Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region: []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: id + "-smoke",
	}}
}

func bridgeWith(t *testing.T, adapters ...adapter.Adapter) (*httptest.Server, []*fake) {
	t.Helper()
	reg := registry.New()
	var fakes []*fake
	for _, a := range adapters {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if f, ok := a.(*fake); ok {
			fakes = append(fakes, f)
		}
	}
	b := agentbridge.New(reg, execution.New(reg), testToken, nil, testGateDeps())
	server := httptest.NewServer(b.Handler())
	t.Cleanup(server.Close)
	return server, fakes
}

func do(t *testing.T, method, url, token, body string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw := make([]byte, 0, 1024)
	buf := make([]byte, 1024)
	for {
		n, readErr := resp.Body.Read(buf)
		raw = append(raw, buf[:n]...)
		if readErr != nil {
			break
		}
	}
	return resp, raw
}

func callBody(t *testing.T, req agentbridge.ToolCallRequest) string {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// ---- auth ---------------------------------------------------------------

func TestListRefusesMissingOrWrongToken(t *testing.T) {
	server, _ := bridgeWith(t, newFake("beeper.message", manifest.Read, manifest.Send))

	for name, token := range map[string]string{"missing": "", "wrong": "not-the-token"} {
		resp, raw := do(t, http.MethodGet, server.URL+"/v1/agent-tools/list", token, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s token: status = %d, want 401", name, resp.StatusCode)
		}
		if strings.Contains(string(raw), "beeper.message") {
			t.Fatalf("%s token: unauthorized body leaked adapter ids: %s", name, raw)
		}
	}
}

func TestCallRefusesMissingTokenWithoutTouchingAdapter(t *testing.T) {
	server, fakes := bridgeWith(t, newFake("beeper.message", manifest.Send))

	body := callBody(t, agentbridge.ToolCallRequest{Adapter: "beeper.message", Verb: "send", Handle: "+15550000000", Body: "hi"})
	resp, _ := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", "", body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if len(fakes[0].calls) != 0 {
		t.Fatalf("adapter was called without auth: %v", fakes[0].calls)
	}
}

func TestEmptyTokenBridgeRefusesEverything(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(newFake("beeper.message", manifest.Read)); err != nil {
		t.Fatal(err)
	}
	b := agentbridge.New(reg, execution.New(reg), "", nil, testGateDeps())
	server := httptest.NewServer(b.Handler())
	defer server.Close()

	// Even a request presenting an empty bearer must be refused: an empty
	// configured token is a misconfiguration, not open access.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/agent-tools/list", nil)
	req.Header.Set("Authorization", "Bearer ")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// ---- list ---------------------------------------------------------------

func TestListDescribesEveryRegisteredAdapter(t *testing.T) {
	server, _ := bridgeWith(t,
		newFake("beeper.message", manifest.Read, manifest.Send),
		newFake("gcal.event", manifest.Read),
	)

	resp, raw := do(t, http.MethodGet, server.URL+"/v1/agent-tools/list", testToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var result agentbridge.ToolListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("list body not JSON: %v; body: %s", err, raw)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2; body: %s", len(result.Tools), raw)
	}

	byName := map[string]agentbridge.ToolDescriptor{}
	for _, tool := range result.Tools {
		byName[tool.Name] = tool
	}
	beeper, ok := byName["beeper.message"]
	if !ok {
		t.Fatalf("beeper.message missing from list: %s", raw)
	}
	if beeper.Ceiling != "completes" {
		t.Fatalf("Ceiling = %q, want completes", beeper.Ceiling)
	}
	if beeper.Description == "" {
		t.Fatal("Description empty")
	}
	verbs := map[string]bool{}
	for _, v := range beeper.Verbs {
		verbs[v.Name] = v.RequiresPreview
	}
	if preview, ok := verbs["read"]; !ok || preview {
		t.Fatalf("read verb: present=%v requiresPreview=%v, want present, no preview", ok, preview)
	}
	if preview, ok := verbs["send"]; !ok || !preview {
		t.Fatalf("send verb: present=%v requiresPreview=%v, want present with preview", ok, preview)
	}

	// The invented input schema must name the intent fields and pin the
	// verb to what this adapter actually offers.
	props, ok := beeper.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("InputSchema.properties missing: %#v", beeper.InputSchema)
	}
	for _, field := range []string{"verb", "subject", "handle", "body", "fields"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("InputSchema.properties missing %q: %#v", field, props)
		}
	}
	verbProp, ok := props["verb"].(map[string]any)
	if !ok {
		t.Fatalf("verb property not an object: %#v", props["verb"])
	}
	enum, ok := verbProp["enum"].([]any)
	if !ok || len(enum) != 2 {
		t.Fatalf("verb enum = %#v, want the adapter's two verbs", verbProp["enum"])
	}
}

// ---- call ---------------------------------------------------------------

func TestCallDrivesResolvePreviewExecuteInOrder(t *testing.T) {
	server, fakes := bridgeWith(t, newFake("beeper.message", manifest.Send))

	body := callBody(t, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send",
		Subject: "Maya K", Handle: "+15550000000", Body: "running ten minutes late",
	})
	resp, raw := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", testToken, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var result agentbridge.ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("call body not JSON: %v; body: %s", err, raw)
	}
	if !result.OK {
		t.Fatalf("OK = false: %s", raw)
	}
	if result.Reached != "completes" || !result.Done {
		t.Fatalf("Reached=%q Done=%v, want completes/true", result.Reached, result.Done)
	}
	if result.Preview == nil || result.Preview.Headline == "" {
		t.Fatalf("send call must echo the self-confirmed preview: %s", raw)
	}
	got := strings.Join(fakes[0].calls, ",")
	if got != "resolve,preview,execute" {
		t.Fatalf("adapter calls = %q, want resolve,preview,execute", got)
	}
}

func TestCallUnknownAdapterIs404(t *testing.T) {
	server, _ := bridgeWith(t, newFake("beeper.message", manifest.Send))

	body := callBody(t, agentbridge.ToolCallRequest{Adapter: "no.such.adapter", Verb: "send", Handle: "x", Body: "y"})
	resp, raw := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", testToken, body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", resp.StatusCode, raw)
	}
	var result agentbridge.ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("error body not JSON: %v; body: %s", err, raw)
	}
	if result.OK || result.Error == nil || result.Error.Code != "unknown_adapter" {
		t.Fatalf("want ok=false error.code=unknown_adapter, got: %s", raw)
	}
}

func TestCallVerbNotOfferedNeverTouchesAdapter(t *testing.T) {
	server, fakes := bridgeWith(t, newFake("beeper.message", manifest.Read))

	body := callBody(t, agentbridge.ToolCallRequest{Adapter: "beeper.message", Verb: "send", Handle: "x", Body: "y"})
	resp, raw := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", testToken, body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", resp.StatusCode, raw)
	}
	var result agentbridge.ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("error body not JSON: %v; body: %s", err, raw)
	}
	if result.OK || result.Error == nil || result.Error.Code != "verb_not_offered" {
		t.Fatalf("want ok=false error.code=verb_not_offered, got: %s", raw)
	}
	if len(fakes[0].calls) != 0 {
		t.Fatalf("adapter was called for an unoffered verb: %v", fakes[0].calls)
	}
}

func TestCallRejectsBadJSONAndUnknownVerb(t *testing.T) {
	server, fakes := bridgeWith(t, newFake("beeper.message", manifest.Send))

	for name, body := range map[string]string{
		"not json":     "{nope",
		"unknown verb": `{"adapter":"beeper.message","verb":"detonate"}`,
	} {
		resp, raw := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", testToken, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400; body: %s", name, resp.StatusCode, raw)
		}
		var result agentbridge.ToolCallResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("%s: error body not JSON: %v; body: %s", name, err, raw)
		}
		if result.OK || result.Error == nil || result.Error.Code != "bad_request" {
			t.Fatalf("%s: want ok=false error.code=bad_request, got: %s", name, raw)
		}
	}
	if len(fakes[0].calls) != 0 {
		t.Fatalf("adapter was called on a bad request: %v", fakes[0].calls)
	}
}

func TestMethodsArePinnedPerRoute(t *testing.T) {
	server, _ := bridgeWith(t, newFake("beeper.message", manifest.Read))

	resp, _ := do(t, http.MethodPost, server.URL+"/v1/agent-tools/list", testToken, "{}")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST list: status = %d, want 405", resp.StatusCode)
	}
	resp, _ = do(t, http.MethodGet, server.URL+"/v1/agent-tools/call", testToken, "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET call: status = %d, want 405", resp.StatusCode)
	}
}
