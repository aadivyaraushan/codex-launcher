package slack

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

type fakeAPI struct {
	channels  []Channel
	posted    []PostMessage
	postReply PostedMessage
	cleared   int
	err       error
}

func (f *fakeAPI) ListChannels(context.Context) ([]Channel, error) { return f.channels, f.err }
func (f *fakeAPI) PostMessage(_ context.Context, request PostMessage) (PostedMessage, error) {
	f.posted = append(f.posted, request)
	return f.postReply, f.err
}
func (f *fakeAPI) Clear(context.Context) error { f.cleared++; return f.err }

func TestManifestIsFreeAndroidRT2SlackUserRoute(t *testing.T) {
	a := New(&fakeAPI{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Send) {
		t.Fatalf("Slack must offer read and send: %v", m.Verbs)
	}
	if m.Ceiling != manifest.Completes || m.Consent != manifest.ConsentA {
		t.Fatalf("ceiling/consent=%s/%s", m.Ceiling, m.Consent)
	}
}

func TestReadFindsOneChannelAndRefusesAmbiguousMatch(t *testing.T) {
	api := &fakeAPI{channels: []Channel{
		{ID: "C1", Name: "alerts"},
		{ID: "C2", Name: "team-alerts"},
	}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "alert"})
	if !errors.Is(err, ErrAmbiguousChannel) {
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
	if plan.Handle != "C1" || plan.Details["channel_name"] != "alerts" {
		t.Fatalf("plan=%+v", plan)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil || !out.Done || !strings.Contains(out.Detail, "alerts") {
		t.Fatalf("execute=%+v err=%v", out, err)
	}
}

func TestSendNeedsExactPreviewAndPostsOnce(t *testing.T) {
	api := &fakeAPI{
		channels:  []Channel{{ID: "C9", Name: "wave1"}},
		postReply: PostedMessage{Channel: "C9", Timestamp: "1503435956.000247"},
	}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID, Verb: manifest.Send, Subject: "wave1", Body: "Operator Wave 1 Slack proof",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := run.Execute(ctx, plan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed send returned %v", err)
	}
	if len(api.posted) != 0 {
		t.Fatalf("unconfirmed send posted %d messages", len(api.posted))
	}
	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "wave1") || !strings.Contains(shown, "Operator Wave 1 Slack proof") {
		t.Fatalf("preview did not show channel and text: %q", shown)
	}
	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("confirmed execute: %v", err)
	}
	if !out.Done || out.Reached != manifest.Completes || !strings.Contains(out.Detail, "1503435956.000247") {
		t.Fatalf("outcome=%+v", out)
	}
	if len(api.posted) != 1 || api.posted[0].Channel != "C9" || api.posted[0].Text != "Operator Wave 1 Slack proof" {
		t.Fatalf("posted=%+v", api.posted)
	}
}

func TestHTTPClientListsChannelsAndPostsWithUserToken(t *testing.T) {
	var paths []string
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/conversations.list":
			_, _ = io.WriteString(w, `{"ok":true,"channels":[{"id":"C1","name":"general","is_archived":false}]}`)
		case "/api/chat.postMessage":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["channel"] != "C1" || body["text"] != "hello" {
				t.Fatalf("post body=%v", body)
			}
			_, _ = io.WriteString(w, `{"ok":true,"channel":"C1","ts":"1.2"}`)
		case "/api/auth.revoke":
			_, _ = io.WriteString(w, `{"ok":true,"revoked":true}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	tokens := &staticTokens{token: "xoxp-user"}
	client := NewHTTPClient(server.URL, tokens, server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	channels, err := client.ListChannels(context.Background())
	if err != nil || len(channels) != 1 || channels[0].Name != "general" {
		t.Fatalf("ListChannels=%v err=%v", channels, err)
	}
	posted, err := client.PostMessage(context.Background(), PostMessage{Channel: "C1", Text: "hello"})
	if err != nil || posted.Timestamp != "1.2" {
		t.Fatalf("PostMessage=%+v err=%v", posted, err)
	}
	if err := client.Clear(context.Background()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if tokens.cleared != 1 {
		t.Fatalf("token clear calls=%d", tokens.cleared)
	}
	for _, header := range authHeaders {
		if header != "Bearer xoxp-user" {
			t.Fatalf("auth header=%q", header)
		}
	}
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, "/api/conversations.list") || !strings.Contains(joined, "/api/chat.postMessage") {
		t.Fatalf("paths=%v", paths)
	}
}

func TestHTTPClientReadsAuthenticatedWorkspaceIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth.test" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer xoxp-user" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"ok":true,"url":"https://aadivyasagents.slack.com/","team":"aadivya's agents","user":"aadivya","team_id":"T1","user_id":"U1"}`)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, &staticTokens{token: "xoxp-user"}, server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	identity, err := client.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if identity.Team != "aadivya's agents" || identity.URL != "https://aadivyasagents.slack.com/" || identity.TeamID != "T1" || identity.UserID != "U1" {
		t.Fatalf("Identity = %+v", identity)
	}
}

func TestRevokeClearsTokenAndDisconnectsAdapter(t *testing.T) {
	api := &fakeAPI{channels: []Channel{{ID: "C1", Name: "general"}}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := a.Revoke(context.Background()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if api.cleared != 1 {
		t.Fatalf("cleared=%d", api.cleared)
	}
	if _, err := a.Resolve(context.Background(), adapter.Intent{AdapterID: ID, Verb: manifest.Read, Subject: "general"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("resolve after revoke=%v", err)
	}
}

type staticTokens struct {
	token   string
	cleared int
}

func (s *staticTokens) AccessToken(context.Context) (string, error) { return s.token, nil }
func (s *staticTokens) Clear(context.Context) error                 { s.cleared++; return nil }
