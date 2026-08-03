package maps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// PlacesAPIBaseURL and RoutesAPIBaseURL are the current Google Maps
// Platform surfaces this client calls: Places API (New) Text Search (the
// older Places API is superseded by this) and Routes API computeRoutes
// (which supersedes the legacy Directions API).
const (
	PlacesAPIBaseURL = "https://places.googleapis.com"
	RoutesAPIBaseURL = "https://routes.googleapis.com"
)

// Place is the part of a Places API (New) Text Search result the adapter
// needs.
type Place struct {
	ID      string
	Name    string
	Address string
}

// Route is the part of a Routes API computeRoutes result the adapter
// needs, rendered into human-readable text.
type Route struct {
	Summary  string
	Distance string
	Duration string
}

type placesSearchResponse struct {
	Places []struct {
		ID          string `json:"id"`
		DisplayName struct {
			Text string `json:"text"`
		} `json:"displayName"`
		FormattedAddress string `json:"formattedAddress"`
	} `json:"places"`
}

type routesComputeResponse struct {
	Routes []struct {
		DistanceMeters int    `json:"distanceMeters"`
		Duration       string `json:"duration"`
		Description    string `json:"description"`
	} `json:"routes"`
}

// HTTPClient calls the real Places API (New) and Routes API using a plain
// API key (no per-user OAuth step). Both APIs are billed per call; there is
// no free daily quota the way YouTube has one.
type HTTPClient struct {
	placesBaseURL string
	routesBaseURL string
	apiKey        string
	http          *http.Client
	logger        *slog.Logger
}

var _ API = (*HTTPClient)(nil)

func NewHTTPClient(placesBaseURL, routesBaseURL, apiKey string, client *http.Client, logger *slog.Logger) *HTTPClient {
	if placesBaseURL == "" {
		placesBaseURL = PlacesAPIBaseURL
	}
	if routesBaseURL == "" {
		routesBaseURL = RoutesAPIBaseURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPClient{
		placesBaseURL: strings.TrimRight(placesBaseURL, "/"),
		routesBaseURL: strings.TrimRight(routesBaseURL, "/"),
		apiKey:        apiKey, http: client, logger: logger,
	}
}

// SearchPlace calls Places API (New) Text Search and returns the top match.
func (c *HTTPClient) SearchPlace(ctx context.Context, query string) (Place, error) {
	body, err := json.Marshal(map[string]string{"textQuery": query})
	if err != nil {
		return Place{}, fmt.Errorf("maps: encode place search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.placesBaseURL+"/v1/places:searchText", bytes.NewReader(body))
	if err != nil {
		return Place{}, fmt.Errorf("maps: build place search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-FieldMask", "places.id,places.displayName,places.formattedAddress")

	c.logger.Info("[maps] request", "method", http.MethodPost, "path", "/v1/places:searchText", "query_length", len(query))
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[maps] request failed", "path", "/v1/places:searchText")
		return Place{}, fmt.Errorf("maps: place search request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		c.logger.Error("[maps] response rejected", "path", "/v1/places:searchText", "status", resp.StatusCode)
		return Place{}, fmt.Errorf("maps: place search returned status %d", resp.StatusCode)
	}
	var parsed placesSearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return Place{}, fmt.Errorf("maps: decode place search response: %w", err)
	}
	if len(parsed.Places) == 0 {
		return Place{}, nil
	}
	top := parsed.Places[0]
	return Place{ID: top.ID, Name: top.DisplayName.Text, Address: top.FormattedAddress}, nil
}

// ComputeRoute calls Routes API computeRoutes for driving directions
// between two free-text addresses (the Routes API Waypoint accepts an
// address field directly; no separate geocoding step is required).
func (c *HTTPClient) ComputeRoute(ctx context.Context, origin, destination string) (Route, error) {
	payload := map[string]any{
		"origin":            map[string]any{"address": origin},
		"destination":       map[string]any{"address": destination},
		"travelMode":        "DRIVE",
		"routingPreference": "TRAFFIC_AWARE",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Route{}, fmt.Errorf("maps: encode route request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.routesBaseURL+"/directions/v2:computeRoutes", bytes.NewReader(body))
	if err != nil {
		return Route{}, fmt.Errorf("maps: build route request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-FieldMask", "routes.duration,routes.distanceMeters,routes.description")

	c.logger.Info("[maps] request", "method", http.MethodPost, "path", "/directions/v2:computeRoutes")
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Error("[maps] request failed", "path", "/directions/v2:computeRoutes")
		return Route{}, fmt.Errorf("maps: route request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		c.logger.Error("[maps] response rejected", "path", "/directions/v2:computeRoutes", "status", resp.StatusCode)
		return Route{}, fmt.Errorf("maps: compute routes returned status %d", resp.StatusCode)
	}
	var parsed routesComputeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return Route{}, fmt.Errorf("maps: decode route response: %w", err)
	}
	if len(parsed.Routes) == 0 {
		return Route{}, nil
	}
	top := parsed.Routes[0]
	return Route{
		Summary:  top.Description,
		Distance: formatDistance(top.DistanceMeters),
		Duration: formatDuration(top.Duration),
	}, nil
}

func formatDistance(meters int) string {
	km := float64(meters) / 1000
	return strconv.FormatFloat(km, 'f', 1, 64) + " km"
}

// formatDuration turns the Routes API's "540s" style duration into a
// rounded, human-readable "9 mins".
func formatDuration(raw string) string {
	seconds := strings.TrimSuffix(raw, "s")
	n, err := strconv.Atoi(seconds)
	if err != nil {
		return raw
	}
	mins := n / 60
	if n%60 >= 30 {
		mins++
	}
	if mins <= 1 {
		return strconv.Itoa(mins) + " min"
	}
	return strconv.Itoa(mins) + " mins"
}
