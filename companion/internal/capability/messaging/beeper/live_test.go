package beeper

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// Everything in client_test.go points at an httptest server, so it passes
// whether or not a real Beeper would accept a single one of these calls. That
// is precisely how this repo's OAuth tests stayed green for months against
// credentials nobody had ever checked. These tests talk to a real Beeper.
//
// Run: BEEPER_LIVE=1 go test ./internal/capability/messaging/beeper/ -run Live -v
// with BEEPER_ACCESS_TOKEN set (it is already in the repo's .env).
//
// Read-only by construction: nothing here sends. Sending is the owner's call.

const liveGate = "BEEPER_LIVE"

func requireLive(t *testing.T, untested string) (*Client, context.Context) {
	t.Helper()
	if os.Getenv(liveGate) != "1" {
		t.Skipf("%s=1 not set, so this is UNTESTED: %s", liveGate, untested)
	}
	token := os.Getenv("BEEPER_ACCESS_TOKEN")
	if token == "" {
		t.Fatalf("%s=1 but BEEPER_ACCESS_TOKEN is empty; export it from .env", liveGate)
	}
	baseURL := os.Getenv("BEEPER_DESKTOP_BASE_URL")
	client := NewClient(baseURL, StaticToken(token), &http.Client{Timeout: 20 * time.Second}, discardLogger()).ReadOnly()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return client, ctx
}

// The one that matters: a real Beeper accepts our token and our request shape,
// and hands back chats we can actually address.
func TestLiveBeeperReturnsAddressableChats(t *testing.T) {
	client, ctx := requireLive(t, "whether a real Beeper accepts this client's token and search request at all")

	chats, err := client.SearchChats(ctx, "")
	if err != nil {
		t.Fatalf("live Beeper rejected the search: %v\n"+
			"Is Beeper Desktop or Beeper Server running, and is BEEPER_ACCESS_TOKEN current?", err)
	}
	if len(chats) == 0 {
		t.Fatal("live Beeper returned no chats at all; expected at least one bridged conversation")
	}
	networks := map[string]int{}
	for _, chat := range chats {
		if chat.ID == "" {
			t.Fatalf("a chat came back with no id, so nothing could be sent to it: %+v", chat)
		}
		networks[chat.Network]++
	}
	t.Logf("live Beeper returned %d chats across %d networks", len(chats), len(networks))
	for network, count := range networks {
		t.Logf("  %s: %d", network, count)
	}
}

// Read smoke for B1: list inbox + messages against a real Desktop.
func TestLiveBeeperListsChatsAndMessages(t *testing.T) {
	client, ctx := requireLive(t, "whether ListChats/ListMessages match live Desktop /v1/spec shapes")

	page, err := client.ListChats(ctx, ListChatsOptions{})
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("ListChats returned no chats")
	}
	chat := page.Items[0]
	if chat.ID == "" {
		t.Fatalf("chat missing id: %+v", chat)
	}
	msgs, err := client.ListMessages(ctx, chat.ID, MessageListOptions{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	t.Logf("listed %d chats (hasMore=%v); chat %q had %d messages (hasMore=%v)",
		len(page.Items), page.HasMore, chat.Title, len(msgs.Items), msgs.HasMore)
}

// The safety guard has to hold against the real thing, not just a fake one.
func TestLiveReadOnlyClientStillRefusesToSend(t *testing.T) {
	client, ctx := requireLive(t, "whether the read-only guard actually protects a live Beeper account")

	chats, err := client.SearchChats(ctx, "")
	if err != nil {
		t.Fatalf("live Beeper rejected the search: %v", err)
	}
	if len(chats) == 0 {
		t.Skip("no chats to aim the guard at")
	}
	_, err = client.Send(ctx, chats[0].ID, "this must never leave the process")
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only guard did not hold against a live Beeper: %v", err)
	}
}
