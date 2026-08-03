package stage2

import (
	"context"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
)

var now = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return now.AddDate(0, 0, -n) }

type stub struct{ m manifest.Manifest }

func (s *stub) Describe() manifest.Manifest { return s.m }
func (s *stub) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{AdapterID: s.m.ID}, nil
}
func (s *stub) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}
func (s *stub) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	return adapter.Outcome{Reached: s.m.Ceiling, Done: true}, nil
}
func (s *stub) Revoke(context.Context) error { return nil }

func adapterFor(id string, verbs ...manifest.Verb) *stub {
	return &stub{m: manifest.Manifest{
		ID: id, Runtime: manifest.RT2, Verbs: verbs,
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region: []string{"global"}, Platform: manifest.PlatformBoth, ProvesCeiling: id + "-smoke",
	}}
}

// Wave 0's world: two note surfaces that compete for the same words, and two
// messaging surfaces for one person.
func testResolver(t *testing.T, entries ...contacts.Entry) (*Resolver, *registry.Registry, *contacts.Graph) {
	t.Helper()
	reg := registry.New()
	for _, a := range []*stub{
		adapterFor("notion", manifest.Read, manifest.Write),
		adapterFor("apple-notes", manifest.Read, manifest.Write),
		adapterFor("sms", manifest.Send, manifest.Read),
		adapterFor("whatsapp", manifest.Send, manifest.Read),
	} {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register(%s): %v", a.m.ID, err)
		}
	}

	graph := contacts.NewGraph(func() time.Time { return now })
	for _, e := range entries {
		graph.Add(e)
	}

	// Whether a class is addressed to a person is declared, not guessed. A
	// note can mention someone's name without being a message to them.
	classes := ClassMap{
		"messaging": {Adapters: []string{"sms", "whatsapp"}, Addressing: ToAPerson},
		"notes":     {Adapters: []string{"notion", "apple-notes"}, Addressing: ToAThing},
	}
	return New(reg, graph, classes, manifest.PlatformAndroid), reg, graph
}

func route(verb manifest.Verb, class, subject string, conf float64) stage1.Route {
	return stage1.Route{Verb: verb, AppClass: class, Subject: subject, Body: "hi", Confidence: conf}
}

// ---- the two stages join up ---------------------------------------------

func TestAMessagingRouteIsResolvedThroughTheContactGraph(t *testing.T) {
	r, _, _ := testResolver(t, contacts.Entry{
		Person: "Maya K", AdapterID: "whatsapp", Handle: "+15550000001",
		LastSeen: daysAgo(2), Source: contacts.Observed,
	})

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.MustAsk {
		t.Fatalf("a single unambiguous contact still asked: %+v", got)
	}
	if got.AdapterID != "whatsapp" || got.Handle != "+15550000001" {
		t.Fatalf("resolved to %s/%s", got.AdapterID, got.Handle)
	}
}

func TestALowConfidenceRouteAsksBeforeTheContactGraphIsEvenConsulted(t *testing.T) {
	r, _, _ := testResolver(t, contacts.Entry{
		Person: "Maya K", AdapterID: "whatsapp", Handle: "+15550000001",
		LastSeen: daysAgo(2), Source: contacts.Observed,
	})

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.3))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !got.MustAsk {
		t.Fatal("a low-confidence route executed instead of asking")
	}
	if got.Handle != "" {
		t.Errorf("an unresolved decision carries a handle: %q", got.Handle)
	}
}

func TestAnAmbiguousPersonProducesAQuestionWithTheCandidatesInIt(t *testing.T) {
	r, _, _ := testResolver(t,
		contacts.Entry{Person: "Maya K", AdapterID: "whatsapp", Handle: "+15550000001", LastSeen: daysAgo(3), Source: contacts.Observed},
		contacts.Entry{Person: "Maya K", AdapterID: "sms", Handle: "+15550000009", LastSeen: daysAgo(40), Source: contacts.Observed},
	)

	got, _ := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if !got.MustAsk {
		t.Fatalf("guessed %s between two live surfaces", got.AdapterID)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("the question offers %d candidates, want 2", len(got.Candidates))
	}
	if got.Question == "" {
		t.Error("the question has no text; DESIGN.md requires the sheet explain why it is waiting")
	}
}

// ---- a verb with no person in it ----------------------------------------

func TestTwoNoteAppsForOneRequestIsAnAskNotAToss(t *testing.T) {
	// notion and apple-notes both take "write". Nothing in the utterance
	// chooses between them, so the product asks once and pins the answer.
	r, _, _ := testResolver(t)

	got, err := r.Resolve(context.Background(), route(manifest.Write, "notes", "", 0.97))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !got.MustAsk {
		t.Fatalf("picked %s between two note apps with nothing to go on", got.AdapterID)
	}
}

func TestNamingTheAppSettlesAVerbThatHasNoPersonInIt(t *testing.T) {
	r, _, _ := testResolver(t)
	rt := route(manifest.Write, "notes", "", 0.97)
	rt.AppNamed = "notion"

	got, _ := r.Resolve(context.Background(), rt)
	if got.MustAsk || got.AdapterID != "notion" {
		t.Fatalf("naming notion did not settle it: %+v", got)
	}
}

func TestOnlyOneAdapterInTheClassResolvesWithoutAsking(t *testing.T) {
	r, reg, _ := testResolver(t)
	if err := reg.Disable("apple-notes", "companion asleep"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	got, err := r.Resolve(context.Background(), route(manifest.Write, "notes", "", 0.97))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.MustAsk || got.AdapterID != "notion" {
		t.Fatalf("one remaining note app still asked: %+v", got)
	}
}

// ---- the doors stay shut ------------------------------------------------

func TestASwitchedOffAdapterIsNeverRoutedTo(t *testing.T) {
	r, reg, _ := testResolver(t, contacts.Entry{
		Person: "Maya K", AdapterID: "whatsapp", Handle: "+15550000001",
		LastSeen: daysAgo(2), Source: contacts.Observed,
	})
	_ = reg.Disable("whatsapp", "ban risk spiked")

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if err == nil && !got.MustAsk {
		t.Fatalf("routed to a switched-off adapter: %+v", got)
	}
	if got.AdapterID == "whatsapp" {
		t.Fatal("the decision names a switched-off adapter")
	}
}

func TestAnAdapterThatDoesNotOfferTheVerbIsNotACandidate(t *testing.T) {
	// "get me a ride" must not land on an adapter that can only read
	// estimates. The verb filter is what makes that impossible rather than
	// merely unlikely.
	r, _, _ := testResolver(t)

	got, err := r.Resolve(context.Background(), route(manifest.Send, "notes", "", 0.97))
	if err == nil && !got.MustAsk {
		t.Fatalf("routed a send to a notes adapter: %+v", got)
	}
}

func TestADecisionForAnIrreversibleVerbAlwaysCarriesThePreviewRequirement(t *testing.T) {
	r, _, _ := testResolver(t, contacts.Entry{
		Person: "Maya K", AdapterID: "whatsapp", Handle: "+15550000001",
		LastSeen: daysAgo(2), Source: contacts.Observed,
	})

	got, _ := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if !got.RequiresPreview {
		t.Fatal("a send was routed without carrying the preview requirement")
	}

	read, _ := r.Resolve(context.Background(), route(manifest.Read, "messaging", "Maya", 0.95))
	if read.RequiresPreview {
		t.Error("a read carries a preview requirement")
	}
}

func TestNamedSlotsSurviveIntoTheDecision(t *testing.T) {
	// Stage 2 chooses the adapter; it must not drop the slots stage 1 found
	// on the way, or an adapter needing two of them can never be reached.
	resolver, _, _ := testResolver(t)
	decision, err := resolver.Resolve(context.Background(), stage1.Route{
		Verb: manifest.Read, AppClass: "notes", AppNamed: "notion",
		Subject:    "directions to the airport",
		Fields:     map[string]string{"origin": "home", "destination": "SFO"},
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Fields["origin"] != "home" || decision.Fields["destination"] != "SFO" {
		t.Fatalf("decision fields=%v, want both endpoints", decision.Fields)
	}
}
