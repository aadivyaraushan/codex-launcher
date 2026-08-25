// Package adapter defines the contract every capability adapter implements:
// resolve an intent into a plan, preview that plan for the user, execute it,
// and revoke access when asked. The registry and execution packages depend
// only on this contract, never on a concrete adapter.
package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ClarificationError means an adapter found a real choice it cannot safely
// make, such as two Beeper conversations with the same visible name. The flow
// turns this into a question for the phone instead of reporting a broken
// capability or guessing a recipient.
type ClarificationError struct {
	Question string
	Cause    error
}

func (e *ClarificationError) Error() string {
	return fmt.Sprintf("capability needs clarification: %s", e.Question)
}

func (e *ClarificationError) Unwrap() error { return e.Cause }

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

// OutcomeMessage is one received message a read outcome carries alongside
// its flattened Detail sentence, so a thread UI can render it as its own
// chat row instead of re-parsing Detail's "sender: text · …" string.
type OutcomeMessage struct {
	Sender string
	Text   string
	SentAt time.Time
}

// Outcome is what actually happened when a plan was executed. Reached is
// the ceiling actually reached, which may be lower than the manifest's
// declared ceiling; Done is true only when the request fully completed
// on-device; HandedOffTo names the app control was handed to, when it was.
// Messages carries the structured rows behind a read's Detail sentence, when
// the adapter has them; Detail remains the fallback and the desktop
// rendering regardless.
type Outcome struct {
	Reached     manifest.Ceiling
	Done        bool
	HandedOffTo string
	Detail      string
	Messages    []OutcomeMessage
}

// Adapter is the contract every capability adapter implements.
type Adapter interface {
	Describe() manifest.Manifest
	Resolve(context.Context, Intent) (Plan, error)
	Preview(context.Context, Plan) (Preview, error)
	Execute(context.Context, Plan) (Outcome, error)
	Revoke(context.Context) error
}
