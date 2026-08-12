package disconnect

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
)

// This package takes over the disconnect behavior from flow.Service when the
// routed pipeline is deleted: one call undoes both halves of a connection —
// the adapter's own credential drop and the consent store's grant/vault wipe.
// The rules about half-failures carry over unchanged, because a disconnect
// that reports success while a token is still live is worse than one that
// plainly fails. What does not carry over is the pending-preview scan:
// previews no longer exist once the flow pipeline is gone.

// stubbornAdapter refuses to let go of its credentials. It stands for the
// real case: a token endpoint that is down, or an API that answers 500 to a
// revoke.
type stubbornAdapter struct {
	id        string
	consent   manifest.Consent
	revokeErr error
	revokes   int
}

func (s *stubbornAdapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID:            s.id,
		Runtime:       manifest.RT2,
		Verbs:         []manifest.Verb{manifest.Write},
		Ceiling:       manifest.Completes,
		Consent:       s.consent,
		Auth:          manifest.AuthNone,
		Cost:          manifest.CostFree,
		Capacity:      manifest.Capacity{Kind: manifest.CapacityNone},
		Platform:      manifest.PlatformBoth,
		Gates:         []manifest.Gate{manifest.GateNone},
		ProvesCeiling: s.id + "_smoke",
	}
}

func (s *stubbornAdapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{AdapterID: s.id, Verb: in.Verb, Summary: in.Subject}, nil
}

func (s *stubbornAdapter) Preview(_ context.Context, p adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{Plan: p, Headline: p.Summary, Confirm: "Do it"}, nil
}

func (s *stubbornAdapter) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	return adapter.Outcome{Reached: manifest.Completes, Done: true}, nil
}

func (s *stubbornAdapter) Revoke(context.Context) error {
	s.revokes++
	return s.revokeErr
}

// countingVault records whether the token store still holds anything, so a
// test can ask the same question consent.Revoke's proof asks.
type countingVault struct{ held map[string]bool }

func newCountingVault() *countingVault { return &countingVault{held: map[string]bool{}} }

func (v *countingVault) Delete(_ context.Context, id string) error {
	delete(v.held, id)
	return nil
}

func (v *countingVault) Has(_ context.Context, id string) (bool, error) { return v.held[id], nil }

func disconnectService(t *testing.T, a *stubbornAdapter) (*Service, *consent.Store, *countingVault) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	tokens := newCountingVault()
	gate := consent.New(
		func() time.Time { return time.Unix(0, 0) },
		map[string]consent.Copy{a.id: {
			Situation: "This app has no official route, so Operator acts as you.",
			Grants:    []string{"Create items on your behalf"},
			Risk:      "Anything it creates looks like you created it.",
			Absence:   consent.AbsenceChoice,
		}},
		tokens, consent.NoVault{},
	)
	return New(execution.New(reg), gate, logger), gate, tokens
}

// The plain case: one call undoes both halves of a connection.
func TestDisconnectingAnAppDropsItsCredentialsAndItsGrant(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentB}
	service, gate, tokens := disconnectService(t, a)
	ctx := context.Background()

	screen, err := gate.Screen(a.Describe())
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if err := gate.Grant(ctx, a.Describe(), screen); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	tokens.held["gated"] = true

	if err := service.Disconnect(ctx, "gated"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if a.revokes == 0 {
		t.Error("the adapter was never asked to drop its own credentials")
	}
	if gate.Granted("gated") {
		t.Error("the consent grant survived a disconnect; the app is still allowed to act")
	}
	if held, _ := tokens.Has(ctx, "gated"); held {
		t.Error("the token vault still holds this app's token after a disconnect")
	}
}

// Credentials go first, and a failure there stops the whole thing. If the
// consent record were cleared first and dropping the credentials then failed,
// Operator would have thrown away its own memory of the connection while the
// live token is still out there — nothing left to point a retry at, and the
// user told nothing is wrong.
func TestAFailedCredentialRevokeLeavesTheGrantAloneAndReportsTheFailure(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentB, revokeErr: errors.New("token endpoint is down")}
	service, gate, tokens := disconnectService(t, a)
	ctx := context.Background()

	screen, err := gate.Screen(a.Describe())
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if err := gate.Grant(ctx, a.Describe(), screen); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	tokens.held["gated"] = true

	if err := service.Disconnect(ctx, "gated"); err == nil {
		t.Fatal("a disconnect that could not drop the credentials reported success")
	}
	if !gate.Granted("gated") {
		t.Error("the grant was cleared even though the credentials are still live; a retry now has nothing to aim at")
	}
	if held, _ := tokens.Has(ctx, "gated"); !held {
		t.Error("the token vault was emptied even though the adapter refused to revoke")
	}
}

// Disconnecting twice is the most ordinary thing that happens — the agent (or
// the owner behind it) is not sure it worked, so it is asked again. It has to
// say yes, and it must not fail on the registry lookup that can never come
// back after the first disconnect unregistered the adapter.
func TestDisconnectingTwiceStillSucceeds(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentA}
	service, _, _ := disconnectService(t, a)
	ctx := context.Background()

	if err := service.Disconnect(ctx, "gated"); err != nil {
		t.Fatalf("first Disconnect: %v", err)
	}
	if err := service.Disconnect(ctx, "gated"); err != nil {
		t.Fatalf("second Disconnect on an already-disconnected app failed: %v", err)
	}
	if a.revokes != 1 {
		t.Fatalf("adapter revoked %d times, want exactly 1 — the repeat must not reach the registry", a.revokes)
	}
}

// An id no build has is not "nothing to do" — it is a caller asking about an
// app this companion has never heard of, and answering "disconnected" tells
// the user a connection was undone that was never examined at all.
func TestDisconnectingAnAppThisBuildDoesNotHaveIsAnError(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentA}
	service, _, _ := disconnectService(t, a)

	if err := service.Disconnect(context.Background(), "an-app-that-is-not-here"); err == nil {
		t.Fatal("disconnecting an app this build does not have was reported as a success")
	}
}

// Disconnected is how the bridge tells a repeat disconnect (idempotent
// success, no new owner gate) apart from an id this build never had (404).
func TestDisconnectedReportsOnlyAppsThatWentThroughHere(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentA}
	service, _, _ := disconnectService(t, a)
	ctx := context.Background()

	if service.Disconnected("gated") {
		t.Fatal("an app that was never disconnected must not report as disconnected")
	}
	if err := service.Disconnect(ctx, "gated"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if !service.Disconnected("gated") {
		t.Fatal("a disconnected app must report as disconnected")
	}
	if service.Disconnected("an-app-that-is-not-here") {
		t.Fatal("an id this build never had must not report as disconnected")
	}
}
