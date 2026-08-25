// Package discovery drives raw utterances through the real phone router —
// stage 1's explicit-app model, then stage 2's resolver — end to end, so a
// corpus sweep can catch a misroute that a stage2-only eval (which starts
// from a pre-recorded Route and never runs stage 1 at all) cannot see.
package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// RouterFunc drives one utterance through a stage 1 model. It is the exact
// shape stage1.ModelFunc already uses, so the phone's explicit router and
// the OpenAI broker are both droppable in without a wrapper.
type RouterFunc = stage1.ModelFunc

// Case is one utterance a sweep checks, and the app/class/verb it should
// resolve to. ExpectedApp empty means the utterance should not resolve to
// any app at all — an app the phone's inventory deliberately omits, for
// instance. Beeper selects which inventory (or both) to run it against.
type Case struct {
	Utterance         string `json:"utterance"`
	ExpectedApp       string `json:"expected_app"`
	ExpectedClass     string `json:"expected_class"`
	ExpectedVerb      string `json:"expected_verb"`
	ExpectedOperation string `json:"expected_operation,omitempty"`
	Beeper            string `json:"beeper"`
	Note              string `json:"note,omitempty"`
}

// Verdict is what a Case's actual result was, held against what it expected.
type Verdict string

const (
	// PASS means the actual route matched what the Case expected.
	PASS Verdict = "PASS"
	// MISROUTE means the request resolved to the wrong app, class, or verb.
	MISROUTE Verdict = "MISROUTE"
	// DEAD_END means stage 2 had to ask rather than act — including the
	// hard-coded "I don't have the app you named connected for this."
	DEAD_END Verdict = "DEAD_END"
	// CAPABILITY_GAP means the request reached the right app and class,
	// but that adapter's manifest never declares the verb the Case expected
	// — a real capability limit, not a routing bug.
	CAPABILITY_GAP Verdict = "CAPABILITY_GAP"
)

// FailKind sub-classifies a MustAsk terminal outcome by which resolver
// question produced it, so a routing bug, an empty-contacts miss, and an
// acceptable clarifying question stop collapsing into the same DEAD_END.
const (
	FailKindOrphanOrUnnamed    = "orphan_or_unnamed"
	FailKindEmptyContacts      = "empty_contacts"
	FailKindDisambiguation     = "disambiguation"
	FailKindNoCapableAdapter   = "no_capable_adapter"
	FailKindSurfaceUnavailable = "surface_unavailable"
	FailKindMultiApp           = "multi_app"
	FailKindLowConfidence      = "low_confidence"
	FailKindUnknownClass       = "unknown_class"
	FailKindUndeclaredClass    = "undeclared_class"
	FailKindOther              = "other"
)

// Result is one Case run against one Beeper state.
type Result struct {
	Utterance       string
	Beeper          string
	ActualApp       string
	ActualClass     string
	ActualVerb      string
	ActualOperation string
	Decision        string
	Question        string
	ExpectedApp     string
	ExpectedClass   string
	ExpectedVerb    string
	// RouteOK reports whether stage1 alone aimed at the right app, class,
	// and verb, independent of whether stage2 could complete — so a
	// routing bug is never conflated with an empty-contacts dead end or
	// an acceptable clarifying question.
	RouteOK bool
	// FailKind sub-classifies a MustAsk terminal outcome (see the
	// FailKind* constants); it is "" whenever the decision executed.
	FailKind string
	Verdict  Verdict
	Reason   string
}

// Environment is one Beeper state's whole world: the stage 1 router built
// for that Beeper state, and enough of stage 2 to resolve a Route into a
// Decision and to look up what an adapter's manifest declares. It carries
// its own Router (rather than sharing one Router across both states)
// because Beeper on and off route some apps under different classes and
// verbs (see routing/phonerules) — a single shared router could never be
// right for both.
type Environment struct {
	// Router is the stage1 router for this Beeper state.
	Router   RouterFunc
	Resolve  func(ctx context.Context, route stage1.Route) (stage2.Decision, error)
	Manifest func(id string) (manifest.Manifest, error)
}

// Sweep drives a corpus of Cases through the stage 1 router and stage 2
// environment matching each Case's declared Beeper state.
type Sweep struct {
	BeeperOn  Environment
	BeeperOff Environment
}

// Run drives every Case through the Sweep, expanding Beeper: "both" into
// one Result per state. It returns an error only for a malformed Case
// (an unknown Beeper value); a routing failure inside a Case becomes a
// Result, not an error.
func (s Sweep) Run(ctx context.Context, cases []Case) ([]Result, error) {
	var results []Result
	for _, c := range cases {
		variants, err := beeperVariants(c.Beeper)
		if err != nil {
			return nil, fmt.Errorf("case %q: %w", c.Utterance, err)
		}
		for _, variant := range variants {
			env := s.BeeperOff
			if variant == "on" {
				env = s.BeeperOn
			}
			results = append(results, s.runOne(ctx, c, variant, env))
		}
	}
	return results, nil
}

func beeperVariants(value string) ([]string, error) {
	switch value {
	case "on":
		return []string{"on"}, nil
	case "off":
		return []string{"off"}, nil
	case "both":
		return []string{"on", "off"}, nil
	default:
		return nil, fmt.Errorf("discovery: beeper must be \"on\", \"off\", or \"both\", got %q", value)
	}
}

func (s Sweep) runOne(ctx context.Context, c Case, beeper string, env Environment) Result {
	result := Result{
		Utterance:     c.Utterance,
		Beeper:        beeper,
		ExpectedApp:   c.ExpectedApp,
		ExpectedClass: c.ExpectedClass,
		ExpectedVerb:  c.ExpectedVerb,
	}

	raw, err := env.Router(ctx, c.Utterance)
	if err != nil {
		result.RouteOK = routeOK(c, stage1.Route{}, false)
		result.Decision = "no_route"
		result.Verdict, result.Reason = classifyNoRoute(c, err)
		return result
	}
	route, err := stage1.ParseRoute(raw)
	if err != nil {
		result.RouteOK = routeOK(c, stage1.Route{}, false)
		result.Decision = "no_route"
		result.Verdict, result.Reason = classifyNoRoute(c, err)
		return result
	}
	result.RouteOK = routeOK(c, route, true)

	decision, err := env.Resolve(ctx, route)
	if err != nil {
		result.Decision = "no_route"
		result.Verdict, result.Reason = classifyNoRoute(c, err)
		return result
	}
	result.ActualOperation = route.Fields["operation"]

	if decision.MustAsk {
		result.ActualApp = route.AppNamed
		result.ActualClass = route.AppClass
		result.ActualVerb = string(route.Verb)
		result.Decision = "must_ask"
		result.Question = decision.Question
		result.FailKind = classifyFailKind(decision.Question)
		result.Verdict, result.Reason = classifyMustAsk(c, decision)
		return result
	}

	result.ActualApp = decision.AdapterID
	result.ActualClass = route.AppClass
	result.ActualVerb = string(decision.Verb)
	result.Decision = "executed"
	result.Verdict, result.Reason = classifyExecuted(env.Manifest, c, result.ActualApp, result.ActualClass, result.ActualVerb, result.ActualOperation)
	return result
}

// routeOK reports whether stage1 alone aimed at the app/class/verb a Case
// expects, independent of anything stage2 does with that route. hasRoute
// is false when stage1 or route parsing failed outright.
func routeOK(c Case, route stage1.Route, hasRoute bool) bool {
	if c.ExpectedApp == "" {
		return !hasRoute
	}
	if !hasRoute {
		return false
	}
	return strings.EqualFold(route.AppNamed, c.ExpectedApp) &&
		route.AppClass == c.ExpectedClass &&
		string(route.Verb) == c.ExpectedVerb
}

// classifyFailKind maps a MustAsk decision's Question to one of the
// resolver's known refusal reasons (internal/capability/routing/stage2/resolver.go).
// Matching is on the stable, non-interpolated part of each sentence, never
// full-string equality, since several of these sentences interpolate a
// name.
func classifyFailKind(question string) string {
	switch {
	case strings.HasPrefix(question, "I don't have the app you named connected for this."):
		return FailKindOrphanOrUnnamed
	case strings.HasPrefix(question, "I don't know how to reach "):
		return FailKindEmptyContacts
	case strings.Contains(question, " is on ") && strings.Contains(question, " — which did you mean?"):
		return FailKindDisambiguation
	case strings.HasPrefix(question, "No available app can handle this right now."):
		return FailKindNoCapableAdapter
	case strings.HasPrefix(question, "The surface I'd normally use for this isn't available"):
		return FailKindSurfaceUnavailable
	case strings.HasPrefix(question, "More than one app could handle this"):
		return FailKindMultiApp
	case strings.HasPrefix(question, "I'm not confident enough"):
		return FailKindLowConfidence
	case strings.HasPrefix(question, "I don't know which app to use for"):
		return FailKindUnknownClass
	case strings.Contains(question, "was never declared as addressed to"):
		return FailKindUndeclaredClass
	default:
		return FailKindOther
	}
}

func classifyNoRoute(c Case, err error) (Verdict, string) {
	if c.ExpectedApp == "" {
		return PASS, fmt.Sprintf("stage1 found no explicit app (%v), matching the expectation that this utterance is not routable", err)
	}
	return MISROUTE, fmt.Sprintf("stage1 could not route to %q: %v", c.ExpectedApp, err)
}

func classifyMustAsk(c Case, decision stage2.Decision) (Verdict, string) {
	if c.ExpectedApp == "" {
		return PASS, "stage2 asked rather than routing, matching the expectation that this utterance is not routable"
	}
	return DEAD_END, decision.Question
}

func classifyExecuted(lookup func(string) (manifest.Manifest, error), c Case, actualApp, actualClass, actualVerb, actualOperation string) (Verdict, string) {
	if c.ExpectedApp == "" {
		return MISROUTE, fmt.Sprintf("routed to %q when no app was expected", actualApp)
	}
	if actualApp != c.ExpectedApp || actualClass != c.ExpectedClass {
		return MISROUTE, fmt.Sprintf("got app=%q class=%q, expected app=%q class=%q", actualApp, actualClass, c.ExpectedApp, c.ExpectedClass)
	}
	if c.ExpectedVerb != "" && actualVerb != c.ExpectedVerb {
		if m, err := lookup(actualApp); err == nil && !m.Allows(manifest.Verb(c.ExpectedVerb)) {
			return CAPABILITY_GAP, fmt.Sprintf("%s does not declare verb %q (declares %v)", actualApp, c.ExpectedVerb, m.Verbs)
		}
		return MISROUTE, fmt.Sprintf("got verb=%q, expected verb=%q", actualVerb, c.ExpectedVerb)
	}
	if c.ExpectedOperation != "" && actualOperation != c.ExpectedOperation {
		return MISROUTE, fmt.Sprintf("got operation=%q, expected operation=%q", actualOperation, c.ExpectedOperation)
	}
	return PASS, "matches expected app, class, and verb"
}
