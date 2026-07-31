// Package eval is the offline routing harness: it replays a fixture of
// recorded stage-1 replies through stage 2 and reports how often the
// router lands where it should. It never reaches the network — every case
// carries its reply already recorded, and changing the prompt is the only
// thing that costs a live call.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// evalNow is the fixed clock the eval set resolves contacts against. A
// real clock would make last_seen_days drift into a different answer every
// day the eval runs; a fixed one makes it arithmetic.
var evalNow = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

// fixtureAdapter is one entry in the fixture's "adapters" list: a bare id
// and the verbs it declares, enough to build a stub registered adapter.
type fixtureAdapter struct {
	ID    string          `json:"id"`
	Verbs []manifest.Verb `json:"verbs"`
}

// fixtureContact is one entry in the fixture's "contacts" list. A nil
// LastSeenDays means an address-book entry; any other value is an
// observed sighting that many days before evalNow.
type fixtureContact struct {
	Person       string `json:"person"`
	Adapter      string `json:"adapter"`
	Handle       string `json:"handle"`
	LastSeenDays *int   `json:"last_seen_days"`
	Source       string `json:"source"`
}

// Case is one utterance the eval set expects a specific outcome for.
type Case struct {
	Utterance string          `json:"utterance"`
	Reply     json.RawMessage `json:"reply"`
	Expect    struct {
		Adapter string `json:"adapter"`
		Verb    string `json:"verb"`
		MustAsk bool   `json:"must_ask"`
	} `json:"expect"`
	Adversarial string `json:"adversarial"`
	Why         string `json:"why"`
}

// Set is a loaded eval fixture: the cases to run, and the world (adapters,
// classes, contacts) they run against.
type Set struct {
	Cases    []Case
	Classes  stage2.ClassMap
	adapters []fixtureAdapter
	contacts []fixtureContact
}

// fixtureFile is the on-disk shape of the whole fixture.
type fixtureFile struct {
	Adapters []fixtureAdapter `json:"adapters"`
	Classes  stage2.ClassMap  `json:"classes"`
	Contacts []fixtureContact `json:"contacts"`
	Cases    []Case           `json:"cases"`
}

// Load reads and parses an eval fixture from path.
func Load(path string) (*Set, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: reading %s: %w", path, err)
	}
	var f fixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("eval: parsing %s: %w", path, err)
	}
	return &Set{
		Cases:    f.Cases,
		Classes:  f.Classes,
		adapters: f.Adapters,
		contacts: f.Contacts,
	}, nil
}

// Miss records one case whose actual outcome did not match what it
// expected.
type Miss struct {
	Utterance string
	Want      string
	Got       string
	Why       string
}

// Result is the outcome of running a Set.
type Result struct {
	Hits                    int
	Total                   int
	Misses                  []Miss
	LowConfidenceExecutions []string
}

// TopOne is the fraction of cases that landed on the expected outcome.
func (r *Result) TopOne() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Hits) / float64(r.Total)
}

// stubAdapter is a minimal adapter.Adapter that declares exactly the verbs
// the fixture says it should, so the eval registry matches the fixture's
// declared world without needing any real adapter implementation.
type stubAdapter struct{ m manifest.Manifest }

func (s *stubAdapter) Describe() manifest.Manifest { return s.m }
func (s *stubAdapter) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{AdapterID: s.m.ID}, nil
}
func (s *stubAdapter) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}
func (s *stubAdapter) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	return adapter.Outcome{Reached: s.m.Ceiling, Done: true}, nil
}
func (s *stubAdapter) Revoke(context.Context) error { return nil }

func buildRegistry(adapters []fixtureAdapter) (*registry.Registry, error) {
	reg := registry.New()
	for _, fa := range adapters {
		m := manifest.Manifest{
			ID:            fa.ID,
			Runtime:       manifest.RT2,
			Verbs:         fa.Verbs,
			Ceiling:       manifest.Completes,
			Consent:       manifest.ConsentA,
			Auth:          manifest.AuthOAuth,
			Cost:          manifest.CostFree,
			Gates:         []manifest.Gate{manifest.GateNone},
			Capacity:      manifest.Capacity{Kind: manifest.CapacityNone},
			Region:        []string{"global"},
			Platform:      manifest.PlatformBoth,
			ProvesCeiling: fa.ID + "-smoke",
		}
		if err := reg.Register(&stubAdapter{m: m}); err != nil {
			return nil, fmt.Errorf("eval: registering %s: %w", fa.ID, err)
		}
	}
	return reg, nil
}

func buildGraph(contactRows []fixtureContact) *contacts.Graph {
	graph := contacts.NewGraph(func() time.Time { return evalNow })
	for _, c := range contactRows {
		e := contacts.Entry{Person: c.Person, AdapterID: c.Adapter, Handle: c.Handle}
		if c.LastSeenDays == nil {
			e.Source = contacts.AddressBook
		} else {
			e.Source = contacts.Observed
			e.LastSeen = evalNow.AddDate(0, 0, -*c.LastSeenDays)
		}
		graph.Add(e)
	}
	return graph
}

func wantString(c Case) string {
	if c.Expect.MustAsk {
		return "ask"
	}
	return fmt.Sprintf("%s/%s", c.Expect.Adapter, c.Expect.Verb)
}

func gotString(dec stage2.Decision) string {
	if dec.MustAsk {
		return "ask"
	}
	return fmt.Sprintf("%s/%s", dec.AdapterID, dec.Verb)
}

func decisionMatches(dec stage2.Decision, c Case) bool {
	if c.Expect.MustAsk {
		return dec.MustAsk
	}
	return !dec.MustAsk && dec.AdapterID == c.Expect.Adapter && string(dec.Verb) == c.Expect.Verb
}

// Run drives every case in the set through stage 1 (parsing the case's
// recorded reply) and stage 2 (resolving it against a fresh registry and
// contact graph built from the fixture), and compares the outcome against
// what the case expects. It never touches the network: every reply is
// already recorded on the case.
func (s *Set) Run() (*Result, error) {
	reg, err := buildRegistry(s.adapters)
	if err != nil {
		return nil, err
	}
	graph := buildGraph(s.contacts)
	resolver := stage2.New(reg, graph, s.Classes, manifest.PlatformAndroid)

	result := &Result{}
	ctx := context.Background()

	for _, c := range s.Cases {
		result.Total++

		route, err := stage1.ParseRoute(c.Reply)
		if err != nil {
			// A reply stage 1 refuses to parse never executes anything —
			// that counts as an ask, not as a harness error.
			if c.Expect.MustAsk {
				result.Hits++
			} else {
				result.Misses = append(result.Misses, Miss{
					Utterance: c.Utterance,
					Want:      wantString(c),
					Got:       "parse error: " + err.Error(),
					Why:       c.Why,
				})
			}
			continue
		}

		dec, err := resolver.Resolve(ctx, route)
		if err != nil {
			result.Misses = append(result.Misses, Miss{
				Utterance: c.Utterance,
				Want:      wantString(c),
				Got:       "resolve error: " + err.Error(),
				Why:       c.Why,
			})
			continue
		}

		if route.Confidence < stage1.ConfidenceFloor && !dec.MustAsk {
			result.LowConfidenceExecutions = append(result.LowConfidenceExecutions, c.Utterance)
		}

		if decisionMatches(dec, c) {
			result.Hits++
		} else {
			result.Misses = append(result.Misses, Miss{
				Utterance: c.Utterance,
				Want:      wantString(c),
				Got:       gotString(dec),
				Why:       c.Why,
			})
		}
	}

	return result, nil
}
