package todoist

import (
	"bytes"
	"context"
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
	tasks       []Task
	created     []CreateTask
	createReply Task
	cleared     int
	err         error
}

func (f *fakeAPI) ListTasks(context.Context) ([]Task, error) {
	return f.tasks, f.err
}

func (f *fakeAPI) CreateTask(_ context.Context, request CreateTask) (Task, error) {
	f.created = append(f.created, request)
	return f.createReply, f.err
}

func (f *fakeAPI) Clear(context.Context) error {
	f.cleared++
	return f.err
}

func TestManifestIsTheFreeAndroidRT2TodoistRoute(t *testing.T) {
	a := New(&fakeAPI{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := a.Describe()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	if m.ID != ID || m.Runtime != manifest.RT2 || m.Auth != manifest.AuthOAuth || m.Cost != manifest.CostFree {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if !m.Allows(manifest.Read) || !m.Allows(manifest.Write) {
		t.Fatalf("Todoist must offer read and write: %v", m.Verbs)
	}
	if m.Platform != manifest.PlatformAndroid || m.ProvesCeiling == "" {
		t.Fatalf("platform=%s proves_ceiling=%q", m.Platform, m.ProvesCeiling)
	}
}

func TestWriteNeedsItsExactPreviewAndCreatesExactlyOneTask(t *testing.T) {
	api := &fakeAPI{createReply: Task{ID: "task-42", Content: "Buy oat milk"}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	run := execution.New(reg)
	ctx := context.Background()

	plan, err := run.Resolve(ctx, adapter.Intent{
		AdapterID: ID,
		Verb:      manifest.Write,
		Subject:   "Buy oat milk",
		Body:      "Before tomorrow morning",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := run.Execute(ctx, plan, execution.Confirmation{}); !errors.Is(err, execution.ErrPreviewRequired) {
		t.Fatalf("unconfirmed write returned %v", err)
	}
	if len(api.created) != 0 {
		t.Fatalf("unconfirmed write created %d tasks", len(api.created))
	}

	preview, err := run.Preview(ctx, plan)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	shown := preview.Headline + " " + strings.Join(preview.Lines, " ")
	if !strings.Contains(shown, "Buy oat milk") || !strings.Contains(shown, "Before tomorrow morning") {
		t.Fatalf("preview did not show the exact task: %q", shown)
	}

	out, err := run.Execute(ctx, plan, preview.Confirmed())
	if err != nil {
		t.Fatalf("confirmed execute: %v", err)
	}
	if !out.Done || out.Reached != manifest.Completes || !strings.Contains(out.Detail, "task-42") {
		t.Fatalf("outcome = %+v", out)
	}
	if len(api.created) != 1 {
		t.Fatalf("confirmed write created %d tasks, want exactly one", len(api.created))
	}
	if api.created[0].Content != "Buy oat milk" || api.created[0].Description != "Before tomorrow morning" {
		t.Fatalf("create request = %+v", api.created[0])
	}
}

func TestReadFindsOneTaskAndRefusesAnAmbiguousMatch(t *testing.T) {
	api := &fakeAPI{tasks: []Task{
		{ID: "one", Content: "Buy oat milk"},
		{ID: "two", Content: "Buy oat milk for office"},
	}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "oat milk",
	})
	if !errors.Is(err, ErrAmbiguousTask) {
		t.Fatalf("ambiguous read returned %v", err)
	}

	plan, err := a.Resolve(context.Background(), adapter.Intent{
		AdapterID: ID, Verb: manifest.Read, Subject: "Buy oat milk for office",
	})
	if err != nil {
		t.Fatalf("exact read resolve: %v", err)
	}
	out, err := a.Execute(context.Background(), plan)
	if err != nil || !out.Done || !strings.Contains(out.Detail, "Buy oat milk for office") {
		t.Fatalf("read outcome=%+v err=%v", out, err)
	}
}

func TestRevokeClearsTheConnectionAndStopsFutureCalls(t *testing.T) {
	api := &fakeAPI{tasks: []Task{{ID: "one", Content: "task"}}}
	a := New(api, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := a.Revoke(context.Background()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if api.cleared != 1 {
		t.Fatalf("clear calls=%d, want 1", api.cleared)
	}
	if _, err := a.Resolve(context.Background(), adapter.Intent{Verb: manifest.Read, Subject: "task"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("resolve after revoke returned %v", err)
	}
}

type staticToken string

func (s staticToken) AccessToken(context.Context) (string, error) { return string(s), nil }

func TestHTTPClientPaginatesAndSendsTheDocumentedCreateShape(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks" && r.URL.Query().Get("cursor") == "":
			_, _ = io.WriteString(w, `{"results":[{"id":"one","content":"First"}],"next_cursor":"next"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks" && r.URL.Query().Get("cursor") == "next":
			_, _ = io.WriteString(w, `{"results":[{"id":"two","content":"Second"}],"next_cursor":null}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"content":"Buy oat milk","description":"Before tomorrow"}`+"\n" {
				t.Errorf("create body = %q", body)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"three","content":"Buy oat milk","description":"Before tomorrow"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	tasks, err := client.ListTasks(context.Background())
	if err != nil || len(tasks) != 2 {
		t.Fatalf("ListTasks len=%d err=%v", len(tasks), err)
	}
	created, err := client.CreateTask(context.Background(), CreateTask{Content: "Buy oat milk", Description: "Before tomorrow"})
	if err != nil || created.ID != "three" {
		t.Fatalf("CreateTask=%+v err=%v", created, err)
	}
	if requests != 3 {
		t.Fatalf("requests=%d, want 3", requests)
	}
}

func TestHTTPClientErrorsAndLogsNeverContainTokensOrTaskText(t *testing.T) {
	const token = "access-super-secret"
	const taskText = "private medical appointment"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "vendor rejected secret payload", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := NewHTTPClient(server.URL, staticToken(token), server.Client(), slog.New(slog.NewTextHandler(&logs, nil)))
	_, err := client.CreateTask(context.Background(), CreateTask{Content: taskText})
	if err == nil {
		t.Fatal("failed response returned no error")
	}
	combined := err.Error() + logs.String()
	for _, secret := range []string{token, taskText, "vendor rejected secret payload"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("error or logs leaked %q: %s", secret, combined)
		}
	}
}
