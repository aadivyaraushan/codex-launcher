package eval

import (
	"testing"
)

// The whole point of this file: it runs offline, in seconds, against recorded
// stage-1 replies. Changing the prompt is the only thing that costs a live
// call. This is the cheapest loop in the plan and the one that decides whether
// the product feels intelligent.

const (
	topOneTarget = 0.95
	fixture      = "testdata/routing-eval.json"
)

func TestTheEvalSetRoutesAtOrAboveTheShippingTarget(t *testing.T) {
	set, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load(%s) failed: %v", fixture, err)
	}
	if len(set.Cases) < 20 {
		t.Fatalf("the eval set has %d cases; too few to mean anything", len(set.Cases))
	}

	result, err := set.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	for _, miss := range result.Misses {
		t.Errorf("MISS %q\n  want %s\n  got  %s\n  why  %s",
			miss.Utterance, miss.Want, miss.Got, miss.Why)
	}

	if result.TopOne() < topOneTarget {
		t.Fatalf("top-1 is %.1f%% (%d/%d), target is %.0f%%",
			result.TopOne()*100, result.Hits, result.Total, topOneTarget*100)
	}
	t.Logf("top-1 %.1f%% (%d/%d)", result.TopOne()*100, result.Hits, result.Total)
}

func TestNoLowConfidenceRouteEverExecutes(t *testing.T) {
	// This one has no percentage attached. It is zero or the wave does not
	// ship. A low-confidence route that acts is the failure the whole
	// two-stage design is arranged to prevent.
	set, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	result, err := set.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if n := len(result.LowConfidenceExecutions); n != 0 {
		for _, u := range result.LowConfidenceExecutions {
			t.Errorf("a low-confidence route executed instead of asking: %q", u)
		}
		t.Fatalf("%d low-confidence executions; the target is zero", n)
	}
}

func TestEveryAdapterInTheSetCarriesAdversarialCases(t *testing.T) {
	// Every new adapter adds its own utterances AND three adversarial ones
	// aimed at stealing traffic from a neighbouring adapter. Without this
	// check the rule is a sentence in a plan rather than a property of the
	// file.
	set, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	adversarial := 0
	for _, c := range set.Cases {
		if c.Adversarial != "" {
			adversarial++
		}
	}
	if adversarial < 3*len(set.Classes) {
		t.Fatalf("the set has %d adversarial cases for %d app classes; want at least 3 per class",
			adversarial, len(set.Classes))
	}
}

func TestEveryCaseSaysWhyItExpectsWhatItExpects(t *testing.T) {
	// A case with no reason behind it becomes unmaintainable the first time
	// it fails, because nobody can tell whether the expectation or the code
	// is wrong.
	set, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	for _, c := range set.Cases {
		if c.Why == "" {
			t.Errorf("case %q has no \"why\"", c.Utterance)
		}
		if c.Expect.MustAsk && c.Expect.Adapter != "" {
			t.Errorf("case %q expects an ask AND an adapter; pick one", c.Utterance)
		}
		if !c.Expect.MustAsk && c.Expect.Adapter == "" {
			t.Errorf("case %q expects a route to nothing", c.Utterance)
		}
	}
}

func TestTheWholeSetRunsWithoutTouchingTheNetwork(t *testing.T) {
	// Guarded structurally: Run drives stage 1 from the recorded reply on
	// each case and has no way to reach a live model.
	set, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	for _, c := range set.Cases {
		if len(c.Reply) == 0 {
			t.Errorf("case %q has no recorded reply, so running it would need a live call", c.Utterance)
		}
	}
}
