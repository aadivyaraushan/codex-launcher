package gcalendar

// Callers: gcalendar HTTPClient unit tests only.
// Affected API: ListEvents/CreateEvent request shape + Bearer header.
// Schema: Calendar events JSON list/create. No secrets.
// User: follow-up on Google judge — add httptest coverage for Calendar client.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticToken string

func (s staticToken) AccessToken(context.Context) (string, error) { return string(s), nil }

func TestHTTPClientListsAndCreatesWithBearerAuth(t *testing.T) {
	var sawList, sawCreate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events"):
			sawList = true
			if r.URL.Query().Get("q") != "Standup" {
				t.Errorf("query=%v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"e1","summary":"Standup","start":{"dateTime":"2026-08-02T10:00:00Z"},"end":{"dateTime":"2026-08-02T11:00:00Z"}}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/calendar/v3/calendars/primary/events":
			sawCreate = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"summary":"Demo"`) {
				t.Errorf("create body=%s", body)
			}
			_, _ = io.WriteString(w, `{"id":"e2","summary":"Demo","start":{"dateTime":"2026-08-02T12:00:00Z"},"end":{"dateTime":"2026-08-02T13:00:00Z"}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	events, err := client.ListEvents(context.Background(), "Standup")
	if err != nil || len(events) != 1 || events[0].ID != "e1" {
		t.Fatalf("ListEvents=%+v err=%v", events, err)
	}
	created, err := client.CreateEvent(context.Background(), CreateEvent{
		Summary: "Demo", Start: "2026-08-02T12:00:00Z", End: "2026-08-02T13:00:00Z",
	})
	if err != nil || created.ID != "e2" {
		t.Fatalf("CreateEvent=%+v err=%v", created, err)
	}
	if !sawList || !sawCreate {
		t.Fatalf("sawList=%v sawCreate=%v", sawList, sawCreate)
	}
}
