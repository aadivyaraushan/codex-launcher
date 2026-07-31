// Package contacts holds the on-device contact graph: which handle a
// person is reached at, on which adapter, and how recently. It is the only
// place a subject name (what the cloud router heard) turns into a handle
// (a phone number, a thread id, an address) — that turn never happens in
// the cloud, and it never guesses between two different humans.
package contacts

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Source is where a contact graph entry came from. The graph has exactly
// two sources on purpose: a thread we have already seen, or the device
// address book. Nothing is scraped or enriched.
type Source int

const (
	Observed Source = iota
	AddressBook
)

// AllSources is the closed set of sources an entry may carry.
var AllSources = []Source{Observed, AddressBook}

// String renders a Source in its written form, so it prints readably (and
// satisfies fmt.Stringer for %q/%s in error messages and test output).
func (s Source) String() string {
	switch s {
	case Observed:
		return "observed"
	case AddressBook:
		return "addressbook"
	default:
		return "unknown"
	}
}

// Entry is one known way to reach a person: a handle on a specific
// adapter, and when it was last used (zero for an address-book entry that
// has never actually been used).
type Entry struct {
	Person    string
	AdapterID string
	Handle    string
	LastSeen  time.Time
	Source    Source
}

// Rule names which resolution rule produced a Decision, in the fixed order
// those rules are tried.
type Rule int

const (
	RuleAppNamed Rule = iota + 1
	RulePinned
	RuleOnlyCandidate
	RuleClearlyRecent
	RuleAsk
)

// Decision is the result of resolving a subject: either a single entry to
// use, or a question the caller must put to the user, carrying whichever
// candidates it was choosing between.
type Decision struct {
	MustAsk    bool
	Rule       Rule
	Entry      Entry
	Candidates []Entry
}

// Graph is the on-device contact graph. It is safe for concurrent use.
type Graph struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]Entry
	pins    map[string]string // person -> pinned adapter id
}

// NewGraph returns an empty graph. now is injected so recency rules are
// arithmetic against a fixed clock rather than a race with the real one.
func NewGraph(now func() time.Time) *Graph {
	return &Graph{
		now:     now,
		entries: make(map[string]Entry),
		pins:    make(map[string]string),
	}
}

func entryKey(person, adapterID, handle string) string {
	return person + "\x00" + adapterID + "\x00" + handle
}

// Add records an entry, deduping on person+adapter+handle: a newer sighting
// replaces an older one at the same key rather than duplicating it.
func (g *Graph) Add(e Entry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := entryKey(e.Person, e.AdapterID, e.Handle)
	if existing, ok := g.entries[key]; ok && existing.LastSeen.After(e.LastSeen) {
		return
	}
	g.entries[key] = e
}

// Pin fixes a person to a specific adapter, refusing to pin them to an
// adapter they have no entry on.
func (g *Graph) Pin(person, adapterID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.hasEntryLocked(person, adapterID) {
		return fmt.Errorf("contacts: no entry for %q on %q", person, adapterID)
	}
	g.pins[person] = adapterID
	return nil
}

func (g *Graph) hasEntryLocked(person, adapterID string) bool {
	for _, e := range g.entries {
		if e.Person == person && e.AdapterID == adapterID {
			return true
		}
	}
	return false
}

// Answer records the user's answer to an ask as a pin, so the same subject
// resolves without asking again next time.
func (g *Graph) Answer(subject string, chosen Entry) error {
	return g.Pin(chosen.Person, chosen.AdapterID)
}

// Wipe empties the graph entirely: every entry and every pin.
func (g *Graph) Wipe() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entries = make(map[string]Entry)
	g.pins = make(map[string]string)
}

// Size reports how many entries the graph currently holds.
func (g *Graph) Size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.entries)
}

// matchesPerson reports whether subject names person: either exactly (case
// insensitive) or as a prefix of it that ends on a word boundary, so
// "Maya" matches "Maya K" and "Maya H" but not "Mayak".
func matchesPerson(subject, person string) bool {
	subject = strings.ToLower(strings.TrimSpace(subject))
	person = strings.ToLower(strings.TrimSpace(person))
	if subject == "" {
		return false
	}
	if subject == person {
		return true
	}
	return strings.HasPrefix(person, subject+" ")
}

// sortCandidates orders entries newest-seen first, then by adapter id, so
// an ask's candidate list is deterministic.
func sortCandidates(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].LastSeen.Equal(entries[j].LastSeen) {
			return entries[i].LastSeen.After(entries[j].LastSeen)
		}
		return entries[i].AdapterID < entries[j].AdapterID
	})
}

// withinWindow returns the entries seen within window of now, excluding
// address-book entries (zero LastSeen), which are never "recent".
func withinWindow(entries []Entry, now time.Time, window time.Duration) []Entry {
	var out []Entry
	for _, e := range entries {
		if e.LastSeen.IsZero() {
			continue
		}
		age := now.Sub(e.LastSeen)
		if age >= 0 && age <= window {
			out = append(out, e)
		}
	}
	return out
}

// Resolve turns a subject name (and, if the utterance said so, a named
// app) into a Decision, trying the rules in a fixed order and stopping at
// the first one that fires:
//
//  1. More than one distinct person matches the subject: always ask.
//  2. The utterance named the app: resolve to that person's entry on it,
//     or ask if they have none there — never substitute a neighbour.
//  3. A pin exists for the person: resolve to it.
//  4. The person has exactly one entry: resolve to it regardless of age.
//  5. The person has exactly one entry inside 14 days and no other entry
//     inside 90 days: resolve to it. Otherwise ask.
func (g *Graph) Resolve(subject, appNamed string) Decision {
	g.mu.Lock()
	defer g.mu.Unlock()

	byPerson := make(map[string][]Entry)
	for _, e := range g.entries {
		byPerson[e.Person] = append(byPerson[e.Person], e)
	}

	var matchedPersons []string
	for person := range byPerson {
		if matchesPerson(subject, person) {
			matchedPersons = append(matchedPersons, person)
		}
	}

	if len(matchedPersons) == 0 {
		return Decision{MustAsk: true}
	}

	if len(matchedPersons) > 1 {
		var candidates []Entry
		for _, p := range matchedPersons {
			candidates = append(candidates, byPerson[p]...)
		}
		sortCandidates(candidates)
		return Decision{MustAsk: true, Rule: RuleAsk, Candidates: candidates}
	}

	person := matchedPersons[0]
	personEntries := append([]Entry(nil), byPerson[person]...)
	sortCandidates(personEntries)

	if appNamed != "" {
		for _, e := range personEntries {
			if e.AdapterID == appNamed {
				return Decision{Entry: e, Rule: RuleAppNamed}
			}
		}
		return Decision{MustAsk: true, Rule: RuleAsk, Candidates: personEntries}
	}

	if pinnedAdapter, ok := g.pins[person]; ok {
		for _, e := range personEntries {
			if e.AdapterID == pinnedAdapter {
				return Decision{Entry: e, Rule: RulePinned}
			}
		}
	}

	if len(personEntries) == 1 {
		return Decision{Entry: personEntries[0], Rule: RuleOnlyCandidate}
	}

	now := g.now()
	within14 := withinWindow(personEntries, now, 14*24*time.Hour)
	within90 := withinWindow(personEntries, now, 90*24*time.Hour)
	if len(within14) == 1 && len(within90) == 1 {
		return Decision{Entry: within14[0], Rule: RuleClearlyRecent}
	}

	return Decision{MustAsk: true, Rule: RuleAsk, Candidates: personEntries}
}
