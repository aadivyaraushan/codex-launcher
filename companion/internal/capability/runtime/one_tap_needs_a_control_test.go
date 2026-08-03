package runtime

import (
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// The phone cannot finish a one_tap run yet, so nothing may ask it to.
//
// A one_tap ceiling means the adapter got everything done except one tap the
// user still has to make. The phone models that: CapabilityOutcome.of builds
// an ONE_TAP_LEFT outcome carrying confirmControl = "Confirm", the name of the
// control that finishes it (CapabilityOutcome.kt:245). Nothing renders it.
// CapabilitySheet's RESULT dialog shows the detail line, the message and
// recoveryAction, and its buttons are Copy draft, Open <app>, Disconnect
// <name> and Done — not one of them driven by confirmControl.
//
// That gap is invisible today only because no adapter declares one_tap: the
// ceiling reaches the phone over the wire via Ceiling.fromWire, so the first
// adapter that declares it is also the first to expose the missing button.
// And the user would be told to look for it — ConnectionNotificationPolicy.kt:33
// pushes "One tap left" / "Open Codex Launcher to finish it", which sends
// someone to a dialog whose only button is Done.
//
// So this test is a tripwire, not a rule against the feature. one_tap is a
// perfectly good ceiling and the phone is half-built for it. Whoever finishes
// that half deletes this test in the same change.
func TestNoAdapterAsksForATapThePhoneCannotOffer(t *testing.T) {
	var oneTap []string
	for _, b := range everyAdapter(t, nil) {
		m := b.a.Describe()
		if strings.TrimSpace(m.Unshipped) != "" {
			continue
		}
		if m.Ceiling == manifest.OneTap {
			oneTap = append(oneTap, m.ID)
		}
	}

	if len(oneTap) > 0 {
		t.Errorf("these adapters declare a one_tap ceiling: %v.\n"+
			"The phone has no way to offer that tap yet: CapabilityOutcome carries "+
			"confirmControl but CapabilitySheet's RESULT dialog never renders it, so "+
			"the user gets a 'One tap left' notification and then a dialog whose only "+
			"button is Done. Render confirmControl in that dialog (or drop the field), "+
			"then delete this test.", oneTap)
	}
}
