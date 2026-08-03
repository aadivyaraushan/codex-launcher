package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// The router has two stages so that turning a person's name into a handle
// happens on the user's side and nowhere else. Whether a class needs that
// turn is declared per class, and production is the only place that
// declaration matters — every other ClassMap in this package belongs to an
// owner-only proof flow.
//
// These tests are deliberately about behaviour rather than about the enum.
// What matters is not which constant a class is tagged with; it is that a
// message aimed at a person goes through the contact graph, that a request
// aimed at a thing does not, and that no class is left saying nothing at all.
// Assert those three and the tagging follows; assert the tagging and you have
// only tested that a line of code says what it says.

// classProbe is the smallest adapter that survives the resolver's filter, so
// these tests turn on the class declaration and nothing else.
type classProbe struct{ m manifest.Manifest }

func (p *classProbe) Describe() manifest.Manifest { return p.m }
func (p *classProbe) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{AdapterID: p.m.ID}, nil
}
func (p *classProbe) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}
func (p *classProbe) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	return adapter.Outcome{Reached: p.m.Ceiling, Done: true}, nil
}
func (p *classProbe) Revoke(context.Context) error { return nil }

func probeFor(id string, verbs ...manifest.Verb) *classProbe {
	return &classProbe{m: manifest.Manifest{
		ID: id, Runtime: manifest.RT2, Verbs: verbs,
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformBoth,
		ProvesCeiling: id + "-smoke",
	}}
}

// productionResolver builds a resolver whose class declarations come from the
// same function production uses, so these tests fail if that function stops
// declaring a class production serves.
func productionResolver(t *testing.T, id, class string, verb manifest.Verb) *stage2.Resolver {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(probeFor(id, verb)); err != nil {
		t.Fatalf("Register(%s): %v", id, err)
	}
	byClass := map[string][]string{class: {id}}
	return stage2.New(reg, contacts.NewGraph(time.Now), classMapFor(byClass), manifest.PlatformAndroid)
}

func routeTo(verb manifest.Verb, class, subject string) stage1.Route {
	return stage1.Route{
		Verb: verb, AppClass: class, Subject: subject,
		Body: "running late", Confidence: 0.95,
	}
}

// The one that would have caught the original bug. With the contact graph
// empty — which it always is, since nothing in production ever calls Add —
// the only honest answer to "message Maya" is to ask which Maya. Resolving to
// a messaging adapter with no handle is a decision to message nobody, and
// nothing downstream checks the handle before executing.
func TestMessagingInProductionGoesThroughTheContactGraph(t *testing.T) {
	r := productionResolver(t, "sms", "messaging", manifest.Send)

	got, err := r.Resolve(context.Background(), routeTo(manifest.Send, "messaging", "Maya"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !got.MustAsk {
		t.Fatalf("messaged an unresolved person instead of asking: %+v", got)
	}
	if got.Handle != "" || got.AdapterID != "" {
		t.Fatalf("asked, yet still named a destination: adapter=%q handle=%q", got.AdapterID, got.Handle)
	}
}

// Money moves to a person the same way a message does, and gets it wrong in a
// way that cannot be taken back.
func TestMovingMoneyInProductionGoesThroughTheContactGraph(t *testing.T) {
	r := productionResolver(t, "cashapp", "money", manifest.Send)

	got, err := r.Resolve(context.Background(), routeTo(manifest.Send, "money", "Maya"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !got.MustAsk {
		t.Fatalf("moved money to an unresolved person instead of asking: %+v", got)
	}
	if got.Handle != "" || got.AdapterID != "" {
		t.Fatalf("asked, yet still named a destination: adapter=%q handle=%q", got.AdapterID, got.Handle)
	}
}

// The control, and the reason this cannot be fixed by refusing everything.
// A note has no person in it, even when the sentence has a subject. It must
// resolve without the contact graph, and must not be refused as a class
// nobody declared.
func TestANoteInProductionResolvesWithoutTheContactGraph(t *testing.T) {
	r := productionResolver(t, "notion", "notes", manifest.Write)

	got, err := r.Resolve(context.Background(), routeTo(manifest.Write, "notes", "the Q3 plan"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.MustAsk {
		t.Fatalf("a note with exactly one note app still asked: %+v", got)
	}
	if got.AdapterID != "notion" {
		t.Fatalf("resolved to %q, not notion", got.AdapterID)
	}
}

// A class that arrives here having said nothing is a wiring mistake, and the
// product must not guess which kind it is. classMapFor is the only thing that
// builds production's declarations, so a class it has never heard of must
// still come out declared — not left at whatever the language fills in.
//
// This is what keeps the bug from returning the next time somebody adds a
// class: they get a refusal that names it, at the first request, instead of
// silent delivery to an empty handle.
func TestAClassProductionHasNeverHeardOfIsRefusedByName(t *testing.T) {
	r := productionResolver(t, "zeta", "quantum-laundry", manifest.Send)

	got, err := r.Resolve(context.Background(), routeTo(manifest.Send, "quantum-laundry", "Maya"))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.AdapterID != "" || got.Handle != "" {
		t.Fatalf("acted on a class nobody declared: adapter=%q handle=%q", got.AdapterID, got.Handle)
	}
	if !got.MustAsk {
		t.Fatalf("a class nobody declared did not ask: %+v", got)
	}
	if !strings.Contains(got.Question, "quantum-laundry") {
		t.Fatalf("refusal does not name the class it is about: %q", got.Question)
	}
}
