package manifest

import (
	"errors"
	"strings"
	"testing"
)

// A manifest that passes validation, used as the starting point for the cases
// below so each one changes exactly the field it is about.
func good() Manifest {
	return Manifest{
		ID:            "notion",
		Runtime:       RT1,
		Verbs:         []Verb{Read, Write},
		Ceiling:       Completes,
		Consent:       ConsentA,
		Auth:          AuthOAuth,
		Cost:          CostFree,
		Gates:         []Gate{GateNone},
		Capacity:      Capacity{Kind: CapacityNone},
		Region:        []string{"global"},
		Platform:      PlatformBoth,
		ProvesCeiling: "notion-smoke",
	}
}

// ---- the closed verb set ------------------------------------------------

func TestTheVerbSetIsExactlyTheNineTheSpineAllows(t *testing.T) {
	want := []string{"read", "compose", "send", "order", "book", "play", "write", "cancel", "modify"}
	if len(AllVerbs) != len(want) {
		t.Fatalf("verb set has %d entries, want %d: %v", len(AllVerbs), len(want), AllVerbs)
	}
	for _, name := range want {
		if _, err := ParseVerb(name); err != nil {
			t.Errorf("ParseVerb(%q) failed: %v", name, err)
		}
	}
}

func TestPayIsNotAVerbAndNeverBecomesOne(t *testing.T) {
	// Money movement is deep-link only, forever, on every runtime. The verb
	// set is where that is enforced, because an adapter cannot ask for a verb
	// that does not exist.
	if _, err := ParseVerb("pay"); err == nil {
		t.Fatal("ParseVerb(\"pay\") succeeded; pay is deliberately absent from the verb set")
	}
}

func TestOpenIsNotAVerbBecauseItIsTheFloorUnderEveryVerb(t *testing.T) {
	if _, err := ParseVerb("open"); err == nil {
		t.Fatal("ParseVerb(\"open\") succeeded; opening the app is the floor under every verb, not one of them")
	}
}

func TestAManifestAskingForAnUnknownVerbIsRejected(t *testing.T) {
	m := good()
	m.Verbs = []Verb{Read, Verb("pay")}
	if err := m.Validate(); err == nil {
		t.Fatal("a manifest declaring the verb \"pay\" validated; it must not")
	}
}

func TestAManifestWithNoVerbsIsRejected(t *testing.T) {
	m := good()
	m.Verbs = nil
	if err := m.Validate(); err == nil {
		t.Fatal("a manifest with no verbs validated; an adapter that can do nothing is a bug, not a capability")
	}
}

// ---- ceilings -----------------------------------------------------------

func TestTheThreeCeilingsAreOrderedSoDemotionCanBeComputed(t *testing.T) {
	if !(Completes.Rank() > OneTap.Rank() && OneTap.Rank() > HandsOff.Rank()) {
		t.Fatalf("ceilings are not ordered: completes=%d one_tap=%d hands_off=%d",
			Completes.Rank(), OneTap.Rank(), HandsOff.Rank())
	}
}

func TestAtMostTakesTheLowerOfTwoCeilings(t *testing.T) {
	// This is how a measured outcome demotes a declared claim. The manifest
	// says completes; the smoke test reached hands_off; the user is told
	// hands_off.
	cases := []struct {
		declared, measured, want Ceiling
	}{
		{Completes, HandsOff, HandsOff},
		{Completes, OneTap, OneTap},
		{OneTap, Completes, OneTap},
		{HandsOff, Completes, HandsOff},
		{Completes, Completes, Completes},
	}
	for _, c := range cases {
		if got := c.declared.AtMost(c.measured); got != c.want {
			t.Errorf("%s.AtMost(%s) = %s, want %s", c.declared, c.measured, got, c.want)
		}
	}
}

func TestAnUnknownCeilingIsRejected(t *testing.T) {
	m := good()
	m.Ceiling = Ceiling("mostly_works")
	if err := m.Validate(); err == nil {
		t.Fatal("a manifest declaring an invented ceiling validated")
	}
}

// ---- consent classes ----------------------------------------------------

func TestOnlyConsentClassesAAndBAreShippable(t *testing.T) {
	shippable := map[Consent]bool{
		ConsentA: true, ConsentB: true,
		ConsentC1: false, ConsentC2: false, ConsentC3: false,
	}
	for class, want := range shippable {
		if got := class.Shippable(); got != want {
			t.Errorf("Consent(%s).Shippable() = %v, want %v", class, got, want)
		}
	}
}

func TestAClassCManifestIsRejectedAtValidation(t *testing.T) {
	// C1/C2/C3 are the classes that are never shipped. Catching them here
	// means no later stage has to remember the rule.
	for _, class := range []Consent{ConsentC1, ConsentC2, ConsentC3} {
		m := good()
		m.Consent = class
		err := m.Validate()
		if err == nil {
			t.Errorf("a class %s manifest validated; class C is never shipped", class)
			continue
		}
		if !strings.Contains(err.Error(), string(class)) {
			t.Errorf("error for class %s does not name the class: %v", class, err)
		}
	}
}

// ---- capacity, which the capacity gate reads ----------------------------

func TestCapacityRoundTripsThroughItsWrittenForm(t *testing.T) {
	cases := []struct {
		text string
		want Capacity
	}{
		{"none", Capacity{Kind: CapacityNone}},
		{"capped:5", Capacity{Kind: CapacityCapped, Limit: 5}},
		{"capped:100", Capacity{Kind: CapacityCapped, Limit: 100}},
		{"pending_application", Capacity{Kind: CapacityPendingApplication}},
	}
	for _, c := range cases {
		got, err := ParseCapacity(c.text)
		if err != nil {
			t.Errorf("ParseCapacity(%q) failed: %v", c.text, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseCapacity(%q) = %+v, want %+v", c.text, got, c.want)
		}
		if got.String() != c.text {
			t.Errorf("Capacity(%+v).String() = %q, want %q", got, got.String(), c.text)
		}
	}
}

func TestACapWithoutANumberIsRejected(t *testing.T) {
	for _, text := range []string{"capped", "capped:", "capped:many", "capped:0", "capped:-3"} {
		if _, err := ParseCapacity(text); err == nil {
			t.Errorf("ParseCapacity(%q) succeeded; a cap with no usable number is not a cap", text)
		}
	}
}

func TestACappedAdapterStopsAdmittingUsersAtTheCap(t *testing.T) {
	// Spotify is five users. Without this the capacity gate is a paragraph
	// rather than a check.
	c := Capacity{Kind: CapacityCapped, Limit: 5}
	if !c.Admits(4) {
		t.Error("a 5-user cap refused the 5th user (4 already connected)")
	}
	if c.Admits(5) {
		t.Error("a 5-user cap admitted a 6th user")
	}
}

func TestAnAdapterPendingApplicationAdmitsNobody(t *testing.T) {
	c := Capacity{Kind: CapacityPendingApplication}
	if c.Admits(0) {
		t.Error("an adapter whose application has not been granted admitted a user")
	}
}

func TestUncappedAdaptersAdmitEveryone(t *testing.T) {
	c := Capacity{Kind: CapacityNone}
	if !c.Admits(10_000) {
		t.Error("an uncapped adapter refused a user")
	}
}

// ---- platform, which is what makes an App Store refusal survivable ------

func TestAnAndroidOnlyAdapterIsNotOfferedOnIOS(t *testing.T) {
	if PlatformAndroid.Includes(PlatformIOS) {
		t.Error("an android-only adapter claimed to cover iOS")
	}
	if !PlatformAndroid.Includes(PlatformAndroid) {
		t.Error("an android-only adapter did not cover android")
	}
	if !PlatformBoth.Includes(PlatformIOS) || !PlatformBoth.Includes(PlatformAndroid) {
		t.Error("a both-platform adapter did not cover one of the platforms")
	}
}

// ---- the ceiling is a claim until something proves it -------------------

func TestAManifestWithoutASmokeTestIsValidButUnproven(t *testing.T) {
	// It still ships. It ships as unverified, and the UI says so. Refusing to
	// validate it would stop adapters existing before their test does.
	m := good()
	m.ProvesCeiling = ""
	if err := m.Validate(); err != nil {
		t.Fatalf("a manifest with no smoke test failed validation: %v", err)
	}
	if m.NamesAProof() {
		t.Error("a manifest with no smoke test claims to name one")
	}
}

func TestAManifestNamingASmokeTestClaimsAProvableCeiling(t *testing.T) {
	// Note what this does and does not say. It says the manifest named
	// something. Whether that name belongs to a test that exists is a
	// separate question, and today the answer is no for every shipped
	// adapter — see runtime/proof_names_resolve_test.go.
	if !good().NamesAProof() {
		t.Error("a manifest naming a smoke test does not claim a provable ceiling")
	}
}

// ---- preview is a property of the verb, not of the adapter --------------

func TestEveryVerbThatMovesSomethingIrreversibleRequiresAPreview(t *testing.T) {
	// "order the usual" spending money with no preview is the silent
	// over-reach this list exists to stop. cancel and modify are here because
	// a cancellation is irreversible in the direction that matters.
	mustPreview := []Verb{Send, Order, Book, Write, Cancel, Modify}
	for _, v := range mustPreview {
		if !v.RequiresPreview() {
			t.Errorf("verb %s does not require a preview; it must", v)
		}
	}
	for _, v := range []Verb{Read, Compose, Play} {
		if v.RequiresPreview() {
			t.Errorf("verb %s requires a preview; reading, drafting and playback do not move anything irreversible", v)
		}
	}
}

// ---- an app is data, not code ------------------------------------------

func TestAManifestLoadsFromTheJSONFieldNamesThePlanUses(t *testing.T) {
	raw := []byte(`{
	  "id": "telegram",
	  "runtime": "RT-2",
	  "verbs": ["read", "compose", "send"],
	  "ceiling": "completes",
	  "consent": "A",
	  "auth": "oauth",
	  "cost": "free",
	  "gates": ["none"],
	  "capacity": "capped:5",
	  "region": ["global"],
	  "platform": "both",
	  "proves_ceiling": "telegram-send-smoke"
	}`)

	m, err := Load(raw)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if m.ID != "telegram" || m.Runtime != RT2 || m.Ceiling != Completes {
		t.Fatalf("loaded manifest is wrong: %+v", m)
	}
	if len(m.Verbs) != 3 || m.Verbs[2] != Send {
		t.Fatalf("verbs did not load: %+v", m.Verbs)
	}
	if m.Capacity != (Capacity{Kind: CapacityCapped, Limit: 5}) {
		t.Fatalf("capacity did not load: %+v", m.Capacity)
	}
	if m.ProvesCeiling != "telegram-send-smoke" {
		t.Fatalf("proves_ceiling did not load: %q", m.ProvesCeiling)
	}
}

func TestLoadRejectsAManifestThatWouldNotValidate(t *testing.T) {
	// Loading and validating are one step, so no caller can forget the second.
	raw := []byte(`{"id":"x","runtime":"RT-2","verbs":["pay"],"ceiling":"completes",
	  "consent":"A","auth":"none","cost":"free","gates":["none"],"capacity":"none",
	  "region":["global"],"platform":"both"}`)
	if _, err := Load(raw); err == nil {
		t.Fatal("Load accepted a manifest declaring the verb \"pay\"")
	}
}

func TestAManifestWithNoIDIsRejected(t *testing.T) {
	m := good()
	m.ID = ""
	if err := m.Validate(); err == nil {
		t.Fatal("a manifest with no id validated; the id is the routing key")
	}
}

func TestValidationReportsEverythingWrongAtOnce(t *testing.T) {
	// One round trip per fix is a slow loop. Report them together.
	m := good()
	m.ID = ""
	m.Ceiling = Ceiling("nope")
	m.Consent = ConsentC2

	err := m.Validate()
	if err == nil {
		t.Fatal("a manifest with three faults validated")
	}
	for _, want := range []string{"id", "ceiling", "C2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error does not mention %q: %v", want, err)
		}
	}
}

func TestValidationErrorsAreInspectable(t *testing.T) {
	m := good()
	m.Consent = ConsentC3
	err := m.Validate()
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("validation error is not an ErrInvalidManifest: %v", err)
	}
}
