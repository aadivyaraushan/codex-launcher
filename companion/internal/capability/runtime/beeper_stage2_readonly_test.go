package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// Fact-force (new file):
// 1. Callers: `go test` in this package only — B4/B5 pin tests; no production
//    importer. Exercised by Slice 4 of openai-beeper-phone-runtime.
// 2. Search: no existing BeeperReadOnly / TestBeeperRead* / *beeper*stage2*
//    test file under companion/internal/capability/runtime/ (rg 2026-08-06).
// 3. No data files read/written.
// 4. User: "Continue OpenAI+Beeper — **SLICE 4: B4 + B5**."

func productionBeeperResolver(t *testing.T, inv Inventory) *stage2.Resolver {
	t.Helper()
	if inv.reg == nil {
		t.Fatal("inventory has no registry")
	}
	return stage2.New(inv.reg, contacts.NewGraph(time.Now), classMapFor(inv.Classes), manifest.PlatformAndroid)
}

// B4: named read must land on that one Beeper network adapter.
func TestBeeperReadWithAppNamedInstagramResolvesToThatAdapter(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{
		Model: stubModel, Logger: quietLogger(), BeeperAPI: &fakeProductionBeeper{},
	})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	r := productionBeeperResolver(t, inv)

	got, err := r.Resolve(context.Background(), stage1.Route{
		Verb: manifest.Read, AppClass: "beeper_messaging", AppNamed: "instagram",
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.MustAsk {
		t.Fatalf("named Instagram read asked instead of resolving: %+v", got)
	}
	if got.AdapterID != "instagram" {
		t.Fatalf("AdapterID=%q, want instagram", got.AdapterID)
	}
}

// B4: unnamed read leaves three Beeper networks — ask which, do not error.
func TestBeeperReadWithoutAppNamedAsksWhichNetwork(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{
		Model: stubModel, Logger: quietLogger(), BeeperAPI: &fakeProductionBeeper{},
	})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	r := productionBeeperResolver(t, inv)

	got, err := r.Resolve(context.Background(), stage1.Route{
		Verb: manifest.Read, AppClass: "beeper_messaging", AppNamed: "",
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("Resolve returned error %v; want a which-network question", err)
	}
	if !got.MustAsk {
		t.Fatalf("unnamed read resolved to %q instead of asking which network", got.AdapterID)
	}
	q := strings.ToLower(got.Question)
	if !strings.Contains(q, "which") {
		t.Fatalf("question missing which-network ask: %q", got.Question)
	}
	for _, id := range []string{"instagram", "discord", "messages"} {
		if !strings.Contains(got.Question, id) {
			t.Fatalf("question %q does not name %q", got.Question, id)
		}
	}
}

// B4: unsupported network name asks; does not substitute a neighbour.
func TestBeeperReadWithUnknownAppNamedAsksForConnectedApp(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{
		Model: stubModel, Logger: quietLogger(), BeeperAPI: &fakeProductionBeeper{},
	})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	r := productionBeeperResolver(t, inv)

	got, err := r.Resolve(context.Background(), stage1.Route{
		Verb: manifest.Read, AppClass: "beeper_messaging", AppNamed: "telegram",
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !got.MustAsk {
		t.Fatalf("unknown network resolved to %q", got.AdapterID)
	}
	if !strings.Contains(strings.ToLower(got.Question), "don't have the app you named") {
		t.Fatalf("want named-app question, got %q", got.Question)
	}
}

// B5: read-only mode still registers Beeper networks, but only verb read.
func TestBeeperReadOnlyModeRegistersAdaptersWithReadVerbOnly(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{
		Model: stubModel, Logger: quietLogger(),
		BeeperAPI: &fakeProductionBeeper{}, BeeperReadOnly: true,
	})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	want := map[string]bool{"instagram": false, "discord": false, "messages": false}
	for _, id := range inv.Classes["beeper_messaging"] {
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("read-only Beeper missing %q under beeper_messaging: %v", id, inv.Classes)
		}
	}

	for id := range want {
		a, err := inv.reg.Get(id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		m := a.Describe()
		if len(m.Verbs) != 1 || m.Verbs[0] != manifest.Read {
			t.Fatalf("%s verbs=%v, want [read] only", id, m.Verbs)
		}
	}
}

// B5: read still resolves under read-only; send asks because no write verbs survive.
func TestBeeperReadOnlyModeAllowsReadButNotSendAtStage2(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{
		Model: stubModel, Logger: quietLogger(),
		BeeperAPI: &fakeProductionBeeper{}, BeeperReadOnly: true,
	})
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	r := productionBeeperResolver(t, inv)

	read, err := r.Resolve(context.Background(), stage1.Route{
		Verb: manifest.Read, AppClass: "beeper_messaging", AppNamed: "instagram",
		Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("read Resolve: %v", err)
	}
	if read.MustAsk || read.AdapterID != "instagram" {
		t.Fatalf("read-only read failed: %+v", read)
	}

	send, err := r.Resolve(context.Background(), stage1.Route{
		Verb: manifest.Send, AppClass: "beeper_messaging", AppNamed: "instagram",
		Subject: "Maya", Body: "hi", Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("send Resolve: %v", err)
	}
	if !send.MustAsk {
		t.Fatalf("read-only send resolved to %q; write verbs must not survive", send.AdapterID)
	}
}
