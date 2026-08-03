package outlook

// Callers: outlook HTTPClient unit tests only.
// Affected API: ListMessages/CreateDraft/SendMail shapes + Bearer + ConsistencyLevel.
// Schema: Graph mail JSON. No secrets.
// User: follow-up on Microsoft judge — add httptest coverage for Outlook client.

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

func TestHTTPClientListsDraftsAndSendsWithBearerAuth(t *testing.T) {
	var sawList, sawDraft, sawSend bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1.0/me/messages"):
			sawList = true
			if r.URL.Query().Get("$search") == "" {
				t.Errorf("expected $search, got %v", r.URL.Query())
			}
			if r.Header.Get("ConsistencyLevel") != "eventual" {
				t.Errorf("ConsistencyLevel=%q", r.Header.Get("ConsistencyLevel"))
			}
			_, _ = io.WriteString(w, `{"value":[{"id":"m1","subject":"Hello","bodyPreview":"Hi","from":{"emailAddress":{"address":"a@b.com"}}}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.0/me/messages":
			sawDraft = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"subject":"Draft"`) {
				t.Errorf("draft body=%s", body)
			}
			_, _ = io.WriteString(w, `{"id":"m2","subject":"Draft"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.0/me/sendMail":
			sawSend = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"saveToSentItems":true`) {
				t.Errorf("sendMail body=%s", body)
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	messages, err := client.ListMessages(context.Background(), "Hello")
	if err != nil || len(messages) != 1 || messages[0].ID != "m1" {
		t.Fatalf("ListMessages=%+v err=%v", messages, err)
	}
	draft, err := client.CreateDraft(context.Background(), CreateDraft{Subject: "Draft", Body: "text", To: "to@b.com"})
	if err != nil || draft.ID != "m2" {
		t.Fatalf("CreateDraft=%+v err=%v", draft, err)
	}
	if err := client.SendMail(context.Background(), SendMail{To: "to@b.com", Subject: "Hi", Body: "body"}); err != nil {
		t.Fatalf("SendMail: %v", err)
	}
	if !sawList || !sawDraft || !sawSend {
		t.Fatalf("sawList=%v sawDraft=%v sawSend=%v", sawList, sawDraft, sawSend)
	}
}
