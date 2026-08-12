package runtime

import (
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// The unattended `serve` build must never register an adapter that needs a
// person to click through a browser consent screen. oauthReason
// (production.go:81-97) says exactly why, and says what happens if one slips
// through: "Registering one of these adapters anyway would let the router
// choose it, let the user confirm a preview, and only then discover — after
// they had already said yes — that there was never a credential behind it."
//
// That is the authorization half of the plan's authorization gate: a verb
// with no user grant behind it must not be reachable, and the person must not
// find out only after confirming.
//
// The way the rule is currently kept is a hand-written list of seven adapter
// ids (production.go:284-294). Nothing ties that list to the thing it is
// supposed to track — each adapter's own declared Auth — so it is correct
// only as long as everyone who adds an OAuth adapter remembers to edit a
// list in a different package. That is the whole failure mode this repo keeps
// hitting: a rule that lives in someone's memory rather than in a check.
//
// These tests tie the list to the manifests. They walk every adapter the repo
// actually builds, ask each one what it declared, and hold the production
// inventory against the answer. Adding an eighth OAuth adapter and forgetting
// the list now fails here instead of failing in front of a user.

// fullyConfigured is a production build with every credential the unattended
// serve command can supply. Using the fully-configured build rather than the
// empty one matters: a missing key is its own reason to skip an adapter, and
// would mask an adapter that should have been skipped for needing a sign-in.
func fullyConfigured(t *testing.T) Inventory {
	t.Helper()
	inv, err := NewProduction(ProductionConfig{
		Logger:          quietLogger(),
		MapsAPIKey:      "test-maps-key",
		YouTubeAPIKey:   "test-youtube-key",
		PodcastsFeedURL: "https://feed.invalid/production.xml",
	})
	if err != nil {
		t.Fatalf("NewProduction failed: %v", err)
	}
	return inv
}

// The invariant itself. Nothing the router can reach may need a sign-in that
// unattended serve has no way to perform.
func TestNothingRegisteredNeedsAnInteractiveSignIn(t *testing.T) {
	inv := fullyConfigured(t)

	for _, id := range inv.Registered {
		a, err := inv.reg.Get(id)
		if err != nil {
			t.Fatalf("%s is listed as registered but is not in the registry: %v", id, err)
		}
		if a.Describe().Auth == manifest.AuthOAuth {
			t.Errorf("%s needs a browser sign-in and was registered anyway; the router can choose it and the user would only find out after confirming", id)
		}
	}
}

// The other half, and the one that actually catches the hand-written list
// falling behind. Every adapter in the repo that declares it needs a sign-in
// must appear in the inventory's Skipped map with a plain-English reason —
// Inventory's own doc comment calls itself "the honest record of what
// NewProduction actually built: what went in, what was left out and why".
// An OAuth adapter that is in neither list is not skipped honestly, it is
// simply forgotten, and nobody reading the inventory can tell the difference.
func TestEveryAdapterThatNeedsASignInIsAccountedFor(t *testing.T) {
	inv := fullyConfigured(t)

	registered := map[string]bool{}
	for _, id := range inv.Registered {
		registered[id] = true
	}

	var needsSignIn int
	for _, built := range everyAdapter(t, nil) {
		m := built.a.Describe()
		if m.Auth != manifest.AuthOAuth {
			continue
		}
		needsSignIn++
		reason := inv.Skipped[m.ID]
		if reason == "" {
			t.Errorf("%s needs a browser sign-in but appears in neither Registered nor Skipped; the inventory does not account for it at all", m.ID)
			continue
		}
		// Some ids carry two adapters: a full-control one that needs a
		// sign-in, and a hand-off one that only opens the app. Spotify is
		// the live case. The registry is keyed by id so only one can be in
		// it at a time, and the hand-off is what ships until the sign-in
		// exists — this is the demotion the plan's authorization gate
		// describes. It is fine, but the inventory must not report it as a
		// flat contradiction: reading "spotify: registered" next to
		// "spotify: skipped, needs OAuth" tells whoever is debugging it
		// nothing true. The reason has to say which one was left out.
		if registered[m.ID] && !strings.Contains(reason, "hand-off") {
			t.Errorf("%s is listed as both registered and skipped, and the skip reason does not explain that the registered one is the hand-off: %q", m.ID, reason)
		}
	}

	// Guards against the test passing because everyAdapter stopped returning
	// anything, or because Auth stopped being declared. If this ever reads
	// zero, the loop above checked nothing and said so silently.
	if needsSignIn == 0 {
		t.Fatal("no adapter in the repo declares AuthOAuth; this test checked nothing")
	}
}

// The demotion itself, which is the other half of the plan's authorization
// gate: "a missing check or a separate acting identity demotes the verb to
// class H hands_off". Spotify is the one place that actually happens today.
// The registered stand-in must be a real hand-off — it opens the app and the
// person finishes the job — and must not claim it completes anything, because
// the user reads that claim as fact.
func TestAnIdThatLostItsSignInFallsBackToARealHandOff(t *testing.T) {
	inv := fullyConfigured(t)

	var checked int
	for _, built := range everyAdapter(t, nil) {
		m := built.a.Describe()
		if m.Auth != manifest.AuthOAuth || inv.Skipped[m.ID] == "" {
			continue
		}
		stand, err := inv.reg.Get(m.ID)
		if err != nil {
			continue // nothing registered under this id; nothing to demote to
		}
		checked++
		if got := stand.Describe().Ceiling; got != manifest.HandsOff {
			t.Errorf("%s lost its sign-in and the adapter standing in for it claims ceiling %q; a route with no credential behind it must not claim more than a hand-off", m.ID, got)
		}
	}
	if checked == 0 {
		t.Skip("no id currently carries both a sign-in adapter and a stand-in")
	}
}

// The wider version of the same rule, and the one that catches the defect
// this repo keeps producing: an adapter that is finished, tested, and
// reachable by nobody, with nothing anywhere saying so.
//
// There are exactly three honest states for an adapter. It is registered, so
// a user can reach it. Or the inventory says why this build left it out —
// a missing API key, a sign-in nobody can perform unattended. Or its own
// manifest carries an Unshipped reason, which is the house pattern already
// used by maps_saved_places (maps/adapter.go:221-224) and already has teeth:
// TestNothingMarkedUnshippedIsActuallyShipped refuses to let an adapter claim
// it is unshipped while production registers it.
//
// A fourth state — in none of the three — is the defect. Nobody reading the
// code can tell whether that adapter was left out on purpose or forgotten,
// and its passing unit tests read exactly like a shipped adapter's.
func TestEveryAdapterIsRegisteredExplainedOrDeclaredUnshipped(t *testing.T) {
	inv := fullyConfigured(t)

	registered := map[string]bool{}
	for _, id := range inv.Registered {
		registered[id] = true
	}

	for _, built := range everyAdapter(t, nil) {
		m := built.a.Describe()
		switch {
		case registered[m.ID]:
		case inv.Skipped[m.ID] != "":
		case strings.TrimSpace(m.Unshipped) != "":
		default:
			t.Errorf("%s is in none of the three honest states: not registered, not explained in the inventory, and its manifest does not declare it unshipped. Nobody can tell whether it was left out on purpose", m.ID)
		}
	}
}

// The control. Refusing everything that needs a credential would pass both
// tests above and leave the product with nothing but hand-off adapters. The
// key-only adapters — an API key is not a sign-in, nobody has to be watching
// a browser — must still come up.
func TestKeyOnlyAdaptersAreStillRegistered(t *testing.T) {
	inv := fullyConfigured(t)

	for _, id := range []string{"maps", "youtube", "podcasts"} {
		var found bool
		for _, got := range inv.Registered {
			if got == id {
				found = true
			}
		}
		if !found {
			t.Errorf("%s needs only an API key and was given one, but is not registered: %v", id, inv.Registered)
		}
	}
}
