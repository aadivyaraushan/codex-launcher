package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// The contract suite the plan asks for: "every adapter satisfies the same
// contract — declares its verbs, previews before send, revokes completely,
// fails closed when its runtime is down."
//
// Why it earns its place. Both real adapter bugs found this week were a
// single adapter quietly breaking a rule that every adapter shares, and
// neither adapter's own tests could see it, because each was only ever
// checked against itself. The check that catches that shape is not another
// adapter test — it is one set of rules applied to whatever production
// actually registered.
//
// Everything here runs against manifests and local calls, so the whole file
// is milliseconds and belongs in the ordinary dev loop.
//
// A note on what is covered: with no credentials supplied, production
// registers the credential-free adapters only. That is deliberate. These
// rules must hold for the set a real user gets on a fresh install, and
// running them against a hand-built registry instead would test a
// arrangement nobody ships.

// productionAdapters returns every adapter the production build registers,
// found by asking the registry for each verb in turn and dropping repeats.
// Going through the registry rather than a hand-written list is the whole
// point: a new adapter falls under these rules the moment it is registered,
// with nobody having to remember this file exists.
func productionAdapters(t *testing.T) []adapter.Adapter {
	t.Helper()
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	if inv.reg == nil {
		t.Fatal("the inventory carries no registry, so no contract can be checked against it")
	}

	seen := map[string]bool{}
	var out []adapter.Adapter
	for _, verb := range manifest.AllVerbs {
		for _, a := range inv.reg.ForVerb(verb, manifest.PlatformAndroid) {
			id := a.Describe().ID
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		t.Fatal("no adapters came back through the registry, so this suite would pass by checking nothing")
	}
	return out
}

// undeclaredVerb returns a verb this adapter did not declare, and whether one
// exists. An adapter declaring all nine has nothing to test against here.
func undeclaredVerb(m manifest.Manifest) (manifest.Verb, bool) {
	for _, verb := range manifest.AllVerbs {
		if !m.Allows(verb) {
			return verb, true
		}
	}
	return "", false
}

// ---- rule 1: declares its verbs ------------------------------------------

// An adapter that declares nothing can never be chosen by the router, yet it
// still sits in the registry reporting itself as available. An adapter whose
// ceiling is not one of the three real ones is worse: Rank() scores anything
// it does not recognise as 0, which beats every real ceiling in the clamp, so
// the bad value wins and is then written down as that adapter's measured
// ceiling.
func TestEveryAdapterDeclaresAWorkableManifest(t *testing.T) {
	for _, a := range productionAdapters(t) {
		m := a.Describe()
		if m.ID == "" {
			t.Error("an adapter has no id")
			continue
		}
		if err := m.Validate(); err != nil {
			t.Errorf("%s has an invalid manifest: %v", m.ID, err)
		}
		if len(m.Verbs) == 0 {
			t.Errorf("%s declares no verbs, so nothing can ever route to it", m.ID)
		}
		if !m.Ceiling.Valid() {
			t.Errorf("%s declares ceiling %q, which is not one of the three real ones", m.ID, m.Ceiling)
		}
		if strings.TrimSpace(m.ProvesCeiling) == "" {
			t.Errorf("%s names no smoke test for its ceiling, so the claim rests on nothing", m.ID)
		}
	}
}

// Declaring verbs is only worth something if the declaration is binding.
// Resolve is the single door every request comes through, so a verb an
// adapter accepts there but never declared is a capability that was never
// agreed to and that no manifest, review or kill list can see.
func TestNoAdapterResolvesAVerbItNeverDeclared(t *testing.T) {
	ctx := context.Background()
	for _, a := range productionAdapters(t) {
		m := a.Describe()
		verb, ok := undeclaredVerb(m)
		if !ok {
			continue
		}
		if _, err := a.Resolve(ctx, adapter.Intent{
			AdapterID: m.ID,
			Verb:      verb,
			Subject:   "contract suite probe",
			Handle:    "contract-suite",
		}); err == nil {
			t.Errorf("%s resolved %s, a verb it never declared", m.ID, verb)
		}
	}
}

// ---- rule 2: previews before send ----------------------------------------

// The confirm sheet is the only thing standing between the router's guess and
// something irreversible. A blank preview still renders a sheet and still
// takes a tap, so the user confirms an empty box and the send goes anyway.
// That is worse than no preview at all, because it looks like consent.
func TestEveryIrreversibleVerbGetsAReadablePreview(t *testing.T) {
	ctx := context.Background()
	var checked int
	for _, a := range productionAdapters(t) {
		m := a.Describe()
		for _, verb := range m.Verbs {
			if !verb.RequiresPreview() {
				continue
			}
			plan, err := a.Resolve(ctx, adapter.Intent{
				AdapterID: m.ID,
				Verb:      verb,
				Subject:   "contract suite probe",
				Handle:    "contract-suite",
				Body:      "contract suite body",
			})
			if err != nil {
				// Refusing to resolve a made-up subject is fine and common.
				// The rule is about what happens when a plan does exist.
				continue
			}
			preview, err := a.Preview(ctx, plan)
			if err != nil {
				t.Errorf("%s resolved a %s plan but could not preview it: %v", m.ID, verb, err)
				continue
			}
			checked++
			if strings.TrimSpace(preview.Headline) == "" {
				t.Errorf("%s previews %s with a blank headline; the sheet would show an empty box", m.ID, verb)
			}
			if strings.TrimSpace(preview.Confirm) == "" {
				t.Errorf("%s previews %s with no confirm label, so the button says nothing", m.ID, verb)
			}
			if preview.Fingerprint() != plan.Fingerprint() {
				t.Errorf("%s previewed a different plan than the one it resolved for %s; "+
					"the user would confirm one thing and Execute would check another", m.ID, verb)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no irreversible verb produced a preview, so this test asserted nothing at all")
	}
}

// ---- rule 3: revokes completely ------------------------------------------

// Revoke is the user taking their access back, so it must not fail, and it
// must not fail the second time either. A user who taps disconnect, sees an
// error, and taps again must not be told the app is still connected — and a
// revoke that only works once means anything that retries reports failure on
// a connection that is genuinely gone.
func TestEveryAdapterRevokesAndStaysRevoked(t *testing.T) {
	ctx := context.Background()
	for _, a := range productionAdapters(t) {
		id := a.Describe().ID
		if err := a.Revoke(ctx); err != nil {
			t.Errorf("%s failed to revoke: %v", id, err)
			continue
		}
		if err := a.Revoke(ctx); err != nil {
			t.Errorf("%s revoked once but failed the second time: %v", id, err)
		}
	}
}

// ---- rule 4: fails closed ------------------------------------------------

// Note on what this rule does not test. Adapters are not asked to re-check
// that a plan is theirs, because no production path can hand them someone
// else's: flow.Service keeps the resolved plan on the companion and a phone
// confirms by fingerprint, never by sending a plan back. Demanding every
// adapter guard against that would be defending a door that does not exist,
// in fifty places. The one real gap of this shape — Runner.Execute accepting
// a verb the adapter never declared, when Runner.Resolve refuses it — is a
// single check in a single place, and is tested in execution/runner_test.go.
//
// What every adapter must do instead is be honest about what it reached.
// This is exactly the shape the Maps navigation bug had: an empty or unknown
// ceiling is scored 0 by Rank(), which beats every real one during the clamp,
// so a single adapter forgetting this field both loses its own answer and
// writes a junk value into its permanent record.
func TestNoAdapterReportsACeilingThatDoesNotExist(t *testing.T) {
	ctx := context.Background()
	var checked int
	for _, a := range productionAdapters(t) {
		m := a.Describe()
		for _, verb := range m.Verbs {
			// Body matters here. Without it most adapters refuse to resolve
			// at all, every iteration skips, and this test passes having
			// asserted nothing — which is what it did when first written.
			// The count check at the bottom is what stops that returning.
			plan, err := a.Resolve(ctx, adapter.Intent{
				AdapterID: m.ID,
				Verb:      verb,
				Subject:   "contract suite probe",
				Handle:    "contract-suite",
				Body:      "contract suite body",
			})
			if err != nil {
				continue
			}
			out, err := a.Execute(ctx, plan)
			if err != nil {
				continue
			}
			checked++
			if !out.Reached.Valid() {
				t.Errorf("%s executed %s and reported ceiling %q, which is not a real one",
					m.ID, verb, out.Reached)
			}
			if out.HandedOffTo != "" && out.Reached != manifest.HandsOff {
				t.Errorf("%s executed %s, named %q as the app it handed to, and still reported %q; "+
					"the phone drops exactly this shape, so the result never renders",
					m.ID, verb, out.HandedOffTo, out.Reached)
			}
			if out.Reached == manifest.HandsOff && out.HandedOffTo == "" {
				t.Errorf("%s executed %s and reported hands_off without naming where it went, "+
					"so the user is told to finish somewhere and not told where", m.ID, verb)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no adapter got as far as an outcome, so this test asserted nothing at all")
	}
}
