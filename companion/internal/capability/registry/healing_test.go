package registry

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// How a demoted adapter is allowed to recover.
//
// The existing rule — a measurement can lower the shown ceiling but never
// raise it above what the manifest claimed — says nothing about how a
// demotion ends. Recording the latest measurement and nothing else means one
// lucky run erases a demotion, so an adapter that works half the time reads
// as fully working the moment it happens to succeed.
//
// The rule these tests fix is deliberately lopsided:
//
//	going DOWN is immediate      one bad run is enough to stop promising
//	going UP needs repeating     one good run is not enough to start again
//
// The asymmetry is the point. Under-promising costs a user a hand-off they
// did not need. Over-promising tells them something happened when it did
// not, which is the failure this whole product is arranged to avoid.
const healRuns = 2

func TestOneBadRunDemotesImmediately(t *testing.T) {
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)

	if ceiling, _, _ := r.EffectiveCeiling("uber"); ceiling != manifest.HandsOff {
		t.Fatalf("effective ceiling = %s, want hands_off after one shortfall", ceiling)
	}
}

func TestOneGoodRunDoesNotUndoADemotion(t *testing.T) {
	// The judge's case: an adapter demotes, then has a single clean run and
	// reads as fully healed. A user acting on that is being told a flaky
	// route is reliable on the strength of one sample.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)
	_ = r.RecordMeasuredCeiling("uber", manifest.Completes)

	ceiling, _, _ := r.EffectiveCeiling("uber")
	if ceiling != manifest.HandsOff {
		t.Fatalf("effective ceiling = %s, want hands_off; one good run undid the demotion", ceiling)
	}
}

func TestARepeatedGoodRunDoesUndoADemotion(t *testing.T) {
	// A demotion that can never be undone is a delete. A vendor that fixes
	// its API has to be able to get its ceiling back.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)
	for i := 0; i < healRuns; i++ {
		_ = r.RecordMeasuredCeiling("uber", manifest.Completes)
	}

	ceiling, proven, _ := r.EffectiveCeiling("uber")
	if ceiling != manifest.Completes {
		t.Fatalf("effective ceiling = %s, want completes after %d clean runs", ceiling, healRuns)
	}
	if !proven {
		t.Error("a healed adapter reads as never measured")
	}
}

func TestASecondShortfallRestartsTheCountingFromScratch(t *testing.T) {
	// This is what separates "flaky" from "recovering". Good, bad, good is
	// not two good runs — it is an adapter still failing.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)
	_ = r.RecordMeasuredCeiling("uber", manifest.Completes)
	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)
	_ = r.RecordMeasuredCeiling("uber", manifest.Completes)

	ceiling, _, _ := r.EffectiveCeiling("uber")
	if ceiling != manifest.HandsOff {
		t.Fatalf("effective ceiling = %s, want hands_off; a run of good-bad-good healed it", ceiling)
	}
}

func TestAPartialRecoveryStillCountsAsARecoveryToThatLevel(t *testing.T) {
	// An adapter demoted all the way to hands_off that now reliably reaches
	// one_tap should say one_tap, not stay at hands_off and not jump to
	// completes. It heals to what it actually reached.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)
	for i := 0; i < healRuns; i++ {
		_ = r.RecordMeasuredCeiling("uber", manifest.OneTap)
	}

	if ceiling, _, _ := r.EffectiveCeiling("uber"); ceiling != manifest.OneTap {
		t.Fatalf("effective ceiling = %s, want one_tap", ceiling)
	}
}

func TestAnAdapterThatHasOnlyEverSucceededIsNotHeldBack(t *testing.T) {
	// The healing rule applies to coming back from a demotion. An adapter
	// that has never fallen short must not need two runs to be believed.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	_ = r.RecordMeasuredCeiling("uber", manifest.Completes)

	ceiling, proven, _ := r.EffectiveCeiling("uber")
	if ceiling != manifest.Completes {
		t.Fatalf("effective ceiling = %s, want completes on the first clean measurement", ceiling)
	}
	if !proven {
		t.Error("a measured adapter reads as never measured")
	}
}

func TestTheShownCeilingIsStillCappedByTheManifestWhileHealing(t *testing.T) {
	// Healing must not become a back door around the older rule. Repeated
	// measurements above the declared claim still cannot promote it.
	r := New()
	mustRegister(t, r, adapterFor("draft-only", func(m *manifest.Manifest) {
		m.Ceiling = manifest.HandsOff
	}))

	for i := 0; i < healRuns+2; i++ {
		_ = r.RecordMeasuredCeiling("draft-only", manifest.Completes)
	}

	if ceiling, _, _ := r.EffectiveCeiling("draft-only"); ceiling != manifest.HandsOff {
		t.Fatalf("effective ceiling = %s, want hands_off; healing promoted past the manifest", ceiling)
	}
}
