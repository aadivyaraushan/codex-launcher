package beeper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Workstream B1 (read side). These hold the new read methods to the endpoint
// shapes inventoried in the plan (developers.beeper.com v4.2.808). The field
// names of a message body are unconfirmed until the B0 live /v1/spec check runs
// on a machine with Beeper Desktop reachable, so these tests assert the WIRING
// the client controls — method, path, query, bearer, decode into the struct —
// against a fake server whose JSON this test authors. B0 confirms the shape.

func TestListChatsRequestsTheUnreadPageAndDecodesUnreadCounts(t *testing.T) {
	var gotPath, gotUnread, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUnread = r.URL.Query().Get("unread")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[
			{"id":"c1","network":"Instagram","title":"Maya","unreadCount":2,"lastActivity":"2026-08-07T10:00:00Z"},
			{"id":"c2","network":"Discord","title":"Guild","unreadCount":5,"lastActivity":"2026-08-07T09:00:00Z"}
		]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok-abc"), server.Client(), discardLogger())
	chats, err := client.ListChats(context.Background(), ListChatsOptions{Unread: true, Limit: 20})
	if err != nil {
		t.Fatalf("ListChats returned an error: %v", err)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Fatalf("expected a bearer token header, got %q", gotAuth)
	}
	if gotPath != "/v1/chats" {
		t.Fatalf("expected /v1/chats, got %q", gotPath)
	}
	if gotUnread != "true" {
		t.Fatalf("expected ?unread=true, got %q", gotUnread)
	}
	if len(chats) != 2 || chats[0].UnreadCount != 2 || chats[0].LastActivity == "" {
		t.Fatalf("chats decoded wrong: %+v", chats)
	}
}

func TestListMessagesGetsAChatsMessagesAndDecodesSenderAndText(t *testing.T) {
	var gotPath, gotLimit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[
			{"id":"m1","chatID":"c1","senderName":"Maya","text":"you around?","timestamp":"2026-08-07T10:00:00Z","isSender":false},
			{"id":"m2","chatID":"c1","senderName":"You","text":"yes","timestamp":"2026-08-07T10:01:00Z","isSender":true}
		]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok-abc"), server.Client(), discardLogger())
	msgs, err := client.ListMessages(context.Background(), "c1", 5)
	if err != nil {
		t.Fatalf("ListMessages returned an error: %v", err)
	}
	if gotPath != "/v1/chats/c1/messages" {
		t.Fatalf("expected /v1/chats/c1/messages, got %q", gotPath)
	}
	if gotLimit != "5" {
		t.Fatalf("expected ?limit=5, got %q", gotLimit)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].SenderName != "Maya" || msgs[0].Text != "you around?" || msgs[0].IsSender {
		t.Fatalf("first message decoded wrong: %+v", msgs[0])
	}
	if !msgs[1].IsSender {
		t.Fatalf("own message should decode isSender=true: %+v", msgs[1])
	}
}

func TestListMessagesEscapesTheChatIDInThePath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if _, err := client.ListMessages(context.Background(), "acc!weird/id", 5); err != nil {
		t.Fatalf("ListMessages returned an error: %v", err)
	}
	if gotPath != "/v1/chats/acc%21weird%2Fid/messages" {
		t.Fatalf("chat id was not path-escaped, got %q", gotPath)
	}
}

func TestGetChatFetchesOneChatByID(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c1","network":"Instagram","title":"Maya","unreadCount":1}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	chat, err := client.GetChat(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetChat returned an error: %v", err)
	}
	if gotPath != "/v1/chats/c1" {
		t.Fatalf("expected /v1/chats/c1, got %q", gotPath)
	}
	if chat.ID != "c1" || chat.Title != "Maya" {
		t.Fatalf("chat decoded wrong: %+v", chat)
	}
}

func TestSearchMessagesSendsTheQueryAndDecodesResults(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[
			{"id":"m9","chatID":"c1","senderName":"Maya","text":"Friday works","timestamp":"2026-08-07T08:00:00Z","isSender":false}
		]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	msgs, err := client.SearchMessages(context.Background(), "Friday", 10)
	if err != nil {
		t.Fatalf("SearchMessages returned an error: %v", err)
	}
	if gotPath != "/v1/messages/search" {
		t.Fatalf("expected /v1/messages/search, got %q", gotPath)
	}
	if gotQuery != "Friday" {
		t.Fatalf("expected ?query=Friday, got %q", gotQuery)
	}
	if len(msgs) != 1 || msgs[0].Text != "Friday works" {
		t.Fatalf("messages decoded wrong: %+v", msgs)
	}
}

// Reads are safe on a read-only client: a read-only client must still answer
// every read. This guards against a future change that over-broadly blocks all
// traffic on the read-only flag.
func TestReadOnlyClientStillAnswersReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"id":"c1","network":"Instagram","title":"Maya","unreadCount":1}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger()).ReadOnly()
	chats, err := client.ListChats(context.Background(), ListChatsOptions{Unread: true, Limit: 20})
	if err != nil {
		t.Fatalf("a read-only client must still read: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("expected the read to return, got %+v", chats)
	}
}
