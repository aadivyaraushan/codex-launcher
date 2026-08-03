package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notificationreply"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// Writing the adapter is not the job. Making it reachable is.
//
// This repository's most repeated defect is a subsystem that is finished,
// commented, and fully unit-tested, with nothing in production that can reach
// it. Its own test file passes, so nothing complains. The reply route is
// already an example — every link had a real caller and the chain still never
// ran, because nothing constructed the thing that starts it.
//
// So the adapter's own tests (adapters/notificationreply/adapter_test.go) are
// deliberately not enough. These tests ask the harder question: given the
// production build exactly as it ships, does "reply to Maya" actually arrive at
// this adapter?
//
// There is one specific trap in the way, and it is why this file exists rather
// than a line in the adapter's own tests. Before any adapter is chosen, a class
// declares how it is addressed. `to_a_person` sends the request through the
// contact graph to turn a name into a handle. **The production contact graph is
// permanently empty** — it is built inline at production.go and `Graph.Add` has
// no production caller anywhere, only two test files and the offline eval
// harness. So any class marked `to_a_person` answers every request with "which
// of Maya's surfaces did you mean?" and an empty list of choices.
//
// Putting reply in the existing `messaging` class would therefore make it
// unreachable on day one, in a way its own passing tests would never show.
//
// It also would not be right even if the graph were full. Only the phone knows
// which conversations still have a live notification to reply into, and it
// already matches a name against them. The Mac resolving the person first is
// both broken and redundant.
//
// Hence the third addressing word. `to_a_thing` says "no person to resolve" and
// that is a lie here — there is a person, and getting them wrong sends a real
// message to the wrong human. `to_a_person` says "resolve them here" and that
// is also a lie. Reusing either one would be this codebase's other recurring
// defect: one word standing for two different truths. The class needs to say
// what is true — there is a person, and the device resolves them.

// replyRoute is what stage 1 produces for "reply to Maya: on my way".
func replyRoute(subject string) stage1.Route {
	return stage1.Route{
		Verb:       manifest.Send,
		AppClass:   notificationreply.Class,
		Subject:    subject,
		Body:       "on my way",
		Confidence: 0.9,
	}
}

// The headline test. The graph here is empty on purpose — that is not a
// simplification of production, it *is* production.
func TestAReplyReachesTheAdapterWithAnEmptyContactBook(t *testing.T) {
	flow, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	_ = flow

	// inv.reg is the registry production actually built — this test lives in
	// the same package so it can use the real one rather than a stand-in.
	resolver := stage2.New(inv.reg, contacts.NewGraph(time.Now), classMapFor(inv.Classes), manifest.PlatformAndroid)

	dec, err := resolver.Resolve(context.Background(), replyRoute("Maya"))
	if err != nil {
		t.Fatalf("resolving a reply failed: %v", err)
	}
	if dec.MustAsk {
		t.Fatalf("a reply was turned into a question (%q) with %d choices offered; the phone knows who Maya is and the Mac must not ask", dec.Question, len(dec.Candidates))
	}
	if dec.AdapterID != notificationreply.ID {
		t.Fatalf("a reply resolved to %q, want %q", dec.AdapterID, notificationreply.ID)
	}
}

// The adapter must actually be in the shipped build. An adapter that exists in
// the tree but was never registered is the defect this file is named after.
func TestTheReplyAdapterIsInTheShippedBuild(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	var registered bool
	for _, id := range inv.Registered {
		if id == notificationreply.ID {
			registered = true
		}
	}
	if !registered {
		if why, skipped := inv.Skipped[notificationreply.ID]; skipped {
			t.Fatalf("the reply adapter was deliberately left out: %s", why)
		}
		t.Fatalf("the reply adapter is in neither the registered list nor the skipped list, so nobody can tell whether it was left out on purpose or forgotten; registered=%s", strings.Join(inv.Registered, ","))
	}

	if ids := inv.Classes[notificationreply.Class]; len(ids) != 1 || ids[0] != notificationreply.ID {
		t.Fatalf("class %q holds %v; a reply must have exactly one answer or the resolver stops to ask which app", notificationreply.Class, ids)
	}
}

// First control. Starting a conversation must still go the old way — opened in
// the app, with a draft, claiming nothing. If the reply class swallowed compose
// too, a user asking to message someone new would get a refusal.
func TestStartingAConversationStillGoesToTheHandOffAdapters(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	for _, id := range inv.Classes["messaging"] {
		if id == notificationreply.ID {
			t.Fatal("the reply adapter joined the messaging class, where it would take compose requests it cannot fulfil")
		}
	}
	if len(inv.Classes["messaging"]) == 0 {
		t.Fatal("the messaging class is now empty; starting a conversation has no adapter at all")
	}
}

// Second control, on the addressing itself rather than on a happy path: a class
// that never declares how it is addressed is refused by the resolver, by
// design. This catches the reply class being added to the adapter map and
// forgotten in the addressing map — which is exactly how the OAuth skip list
// lost notion.
func TestEveryClassInTheBuildSaysHowItIsAddressed(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	classes := classMapFor(inv.Classes)
	for name := range inv.Classes {
		if classes[name].Addressing == stage2.AddressingUndeclared {
			t.Fatalf("class %q never says how it is addressed, so the resolver refuses every request to it", name)
		}
	}
}

// Third control, and the reason the third word had to be added rather than
// borrowed. Reply must not consult the contact graph, and messaging must. Both
// halves matter: if reply consulted it, it would be dead on an empty book; if
// messaging stopped consulting it, a "message Maya" would pick an app at random
// and send to whoever it found.
func TestAReplySkipsTheContactBookAndOrdinaryMessagingStillUsesIt(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	classes := classMapFor(inv.Classes)

	if got := classes[notificationreply.Class].Addressing; got != stage2.ResolvedOnTheDevice {
		t.Fatalf("the reply class is addressed %q; it must be resolved_on_the_device — see the note at the top of this file", got)
	}
	if got := classes["messaging"].Addressing; got != stage2.ToAPerson {
		t.Fatalf("the messaging class is addressed %q, want to_a_person; a request to message a person must still be resolved before anything is sent", got)
	}
}
