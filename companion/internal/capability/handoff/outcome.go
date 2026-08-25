package handoff

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// DraftOutcome is a successful hand-off: the draft is ready, control leaves
// Operator, and we never claim the user finished in the other app.
//
// Wire contract (validation.go + ProtocolCodec): hands_off with a named app
// requires Done=true and HandedOffTo set. Android maps that to HANDED_OFF
// without claimsSuccess. Detail must stay free of control characters —
// safeDisplayString rejects newlines and a bad detail breaks EncodeText
// (including hello warm-replay of a journaled frame).
//
// Callers: adapters/instagram, adapters/deeplink (and any class-H DraftOutcome user).
// User ask: follow up on Instagram judge — unblock Pixel proof after serve-instagram-proof.
func DraftOutcome(appName, draft string) adapter.Outcome {
	cleanApp := scrubDisplayText(appName)
	cleanDraft := scrubDisplayText(draft)
	return adapter.Outcome{
		Reached:     manifest.HandsOff,
		Done:        true,
		HandedOffTo: cleanApp,
		Detail: fmt.Sprintf(
			"Draft ready for %s: %s — Copy it, open %s, choose where it goes, paste, and finish there. Operator cannot know whether you finished in %s.",
			cleanApp, cleanDraft, cleanApp, cleanApp,
		),
	}
}

func scrubDisplayText(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	lastSpace := false
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsControl(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = unicode.IsSpace(r)
	}
	return strings.TrimSpace(b.String())
}
