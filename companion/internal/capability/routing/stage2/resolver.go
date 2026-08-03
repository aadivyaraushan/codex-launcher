// Package stage2 is the on-device half of the router. It takes a Route
// stage 1 produced in the cloud and turns it into a Decision: which
// adapter should handle it, and — for a class addressed to a person —
// which handle on that adapter, resolved through the contact graph that
// never leaves the device.
package stage2

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

// Addressing says whether a class's requests name a person — and so need
// the contact graph to turn that name into a handle — or name a thing that
// has no person to resolve. It used to be a plain bool, so a class nobody
// wrote an opinion for silently read as "not addressed to a person": the
// answer that looked safe but was not, since production built every class
// that way and every "message Maya" fell through to a bare adapter carrying
// an empty handle, with nothing downstream checking before it sent. Making
// the zero value its own third state, AddressingUndeclared, means Go can no
// longer manufacture that "no" on its own — Resolve refuses on it instead
// of guessing which kind it is.
type Addressing int

const (
	// AddressingUndeclared is what a Class has if nobody set Addressing. It
	// is a wiring mistake, not a routing outcome, so Resolve refuses rather
	// than treating it as either real kind.
	AddressingUndeclared Addressing = iota
	// ToAPerson marks a class whose Subject names a person; Resolve turns
	// that name into a handle through the contact graph.
	ToAPerson
	// ToAThing marks a class with no person to resolve — the adapter alone
	// is a complete decision.
	ToAThing
	// ResolvedOnTheDevice marks a class whose Subject does name a person,
	// but where the Mac must not be the one to resolve them. A notification
	// reply is the case this exists for: only the phone knows which
	// conversations still have a live notification, and it already matches
	// a name against them. ToAPerson would be wrong here because it sends
	// the name through the contact graph on the Mac — a step this route
	// must skip, not merely one it happens to skip today. ToAThing would
	// also be wrong, because it is a lie: there is a person, and getting
	// them wrong sends a real message to the wrong human. So Resolve passes
	// the route's Subject straight through as the Handle and lets the
	// adapter's one surviving choice carry it to the device unresolved.
	ResolvedOnTheDevice
)

// String names an Addressing the way a class declaration or an error
// message should show it — a word, not the underlying int, so a hand-edited
// fixture or a log line reads as an English answer rather than a magic
// number nobody can decode without opening this file.
func (a Addressing) String() string {
	switch a {
	case ToAPerson:
		return "to_a_person"
	case ToAThing:
		return "to_a_thing"
	case ResolvedOnTheDevice:
		return "resolved_on_the_device"
	default:
		return "undeclared"
	}
}

// MarshalJSON writes an Addressing as the same word String returns, so a
// declared ClassMap on disk (an eval fixture, say) reads as English rather
// than an int whose meaning only this file knows.
func (a Addressing) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// UnmarshalJSON accepts the words MarshalJSON writes. A field left out of
// the JSON entirely never reaches this method at all — Go leaves the zero
// value, AddressingUndeclared, in place — which is exactly the silence this
// type exists to catch.
func (a *Addressing) UnmarshalJSON(data []byte) error {
	var word string
	if err := json.Unmarshal(data, &word); err != nil {
		return fmt.Errorf("stage2: addressing must be a string: %w", err)
	}
	switch word {
	case "to_a_person":
		*a = ToAPerson
	case "to_a_thing":
		*a = ToAThing
	case "resolved_on_the_device":
		*a = ResolvedOnTheDevice
	case "undeclared":
		*a = AddressingUndeclared
	default:
		return fmt.Errorf("stage2: unknown addressing %q", word)
	}
	return nil
}

// Class is one app class a Route's AppClass may name: the adapters that
// offer it, and how it is addressed.
type Class struct {
	Adapters   []string   `json:"adapters"`
	Addressing Addressing `json:"addressing"`
}

// ClassMap is the declared set of app classes, keyed by class name. Which
// classes exist, which adapters serve them, and whether they are addressed
// to a person are all declared here rather than guessed from the words.
type ClassMap map[string]Class

// Decision is the result of resolving a Route: either an adapter (and, for
// a class addressed to a person, a handle) to act on, or a question the
// caller must put to the user.
type Decision struct {
	AdapterID       string
	Handle          string
	Question        string
	MustAsk         bool
	RequiresPreview bool
	Candidates      []contacts.Entry
	Verb            manifest.Verb
	Body            string
	Fields          map[string]string
}

// Resolver turns Routes into Decisions using a registry of adapters, a
// contact graph, a declared set of app classes, and the platform it is
// running on.
type Resolver struct {
	reg      *registry.Registry
	graph    *contacts.Graph
	classes  ClassMap
	platform manifest.Platform
}

// New returns a Resolver backed by the given registry, contact graph,
// class declarations and platform.
func New(reg *registry.Registry, graph *contacts.Graph, classes ClassMap, platform manifest.Platform) *Resolver {
	return &Resolver{reg: reg, graph: graph, classes: classes, platform: platform}
}

// copyFields returns an independent copy of a route's fields, so a later
// change to the route or the decision can never alter the other.
func copyFields(fields map[string]string) map[string]string {
	if fields == nil {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// joinIDs reads out adapter ids in the order given, so a question can name
// them without a separate list ever needing to travel alongside it. Every
// place in this file that has to name more than one option funnels through
// here, so there is one join, not several.
func joinIDs(ids []string) string {
	return strings.Join(ids, " and ")
}

// joinSurfaces reads out a person's candidate surfaces in the order the
// contact graph already sorted them.
func joinSurfaces(candidates []contacts.Entry) string {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.AdapterID
	}
	return joinIDs(ids)
}

// Resolve turns a Route into a Decision.
//
// A route that must ask at stage 1 (low confidence, or no app class) asks
// immediately, without consulting the contact graph and without a handle. A
// route whose class exists but never declared its Addressing asks too, for
// the same reason — silence is not a declaration, and nothing here can tell
// which kind of class it should have been. Otherwise, the class's adapters
// are narrowed to the ones that are
// registered, enabled, offered on this platform, and that declare the
// route's verb. If the route names an app, only that adapter (if it is
// part of the class) survives; naming an app outside the class asks rather
// than substituting a neighbour. For a class addressed to a person, the
// subject is resolved through the contact graph, and only an adapter that
// also survived the filter above is accepted. For a class resolved on the
// device, the contact graph is skipped entirely and the subject is passed
// through unresolved — exactly one surviving adapter resolves; zero or
// several ask. For a class not addressed to a person, exactly one
// surviving adapter resolves; zero or several ask.
func (r *Resolver) Resolve(_ context.Context, route stage1.Route) (Decision, error) {
	dec := Decision{
		Verb:            route.Verb,
		Body:            route.Body,
		Fields:          copyFields(route.Fields),
		RequiresPreview: route.Verb.RequiresPreview(),
	}

	if route.MustAsk() {
		dec.MustAsk = true
		dec.Question = "I'm not confident enough about what you want me to do — can you say it again?"
		return dec, nil
	}

	class, ok := r.classes[route.AppClass]
	if !ok {
		dec.MustAsk = true
		dec.Question = fmt.Sprintf("I don't know which app to use for %q.", route.AppClass)
		return dec, nil
	}

	// This has to run before any adapter filtering below: an undeclared
	// class is a wiring mistake, and whether it gets caught must not depend
	// on how many adapters happen to survive the filter — that would make
	// the refusal flaky instead of certain.
	if class.Addressing == AddressingUndeclared {
		dec.MustAsk = true
		dec.Question = fmt.Sprintf("The %q class was never declared as addressed to a person or a thing, so I won't guess.", route.AppClass)
		return dec, nil
	}

	var survivors []string
	for _, id := range class.Adapters {
		a, err := r.reg.Get(id)
		if err != nil {
			continue
		}
		m := a.Describe()
		if !m.Platform.Includes(r.platform) {
			continue
		}
		if !m.Allows(route.Verb) {
			continue
		}
		survivors = append(survivors, id)
	}

	namedAdapter := route.AppNamed
	if namedAdapter != "" {
		canonical := ""
		for _, id := range class.Adapters {
			if strings.EqualFold(id, namedAdapter) {
				canonical = id
				break
			}
		}
		if canonical == "" {
			dec.MustAsk = true
			dec.Question = "I don't have the app you named connected for this."
			return dec, nil
		}
		namedAdapter = canonical
		var narrowed []string
		for _, id := range survivors {
			if id == namedAdapter {
				narrowed = append(narrowed, id)
			}
		}
		survivors = narrowed
	}

	if class.Addressing == ToAPerson {
		contactDec := r.graph.Resolve(route.Subject, namedAdapter)
		if contactDec.MustAsk {
			dec.MustAsk = true
			dec.Candidates = contactDec.Candidates
			// Only the question string ever reaches the user (flow/service.go
			// builds the error from this text alone and drops Candidates), so
			// the sentence must stand on its own. An unseen person and a real
			// choice between known surfaces are different truths and need
			// different words: one has nothing to offer, the other has to
			// name what it's offering right here.
			if len(contactDec.Candidates) == 0 {
				dec.Question = fmt.Sprintf("I don't know how to reach %s.", route.Subject)
			} else {
				dec.Question = fmt.Sprintf("%s is on %s — which did you mean?", route.Subject, joinSurfaces(contactDec.Candidates))
			}
			return dec, nil
		}
		if !contains(survivors, contactDec.Entry.AdapterID) {
			dec.MustAsk = true
			dec.Question = "The surface I'd normally use for this isn't available right now."
			return dec, nil
		}
		dec.AdapterID = contactDec.Entry.AdapterID
		dec.Handle = contactDec.Entry.Handle
		return dec, nil
	}

	// A class resolved on the device does name a person, but the contact
	// graph must never be asked — the phone is the only side that can tell
	// which name is right, and the Mac would only be guessing. So this picks
	// the same way a class with no person to resolve would (exactly one
	// surviving adapter, or ask), and then hands that adapter the subject
	// exactly as the router produced it, unresolved.
	if class.Addressing == ResolvedOnTheDevice {
		if len(survivors) == 0 {
			dec.MustAsk = true
			dec.Question = "No available app can handle this right now."
			return dec, nil
		}
		if len(survivors) > 1 {
			dec.MustAsk = true
			dec.Question = fmt.Sprintf("More than one app could handle this — %s — which one did you mean?", joinIDs(survivors))
			return dec, nil
		}
		dec.AdapterID = survivors[0]
		dec.Handle = route.Subject
		return dec, nil
	}

	if len(survivors) == 0 {
		dec.MustAsk = true
		dec.Question = "No available app can handle this right now."
		return dec, nil
	}
	if len(survivors) > 1 {
		dec.MustAsk = true
		dec.Question = fmt.Sprintf("More than one app could handle this — %s — which one did you mean?", joinIDs(survivors))
		return dec, nil
	}
	dec.AdapterID = survivors[0]
	return dec, nil
}
