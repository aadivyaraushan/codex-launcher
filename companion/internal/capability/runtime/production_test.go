package runtime

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// The gap these tests close: every adapter in this repo was reachable only
// from its own owner-only `serve-<name>-proof` command. Ordinary `serve` never
// built a capability flow at all, so handler.go's nil check refused every
// capability request a real phone could send. Roughly fifty adapters, the
// preview sheet and the whole confirm path were finished code that no user
// could reach.
//
// So this file tests the production build itself, not any one adapter: that it
// exists, that nothing registered is unroutable, and that nothing routable is
// unregistered.

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubModel stands in for the cloud router. Production wiring must be testable
// without a network call or an API key, or it will not be tested.
func stubModel(context.Context, string) ([]byte, error) {
	return []byte(`{"verb":"read","app_class":"tasks","app_named":"","subject":"x","body":"","confidence":0.9}`), nil
}

// The base case, and the most important one: a brand-new install with no keys
// for anything. Hand-off adapters need no credential — they only open an app —
// so the flow must still come up and still be useful. Returning an error here
// would mean a user with no accounts connected gets nothing at all, when what
// they should get is every prepare-and-open app.
func TestProductionFlowComesUpWithNoCredentialsAtAll(t *testing.T) {
	flow, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction with no credentials failed: %v", err)
	}
	if flow == nil {
		t.Fatal("no capability flow was built; every capability request would be refused")
	}
	if len(inv.Registered) == 0 {
		t.Fatal("no adapters registered; the phone would have nothing to route to")
	}
	// The deep-link pack is the credential-free half. If it is missing, the
	// build silently dropped the only adapters that always work.
	var sawDeeplink bool
	for _, id := range inv.Registered {
		if id == "uber" || id == "doordash" || id == "venmo" {
			sawDeeplink = true
		}
	}
	if !sawDeeplink {
		t.Errorf("registered %d adapters but none of the credential-free deep-link ones: %v",
			len(inv.Registered), inv.Registered)
	}
}

// The invariant that would have caught the original bug: something built but
// unreachable. An adapter in the registry that no class points at can never be
// chosen by the router, so it is dead weight that still reports itself as
// available.
func TestEveryRegisteredAdapterIsReachableFromSomeClass(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction failed: %v", err)
	}

	routable := map[string]bool{}
	for _, ids := range inv.Classes {
		for _, id := range ids {
			routable[id] = true
		}
	}
	var unreachable []string
	for _, id := range inv.Registered {
		if !routable[id] {
			unreachable = append(unreachable, id)
		}
	}
	if len(unreachable) > 0 {
		t.Errorf("registered but no class routes to them, so nothing can ever pick them: %v", unreachable)
	}
}

// The mirror failure: a class that names an adapter which was never registered.
// The router would choose it and the lookup would fail deep inside the run,
// after the user had already been shown a preview and confirmed it.
func TestEveryRoutableAdapterWasActuallyRegistered(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction failed: %v", err)
	}

	registered := map[string]bool{}
	for _, id := range inv.Registered {
		registered[id] = true
	}
	for class, ids := range inv.Classes {
		for _, id := range ids {
			if !registered[id] {
				t.Errorf("class %q routes to %q, which was never registered; "+
					"the user would confirm a preview and then hit a missing adapter", class, id)
			}
		}
	}
}

// A missing credential must leave the adapter out entirely, and say why. The
// failure this prevents: an adapter registered without its key, chosen by the
// router because it looked available, and failing only once the user has
// already confirmed.
func TestAnAdapterWithNoCredentialIsLeftOutAndTheReasonIsRecorded(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{Model: stubModel, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewProduction failed: %v", err)
	}

	registered := map[string]bool{}
	for _, id := range inv.Registered {
		registered[id] = true
	}
	if registered["maps"] {
		t.Error("the Maps adapter was registered with no API key; it would fail after the user confirmed")
	}
	reason, ok := inv.Skipped["maps"]
	if !ok {
		t.Fatal("Maps was skipped but no reason was recorded; nobody can tell why it is missing")
	}
	if strings.TrimSpace(reason) == "" {
		t.Error("the skip reason for Maps is blank")
	}
}

// The other half of the same rule: supply the credential and the adapter must
// appear, and must be routable. Otherwise connecting an account would silently
// change nothing.
func TestSupplyingACredentialAddsThatAdapterAndMakesItRoutable(t *testing.T) {
	_, inv, err := NewProduction(ProductionConfig{
		Model:       stubModel,
		Logger:      quietLogger(),
		MapsAPIKey:  "test-maps-key",
	})
	if err != nil {
		t.Fatalf("NewProduction failed: %v", err)
	}

	var found bool
	for _, id := range inv.Registered {
		if id == "maps" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a Maps API key was supplied but the adapter is missing: registered=%v skipped=%v",
			inv.Registered, inv.Skipped)
	}
	if _, stillSkipped := inv.Skipped["maps"]; stillSkipped {
		t.Error("Maps was registered and also reported as skipped")
	}

	var routable bool
	for _, ids := range inv.Classes {
		for _, id := range ids {
			if id == "maps" {
				routable = true
			}
		}
	}
	if !routable {
		t.Error("Maps was registered but no class routes to it, so the key changed nothing")
	}
}

// A build with no router cannot route, so it must fail loudly at startup
// rather than come up looking healthy and refuse every request later.
func TestAFlowWithNoRouterIsRefusedAtStartup(t *testing.T) {
	_, _, err := NewProduction(ProductionConfig{Logger: quietLogger()})
	if err == nil {
		t.Fatal("NewProduction with no model returned no error; it would start up unable to route anything")
	}
}
