package beeper

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListChatsUsesAccountFilterAndReturnsUnreadFields(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"items":[
			{"id":"c1","accountID":"ig","network":"Instagram","title":"Maya","type":"single","unreadCount":3,"lastActivity":"2026-08-06T10:00:00Z"},
			{"id":"c2","accountID":"ig","network":"Instagram","title":"Raina","type":"single","unreadCount":0,"lastActivity":"2026-08-05T10:00:00Z"}
		],"hasMore":false,"oldestCursor":"old","newestCursor":"new"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	page, err := client.ListChats(context.Background(), ListChatsOptions{AccountIDs: []string{"ig"}})
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if gotPath != "/v1/chats" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.Contains(gotQuery, "accountIDs=ig") {
		t.Fatalf("expected accountIDs in query, got %q", gotQuery)
	}
	if len(page.Items) != 2 || page.Items[0].UnreadCount != 3 || page.Items[0].LastActivity == "" {
		t.Fatalf("page=%+v", page)
	}
	if page.HasMore || page.OldestCursor != "old" {
		t.Fatalf("pagination fields wrong: %+v", page)
	}
}

func TestGetChatReturnsCapabilities(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{
			"id":"c1","accountID":"ig","network":"Instagram","title":"Maya","type":"single","unreadCount":1,
			"capabilities":{"edit":2,"delete":2,"reply":2,"reaction":1,"archive":true,"markAsUnread":true}
		}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	chat, err := client.GetChat(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if gotPath != "/v1/chats/c1" {
		t.Fatalf("path=%q", gotPath)
	}
	if chat.Capabilities.Edit != 2 || !chat.Capabilities.Archive || !chat.Capabilities.MarkAsUnread {
		t.Fatalf("capabilities=%+v", chat.Capabilities)
	}
}

func TestListMessagesReturnsSenderFlagsAndAttachmentHints(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"items":[
			{"id":"m1","chatID":"c1","senderName":"Maya","text":"hello","isSender":false,"isUnread":true,"timestamp":"2026-08-06T12:00:00Z"},
			{"id":"m2","chatID":"c1","senderName":"me","text":"","isSender":true,"attachments":[{"type":"image"}],"timestamp":"2026-08-06T11:00:00Z"}
		],"hasMore":true,"oldestCursor":"o","newestCursor":"n"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	page, err := client.ListMessages(context.Background(), "c1", MessageListOptions{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if gotPath != "/v1/chats/c1/messages" {
		t.Fatalf("path=%q", gotPath)
	}
	if len(page.Items) != 2 || page.Items[0].IsSender || !page.Items[0].IsUnread {
		t.Fatalf("messages=%+v", page.Items)
	}
	if len(page.Items[1].Attachments) != 1 || page.Items[1].Attachments[0].Type != "image" {
		t.Fatalf("attachments=%+v", page.Items[1].Attachments)
	}
}

func TestSearchMessagesSendsQueryAndAccountFilter(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"items":[{"id":"m9","chatID":"c1","senderName":"Maya","text":"Friday dinner","isSender":false}],"hasMore":false}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	page, err := client.SearchMessages(context.Background(), SearchMessagesOptions{
		Query: "Friday", AccountIDs: []string{"ig"}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if gotPath != "/v1/messages/search" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.Contains(gotQuery, "query=Friday") || !strings.Contains(gotQuery, "accountIDs=ig") || !strings.Contains(gotQuery, "limit=10") {
		t.Fatalf("query=%q", gotQuery)
	}
	if len(page.Items) != 1 || page.Items[0].Text != "Friday dinner" {
		t.Fatalf("page=%+v", page)
	}
}

func TestSendWithReplyIncludesReplyToMessageID(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"chatID":"c1","pendingMessageID":"pm-1"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	sent, err := client.SendReply(context.Background(), "c1", "on my way", "m-incoming")
	if err != nil {
		t.Fatalf("SendReply: %v", err)
	}
	if gotBody["text"] != "on my way" || gotBody["replyToMessageID"] != "m-incoming" {
		t.Fatalf("body=%+v", gotBody)
	}
	if sent.PendingMessageID != "pm-1" {
		t.Fatalf("sent=%+v", sent)
	}
}

func TestEditMessagePutsNewText(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"id":"m1","chatID":"c1","text":"edited","isSender":true,"success":true}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	msg, err := client.EditMessage(context.Background(), "c1", "m1", "edited")
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/v1/chats/c1/messages/m1" {
		t.Fatalf("request=%s %s", gotMethod, gotPath)
	}
	if gotBody["text"] != "edited" || msg.Text != "edited" {
		t.Fatalf("body=%+v msg=%+v", gotBody, msg)
	}
}

func TestDeleteMessageAcceptsNoContent(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.DeleteMessage(context.Background(), "c1", "m1"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/v1/chats/c1/messages/m1" {
		t.Fatalf("request=%s %s", gotMethod, gotPath)
	}
}

func TestReactAndUnreactHitReactionPaths(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		_, _ = io.WriteString(w, `{"success":true,"chatID":"c1","messageID":"m1","reactionKey":"ok"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.React(context.Background(), "c1", "m1", "ok"); err != nil {
		t.Fatalf("React: %v", err)
	}
	if err := client.Unreact(context.Background(), "c1", "m1", "ok"); err != nil {
		t.Fatalf("Unreact: %v", err)
	}
	if len(calls) != 2 ||
		calls[0] != "POST /v1/chats/c1/messages/m1/reactions" ||
		calls[1] != "DELETE /v1/chats/c1/messages/m1/reactions/ok" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestMarkReadAndUnreadReturnChat(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		_, _ = io.WriteString(w, `{"id":"c1","network":"Instagram","title":"Maya","unreadCount":0}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	chat, err := client.MarkRead(context.Background(), "c1", "")
	if err != nil || chat.ID != "c1" {
		t.Fatalf("MarkRead chat=%+v err=%v", chat, err)
	}
	chat, err = client.MarkUnread(context.Background(), "c1", "")
	if err != nil || chat.ID != "c1" {
		t.Fatalf("MarkUnread chat=%+v err=%v", chat, err)
	}
	if calls[0] != "POST /v1/chats/c1/read" || calls[1] != "POST /v1/chats/c1/unread" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestArchivePostsArchivedFlagAndAcceptsNoContent(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.Archive(context.Background(), "c1", true); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if gotBody["archived"] != true {
		t.Fatalf("body=%+v", gotBody)
	}
}

func TestUpdateChatPatchesPinAndMute(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"id":"c1","isPinned":true,"isMuted":true}`)
	}))
	defer server.Close()

	pinned, muted := true, true
	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	chat, err := client.UpdateChat(context.Background(), "c1", UpdateChatOptions{Pinned: &pinned, Muted: &muted})
	if err != nil {
		t.Fatalf("UpdateChat: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/v1/chats/c1" {
		t.Fatalf("request=%s %s", gotMethod, gotPath)
	}
	if gotBody["isPinned"] != true || gotBody["isMuted"] != true || !chat.IsPinned {
		t.Fatalf("body=%+v chat=%+v", gotBody, chat)
	}
}

func TestSetAndClearReminder(t *testing.T) {
	var calls []string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	when := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	if err := client.SetReminder(context.Background(), "c1", when, true); err != nil {
		t.Fatalf("SetReminder: %v", err)
	}
	if err := client.ClearReminder(context.Background(), "c1"); err != nil {
		t.Fatalf("ClearReminder: %v", err)
	}
	if calls[0] != "POST /v1/chats/c1/reminders" || calls[1] != "DELETE /v1/chats/c1/reminders" {
		t.Fatalf("calls=%v", calls)
	}
	reminder, _ := gotBody["reminder"].(map[string]any)
	if reminder["remindAt"] != "2026-08-07T12:00:00Z" || reminder["dismissOnIncomingMessage"] != true {
		t.Fatalf("reminder body=%+v", gotBody)
	}
}

func TestReadOnlyClientRefusesEveryWriteOp(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger()).ReadOnly()
	ctx := context.Background()
	ops := []struct {
		name string
		fn   func() error
	}{
		{"SendReply", func() error { _, err := client.SendReply(ctx, "c1", "hi", "m1"); return err }},
		{"EditMessage", func() error { _, err := client.EditMessage(ctx, "c1", "m1", "x"); return err }},
		{"DeleteMessage", func() error { return client.DeleteMessage(ctx, "c1", "m1") }},
		{"React", func() error { return client.React(ctx, "c1", "m1", "👍") }},
		{"Unreact", func() error { return client.Unreact(ctx, "c1", "m1", "👍") }},
		{"MarkRead", func() error { _, err := client.MarkRead(ctx, "c1", ""); return err }},
		{"MarkUnread", func() error { _, err := client.MarkUnread(ctx, "c1", ""); return err }},
		{"Archive", func() error { return client.Archive(ctx, "c1", true) }},
		{"UpdateChat", func() error {
			pinned := true
			_, err := client.UpdateChat(ctx, "c1", UpdateChatOptions{Pinned: &pinned})
			return err
		}},
		{"SetReminder", func() error { return client.SetReminder(ctx, "c1", time.Now().UTC(), false) }},
		{"ClearReminder", func() error { return client.ClearReminder(ctx, "c1") }},
		{"StartChat", func() error { _, err := client.StartChat(ctx, "acct", "+15551212"); return err }},
	}
	for _, op := range ops {
		if err := op.fn(); !errors.Is(err, ErrReadOnly) {
			t.Fatalf("%s: want ErrReadOnly, got %v", op.name, err)
		}
	}
	if called {
		t.Fatal("read-only client must not open a connection for any write")
	}
}
