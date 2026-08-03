package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	mapsadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// fakeMapsRouteAPI answers the Routes half of the Maps client. It is separate
// from fakeMapsAPI so the places test keeps its own expectations. calls
// counts every request that reached the fake, regardless of which method —
// the billing gate test below needs to prove neither one ran.
type fakeMapsRouteAPI struct {
	route mapsadapter.Route
	from  string
	to    string
	calls int
}

func (f *fakeMapsRouteAPI) SearchPlace(context.Context, string) (mapsadapter.Place, error) {
	f.calls++
	return mapsadapter.Place{}, nil
}

func (f *fakeMapsRouteAPI) ComputeRoute(_ context.Context, origin, destination string) (mapsadapter.Route, error) {
	f.calls++
	f.from, f.to = origin, destination
	return f.route, nil
}

// The maps flow declares a billing gate (Cost: CostPerCall, Gates:
// [GateBilling]) and nothing in this product can clear one today — no
// approval flow, no billing consent. Prepare must fail before the adapter is
// ever asked to resolve, because resolving directions is where the paid
// Google Routes call happens. This is deliberate, not a bug: the alternative
// was billing the owner's cloud account on every request with no checkpoint
// at all.
func TestMapsFlowComputesDirectionsFromNamedSlots(t *testing.T) {
	api := &fakeMapsRouteAPI{route: mapsadapter.Route{
		Summary: "I-880 N", Distance: "22 mi", Duration: "35 mins",
	}}
	service, err := NewMaps(MapsConfig{
		API: api,
		Model: func(context.Context, string) ([]byte, error) {
			return []byte(`{"verb":"read","app_class":"travel","app_named":"maps",
			  "subject":"directions to the airport","body":"",
			  "fields":{"origin":"Blue Bottle Coffee Oakland","destination":"SFO"},
			  "confidence":0.95}`), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewMaps: %v", err)
	}

	_, err = service.Prepare(context.Background(), "pixel/session/1", "request-1",
		"Directions from Blue Bottle Coffee Oakland to SFO")
	if !errors.Is(err, manifest.ErrGateNotCleared) {
		t.Fatalf("Prepare = %v, want ErrGateNotCleared", err)
	}
	// The point of the whole change: the paid API call must never have
	// happened.
	if api.calls != 0 {
		t.Fatalf("api calls=%d, want 0 — the billing gate must stop resolve before it calls the API", api.calls)
	}
}
