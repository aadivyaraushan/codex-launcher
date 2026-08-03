package handoff

import (
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func TestDraftOutcomeMatchesWireHandsOffContract(t *testing.T) {
	out := DraftOutcome("Instagram", "draft text here")
	if out.Reached != manifest.HandsOff || !out.Done || out.HandedOffTo != "Instagram" {
		t.Fatalf("outcome=%+v", out)
	}
	lower := strings.ToLower(out.Detail)
	if strings.Contains(lower, "sent") {
		t.Fatalf("detail claims send: %q", out.Detail)
	}
	if !strings.Contains(out.Detail, "draft text here") {
		t.Fatalf("detail dropped the draft: %q", out.Detail)
	}
	if !strings.Contains(lower, "cannot know") {
		t.Fatalf("detail must say we cannot know whether the user finished: %q", out.Detail)
	}
}

// Wire safeDisplayString rejects unicode control characters (including newlines).
// A detail with \n fails EncodeText and kills phone hello warm-replay.
func TestDraftOutcomeDetailHasNoControlCharacters(t *testing.T) {
	out := DraftOutcome("Instagram", "line one\nline two")
	for _, r := range out.Detail {
		if r < 0x20 {
			t.Fatalf("detail has control rune %U: %q", r, out.Detail)
		}
	}
	if !strings.Contains(out.Detail, "line one") || !strings.Contains(out.Detail, "line two") {
		t.Fatalf("detail should keep draft words without raw newlines: %q", out.Detail)
	}
}
