package consent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

var now = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

// A token store and a local-state store that can be inspected afterwards,
// because "revoke must be proven" means proven against something, not
// asserted by the thing doing the revoking.
type fakeStore struct {
	held    map[string]bool
	refuse  map[string]bool // ids whose delete silently fails, as a real one might
	deletes []string
}

func newFakeStore(ids ...string) *fakeStore {
	s := &fakeStore{held: map[string]bool{}, refuse: map[string]bool{}}
	for _, id := range ids {
		s.held[id] = true
	}
	return s
}

func (s *fakeStore) Delete(_ context.Context, id string) error {
	s.deletes = append(s.deletes, id)
	if s.refuse[id] {
		return nil // reports success, leaves the data behind
	}
	delete(s.held, id)
	return nil
}

func (s *fakeStore) Has(_ context.Context, id string) (bool, error) { return s.held[id], nil }

func manifestFor(id string, class manifest.Consent) manifest.Manifest {
	return manifest.Manifest{
		ID: id, Runtime: manifest.RT6, Verbs: []manifest.Verb{manifest.Send},
		Ceiling: manifest.Completes, Consent: class,
		Auth: manifest.AuthLocal, Cost: manifest.CostFree,
		Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region: []string{"global"}, Platform: manifest.PlatformBoth, ProvesCeiling: id + "-smoke",
	}
}

// The copy that ships. Each line names that specific app's situation, which
// is the whole point of a per-app screen.
func testCopy() map[string]Copy {
	return map[string]Copy{
		"whatsapp": {
			Situation: "Meta banned third-party AI assistants from WhatsApp on 15 January 2026. " +
				"This is against Meta's wishes today.",
			Grants: []string{"send and read messages as your linked device"},
			Risk:   "Your account could be actioned.",
		},
		"instagram": {
			Situation: "Meta's terms prohibit automated access. This runs a browser logged in as you.",
			Grants:    []string{"read and send direct messages as you"},
			Risk:      "Your account could be actioned.",
		},
		"strava": {
			Absence: AbsenceChoice,
			Situation: "Strava's policy names context-window ingestion, and we hold a developer " +
				"credential that accepted it. We will not break it.",
		},
		"venmo": {
			Absence:   AbsenceNoDoor,
			Situation: "Venmo has no API for sending money and no route in. Operator opens the app instead.",
		},
	}
}

func testStore(t *testing.T, tokens, local *fakeStore) *Store {
	t.Helper()
	return New(func() time.Time { return now }, testCopy(), tokens, local)
}

// ---- who needs a screen at all ------------------------------------------

func TestOnlyClassBNeedsAScreen(t *testing.T) {
	if Requires(manifest.ConsentA) {
		t.Error("class A asked for a consent screen; official routes use the normal connect flow")
	}
	if !Requires(manifest.ConsentB) {
		t.Error("class B did not ask for a screen; account risk always gets one")
	}
	for _, c := range []manifest.Consent{manifest.ConsentC1, manifest.ConsentC2, manifest.ConsentC3} {
		if Requires(c) {
			t.Errorf("class %s asked for a screen; no screen can cure a class C", c)
		}
	}
}

func TestAClassAAdapterIsAllowedWithoutAnythingBeingGranted(t *testing.T) {
	s := testStore(t, newFakeStore(), newFakeStore())
	if err := s.Allow(context.Background(), manifestFor("notion", manifest.ConsentA)); err != nil {
		t.Fatalf("an official adapter was blocked: %v", err)
	}
}

// ---- class C is never shipped, and no consent unlocks it ----------------

func TestAClassCAdapterHasNoScreenAtAll(t *testing.T) {
	s := testStore(t, newFakeStore(), newFakeStore())
	if _, err := s.Screen(manifestFor("strava", manifest.ConsentC1)); !errors.Is(err, ErrNeverShipped) {
		t.Fatalf("class C produced a screen: %v", err)
	}
}

func TestAClassCAdapterStaysBlockedEvenAfterAGrantIsAttempted(t *testing.T) {
	// The user cannot consent their way out of a contract we signed.
	s := testStore(t, newFakeStore(), newFakeStore())
	m := manifestFor("strava", manifest.ConsentC1)

	if err := s.Grant(context.Background(), m, Screen{AdapterID: "strava"}); !errors.Is(err, ErrNeverShipped) {
		t.Fatalf("class C accepted a grant: %v", err)
	}
	if err := s.Allow(context.Background(), m); !errors.Is(err, ErrNeverShipped) {
		t.Fatalf("class C was allowed to run: %v", err)
	}
}

func TestRefusingIsAnsweredWithAReasonRatherThanSilence(t *testing.T) {
	// A user who asks twice deserves a reason.
	s := testStore(t, newFakeStore(), newFakeStore())

	kind, why, err := s.WhyNot("strava")
	if err != nil {
		t.Fatalf("WhyNot failed: %v", err)
	}
	if why == "" {
		t.Error("a refused adapter has no explanation")
	}
	if kind != AbsenceChoice {
		t.Errorf("kind = %q, want %q", kind, AbsenceChoice)
	}
}

func TestChoosingNotToAndHavingNoWayInNeverReadTheSame(t *testing.T) {
	// Only one of the two might change, so the copy must not blur them.
	s := testStore(t, newFakeStore(), newFakeStore())

	choiceKind, choice, _ := s.WhyNot("strava")
	doorKind, door, _ := s.WhyNot("venmo")

	if choiceKind == doorKind {
		t.Fatalf("both answers are kind %q", choiceKind)
	}
	if strings.EqualFold(choice, door) {
		t.Error("\"we chose not to\" and \"there is no way in\" ship the same sentence")
	}
	if doorKind != AbsenceNoDoor {
		t.Errorf("venmo's kind = %q, want %q", doorKind, AbsenceNoDoor)
	}
}

// ---- the class B screen --------------------------------------------------

func TestAClassBAdapterIsBlockedUntilItIsGranted(t *testing.T) {
	s := testStore(t, newFakeStore(), newFakeStore())
	if err := s.Allow(context.Background(), manifestFor("whatsapp", manifest.ConsentB)); !errors.Is(err, ErrNotGranted) {
		t.Fatalf("an ungranted class B adapter ran: %v", err)
	}
}

func TestAClassBAdapterWithNoCopyOfItsOwnCannotShipAGenericWarning(t *testing.T) {
	// A generic warning is worse than none: it teaches the user the screen
	// says nothing, so the next real one gets tapped through too.
	s := testStore(t, newFakeStore(), newFakeStore())
	if _, err := s.Screen(manifestFor("messenger", manifest.ConsentB)); !errors.Is(err, ErrNoCopy) {
		t.Fatalf("an adapter with no copy still produced a screen: %v", err)
	}
}

func TestTheScreenNamesTheSituationTheAccessAndTheDeny(t *testing.T) {
	s := testStore(t, newFakeStore(), newFakeStore())

	sc, err := s.Screen(manifestFor("whatsapp", manifest.ConsentB))
	if err != nil {
		t.Fatalf("Screen failed: %v", err)
	}
	if !strings.Contains(sc.Situation, "Meta") {
		t.Errorf("the screen does not name WhatsApp's actual situation: %q", sc.Situation)
	}
	if len(sc.Grants) == 0 {
		t.Error("the screen does not say what access is granted")
	}
	if sc.Risk == "" {
		t.Error("the screen does not say what could go wrong")
	}
	if sc.Deny == "" {
		t.Error("the screen has no Deny; DESIGN.md says Deny is never hidden")
	}
	if sc.Runtime != manifest.RT6 {
		t.Errorf("the screen does not name the runtime: %q", sc.Runtime)
	}
}

func TestAScreenShownForOneAppDoesNotGrantAnother(t *testing.T) {
	// Otherwise a user who consented to Instagram has silently consented to
	// WhatsApp, which is exactly the per-app promise being broken.
	s := testStore(t, newFakeStore(), newFakeStore())
	ctx := context.Background()

	shown, err := s.Screen(manifestFor("instagram", manifest.ConsentB))
	if err != nil {
		t.Fatalf("Screen failed: %v", err)
	}
	if err := s.Grant(ctx, manifestFor("whatsapp", manifest.ConsentB), shown); !errors.Is(err, ErrScreenNotShown) {
		t.Fatalf("Instagram's screen granted WhatsApp: %v", err)
	}
	if s.Granted("whatsapp") {
		t.Fatal("WhatsApp reads as granted")
	}
}

func TestGrantingRecordsTheExactWordsTheUserAgreedTo(t *testing.T) {
	// When the copy changes because the world changed, we must still be able
	// to say what this user actually saw.
	s := testStore(t, newFakeStore(), newFakeStore())
	ctx := context.Background()
	m := manifestFor("whatsapp", manifest.ConsentB)

	shown, _ := s.Screen(m)
	if err := s.Grant(ctx, m, shown); err != nil {
		t.Fatalf("Grant failed: %v", err)
	}

	g, ok := s.Record("whatsapp")
	if !ok {
		t.Fatal("no grant was recorded")
	}
	if g.Situation != shown.Situation {
		t.Errorf("the recorded wording is not what was shown:\n got %q\nwant %q", g.Situation, shown.Situation)
	}
	if !g.GrantedAt.Equal(now) {
		t.Errorf("granted at %v, want %v", g.GrantedAt, now)
	}
	if err := s.Allow(ctx, m); err != nil {
		t.Fatalf("a granted class B adapter was still blocked: %v", err)
	}
}

// ---- revoke, and the proof that it happened -----------------------------

func TestRevokeDeletesTheTokensTheLocalStateAndTheGrant(t *testing.T) {
	tokens, local := newFakeStore("whatsapp"), newFakeStore("whatsapp")
	s := testStore(t, tokens, local)
	ctx := context.Background()
	m := manifestFor("whatsapp", manifest.ConsentB)

	shown, _ := s.Screen(m)
	_ = s.Grant(ctx, m, shown)

	proof, err := s.Revoke(ctx, "whatsapp")
	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	if !proof.Complete() {
		t.Fatalf("the revoke proof is incomplete: %+v", proof)
	}
	if held, _ := tokens.Has(ctx, "whatsapp"); held {
		t.Error("the token store still holds a token")
	}
	if held, _ := local.Has(ctx, "whatsapp"); held {
		t.Error("local state survived the revoke")
	}
	if s.Granted("whatsapp") {
		t.Error("the grant survived the revoke")
	}
}

func TestTheProofIsAReReadNotAClaimByTheCodeThatJustRevoked(t *testing.T) {
	// This is the test the plan calls the revoke proof. A delete that reports
	// success and leaves the token behind must fail here, not ship.
	tokens, local := newFakeStore("whatsapp"), newFakeStore("whatsapp")
	tokens.refuse["whatsapp"] = true
	s := testStore(t, tokens, local)
	ctx := context.Background()
	m := manifestFor("whatsapp", manifest.ConsentB)

	shown, _ := s.Screen(m)
	_ = s.Grant(ctx, m, shown)

	proof, err := s.Revoke(ctx, "whatsapp")
	if !errors.Is(err, ErrRevokeIncomplete) {
		t.Fatalf("a revoke that left the token behind reported success: %v", err)
	}
	if proof.TokensGone {
		t.Error("the proof claims the tokens are gone while the store still holds them")
	}
}

func TestARevokedAdapterIsBlockedAgain(t *testing.T) {
	s := testStore(t, newFakeStore("whatsapp"), newFakeStore("whatsapp"))
	ctx := context.Background()
	m := manifestFor("whatsapp", manifest.ConsentB)

	shown, _ := s.Screen(m)
	_ = s.Grant(ctx, m, shown)
	_, _ = s.Revoke(ctx, "whatsapp")

	if err := s.Allow(ctx, m); !errors.Is(err, ErrNotGranted) {
		t.Fatalf("a revoked adapter still ran: %v", err)
	}
}

func TestRevokeAtScaleIsTheSamePathRunForEveryone(t *testing.T) {
	// The breach response needs mass revoke, and the per-user path is most of
	// it. Building it as a loop over the proven path is what keeps it honest.
	tokens := newFakeStore("whatsapp", "instagram")
	local := newFakeStore("whatsapp", "instagram")
	s := testStore(t, tokens, local)
	ctx := context.Background()

	for _, id := range []string{"whatsapp", "instagram"} {
		m := manifestFor(id, manifest.ConsentB)
		shown, _ := s.Screen(m)
		if err := s.Grant(ctx, m, shown); err != nil {
			t.Fatalf("Grant(%s) failed: %v", id, err)
		}
	}

	proofs, err := s.RevokeAll(ctx)
	if err != nil {
		t.Fatalf("RevokeAll failed: %v", err)
	}
	if len(proofs) != 2 {
		t.Fatalf("RevokeAll produced %d proofs, want 2", len(proofs))
	}
	for _, p := range proofs {
		if !p.Complete() {
			t.Errorf("proof for %s is incomplete: %+v", p.AdapterID, p)
		}
	}
	if len(tokens.held) != 0 || len(local.held) != 0 {
		t.Errorf("data survived the mass revoke: tokens=%v local=%v", tokens.held, local.held)
	}
}
