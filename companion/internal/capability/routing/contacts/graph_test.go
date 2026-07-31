package contacts

import (
	"testing"
	"time"
)

// A fixed "now" so recency rules are arithmetic rather than a race with the
// clock, and so the whole file runs offline in milliseconds.
var now = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return now.AddDate(0, 0, -n) }

func newTestGraph(entries ...Entry) *Graph {
	g := NewGraph(func() time.Time { return now })
	for _, e := range entries {
		g.Add(e)
	}
	return g
}

func observed(person, adapterID, handle string, seen time.Time) Entry {
	return Entry{Person: person, AdapterID: adapterID, Handle: handle, LastSeen: seen, Source: Observed}
}

func fromAddressBook(person, adapterID, handle string) Entry {
	return Entry{Person: person, AdapterID: adapterID, Handle: handle, Source: AddressBook}
}

// ---- rule 1: the utterance names the app --------------------------------

func TestNamingTheAppInTheUtteranceSettlesIt(t *testing.T) {
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(2)),
		observed("Maya K", "telegram", "@mayak", daysAgo(90)),
	)

	got := g.Resolve("Maya", "telegram")

	if got.MustAsk {
		t.Fatalf("naming the app still asked: %+v", got)
	}
	if got.Rule != RuleAppNamed {
		t.Errorf("rule = %d, want %d (app named)", got.Rule, RuleAppNamed)
	}
	if got.Entry.AdapterID != "telegram" {
		t.Errorf("resolved to %s, want telegram", got.Entry.AdapterID)
	}
}

func TestANamedAppBeatsAPinPointingSomewhereElse(t *testing.T) {
	// The user just said which app. That is more current than a pin they set
	// weeks ago, and the order of the rules says so.
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(2)),
		observed("Maya K", "telegram", "@mayak", daysAgo(90)),
	)
	if err := g.Pin("Maya K", "whatsapp"); err != nil {
		t.Fatalf("Pin failed: %v", err)
	}

	got := g.Resolve("Maya", "telegram")
	if got.MustAsk || got.Entry.AdapterID != "telegram" {
		t.Fatalf("a named app did not beat a pin: %+v", got)
	}
}

func TestNamingAnAppThePersonIsNotOnAsksRatherThanSubstituting(t *testing.T) {
	// Quietly sending on a different app than the one the user named is a
	// wrong-app failure the user did not consent to.
	g := newTestGraph(observed("Maya K", "whatsapp", "+15550000001", daysAgo(2)))

	got := g.Resolve("Maya", "telegram")
	if !got.MustAsk {
		t.Fatalf("substituted %s for the telegram the user asked for", got.Entry.AdapterID)
	}
}

// ---- rule 2: pins always beat recency -----------------------------------

func TestAPinBeatsAMoreRecentlyUsedApp(t *testing.T) {
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)),
		observed("Maya K", "telegram", "@mayak", daysAgo(60)),
	)
	_ = g.Pin("Maya K", "telegram")

	got := g.Resolve("Maya", "")
	if got.MustAsk {
		t.Fatalf("a pinned contact still asked: %+v", got)
	}
	if got.Rule != RulePinned {
		t.Errorf("rule = %d, want %d (pinned)", got.Rule, RulePinned)
	}
	if got.Entry.AdapterID != "telegram" {
		t.Errorf("resolved to %s, want the pinned telegram", got.Entry.AdapterID)
	}
}

func TestPinningSomeoneWeHaveNoEntryForIsAnError(t *testing.T) {
	g := newTestGraph(observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)))
	if err := g.Pin("Maya K", "signal"); err == nil {
		t.Fatal("pinned an adapter this person has no handle on")
	}
}

// ---- rule 3: exactly one candidate --------------------------------------

func TestOneCandidateResolvesWithoutAsking(t *testing.T) {
	g := newTestGraph(observed("Maya K", "whatsapp", "+15550000001", daysAgo(400)))

	got := g.Resolve("Maya", "")
	if got.MustAsk {
		t.Fatalf("a single candidate still asked: %+v", got)
	}
	if got.Rule != RuleOnlyCandidate {
		t.Errorf("rule = %d, want %d (only candidate)", got.Rule, RuleOnlyCandidate)
	}
}

func TestNobodyMatchingIsAnAskNotAnEmptyResolution(t *testing.T) {
	g := newTestGraph(observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)))

	got := g.Resolve("Devansh", "")
	if !got.MustAsk {
		t.Fatal("an unknown name resolved to something")
	}
	if len(got.Candidates) != 0 {
		t.Errorf("an unknown name produced candidates: %+v", got.Candidates)
	}
}

// ---- rule 4: one candidate is clearly recent ----------------------------

func TestAClearlyRecentCandidateWinsWhenNoOtherIsAnywhereNear(t *testing.T) {
	// Used in the last 14 days, and no other candidate inside 90.
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(3)),
		observed("Maya K", "telegram", "@mayak", daysAgo(200)),
	)

	got := g.Resolve("Maya", "")
	if got.MustAsk {
		t.Fatalf("a clearly recent candidate still asked: %+v", got)
	}
	if got.Rule != RuleClearlyRecent {
		t.Errorf("rule = %d, want %d (clearly recent)", got.Rule, RuleClearlyRecent)
	}
	if got.Entry.AdapterID != "whatsapp" {
		t.Errorf("resolved to %s, want whatsapp", got.Entry.AdapterID)
	}
}

func TestTwoCandidatesInsideNinetyDaysIsAlwaysAnAsk(t *testing.T) {
	// This is the case the rule is carefully drawn to catch. One is recent,
	// but the other is live enough that picking is a guess.
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(3)),
		observed("Maya K", "telegram", "@mayak", daysAgo(40)),
	)

	got := g.Resolve("Maya", "")
	if !got.MustAsk {
		t.Fatalf("guessed %s between two live candidates", got.Entry.AdapterID)
	}
	if len(got.Candidates) != 2 {
		t.Errorf("the ask offers %d candidates, want 2", len(got.Candidates))
	}
}

func TestNothingUsedInsideFourteenDaysIsAnAsk(t *testing.T) {
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(30)),
		observed("Maya K", "telegram", "@mayak", daysAgo(200)),
	)

	if got := g.Resolve("Maya", ""); !got.MustAsk {
		t.Fatalf("guessed %s with nothing recent to go on", got.Entry.AdapterID)
	}
}

// ---- rule 5: never guess between people ---------------------------------

func TestTwoPeopleWithTheSameFirstNameIsAlwaysAnAsk(t *testing.T) {
	// Wrong app is recoverable and embarrassing. Wrong person is not. The
	// router may be confident about verbs and apps; never about which human.
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)),
		fromAddressBook("Maya H", "imessage", "maya.h@example.com"),
	)

	got := g.Resolve("Maya", "")
	if !got.MustAsk {
		t.Fatalf("guessed between two people and picked %s", got.Entry.Person)
	}
	if got.Rule != RuleAsk {
		t.Errorf("rule = %d, want %d (ask)", got.Rule, RuleAsk)
	}
}

func TestTwoPeopleAreAnAskEvenWhenOnlyOneOfThemIsRecent(t *testing.T) {
	// Recency is allowed to choose between two apps for one person. It is
	// never allowed to choose between two people.
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)),
		observed("Maya H", "whatsapp", "+15550000002", daysAgo(300)),
	)

	if got := g.Resolve("Maya", ""); !got.MustAsk {
		t.Fatalf("recency picked between two people: %s", got.Entry.Person)
	}
}

func TestNamingTheAppDoesNotResolveWhichPersonItIs(t *testing.T) {
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)),
		observed("Maya H", "whatsapp", "+15550000002", daysAgo(2)),
	)

	if got := g.Resolve("Maya", "whatsapp"); !got.MustAsk {
		t.Fatalf("naming the app picked a person: %s", got.Entry.Person)
	}
}

func TestAFullNameNarrowsToOnePerson(t *testing.T) {
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)),
		observed("Maya H", "whatsapp", "+15550000002", daysAgo(2)),
	)

	got := g.Resolve("Maya K", "")
	if got.MustAsk {
		t.Fatalf("a full name still asked: %+v", got)
	}
	if got.Entry.Person != "Maya K" {
		t.Errorf("resolved to %s, want Maya K", got.Entry.Person)
	}
}

func TestNamesMatchWithoutRegardToCase(t *testing.T) {
	g := newTestGraph(observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)))

	if got := g.Resolve("maya k", ""); got.MustAsk {
		t.Fatalf("a lowercase name failed to match: %+v", got)
	}
}

func TestABrandNewContactWithNoHistoryAlwaysAsks(t *testing.T) {
	// The first message to someone is exactly when a wrong guess is most
	// expensive, so a never-seen address-book entry is not a free pass —
	// unless it is genuinely the only candidate, which rule 3 already covers.
	g := newTestGraph(
		fromAddressBook("Devansh R", "imessage", "devansh@example.com"),
		fromAddressBook("Devansh R", "whatsapp", "+15550000003"),
	)

	if got := g.Resolve("Devansh", ""); !got.MustAsk {
		t.Fatalf("guessed %s for a contact with no history at all", got.Entry.AdapterID)
	}
}

// ---- an ask is paid for once --------------------------------------------

func TestAnsweringTheAskStoresAPinSoItIsNotAskedAgain(t *testing.T) {
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(3)),
		observed("Maya K", "telegram", "@mayak", daysAgo(40)),
	)

	first := g.Resolve("Maya", "")
	if !first.MustAsk {
		t.Fatal("setup is wrong; this case should ask")
	}

	if err := g.Answer("Maya", first.Candidates[1]); err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	second := g.Resolve("Maya", "")
	if second.MustAsk {
		t.Fatalf("asked twice for the same thing: %+v", second)
	}
	if second.Entry.AdapterID != first.Candidates[1].AdapterID {
		t.Errorf("the answer was not honoured: got %s, want %s",
			second.Entry.AdapterID, first.Candidates[1].AdapterID)
	}
	if second.Rule != RulePinned {
		t.Errorf("the stored answer did not become a pin (rule = %d)", second.Rule)
	}
}

// ---- the most sensitive table in the product ----------------------------

func TestWipeLeavesNothingBehind(t *testing.T) {
	// It is wiped by the existing local-state-wipe path, so wiping has to
	// actually empty it rather than mark it stale.
	g := newTestGraph(
		observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)),
		observed("Devansh R", "telegram", "@dev", daysAgo(1)),
	)
	_ = g.Pin("Maya K", "whatsapp")

	g.Wipe()

	if n := g.Size(); n != 0 {
		t.Fatalf("graph still holds %d entries after a wipe", n)
	}
	if got := g.Resolve("Maya", ""); !got.MustAsk || len(got.Candidates) != 0 {
		t.Fatalf("a wiped graph still resolves: %+v", got)
	}
}

func TestAddingTheSameHandleTwiceUpdatesItRatherThanDuplicatingIt(t *testing.T) {
	g := newTestGraph(observed("Maya K", "whatsapp", "+15550000001", daysAgo(40)))
	g.Add(observed("Maya K", "whatsapp", "+15550000001", daysAgo(1)))

	if n := g.Size(); n != 1 {
		t.Fatalf("graph holds %d entries, want 1", n)
	}
	got := g.Resolve("Maya", "")
	if got.MustAsk || !got.Entry.LastSeen.Equal(daysAgo(1)) {
		t.Fatalf("the newer sighting did not replace the older one: %+v", got)
	}
}

func TestTheOnlySourcesAreThreadsWeAlreadySeeAndTheAddressBook(t *testing.T) {
	// No new collection, no scraping, no enrichment. Fixing the source list
	// in a test is what stops a fourth one being added quietly.
	if len(AllSources) != 2 {
		t.Fatalf("the graph has %d sources, want exactly 2: %v", len(AllSources), AllSources)
	}
	for _, want := range []Source{Observed, AddressBook} {
		found := false
		for _, s := range AllSources {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("source %q is missing from AllSources", want)
		}
	}
}
