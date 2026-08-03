// Package execution drives a single request through an adapter: resolve an
// intent to a plan, preview it when the verb demands one, execute it only
// once that preview has been confirmed, and feed the real outcome back into
// the registry. It never holds an adapter directly — every step looks the
// adapter up through the registry, so a kill switch flipped mid-request
// takes effect immediately.
package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/verification"
)

// ErrPreviewRequired is returned when Execute is called for a verb that
// requires a preview, and the confirmation given does not match the plan.
var ErrPreviewRequired = errors.New("preview required before execute")

// ErrUnknownCeiling is returned when an adapter's Execute reports a ceiling
// that is not one of the three known ones. It is refused rather than
// repaired, because the runner cannot guess what the adapter meant, and a
// silent repair would hide the adapter bug that caused it.
var ErrUnknownCeiling = errors.New("adapter reported an unknown ceiling")

// ErrVerbNotOffered is returned when Resolve is asked for a verb the
// adapter's manifest does not declare. The adapter is never called in that
// case.
var ErrVerbNotOffered = errors.New("adapter does not offer this verb")

// Confirmation is proof that the user confirmed a specific plan's preview.
// It carries the plan's fingerprint, not the plan itself, so Execute can
// check it was made against exactly this plan rather than a similar one.
// The zero value never satisfies a preview requirement.
type Confirmation struct {
	fingerprint string
}

// Preview wraps an adapter's preview with the ability to turn itself into a
// Confirmation once the user has agreed to it.
type Preview struct {
	adapter.Preview
}

// Confirmed records that the user confirmed this preview, producing the
// Confirmation Execute requires for a verb that needs one.
func (p Preview) Confirmed() Confirmation {
	return Confirmation{fingerprint: p.Fingerprint()}
}

// Runner drives intents through adapters looked up from a registry.
type Runner struct {
	reg    *registry.Registry
	tel    *verification.Telemetry
	logger *slog.Logger
}

// New returns a Runner backed by the given registry.
func New(reg *registry.Registry) *Runner {
	return &Runner{reg: reg, tel: verification.NewTelemetry(reg), logger: slog.Default()}
}

// Telemetry returns the runner's own live Telemetry — the one every real
// Execute call feeds through Observe, carrying whatever production
// observations have actually accumulated. It exists so a background
// watcher (internal/capability/verification/alerts) can read the real
// signal instead of being handed a second, freshly-built Telemetry that
// would never see a single production outcome, which would be the exact
// same bug — a computed signal nobody reads — in a new place.
func (r *Runner) Telemetry() *verification.Telemetry {
	return r.tel
}

// Resolve looks the adapter up through the registry (so a disabled adapter
// fails here) and asks it to resolve the intent into a plan, refusing a
// verb the adapter's manifest does not declare without calling the
// adapter at all.
func (r *Runner) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	a, err := r.reg.Get(in.AdapterID)
	if err != nil {
		return adapter.Plan{}, err
	}
	if !a.Describe().Allows(in.Verb) {
		return adapter.Plan{}, fmt.Errorf("%w: %s does not offer %s", ErrVerbNotOffered, in.AdapterID, in.Verb)
	}
	// See the gate comment in Execute: the billing-gated maps adapter makes
	// its paid API call right here in Resolve, so this door needs the same
	// refusal, not just the last one.
	if err := a.Describe().CheckGates(); err != nil {
		return adapter.Plan{}, err
	}
	return a.Resolve(ctx, in)
}

// Preview looks the adapter up through the registry and asks it to preview
// the plan, wrapping the result so it can be confirmed.
func (r *Runner) Preview(ctx context.Context, plan adapter.Plan) (Preview, error) {
	a, err := r.reg.Get(plan.AdapterID)
	if err != nil {
		return Preview{}, err
	}
	// Refuse here rather than one door later, so a capability nobody
	// declared never gets as far as a sheet the user is asked to confirm.
	if !a.Describe().Allows(plan.Verb) {
		return Preview{}, fmt.Errorf("%w: %s does not offer %s", ErrVerbNotOffered, plan.AdapterID, plan.Verb)
	}
	// See the gate comment in Execute: every door into a gated adapter
	// refuses, not just the last one.
	if err := a.Describe().CheckGates(); err != nil {
		return Preview{}, err
	}
	p, err := a.Preview(ctx, plan)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Preview: p}, nil
}

// Execute re-looks-up the adapter through the registry, so a confirmation
// made before a kill switch does not survive it. For a verb that requires
// a preview, it demands a Confirmation whose fingerprint matches the
// plan's; otherwise it returns ErrPreviewRequired without calling the
// adapter. On success it feeds the real outcome into Telemetry.Observe
// rather than the registry directly: Observe both records the measured
// ceiling and counts the run toward the shortfall alert, so real traffic —
// the best rot signal there is — actually reaches the watchdog instead of
// bypassing it. On adapter failure it returns the error and an Outcome
// that reports neither done nor handed off.
func (r *Runner) Execute(ctx context.Context, plan adapter.Plan, confirm Confirmation) (adapter.Outcome, error) {
	a, err := r.reg.Get(plan.AdapterID)
	if err != nil {
		return adapter.Outcome{}, err
	}

	// The same check Resolve makes, at the second door. A plan that came
	// from Resolve has already passed this, so in `serve` it never fires.
	// It is here for every other way in — the proving commands call Execute
	// directly — because a verb an adapter never declared is one nobody
	// reviewed, and the preview check below cannot stop it: verbs that need
	// no preview walk straight past.
	if !a.Describe().Allows(plan.Verb) {
		return adapter.Outcome{}, fmt.Errorf("%w: %s does not offer %s", ErrVerbNotOffered, plan.AdapterID, plan.Verb)
	}

	// A gate is the manifest's way of saying "this request needs a checkpoint
	// before it runs" — someone's approval, or a billing account that agreed
	// to be charged. Nothing in this product can clear one today: there is no
	// approval flow and no billing consent mechanism anywhere in the runner,
	// the flow, or the handler. So a declared checkpoint that nothing can
	// clear must stop the request rather than be silently ignored — refusing
	// to run is recoverable, and a charge on someone's account is not. This
	// has to happen before the preview check below, not after: Read needs no
	// preview, and the one adapter that actually declares a billing gate only
	// offers Read, so a gate check placed after the preview check would never
	// fire for the adapter it exists to stop. Whether a gate is enforced must
	// not depend on which verb was asked for. The whole list is scanned
	// because GateNone sitting alongside a real gate is not permission — the
	// strictest entry decides.
	if err := a.Describe().CheckGates(); err != nil {
		return adapter.Outcome{}, err
	}

	if plan.Verb.RequiresPreview() {
		if confirm.fingerprint == "" || confirm.fingerprint != plan.Fingerprint() {
			return adapter.Outcome{}, ErrPreviewRequired
		}
	}

	out, err := a.Execute(ctx, plan)
	if err != nil {
		return adapter.Outcome{}, err
	}

	// An adapter's own Execute cannot be trusted to out-claim its own
	// manifest: the manifest's declared ceiling is what the adapter itself
	// promised, and this number goes straight to the phone, where the user
	// reads it as fact. If the adapter reports a ceiling that isn't one of
	// the three real ones (including a plain empty string), the runner can't
	// rank it or clamp it — it isn't a ceiling at all, just a bug in the
	// adapter — so it is refused outright, before telemetry ever sees it,
	// rather than silently let through or repaired into a guess.
	if !out.Reached.Valid() {
		return adapter.Outcome{}, fmt.Errorf("%w: adapter %s reported %q", ErrUnknownCeiling, plan.AdapterID, out.Reached)
	}

	// Otherwise, before telemetry ever sees this outcome, clamp Reached to
	// the adapter's declared ceiling — the same "can only pull down, never
	// push up" rule already applied to measured history is applied here to a
	// single run.
	//
	// Naming an app it handed control to IS a hand-off, no matter what the
	// adapter or its manifest claimed, so a non-empty HandedOffTo forces
	// Reached to manifest.HandsOff regardless of the clamp above.
	//
	// Done follows from that. A hand-off that names an app has finished
	// Operator's own part of the work (the app is what happens next), so it
	// stays done. A result that ends up hands_off but names no app is never
	// done, whether that shape came from the adapter itself or from the
	// clamp pulling Reached down to hands_off — "done" with nothing to hand
	// off to is not a real finish. If the clamp pulled Reached down to
	// something other than hands_off, Done can no longer be true either,
	// because the adapter's own claim about finishing did not hold up.
	// Otherwise Done is left exactly as the adapter reported it.
	//
	// This isn't a style choice: the phone-side decoder enforces this exact
	// shape and silently discards anything that violates it — see
	// ProtocolCodec.kt:187, which fails the envelope if hands_off+done has no
	// named app, if a named app appears without hands_off, or if an app is
	// named while Done is false.
	clamped := a.Describe().Ceiling.AtMost(out.Reached)
	handedOff := out.HandedOffTo != ""
	if handedOff {
		clamped = manifest.HandsOff
	}
	done := out.Done
	switch {
	case handedOff:
		done = true
	case clamped == manifest.HandsOff:
		done = false
	case clamped != out.Reached:
		done = false
	}
	out = adapter.Outcome{
		Reached:     clamped,
		Done:        done,
		HandedOffTo: out.HandedOffTo,
		Detail:      out.Detail,
	}

	// The adapter has already done the real, irreversible thing by this
	// point — the message is sent, the reminder is created. Observe is pure
	// bookkeeping: it only reads the registry and records a measured
	// ceiling (verification.go:326-334), touching nothing in the outside
	// world. A failure here (most plausibly a kill-switch race, where the
	// adapter is unregistered while this run is in flight) is a failure to
	// write down what happened, never a failure of the thing itself — so it
	// is logged, not surfaced, and the real outcome is still returned.
	if err := r.tel.Observe(plan.AdapterID, out.Reached); err != nil {
		r.logger.Error("[capability-execution] bookkeeping failed after execute succeeded", "adapter_id", plan.AdapterID, "verb", plan.Verb, "error_class", fmt.Sprintf("%T", err))
	}
	return out, nil
}

// Describe answers what an adapter declares about itself. The flow service
// needs a manifest to run the consent gate and has no registry of its own.
func (r *Runner) Describe(adapterID string) (manifest.Manifest, error) {
	a, err := r.reg.Get(adapterID)
	if err != nil {
		return manifest.Manifest{}, err
	}
	return a.Describe(), nil
}

// Report returns what real traffic routed through this Runner has shown
// about one adapter: how many executions ran, how many fell short of the
// declared ceiling, and whether the shortfall streak has crossed the alert
// threshold.
func (r *Runner) Report(adapterID string) (verification.Report, error) {
	return r.tel.Report(adapterID)
}

// Revoke calls the adapter's own Revoke and then removes it from the
// registry, so it is unreachable afterwards — proof revocation happened,
// not merely a flag flipped.
func (r *Runner) Revoke(ctx context.Context, id string) error {
	a, err := r.reg.Get(id)
	if err != nil {
		return err
	}
	if err := a.Revoke(ctx); err != nil {
		return err
	}
	return r.reg.Unregister(id)
}
