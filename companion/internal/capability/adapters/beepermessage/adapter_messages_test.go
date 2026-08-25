package beepermessage

// The thread UI renders each received DM as its own chat row: sender name,
// body, sent time. Today executeRead flattens everything into one Detail
// string ("raina: hi · Sam: call me"), so the structure the rows need is
// destroyed before it leaves the adapter. These tests make the read outcome
// carry the messages themselves alongside the human Detail sentence, as
// adapter.OutcomeMessage{Sender, Text, SentAt}.
//
// Beeper's live API returns timestamps as RFC3339 strings with fractional
// seconds ("2025-08-28T11:04:29.621Z" — developers.beeper.com desktop-api
// message schema). The wire's message entry requires a valid RFC3339 sentAt,
// so the adapter parses here at the boundary; a timestamp it cannot parse
// becomes a zero time, which the thread store must not turn into a row.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

func executeRead(t *testing.T, f *fakeBeeper, subject string) adapter.Outcome {
	t.Helper()
	a := igAdapter(f)
	plan := mustResolve(t, a, adapter.Intent{Verb: manifest.Read, Subject: subject})
	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	return out
}

func TestReadUnreadOutcomeCarriesStructuredMessages(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{
			{ID: "ig1", Network: "Instagram", Title: "raina", UnreadCount: 1},
			{ID: "ig2", Network: "Instagram", Title: "Skibidi sigma", UnreadCount: 1},
		},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "raina", Text: "are you free tonight?\ncall me", Timestamp: "2026-08-07T18:30:00.123Z", IsSender: false}},
			"ig2": {{ID: "m2", SenderName: "Skibidi sigma", Text: `<strong><a href="http://flamingo.app">flamingo.app</a> Take the quiz &amp; win 👇</strong>`, Timestamp: "2026-08-07T17:05:12Z", IsSender: false}},
		},
	}
	out := executeRead(t, f, "")

	if strings.TrimSpace(out.Detail) == "" {
		t.Fatal("the flat Detail sentence must survive; it is the fallback and the desktop rendering")
	}
	if len(out.Messages) != 2 {
		t.Fatalf("the outcome must carry one structured message per unread DM, got %d: %+v", len(out.Messages), out.Messages)
	}

	first := out.Messages[0]
	if first.Sender != "raina" {
		t.Fatalf("first message sender must be the display name, got %q", first.Sender)
	}
	if first.Text != "are you free tonight?\ncall me" {
		t.Fatalf("a multi-line body must survive with its newlines, got %q", first.Text)
	}
	want := time.Date(2026, 8, 7, 18, 30, 0, 123_000_000, time.UTC)
	if !first.SentAt.Equal(want) {
		t.Fatalf("SentAt must parse Beeper's fractional-second RFC3339, got %v want %v", first.SentAt, want)
	}

	second := out.Messages[1]
	for _, tag := range []string{"<strong", "<a ", "href=", "&amp;"} {
		if strings.Contains(second.Text, tag) {
			t.Fatalf("structured text still shows raw HTML %q: %q", tag, second.Text)
		}
	}
	for _, visible := range []string{"flamingo.app", "Take the quiz & win", "👇"} {
		if !strings.Contains(second.Text, visible) {
			t.Fatalf("structured text dropped visible content %q: %q", visible, second.Text)
		}
	}
}

func TestReadNamedConversationOutcomeCarriesStructuredMessages(t *testing.T) {
	f := &fakeBeeper{
		chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}},
		messages: map[string][]beeper.Message{
			"ig1": {
				{ID: "m1", SenderName: "Maya", Text: "dinner friday?", Timestamp: "2026-08-06T09:00:00Z", IsSender: false},
				{ID: "m2", SenderName: "You", Text: "sure", Timestamp: "2026-08-06T09:01:00Z", IsSender: true},
			},
		},
	}
	out := executeRead(t, f, "Maya")

	// The thread shows their side of the conversation ("without any text from
	// you, just from them" — the owner's spec), so the user's own sends stay
	// out of the structured rows.
	if len(out.Messages) != 1 {
		t.Fatalf("expected only Maya's message as a row, got %d: %+v", len(out.Messages), out.Messages)
	}
	if out.Messages[0].Sender != "Maya" || out.Messages[0].Text != "dinner friday?" {
		t.Fatalf("named read row must be the received message, got %+v", out.Messages[0])
	}
}

func TestReadUnparseableTimestampBecomesZeroTimeNotAGuess(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "raina", UnreadCount: 1}},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "raina", Text: "hi", Timestamp: "1754591112000", IsSender: false}},
		},
	}
	out := executeRead(t, f, "")
	if len(out.Messages) != 1 {
		t.Fatalf("the message itself must still be reported, got %d", len(out.Messages))
	}
	if !out.Messages[0].SentAt.IsZero() {
		t.Fatalf("an unparseable timestamp must become a zero time, not an invented one, got %v", out.Messages[0].SentAt)
	}
}

func TestReadSenderNameIsWireSafeSingleLine(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "weird", UnreadCount: 1}},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "ra\nina\t", Text: "hi", Timestamp: "2026-08-07T18:30:00Z", IsSender: false}},
		},
	}
	out := executeRead(t, f, "")
	if len(out.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(out.Messages))
	}
	// The wire's safeDisplayString rejects any control character in sender, so
	// a newline that slips through here kills the whole task_page frame.
	if hasControl(out.Messages[0].Sender) {
		t.Fatalf("sender must be a single control-free line, got %q", out.Messages[0].Sender)
	}
	if !strings.Contains(out.Messages[0].Sender, "ra") {
		t.Fatalf("sanitizing must not erase the name, got %q", out.Messages[0].Sender)
	}
}
