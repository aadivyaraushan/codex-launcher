package msteams

// Callers: msteams HTTPClient unit tests only.
// Affected API: ListChats / SendMessage shapes + Bearer auth.
// Schema: Graph chat JSON. No secrets.
// Docs: Context7 /websites/learn_microsoft_en-us_graph — GET /me/chats,
// POST /chats/{id}/messages; personal MSA unsupported.

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

func TestHTTPClientListsAndSendsWithBearerAuth(t *testing.T) {
	var sawList, sawSend bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-secret" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1.0/me/chats"):
			sawList = true
			_, _ = io.WriteString(w, `{"value":[{"id":"c1","topic":"Wave1","chatType":"group"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.0/chats/c1/messages":
			sawSend = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"content":"hello work teams"`) {
				t.Errorf("send body=%s", body)
			}
			_, _ = io.WriteString(w, `{"id":"msg-9","chatId":"c1"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, staticToken("access-secret"), server.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	chats, err := client.ListChats(context.Background())
	if err != nil || len(chats) != 1 || chats[0].ID != "c1" || chats[0].Topic != "Wave1" {
		t.Fatalf("ListChats=%+v err=%v", chats, err)
	}
	sent, err := client.SendMessage(context.Background(), SendMessage{ChatID: "c1", Body: "hello work teams"})
	if err != nil || sent.ID != "msg-9" || sent.ChatID != "c1" {
		t.Fatalf("SendMessage=%+v err=%v", sent, err)
	}
	if !sawList || !sawSend {
		t.Fatalf("sawList=%v sawSend=%v", sawList, sawSend)
	}
}
