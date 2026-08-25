package main

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/deeplink"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/discovery"
)

func casesFor(cases []discovery.Case, id string) []discovery.Case {
	var out []discovery.Case
	for _, c := range cases {
		if c.ExpectedApp == id {
			out = append(out, c)
		}
	}
	return out
}

func findBeeper(cases []discovery.Case, id, beeper string) *discovery.Case {
	for i := range cases {
		if cases[i].ExpectedApp == id && cases[i].Beeper == beeper {
			return &cases[i]
		}
	}
	return nil
}

// A Beeper-moved app that lives in Wave1 (discord, messages) must not be graded
// with one static label across both Beeper states: with Beeper off it is
// messaging/compose, with Beeper on production.go relocates it to
// beeper_messaging/send. A single beeper:"both" case would score the on-state
// relocation as a misroute once Fix 1 lands.
func TestBeeperMovedAppsGetPerStateLabels(t *testing.T) {
	cases := generate()
	for _, id := range []string{"discord", "messages"} {
		if c := findBeeper(cases, id, "both"); c != nil {
			t.Errorf("%s: expected no beeper:\"both\" case, still found one", id)
		}
		off := findBeeper(cases, id, "off")
		if off == nil {
			t.Fatalf("%s: missing beeper:\"off\" case", id)
		}
		if off.ExpectedClass != "messaging" || off.ExpectedVerb != "compose" {
			t.Errorf("%s off: got class=%q verb=%q, want messaging/compose", id, off.ExpectedClass, off.ExpectedVerb)
		}
		on := findBeeper(cases, id, "on")
		if on == nil {
			t.Fatalf("%s: missing beeper:\"on\" case", id)
		}
		if on.ExpectedClass != beeperMovedClass || on.ExpectedVerb != beeperMovedVerb {
			t.Errorf("%s on: got class=%q verb=%q, want %s/%s", id, on.ExpectedClass, on.ExpectedVerb, beeperMovedClass, beeperMovedVerb)
		}
		if off.Utterance != on.Utterance {
			t.Errorf("%s: off/on utterances differ (%q vs %q); the ask is the same, only the expected label changes", id, off.Utterance, on.Utterance)
		}
	}
}

// A messaging app Beeper does not move (whatsapp) stays a single beeper:"both"
// case with its static Wave1 label. Fix 0 must touch only the moved apps.
func TestNonMovedMessagingAppKeepsSingleBothCase(t *testing.T) {
	cases := generate()
	got := casesFor(cases, "whatsapp")
	if len(got) != 1 {
		t.Fatalf("whatsapp: got %d cases, want exactly 1", len(got))
	}
	c := got[0]
	if c.Beeper != "both" {
		t.Errorf("whatsapp: got beeper=%q, want both", c.Beeper)
	}
	if c.ExpectedClass != "messaging" || c.ExpectedVerb != "compose" {
		t.Errorf("whatsapp: got class=%q verb=%q, want messaging/compose", c.ExpectedClass, c.ExpectedVerb)
	}
}

// The moved set is read from beepermessage.ProductionSpecs via beeperMovedIDs,
// not hand-listed, so it can't drift. instagram is in that set but not in
// Wave1 (it has a bespoke off-state adapter), so it must never appear in the
// generated auto-corpus at all.
func TestInstagramNotInGeneratedCorpus(t *testing.T) {
	if !beeperMovedIDs()["instagram"] {
		t.Fatal("expected instagram in beeperMovedIDs (source: beepermessage.ProductionSpecs)")
	}
	inWave1 := false
	for _, spec := range deeplink.Wave1Specs() {
		if spec.ID == "instagram" {
			inWave1 = true
		}
	}
	if inWave1 {
		t.Skip("instagram is in Wave1 after all; this guard no longer applies")
	}
	if got := casesFor(generate(), "instagram"); len(got) != 0 {
		t.Errorf("instagram: got %d generated cases, want 0 (it's a hand probe, not auto)", len(got))
	}
}
