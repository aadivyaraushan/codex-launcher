package instagram

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

func TestManifestIsAndroidRT4HandsOffComposeWithoutAuth(t *testing.T) {
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT4 || m.Ceiling != manifest.HandsOff {
		t.Fatalf("unexpected manifest core: %+v", m)
	}
	if m.Consent != manifest.ConsentA || m.Auth != manifest.AuthNone || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected consent/auth/cost: %+v", m)
	}
	if !m.Allows(manifest.Compose) || m.Allows(manifest.Send) {
		t.Fatalf("Instagram DM must offer compose only, not send: %v", m.Verbs)
	}
	if m.Platform != manifest.PlatformAndroid || m.ProvesCeiling == "" {
		t.Fatalf("platform=%s proves_ceiling=%q", m.Platform, m.ProvesCeiling)
	}
}

func TestComposeShowsDraftAndHandsOffWithoutClaimingSend(t *testing.T) {
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID,
		Verb:      manifest.Compose,
		Subject:   "Maya",
		Body:      "Running ten minutes late",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if plan.Details["draft"] != "Running ten minutes late" {
		t.Fatalf("draft = %q, want the body text the user will paste", plan.Details["draft"])
	}
	if plan.Details["android_package"] != AndroidPackage {
		t.Fatalf("android_package = %q, want %q", plan.Details["android_package"], AndroidPackage)
	}

	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Running ten minutes late") {
		t.Fatalf("preview did not show the draft: %q", shown)
	}
	if !strings.Contains(shown, "Maya") {
		t.Fatalf("preview did not name the subject as a hint: %q", shown)
	}
	if strings.Contains(strings.ToLower(preview.Confirm), "send") {
		t.Fatalf("confirm label must not claim send: %q", preview.Confirm)
	}

	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Reached != manifest.HandsOff || !out.Done || out.HandedOffTo != "Instagram" {
		t.Fatalf("outcome = %+v, want hands_off Done=true HandedOffTo=Instagram", out)
	}
	lower := strings.ToLower(out.Detail)
	if strings.Contains(lower, "sent") || strings.Contains(lower, "message sent") {
		t.Fatalf("detail claims a send: %q", out.Detail)
	}
	if !strings.Contains(lower, "cannot know") && !strings.Contains(lower, "does not know") {
		t.Fatalf("detail must say Operator cannot know whether the user finished: %q", out.Detail)
	}
	if !strings.Contains(out.Detail, "Running ten minutes late") {
		t.Fatalf("detail must keep the draft visible after hand-off: %q", out.Detail)
	}
}

func TestEmptyDraftAndSendAreRejected(t *testing.T) {
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Compose, Subject: "Maya", Body: "   ",
	})
	if !errors.Is(err, ErrEmptyDraft) {
		t.Fatalf("empty draft returned %v", err)
	}
	_, err = a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Send, Subject: "Maya", Body: "hi",
	})
	if err == nil {
		t.Fatal("send was accepted; Instagram must never claim send")
	}
}

func TestRevokeIsANoOpBecauseThereIsNoCredential(t *testing.T) {
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := a.Revoke(context.Background()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
}
