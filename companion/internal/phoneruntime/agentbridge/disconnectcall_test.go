package agentbridge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

// fakeDisconnector records what the bridge asked it to undo. It stands in
// for capability/disconnect's Service, which owns the real two-halves revoke.
type fakeDisconnector struct {
	calls        []string
	disconnected map[string]bool
	err          error
}

func newFakeDisconnector() *fakeDisconnector {
	return &fakeDisconnector{disconnected: map[string]bool{}}
}

func (d *fakeDisconnector) Disconnect(_ context.Context, adapterID string) error {
	d.calls = append(d.calls, adapterID)
	if d.err != nil {
		return d.err
	}
	d.disconnected[adapterID] = true
	return nil
}

func (d *fakeDisconnector) Disconnected(adapterID string) bool {
	return d.disconnected[adapterID]
}

func disconnectBridge(t *testing.T, adapters ...*fake) (*httptest.Server, *agentbridge.Bridge, *notifier, *fakeDisconnector) {
	t.Helper()
	reg := registry.New()
	for _, a := range adapters {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	store := newGateStore()
	serial := 0
	n := &notifier{}
	d := newFakeDisconnector()
	b := agentbridge.New(reg, execution.New(reg), testToken, nil, agentbridge.GateDeps{
		Policy: gates.New(store, func() string {
			serial++
			return "gate-" + string(rune('0'+serial))
		}),
		Store:        store,
		Notifier:     n,
		AllowListed:  func(string) bool { return true },
		Disconnector: d,
	})
	server := httptest.NewServer(b.Handler())
	t.Cleanup(server.Close)
	return server, b, n, d
}

// Disconnecting an app is a revoke — one of the four hard gates. It always
// stops for the owner, allow list or not, and nothing is undone until the
// owner says so.
func TestDisconnectCallAlwaysStopsForApproval(t *testing.T) {
	messages := newFake("beeper.message", manifest.Read, manifest.Send)
	server, _, n, d := disconnectBridge(t, messages)

	resp, result := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "disconnect", TurnKey: "turn-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (approval_required is an envelope answer)", resp.StatusCode)
	}
	if result.OK {
		t.Fatal("a disconnect must not report success before the owner approves it")
	}
	if result.Error == nil || result.Error.Code != "approval_required" {
		t.Fatalf("error = %+v, want code approval_required", result.Error)
	}
	if result.GateID == "" {
		t.Fatal("a gated disconnect must carry the gate id the owner will approve or deny")
	}
	if result.Preview == nil || !strings.Contains(result.Preview.Headline, "beeper.message") {
		t.Fatalf("the owner must be shown which app is being disconnected, got %+v", result.Preview)
	}
	if len(d.calls) != 0 {
		t.Fatalf("disconnect ran %d times before approval, want 0", len(d.calls))
	}
	if len(messages.calls) != 0 {
		t.Fatalf("a disconnect must not drive the adapter's plan path, got %v", messages.calls)
	}
	if len(n.gates) != 1 || n.gates[0].Kind != gates.KindRevoke {
		t.Fatalf("raised gate must be a revoke gate, got %+v", n.gates)
	}
	if n.gates[0].Adapter != "beeper.message" {
		t.Fatalf("raised gate must name the app being disconnected, got %+v", n.gates[0])
	}
}

func TestApprovedDisconnectRevokesExactlyOnce(t *testing.T) {
	messages := newFake("beeper.message", manifest.Send)
	server, bridge, _, d := disconnectBridge(t, messages)

	_, gated := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "disconnect", TurnKey: "turn-1",
	})

	result, err := bridge.ApproveGate(context.Background(), gated.GateID)
	if err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}
	if !result.OK || !result.Done {
		t.Fatalf("an approved disconnect must report done, got %+v", result)
	}
	if len(d.calls) != 1 || d.calls[0] != "beeper.message" {
		t.Fatalf("disconnector calls = %v, want exactly [beeper.message]", d.calls)
	}

	if _, err := bridge.ApproveGate(context.Background(), gated.GateID); err == nil {
		t.Fatal("a gate must release at most once; second approval must fail")
	}
	if len(d.calls) != 1 {
		t.Fatalf("second approval ran the disconnect again: %v", d.calls)
	}
}

func TestDeniedDisconnectNeverRevokes(t *testing.T) {
	messages := newFake("beeper.message", manifest.Send)
	server, bridge, _, d := disconnectBridge(t, messages)

	_, gated := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "disconnect", TurnKey: "turn-1",
	})

	if err := bridge.DenyGate(gated.GateID); err != nil {
		t.Fatalf("DenyGate: %v", err)
	}
	if _, err := bridge.ApproveGate(context.Background(), gated.GateID); err == nil {
		t.Fatal("a denied gate must never release")
	}
	if len(d.calls) != 0 {
		t.Fatalf("disconnect ran %d times after denial, want 0", len(d.calls))
	}
}

// A repeat disconnect is idempotent success, not a fresh owner interruption:
// the connection is already undone, so there is nothing left to approve.
func TestRepeatDisconnectSucceedsWithoutANewGate(t *testing.T) {
	messages := newFake("beeper.message", manifest.Send)
	server, _, n, d := disconnectBridge(t, messages)
	d.disconnected["beeper.message"] = true

	resp, result := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "beeper.message", Verb: "disconnect", TurnKey: "turn-1",
	})
	if resp.StatusCode != http.StatusOK || !result.OK || !result.Done {
		t.Fatalf("repeat disconnect must answer success, got status %d result %+v", resp.StatusCode, result)
	}
	if len(n.gates) != 0 {
		t.Fatalf("repeat disconnect must not raise a new gate, got %+v", n.gates)
	}
	if len(d.calls) != 0 {
		t.Fatalf("repeat disconnect must not run the revoke again, got %v", d.calls)
	}
}

// A typo'd adapter id must not nag the owner with an approval card for an
// app that does not exist.
func TestDisconnectUnknownAdapterIs404AndNeverGates(t *testing.T) {
	messages := newFake("beeper.message", manifest.Send)
	server, _, n, d := disconnectBridge(t, messages)

	resp, result := callResult(t, server, agentbridge.ToolCallRequest{
		Adapter: "no.such.adapter", Verb: "disconnect", TurnKey: "turn-1",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if result.OK || result.Error == nil || result.Error.Code != "unknown_adapter" {
		t.Fatalf("want ok=false error.code=unknown_adapter, got %+v", result)
	}
	if len(n.gates) != 0 {
		t.Fatalf("unknown adapter must not raise a gate, got %+v", n.gates)
	}
	if len(d.calls) != 0 {
		t.Fatalf("unknown adapter must not reach the disconnector, got %v", d.calls)
	}
}

// The agent can only plan with verbs it is offered: every listed tool
// carries disconnect alongside the adapter's own verbs.
func TestToolListOffersDisconnectOnEveryAdapter(t *testing.T) {
	server, _, _, _ := disconnectBridge(t, newFake("beeper.message", manifest.Read, manifest.Send))

	resp, raw := do(t, http.MethodGet, server.URL+"/v1/agent-tools/list", testToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, raw)
	}
	body := string(raw)
	if !strings.Contains(body, `"disconnect"`) {
		t.Fatalf("tool list must offer the disconnect verb, got: %s", body)
	}
}
