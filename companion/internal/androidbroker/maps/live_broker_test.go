// Fact-force:
// 1) Callers: `go test` with MAPS_BROKER_LIVE=1; Client.SearchPlace/ComputeRoute
// 2) No prior live_broker_test.go in androidbroker/maps
// 3) No data files; live HTTP JSON Place/Route
// 4) User: "Run live Maps place + directions via Go→Android broker / phone-runtime path"
package mapsbroker

import (
	"context"
	"os"
	"testing"
)

func TestLiveBrokerPlaceAndRoute(t *testing.T) {
	if os.Getenv("MAPS_BROKER_LIVE") == "" {
		t.Skip("set MAPS_BROKER_LIVE=1 with adb forward tcp:9451 tcp:9451")
	}
	base := os.Getenv("MAPS_BROKER_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:9451"
	}
	c := NewClient(base, nil, nil)
	place, err := c.SearchPlace(context.Background(), "Ferry Building San Francisco")
	if err != nil {
		t.Fatalf("SearchPlace: %v", err)
	}
	if place.ID == "" || place.Name == "" {
		t.Fatalf("empty place: %+v", place)
	}
	route, err := c.ComputeRoute(context.Background(),
		"Ferry Building, San Francisco, CA",
		"Golden Gate Bridge, San Francisco, CA")
	if err != nil {
		t.Fatalf("ComputeRoute: %v", err)
	}
	if route.Distance == "" || route.Duration == "" {
		t.Fatalf("empty route: %+v", route)
	}
	t.Logf("place=%s id=%s route=%s %s %s", place.Name, place.ID, route.Summary, route.Distance, route.Duration)
}
