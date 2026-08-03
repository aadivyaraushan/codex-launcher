// Package manifest defines the vocabulary every capability adapter declares
// itself in: which runtime it targets, which verbs it offers, how far it can
// carry a request, what it costs, and what it is allowed to touch. The
// manifest is data, not code — an adapter ships a JSON document in this
// shape, and everything downstream (registry, execution) trusts only what
// validates.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidManifest is the sentinel every validation failure wraps, so a
// caller can test for "this manifest is bad" without parsing the message.
var ErrInvalidManifest = errors.New("invalid manifest")

// ErrGateNotCleared is returned when a manifest declares a checkpoint the
// request has not cleared. Nothing in the product can clear one today, so
// any gate other than GateNone refuses. This lives on the manifest, not on
// any one caller, because both the user-facing runner (execution.Runner) and
// the unattended verification runners have to refuse the same way, and
// execution already imports verification — so the rule has to sit somewhere
// both can reach without a cycle.
var ErrGateNotCleared = errors.New("adapter declares a gate that has not been cleared")

// ---- runtime --------------------------------------------------------------

// Runtime identifies which execution environment a capability adapter runs
// in (on-device shell, cloud worker, browser automation, and so on).
type Runtime string

const (
	RT1 Runtime = "RT-1"
	RT2 Runtime = "RT-2"
	RT3 Runtime = "RT-3"
	RT4 Runtime = "RT-4"
	RT5 Runtime = "RT-5"
	RT6 Runtime = "RT-6"
)

var allRuntimes = []Runtime{RT1, RT2, RT3, RT4, RT5, RT6}

func validRuntime(r Runtime) bool {
	for _, want := range allRuntimes {
		if r == want {
			return true
		}
	}
	return false
}

// ---- verbs ------------------------------------------------------------

// Verb is one of the nine actions the spine allows an adapter to offer.
// "pay" and "open" are deliberately absent: money movement is deep-link
// only forever, and opening the app is the floor under every verb, not one
// of them.
type Verb string

const (
	Read    Verb = "read"
	Compose Verb = "compose"
	Send    Verb = "send"
	Order   Verb = "order"
	Book    Verb = "book"
	Play    Verb = "play"
	Write   Verb = "write"
	Cancel  Verb = "cancel"
	Modify  Verb = "modify"
)

// AllVerbs is the closed set of verbs a manifest may declare.
var AllVerbs = []Verb{Read, Compose, Send, Order, Book, Play, Write, Cancel, Modify}

// ParseVerb resolves a verb by its written form, refusing anything outside
// the closed set.
func ParseVerb(name string) (Verb, error) {
	for _, v := range AllVerbs {
		if string(v) == name {
			return v, nil
		}
	}
	return "", fmt.Errorf("unknown verb %q", name)
}

// RequiresPreview reports whether this verb moves something irreversible
// and therefore must be shown to the user before Execute runs.
func (v Verb) RequiresPreview() bool {
	switch v {
	case Send, Order, Book, Write, Cancel, Modify:
		return true
	default:
		return false
	}
}

// ---- ceilings ---------------------------------------------------------

// Ceiling is the highest level of automation an adapter claims (or is
// measured) to reach.
type Ceiling string

const (
	Completes Ceiling = "completes"
	OneTap    Ceiling = "one_tap"
	HandsOff  Ceiling = "hands_off"
)

func validCeiling(c Ceiling) bool {
	return c.Valid()
}

// Valid reports whether c is one of the three known ceilings. Anything else
// (including the empty string) is not a real ceiling and must be refused
// rather than ranked or compared.
func (c Ceiling) Valid() bool {
	switch c {
	case Completes, OneTap, HandsOff:
		return true
	default:
		return false
	}
}

// Rank orders ceilings from most automated (highest) to least (lowest), so
// a measured outcome can be compared against a declared claim.
func (c Ceiling) Rank() int {
	switch c {
	case Completes:
		return 3
	case OneTap:
		return 2
	case HandsOff:
		return 1
	default:
		return 0
	}
}

// AtMost returns the weaker of the two ceilings. This is how a measured
// outcome demotes a declared claim: it can only pull the ceiling down,
// never push it up.
func (c Ceiling) AtMost(other Ceiling) Ceiling {
	if other.Rank() < c.Rank() {
		return other
	}
	return c
}

// OrHandsOff reads a ceiling an adapter handed over at some earlier moment
// (not from the manifest file, so it may be blank or misspelled) and returns
// the ceiling that is safe to act on. Blank is not a claim of any kind, and
// the honest reading of "no claim" is the most modest ceiling there is, not
// the strongest — an adapter that forgot to declare must never be read as
// having declared the best possible outcome.
func (c Ceiling) OrHandsOff() Ceiling {
	if !c.Valid() {
		return HandsOff
	}
	return c
}

// ---- consent classes ----------------------------------------------------

// Consent is the consent class an adapter's data access falls under. Only
// classes A and B are ever shipped; C1/C2/C3 never are.
type Consent string

const (
	ConsentA  Consent = "A"
	ConsentB  Consent = "B"
	ConsentC1 Consent = "C1"
	ConsentC2 Consent = "C2"
	ConsentC3 Consent = "C3"
)

func validConsent(c Consent) bool {
	switch c {
	case ConsentA, ConsentB, ConsentC1, ConsentC2, ConsentC3:
		return true
	default:
		return false
	}
}

// Shippable reports whether this consent class is ever allowed to ship.
func (c Consent) Shippable() bool {
	switch c {
	case ConsentA, ConsentB:
		return true
	default:
		return false
	}
}

// ---- auth, cost, gates ---------------------------------------------------

// Auth is how an adapter authenticates to the service it talks to.
type Auth string

const (
	AuthOAuth  Auth = "oauth"
	AuthDevice Auth = "device"
	AuthLocal  Auth = "local"
	AuthNone   Auth = "none"
)

func validAuth(a Auth) bool {
	switch a {
	case AuthOAuth, AuthDevice, AuthLocal, AuthNone:
		return true
	default:
		return false
	}
}

// Cost is what using an adapter costs the user.
type Cost string

const (
	CostFree    Cost = "free"
	CostPerCall Cost = "per_call"
	CostMetered Cost = "metered"
)

func validCost(c Cost) bool {
	switch c {
	case CostFree, CostPerCall, CostMetered:
		return true
	default:
		return false
	}
}

// Gate is an extra checkpoint a request must clear before it runs.
type Gate string

const (
	GateNone     Gate = "none"
	GateApproval Gate = "approval"
	GateBilling  Gate = "billing"
)

func validGate(g Gate) bool {
	switch g {
	case GateNone, GateApproval, GateBilling:
		return true
	default:
		return false
	}
}

// ---- capacity -------------------------------------------------------------

// CapacityKind is the shape of an adapter's capacity limit.
type CapacityKind string

const (
	CapacityNone               CapacityKind = "none"
	CapacityCapped             CapacityKind = "capped"
	CapacityPendingApplication CapacityKind = "pending_application"
)

func validCapacityKind(k CapacityKind) bool {
	switch k {
	case CapacityNone, CapacityCapped, CapacityPendingApplication:
		return true
	default:
		return false
	}
}

// Capacity is how many concurrent users an adapter can admit. It is written
// as a single string ("none", "capped:5", "pending_application") but held
// as a struct so the gate can compare against it without reparsing.
type Capacity struct {
	Kind  CapacityKind
	Limit int
}

// ParseCapacity parses a capacity's written form.
func ParseCapacity(text string) (Capacity, error) {
	switch {
	case text == string(CapacityNone):
		return Capacity{Kind: CapacityNone}, nil
	case text == string(CapacityPendingApplication):
		return Capacity{Kind: CapacityPendingApplication}, nil
	case strings.HasPrefix(text, "capped:"):
		rest := strings.TrimPrefix(text, "capped:")
		n, err := strconv.Atoi(rest)
		if err != nil || n <= 0 {
			return Capacity{}, fmt.Errorf("capacity: %q is not a usable cap", text)
		}
		return Capacity{Kind: CapacityCapped, Limit: n}, nil
	default:
		return Capacity{}, fmt.Errorf("capacity: unknown capacity %q", text)
	}
}

// String renders a Capacity back to its written form. ParseCapacity and
// String round-trip each other.
func (c Capacity) String() string {
	switch c.Kind {
	case CapacityNone:
		return string(CapacityNone)
	case CapacityPendingApplication:
		return string(CapacityPendingApplication)
	case CapacityCapped:
		return fmt.Sprintf("capped:%d", c.Limit)
	default:
		return "unknown"
	}
}

// Admits reports whether one more user can be admitted given how many are
// already connected.
func (c Capacity) Admits(connected int) bool {
	switch c.Kind {
	case CapacityNone:
		return true
	case CapacityCapped:
		return connected < c.Limit
	case CapacityPendingApplication:
		return false
	default:
		return false
	}
}

// MarshalJSON writes a Capacity as its written-form string.
func (c Capacity) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// UnmarshalJSON reads a Capacity from its written-form string.
func (c *Capacity) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	parsed, err := ParseCapacity(text)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// ---- platform ---------------------------------------------------------

// Platform is which mobile platform(s) an adapter runs on.
type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIOS     Platform = "ios"
	PlatformBoth    Platform = "both"
)

func validPlatform(p Platform) bool {
	switch p {
	case PlatformAndroid, PlatformIOS, PlatformBoth:
		return true
	default:
		return false
	}
}

// Includes reports whether an adapter declaring platform p is offered when
// a caller is running on platform other.
func (p Platform) Includes(other Platform) bool {
	if p == PlatformBoth {
		return true
	}
	return p == other
}

// ---- the manifest itself -------------------------------------------------

// Manifest is the complete, data-only description of a capability adapter.
type Manifest struct {
	ID            string   `json:"id"`
	Runtime       Runtime  `json:"runtime"`
	Verbs         []Verb   `json:"verbs"`
	Ceiling       Ceiling  `json:"ceiling"`
	Consent       Consent  `json:"consent"`
	Auth          Auth     `json:"auth"`
	Cost          Cost     `json:"cost"`
	Gates         []Gate   `json:"gates"`
	Capacity      Capacity `json:"capacity"`
	Region        []string `json:"region"`
	Platform      Platform `json:"platform"`
	ProvesCeiling string   `json:"proves_ceiling"`

	// Unshipped, when set, is the plain-English reason no build registers
	// this adapter. An adapter nobody can reach makes no claim to any user,
	// so it is not asked to name a smoke test proving its ceiling. It is a
	// reason rather than a yes/no on purpose: the exemption has to be
	// written down and readable in the manifest, not taken quietly. Empty
	// means the adapter ships and every rule applies.
	Unshipped string `json:"unshipped,omitempty"`
}

// Allows reports whether this manifest declares the given verb.
func (m Manifest) Allows(v Verb) bool {
	for _, want := range m.Verbs {
		if want == v {
			return true
		}
	}
	return false
}

// NamesAProof reports whether this manifest names a smoke test for its
// ceiling. It does NOT report that the ceiling is proven, and the difference
// is not pedantic: nothing here checks that the name belongs to a test that
// exists. Measured 2026-08-03, all 14 shipped adapters name a proof that
// resolves to nothing, so a method called CeilingIsProven — which this was —
// would have answered "yes, proven" for every adapter in the build and been
// wrong every time. See runtime/proof_names_resolve_test.go, which pins that
// count so it cannot grow.
//
// A manifest without a name is still valid; it ships as unverified. A
// manifest with one has made a claim, which is a different and much weaker
// thing than having kept it.
func (m Manifest) NamesAProof() bool {
	return m.ProvesCeiling != ""
}

// CheckGates scans the manifest's whole declared Gates list and refuses if
// any entry is not GateNone. GateNone is not a gate — a list of nothing but
// GateNone passes — but GateNone sitting alongside a real gate is not
// permission either: the whole list is scanned because the strictest entry
// decides, not the first or the last.
func (m Manifest) CheckGates() error {
	for _, g := range m.Gates {
		if g != GateNone {
			return fmt.Errorf("%w: %s requires %s", ErrGateNotCleared, m.ID, g)
		}
	}
	return nil
}

// Validate checks every field and accumulates every fault, rather than
// stopping at the first one, so a caller fixes them all in one pass. The
// returned error, if any, wraps ErrInvalidManifest.
func (m Manifest) Validate() error {
	var faults []error

	if strings.TrimSpace(m.ID) == "" {
		faults = append(faults, errors.New("id: must not be empty"))
	}
	if !validRuntime(m.Runtime) {
		faults = append(faults, fmt.Errorf("runtime: unknown runtime %q", m.Runtime))
	}
	if len(m.Verbs) == 0 {
		faults = append(faults, errors.New("verbs: must declare at least one verb"))
	}
	for _, v := range m.Verbs {
		if _, err := ParseVerb(string(v)); err != nil {
			faults = append(faults, fmt.Errorf("verbs: unknown verb %q", v))
		}
	}
	if !validCeiling(m.Ceiling) {
		faults = append(faults, fmt.Errorf("ceiling: unknown ceiling %q", m.Ceiling))
	}
	if !validConsent(m.Consent) {
		faults = append(faults, fmt.Errorf("consent: unknown consent class %q", m.Consent))
	} else if !m.Consent.Shippable() {
		faults = append(faults, fmt.Errorf("consent: class %s is never shipped", m.Consent))
	}
	if !validAuth(m.Auth) {
		faults = append(faults, fmt.Errorf("auth: unknown auth %q", m.Auth))
	}
	if !validCost(m.Cost) {
		faults = append(faults, fmt.Errorf("cost: unknown cost %q", m.Cost))
	}
	if len(m.Gates) == 0 {
		faults = append(faults, errors.New("gates: must declare at least one gate"))
	}
	for _, g := range m.Gates {
		if !validGate(g) {
			faults = append(faults, fmt.Errorf("gates: unknown gate %q", g))
		}
	}
	if !validCapacityKind(m.Capacity.Kind) {
		faults = append(faults, fmt.Errorf("capacity: unknown capacity kind %q", m.Capacity.Kind))
	}
	if !validPlatform(m.Platform) {
		faults = append(faults, fmt.Errorf("platform: unknown platform %q", m.Platform))
	}

	if len(faults) == 0 {
		return nil
	}
	return errors.Join(append([]error{ErrInvalidManifest}, faults...)...)
}

// Load unmarshals a manifest from JSON and validates it in one step, so no
// caller can forget the second half.
func Load(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
