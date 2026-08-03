package maps

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClientSearchesPlacesNewTextSearchAndParsesTheFirstPlace(t *testing.T) {
	var gotPath, gotFieldMask, gotAPIKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFieldMask = r.Header.Get("X-Goog-FieldMask")
		gotAPIKeyHeader = r.Header.Get("X-Goog-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"places":[{"id":"place1","displayName":{"text":"Blue Bottle Coffee"},"formattedAddress":"300 Webster St, Oakland, CA"}]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", "test-key-do-not-leak", nil, nil)
	place, err := client.SearchPlace(context.Background(), "blue bottle coffee")
	if err != nil {
		t.Fatalf("search place: %v", err)
	}
	if gotPath != "/v1/places:searchText" {
		t.Fatalf("path = %q, want /v1/places:searchText", gotPath)
	}
	if gotAPIKeyHeader != "test-key-do-not-leak" {
		t.Fatalf("api key header not set")
	}
	if gotFieldMask == "" {
		t.Fatalf("field mask header must be set to keep the response cheap")
	}
	if place.Name != "Blue Bottle Coffee" || place.Address != "300 Webster St, Oakland, CA" || place.ID != "place1" {
		t.Fatalf("place = %+v", place)
	}
}

func TestHTTPClientComputesRoutesAndParsesTheSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"routes":[{"distanceMeters":3100,"duration":"540s","description":"I-80 W"}]}`))
	}))
	defer server.Close()

	client := NewHTTPClient("", server.URL, "test-key-do-not-leak", nil, nil)
	route, err := client.ComputeRoute(context.Background(), "Home", "Work")
	if err != nil {
		t.Fatalf("compute route: %v", err)
	}
	if route.Summary != "I-80 W" {
		t.Fatalf("route summary = %q", route.Summary)
	}
	if route.Duration == "" || route.Distance == "" {
		t.Fatalf("route = %+v, missing human-readable duration/distance", route)
	}
}

func TestHTTPClientNeverLeaksTheAPIKeyInAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer server.Close()

	const secret = "super-secret-maps-key-value"
	client := NewHTTPClient(server.URL, server.URL, secret, nil, nil)
	_, err := client.SearchPlace(context.Background(), "anywhere")
	if err == nil {
		t.Fatalf("expected an error from a 403 response")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked the api key: %v", err)
	}
}
