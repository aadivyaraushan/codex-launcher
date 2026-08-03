package maps

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

type fakeAPI struct {
	place     Place
	route     Route
	placeErr  error
	routeErr  error
	placeCall string
}

func (f *fakeAPI) SearchPlace(_ context.Context, query string) (Place, error) {
	f.placeCall = query
	return f.place, f.placeErr
}

func (f *fakeAPI) ComputeRoute(_ context.Context, _, _ string) (Route, error) {
	return f.route, f.routeErr
}

func newLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPlacesManifestIsTheAndroidRT2CompletesRoute(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Ceiling != manifest.Completes {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) {
		t.Fatalf("maps must offer read: %v", m.Verbs)
	}
}

// Requirement: places search returns a named place with an address. maps
// declares a billing gate (Cost: CostPerCall, Gates: [GateBilling]), so the
// runner now refuses it at every door — see
// TestRunnerRefusesMapsBecauseItDeclaresABillingGate below. This test drives
// the adapter directly, the way TestDirectionsReturnsARouteSummary does, to
// keep coverage on the parsing this requirement is actually about.
func TestPlaceSearchReturnsANamedPlaceWithAnAddress(t *testing.T) {
	api := &fakeAPI{place: Place{ID: "place1", Name: "Blue Bottle Coffee", Address: "300 Webster St, Oakland, CA"}}
	a := New(api, newLogger())
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "blue bottle coffee"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["place_name"] != "Blue Bottle Coffee" || plan.Details["address"] == "" {
		t.Fatalf("plan = %+v, want a named place with an address", plan.Details)
	}

	preview, err := a.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "300 Webster St") {
		t.Fatalf("preview did not show the address: %q", shown)
	}

	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done {
		t.Fatalf("outcome = %+v", out)
	}
}

// Requirement: maps declares a billing gate, so the runner refuses it at
// every door rather than letting a paid Google API call happen with no
// checkpoint.
func TestRunnerRefusesMapsBecauseItDeclaresABillingGate(t *testing.T) {
	api := &fakeAPI{place: Place{ID: "place1", Name: "Blue Bottle Coffee", Address: "300 Webster St, Oakland, CA"}}
	a := New(api, newLogger())
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	_, err := run.Resolve(ctx, adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "blue bottle coffee"})
	if !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("resolve = %v, want ErrGateNotCleared", err)
	}
}

// Requirement: directions returns a route summary.
func TestDirectionsReturnsARouteSummary(t *testing.T) {
	api := &fakeAPI{route: Route{Summary: "I-80 W", Distance: "3.1 km", Duration: "9 mins"}}
	a := New(api, newLogger())
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Read,
		Fields: map[string]string{"origin": "Home", "destination": "Work"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["duration"] != "9 mins" || plan.Details["distance"] != "3.1 km" {
		t.Fatalf("plan = %+v, want a route summary", plan.Details)
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.Completes || !out.Done || !strings.Contains(out.Detail, "9 mins") {
		t.Fatalf("outcome = %+v", out)
	}
}

// Requirement: navigation intent opens Maps with the route loaded — still
// completes per the plan's entertainment/navigation open-via-intent rule.
func TestNavigationIntentHandsOffToMapsWithTheRouteLoaded(t *testing.T) {
	api := &fakeAPI{route: Route{Summary: "I-80 W", Distance: "3.1 km", Duration: "9 mins"}}
	a := New(api, newLogger())
	ctx := context.Background()

	plan, err := a.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Read,
		Fields: map[string]string{"origin": "Home", "destination": "Work", "navigate": "true"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.HasPrefix(plan.Details["maps_uri"], "google.navigation:q=") {
		t.Fatalf("maps_uri = %q", plan.Details["maps_uri"])
	}
	out, err := a.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Naming an app it handed control to makes this a hand-off, whatever the
	// manifest says. The phone throws away any result that names an app at a
	// ceiling other than hands_off (ProtocolCodec.kt:187), so a "completes"
	// here would have shown the user nothing at all.
	if out.Reached != manifest.HandsOff || !out.Done || out.HandedOffTo != "Google Maps" {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestNoPlaceFoundFailsClosed(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "nowhere at all"})
	if !errors.Is(err, ErrNoPlaceFound) {
		t.Fatalf("no place returned %v, want ErrNoPlaceFound", err)
	}
}

func TestDirectionsWithoutBothEndpointsIsRejected(t *testing.T) {
	a := New(&fakeAPI{}, newLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Fields: map[string]string{"destination": "Work"},
	})
	if !errors.Is(err, ErrMissingRouteEndpoints) {
		t.Fatalf("missing origin returned %v, want ErrMissingRouteEndpoints", err)
	}
}

// ---- saved places (RT-4 hand-off) ----

func TestSavedPlacesManifestIsTheHandsOffRT4Route(t *testing.T) {
	a := NewSavedPlaces(newLogger())
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != SavedPlacesID || m.Runtime != manifest.RT4 || m.Ceiling != manifest.HandsOff {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Write) {
		t.Fatalf("saved places must offer write: %v", m.Verbs)
	}
}

// Requirement: the saved-places path produces a hands_off outcome whose
// user-facing text never says "saved" anywhere — Operator prepares, Maps
// opens, the user finishes the save; no "saved" claim.
func TestSavedPlacesNeverClaimsSavedAnywhereInTheCopy(t *testing.T) {
	a := NewSavedPlaces(newLogger())
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{AdapterID: SavedPlacesID, Verb: manifest.Write, Subject: "Blue Bottle Coffee"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.HandsOff || !out.Done || out.HandedOffTo != "Google Maps" {
		t.Fatalf("outcome = %+v", out)
	}
	everything := strings.ToLower(preview.Headline + " " + strings.Join(preview.Lines, " ") + " " + out.Detail)
	if strings.Contains(everything, "saved") {
		t.Fatalf("copy claims saved: %q", everything)
	}
}

func TestSavedPlacesRejectsAnEmptyPlaceName(t *testing.T) {
	a := NewSavedPlaces(newLogger())
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: SavedPlacesID, Verb: manifest.Write, Subject: "  "})
	if !errors.Is(err, ErrEmptyPlaceName) {
		t.Fatalf("blank place name returned %v, want ErrEmptyPlaceName", err)
	}
}
