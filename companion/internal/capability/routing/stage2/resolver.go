// Package stage2 is the on-device half of the router. It takes a Route
// stage 1 produced in the cloud and turns it into a Decision: which
// adapter should handle it, and — for a class addressed to a person —
// which handle on that adapter, resolved through the contact graph that
// never leaves the device.
package stage2

import (
	"context"
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

// Class is one app class a Route's AppClass may name: the adapters that
// offer it, and whether requests in this class are addressed to a person
// (and therefore need the contact graph) or not.
type Class struct {
	Adapters          []string `json:"adapters"`
	AddressedToPerson bool     `json:"addressed_to_person"`
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

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Resolve turns a Route into a Decision.
//
// A route that must ask at stage 1 (low confidence, or no app class) asks
// immediately, without consulting the contact graph and without a handle.
// Otherwise, the class's adapters are narrowed to the ones that are
// registered, enabled, offered on this platform, and that declare the
// route's verb. If the route names an app, only that adapter (if it is
// part of the class) survives; naming an app outside the class asks rather
// than substituting a neighbour. For a class addressed to a person, the
// subject is resolved through the contact graph, and only an adapter that
// also survived the filter above is accepted. For a class not addressed to
// a person, exactly one surviving adapter resolves; zero or several ask.
func (r *Resolver) Resolve(_ context.Context, route stage1.Route) (Decision, error) {
	dec := Decision{
		Verb:            route.Verb,
		Body:            route.Body,
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

	if route.AppNamed != "" {
		if !contains(class.Adapters, route.AppNamed) {
			dec.MustAsk = true
			dec.Question = "I don't have the app you named connected for this."
			return dec, nil
		}
		var narrowed []string
		for _, id := range survivors {
			if id == route.AppNamed {
				narrowed = append(narrowed, id)
			}
		}
		survivors = narrowed
	}

	if class.AddressedToPerson {
		contactDec := r.graph.Resolve(route.Subject, route.AppNamed)
		if contactDec.MustAsk {
			dec.MustAsk = true
			dec.Candidates = contactDec.Candidates
			dec.Question = fmt.Sprintf("Which of %s's surfaces did you mean?", route.Subject)
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

	if len(survivors) == 0 {
		dec.MustAsk = true
		dec.Question = "No available app can handle this right now."
		return dec, nil
	}
	if len(survivors) > 1 {
		dec.MustAsk = true
		dec.Question = "More than one app could handle this — which one did you mean?"
		return dec, nil
	}
	dec.AdapterID = survivors[0]
	return dec, nil
}
