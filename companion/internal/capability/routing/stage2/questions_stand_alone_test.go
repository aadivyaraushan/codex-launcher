package stage2

import (
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
)

// A question has to be answerable from its own words, because its own words
// are the whole of what the user gets.
//
// Decision.Candidates is written in exactly one place — resolver.go, in the
// to-a-person branch — and read in none. The single path from here to a human
// is flow/service.go:102, which builds &QuestionError{Question: decision.Question}
// from the string and drops everything else. So a sentence that asks the user
// to choose from a list arrives with no list attached, whether the list was
// empty or full.
//
// Every other ask in Resolve already respects this and says something whole:
// "I don't have the app you named connected for this.", "No available app can
// handle this right now." The to-a-person branch is the only one that points
// at a list that cannot follow it.
//
// These two tests are the same rule applied to the two ways the branch fires.

// answerable reports whether a question can be acted on by someone who sees
// only this sentence. A question that asks "which" has to name the options it
// is asking between.
func answerable(question string, options []string) (bool, string) {
	asksToChoose := strings.Contains(strings.ToLower(question), "which") ||
		strings.Contains(strings.ToLower(question), "did you mean")
	if !asksToChoose {
		return true, ""
	}
	if len(options) == 0 {
		return false, "it asks the user to choose, but there is nothing to choose between"
	}
	for _, o := range options {
		if !strings.Contains(strings.ToLower(question), strings.ToLower(o)) {
			return false, "it asks the user to choose but never names " + o
		}
	}
	return true, ""
}

func TestAskingAboutAPersonWeHaveNeverSeenDoesNotOfferAChoice(t *testing.T) {
	// The production graph is empty and stays empty: Graph.Add's only callers
	// are tests and the offline eval harness. So this is not an edge case,
	// it is the only path a "message Maya" takes today.
	r, _, _ := testResolver(t)

	dec, err := r.Resolve(t.Context(), route(manifest.Send, "messaging", "Maya", 0.9))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !dec.MustAsk {
		t.Fatalf("expected an ask for an unknown person, got adapter %q", dec.AdapterID)
	}
	if len(dec.Candidates) != 0 {
		t.Fatalf("an unknown person should have no candidates, got %d", len(dec.Candidates))
	}

	if ok, why := answerable(dec.Question, nil); !ok {
		t.Errorf("the user is asked %q and %s.\n"+
			"We have never seen Maya, so the truthful thing to say is that we do "+
			"not know how to reach her — not to ask her which of her surfaces she "+
			"meant and then offer none. Graph.Resolve already separates the two: "+
			"an unknown person comes back with Rule 0 and no candidates, while a "+
			"real ambiguity comes back as RuleAsk. Resolve throws that apart away.",
			dec.Question, why)
	}
}

func TestAskingBetweenTwoSurfacesNamesThem(t *testing.T) {
	// Maya on two surfaces, both seen yesterday, so no rule can pick between
	// them and the resolver has to ask. Here the candidates genuinely exist —
	// and still never reach the user, because only the string travels.
	r, _, _ := testResolver(t,
		contacts.Entry{Person: "Maya", AdapterID: "sms", Handle: "+15551234567", LastSeen: daysAgo(1), Source: contacts.Observed},
		contacts.Entry{Person: "Maya", AdapterID: "whatsapp", Handle: "maya@wa", LastSeen: daysAgo(1), Source: contacts.Observed},
	)

	dec, err := r.Resolve(t.Context(), route(manifest.Send, "messaging", "Maya", 0.9))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !dec.MustAsk {
		t.Fatalf("expected an ask between two surfaces, got adapter %q", dec.AdapterID)
	}
	if len(dec.Candidates) != 2 {
		t.Fatalf("expected both surfaces as candidates, got %d", len(dec.Candidates))
	}

	if ok, why := answerable(dec.Question, []string{"sms", "whatsapp"}); !ok {
		t.Errorf("the user is asked %q and %s.\n"+
			"dec.Candidates holds both surfaces, but nothing reads that field: "+
			"flow/service.go:102 sends the question string on its own. Until a "+
			"candidate list can travel to the phone, the surfaces have to be named "+
			"in the sentence itself.", dec.Question, why)
	}
}
