package beeper

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Workstream B1 (write side). The manage operations the adapter dispatches:
// reply, edit, delete, react/unreact, mark read/unread, archive, chat state
// (pin/mute), and reminders. Endpoint shapes are the plan's table, provisional
// until the B0 live /v1/spec check. These tests assert the WIRING the client
// controls (method, path, body, read-only refusal, id escaping) against a fake
// server whose responses this test authors. Most state ops answer 204 No
// Content, so the client must not try to decode an empty body.

// writeServer records the last request and answers 204 unless a body is set.
func writeServer(t *testing.T, status int, respBody string, capture *struct {
	Method, Path, Auth string
	Body               map[string]any
}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.Method = r.Method
		capture.Path = r.URL.EscapedPath()
		capture.Auth = r.Header.Get("Authorization")
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &capture.Body)
			}
		}
		if respBody != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, respBody)
			return
		}
		w.WriteHeader(status)
	}))
}

func TestReplyPostsToTheChatWithAReplyTarget(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusOK, `{"chatID":"c1","pendingMessageID":"p9"}`, &got)
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	sent, err := client.Reply(context.Background(), "c1", "m5", "on my way")
	if err != nil {
		t.Fatalf("Reply returned an error: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/chats/c1/messages" {
		t.Fatalf("reply wired wrong: %s %s", got.Method, got.Path)
	}
	if got.Body["text"] != "on my way" || got.Body["replyToMessageID"] != "m5" {
		t.Fatalf("reply body wrong: %+v", got.Body)
	}
	if sent.PendingMessageID != "p9" {
		t.Fatalf("reply should decode the pending id: %+v", sent)
	}
}

func TestEditMessagePutsTheNewText(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusNoContent, "", &got)
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.EditMessage(context.Background(), "c1", "m5", "fixed text"); err != nil {
		t.Fatalf("EditMessage returned an error: %v", err)
	}
	if got.Method != http.MethodPut || got.Path != "/v1/chats/c1/messages/m5" {
		t.Fatalf("edit wired wrong: %s %s", got.Method, got.Path)
	}
	if got.Body["text"] != "fixed text" {
		t.Fatalf("edit body wrong: %+v", got.Body)
	}
}

func TestDeleteMessageDeletesByID(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusNoContent, "", &got)
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.DeleteMessage(context.Background(), "c1", "m5"); err != nil {
		t.Fatalf("DeleteMessage returned an error: %v", err)
	}
	if got.Method != http.MethodDelete || got.Path != "/v1/chats/c1/messages/m5" {
		t.Fatalf("delete wired wrong: %s %s", got.Method, got.Path)
	}
}

func TestReactPostsTheEmojiKey(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusNoContent, "", &got)
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.React(context.Background(), "c1", "m5", "❤️"); err != nil {
		t.Fatalf("React returned an error: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/chats/c1/messages/m5/reactions" {
		t.Fatalf("react wired wrong: %s %s", got.Method, got.Path)
	}
	if got.Body["key"] != "❤️" {
		t.Fatalf("react body wrong: %+v", got.Body)
	}
}

func TestUnreactDeletesTheReactionKeyInThePath(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusNoContent, "", &got)
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.Unreact(context.Background(), "c1", "m5", "👍"); err != nil {
		t.Fatalf("Unreact returned an error: %v", err)
	}
	if got.Method != http.MethodDelete {
		t.Fatalf("unreact method wrong: %s", got.Method)
	}
	// The reaction key rides in the path and must be escaped.
	if got.Path != "/v1/chats/c1/messages/m5/reactions/%F0%9F%91%8D" {
		t.Fatalf("unreact path wrong: %s", got.Path)
	}
}

func TestMarkReadAndMarkUnreadHitTheirEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, wantPath string
		call           func(*Client) error
	}{
		{"read", "/v1/chats/c1/read", func(c *Client) error { return c.MarkRead(context.Background(), "c1") }},
		{"unread", "/v1/chats/c1/unread", func(c *Client) error { return c.MarkUnread(context.Background(), "c1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				Method, Path, Auth string
				Body               map[string]any
			}
			server := writeServer(t, http.StatusNoContent, "", &got)
			defer server.Close()

			client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
			if err := tc.call(client); err != nil {
				t.Fatalf("%s returned an error: %v", tc.name, err)
			}
			if got.Method != http.MethodPost || got.Path != tc.wantPath {
				t.Fatalf("%s wired wrong: %s %s", tc.name, got.Method, got.Path)
			}
		})
	}
}

func TestArchiveSendsTheArchivedFlag(t *testing.T) {
	for _, tc := range []struct {
		name     string
		archived bool
	}{{"archive", true}, {"unarchive", false}} {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				Method, Path, Auth string
				Body               map[string]any
			}
			server := writeServer(t, http.StatusNoContent, "", &got)
			defer server.Close()

			client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
			if err := client.Archive(context.Background(), "c1", tc.archived); err != nil {
				t.Fatalf("Archive returned an error: %v", err)
			}
			if got.Method != http.MethodPost || got.Path != "/v1/chats/c1/archive" {
				t.Fatalf("archive wired wrong: %s %s", got.Method, got.Path)
			}
			if got.Body["archived"] != tc.archived {
				t.Fatalf("archive flag wrong: %+v", got.Body)
			}
		})
	}
}

func TestUpdateChatPatchesPinAndMute(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusNoContent, "", &got)
	defer server.Close()

	pinned, muted := true, true
	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.UpdateChat(context.Background(), "c1", ChatState{Pinned: &pinned, Muted: &muted}); err != nil {
		t.Fatalf("UpdateChat returned an error: %v", err)
	}
	if got.Method != http.MethodPatch || got.Path != "/v1/chats/c1" {
		t.Fatalf("update wired wrong: %s %s", got.Method, got.Path)
	}
	if got.Body["pinned"] != true || got.Body["muted"] != true {
		t.Fatalf("update body wrong: %+v", got.Body)
	}
}

func TestUpdateChatOmitsUnsetFields(t *testing.T) {
	var got struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	server := writeServer(t, http.StatusNoContent, "", &got)
	defer server.Close()

	pinned := true
	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
	if err := client.UpdateChat(context.Background(), "c1", ChatState{Pinned: &pinned}); err != nil {
		t.Fatalf("UpdateChat returned an error: %v", err)
	}
	if _, present := got.Body["muted"]; present {
		t.Fatalf("an unset field must not be sent: %+v", got.Body)
	}
}

func TestSetAndClearReminderHitTheReminderEndpoint(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		var got struct {
			Method, Path, Auth string
			Body               map[string]any
		}
		server := writeServer(t, http.StatusNoContent, "", &got)
		defer server.Close()

		client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
		if err := client.SetReminder(context.Background(), "c1", "2026-08-08T09:00:00Z"); err != nil {
			t.Fatalf("SetReminder returned an error: %v", err)
		}
		if got.Method != http.MethodPost || got.Path != "/v1/chats/c1/reminders" {
			t.Fatalf("set reminder wired wrong: %s %s", got.Method, got.Path)
		}
		if got.Body["remindAt"] != "2026-08-08T09:00:00Z" {
			t.Fatalf("reminder body wrong: %+v", got.Body)
		}
	})
	t.Run("clear", func(t *testing.T) {
		var got struct {
			Method, Path, Auth string
			Body               map[string]any
		}
		server := writeServer(t, http.StatusNoContent, "", &got)
		defer server.Close()

		client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger())
		if err := client.ClearReminder(context.Background(), "c1"); err != nil {
			t.Fatalf("ClearReminder returned an error: %v", err)
		}
		if got.Method != http.MethodDelete || got.Path != "/v1/chats/c1/reminders" {
			t.Fatalf("clear reminder wired wrong: %s %s", got.Method, got.Path)
		}
	})
}

// Every write must refuse on a read-only client, exactly like Send. A read-only
// client is the phone's fail-safe when the user has not enabled writes, so a new
// write method that forgets the guard would be a silent hole.
func TestEveryWriteRefusesOnAReadOnlyClient(t *testing.T) {
	pinned := true
	writes := map[string]func(*Client) error{
		"Reply":         func(c *Client) error { _, err := c.Reply(context.Background(), "c1", "m5", "hi"); return err },
		"EditMessage":   func(c *Client) error { return c.EditMessage(context.Background(), "c1", "m5", "x") },
		"DeleteMessage": func(c *Client) error { return c.DeleteMessage(context.Background(), "c1", "m5") },
		"React":         func(c *Client) error { return c.React(context.Background(), "c1", "m5", "❤️") },
		"Unreact":       func(c *Client) error { return c.Unreact(context.Background(), "c1", "m5", "❤️") },
		"MarkRead":      func(c *Client) error { return c.MarkRead(context.Background(), "c1") },
		"MarkUnread":    func(c *Client) error { return c.MarkUnread(context.Background(), "c1") },
		"Archive":       func(c *Client) error { return c.Archive(context.Background(), "c1", true) },
		"UpdateChat":    func(c *Client) error { return c.UpdateChat(context.Background(), "c1", ChatState{Pinned: &pinned}) },
		"SetReminder":   func(c *Client) error { return c.SetReminder(context.Background(), "c1", "2026-08-08T09:00:00Z") },
		"ClearReminder": func(c *Client) error { return c.ClearReminder(context.Background(), "c1") },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a read-only client reached the network: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client := NewClient(server.URL, staticToken("tok"), server.Client(), discardLogger()).ReadOnly()
	for name, call := range writes {
		if err := call(client); err != ErrReadOnly {
			t.Errorf("%s on a read-only client returned %v, want ErrReadOnly", name, err)
		}
	}
}
