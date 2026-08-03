package flow

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
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/contacts"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

// The plan promises that "every official account connection can be revoked by
// the user". Today it cannot. Both halves of a revoke are written and tested —
// consent.Store.Revoke proves the grant and both vaults are empty, and every
// adapter's own Revoke drops its client — and the only things that call either
// are three proof commands in internal/capability/proving. Nothing a person
// holding the phone can touch reaches them.
//
// These tests define the one method that joins the two, so a single user
// action can undo a connection completely. The interesting rules are all about
// what happens when half of it fails, because a disconnect that reports
// success while a token is still live is worse than one that plainly fails.

// stubbornAdapter refuses to let go of its credentials. It stands for the real
// case: a token endpoint that is down, or an API that answers 500 to a revoke.
type stubbornAdapter struct {
	id       string
	consent  manifest.Consent
	revokeErr error
	revokes  int
	executed int
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
	s.executed++
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
	resolver := stage2.New(reg, contacts.NewGraph(nil), stage2.ClassMap{
		"tasks": {Adapters: []string{a.id}, Addressing: stage2.ToAThing},
	}, manifest.PlatformAndroid)

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

	model := func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"tasks","app_named":"","subject":"a thing","body":"","confidence":0.97}`), nil
	}
	return New(stage1.New(model), resolver, execution.New(reg), gate, logger), gate, tokens
}

// The plain case: one user action undoes both halves of a connection.
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

// The rule that decides the order of the two halves. If the consent record is
// cleared first and dropping the credentials then fails, Operator has thrown
// away its own memory of the connection while the live token is still out
// there — nothing left to point a retry at, and the user was told nothing is
// wrong. Credentials go first, and a failure there stops the whole thing.
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

// A preview can be sitting on screen when the user goes and disconnects the
// app. Confirming it afterwards must find nothing to confirm — otherwise the
// action they just withdrew the connection for runs anyway.
func TestDisconnectingLeavesNoPendingPreviewBehind(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentA}
	service, _, _ := disconnectService(t, a)
	ctx := context.Background()

	preview, err := service.Prepare(ctx, "pixel-9/session-1/1", "request-1", "add a thing")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := service.Disconnect(ctx, "gated"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if service.Pending("pixel-9/session-1/1", "request-1") {
		t.Error("a preview for the disconnected app is still pending")
	}
	if _, err := service.Confirm(ctx, "pixel-9/session-1/1", "request-1", preview.Fingerprint); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("Confirm after disconnect returned %v, want ErrUnknownRequest", err)
	}
	if a.executed != 0 {
		t.Error("the action ran after the user disconnected the app it belonged to")
	}
}

// Only previews for the disconnected app go. Disconnecting Todoist must not
// silently cancel a Spotify sheet the user is halfway through.
func TestDisconnectingOneAppDoesNotCancelAnotherAppsPreview(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tasks := &stubbornAdapter{id: "tasks-app", consent: manifest.ConsentA}
	music := &stubbornAdapter{id: "music-app", consent: manifest.ConsentA}
	reg := registry.New()
	for _, a := range []*stubbornAdapter{tasks, music} {
		if err := reg.Register(a); err != nil {
			t.Fatalf("register %s: %v", a.id, err)
		}
	}
	resolver := stage2.New(reg, contacts.NewGraph(nil), stage2.ClassMap{
		"tasks": {Adapters: []string{"tasks-app"}, Addressing: stage2.ToAThing},
		"music": {Adapters: []string{"music-app"}, Addressing: stage2.ToAThing},
	}, manifest.PlatformAndroid)
	class := "tasks"
	model := func(context.Context, string) ([]byte, error) {
		return []byte(`{"verb":"write","app_class":"` + class + `","app_named":"","subject":"a thing","body":"","confidence":0.97}`), nil
	}
	service := New(stage1.New(model), resolver, execution.New(reg),
		consent.New(func() time.Time { return time.Unix(0, 0) }, nil, consent.NoVault{}, consent.NoVault{}), logger)
	ctx := context.Background()

	if _, err := service.Prepare(ctx, "pixel-9/session-1/1", "request-tasks", "add a thing"); err != nil {
		t.Fatalf("Prepare tasks: %v", err)
	}
	class = "music"
	if _, err := service.Prepare(ctx, "pixel-9/session-1/1", "request-music", "play a thing"); err != nil {
		t.Fatalf("Prepare music: %v", err)
	}

	if err := service.Disconnect(ctx, "tasks-app"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if service.Pending("pixel-9/session-1/1", "request-tasks") {
		t.Error("the disconnected app's preview is still pending")
	}
	if !service.Pending("pixel-9/session-1/1", "request-music") {
		t.Error("disconnecting one app cancelled a different app's pending preview")
	}
}

// Tapping disconnect on an app that is already disconnected is the most
// ordinary thing a user does — they are not sure it worked, so they tap it
// again. It has to say yes.
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
}

// An id no build has is not "nothing to do" — it is a phone asking about an
// app this companion has never heard of, and answering "disconnected" tells
// the user a connection was undone that was never examined at all.
func TestDisconnectingAnAppThisBuildDoesNotHaveIsAnError(t *testing.T) {
	a := &stubbornAdapter{id: "gated", consent: manifest.ConsentA}
	service, _, _ := disconnectService(t, a)

	if err := service.Disconnect(context.Background(), "an-app-that-is-not-here"); err == nil {
		t.Fatal("disconnecting an app this build does not have was reported as a success")
	}
}
