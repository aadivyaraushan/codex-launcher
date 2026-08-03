package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// A stub adapter. Every test here is about the registry, so the adapter only
// has to describe itself and record that it was asked to revoke.
type stub struct {
	m       manifest.Manifest
	revoked int
}

func (s *stub) Describe() manifest.Manifest { return s.m }
func (s *stub) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{AdapterID: s.m.ID}, nil
}
func (s *stub) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}
func (s *stub) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	return adapter.Outcome{Reached: s.m.Ceiling, Done: true}, nil
}
func (s *stub) Revoke(context.Context) error { s.revoked++; return nil }

func adapterFor(id string, mutate ...func(*manifest.Manifest)) *stub {
	m := manifest.Manifest{
		ID:            id,
		Runtime:       manifest.RT2,
		Verbs:         []manifest.Verb{manifest.Read, manifest.Send},
		Ceiling:       manifest.Completes,
		Consent:       manifest.ConsentA,
		Auth:          manifest.AuthOAuth,
		Cost:          manifest.CostFree,
		Gates:         []manifest.Gate{manifest.GateNone},
		Capacity:      manifest.Capacity{Kind: manifest.CapacityNone},
		Region:        []string{"global"},
		Platform:      manifest.PlatformBoth,
		ProvesCeiling: id + "-smoke",
	}
	for _, f := range mutate {
		f(&m)
	}
	return &stub{m: m}
}

func mustRegister(t *testing.T, r *Registry, a adapter.Adapter) {
	t.Helper()
	if err := r.Register(a); err != nil {
		t.Fatalf("Register(%s) failed: %v", a.Describe().ID, err)
	}
}

// ---- registering --------------------------------------------------------

func TestARegisteredAdapterCanBeFoundByID(t *testing.T) {
	r := New()
	a := adapterFor("notion")
	mustRegister(t, r, a)

	got, err := r.Get("notion")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Describe().ID != "notion" {
		t.Fatalf("Get returned %q", got.Describe().ID)
	}
}

func TestAnUnknownIDIsAnErrorNotANilAdapter(t *testing.T) {
	r := New()
	if _, err := r.Get("nothing"); !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Get on an unknown id returned %v, want ErrUnknownAdapter", err)
	}
}

func TestRegisteringAnInvalidManifestIsRefused(t *testing.T) {
	r := New()
	a := adapterFor("broken", func(m *manifest.Manifest) { m.Verbs = []manifest.Verb{"pay"} })
	if err := r.Register(a); err == nil {
		t.Fatal("the registry accepted an adapter whose manifest does not validate")
	}
	if _, err := r.Get("broken"); !errors.Is(err, ErrUnknownAdapter) {
		t.Fatal("a refused adapter is still reachable")
	}
}

func TestRegisteringAClassCAdapterIsRefused(t *testing.T) {
	r := New()
	a := adapterFor("scraper", func(m *manifest.Manifest) { m.Consent = manifest.ConsentC2 })
	if err := r.Register(a); err == nil {
		t.Fatal("the registry accepted a class C adapter; class C is never shipped")
	}
}

func TestRegisteringTheSameIDTwiceIsRefused(t *testing.T) {
	r := New()
	mustRegister(t, r, adapterFor("notion"))
	if err := r.Register(adapterFor("notion")); err == nil {
		t.Fatal("the registry accepted a second adapter with an id it already has")
	}
}

// ---- finding an adapter for a verb --------------------------------------

func TestForVerbReturnsOnlyAdaptersThatDeclareIt(t *testing.T) {
	r := New()
	mustRegister(t, r, adapterFor("whatsapp"))
	mustRegister(t, r, adapterFor("spotify", func(m *manifest.Manifest) {
		m.Verbs = []manifest.Verb{manifest.Play, manifest.Read}
	}))

	ids := idsOf(r.ForVerb(manifest.Send, manifest.PlatformAndroid))
	if len(ids) != 1 || ids[0] != "whatsapp" {
		t.Fatalf("ForVerb(send) = %v, want [whatsapp]", ids)
	}
}

func TestForVerbSkipsAdaptersThatDoNotRunOnThisPlatform(t *testing.T) {
	// An adapter can exist on one platform and not the other. This is what
	// makes an App Store refusal survivable rather than fatal.
	r := New()
	mustRegister(t, r, adapterFor("sms", func(m *manifest.Manifest) {
		m.Platform = manifest.PlatformAndroid
	}))

	if ids := idsOf(r.ForVerb(manifest.Send, manifest.PlatformIOS)); len(ids) != 0 {
		t.Fatalf("an android-only adapter was offered on iOS: %v", ids)
	}
	if ids := idsOf(r.ForVerb(manifest.Send, manifest.PlatformAndroid)); len(ids) != 1 {
		t.Fatalf("an android-only adapter was not offered on android: %v", ids)
	}
}

// ---- the kill switch ----------------------------------------------------

func TestADisabledAdapterCannotBeObtainedAtAll(t *testing.T) {
	// Disabling has to make the adapter unreachable rather than merely
	// unrecommended, because Get is the only door to Execute.
	r := New()
	mustRegister(t, r, adapterFor("whatsapp"))

	if err := r.Disable("whatsapp", "ban risk spiked"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	_, err := r.Get("whatsapp")
	if !errors.Is(err, ErrAdapterDisabled) {
		t.Fatalf("Get on a disabled adapter returned %v, want ErrAdapterDisabled", err)
	}
	if ids := idsOf(r.ForVerb(manifest.Send, manifest.PlatformAndroid)); len(ids) != 0 {
		t.Fatalf("a disabled adapter was still offered for a verb: %v", ids)
	}
}

func TestADisabledAdapterSaysWhyAndWhen(t *testing.T) {
	r := New()
	mustRegister(t, r, adapterFor("whatsapp"))
	_ = r.Disable("whatsapp", "ban risk spiked")

	reason, off := r.Disabled("whatsapp")
	if !off {
		t.Fatal("Disabled reported an off adapter as on")
	}
	if reason != "ban risk spiked" {
		t.Fatalf("reason = %q, want %q", reason, "ban risk spiked")
	}
}

func TestAnAdapterCanBeSwitchedBackOn(t *testing.T) {
	r := New()
	mustRegister(t, r, adapterFor("whatsapp"))
	_ = r.Disable("whatsapp", "ban risk spiked")

	if err := r.Enable("whatsapp"); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	if _, err := r.Get("whatsapp"); err != nil {
		t.Fatalf("a re-enabled adapter is still unreachable: %v", err)
	}
}

func TestARemoteKillListSwitchesAdaptersOffAndBackOnInOneStep(t *testing.T) {
	// The list is fetched from the cloud, so it is the whole truth about
	// what is off. An adapter absent from the list is on, which is what makes
	// switching one back on a one-line change at our end.
	r := New()
	mustRegister(t, r, adapterFor("whatsapp"))
	mustRegister(t, r, adapterFor("instagram"))
	mustRegister(t, r, adapterFor("notion"))

	changed := r.ApplyKillList(KillList{
		Entries: []KillEntry{
			{ID: "whatsapp", Reason: "ban risk spiked"},
			{ID: "instagram", Reason: "login wall changed"},
		},
	})
	if len(changed) != 2 {
		t.Fatalf("ApplyKillList reported %d changes, want 2: %v", len(changed), changed)
	}
	if _, err := r.Get("whatsapp"); !errors.Is(err, ErrAdapterDisabled) {
		t.Error("whatsapp was not switched off by the kill list")
	}
	if _, err := r.Get("notion"); err != nil {
		t.Errorf("notion was switched off though it is not on the kill list: %v", err)
	}

	changed = r.ApplyKillList(KillList{Entries: []KillEntry{{ID: "whatsapp", Reason: "ban risk spiked"}}})
	if len(changed) != 1 || changed[0] != "instagram" {
		t.Fatalf("removing instagram from the list did not switch it back on: %v", changed)
	}
	if _, err := r.Get("instagram"); err != nil {
		t.Errorf("instagram is still off after leaving the kill list: %v", err)
	}
}

func TestAKillListNamingAnAdapterWeDoNotHaveIsNotAnError(t *testing.T) {
	// The cloud list covers every client version. An older build simply does
	// not have some of the adapters named on it.
	r := New()
	mustRegister(t, r, adapterFor("notion"))
	changed := r.ApplyKillList(KillList{Entries: []KillEntry{{ID: "tiktok", Reason: "never shipped"}}})
	if len(changed) != 0 {
		t.Fatalf("a kill list naming an unknown adapter reported changes: %v", changed)
	}
}

// ---- the ceiling is a measured fact -------------------------------------

func TestAnUnprovenAdapterReadsAsUnverifiedRatherThanAsItsClaim(t *testing.T) {
	r := New()
	mustRegister(t, r, adapterFor("notion"))

	ceiling, proven, err := r.EffectiveCeiling("notion")
	if err != nil {
		t.Fatalf("EffectiveCeiling failed: %v", err)
	}
	if proven {
		t.Error("an adapter whose smoke test has never run reads as proven")
	}
	if ceiling != manifest.Completes {
		t.Errorf("effective ceiling = %s, want the declared %s until something measures otherwise", ceiling, manifest.Completes)
	}
}

func TestAMeasuredCeilingBelowTheClaimDemotesTheAdapter(t *testing.T) {
	// Uber ships an official connector that cannot book a ride. This is the
	// mechanism that catches that a year from now, when nobody remembers.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	if err := r.RecordMeasuredCeiling("uber", manifest.HandsOff); err != nil {
		t.Fatalf("RecordMeasuredCeiling failed: %v", err)
	}

	ceiling, proven, _ := r.EffectiveCeiling("uber")
	if ceiling != manifest.HandsOff {
		t.Errorf("effective ceiling = %s, want hands_off after the measurement demoted it", ceiling)
	}
	if !proven {
		t.Error("an adapter with a measurement reads as unproven")
	}
}

func TestAMeasurementThatIsNotARealCeilingIsRefusedNotRecorded(t *testing.T) {
	// The worst version of a bad measurement is not that it is wrong — it is
	// that it wins. Rank() scores anything it does not recognise as 0, which
	// is below every real ceiling, so an adapter that forgot to set the field
	// reads as a fall and is written down as permanently demoted. Nothing
	// undoes that except three clean runs it may never get.
	//
	// The runner already refuses these before it reports one. This is the
	// same refusal at the place the value actually lands, so the two
	// verification paths that record measurements directly cannot open the
	// hole again.
	r := New()
	mustRegister(t, r, adapterFor("uber"))

	for _, bad := range []manifest.Ceiling{"", "sort_of", "COMPLETES"} {
		if err := r.RecordMeasuredCeiling("uber", bad); err == nil {
			t.Errorf("recording %q as a measured ceiling was accepted", bad)
		}
		ceiling, proven, _ := r.EffectiveCeiling("uber")
		if proven {
			t.Errorf("after a refused measurement of %q the adapter reads as proven", bad)
		}
		if ceiling != manifest.Completes {
			t.Errorf("after a refused measurement of %q the ceiling is %s, want the declared completes",
				bad, ceiling)
		}
	}
}

func TestAMeasurementCannotPromoteAnAdapterAboveItsOwnClaim(t *testing.T) {
	// The manifest is the ceiling on the ceiling. A measurement can only
	// lower it, so a bug in a smoke test cannot hand an adapter more
	// authority than its author asked for.
	r := New()
	mustRegister(t, r, adapterFor("draft-only", func(m *manifest.Manifest) {
		m.Ceiling = manifest.HandsOff
	}))

	_ = r.RecordMeasuredCeiling("draft-only", manifest.Completes)

	ceiling, _, _ := r.EffectiveCeiling("draft-only")
	if ceiling != manifest.HandsOff {
		t.Errorf("effective ceiling = %s, want hands_off; a measurement must never promote", ceiling)
	}
}

func TestADemotedAdapterIsStillUsableJustHonest(t *testing.T) {
	// Demotion changes what we promise, not whether the adapter runs.
	r := New()
	mustRegister(t, r, adapterFor("uber"))
	_ = r.RecordMeasuredCeiling("uber", manifest.HandsOff)

	if _, err := r.Get("uber"); err != nil {
		t.Fatalf("a demoted adapter became unreachable: %v", err)
	}
}

func idsOf(as []adapter.Adapter) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.Describe().ID)
	}
	return out
}
