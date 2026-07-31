// Package consent is the consent framework for Operator's capability
// adapters. Class A adapters are official integrations and need no screen at
// all — the normal connect flow covers them. Class B adapters run on the
// user's own hardware against the user's own account, and each one gets its
// own screen naming that specific app's situation, its access, and its
// risk, because a generic warning teaches the user the screen says nothing.
// Class C adapters are never shipped, and no consent — however earnestly
// given — unlocks one.
package consent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Errors the consent gate returns. Every one wraps into a sentinel so a
// caller can test for "why" without parsing a message.
var (
	// ErrNeverShipped is returned for a class C adapter: no screen, no
	// grant, no path through Allow. The user cannot consent their way out
	// of a contract Operator signed.
	ErrNeverShipped = errors.New("consent: this adapter's class is never shipped")

	// ErrNoCopy is returned when a class B adapter has no per-app copy
	// registered. Shipping a generic warning in its place would be worse
	// than refusing outright.
	ErrNoCopy = errors.New("consent: no per-app copy registered for this adapter")

	// ErrNotGranted is returned by Allow when a class B adapter has not
	// been granted (or was revoked).
	ErrNotGranted = errors.New("consent: not granted")

	// ErrScreenNotShown is returned by Grant when the Screen handed to it
	// was not the one Screen produced for this exact adapter.
	ErrScreenNotShown = errors.New("consent: this is not the screen shown for this adapter")

	// ErrRevokeIncomplete is returned when a re-read after Revoke still
	// finds a token, local state, or the grant itself.
	ErrRevokeIncomplete = errors.New("consent: revoke left data behind")
)

// Absence names why an adapter that will never ship still has something to
// say when the user asks for it. The two kinds must never read the same,
// because only one of them might change: a policy holder can reopen a door
// it chose to close, but nothing turns "there is no way in" into a door.
type Absence string

const (
	// AbsenceChoice means a door exists and Operator chose not to walk
	// through it — a policy or contract it holds, not a technical wall.
	AbsenceChoice Absence = "a door exists, we chose not to"

	// AbsenceNoDoor means there is no way in at all — no API, no route,
	// nothing consent or a decision could open.
	AbsenceNoDoor Absence = "there is no way in at all"
)

// deny is the fixed refusal text shown on every class B screen. DESIGN.md
// is explicit that Deny is never hidden, so this is never empty and never
// buried under the grant button.
const deny = "Deny. Nothing connects, and you can grant it later from Settings if you change your mind."

// Copy is the per-app text a class B consent screen shows, registered up
// front rather than generated at request time — the situation, the access,
// and the risk are decisions made deliberately, not phrased on the fly.
// For a class C adapter, only Absence and Situation are used, by WhyNot.
type Copy struct {
	Situation string
	Grants    []string
	Risk      string
	Absence   Absence
}

// Screen is what Store.Screen renders: the exact text a user is shown
// before granting a class B adapter.
type Screen struct {
	AdapterID string
	Class     manifest.Consent
	Runtime   manifest.Runtime
	Situation string
	Grants    []string
	Risk      string
	Deny      string
}

// Grant is the record kept once a user grants an adapter: the exact wording
// they agreed to, so that once the copy changes later, Operator can still
// say what this particular user actually saw.
type Grant struct {
	AdapterID string
	Situation string
	GrantedAt time.Time
}

// Proof is the result of a Revoke: what the store re-read from the token
// and local-state vaults after deleting, not what the deleting code claims
// to have done.
type Proof struct {
	AdapterID      string
	GrantGone      bool
	TokensGone     bool
	LocalStateGone bool
	At             time.Time
}

// Complete reports whether every piece of an adapter's consent state is
// actually gone.
func (p Proof) Complete() bool {
	return p.GrantGone && p.TokensGone && p.LocalStateGone
}

// Vault is a store Revoke can delete an adapter's data from and then
// re-check. It is small on purpose: the token store and the local-state
// store both satisfy it, so Revoke runs the identical proof against each.
type Vault interface {
	Delete(ctx context.Context, adapterID string) error
	Has(ctx context.Context, adapterID string) (bool, error)
}

// Requires reports whether a consent class needs a per-app screen at all.
// Only class B does: class A is the official route and needs none, and no
// screen can cure a class C.
func Requires(c manifest.Consent) bool {
	return c == manifest.ConsentB
}

// Store is the consent gate: it renders screens, records grants, answers
// Allow for every request an adapter makes, and proves a revoke actually
// happened. It is safe for concurrent use, the same as registry.Registry.
type Store struct {
	mu     sync.RWMutex
	now    func() time.Time
	copies map[string]Copy
	tokens Vault
	local  Vault
	grants map[string]Grant
}

// New builds a Store. now is injected so tests can pin the clock; copies is
// the registered per-app screen and absence text; tokens and local are the
// two vaults Revoke proves itself against.
func New(now func() time.Time, copies map[string]Copy, tokens, local Vault) *Store {
	cp := make(map[string]Copy, len(copies))
	for id, c := range copies {
		cp[id] = c
	}
	return &Store{
		now:    now,
		copies: cp,
		tokens: tokens,
		local:  local,
		grants: make(map[string]Grant),
	}
}

// Screen renders the consent screen for a manifest's adapter. A class C
// manifest never gets one: ErrNeverShipped. A class A or B adapter with no
// registered copy gets ErrNoCopy rather than a generic warning.
func (s *Store) Screen(m manifest.Manifest) (Screen, error) {
	if !m.Consent.Shippable() {
		return Screen{}, fmt.Errorf("consent screen for %s: %w", m.ID, ErrNeverShipped)
	}

	s.mu.RLock()
	c, ok := s.copies[m.ID]
	s.mu.RUnlock()
	if !ok {
		return Screen{}, fmt.Errorf("consent screen for %s: %w", m.ID, ErrNoCopy)
	}

	grants := make([]string, len(c.Grants))
	copy(grants, c.Grants)

	return Screen{
		AdapterID: m.ID,
		Class:     m.Consent,
		Runtime:   m.Runtime,
		Situation: c.Situation,
		Grants:    grants,
		Risk:      c.Risk,
		Deny:      deny,
	}, nil
}

// Grant records a user's consent, but only when shown is the exact screen
// Screen produced for this adapter just now — matched on both adapter id
// and situation text, so a screen shown for one app can never grant
// another, and a stale screen from before the copy changed cannot either.
func (s *Store) Grant(ctx context.Context, m manifest.Manifest, shown Screen) error {
	if !m.Consent.Shippable() {
		return fmt.Errorf("grant %s: %w", m.ID, ErrNeverShipped)
	}

	current, err := s.Screen(m)
	if err != nil {
		return err
	}
	if shown.AdapterID != current.AdapterID || shown.Situation != current.Situation {
		return fmt.Errorf("grant %s: %w", m.ID, ErrScreenNotShown)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[m.ID] = Grant{
		AdapterID: m.ID,
		Situation: shown.Situation,
		GrantedAt: s.now(),
	}
	return nil
}

// Granted reports whether an adapter currently has a live grant.
func (s *Store) Granted(adapterID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.grants[adapterID]
	return ok
}

// Record returns the grant on file for an adapter, if any — the exact
// wording the user agreed to, independent of whatever the copy says today.
func (s *Store) Record(adapterID string) (Grant, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.grants[adapterID]
	return g, ok
}

// Allow is the consent gate every request runs through before it is
// allowed to execute. Class A passes with nothing granted. Class C never
// passes. Class B passes only once granted.
func (s *Store) Allow(ctx context.Context, m manifest.Manifest) error {
	if !m.Consent.Shippable() {
		return fmt.Errorf("allow %s: %w", m.ID, ErrNeverShipped)
	}
	if !Requires(m.Consent) {
		return nil
	}
	if !s.Granted(m.ID) {
		return fmt.Errorf("allow %s: %w", m.ID, ErrNotGranted)
	}
	return nil
}

// WhyNot answers a user who asks why an adapter isn't there: which kind of
// absence it is, and the situation text explaining it. It refuses silence —
// a user who asks twice deserves a reason.
func (s *Store) WhyNot(adapterID string) (Absence, string, error) {
	s.mu.RLock()
	c, ok := s.copies[adapterID]
	s.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("why not %s: %w", adapterID, ErrNoCopy)
	}
	return c.Absence, c.Situation, nil
}

// Revoke deletes an adapter's tokens and local state, drops its grant, and
// then re-reads both vaults: the proof is what Revoke finds afterwards, not
// a claim by the code that just did the deleting. If anything survives —
// including a vault whose Delete reported success but left the data behind
// — Revoke returns ErrRevokeIncomplete alongside a proof whose matching
// field is false.
func (s *Store) Revoke(ctx context.Context, adapterID string) (Proof, error) {
	proof := Proof{AdapterID: adapterID, At: s.now()}

	// Best-effort deletes. Their return values are not trusted: the
	// re-read below is the only thing that decides the proof.
	_ = s.tokens.Delete(ctx, adapterID)
	_ = s.local.Delete(ctx, adapterID)

	s.mu.Lock()
	delete(s.grants, adapterID)
	s.mu.Unlock()

	tokensHeld, _ := s.tokens.Has(ctx, adapterID)
	localHeld, _ := s.local.Has(ctx, adapterID)

	proof.TokensGone = !tokensHeld
	proof.LocalStateGone = !localHeld
	proof.GrantGone = !s.Granted(adapterID)

	if !proof.Complete() {
		return proof, fmt.Errorf("revoke %s: %w", adapterID, ErrRevokeIncomplete)
	}
	return proof, nil
}

// RevokeAll runs Revoke for every currently granted adapter. It is a loop
// over the same proven per-adapter path, not a separate shortcut, because a
// breach-response mass revoke is only trustworthy if it is the thing that
// was already proven to work one adapter at a time.
func (s *Store) RevokeAll(ctx context.Context) ([]Proof, error) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.grants))
	for id := range s.grants {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	proofs := make([]Proof, 0, len(ids))
	var faults []error
	for _, id := range ids {
		p, err := s.Revoke(ctx, id)
		proofs = append(proofs, p)
		if err != nil {
			faults = append(faults, err)
		}
	}
	if len(faults) > 0 {
		return proofs, errors.Join(faults...)
	}
	return proofs, nil
}

// Wipe clears every grant this store holds in memory, so the store as a
// whole can always be brought back to nothing — the same guarantee
// routing/contacts/graph.go's Wipe gives the contact graph. It does not
// touch the token or local-state vaults; call RevokeAll first if those
// need clearing too.
func (s *Store) Wipe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants = make(map[string]Grant)
}
