// Gate: importers=AndroidBroker maps client; callers=unit tests;
// API=POST /v1/broker/maps/places:searchText + routes:computeRoutes;
// schemas=maps.API Place/Route JSON; user: Go→Android Places/Routes RPC
package mapsbroker_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mapsbroker "github.com/codex-launcher/codex-launcher/companion/internal/androidbroker/maps"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/maps"
)

func TestClientSearchPlacePostsBrokerPathAndParsesPlace(t *testing.T) {
	var gotPath, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "ChIJtest",
			"name":    "Ferry Building",
			"address": "San Francisco, CA",
		})
	}))
	t.Cleanup(server.Close)

	client := mapsbroker.NewClient(server.URL, nil, nil)
	place, err := client.SearchPlace(context.Background(), "Ferry Building")
	if err != nil {
		t.Fatalf("SearchPlace: %v", err)
	}
	if gotPath != "/v1/broker/maps/places:searchText" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.Contains(gotBody, `"query":"Ferry Building"`) {
		t.Fatalf("body=%q", gotBody)
	}
	if place.ID != "ChIJtest" || place.Name != "Ferry Building" || place.Address != "San Francisco, CA" {
		t.Fatalf("place=%+v", place)
	}
	var _ maps.API = client
}

func TestClientComputeRoutePostsBrokerPathAndParsesRoute(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"summary":  "US-101",
			"distance": "8.7 km",
			"duration": "16 mins",
		})
	}))
	t.Cleanup(server.Close)

	client := mapsbroker.NewClient(server.URL, nil, nil)
	route, err := client.ComputeRoute(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("ComputeRoute: %v", err)
	}
	if gotPath != "/v1/broker/maps/routes:computeRoutes" {
		t.Fatalf("path=%q", gotPath)
	}
	if route.Summary != "US-101" || route.Distance != "8.7 km" || route.Duration != "16 mins" {
		t.Fatalf("route=%+v", route)
	}
}

func TestClientPropagatesNonOKWithoutLeakingQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream"}`))
	}))
	t.Cleanup(server.Close)

	client := mapsbroker.NewClient(server.URL, nil, nil)
	_, err := client.SearchPlace(context.Background(), "secret-place-query")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-place-query") {
		t.Fatalf("error leaked query: %v", err)
	}
}
