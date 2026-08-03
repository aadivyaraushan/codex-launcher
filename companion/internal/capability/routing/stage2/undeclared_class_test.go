package stage2

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
)

// The contact graph is the reason this router has two stages at all: stage 1
// runs in the cloud and is never allowed to turn a name into a handle, so
// stage 2 must do it here or nobody does. Whether a class needs that turn is
// declared on the class, in AddressedToPerson.
//
// That declaration is a plain bool, so a class built without mentioning it
// arrives here saying "not addressed to a person" — and nobody wrote that
// down. It is what the language filled in. Production builds every one of its
// classes that way (runtime/production.go, in the loop over byClass), so the
// contact-graph branch in Resolve is skipped for every request the product
// actually serves, including "message Maya".
//
// What happens instead is worse than asking. The request falls through to the
// plain one-adapter path, finds a single messaging adapter, and resolves to
// it carrying an empty handle — a decision to send a message to nobody in
// particular. Nothing downstream checks the handle before executing, so there
// is no second chance to catch it.
//
// These tests pin the rule that closes it: a class that never declared which
// kind it is must not be treated as the safe kind. Silence is not a
// declaration, and the direction the zero value guesses is the harmful one.
func undeclaredResolver(t *testing.T) *Resolver {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(adapterFor("sms", manifest.Send, manifest.Read)); err != nil {
		t.Fatalf("Register(sms): %v", err)
	}
	// Written exactly the way production writes it: the adapters are listed
	// and nothing at all is said about whether this class names a person.
	classes := ClassMap{"messaging": {Adapters: []string{"sms"}}}
	return New(reg, contacts.NewGraph(func() time.Time { return now }), classes, manifest.PlatformAndroid)
}

func TestAClassThatNeverDeclaredItselfIsNotTreatedAsSafe(t *testing.T) {
	r := undeclaredResolver(t)

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !got.MustAsk {
		t.Fatalf("acted on a class nobody declared, instead of asking: %+v", got)
	}
}

// Asking is only half of it. A decision that says "ask" while still carrying
// somewhere to send would let a caller that reads the fields before the flag
// send the message anyway, which is the whole failure this is here to stop.
func TestAnUndeclaredClassHandsBackNowhereToSend(t *testing.T) {
	r := undeclaredResolver(t)

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.AdapterID != "" || got.Handle != "" {
		t.Fatalf(
			"asked, but still handed back a destination: adapter=%q handle=%q",
			got.AdapterID, got.Handle,
		)
	}
}

// The refusal has to say which class was never declared. "No available app
// can handle this right now" sends whoever is debugging it to look at the
// registry, the platform filter and the verb list — none of which are wrong.
func TestTheRefusalNamesTheClassThatWasNeverDeclared(t *testing.T) {
	r := undeclaredResolver(t)

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.Question == "" {
		t.Fatal("refused with no question at all")
	}
	if !strings.Contains(got.Question, "messaging") {
		t.Fatalf("refusal does not name the class it is about: %q", got.Question)
	}
}

// The control. Without it, "refuse everything" would pass the three tests
// above and break the product. This class says what it is, so it must keep
// resolving exactly as it always has.
func TestADeclaredClassIsUnaffected(t *testing.T) {
	r, _, _ := testResolver(t, contacts.Entry{
		Person: "Maya K", AdapterID: "whatsapp", Handle: "+15550000001",
		LastSeen: daysAgo(2), Source: contacts.Observed,
	})

	got, err := r.Resolve(context.Background(), route(manifest.Send, "messaging", "Maya", 0.95))
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.MustAsk || got.AdapterID != "whatsapp" || got.Handle != "+15550000001" {
		t.Fatalf("a properly declared class stopped resolving: %+v", got)
	}
}
