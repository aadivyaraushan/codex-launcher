package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// A recording adapter, so a test can say what happened and in what order.
type recorder struct {
	m       manifest.Manifest
	calls   []string
	reached manifest.Ceiling
	handTo  string
	execErr error
	// exact, when set, is returned by Execute verbatim. It lets a test hand
	// the runner an outcome no well-behaved adapter would build, which is
	// the only way to check that the runner catches a misbehaving one.
	exact *adapter.Outcome
}

func (r *recorder) Describe() manifest.Manifest { return r.m }

func (r *recorder) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	r.calls = append(r.calls, "resolve")
	return adapter.Plan{
		AdapterID: r.m.ID,
		Verb:      in.Verb,
		Handle:    in.Handle,
		Summary:   "send \"" + in.Body + "\" to " + in.Handle,
	}, nil
}

func (r *recorder) Preview(_ context.Context, p adapter.Plan) (adapter.Preview, error) {
	r.calls = append(r.calls, "preview")
	return adapter.Preview{Plan: p, Headline: p.Summary, Confirm: "Send"}, nil
}

func (r *recorder) Execute(_ context.Context, _ adapter.Plan) (adapter.Outcome, error) {
	r.calls = append(r.calls, "execute")
	if r.execErr != nil {
		return adapter.Outcome{}, r.execErr
	}
	if r.exact != nil {
		return *r.exact, nil
	}
	return adapter.Outcome{
		Reached:     r.reached,
		Done:        r.reached == manifest.Completes,
		HandedOffTo: r.handTo,
	}, nil
}

func (r *recorder) Revoke(context.Context) error { r.calls = append(r.calls, "revoke"); return nil }

func newRecorder(id string, verbs []manifest.Verb, reached manifest.Ceiling) *recorder {
	return &recorder{
		m: manifest.Manifest{
			ID: id, Runtime: manifest.RT4, Verbs: verbs,
			Ceiling: manifest.Completes, Consent: manifest.ConsentA,
			Auth: manifest.AuthDevice, Cost: manifest.CostFree,
			Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
			Region: []string{"global"}, Platform: manifest.PlatformAndroid,
			ProvesCeiling: id + "-smoke",
		},
		reached: reached,
	}
}

func runnerWith(t *testing.T, a adapter.Adapter) (*Runner, *registry.Registry) {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	return New(reg), reg
}

func sendIntent(id string) adapter.Intent {
	return adapter.Intent{
		AdapterID: id, Verb: manifest.Send,
		Subject: "Maya K", Handle: "+15550000000", Body: "running ten minutes late",
	}
}

// ---- preview is never skipped for a send --------------------------------

func TestAnIrreversibleVerbCannotBeExecutedWithoutAConfirmedPreview(t *testing.T) {
	rec := newRecorder("sms", []manifest.Verb{manifest.Send}, manifest.Completes)
	run, _ := runnerWith(t, rec)

	plan, err := run.Resolve(context.Background(), sendIntent("sms"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	_, err = run.Execute(context.Background(), plan, Confirmation{})
	if !errors.Is(err, ErrPreviewRequired) {
		t.Fatalf("Execute without a preview returned %v, want ErrPreviewRequired", err)
	}
	for _, call := range rec.calls {
		if call == "execute" {
			t.Fatal("the adapter's Execute ran despite the missing preview")
		}
	}
}

func TestAConfirmedPreviewIsWhatUnlocksExecute(t *testing.T) {
	rec := newRecorder("sms", []manifest.Verb{manifest.Send}, manifest.Completes)
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, sendIntent("sms"))
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if preview.Headline == "" {
		t.Error("the preview has no headline; the user has nothing to confirm against")
	}

	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute after a confirmed preview failed: %v", err)
	}
	if !out.Done {
		t.Error("outcome says the send did not finish")
	}
}

func TestAConfirmationForADifferentPlanDoesNotUnlockThisOne(t *testing.T) {
	// The failure this stops: the user confirms "text Maya", the plan is
	// re-resolved to a different person or a different body, and the stale
	// confirmation carries the new plan through.
	rec := newRecorder("sms", []manifest.Verb{manifest.Send}, manifest.Completes)
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	shown, _ := run.Resolve(ctx, sendIntent("sms"))
	preview, _ := run.Preview(ctx, shown)

	other := sendIntent("sms")
	other.Handle = "+15559999999"
	swapped, _ := run.Resolve(ctx, other)

	if _, err := run.Execute(ctx, swapped, preview.Confirmed()); !errors.Is(err, ErrPreviewRequired) {
		t.Fatalf("a confirmation from a different plan unlocked this one: %v", err)
	}
}

func TestReadingNeedsNoPreview(t *testing.T) {
	rec := newRecorder("notion", []manifest.Verb{manifest.Read}, manifest.Completes)
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "notion", Verb: manifest.Read, Subject: "meeting notes"})
	if _, err := run.Execute(ctx, plan, Confirmation{}); err != nil {
		t.Fatalf("a read required a preview: %v", err)
	}
}

// ---- the outcome carries the ceiling actually reached -------------------

func TestTheOutcomeReportsTheCeilingReachedNotTheOneDeclared(t *testing.T) {
	rec := newRecorder("uber", []manifest.Verb{manifest.Book}, manifest.HandsOff)
	rec.handTo = "Uber"
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "uber", Verb: manifest.Book, Subject: "ride home"})
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out.Reached != manifest.HandsOff {
		t.Errorf("outcome ceiling = %s, want hands_off; the manifest claimed completes", out.Reached)
	}
	// Done means Operator finished its own part, not that the user's task is
	// over. A hand-off that names the app it went to is done in that sense,
	// and Android renders it as HANDED_OFF without claiming success
	// (see handoff/outcome.go and ProtocolCodec.kt:187). This assertion used
	// to read Done as "the task finished" and demanded false, which would
	// have produced an envelope the phone throws away.
	if !out.Done {
		t.Error("a hand-off that names its app should be done: Operator's own part is over")
	}
	if out.HandedOffTo != "Uber" {
		t.Errorf("a handed-off outcome does not name the app it went to: %q", out.HandedOffTo)
	}
}

func TestARealOutcomeBelowTheClaimDemotesTheAdapterForEveryoneAfterwards(t *testing.T) {
	// Outcome telemetry from real use is better evidence than a synthetic
	// test and costs nothing extra, so the runner feeds it back itself.
	rec := newRecorder("uber", []manifest.Verb{manifest.Book}, manifest.HandsOff)
	run, reg := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "uber", Verb: manifest.Book})
	preview, _ := run.Preview(ctx, plan)
	if _, err := run.Execute(ctx, plan, preview.Confirmed()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	ceiling, proven, _ := reg.EffectiveCeiling("uber")
	if ceiling != manifest.HandsOff || !proven {
		t.Fatalf("after a hands_off outcome the adapter reads %s proven=%v, want hands_off proven=true", ceiling, proven)
	}
}

// ---- the doors the registry closes stay closed here ---------------------

func TestASwitchedOffAdapterCannotBeResolvedOrExecuted(t *testing.T) {
	rec := newRecorder("whatsapp", []manifest.Verb{manifest.Send}, manifest.Completes)
	run, reg := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, sendIntent("whatsapp"))
	preview, _ := run.Preview(ctx, plan)

	_ = reg.Disable("whatsapp", "ban risk spiked")

	if _, err := run.Resolve(ctx, sendIntent("whatsapp")); !errors.Is(err, registry.ErrAdapterDisabled) {
		t.Errorf("Resolve on a switched-off adapter returned %v", err)
	}
	if _, err := run.Execute(ctx, plan, preview.Confirmed()); !errors.Is(err, registry.ErrAdapterDisabled) {
		t.Errorf("Execute on a switched-off adapter returned %v; a confirmation from before the switch must not survive it", err)
	}
}

func TestAnAdapterIsNeverAskedForAVerbItDoesNotDeclare(t *testing.T) {
	rec := newRecorder("spotify", []manifest.Verb{manifest.Play, manifest.Read}, manifest.Completes)
	run, _ := runnerWith(t, rec)

	_, err := run.Resolve(context.Background(), adapter.Intent{AdapterID: "spotify", Verb: manifest.Send, Body: "hi"})
	if !errors.Is(err, ErrVerbNotOffered) {
		t.Fatalf("Resolve for an undeclared verb returned %v, want ErrVerbNotOffered", err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("the adapter was called anyway: %v", rec.calls)
	}
}

// The same check, at the other door. Resolve refuses an undeclared verb and
// always has. Execute never did — it looked the adapter up by the plan's id
// and ran whatever the plan said, so a plan that never came from Resolve could
// carry any verb at all straight into the adapter.
//
// Nothing in `serve` can do this today: flow.Service keeps the resolved plan
// on the companion and a phone confirms by fingerprint alone, so every plan
// Execute sees has already been through Resolve. But Execute is exported and
// the proving commands call it directly, and this is the check whose absence
// only matters on the day someone adds a second way in. Verbs that need no
// preview are the exposed ones — an irreversible verb is stopped by the
// missing confirmation, so `read` is what actually slips through.
func TestAnUndeclaredVerbIsRefusedAtExecuteTooNotJustAtResolve(t *testing.T) {
	rec := newRecorder("spotify", []manifest.Verb{manifest.Play}, manifest.Completes)
	run, _ := runnerWith(t, rec)

	plan := adapter.Plan{AdapterID: "spotify", Verb: manifest.Read, Summary: "a verb spotify never declared"}
	out, err := run.Execute(context.Background(), plan, Confirmation{})
	if !errors.Is(err, ErrVerbNotOffered) {
		t.Fatalf("Execute for an undeclared verb returned (%+v, %v), want ErrVerbNotOffered", out, err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("the adapter ran a verb it never declared: %v", rec.calls)
	}
}

// And at the third door. Preview is the least dangerous of the three —
// nothing irreversible happens — but it is the one the user actually sees.
// Letting an undeclared verb through here means Operator renders a confirm
// sheet for a capability nobody agreed to, the user taps confirm, and only
// then does Execute refuse it. Failing at the first door someone knocks on
// is both safer and the only version that reads honestly.
func TestAnUndeclaredVerbIsRefusedAtPreviewToo(t *testing.T) {
	rec := newRecorder("spotify", []manifest.Verb{manifest.Play}, manifest.Completes)
	run, _ := runnerWith(t, rec)

	plan := adapter.Plan{AdapterID: "spotify", Verb: manifest.Send, Summary: "a verb spotify never declared"}
	if _, err := run.Preview(context.Background(), plan); !errors.Is(err, ErrVerbNotOffered) {
		t.Fatalf("Preview for an undeclared verb returned %v, want ErrVerbNotOffered", err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("the adapter was asked to preview a verb it never declared: %v", rec.calls)
	}
}

// ---- revoke has to be provable ------------------------------------------

func TestRevokeReachesTheAdapterAndThenTheAdapterIsGone(t *testing.T) {
	// Disconnect, delete tokens, prove it. "Prove it" here means the adapter
	// is no longer reachable afterwards, not merely that a flag flipped.
	rec := newRecorder("notion", []manifest.Verb{manifest.Read}, manifest.Completes)
	run, reg := runnerWith(t, rec)

	if err := run.Revoke(context.Background(), "notion"); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	found := false
	for _, c := range rec.calls {
		if c == "revoke" {
			found = true
		}
	}
	if !found {
		t.Error("Revoke never reached the adapter")
	}
	if _, err := reg.Get("notion"); err == nil {
		t.Error("a revoked adapter is still reachable from the registry")
	}
}

func TestAFailedExecuteIsReportedAsFailedNotAsHandedOff(t *testing.T) {
	// Handed off and failed are different things to a user, and the state
	// marks for them are deliberately different. Never blur one into the other.
	rec := newRecorder("sms", []manifest.Verb{manifest.Send}, manifest.Completes)
	rec.execErr = errors.New("carrier rejected the message")
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, sendIntent("sms"))
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())

	if err == nil {
		t.Fatal("a failing Execute returned no error")
	}
	if out.HandedOffTo != "" {
		t.Errorf("a failure was reported as handed off to %q", out.HandedOffTo)
	}
	if out.Done {
		t.Error("a failure was reported as done")
	}
}

// A declared ceiling is a cap, not a suggestion: a real run can lower it and
// may never raise it. That rule was only enforced on the way down. Nothing
// stopped an adapter's own Execute from reporting a ceiling higher than the
// manifest it ships with, and that number goes straight to the phone.
func TestAnOutcomeCannotClaimMoreThanTheManifestDeclares(t *testing.T) {
	rec := newRecorder("youtube", []manifest.Verb{manifest.Play}, manifest.Completes)
	rec.m.Ceiling = manifest.HandsOff
	rec.handTo = "YouTube"
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{AdapterID: "youtube", Verb: manifest.Play, Subject: "bicycle repair"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out.Reached != manifest.HandsOff {
		t.Errorf("outcome ceiling = %s, want hands_off — an adapter may not out-claim its own manifest", out.Reached)
	}
	// Done stays true here. In this codebase Done means Operator finished
	// its own part, not that the user's task is over — that is why a
	// hand-off draft (handoff.DraftOutcome) is hands_off and done at once.
	// The phone's codec enforces exactly that: it throws away any result
	// that names an app it handed to while saying it is not done
	// (ProtocolCodec.kt:187).
	if !out.Done || out.HandedOffTo != "YouTube" {
		t.Errorf("outcome=%+v, want done with the app it handed to still named", out)
	}
}

// The phone rejects a result whose ceiling is anything but hands_off while
// it names an app it handed control to (ProtocolCodec.kt:187). Nothing on
// the companion side checked that, so an adapter could send an envelope the
// phone silently threw away and the user would see no answer at all.
func TestAnOutcomeThatNamesAnAppIsAlwaysAHandOff(t *testing.T) {
	rec := newRecorder("maps", []manifest.Verb{manifest.Read}, manifest.Completes)
	rec.handTo = "Google Maps"
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "maps", Verb: manifest.Read, Subject: "navigate to SFO"})
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.Reached != manifest.HandsOff {
		t.Errorf("ceiling = %s with %q named; only a hand-off may name an app", out.Reached, out.HandedOffTo)
	}
	if !out.Done || out.HandedOffTo != "Google Maps" {
		t.Errorf("outcome=%+v, want done and still naming Google Maps", out)
	}
}

// The mirror case: pulled down to hands_off with no app named. Here Done
// must go false, because the codec throws away a hands_off result that says
// it is done without naming where it went.
func TestClampingWithNoAppNamedMarksItNotDone(t *testing.T) {
	rec := newRecorder("youtube", []manifest.Verb{manifest.Read}, manifest.Completes)
	rec.m.Ceiling = manifest.HandsOff
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "youtube", Verb: manifest.Read, Subject: "bicycle repair"})
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.Reached != manifest.HandsOff || out.Done || out.HandedOffTo != "" {
		t.Fatalf("outcome=%+v, want hands_off, not done, no app named", out)
	}
}

// Clamping down must not quietly clamp up. An adapter that honestly reports
// less than it is allowed keeps its lower number.
func TestClampingNeverRaisesAnHonestlyLowOutcome(t *testing.T) {
	rec := newRecorder("uber", []manifest.Verb{manifest.Book}, manifest.HandsOff)
	rec.handTo = "Uber"
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "uber", Verb: manifest.Book, Subject: "ride home"})
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.Reached != manifest.HandsOff || out.HandedOffTo != "Uber" {
		t.Fatalf("outcome=%+v, want the adapter's own hands_off result untouched", out)
	}
}

// ---- a misbehaving adapter must not reach the wire or the registry ------

// An adapter that forgets to fill in Reached hands back the empty string.
// The clamp compares ceilings by rank, and an unknown ceiling ranks 0, which
// is below every real one — so "weaker wins" lets the garbage through
// untouched. The phone then drops the envelope (ProtocolCodec.kt:185 checks
// the ceiling against a fixed set) AND telemetry has already recorded the
// empty string as this adapter's measured ceiling, so one adapter bug both
// loses the answer and poisons the adapter's record until enough good runs
// wash it out. Refusing it outright is the only honest option: the runner
// cannot guess what the adapter meant.
func TestAnOutcomeWithAnUnknownCeilingIsRefusedOutright(t *testing.T) {
	rec := newRecorder("broken", []manifest.Verb{manifest.Read}, manifest.Completes)
	rec.exact = &adapter.Outcome{Done: true} // Reached left empty
	run, reg := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "broken", Verb: manifest.Read, Subject: "anything"})
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if !errors.Is(err, ErrUnknownCeiling) {
		t.Fatalf("Execute returned outcome=%+v err=%v, want ErrUnknownCeiling", out, err)
	}
	ceiling, proven, _ := reg.EffectiveCeiling("broken")
	if proven {
		t.Errorf("effective ceiling = %q, recorded as proven; the bad run was measured instead of refused", ceiling)
	}
}

// The third rejection clause on the phone: a hands_off result that says it is
// done must name where it went. The clamp only forced Done true when an app
// WAS named; it never handled the case where the ceiling is already hands_off
// and no app is named, because nothing changed so neither switch arm fired.
// An adapter that simply omits the app name produces exactly that shape.
func TestAHandOffThatNamesNoAppIsNotDone(t *testing.T) {
	rec := newRecorder("youtube", []manifest.Verb{manifest.Read}, manifest.HandsOff)
	rec.m.Ceiling = manifest.HandsOff
	rec.exact = &adapter.Outcome{Reached: manifest.HandsOff, Done: true} // HandedOffTo omitted
	run, _ := runnerWith(t, rec)
	ctx := context.Background()

	plan, _ := run.Resolve(ctx, adapter.Intent{AdapterID: "youtube", Verb: manifest.Read, Subject: "bicycle repair"})
	preview, _ := run.Preview(ctx, plan)
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.Done {
		t.Errorf("outcome=%+v: hands_off claims done while naming no app; the phone discards this", out)
	}
}
