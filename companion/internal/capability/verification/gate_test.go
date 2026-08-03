package verification

import (
	"context"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// The unattended path walks straight past the check that stops a charge.
//
// A gate is the manifest's way of saying "this needs a checkpoint first" —
// someone's approval, or a billing account that agreed to be charged. Nothing
// in this product can clear one today, so `execution.Runner` refuses at all
// three of its doors (`runner.go:111`, `:131`, `:181`), and the long comment
// there spells out why every door needs it rather than just the last one: the
// one adapter that declares a billing gate charges during Resolve, so a check
// placed later fires after the money is gone.
//
// `Tier1Runner.Run`, `Tier2Runner.ConnectLoop` and `Tier2Runner.Heartbeat` do
// not go through `execution.Runner` at all. They hold the registry directly and
// call `a.Resolve` and `a.Execute` on the adapter (verification.go:189, :193,
// :237, :241, :270, :275). None of them checks a gate.
//
// This is the worst place to have that hole rather than the safest. These three
// are the *unattended* paths — a nightly probe run and a wake heartbeat, with
// no person watching. A charge on a user-facing request is at least noticed
// when it happens; a charge on a nightly job repeats every night until someone
// reads a bill.
//
// It is latent today: `NewTier1` and `NewTier2` have no production callers, so
// nothing schedules these yet. That is exactly why it is worth closing now.
// Wiring the scheduler is the moment the hole becomes real, and by then the
// person doing the wiring has no reason to look here.
//
// **The fix must not be a second copy of the rule.** `execution` imports
// `verification` (for Telemetry), so `verification` cannot import `execution`
// back — which is why the check was skipped rather than reused. Duplicating
// `checkGate` into this package would leave two copies to drift apart, and this
// codebase's recurring bug is a rule kept in two places that stop agreeing. The
// rule belongs on the manifest itself, which both packages already import:
// `manifest.Manifest.CheckGates() error` returning `manifest.ErrGateNotCleared`,
// with `execution.checkGate` deleted and its callers pointed at the one rule.

// countingStub records every call to the adapter, not just Execute, so a test
// can prove the adapter was never touched at all rather than merely that it did
// not finish.
type countingStub struct {
	m        manifest.Manifest
	resolves int
	executes int
}

func (s *countingStub) Describe() manifest.Manifest { return s.m }

func (s *countingStub) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	s.resolves++
	return adapter.Plan{AdapterID: s.m.ID}, nil
}

func (s *countingStub) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}

func (s *countingStub) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	s.executes++
	return adapter.Outcome{Reached: s.m.Ceiling, Done: true}, nil
}

func (s *countingStub) Revoke(context.Context) error { return nil }

func (s *countingStub) touched() bool { return s.resolves > 0 || s.executes > 0 }

// gated builds an adapter that declares a real checkpoint. GateNone sits
// alongside the billing gate on purpose: a permissive entry next to a real one
// is not permission, and the strictest entry has to decide.
func gated(id string, rt manifest.Runtime, gate manifest.Gate) *countingStub {
	base := adapterFor(id, rt, manifest.Completes).m
	base.Gates = []manifest.Gate{manifest.GateNone, gate}
	return &countingStub{m: base}
}

func ungated(id string, rt manifest.Runtime) *countingStub {
	return &countingStub{m: adapterFor(id, rt, manifest.Completes).m}
}

func regWithCounting(t *testing.T, stubs ...*countingStub) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, s := range stubs {
		if err := reg.Register(s); err != nil {
			t.Fatalf("Register(%s): %v", s.m.ID, err)
		}
	}
	return reg
}

// The nightly probe. Nobody is watching it, and it runs again tomorrow.
func TestATierOneProbeRefusesAnAdapterWithAGateNobodyCanClear(t *testing.T) {
	a := gated("maps", manifest.RT1, manifest.GateBilling)
	runner := NewTier1(regWithCounting(t, a))

	_, err := runner.Run(context.Background(), Probe{AdapterID: "maps", Verb: manifest.Read})

	if !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("Run = %v, want ErrGateNotCleared", err)
	}
	if a.touched() {
		t.Fatalf("the adapter was called anyway: %d resolves, %d executes", a.resolves, a.executes)
	}
}

// The connect loop sends for real, against the user's own account. A gate here
// stops it before anything leaves.
func TestTheConnectLoopRefusesAnAdapterWithAGateNobodyCanClear(t *testing.T) {
	a := gated("whatsapp", manifest.RT5, manifest.GateApproval)
	runner := NewTier2(regWithCounting(t, a))

	_, err := runner.ConnectLoop(context.Background(), "whatsapp", Target{Handle: "self", IsSelf: true})

	if !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("ConnectLoop = %v, want ErrGateNotCleared", err)
	}
	if a.touched() {
		t.Fatalf("the adapter was called anyway: %d resolves, %d executes", a.resolves, a.executes)
	}
}

// The heartbeat is the one that repeats. It runs at every wake, so a billing
// gate it ignores is a charge on a schedule.
func TestTheHeartbeatRefusesAnAdapterWithAGateNobodyCanClear(t *testing.T) {
	a := gated("whatsapp", manifest.RT5, manifest.GateBilling)
	runner := NewTier2(regWithCounting(t, a))

	err := runner.Heartbeat(context.Background(), "whatsapp", manifest.Read)

	if !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("Heartbeat = %v, want ErrGateNotCleared", err)
	}
	if a.touched() {
		t.Fatalf("the adapter was called anyway: %d resolves, %d executes", a.resolves, a.executes)
	}
}

// A gate refusal must not switch the adapter off. Disabling is what a *failed*
// heartbeat means — the session died, so fail the next task fast. A gate is not
// a failure; the adapter is fine and untouched, and disabling it would turn a
// checkpoint nobody has built yet into a permanent kill.
func TestAGateRefusalDoesNotDisableTheAdapter(t *testing.T) {
	a := gated("whatsapp", manifest.RT5, manifest.GateBilling)
	reg := regWithCounting(t, a)
	runner := NewTier2(reg)

	_ = runner.Heartbeat(context.Background(), "whatsapp", manifest.Read)

	if _, err := reg.Get("whatsapp"); err != nil {
		t.Fatalf("a gated adapter must stay registered and enabled, got %v", err)
	}
}

// First control. An adapter with nothing to clear must still run through all
// three, or "refuse everything" passes every test above and the nightly
// verification never runs again.
func TestAnAdapterWithNoGateStillRunsEverywhere(t *testing.T) {
	probe := ungated("slack", manifest.RT1)
	if _, err := NewTier1(regWithCounting(t, probe)).Run(
		context.Background(), Probe{AdapterID: "slack", Verb: manifest.Read},
	); err != nil {
		t.Fatalf("tier-1 probe = %v, want success", err)
	}
	if probe.executes != 1 {
		t.Fatalf("tier-1 executes = %d, want 1", probe.executes)
	}

	connect := ungated("whatsapp", manifest.RT5)
	if _, err := NewTier2(regWithCounting(t, connect)).ConnectLoop(
		context.Background(), "whatsapp", Target{Handle: "self", IsSelf: true},
	); err != nil {
		t.Fatalf("connect loop = %v, want success", err)
	}
	if connect.executes != 1 {
		t.Fatalf("connect loop executes = %d, want 1", connect.executes)
	}

	beat := ungated("signal", manifest.RT5)
	if err := NewTier2(regWithCounting(t, beat)).Heartbeat(
		context.Background(), "signal", manifest.Read,
	); err != nil {
		t.Fatalf("heartbeat = %v, want success", err)
	}
	if beat.executes != 1 {
		t.Fatalf("heartbeat executes = %d, want 1", beat.executes)
	}
}

// Second control, on the rule itself rather than on a runner: a list of
// GateNone is not a gate, and must not start refusing everything.
func TestGateNoneIsNotAGate(t *testing.T) {
	clear := manifest.Manifest{ID: "x", Gates: []manifest.Gate{manifest.GateNone, manifest.GateNone}}
	if err := clear.CheckGates(); err != nil {
		t.Fatalf("CheckGates on an all-clear manifest = %v, want nil", err)
	}

	blocked := manifest.Manifest{ID: "x", Gates: []manifest.Gate{manifest.GateNone, manifest.GateBilling}}
	if err := blocked.CheckGates(); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("CheckGates with one real gate = %v, want ErrGateNotCleared", err)
	}
}

// Third control, and the reason the rule moves rather than gets copied. The
// refusal the user-facing runner gives and the refusal the unattended runner
// gives have to be the same error, or code that handles one silently misses the
// other — `handler.go` already maps this error to the word the phone shows.
func TestBothRunnersRefuseWithTheSameError(t *testing.T) {
	a := gated("maps", manifest.RT1, manifest.GateBilling)
	_, tierErr := NewTier1(regWithCounting(t, a)).Run(
		context.Background(), Probe{AdapterID: "maps", Verb: manifest.Read},
	)

	manifestErr := a.m.CheckGates()

	if !errors.Is(tierErr, manifest.ErrGateNotCleared) || !errors.Is(manifestErr, manifest.ErrGateNotCleared) {
		t.Fatalf("tier-1 = %v, manifest = %v; both must be ErrGateNotCleared", tierErr, manifestErr)
	}
}
