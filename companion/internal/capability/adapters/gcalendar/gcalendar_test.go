package gcalendar

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

type fakeAPI struct {
	events  []Event
	created []CreateEvent
	reply   Event
	cleared int
	err     error
}

func (f *fakeAPI) ListEvents(context.Context, string) ([]Event, error) { return f.events, f.err }
func (f *fakeAPI) CreateEvent(_ context.Context, request CreateEvent) (Event, error) {
	f.created = append(f.created, request)
	return f.reply, f.err
}
func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return f.err }

func TestManifestIsFreeAndroidRT2CalendarOAuthRoute(t *testing.T) {
	a := New(&fakeAPI{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Write) {
		t.Fatalf("calendar must offer read and write: %v", m.Verbs)
	}
}

func TestReadFindsOneEventAndRefusesAmbiguousMatch(t *testing.T) {
	api := &fakeAPI{events: []Event{
		{ID: "e1", Summary: "Standup"},
		{ID: "e2", Summary: "Team Standup"},
	}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "stand"})
	if !errors.Is(err, ErrAmbiguousEvent) {
		t.Fatalf("ambiguous read returned %v", err)
	}
	plan, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "Team Standup"})
	if err != nil {
		t.Fatalf("exact read: %v", err)
	}
	if plan.Handle != "e2" || plan.Details["summary"] != "Team Standup" {
		t.Fatalf("plan=%+v", plan)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil || !out.Done || !strings.Contains(out.Detail, "Team Standup") {
		t.Fatalf("execute=%+v err=%v", out, err)
	}
}

func TestWriteNeedsExactPreviewAndCreatesOnce(t *testing.T) {
	api := &fakeAPI{reply: Event{ID: "e9", Summary: "Operator Wave 1"}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "Operator Wave 1", Body: "proof event",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := run.Execute(ctx, plan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed write returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("unconfirmed write created %d events", len(api.created))
	}
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil || !out.Done || len(api.created) != 1 {
		t.Fatalf("confirmed write outcome=%+v created=%d err=%v", out, len(api.created), err)
	}
	if api.created[0].Summary != "Operator Wave 1" || api.created[0].Description != "proof event" {
		t.Fatalf("created=%+v", api.created[0])
	}
}
