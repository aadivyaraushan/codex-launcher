// Package registry holds the set of capability adapters the companion knows
// about: which ones exist, which are switched off (locally or by a remote
// kill list), and what ceiling each has actually been measured to reach.
// It is the only door to an adapter — execution never holds one directly.
package registry

import (
	"errors"
	"fmt"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ErrUnknownAdapter is returned for an id the registry has never seen.
var ErrUnknownAdapter = errors.New("unknown adapter")

// ErrAdapterDisabled is returned for an id that exists but is switched off,
// locally or by the remote kill list.
var ErrAdapterDisabled = errors.New("adapter disabled")

// ErrUnknownCeiling is returned when something tries to record a measured
// ceiling that is not one of the three real ones.
var ErrUnknownCeiling = errors.New("measured ceiling is not a real ceiling")

type entry struct {
	a        adapter.Adapter
	disabled bool
	reason   string
	measured *manifest.Ceiling

	// pendingCeiling and pendingCount track an in-progress attempt to climb
	// back to a higher ceiling after a fall. See RecordMeasuredCeiling.
	pendingCeiling *manifest.Ceiling
	pendingCount   int
}

// healingRunsRequired is how many consecutive measurements at a higher
// ceiling it takes for that ceiling to actually replace a lower one that is
// currently shown. Falling to a lower ceiling needs none of this — it takes
// effect on the first bad run. The asymmetry is deliberate: under-promising
// costs a user an unnecessary hand-off to do something by hand, which is
// annoying but recoverable. Over-promising tells them something happened
// automatically when it did not, which is the exact failure this whole
// product is built to avoid, so climbing back up has to survive more than
// one lucky run before it is believed.
const healingRunsRequired = 2

// Registry is safe for concurrent use: the companion is a server, and
// requests, background kill-list refreshes, and telemetry feedback all
// touch it from different goroutines.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// Register adds an adapter, refusing one whose manifest does not validate
// (which also refuses class C consent, since that fails validation) and
// refusing a duplicate id.
func (r *Registry) Register(a adapter.Adapter) error {
	m := a.Describe()
	if err := m.Validate(); err != nil {
		return fmt.Errorf("register %s: %w", m.ID, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[m.ID]; exists {
		return fmt.Errorf("register %s: an adapter with this id is already registered", m.ID)
	}
	r.entries[m.ID] = &entry{a: a}
	return nil
}

// Unregister removes an adapter outright, so it is unreachable afterwards.
// It is what Revoke calls once an adapter has cleaned itself up.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownAdapter, id)
	}
	delete(r.entries, id)
	return nil
}

// Get is the only door to a live adapter. It refuses an id that was never
// registered with ErrUnknownAdapter, and an id that exists but is switched
// off with ErrAdapterDisabled.
func (r *Registry) Get(id string) (adapter.Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAdapter, id)
	}
	if e.disabled {
		return nil, fmt.Errorf("%w: %s", ErrAdapterDisabled, id)
	}
	return e.a, nil
}

// ForVerb returns every enabled adapter that declares the given verb and
// runs on the given platform.
func (r *Registry) ForVerb(v manifest.Verb, p manifest.Platform) []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []adapter.Adapter
	for _, e := range r.entries {
		if e.disabled {
			continue
		}
		m := e.a.Describe()
		if !m.Allows(v) {
			continue
		}
		if !m.Platform.Includes(p) {
			continue
		}
		out = append(out, e.a)
	}
	return out
}

// Disable switches an adapter off locally, with a reason recorded for
// Disabled to report back.
func (r *Registry) Disable(id, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownAdapter, id)
	}
	e.disabled = true
	e.reason = reason
	return nil
}

// Enable switches an adapter back on.
func (r *Registry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownAdapter, id)
	}
	e.disabled = false
	e.reason = ""
	return nil
}

// Disabled reports whether an adapter is currently switched off, and why.
func (r *Registry) Disabled(id string) (reason string, off bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return "", false
	}
	return e.reason, e.disabled
}

// AdapterIDs returns the id of every adapter currently registered,
// enabled or not, in no particular order. It exists so a background
// watcher — the alert sweep in internal/capability/verification/alerts,
// today — can enumerate what to check without reaching past the registry
// into the adapters themselves. The return value is a fresh copy, not the
// registry's own map keys, so a caller mutating it cannot reach into the
// registry's internal state.
func (r *Registry) AdapterIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.entries))
	for id := range r.entries {
		ids = append(ids, id)
	}
	return ids
}

// KillEntry names one adapter the remote kill list wants switched off, and
// why.
type KillEntry struct {
	ID     string
	Reason string
}

// KillList is the whole truth about which adapters should be off. An
// adapter absent from the list is on.
type KillList struct {
	Entries []KillEntry
}

// ApplyKillList switches adapters on and off to match the list in one
// step, and returns the ids whose on/off state actually changed. An id on
// the list that this registry does not have is silently ignored — the
// cloud list covers every client version, and an older build simply does
// not have every adapter named on it.
func (r *Registry) ApplyKillList(list KillList) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	killed := make(map[string]string, len(list.Entries))
	for _, e := range list.Entries {
		killed[e.ID] = e.Reason
	}

	var changed []string
	for id, e := range r.entries {
		reason, shouldBeOff := killed[id]
		if shouldBeOff {
			e.reason = reason
			if !e.disabled {
				e.disabled = true
				changed = append(changed, id)
			}
			continue
		}
		if e.disabled {
			e.disabled = false
			e.reason = ""
			changed = append(changed, id)
		}
	}
	return changed
}

// EffectiveCeiling returns the ceiling that should actually be shown to
// the user: the declared ceiling, demoted by whatever has been measured. A
// measurement can only lower the ceiling, never raise it. proven is true
// only once a measurement has been recorded.
func (r *Registry) EffectiveCeiling(id string) (ceiling manifest.Ceiling, proven bool, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return "", false, fmt.Errorf("%w: %s", ErrUnknownAdapter, id)
	}
	declared := e.a.Describe().Ceiling
	if e.measured == nil {
		return declared, false, nil
	}
	return declared.AtMost(*e.measured), true, nil
}

// RecordMeasuredCeiling records what a smoke test or a real execution
// actually reached, so EffectiveCeiling can demote the declared claim.
//
// An adapter with no measurement on file yet is not held back by any of
// this: its first measurement counts immediately, whatever level it is at.
// Once a measurement is on file, a run that falls below it replaces it at
// once — one bad run is enough to stop promising. A run that climbs above
// it only replaces it after healingRunsRequired consecutive runs land at
// that same higher level; a run that does not match — a fall, or simply a
// repeat of the level already shown — resets the count to zero, because
// good-bad-good is not two good runs, it is an adapter still failing.
func (r *Registry) RecordMeasuredCeiling(id string, measured manifest.Ceiling) error {
	// Refuse anything that is not one of the three real ceilings, before it
	// touches the record. This is not tidiness. Rank() scores an unrecognised
	// value as 0, below every real ceiling, so a bad measurement reads as a
	// fall and is stored as a permanent demotion — and unlike a real fall,
	// nothing about it is true. Every path that measures an adapter comes
	// through here, so this is the one place that has to hold.
	if !measured.Valid() {
		return fmt.Errorf("%w: %s reported %q", ErrUnknownCeiling, id, measured)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownAdapter, id)
	}

	m := measured
	if e.measured == nil {
		e.measured = &m
		e.pendingCeiling = nil
		e.pendingCount = 0
		return nil
	}

	switch {
	case m.Rank() < e.measured.Rank():
		// A fall. Immediate, and it cancels any recovery streak in
		// progress — a demotion mid-climb means the climb didn't happen.
		e.measured = &m
		e.pendingCeiling = nil
		e.pendingCount = 0
	case m.Rank() > e.measured.Rank():
		// An attempt to climb back above what's currently shown.
		if e.pendingCeiling != nil && *e.pendingCeiling == m {
			e.pendingCount++
		} else {
			e.pendingCeiling = &m
			e.pendingCount = 1
		}
		if e.pendingCount >= healingRunsRequired {
			e.measured = &m
			e.pendingCeiling = nil
			e.pendingCount = 0
		}
	default:
		// Same level as what's already shown: no fall to react to, and no
		// progress toward a higher one, so any recovery streak stops here.
		e.pendingCeiling = nil
		e.pendingCount = 0
	}
	return nil
}
