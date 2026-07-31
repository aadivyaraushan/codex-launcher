package verification

import (
	"context"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// A stub adapter whose only job is to be registered and to report a ceiling.
type stub struct {
	m       manifest.Manifest
	reached manifest.Ceiling
	err     error
	calls   int
}

func (s *stub) Describe() manifest.Manifest { return s.m }
func (s *stub) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{AdapterID: s.m.ID}, nil
}
func (s *stub) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}
func (s *stub) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	s.calls++
	if s.err != nil {
		return adapter.Outcome{}, s.err
	}
	return adapter.Outcome{Reached: s.reached, Done: s.reached == manifest.Completes}, nil
}
func (s *stub) Revoke(context.Context) error { return nil }

func adapterFor(id string, rt manifest.Runtime, declared manifest.Ceiling) *stub {
	return &stub{
		m: manifest.Manifest{
			ID: id, Runtime: rt, Verbs: []manifest.Verb{manifest.Read, manifest.Write, manifest.Send},
			Ceiling: declared, Consent: manifest.ConsentA,
			Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
			Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
			Region: []string{"global"}, Platform: manifest.PlatformBoth, ProvesCeiling: id + "-smoke",
		},
		reached: declared,
	}
}

func regWith(t *testing.T, stubs ...*stub) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, s := range stubs {
		if err := reg.Register(s); err != nil {
			t.Fatalf("Register(%s): %v", s.m.ID, err)
		}
	}
	return reg
}

// ---- which tier an adapter is in, and why -------------------------------

func TestSharedCredentialRuntimesAreTierOneAndAccountBoundOnesAreTierTwo(t *testing.T) {
	// RT-1/2/3 run on credentials Operator holds, so an unattended nightly run
	// is fine. RT-5/6 are bound to one user's account and hardware.
	for _, rt := range []manifest.Runtime{manifest.RT1, manifest.RT2, manifest.RT3} {
		if got := TierOf(adapterFor("x", rt, manifest.Completes).m); got != Tier1 {
			t.Errorf("%s is tier %d, want 1", rt, got)
		}
	}
	for _, rt := range []manifest.Runtime{manifest.RT5, manifest.RT6} {
		if got := TierOf(adapterFor("x", rt, manifest.Completes).m); got != Tier2 {
			t.Errorf("%s is tier %d, want 2", rt, got)
		}
	}
}

func TestOnDeviceNotificationReplyIsTierTwo(t *testing.T) {
	// RT-4 is not named in the plan's two lists. It runs on the user's own
	// phone against the user's own accounts, and an unattended job that fires
	// a reply into a real conversation is exactly what tier 2 exists to stop,
	// so it goes with the account-bound side.
	if got := TierOf(adapterFor("sms", manifest.RT4, manifest.Completes).m); got != Tier2 {
		t.Errorf("RT-4 is tier %d, want 2", got)
	}
}

// ---- tier 1: unattended, but never into somebody's real document --------

func TestAnUnattendedWriteOnlyGoesIntoAContainerTheAdapterMade(t *testing.T) {
	// The failure this prevents is silent, repeating and hard to undo: a
	// nightly job quietly editing somebody's real page.
	s := adapterFor("notion", manifest.RT1, manifest.Completes)
	run := NewTier1(regWith(t, s))

	_, err := run.Run(context.Background(), Probe{
		AdapterID: "notion", Verb: manifest.Write,
		Container: "Owner's real meeting notes", ContainerCreatedByAdapter: false,
		Account: RealUser,
	})
	if !errors.Is(err, ErrForeignContainer) {
		t.Fatalf("an unattended write into a user's own page returned %v", err)
	}
	if s.calls != 0 {
		t.Fatal("the adapter ran anyway")
	}
}

func TestAnUnattendedWriteIntoTheAdaptersOwnContainerIsFine(t *testing.T) {
	s := adapterFor("notion", manifest.RT1, manifest.Completes)
	run := NewTier1(regWith(t, s))

	if _, err := run.Run(context.Background(), Probe{
		AdapterID: "notion", Verb: manifest.Write,
		Container: "Operator verification", ContainerCreatedByAdapter: true,
		Account: RealUser,
	}); err != nil {
		t.Fatalf("a write into the adapter's own container was refused: %v", err)
	}
}

func TestReadingRealDataUnattendedIsAllowed(t *testing.T) {
	// A hand-built test page with three fake blocks proves only that we can
	// read three fake blocks. Real data is messier, and that is the point.
	s := adapterFor("notion", manifest.RT1, manifest.Completes)
	run := NewTier1(regWith(t, s))

	if _, err := run.Run(context.Background(), Probe{
		AdapterID: "notion", Verb: manifest.Read, Account: RealUser,
	}); err != nil {
		t.Fatalf("an unattended read of real data was refused: %v", err)
	}
}

func TestTheNightlyRunnerRefusesAnAccountBoundAdapterOutright(t *testing.T) {
	s := adapterFor("whatsapp", manifest.RT6, manifest.Completes)
	run := NewTier1(regWith(t, s))

	_, err := run.Run(context.Background(), Probe{AdapterID: "whatsapp", Verb: manifest.Read, Account: RealUser})
	if !errors.Is(err, ErrTierMismatch) {
		t.Fatalf("the unattended runner accepted a tier-2 adapter: %v", err)
	}
}

func TestATierOneRunRecordsWhatItActuallyReached(t *testing.T) {
	s := adapterFor("uber", manifest.RT2, manifest.Completes)
	s.reached = manifest.HandsOff
	reg := regWith(t, s)
	run := NewTier1(reg)

	if _, err := run.Run(context.Background(), Probe{AdapterID: "uber", Verb: manifest.Read, Account: OperatorTest}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ceiling, proven, err := reg.EffectiveCeiling("uber")
	if err != nil {
		t.Fatalf("EffectiveCeiling failed: %v", err)
	}
	if ceiling != manifest.HandsOff || !proven {
		t.Fatalf("after the run the adapter reads %s proven=%v, want hands_off proven=true", ceiling, proven)
	}
}

// ---- how often tier 1 runs ----------------------------------------------

func TestTheAdaptersCarryingRealTrafficRunNightlyAndTheTailRunsWeekly(t *testing.T) {
	// Forty-odd third-party test accounts is real ongoing cost, so the
	// cadence is proportionate rather than "everything, every night".
	m := adapterFor("spotify", manifest.RT2, manifest.Completes).m
	if got := ScheduleFor(m, true); got != Nightly {
		t.Errorf("a traffic-carrying adapter runs %s, want nightly", got)
	}
	if got := ScheduleFor(m, false); got != Weekly {
		t.Errorf("a tail adapter runs %s, want weekly", got)
	}
}

func TestAnAccountBoundAdapterIsNeverScheduledAtAll(t *testing.T) {
	m := adapterFor("whatsapp", manifest.RT6, manifest.Completes).m
	if got := ScheduleFor(m, true); got != Never {
		t.Errorf("a tier-2 adapter is scheduled %s; nothing unattended, ever", got)
	}
}

// ---- tier 2: the self-directed loop at connect time ---------------------

func TestTheConnectLoopOnlySendsToTheUsersOwnAccount(t *testing.T) {
	// Saved Messages, Note to Self, their own number. It proves send
	// end-to-end and reaches no third party.
	s := adapterFor("whatsapp", manifest.RT6, manifest.Completes)
	loop := NewTier2(regWith(t, s))

	_, err := loop.ConnectLoop(context.Background(), "whatsapp", Target{Handle: "+15550000009", IsSelf: false})
	if !errors.Is(err, ErrNotSelf) {
		t.Fatalf("the connect loop was willing to message a third party: %v", err)
	}
	if s.calls != 0 {
		t.Fatal("the adapter ran anyway")
	}
}

func TestTheConnectLoopIsWhereAnAccountBoundCeilingIsCaptured(t *testing.T) {
	s := adapterFor("whatsapp", manifest.RT6, manifest.Completes)
	s.reached = manifest.OneTap
	reg := regWith(t, s)
	loop := NewTier2(reg)

	if _, err := loop.ConnectLoop(context.Background(), "whatsapp", Target{Handle: "+15550000001", IsSelf: true}); err != nil {
		t.Fatalf("ConnectLoop failed: %v", err)
	}

	ceiling, proven, _ := reg.EffectiveCeiling("whatsapp")
	if ceiling != manifest.OneTap || !proven {
		t.Fatalf("the connect loop did not capture the ceiling: %s proven=%v", ceiling, proven)
	}
}

func TestTheConnectLoopRefusesATierOneAdapter(t *testing.T) {
	s := adapterFor("notion", manifest.RT1, manifest.Completes)
	loop := NewTier2(regWith(t, s))

	if _, err := loop.ConnectLoop(context.Background(), "notion", Target{Handle: "me", IsSelf: true}); !errors.Is(err, ErrTierMismatch) {
		t.Fatalf("the connect loop accepted a shared-credential adapter: %v", err)
	}
}

// ---- tier 2: the read-only heartbeat at wake ----------------------------

func TestTheHeartbeatCannotBeGivenAVerbThatChangesAnything(t *testing.T) {
	s := adapterFor("whatsapp", manifest.RT6, manifest.Completes)
	beat := NewTier2(regWith(t, s))

	if err := beat.Heartbeat(context.Background(), "whatsapp", manifest.Send); !errors.Is(err, ErrHeartbeatMustNotMutate) {
		t.Fatalf("the heartbeat accepted a send: %v", err)
	}
	if s.calls != 0 {
		t.Fatal("the heartbeat sent something")
	}
}

func TestAFailedHeartbeatSwitchesTheAdapterOffBeforeItIsUsedMidTask(t *testing.T) {
	// Failing at second 0.2 with a clean login prompt beats dying at second 8.
	s := adapterFor("whatsapp", manifest.RT6, manifest.Completes)
	s.err = errors.New("session expired")
	reg := regWith(t, s)
	beat := NewTier2(reg)

	if err := beat.Heartbeat(context.Background(), "whatsapp", manifest.Read); err == nil {
		t.Fatal("a dead session reported a healthy heartbeat")
	}

	reason, off := reg.Disabled("whatsapp")
	if !off {
		t.Fatal("a dead adapter was left switched on")
	}
	if reason == "" {
		t.Error("the adapter was switched off with no reason to show the user")
	}
}

func TestAHealthyHeartbeatNeverRecordsACeiling(t *testing.T) {
	// It is the cheapest non-mutating call the adapter has, not a
	// measurement. Letting it record would make a read prove a send.
	s := adapterFor("whatsapp", manifest.RT6, manifest.Completes)
	reg := regWith(t, s)
	beat := NewTier2(reg)

	if err := beat.Heartbeat(context.Background(), "whatsapp", manifest.Read); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	if _, proven, _ := reg.EffectiveCeiling("whatsapp"); proven {
		t.Error("the heartbeat recorded a ceiling")
	}
}

// ---- outcome telemetry, and the one-way door on ceilings ----------------

func TestRealOutcomesCanOnlyLowerACeilingNeverRaiseIt(t *testing.T) {
	// A manifest that under-claims stays under-claiming. Promotion needs a
	// deliberate change to the manifest, reviewed, not a lucky run.
	s := adapterFor("opentable", manifest.RT5, manifest.HandsOff)
	reg := regWith(t, s)
	tel := NewTelemetry(reg)

	if err := tel.Observe("opentable", manifest.Completes); err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	ceiling, _, _ := reg.EffectiveCeiling("opentable")
	if ceiling != manifest.HandsOff {
		t.Fatalf("a good run promoted the adapter to %s", ceiling)
	}
}

func TestTelemetryIsThePrimaryRotSignalAndSaysWhenToAlert(t *testing.T) {
	// The scheduled run is a backstop; real traffic is the main sensor. An
	// adapter whose recent runs fall short of its claim has to surface, not
	// just quietly demote.
	s := adapterFor("spotify", manifest.RT2, manifest.Completes)
	reg := regWith(t, s)
	tel := NewTelemetry(reg)

	for range 5 {
		if err := tel.Observe("spotify", manifest.HandsOff); err != nil {
			t.Fatalf("Observe failed: %v", err)
		}
	}

	report, err := tel.Report("spotify")
	if err != nil {
		t.Fatalf("Report failed: %v", err)
	}
	if report.Observations != 5 || report.BelowClaim != 5 {
		t.Fatalf("report = %+v, want 5 observations all below the claim", report)
	}
	if !report.Alert {
		t.Error("five straight runs below the claim did not raise an alert")
	}
}

func TestOneShortfallIsNotAnAlertButIsStillADemotion(t *testing.T) {
	// Demoting on the first real shortfall keeps the shown ceiling honest;
	// alerting on it would drown the signal in one-off flakes.
	s := adapterFor("spotify", manifest.RT2, manifest.Completes)
	reg := regWith(t, s)
	tel := NewTelemetry(reg)

	_ = tel.Observe("spotify", manifest.HandsOff)

	report, _ := tel.Report("spotify")
	if report.Alert {
		t.Error("a single shortfall raised an alert")
	}
	if ceiling, proven, _ := reg.EffectiveCeiling("spotify"); ceiling != manifest.HandsOff || !proven {
		t.Errorf("the shown ceiling is %s proven=%v, want hands_off proven=true", ceiling, proven)
	}
}

func TestTelemetryForAnUnknownAdapterIsAnErrorNotAnEmptyReport(t *testing.T) {
	tel := NewTelemetry(registry.New())
	if err := tel.Observe("ghost", manifest.Completes); !errors.Is(err, registry.ErrUnknownAdapter) {
		t.Fatalf("Observe on an unknown adapter returned %v", err)
	}
}
