package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	mapsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// fakeMapsAPI stands in for the live Places API (New) + Routes API client so
// this test proves the wiring, not the Google Maps Platform contract (that
// is companion/internal/capability/adapters/maps/client_test.go's job).
type fakeMapsAPI struct {
	place mapsadapter.Place
	calls int
}

func (f *fakeMapsAPI) SearchPlace(context.Context, string) (mapsadapter.Place, error) {
	f.calls++
	return f.place, nil
}

func (f *fakeMapsAPI) ComputeRoute(context.Context, string, string) (mapsadapter.Route, error) {
	return mapsadapter.Route{}, nil
}

// The maps flow declares a billing gate (Cost: CostPerCall, Gates:
// [GateBilling]) and nothing in this product can clear one today — no
// approval flow, no billing consent. Prepare must fail before the adapter is
// ever asked to resolve, because resolving is where the paid Google Places
// call happens. This is deliberate, not a bug: the alternative was billing
// the owner's cloud account on every request with no checkpoint at all.
func TestMapsFlowSurfacesAPlacesAnswerInThePreview(t *testing.T) {
	api := &fakeMapsAPI{place: mapsadapter.Place{
		ID: "place-1", Name: "Blue Bottle Coffee", Address: "480 9th St, Oakland, CA 94607, USA",
	}}
	service, err := NewMaps(MapsConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"maps","subject":"Blue Bottle Coffee","body":"","confidence":0.95}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewMaps: %v", err)
	}

	_, err = service.Prepare(context.Background(), "pixel/session/1", "request-1", "Find Blue Bottle Coffee on Google Maps")
	if !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("Prepare = %v, want ErrGateNotCleared", err)
	}
	// The point of the whole change: the paid API call must never have
	// happened.
	if api.calls != 0 {
		t.Fatalf("api calls=%d, want 0 — the billing gate must stop resolve before it calls the API", api.calls)
	}
}

func TestMapsFlowRejectsMissingRuntimeDependencies(t *testing.T) {
	if _, err := NewMaps(MapsConfig{}); err == nil {
		t.Fatal("empty runtime config was accepted")
	}
}

var _ capabilityadapter.Adapter = (*mapsadapter.Adapter)(nil)
