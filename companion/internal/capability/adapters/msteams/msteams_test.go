package msteams

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
	chats   []Chat
	sent    []SendMessage
	reply   SentMessage
	cleared int
	err     error
}

func (f *fakeAPI) ListChats(context.Context) ([]Chat, error) { return f.chats, f.err }
func (f *fakeAPI) SendMessage(_ context.Context, request SendMessage) (SentMessage, error) {
	f.sent = append(f.sent, request)
	return f.reply, f.err
}
func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return f.err }

func TestManifestIsFreeAndroidRT2TeamsWorkOAuthRoute(t *testing.T) {
	a := New(&fakeAPI{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.ID == "teams" {
		t.Fatalf("id must be msteams (not deeplink teams): %+v", m)
	}
	if m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Send) {
		t.Fatalf("msteams must offer read and send: %v", m.Verbs)
	}
	if m.Allows(manifest.Write) || m.Allows(manifest.Compose) {
		t.Fatalf("msteams Wave 1 stays read+send only: %v", m.Verbs)
	}
	if m.Ceiling != manifest.Completes || m.Consent != manifest.ConsentA {
		t.Fatalf("ceiling/consent=%s/%s", m.Ceiling, m.Consent)
	}
}

func TestReadFindsOneChatAndRefusesAmbiguousMatch(t *testing.T) {
	api := &fakeAPI{chats: []Chat{
		{ID: "c1", Topic: "alerts"},
		{ID: "c2", Topic: "team-alerts"},
	}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "alert"})
	if !errors.Is(err, ErrAmbiguousChat) {
		t.Fatalf("ambiguous read returned %v", err)
	}
	var question *adapter.ClarificationError
	if !errors.As(err, &question) || question.Question == "" {
		t.Fatalf("ambiguous read is not a user question: %T %v", err, err)
	}
	plan, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "alerts"})
	if err != nil {
		t.Fatalf("exact read: %v", err)
	}
	if plan.Handle != "c1" || plan.Details["chat_topic"] != "alerts" {
		t.Fatalf("plan=%+v", plan)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil || !out.Done || !strings.Contains(out.Detail, "alerts") {
		t.Fatalf("execute=%+v err=%v", out, err)
	}
}

func TestSendRejectsEmptyBodyNeedsPreviewAndPostsOnce(t *testing.T) {
	api := &fakeAPI{
		chats: []Chat{{ID: "c9", Topic: "wave1"}},
		reply: SentMessage{ID: "msg-1", ChatID: "c9"},
	}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	_, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Send, Subject: "wave1", Body: "   ",
	})
	if !errors.Is(err, ErrEmptyBody) {
		t.Fatalf("empty body returned %v", err)
	}

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Send, Subject: "wave1", Body: "Operator Wave 1 Teams work proof",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := run.Execute(ctx, plan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed send returned %v", err)
	}
	if len(api.sent) != 0 {
		t.Fatalf("unconfirmed send posted %d messages", len(api.sent))
	}
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil || !out.Done || len(api.sent) != 1 {
		t.Fatalf("confirmed send outcome=%+v sent=%d err=%v", out, len(api.sent), err)
	}
	if api.sent[0].ChatID != "c9" || api.sent[0].Body != "Operator Wave 1 Teams work proof" {
		t.Fatalf("sent=%+v", api.sent[0])
	}
}

func TestRevokeClearsTokenSource(t *testing.T) {
	api := &fakeAPI{}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := a.Revoke(context.Background()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if api.cleared != 1 {
		t.Fatalf("cleared=%d", api.cleared)
	}
	if _, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "x"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after revoke resolve=%v", err)
	}
}
