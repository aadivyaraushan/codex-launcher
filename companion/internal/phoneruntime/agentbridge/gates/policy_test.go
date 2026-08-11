package gates

import (
	"fmt"
	"testing"
)

// fakeStore is the in-memory stand-in for the durablestore-backed gate store.
// The real store persists known recipients and denials across restarts; the
// policy only ever talks to this interface.
type fakeStore struct {
	known   map[string]bool
	denials map[string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{known: map[string]bool{}, denials: map[string]bool{}}
}

func recipientKey(adapter, recipient string) string {
	return adapter + "\x00" + recipient
}

func (s *fakeStore) KnownRecipient(adapter, recipient string) (bool, error) {
	return s.known[recipientKey(adapter, recipient)], nil
}

func (s *fakeStore) MarkRecipientMessaged(adapter, recipient string) error {
	s.known[recipientKey(adapter, recipient)] = true
	return nil
}

func (s *fakeStore) RecordDenial(gateID string) error {
	s.denials[gateID] = true
	return nil
}

func (s *fakeStore) WasDenied(gateID string) (bool, error) {
	return s.denials[gateID], nil
}

func newTestPolicy(store Store) *Policy {
	serial := 0
	return New(store, func() string {
		serial++
		return fmt.Sprintf("gate-%d", serial)
	})
}

func TestReadAllowsAndNeverGates(t *testing.T) {
	policy := newTestPolicy(newFakeStore())
	decision, err := policy.Evaluate(CallFacts{Adapter: "instagram", Verb: "read", TurnKey: "turn-1"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow || decision.Gate != nil {
		t.Fatalf("read must be allowed ungated, got %+v", decision)
	}
}

func TestFirstMessageToNewRecipientIsGated(t *testing.T) {
	policy := newTestPolicy(newFakeStore())
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Allow || decision.Gate == nil {
		t.Fatalf("first-ever send must stop for the owner's OK, got %+v", decision)
	}
	if decision.Gate.Kind != KindFirstContact {
		t.Fatalf("gate kind = %q, want %q", decision.Gate.Kind, KindFirstContact)
	}
	if decision.Gate.Adapter != "instagram" || decision.Gate.Recipient != "maya" {
		t.Fatalf("gate must name the call it blocks, got %+v", decision.Gate)
	}
}

func TestKnownRecipientPassesUngated(t *testing.T) {
	store := newFakeStore()
	if err := store.MarkRecipientMessaged("instagram", "maya"); err != nil {
		t.Fatal(err)
	}
	policy := newTestPolicy(store)
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow || decision.Gate != nil {
		t.Fatalf("send to an already-messaged recipient must pass, got %+v", decision)
	}
}

func TestApprovalReleasesExactlyOnce(t *testing.T) {
	policy := newTestPolicy(newFakeStore())
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	gateID := decision.Gate.ID

	released, ok := policy.Approve(gateID)
	if !ok {
		t.Fatal("first approval of a pending gate must release it")
	}
	if released.ID != gateID || released.Recipient != "maya" {
		t.Fatalf("approval must release exactly the gated call, got %+v", released)
	}

	if _, ok := policy.Approve(gateID); ok {
		t.Fatal("a gate must release at most once; second approval must fail")
	}
}

func TestUnknownGateIDDoesNotRelease(t *testing.T) {
	policy := newTestPolicy(newFakeStore())
	if _, ok := policy.Approve("gate-that-never-existed"); ok {
		t.Fatal("approving a gate id that was never issued must fail")
	}
}

func TestDenialIsDurableAndDeniedGateNeverReleases(t *testing.T) {
	store := newFakeStore()
	policy := newTestPolicy(store)
	facts := CallFacts{Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1"}

	decision, err := policy.Evaluate(facts)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	gateID := decision.Gate.ID

	if err := policy.Deny(gateID); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if !store.denials[gateID] {
		t.Fatal("a denial must be recorded in the persistent store, not just memory")
	}
	if _, ok := policy.Approve(gateID); ok {
		t.Fatal("a denied gate must never release")
	}

	// The agent may still want to act; it must re-present a fresh gate, and
	// the denial must not have quietly allow-listed the recipient.
	again, err := policy.Evaluate(facts)
	if err != nil {
		t.Fatalf("Evaluate after deny: %v", err)
	}
	if again.Allow || again.Gate == nil {
		t.Fatalf("retry after denial must gate again, got %+v", again)
	}
	if again.Gate.ID == gateID {
		t.Fatal("retry after denial must issue a fresh gate id")
	}
}

func TestReadOtherAdapterThenSendIsGatedEvenToKnownRecipient(t *testing.T) {
	store := newFakeStore()
	if err := store.MarkRecipientMessaged("instagram", "maya"); err != nil {
		t.Fatal(err)
	}
	policy := newTestPolicy(store)

	// The prompt-injection path: this turn read from a different adapter,
	// then tries an outbound send — even to an allow-listed recipient.
	if _, err := policy.Evaluate(CallFacts{Adapter: "drive", Verb: "read", TurnKey: "turn-1"}); err != nil {
		t.Fatalf("read: %v", err)
	}
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Allow || decision.Gate == nil || decision.Gate.Kind != KindExfiltration {
		t.Fatalf("cross-adapter read-then-send must gate as exfiltration, got %+v", decision)
	}
}

func TestSendAfterReadingOnlyOwnAdapterPasses(t *testing.T) {
	store := newFakeStore()
	if err := store.MarkRecipientMessaged("instagram", "maya"); err != nil {
		t.Fatal(err)
	}
	policy := newTestPolicy(store)

	if _, err := policy.Evaluate(CallFacts{Adapter: "instagram", Verb: "read", TurnKey: "turn-1"}); err != nil {
		t.Fatalf("read: %v", err)
	}
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("read-then-send within one adapter must pass, got %+v", decision)
	}
}

func TestCrossAdapterReadInDifferentTurnDoesNotGate(t *testing.T) {
	store := newFakeStore()
	if err := store.MarkRecipientMessaged("instagram", "maya"); err != nil {
		t.Fatal(err)
	}
	policy := newTestPolicy(store)

	if _, err := policy.Evaluate(CallFacts{Adapter: "drive", Verb: "read", TurnKey: "turn-1"}); err != nil {
		t.Fatalf("read: %v", err)
	}
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-2",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("a read in an earlier turn must not gate this turn's send, got %+v", decision)
	}
}

func TestMissingTurnKeyIsTreatedAsOneSharedTurn(t *testing.T) {
	store := newFakeStore()
	if err := store.MarkRecipientMessaged("instagram", "maya"); err != nil {
		t.Fatal(err)
	}
	policy := newTestPolicy(store)

	// Without turn attribution the policy must err toward gating: all
	// unattributed calls count as one turn.
	if _, err := policy.Evaluate(CallFacts{Adapter: "drive", Verb: "read"}); err != nil {
		t.Fatalf("read: %v", err)
	}
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Allow || decision.Gate == nil || decision.Gate.Kind != KindExfiltration {
		t.Fatalf("unattributed cross-adapter read-then-send must still gate, got %+v", decision)
	}
}

func TestRevokeIsAlwaysGated(t *testing.T) {
	policy := newTestPolicy(newFakeStore())
	decision, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "cancel", Revoke: true, TurnKey: "turn-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Allow || decision.Gate == nil || decision.Gate.Kind != KindRevoke {
		t.Fatalf("revoking or disconnecting a service must stop for the owner's OK, got %+v", decision)
	}
}

func TestIrreversibleOutsideAllowListIsGated(t *testing.T) {
	store := newFakeStore()
	if err := store.MarkRecipientMessaged("instagram", "maya"); err != nil {
		t.Fatal(err)
	}
	policy := newTestPolicy(store)

	gated, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-1",
		Irreversible: true, AllowListed: false,
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if gated.Allow || gated.Gate == nil || gated.Gate.Kind != KindIrreversible {
		t.Fatalf("irreversible action outside the allow-list must gate, got %+v", gated)
	}

	allowed, err := policy.Evaluate(CallFacts{
		Adapter: "instagram", Verb: "send", Recipient: "maya", TurnKey: "turn-2",
		Irreversible: true, AllowListed: true,
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !allowed.Allow {
		t.Fatalf("irreversible action inside the rules-file allow-list must pass, got %+v", allowed)
	}
}
