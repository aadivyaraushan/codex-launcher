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

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/verification"
)

// ErrPreviewRequired is returned when Execute is called for a verb that
// requires a preview, and the confirmation given does not match the plan.
var ErrPreviewRequired = errors.New("preview required before execute")

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
	reg *registry.Registry
	tel *verification.Telemetry
}

// New returns a Runner backed by the given registry.
func New(reg *registry.Registry) *Runner {
	return &Runner{reg: reg, tel: verification.NewTelemetry(reg)}
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
	return a.Resolve(ctx, in)
}

// Preview looks the adapter up through the registry and asks it to preview
// the plan, wrapping the result so it can be confirmed.
func (r *Runner) Preview(ctx context.Context, plan adapter.Plan) (Preview, error) {
	a, err := r.reg.Get(plan.AdapterID)
	if err != nil {
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

	if plan.Verb.RequiresPreview() {
		if confirm.fingerprint == "" || confirm.fingerprint != plan.Fingerprint() {
			return adapter.Outcome{}, ErrPreviewRequired
		}
	}

	out, err := a.Execute(ctx, plan)
	if err != nil {
		return adapter.Outcome{}, err
	}

	if err := r.tel.Observe(plan.AdapterID, out.Reached); err != nil {
		return out, err
	}
	return out, nil
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
