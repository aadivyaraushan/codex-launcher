// Package adapter defines the contract every capability adapter implements:
// resolve an intent into a plan, preview that plan for the user, execute it,
// and revoke access when asked. The registry and execution packages depend
// only on this contract, never on a concrete adapter.
package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// Intent is what the cloud router asked for, before anything on the device
// has resolved it. Subject is the unresolved name from the router (a
// contact name, a place, a track title); Handle is what the adapter
// resolves it to on the device (a phone number, a place id, a URI). The
// two are kept separate because resolving a subject to a handle is a
// device-local step the cloud router never sees.
type Intent struct {
	AdapterID string
	Verb      manifest.Verb
	Subject   string
	Handle    string
	Body      string
	Fields    map[string]string
}

// Plan is what an adapter intends to do, after Resolve, and before Preview
// or Execute. It carries enough detail that Preview can render it and
// Execute can be checked against what the user actually confirmed.
type Plan struct {
	AdapterID string
	Verb      manifest.Verb
	Handle    string
	Summary   string
	Details   map[string]string
}

// Fingerprint is a stable content hash of a plan's meaningful fields. Two
// plans with the same adapter, verb, handle, summary and details produce
// the same fingerprint; anything that changes what the plan actually does
// changes it. This is what lets Execute check that a confirmation was made
// against this exact plan, not a similar one.
func (p Plan) Fingerprint() string {
	var b strings.Builder
	b.WriteString(p.AdapterID)
	b.WriteByte('\n')
	b.WriteString(string(p.Verb))
	b.WriteByte('\n')
	b.WriteString(p.Handle)
	b.WriteByte('\n')
	b.WriteString(p.Summary)
	b.WriteByte('\n')

	keys := make([]string, 0, len(p.Details))
	for k := range p.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(p.Details[k])
		b.WriteByte('\n')
	}

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Preview is what is shown to the user before an irreversible verb runs.
type Preview struct {
	Plan     Plan
	Headline string
	Lines    []string
	Confirm  string
}

// Fingerprint delegates to the underlying plan's fingerprint, so a preview
// and the plan it was built from always agree on identity.
func (p Preview) Fingerprint() string {
	return p.Plan.Fingerprint()
}

// Outcome is what actually happened when a plan was executed. Reached is
// the ceiling actually reached, which may be lower than the manifest's
// declared ceiling; Done is true only when the request fully completed
// on-device; HandedOffTo names the app control was handed to, when it was.
type Outcome struct {
	Reached     manifest.Ceiling
	Done        bool
	HandedOffTo string
	Detail      string
}

// Adapter is the contract every capability adapter implements.
type Adapter interface {
	Describe() manifest.Manifest
	Resolve(context.Context, Intent) (Plan, error)
	Preview(context.Context, Plan) (Preview, error)
	Execute(context.Context, Plan) (Outcome, error)
	Revoke(context.Context) error
}
