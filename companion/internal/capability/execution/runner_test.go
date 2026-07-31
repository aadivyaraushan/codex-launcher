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
	if out.Done {
		t.Error("a handed-off outcome claims it finished; after handing off we cannot know")
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
