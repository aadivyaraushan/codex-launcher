package outlook

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
	messages []Message
	drafts   []CreateDraft
	sent     []SendMail
	reply    Message
	cleared  int
	err      error
}

func (f *fakeAPI) ListMessages(context.Context, string) ([]Message, error) { return f.messages, f.err }
func (f *fakeAPI) CreateDraft(_ context.Context, request CreateDraft) (Message, error) {
	f.drafts = append(f.drafts, request)
	return f.reply, f.err
}
func (f *fakeAPI) SendMail(_ context.Context, request SendMail) error {
	f.sent = append(f.sent, request)
	return f.err
}
func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return f.err }

func TestManifestIsFreeAndroidRT2OutlookOAuthRoute(t *testing.T) {
	a := New(&fakeAPI{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Write) || !m.Allows(manifest.Send) {
		t.Fatalf("outlook must offer read, write, send: %v", m.Verbs)
	}
}

func TestReadFindsOneMessageAndRefusesAmbiguousMatch(t *testing.T) {
	api := &fakeAPI{messages: []Message{
		{ID: "m1", Subject: "Invoice"},
		{ID: "m2", Subject: "Invoice reminder"},
	}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "Invoi"})
	if !errors.Is(err, ErrAmbiguousMessage) {
		t.Fatalf("ambiguous read returned %v", err)
	}
	var question *adapter.ClarificationError
	if !errors.As(err, &question) || question.Question == "" {
		t.Fatalf("ambiguous read is not a user question: %T %v", err, err)
	}
	plan, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "Invoice reminder"})
	if err != nil {
		t.Fatalf("exact read: %v", err)
	}
	if plan.Handle != "m2" || plan.Details["subject"] != "Invoice reminder" {
		t.Fatalf("plan=%+v", plan)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil || !out.Done || !strings.Contains(out.Detail, "Invoice reminder") {
		t.Fatalf("execute=%+v err=%v", out, err)
	}
}

func TestWriteAndSendNeedExactPreview(t *testing.T) {
	api := &fakeAPI{reply: Message{ID: "draft-1", Subject: "Operator draft"}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	writePlan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Write, Subject: "Operator draft", Body: "body text",
	})
	if err != nil {
		t.Fatalf("write resolve: %v", err)
	}
	if _, err := run.Execute(ctx, writePlan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed write returned %v", err)
	}
	preview, err := run.Preview(ctx, writePlan)
	if err != nil {
		t.Fatalf("write preview: %v", err)
	}
	out, err := run.Execute(ctx, writePlan, preview.Confirmed())
	if err != nil || !out.Done || len(api.drafts) != 1 {
		t.Fatalf("confirmed write outcome=%+v drafts=%d err=%v", out, len(api.drafts), err)
	}

	sendPlan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Send, Subject: "friend@example.com", Body: "hello from Operator",
	})
	if err != nil {
		t.Fatalf("send resolve: %v", err)
	}
	if _, err := run.Execute(ctx, sendPlan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed send returned %v", err)
	}
	sendPreview, err := run.Preview(ctx, sendPlan)
	if err != nil {
		t.Fatalf("send preview: %v", err)
	}
	out, err = run.Execute(ctx, sendPlan, sendPreview.Confirmed())
	if err != nil || !out.Done || len(api.sent) != 1 {
		t.Fatalf("confirmed send outcome=%+v sent=%d err=%v", out, len(api.sent), err)
	}
	if api.sent[0].To != "friend@example.com" || api.sent[0].Body != "hello from Operator" {
		t.Fatalf("sent=%+v", api.sent[0])
	}
}
