package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// A gate is the manifest's way of saying "this request needs a checkpoint
// before it runs" — approval from someone, or a billing account that agreed to
// be charged. Every adapter is required to declare its gates: validation
// rejects a manifest with an empty list (manifest.go:437-439), so no adapter
// can stay silent about this the way classes used to stay silent about
// addressing.
//
// Until these tests were written, the declaration was where it stopped.
// Outside its own validator nothing in the product read Gates — not the
// runner, not the flow, not the handler — so an adapter that declared a
// checkpoint was executed exactly like one that declared none, and the
// checkpoint it asked for did not exist. A validator is not an enforcer:
// "unknown gate %q" proves the word is spelled correctly and nothing more.
//
// The one adapter that declares a real gate shows what that cost.
// adapters/maps declares Cost: CostPerCall and Gates: [GateBilling] — every
// call is billed to the owner's cloud account — and the runner called it with
// no billing check at all. It is not registered in the production build today,
// which is the only reason this had not already spent money; the moment it is,
// it spends.
//
// These tests pin the rule that closes it: a declared gate that nothing can
// clear must stop the request. Nothing in this product can clear one — there is
// no approval flow and no billing consent anywhere — so for now every gate
// other than GateNone refuses. That is deliberately the conservative half of
// the feature: refusing to run is recoverable, and a charge on someone's
// account is not.

// gatedRecorder is newRecorder with its declared gates replaced, so a test can
// hand the runner an adapter that asks for a checkpoint.
func gatedRecorder(id string, verb manifest.Verb, gates ...manifest.Gate) *recorder {
	rec := newRecorder(id, []manifest.Verb{verb}, manifest.Completes)
	rec.m.Gates = gates
	return rec
}

// readIntent is the shape the real billing-gated adapter actually has: a read,
// which needs no preview.
func readIntent(id string) adapter.Intent {
	return adapter.Intent{
		AdapterID: id, Verb: manifest.Read,
		Subject: "coffee near me",
	}
}

// gatedPlan builds the plan by hand rather than through run.Resolve. Every
// door refuses a gated adapter, so asking Resolve for a plan first would fail
// in the setup of a test that is trying to check a later door — and would say
// nothing about whether that later door works. Each test here exercises one
// door on its own.
func gatedPlan(id string, verb manifest.Verb) adapter.Plan {
	return adapter.Plan{AdapterID: id, Verb: verb, Summary: "coffee near me"}
}

func TestABillingGatedAdapterIsNotExecuted(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	plan := gatedPlan("maps", manifest.Read)

	if _, err := run.Execute(context.Background(), plan, Confirmation{}); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("ran a request whose billing checkpoint does not exist; got err=%v", err)
	}
}

// The one that actually matters. Refusing *after* calling the adapter would be
// worthless: the charge happens inside the adapter's own Execute, so an error
// returned afterwards is a bill plus an apology.
func TestABillingGatedAdapterNeverReachesItsOwnExecute(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	plan := gatedPlan("maps", manifest.Read)
	_, _ = run.Execute(context.Background(), plan, Confirmation{})

	for _, call := range rec.calls {
		if call == "execute" {
			t.Fatalf("the adapter was called and billed before the gate was checked: calls=%v", rec.calls)
		}
	}
}

// Read needs no preview, so the preview check cannot catch this one. If the
// gate check were placed after it, or made to depend on it, a billing-gated
// read would walk straight past — and a billing-gated read is exactly what the
// real maps adapter is. Whether a gate is enforced must not depend on which
// verb happened to be asked for.
func TestAGatedVerbThatNeedsNoPreviewIsStillRefused(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	plan := gatedPlan("maps", manifest.Read)
	if plan.Verb.RequiresPreview() {
		t.Fatalf("this test is meaningless if %s requires a preview", plan.Verb)
	}

	if _, err := run.Execute(context.Background(), plan, Confirmation{}); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("a gated verb that skips preview was executed anyway; got err=%v", err)
	}
}

// Billing is not the only checkpoint. An approval gate is a different promise
// — a person has to say yes — and nothing can clear that one either.
func TestAnApprovalGatedAdapterIsNotExecuted(t *testing.T) {
	rec := gatedRecorder("payments", manifest.Read, manifest.GateApproval)
	run, _ := runnerWith(t, rec)

	plan := gatedPlan("payments", manifest.Read)

	if _, err := run.Execute(context.Background(), plan, Confirmation{}); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("ran a request needing an approval nobody gave; got err=%v", err)
	}
}

// The refusal has to say which gate and which adapter. "execute failed" sends
// whoever is debugging it to the registry, the verb list and the preview
// machinery, none of which are wrong.
func TestTheRefusalNamesTheGateAndTheAdapter(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	plan := gatedPlan("maps", manifest.Read)

	_, err := run.Execute(context.Background(), plan, Confirmation{})
	if err == nil {
		t.Fatal("expected a refusal, got none")
	}
	if !strings.Contains(err.Error(), "billing") {
		t.Fatalf("refusal does not name the gate: %q", err)
	}
	if !strings.Contains(err.Error(), "maps") {
		t.Fatalf("refusal does not name the adapter: %q", err)
	}
}

// A gate list may hold more than one entry, and GateNone sitting alongside a
// real gate must not read as permission. The strictest entry decides.
func TestGateNoneAlongsideARealGateDoesNotClearIt(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateNone, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	plan := gatedPlan("maps", manifest.Read)

	if _, err := run.Execute(context.Background(), plan, Confirmation{}); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("a GateNone entry was read as clearing a billing gate; got err=%v", err)
	}
}

// Execute is the last door, and for a per-call-cost adapter it is too late.
// The real maps adapter calls the paid API during Resolve — its own runtime
// test asserts "api calls=1, want 1 at resolve" — so a gate that only guards
// Execute lets the billed call happen and then refuses to use the answer. That
// is the worst of both: the charge, and no result. A checkpoint that does not
// stop the thing it exists to stop is not a checkpoint, so all three doors
// into an adapter refuse.
func TestABillingGatedAdapterIsNotEvenResolved(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	if _, err := run.Resolve(context.Background(), readIntent("maps")); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("resolved a gated adapter, which is where it bills; got err=%v", err)
	}
	for _, call := range rec.calls {
		if call == "resolve" {
			t.Fatalf("the adapter was called and billed at resolve: calls=%v", rec.calls)
		}
	}
}

func TestABillingGatedAdapterIsNotPreviewed(t *testing.T) {
	rec := gatedRecorder("maps", manifest.Read, manifest.GateBilling)
	run, _ := runnerWith(t, rec)

	plan := adapter.Plan{AdapterID: "maps", Verb: manifest.Read, Summary: "coffee near me"}
	if _, err := run.Preview(context.Background(), plan); !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("previewed a gated adapter; got err=%v", err)
	}
}

// The control. Without it, "refuse everything" passes every test above and
// breaks the entire product. An adapter that declares no checkpoint must keep
// executing exactly as it always has.
func TestAnUngatedAdapterStillExecutes(t *testing.T) {
	rec := gatedRecorder("sms", manifest.Send, manifest.GateNone)
	run, _ := runnerWith(t, rec)

	plan, err := run.Resolve(context.Background(), sendIntent("sms"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	preview, err := run.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	out, err := run.Execute(context.Background(), plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("an ungated adapter stopped working: %v", err)
	}
	if !out.Done {
		t.Fatalf("an ungated adapter stopped completing: %+v", out)
	}
}
