package stage2

import (
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
)

// The same rule as questions_stand_alone_test.go, applied to the other two
// places Resolve asks the user to choose.
//
// "More than one app could handle this — which one did you mean?" is written
// twice, once for a class resolved on the device and once for a class
// addressed to a thing. Both know exactly which apps they mean: `survivors`
// is in hand on the line above. Neither says. Since only the question string
// reaches the user, a choice between apps that names no app cannot be
// answered — the same defect as the to-a-person branch, in two more spots.

func TestChoosingBetweenTwoNoteAppsNamesThem(t *testing.T) {
	// "notes" is addressed to a thing and has two adapters that both take a
	// write, so nothing can narrow it and the resolver has to ask.
	r, _, _ := testResolver(t)

	dec, err := r.Resolve(t.Context(), route(manifest.Write, "notes", "the roof", 0.9))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !dec.MustAsk {
		t.Fatalf("expected an ask between two note apps, got adapter %q", dec.AdapterID)
	}

	if ok, why := answerable(dec.Question, []string{"notion", "apple-notes"}); !ok {
		t.Errorf("the user is asked %q and %s.\n"+
			"Both surviving adapters are in `survivors` on the line above the "+
			"question. Only the question string reaches the user, so the apps have "+
			"to be named in it.", dec.Question, why)
	}
}

func TestChoosingBetweenTwoOnDeviceAppsNamesThem(t *testing.T) {
	// A class resolved on the device takes the same "one survivor or ask"
	// path, and asks with the same sentence.
	reg := registry.New()
	for _, a := range []*stub{
		adapterFor("notificationreply", manifest.Send),
		adapterFor("sms", manifest.Send),
	} {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register(%s): %v", a.m.ID, err)
		}
	}
	graph := contacts.NewGraph(func() time.Time { return now })
	classes := ClassMap{
		"notification_reply": {
			Adapters:   []string{"notificationreply", "sms"},
			Addressing: ResolvedOnTheDevice,
		},
	}
	r := New(reg, graph, classes, manifest.PlatformAndroid)

	dec, err := r.Resolve(t.Context(), route(manifest.Send, "notification_reply", "Maya", 0.9))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !dec.MustAsk {
		t.Fatalf("expected an ask between two on-device adapters, got adapter %q", dec.AdapterID)
	}

	if ok, why := answerable(dec.Question, []string{"notificationreply", "sms"}); !ok {
		t.Errorf("the user is asked %q and %s.\n"+
			"This branch has `survivors` in hand too, and drops it for a sentence "+
			"that names nothing.", dec.Question, why)
	}
}
