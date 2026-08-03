// Package verification measures whether an adapter's declared ceiling holds
// up against reality. A declared ceiling is a claim; a measurement is a
// fact, and a measurement can only demote a claim, never promote it — the
// registry's EffectiveCeiling already enforces that one-way door.
//
// Verification splits into two tiers because half of the adapters run on
// credentials Operator itself holds (RT-1/2/3: shared, so an unattended
// nightly run is fine) and half are bound to one real user's own account and
// hardware (RT-4/5/6: nothing unattended, ever). Tier 1 gets a scheduled
// runner that proves reads and writes into containers it made itself. Tier 2
// gets a self-directed loop at connect time, plus a cheap read-only
// heartbeat at wake that proves the session is still alive without ever
// proving a send. A third piece, Telemetry, folds real production outcomes
// back in as the primary rot signal, with the scheduled runs as a backstop.
package verification

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// ---- errors ---------------------------------------------------------------

var (
	// ErrForeignContainer is returned when an unattended run would mutate a
	// container the adapter did not create itself — a nightly job quietly
	// editing somebody's real page is exactly the failure this stops.
	ErrForeignContainer = errors.New("verification: refuses to mutate a container the adapter did not create")

	// ErrTierMismatch is returned when a probe is handed to the wrong
	// tier's runner: a tier-2, account-bound adapter given to the
	// unattended tier-1 runner, or a tier-1 adapter given to the tier-2
	// self-directed loop.
	ErrTierMismatch = errors.New("verification: adapter tier does not match this runner")

	// ErrNotSelf is returned when the tier-2 connect loop is asked to
	// prove a send against anyone other than the user's own account.
	ErrNotSelf = errors.New("verification: target is not the user's own account")

	// ErrHeartbeatMustNotMutate is returned when the tier-2 heartbeat is
	// given a verb that changes something. The heartbeat exists to prove a
	// session is alive as cheaply as possible; it must never be the thing
	// that fires a real send.
	ErrHeartbeatMustNotMutate = errors.New("verification: heartbeat verb must not mutate")
)

// ---- tiers ------------------------------------------------------------

// Tier is which verification regime an adapter falls under.
type Tier int

const (
	// Tier1 adapters run on credentials Operator holds, so an unattended
	// scheduled run is fine.
	Tier1 Tier = 1
	// Tier2 adapters are bound to one real user's account and hardware.
	// Nothing about them runs unattended, ever.
	Tier2 Tier = 2
)

// TierOf reports which tier a manifest's runtime falls under. RT-1/2/3 are
// tier 1. Everything else — RT-4/5/6 — is tier 2: RT-4 is not named
// alongside RT-5/6 in the plan, but it runs on-device against the user's own
// accounts, and an unattended job that fires a reply into a real
// conversation is exactly what tier 2 exists to stop.
func TierOf(m manifest.Manifest) Tier {
	switch m.Runtime {
	case manifest.RT1, manifest.RT2, manifest.RT3:
		return Tier1
	default:
		return Tier2
	}
}

// ---- cadence ------------------------------------------------------------

// Cadence is how often a tier-1 adapter is put through the scheduled
// runner.
type Cadence string

const (
	Nightly  Cadence = "nightly"
	Weekly   Cadence = "weekly"
	OnDemand Cadence = "on_demand" // the pre-wave-ship sweep, run by hand
	Never    Cadence = "never"
)

// ScheduleFor reports how often a manifest should be verified. Tier 2 is
// never scheduled — nothing account-bound runs unattended. Tier 1 runs
// nightly when it carries real traffic and weekly otherwise: forty-odd
// third-party test accounts is real ongoing cost, so the cadence is
// proportionate rather than "everything, every night".
func ScheduleFor(m manifest.Manifest, carriesRealTraffic bool) Cadence {
	if TierOf(m) == Tier2 {
		return Never
	}
	if carriesRealTraffic {
		return Nightly
	}
	return Weekly
}

// ---- accounts and probes -------------------------------------------------

// Account is which account a verification run is made against.
type Account string

const (
	// OperatorTest is a credential Operator itself holds, used by
	// unattended tier-1 runs.
	OperatorTest Account = "operator_test"
	// RealUser is one real user's own account.
	RealUser Account = "real_user"
)

// Probe is one tier-1 verification request.
type Probe struct {
	AdapterID                 string
	Verb                      manifest.Verb
	Container                 string
	ContainerCreatedByAdapter bool
	Account                   Account
}

// Target is who a tier-2 connect-loop probe is aimed at.
type Target struct {
	Handle string
	IsSelf bool
}

// mutatingVerbs is the explicit set of verbs that change something in the
// target account or document. This is deliberately not Verb.RequiresPreview:
// RequiresPreview asks "does a human need to see this before it runs",
// which excludes Write (a background edit needs no user-facing preview) —
// but Write still mutates a container, so it still needs the foreign-
// container and heartbeat guards below.
var mutatingVerbs = map[manifest.Verb]bool{
	manifest.Write:  true,
	manifest.Send:   true,
	manifest.Order:  true,
	manifest.Book:   true,
	manifest.Cancel: true,
	manifest.Modify: true,
}

func isMutatingVerb(v manifest.Verb) bool {
	return mutatingVerbs[v]
}

// ---- tier 1: the unattended scheduled runner -----------------------------

// Tier1Runner drives a probe through a tier-1 (shared-credential) adapter
// unattended, and records what it actually reached.
type Tier1Runner struct {
	reg *registry.Registry
}

// NewTier1 returns a Tier1Runner backed by the given registry.
func NewTier1(reg *registry.Registry) *Tier1Runner {
	return &Tier1Runner{reg: reg}
}

// Run drives one probe through its adapter. A tier-2 adapter is refused with
// ErrTierMismatch before it is ever touched. A mutating verb whose container
// was not created by the adapter itself is refused with ErrForeignContainer,
// also without calling the adapter — reads are always allowed, including
// against a real user's account, because reading real data unattended risks
// nothing a nightly job can break. On success it resolves and executes the
// probe (no preview: this is the unattended verification path, not a user
// action) and records the ceiling actually reached.
func (t *Tier1Runner) Run(ctx context.Context, p Probe) (adapter.Outcome, error) {
	a, err := t.reg.Get(p.AdapterID)
	if err != nil {
		return adapter.Outcome{}, err
	}
	if TierOf(a.Describe()) != Tier1 {
		return adapter.Outcome{}, fmt.Errorf("%w: %s is not a tier-1 adapter", ErrTierMismatch, p.AdapterID)
	}
	if isMutatingVerb(p.Verb) && !p.ContainerCreatedByAdapter {
		return adapter.Outcome{}, fmt.Errorf("%w: %s into %q", ErrForeignContainer, p.Verb, p.Container)
	}
	// See the gate comment on execution.Runner.Execute: a declared checkpoint
	// nothing can clear must stop the request before the adapter is touched
	// at all, not just before Execute. This is the unattended path, so there
	// is nobody watching to notice a charge here — the check has to be first.
	if err := a.Describe().CheckGates(); err != nil {
		return adapter.Outcome{}, err
	}

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: p.AdapterID, Verb: p.Verb})
	if err != nil {
		return adapter.Outcome{}, err
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		return adapter.Outcome{}, err
	}
	if err := t.reg.RecordMeasuredCeiling(p.AdapterID, out.Reached); err != nil {
		return out, err
	}
	return out, nil
}

// ---- tier 2: the self-directed connect loop and the wake heartbeat -------

// Tier2Runner verifies tier-2 (account-bound) adapters two ways: a
// self-directed loop run once at connect time, which is the only place an
// account-bound ceiling is ever captured, and a cheap read-only heartbeat
// run at wake to confirm the session is still alive.
type Tier2Runner struct {
	reg *registry.Registry
}

// NewTier2 returns a Tier2Runner backed by the given registry.
func NewTier2(reg *registry.Registry) *Tier2Runner {
	return &Tier2Runner{reg: reg}
}

// ConnectLoop drives a send at connect time against the user's own account —
// Saved Messages, Note to Self, their own number — proving send end-to-end
// while reaching no third party. A tier-1 adapter is refused with
// ErrTierMismatch and a target that is not the user's own account is
// refused with ErrNotSelf, in both cases without calling the adapter. On
// success it records the ceiling reached, which is where an account-bound
// adapter's ceiling gets captured at all.
func (t *Tier2Runner) ConnectLoop(ctx context.Context, adapterID string, target Target) (adapter.Outcome, error) {
	a, err := t.reg.Get(adapterID)
	if err != nil {
		return adapter.Outcome{}, err
	}
	if TierOf(a.Describe()) != Tier2 {
		return adapter.Outcome{}, fmt.Errorf("%w: %s is not a tier-2 adapter", ErrTierMismatch, adapterID)
	}
	if !target.IsSelf {
		return adapter.Outcome{}, fmt.Errorf("%w: %s", ErrNotSelf, target.Handle)
	}
	// See the gate comment in Tier1Runner.Run: the check has to come before
	// the adapter is touched, not just before the send.
	if err := a.Describe().CheckGates(); err != nil {
		return adapter.Outcome{}, err
	}

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: adapterID, Verb: manifest.Send, Handle: target.Handle})
	if err != nil {
		return adapter.Outcome{}, err
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		return adapter.Outcome{}, err
	}
	if err := t.reg.RecordMeasuredCeiling(adapterID, out.Reached); err != nil {
		return out, err
	}
	return out, nil
}

// Heartbeat proves a tier-2 adapter's session is still alive with the
// cheapest non-mutating call the adapter has. A mutating verb is refused
// with ErrHeartbeatMustNotMutate without calling the adapter — the
// heartbeat must never be the thing that fires a real send. A healthy read
// records nothing: it is a liveness check, not a measurement, and letting it
// record would make a read prove a send. A failing read switches the
// adapter off via the registry, with a reason naming the failure, so the
// next task fails at second 0.2 with a clean prompt rather than dying
// mid-task.
func (t *Tier2Runner) Heartbeat(ctx context.Context, adapterID string, verb manifest.Verb) error {
	if isMutatingVerb(verb) {
		return fmt.Errorf("%w: %s", ErrHeartbeatMustNotMutate, verb)
	}

	a, err := t.reg.Get(adapterID)
	if err != nil {
		return err
	}
	// See the gate comment in Tier1Runner.Run. Unlike the failures below, a
	// gate refusal must not disable the adapter: disabling is what a *failed*
	// heartbeat means (the session died), but a gate is not a failure — the
	// adapter is fine and untouched, so it stays registered and enabled.
	if err := a.Describe().CheckGates(); err != nil {
		return err
	}

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: adapterID, Verb: verb})
	if err != nil {
		_ = t.reg.Disable(adapterID, fmt.Sprintf("heartbeat: resolve failed: %v", err))
		return err
	}
	if _, err := a.Execute(ctx, plan); err != nil {
		_ = t.reg.Disable(adapterID, fmt.Sprintf("heartbeat: session check failed: %v", err))
		return err
	}
	return nil
}

// ---- telemetry: real traffic as the primary rot signal --------------------

// shortfallAlertThreshold is how many consecutive observations below an
// adapter's declared ceiling it takes to raise an alert. Demoting on the
// first shortfall keeps the shown ceiling honest; alerting on the first one
// would drown the signal in one-off flakes, so the threshold sits a few
// runs in rather than at one.
const shortfallAlertThreshold = 3

// Report summarizes what real traffic has shown about one adapter.
type Report struct {
	AdapterID    string
	Observations int
	BelowClaim   int
	Alert        bool
}

type telemetryEntry struct {
	observations int
	belowClaim   int
	streak       int // consecutive observations below the declared ceiling
}

// Telemetry folds real production outcomes back into the registry as the
// primary rot signal, with the scheduled tier-1 runs as a backstop. Safe for
// concurrent use.
type Telemetry struct {
	mu      sync.RWMutex
	reg     *registry.Registry
	entries map[string]*telemetryEntry
}

// NewTelemetry returns a Telemetry backed by the given registry.
func NewTelemetry(reg *registry.Registry) *Telemetry {
	return &Telemetry{reg: reg, entries: make(map[string]*telemetryEntry)}
}

// Observe records one real outcome. It always calls
// Registry.RecordMeasuredCeiling, which already clamps with Ceiling.AtMost
// so a good run can never promote a declared ceiling — only demote it. It
// also tracks, against the adapter's declared claim, whether this
// observation fell short and how long the current streak of shortfalls is,
// for Report to summarize. An unknown adapter is an error, checked before
// anything is recorded.
func (t *Telemetry) Observe(adapterID string, reached manifest.Ceiling) error {
	a, err := t.reg.Get(adapterID)
	if err != nil {
		return err
	}
	declared := a.Describe().Ceiling

	if err := t.reg.RecordMeasuredCeiling(adapterID, reached); err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[adapterID]
	if !ok {
		e = &telemetryEntry{}
		t.entries[adapterID] = e
	}
	e.observations++
	if reached.Rank() < declared.Rank() {
		e.belowClaim++
		e.streak++
	} else {
		e.streak = 0
	}
	return nil
}

// Report returns what real traffic has shown about one adapter so far:
// how many observations came in, how many fell below the declared ceiling,
// and whether the current streak of shortfalls has crossed the alert
// threshold. An adapter with no observations yet reports zeros, not an
// error; an adapter the registry has never heard of is an error.
func (t *Telemetry) Report(adapterID string) (Report, error) {
	if _, err := t.reg.Get(adapterID); err != nil {
		return Report{}, err
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.entries[adapterID]
	if !ok {
		return Report{AdapterID: adapterID}, nil
	}
	return Report{
		AdapterID:    adapterID,
		Observations: e.observations,
		BelowClaim:   e.belowClaim,
		Alert:        e.streak >= shortfallAlertThreshold,
	}, nil
}
