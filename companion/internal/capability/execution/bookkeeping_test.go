package execution

import (
	"context"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// Telling someone their message failed when it was actually sent.
//
// Execute does the real, irreversible thing and then records what happened for
// the verification stats:
//
//	if err := r.tel.Observe(plan.AdapterID, out.Reached); err != nil {
//	    return out, err          // runner.go:205-207
//	}
//
// Observe only reads the registry and writes a measured ceiling
// (verification.go:326-334). It touches nothing in the outside world. It can
// still fail — most plausibly a kill-switch race, where the adapter is
// unregistered while a run is in flight.
//
// When that happens the caller sees a non-nil error and throws the outcome
// away (flow/service.go:174-177 returns a zero adapter.Outcome), and the phone
// is told "failed". The message is already sent. The reminder is already
// created. Told it failed, someone sends it again.
//
// Bookkeeping is not the work. A failure to write down what happened must
// never be reported as a failure of the thing itself.

// unregisteringRecorder unregisters itself from the registry the moment its
// Execute runs — the kill-switch race, made deterministic.
type unregisteringRecorder struct {
	*recorder
	reg *registry.Registry
}

func (u *unregisteringRecorder) Execute(ctx context.Context, p adapter.Plan) (adapter.Outcome, error) {
	out, err := u.recorder.Execute(ctx, p)
	_ = u.reg.Unregister(u.m.ID)
	return out, err
}

func TestBookkeepingThatFailsAfterTheWorkIsDoneStillReportsTheWork(t *testing.T) {
	rec := newRecorder("sms", []manifest.Verb{manifest.Send}, manifest.Completes)
	reg := registry.New()
	a := &unregisteringRecorder{recorder: rec, reg: reg}
	if err := reg.Register(a); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	run := New(reg)

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
		t.Fatalf("a send that went through was reported as an error: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done {
		t.Fatalf("the outcome the adapter actually produced was lost: got %+v", out)
	}
}

// The guard against over-correcting: a real refusal must still be a refusal.
// Only bookkeeping is being downgraded here, nothing else.
func TestAnAdapterThatRefusesIsStillAnError(t *testing.T) {
	rec := newRecorder("sms", []manifest.Verb{manifest.Send}, manifest.Completes)
	rec.execErr = context.Canceled
	run, _ := runnerWith(t, rec)

	plan, err := run.Resolve(context.Background(), sendIntent("sms"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	preview, err := run.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if _, err := run.Execute(context.Background(), plan, preview.Confirmed()); err == nil {
		t.Fatal("an adapter that failed outright must still surface as an error")
	}
}
