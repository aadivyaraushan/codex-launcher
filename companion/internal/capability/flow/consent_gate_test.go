package flow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// The consent store is a finished, fully tested 331-line gate that nothing
// has ever called. `Allow` is documented as "the consent gate every request
// runs through before it is allowed to execute" and no request ran through
// it. These tests wire it to the one place every request actually passes.
//
// Worth being straight about what this changes today: every adapter in the
// repo is consent class A, and class A passes with nothing granted, so no
// current behaviour moves. The value is entirely in the day someone adds the
// first class B adapter — the one that needs a per-app screen because it is
// not the official route. Without the gate wired, that adapter ships with
// its consent screen decorative, and nobody finds out from a test.
//
// So these tests use a class B adapter of their own rather than a real one.
// That is the point: the rule has to hold before the adapter exists, or it
// will not hold when it does.

// gateAdapter is a class B adapter that records whether it was ever touched.
// Whether it was touched is the assertion that matters — a gate that refuses
// after the adapter has already run is not a gate.
type gateAdapter struct {
	touched []string
}

func (g *gateAdapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID:            "gated",
		Runtime:       manifest.RT2,
		Verbs:         []manifest.Verb{manifest.Write},
		Ceiling:       manifest.Completes,
		Consent:       manifest.ConsentB,
		Auth:          manifest.AuthNone,
		Cost:          manifest.CostFree,
		Capacity:      manifest.Capacity{Kind: manifest.CapacityNone},
		Platform:      manifest.PlatformBoth,
		Gates:         []manifest.Gate{manifest.GateNone},
		ProvesCeiling: "gated_smoke",
	}
}

func (g *gateAdapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	g.touched = append(g.touched, "resolve")
	return adapter.Plan{AdapterID: "gated", Verb: in.Verb, Summary: in.Subject}, nil
}

func (g *gateAdapter) Preview(_ context.Context, p adapter.Plan) (adapter.Preview, error) {
	g.touched = append(g.touched, "preview")
	return adapter.Preview{Plan: p, Headline: p.Summary, Confirm: "Do it"}, nil
}

func (g *gateAdapter) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	g.touched = append(g.touched, "execute")
	return adapter.Outcome{Reached: manifest.Completes, Done: true}, nil
}

func (g *gateAdapter) Revoke(context.Context) error { return nil }

// emptyVault stands in for the two stores consent.Revoke proves itself
// against. Nothing is persisted in these tests, and it says so honestly
// rather than claiming a deletion it never made.
type emptyVault struct{}

func (emptyVault) Delete(context.Context, string) error       { return nil }
func (emptyVault) Has(context.Context, string) (bool, error)  { return false, nil }

func gatedService(t *testing.T) (*Service, *gateAdapter, *consent.Store) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := &gateAdapter{}
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	resolver := stage2.New(reg, contacts.NewGraph(nil), stage2.ClassMap{
		"tasks": {Adapters: []string{"gated"}, Addressing: stage2.ToAThing},
	}, manifest.PlatformAndroid)

	gate := consent.New(
		func() time.Time { return time.Unix(0, 0) },
		map[string]consent.Copy{"gated": {
			Situation: "This app has no official route, so Operator acts as you.",
			Grants:    []string{"Create items on your behalf"},
			Risk:      "Anything it creates looks like you created it.",
			Absence:   consent.AbsenceChoice,
		}},
		emptyVault{}, emptyVault{},
	)

	model := func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"","subject":"a thing","body":"","confidence":0.97}`), nil
	}
	return New(stage1.New(model), resolver, execution.New(reg), gate, logger), a, gate
}

// The gate has to close before the user sees anything. Refusing at confirm
// instead would mean Operator renders a sheet for an app the user never
// agreed to, takes their tap, and only then admits it cannot proceed — the
// same consent theatre as a blank preview.
func TestAnAdapterNeedingConsentIsRefusedBeforeAnythingIsShown(t *testing.T) {
	service, a, _ := gatedService(t)

	_, err := service.Prepare(context.Background(), "pixel-9/session-1/1", "request-1", "add a thing")
	if !errors.Is(err, consent.ErrNotGranted) {
		t.Fatalf("Prepare for an ungranted class B adapter returned %v, want ErrNotGranted", err)
	}
	if len(a.touched) != 0 {
		t.Fatalf("the adapter was used before consent was granted: %v", a.touched)
	}
}

// The other half: granting must actually open the gate, or consent is a
// button that does nothing.
func TestGrantingConsentLetsTheRequestThrough(t *testing.T) {
	service, a, gate := gatedService(t)
	ctx := context.Background()

	screen, err := gate.Screen((&gateAdapter{}).Describe())
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if err := gate.Grant(ctx, (&gateAdapter{}).Describe(), screen); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	preview, err := service.Prepare(ctx, "pixel-9/session-1/1", "request-1", "add a thing")
	if err != nil {
		t.Fatalf("Prepare after granting consent failed: %v", err)
	}
	outcome, err := service.Confirm(ctx, "pixel-9/session-1/1", "request-1", preview.Fingerprint)
	if err != nil {
		t.Fatalf("Confirm after granting consent failed: %v", err)
	}
	if !outcome.Done {
		t.Fatalf("outcome=%+v", outcome)
	}
	if len(a.touched) == 0 {
		t.Fatal("consent was granted and the adapter still was not used")
	}
}

// This is why the gate cannot live only at Prepare. A preview sits pending
// while the user goes to Settings and disconnects the app. If Confirm does
// not re-check, their revoke is undone by a sheet that was already on
// screen, and the action they just withdrew permission for runs anyway.
func TestConsentWithdrawnWhileAPreviewWasPendingStopsTheConfirm(t *testing.T) {
	service, a, gate := gatedService(t)
	ctx := context.Background()

	screen, err := gate.Screen((&gateAdapter{}).Describe())
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if err := gate.Grant(ctx, (&gateAdapter{}).Describe(), screen); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	preview, err := service.Prepare(ctx, "pixel-9/session-1/1", "request-1", "add a thing")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// The user disconnects the app while the sheet is up.
	if _, err := gate.Revoke(ctx, "gated"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	before := len(a.touched)
	if _, err := service.Confirm(ctx, "pixel-9/session-1/1", "request-1", preview.Fingerprint); !errors.Is(err, consent.ErrNotGranted) {
		t.Fatalf("Confirm after the grant was withdrawn returned %v, want ErrNotGranted", err)
	}
	for _, step := range a.touched[before:] {
		if step == "execute" {
			t.Fatal("the action ran after the user had withdrawn permission for it")
		}
	}
}

// Class A is the official route and needs no screen, so wiring the gate must
// not quietly turn off every adapter that ships today. All sixteen adapters
// in the repo are class A; if this breaks, the gate broke the product.
func TestAClassAAdapterStillNeedsNoGrant(t *testing.T) {
	service, _ := todoistService(t, func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"Todoist","subject":"Buy oat milk","body":"","confidence":0.98}`), nil
	})
	if _, err := service.Prepare(context.Background(), "pixel-9/session-1/1", "request-1", "add buy oat milk"); err != nil {
		t.Fatalf("a class A adapter was refused with no grant: %v", err)
	}
}
