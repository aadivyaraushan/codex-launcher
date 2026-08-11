package agentbridge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

// gateStore is the in-memory stand-in for the durablestore-backed gate
// store the runtime wires in.
type gateStore struct {
	known   map[string]bool
	denials map[string]bool
}

func newGateStore() *gateStore {
	return &gateStore{known: map[string]bool{}, denials: map[string]bool{}}
}

func (s *gateStore) KnownRecipient(adapter, recipient string) (bool, error) {
	return s.known[adapter+"\x00"+recipient], nil
}

func (s *gateStore) MarkRecipientMessaged(adapter, recipient string) error {
	s.known[adapter+"\x00"+recipient] = true
	return nil
}

func (s *gateStore) RecordDenial(gateID string) error {
	s.denials[gateID] = true
	return nil
}

func (s *gateStore) WasDenied(gateID string) (bool, error) {
	return s.denials[gateID], nil
}

// notifier records every gate the bridge raises toward the launcher.
type notifier struct {
	gates    []gates.Gate
	previews []agentbridge.PreviewSummary
}

func (n *notifier) GateRaised(gate gates.Gate, preview agentbridge.PreviewSummary) {
	n.gates = append(n.gates, gate)
	n.previews = append(n.previews, preview)
}

func gatedBridge(t *testing.T, store *gateStore, adapters ...*fake) (*httptest.Server, *agentbridge.Bridge, *notifier) {
	t.Helper()
	reg := registry.New()
	for _, a := range adapters {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	serial := 0
	n := &notifier{}
	b := agentbridge.New(reg, execution.New(reg), testToken, nil, agentbridge.GateDeps{
		Policy: gates.New(store, func() string {
			serial++
			return "gate-" + string(rune('0'+serial))
		}),
		Store:    store,
		Notifier: n,
	})
	server := httptest.NewServer(b.Handler())
	t.Cleanup(server.Close)
	return server, b, n
}

func callResult(t *testing.T, server *httptest.Server, req agentbridge.ToolCallRequest) (*http.Response, agentbridge.ToolCallResult) {
	t.Helper()
	resp, raw := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", testToken, callBody(t, req))
	var result agentbridge.ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return resp, result
}

func executes(f *fake) int {
	count := 0
	for _, call := range f.calls {
		if call == "execute" {
			count++
		}
	}
	return count
}

func TestSendToStrangerStopsForApproval(t *testing.T) {
	store := newGateStore()
	messages := newFake("beeper.message", manifest.Read, manifest.Send)
	server, _, n := gatedBridge(t, store, messages)

	resp, result := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send", Handle: "+15550000001", Body: "hi", TurnKey: "turn-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (approval_required is an envelope answer, not a transport failure)", resp.StatusCode)
	}
	if result.OK {
		t.Fatal("a first-ever send must not report success")
	}
	if result.Error == nil || result.Error.Code != "approval_required" {
		t.Fatalf("error = %+v, want code approval_required", result.Error)
	}
	if result.GateID == "" {
		t.Fatal("a gated call must carry the gate id the owner will approve or deny")
	}
	if result.Preview == nil || result.Preview.Headline == "" {
		t.Fatalf("a gated call must echo the preview the owner will be shown, got %+v", result.Preview)
	}
	if got := executes(messages); got != 0 {
		t.Fatalf("adapter executed %d times before approval, want 0", got)
	}
	if known, _ := store.KnownRecipient("beeper.message", "+15550000001"); known {
		t.Fatal("a gated, unexecuted send must not mark the recipient known")
	}
	if len(n.gates) != 1 || n.gates[0].ID != result.GateID {
		t.Fatalf("launcher must be told about exactly this gate, got %+v", n.gates)
	}
	if n.gates[0].Recipient != "+15550000001" || n.gates[0].Adapter != "beeper.message" {
		t.Fatalf("raised gate must name the blocked call, got %+v", n.gates[0])
	}
	if n.previews[0].Headline == "" {
		t.Fatal("raised gate must carry the preview for the owner's sheet")
	}
}

func TestApproveExecutesTheStoredCallExactlyOnce(t *testing.T) {
	store := newGateStore()
	messages := newFake("beeper.message", manifest.Send)
	server, bridge, _ := gatedBridge(t, store, messages)

	_, gated := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send", Handle: "+15550000001", Body: "hi", TurnKey: "turn-1",
	})

	result, err := bridge.ApproveGate(context.Background(), gated.GateID)
	if err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}
	if !result.OK || !result.Done {
		t.Fatalf("approval must run the stored call to completion, got %+v", result)
	}
	if got := executes(messages); got != 1 {
		t.Fatalf("adapter executed %d times, want exactly 1", got)
	}
	if known, _ := store.KnownRecipient("beeper.message", "+15550000001"); !known {
		t.Fatal("an approved, executed send must mark the recipient known")
	}

	if _, err := bridge.ApproveGate(context.Background(), gated.GateID); err == nil {
		t.Fatal("a gate must release at most once; second approval must fail")
	}
	if got := executes(messages); got != 1 {
		t.Fatalf("second approval executed the call again (%d executes)", got)
	}
}

func TestDenyIsDurableAndNeverExecutes(t *testing.T) {
	store := newGateStore()
	messages := newFake("beeper.message", manifest.Send)
	server, bridge, _ := gatedBridge(t, store, messages)

	_, gated := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send", Handle: "+15550000001", Body: "hi", TurnKey: "turn-1",
	})

	if err := bridge.DenyGate(gated.GateID); err != nil {
		t.Fatalf("DenyGate: %v", err)
	}
	if !store.denials[gated.GateID] {
		t.Fatal("a denial must be recorded durably, not just in memory")
	}
	if _, err := bridge.ApproveGate(context.Background(), gated.GateID); err == nil {
		t.Fatal("a denied gate must never release")
	}
	if got := executes(messages); got != 0 {
		t.Fatalf("adapter executed %d times after denial, want 0", got)
	}
}

func TestKnownRecipientSendExecutesWithoutGate(t *testing.T) {
	store := newGateStore()
	if err := store.MarkRecipientMessaged("beeper.message", "+15550000001"); err != nil {
		t.Fatal(err)
	}
	messages := newFake("beeper.message", manifest.Send)
	server, _, n := gatedBridge(t, store, messages)

	resp, result := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send", Handle: "+15550000001", Body: "hi", TurnKey: "turn-1",
	})
	if resp.StatusCode != http.StatusOK || !result.OK || !result.Done {
		t.Fatalf("send to a known recipient must execute directly, got status %d result %+v", resp.StatusCode, result)
	}
	if result.GateID != "" || result.Error != nil {
		t.Fatalf("ungated call must not carry gate fields, got %+v", result)
	}
	if got := executes(messages); got != 1 {
		t.Fatalf("adapter executed %d times, want 1", got)
	}
	if len(n.gates) != 0 {
		t.Fatalf("no gate should reach the launcher, got %+v", n.gates)
	}
}

func TestCrossAdapterReadThenSendIsGated(t *testing.T) {
	store := newGateStore()
	if err := store.MarkRecipientMessaged("beeper.message", "+15550000001"); err != nil {
		t.Fatal(err)
	}
	messages := newFake("beeper.message", manifest.Send)
	discord := newFake("beeper.discord", manifest.Read)
	server, _, _ := gatedBridge(t, store, messages, discord)

	_, read := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.discord", Verb: "read", Subject: "inbox", TurnKey: "turn-1",
	})
	if !read.OK {
		t.Fatalf("read must pass, got %+v", read)
	}

	_, send := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send", Handle: "+15550000001", Body: "hi", TurnKey: "turn-1",
	})
	if send.OK || send.Error == nil || send.Error.Code != "approval_required" {
		t.Fatalf("send after reading another adapter this turn must gate even to a known recipient, got %+v", send)
	}
	if got := executes(messages); got != 0 {
		t.Fatalf("adapter executed %d times, want 0", got)
	}
}

func TestReadInEarlierTurnDoesNotGateNextTurnSend(t *testing.T) {
	store := newGateStore()
	if err := store.MarkRecipientMessaged("beeper.message", "+15550000001"); err != nil {
		t.Fatal(err)
	}
	messages := newFake("beeper.message", manifest.Send)
	discord := newFake("beeper.discord", manifest.Read)
	server, _, _ := gatedBridge(t, store, messages, discord)

	if _, read := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.discord", Verb: "read", Subject: "inbox", TurnKey: "turn-1",
	}); !read.OK {
		t.Fatalf("read must pass, got %+v", read)
	}

	_, send := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "send", Handle: "+15550000001", Body: "hi", TurnKey: "turn-2",
	})
	if !send.OK || !send.Done {
		t.Fatalf("a read in an earlier turn must not gate this turn's send, got %+v", send)
	}
}

func TestApprovingUnknownGateIDFails(t *testing.T) {
	store := newGateStore()
	_, bridge, _ := gatedBridge(t, store, newFake("beeper.message", manifest.Send))

	if _, err := bridge.ApproveGate(context.Background(), "gate-that-never-existed"); err == nil {
		t.Fatal("approving a gate id that was never issued must fail")
	}
}
