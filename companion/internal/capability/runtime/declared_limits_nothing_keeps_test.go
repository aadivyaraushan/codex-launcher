package runtime

import (
	"sort"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Two adapters declare a rate cap and nothing enforces it. This pins the two
// so the number cannot quietly grow.
//
// `Capacity` is written on every manifest and read only by `Validate()`, which
// checks that `Kind` is a legal word and never looks at `Limit` at all
// (manifest.go:490-491). `Capacity.Admits(connected int)` exists
// (manifest.go:321) and has no callers anywhere outside its own definition —
// nothing counts open connections, so nothing could call it. The only thing
// that can stop a run is `CheckGates()` (execution/runner.go:94, 114, 164),
// which reads the separate `Gates` field.
//
// So a capped adapter whose gates are `GateNone` has declared a limit that no
// code will ever apply. Today that is spotify (25) and youtube (100), and both
// declare `GateNone`.
//
// Contrast maps, which is deliberately NOT in this list: it declares
// `Cost: CostPerCall` alongside `Gates: [GateBilling]`, and the billing gate is
// really enforced. Its cost declaration is backed by something. That is the
// difference this test is drawing — not "declares a restriction" but "declares
// a restriction with nothing behind it".
//
// The fix is one of two things, and both are the owner's call: consult
// `Capacity` beside `CheckGates()` in the runner, or drop the field. See
// `saved-results/cost-capacity-region-are-decoration.md`.

// cappedWithNoGate returns the shipped adapters that declare a capacity limit
// while declaring no real gate to enforce it.
func cappedWithNoGate(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, b := range everyAdapter(t, nil) {
		m := b.a.Describe()
		if strings.TrimSpace(m.Unshipped) != "" {
			continue
		}
		if m.Capacity.Kind != manifest.CapacityCapped {
			continue
		}
		real := false
		for _, g := range m.Gates {
			if g != manifest.GateNone {
				real = true
			}
		}
		if !real {
			out = append(out, m.ID)
		}
	}
	sort.Strings(out)
	return out
}

func TestTheOnlyUnenforcedRateCapsAreTheTwoWeKnowAbout(t *testing.T) {
	got := cappedWithNoGate(t)
	want := []string{"spotify", "youtube"}

	if len(got) > len(want) {
		t.Errorf("an adapter now declares a rate cap that nothing will apply: got %v, pinned %v.\n"+
			"Capacity.Admits has no callers and the runner gates only on Gates, so this "+
			"limit is decoration. Either consult Capacity beside CheckGates() in "+
			"execution/runner.go, or drop the field from this adapter — then update this pin.", got, want)
		return
	}
	if len(got) < len(want) {
		t.Errorf("good news, and this pin is now too high: got %v, pinned %v.\n"+
			"Either a cap was removed or enforcement landed. Lower the pin to match.", got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("the unenforced caps changed identity: got %v, pinned %v", got, want)
			return
		}
	}
}

func TestNoShippedAdapterRestrictsItsRegion(t *testing.T) {
	// Region has zero non-test readers anywhere in companion/. Every shipped
	// adapter says "global", so the missing enforcement costs nothing today.
	// The day one says something narrower, it will be ignored in silence, and
	// a region restriction that is ignored is worse than none — it reads like
	// a promise. This fires on that day.
	var restricted []string
	for _, b := range everyAdapter(t, nil) {
		m := b.a.Describe()
		if strings.TrimSpace(m.Unshipped) != "" {
			continue
		}
		if len(m.Region) != 1 || m.Region[0] != "global" {
			restricted = append(restricted, m.ID)
		}
	}
	if len(restricted) > 0 {
		t.Errorf("these adapters declare a narrower region than global: %v.\n"+
			"Nothing reads Manifest.Region, so the restriction will not be applied "+
			"and the adapter will run everywhere anyway. Give Region a reader "+
			"(next to CheckGates() in execution/runner.go is the natural place) "+
			"or drop the field, then delete this test.", restricted)
	}
}
