package execution

import (
	"context"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Real traffic is the rot signal, so real traffic has to reach the thing
// that watches for rot.
//
// The verification package can already tell a one-off failure from an
// adapter that has stopped working — it counts consecutive runs that fall
// short of what the adapter's manifest claims and raises an alert at three.
// None of that fires if the code path real users travel writes its result
// straight into the registry and skips the counting, which is what it did.
// A watchdog nothing is wired to is not a watchdog.

// runs the same intent n times through the runner, returning the last error.
func execTimes(t *testing.T, r *Runner, id string, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		plan, err := r.Resolve(ctx, sendIntent(id))
		if err != nil {
			t.Fatalf("Resolve on run %d: %v", i+1, err)
		}
		pv, err := r.Preview(ctx, plan)
		if err != nil {
			t.Fatalf("Preview on run %d: %v", i+1, err)
		}
		if _, err := r.Execute(ctx, plan, pv.Confirmed()); err != nil {
			t.Fatalf("Execute on run %d: %v", i+1, err)
		}
	}
}

func TestRealExecutionsAreCountedTowardsTheShortfallAlert(t *testing.T) {
	// An adapter that claims completes and only ever reaches hands_off is
	// broken, and the third run in a row is when we should be saying so.
	a := newRecorder("uber", []manifest.Verb{manifest.Send}, manifest.HandsOff)
	r, _ := runnerWith(t, a)

	execTimes(t, r, "uber", 3)

	rep, err := r.Report("uber")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if rep.Observations != 3 {
		t.Fatalf("Report saw %d executions, the runner ran 3", rep.Observations)
	}
	if rep.BelowClaim != 3 {
		t.Fatalf("Report counted %d shortfalls, all 3 fell short", rep.BelowClaim)
	}
	if !rep.Alert {
		t.Fatal("three consecutive shortfalls through the real path raised no alert")
	}
}

func TestAnAdapterMeetingItsClaimNeverAlerts(t *testing.T) {
	a := newRecorder("uber", []manifest.Verb{manifest.Send}, manifest.Completes)
	r, _ := runnerWith(t, a)

	execTimes(t, r, "uber", 5)

	rep, err := r.Report("uber")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if rep.BelowClaim != 0 || rep.Alert {
		t.Fatalf("an adapter doing what it claims was flagged: %+v", rep)
	}
}

func TestExecutionStillDemotesTheShownCeiling(t *testing.T) {
	// Routing through the counting must not lose the thing it was already
	// doing: what a run actually reached still lowers what we promise.
	a := newRecorder("uber", []manifest.Verb{manifest.Send}, manifest.HandsOff)
	r, reg := runnerWith(t, a)

	execTimes(t, r, "uber", 1)

	ceiling, proven, err := reg.EffectiveCeiling("uber")
	if err != nil {
		t.Fatalf("EffectiveCeiling: %v", err)
	}
	if ceiling != manifest.HandsOff {
		t.Fatalf("shown ceiling = %s, want hands_off after a run that only reached hands_off", ceiling)
	}
	if !proven {
		t.Error("an adapter that has really run reads as never measured")
	}
}

func TestAskingForTheRecordOfAnAdapterWeDoNotHaveIsAnError(t *testing.T) {
	a := newRecorder("uber", []manifest.Verb{manifest.Send}, manifest.Completes)
	r, _ := runnerWith(t, a)

	if _, err := r.Report("lyft"); err == nil {
		t.Fatal("Report invented a record for an adapter that is not registered")
	}
}
