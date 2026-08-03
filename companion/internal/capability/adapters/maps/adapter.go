// Package maps is Operator's Google Maps support. It is two adapters, not
// one, because the two halves reach different ceilings and the manifest
// only carries a single ceiling per adapter:
//
//   - Adapter (ID "maps"): RT-2, verb read, ceiling completes. Places API
//     (New) + Routes API answer places/directions questions entirely inside
//     Operator, and can additionally open Google Maps with a route already
//     loaded via the google.navigation: intent (still completes, per the
//     plan's "put it on screen" rule for navigation/entertainment).
//   - SavedPlacesAdapter (ID "maps_saved_places"): RT-4 device hand-off,
//     verb write, ceiling hands_off. There is no official write API for
//     saving a place, so Operator prepares the place and opens Google Maps;
//     the user finishes adding it there. The copy never claims the place
//     was saved.
package maps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/handoff"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "maps"
const SavedPlacesID = "maps_saved_places"

const androidPackage = "com.google.android.apps.maps"

var (
	ErrNotConnected          = errors.New("maps: adapter is not connected")
	ErrEmptyQuery            = errors.New("maps: place query must not be empty")
	ErrNoPlaceFound          = errors.New("maps: no place matches the search")
	ErrMissingRouteEndpoints = errors.New("maps: directions need both an origin and a destination")
	ErrEmptyPlaceName        = errors.New("maps: place name must not be empty")
)

// API is the small part of Google Maps the places/directions adapter
// needs. HTTPClient is the real implementation; tests use a recording
// fake.
type API interface {
	SearchPlace(ctx context.Context, query string) (Place, error)
	ComputeRoute(ctx context.Context, origin, destination string) (Route, error)
}

// ---- places / directions (RT-2, completes) ----

type Adapter struct {
	api    API
	logger *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(api API, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{api: api, logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthNone, Cost: manifest.CostPerCall,
		Gates:    []manifest.Gate{manifest.GateBilling},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "maps_places_directions_smoke",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	if in.Verb != manifest.Read {
		return adapter.Plan{}, fmt.Errorf("maps: verb %q is not supported", in.Verb)
	}
	a.logger.Info("[maps] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "has_destination", in.Fields["destination"] != "")
	if strings.TrimSpace(in.Fields["destination"]) != "" {
		return a.resolveDirections(ctx, in)
	}
	return a.resolvePlace(ctx, in.Subject)
}

func (a *Adapter) resolvePlace(ctx context.Context, query string) (adapter.Plan, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return adapter.Plan{}, ErrEmptyQuery
	}
	place, err := a.api.SearchPlace(ctx, query)
	if err != nil {
		a.logger.Error("[maps] place search failed", "error", err)
		return adapter.Plan{}, err
	}
	if place.Name == "" {
		return adapter.Plan{}, ErrNoPlaceFound
	}
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Handle: place.ID, Summary: "Find a place on Google Maps",
		Details: map[string]string{"place_name": place.Name, "address": place.Address, "place_id": place.ID},
	}, nil
}

func (a *Adapter) resolveDirections(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	origin := strings.TrimSpace(in.Fields["origin"])
	destination := strings.TrimSpace(in.Fields["destination"])
	if origin == "" || destination == "" {
		return adapter.Plan{}, ErrMissingRouteEndpoints
	}
	route, err := a.api.ComputeRoute(ctx, origin, destination)
	if err != nil {
		a.logger.Error("[maps] compute route failed", "error", err)
		return adapter.Plan{}, err
	}
	details := map[string]string{
		"origin": origin, "destination": destination,
		"summary": route.Summary, "distance": route.Distance, "duration": route.Duration,
	}
	summary := "Get directions on Google Maps"
	if in.Fields["navigate"] == "true" {
		details["navigate"] = "true"
		details["maps_uri"] = "google.navigation:q=" + url.QueryEscape(destination)
		details["android_package"] = androidPackage
		summary = "Start navigation on Google Maps"
	}
	return adapter.Plan{AdapterID: ID, Verb: manifest.Read, Summary: summary, Details: details}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	if plan.Details["place_name"] != "" {
		return adapter.Preview{
			Plan: plan, Headline: plan.Summary,
			Lines:   []string{plan.Details["place_name"], plan.Details["address"]},
			Confirm: "Show place",
		}, nil
	}
	lines := []string{fmt.Sprintf("%s to %s: %s, %s", plan.Details["origin"], plan.Details["destination"], plan.Details["distance"], plan.Details["duration"])}
	confirm := "Show directions"
	if plan.Details["navigate"] == "true" {
		lines = append(lines, "Operator opens Google Maps with this route loaded.")
		confirm = "Open Google Maps"
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(_ context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[maps] execute", "verb", plan.Verb, "navigate", plan.Details["navigate"] == "true")
	if plan.Details["place_name"] != "" {
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("%s — %s", plan.Details["place_name"], plan.Details["address"]),
		}, nil
	}
	if plan.Details["navigate"] == "true" {
		return adapter.Outcome{
			Reached: manifest.HandsOff, Done: true, HandedOffTo: "Google Maps",
			Detail: fmt.Sprintf("Opened Google Maps with directions from %s to %s (%s, %s).",
				plan.Details["origin"], plan.Details["destination"], plan.Details["distance"], plan.Details["duration"]),
		}, nil
	}
	return adapter.Outcome{
		Reached: manifest.Completes, Done: true,
		Detail: fmt.Sprintf("From %s to %s: %s, %s.", plan.Details["origin"], plan.Details["destination"], plan.Details["distance"], plan.Details["duration"]),
	}, nil
}

// Revoke of an already-disconnected adapter reports success, not
// ErrNotConnected — see the comment on todoist's Revoke for why (a retried
// revoke should never look like a failed disconnect).
func (a *Adapter) Revoke(context.Context) error {
	if a.api == nil {
		return nil
	}
	a.api = nil
	a.logger.Info("[maps] revoked")
	return nil
}

// ---- saved places (RT-4 device hand-off, hands_off) ----

// SavedPlacesAdapter prepares a place for Google Maps and opens the app;
// there is no official write API to add a place to a list, so the user
// finishes there. Its copy must never claim the place was saved.
type SavedPlacesAdapter struct {
	logger *slog.Logger
}

var _ adapter.Adapter = (*SavedPlacesAdapter)(nil)

func NewSavedPlaces(logger *slog.Logger) *SavedPlacesAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &SavedPlacesAdapter{logger: logger}
}

func (a *SavedPlacesAdapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: SavedPlacesID, Runtime: manifest.RT4,
		Verbs:   []manifest.Verb{manifest.Write},
		Ceiling: manifest.HandsOff, Consent: manifest.ConsentA,
		Auth: manifest.AuthNone, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		Unshipped: "no build registers this adapter; NewMaps deliberately wires up only the " +
			"completes places/directions adapter and leaves preparing a place and opening Maps " +
			"to the deep-link pack's googlemaps entry instead, and no serve-maps-saved-places-proof " +
			"smoke test exists to back a ceiling claim here.",
	}
}

func (a *SavedPlacesAdapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	if in.Verb != manifest.Write {
		return adapter.Plan{}, fmt.Errorf("maps: verb %q is not supported", in.Verb)
	}
	place := strings.TrimSpace(in.Subject)
	if place == "" {
		return adapter.Plan{}, ErrEmptyPlaceName
	}
	draft := fmt.Sprintf("Add %q to a list in Google Maps", place)
	return adapter.Plan{
		AdapterID: SavedPlacesID, Verb: manifest.Write, Summary: "Prepare a place for Google Maps",
		Details: map[string]string{
			"place": place, "draft": draft, "android_package": androidPackage,
			"maps_uri": "geo:0,0?q=" + url.QueryEscape(place),
		},
	}, nil
}

func (a *SavedPlacesAdapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{
		Plan: plan, Headline: plan.Summary,
		Lines:   []string{plan.Details["draft"], "Operator opens Google Maps. You choose the list and finish there."},
		Confirm: "Open Google Maps",
	}, nil
}

func (a *SavedPlacesAdapter) Execute(_ context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	a.logger.Info("[maps] hand off saved place", "place_length", len(plan.Details["place"]))
	return handoff.DraftOutcome("Google Maps", plan.Details["draft"]), nil
}

func (a *SavedPlacesAdapter) Revoke(context.Context) error {
	return nil
}
