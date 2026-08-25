package beepermessage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

// hasControl mirrors the mobile wire contract's safeDisplayString rule: it
// rejects any run of control characters (newline, tab, carriage return, DEL).
// The contract applies it to every capability_preview line and to the whole
// capability_result detail, so an adapter that lets a control char through
// produces a frame the phone silently drops, killing the session.
func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// Workstream B2. The Beeper adapter grows from send-only to read + manage. A
// "read" with an empty subject is the motivating bug ("what's my most recent
// unread Instagram message" must return messages, not error on an empty
// recipient). "Manage" verbs (reply, edit, delete, react, mark read, archive,
// reminders) dispatch on the stage-1 operation field, pin their target message
// at resolve time by recency, and map their outcome per operation: only a
// reply is delivery-pending, every state op is a plain confirmed outcome.

// --- fake read/manage surface (fields live on fakeBeeper in adapter_test.go) ---

func (f *fakeBeeper) ListChats(_ context.Context, _ beeper.ListChatsOptions) ([]beeper.Chat, error) {
	return append([]beeper.Chat(nil), f.unreadChats...), nil
}

func (f *fakeBeeper) ListMessages(_ context.Context, chatID string, _ int) ([]beeper.Message, error) {
	return append([]beeper.Message(nil), f.messages[chatID]...), nil
}

func (f *fakeBeeper) Reply(_ context.Context, chatID, replyToMessageID, text string) (beeper.Sent, error) {
	f.calls = append(f.calls, fmt.Sprintf("Reply %s %s %s", chatID, replyToMessageID, text))
	return beeper.Sent{ChatID: chatID, PendingMessageID: "pending-reply"}, nil
}

func (f *fakeBeeper) EditMessage(_ context.Context, chatID, messageID, text string) error {
	f.calls = append(f.calls, fmt.Sprintf("Edit %s %s %s", chatID, messageID, text))
	return nil
}

func (f *fakeBeeper) DeleteMessage(_ context.Context, chatID, messageID string) error {
	f.calls = append(f.calls, fmt.Sprintf("Delete %s %s", chatID, messageID))
	return nil
}

func (f *fakeBeeper) React(_ context.Context, chatID, messageID, key string) error {
	f.calls = append(f.calls, fmt.Sprintf("React %s %s %s", chatID, messageID, key))
	return nil
}

func (f *fakeBeeper) Unreact(_ context.Context, chatID, messageID, key string) error {
	f.calls = append(f.calls, fmt.Sprintf("Unreact %s %s %s", chatID, messageID, key))
	return nil
}

func (f *fakeBeeper) MarkRead(_ context.Context, chatID string) error {
	f.calls = append(f.calls, "MarkRead "+chatID)
	return nil
}

func (f *fakeBeeper) MarkUnread(_ context.Context, chatID string) error {
	f.calls = append(f.calls, "MarkUnread "+chatID)
	return nil
}

func (f *fakeBeeper) Archive(_ context.Context, chatID string, archived bool) error {
	f.calls = append(f.calls, fmt.Sprintf("Archive %s %t", chatID, archived))
	return nil
}

func boolPtrText(p *bool) string {
	if p == nil {
		return "<nil>"
	}
	return strconv.FormatBool(*p)
}

func (f *fakeBeeper) UpdateChat(_ context.Context, chatID string, state beeper.ChatState) error {
	f.calls = append(f.calls, fmt.Sprintf("UpdateChat %s pinned=%s muted=%s", chatID, boolPtrText(state.Pinned), boolPtrText(state.Muted)))
	return nil
}

func (f *fakeBeeper) SetReminder(_ context.Context, chatID, remindAt string) error {
	f.calls = append(f.calls, fmt.Sprintf("SetReminder %s %s", chatID, remindAt))
	return nil
}

func (f *fakeBeeper) ClearReminder(_ context.Context, chatID string) error {
	f.calls = append(f.calls, "ClearReminder "+chatID)
	return nil
}

// --- helpers ---

func igAdapter(f *fakeBeeper) *Adapter {
	return New(Spec{ID: "instagram", Network: "Instagram"}, f, testLogger())
}

func mustResolve(t *testing.T, a *Adapter, in adapter.Intent) adapter.Plan {
	t.Helper()
	plan, err := a.Resolve(context.Background(), in)
	if err != nil {
		t.Fatalf("Resolve returned an error: %v", err)
	}
	return plan
}

func previewText(t *testing.T, a *Adapter, plan adapter.Plan) string {
	t.Helper()
	pv, err := a.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("Preview returned an error: %v", err)
	}
	for _, line := range pv.Lines {
		if len(line) > 1024 {
			t.Fatalf("a preview line exceeds the 1024-char codec limit: %d chars", len(line))
		}
	}
	if len(pv.Lines) > 8 {
		t.Fatalf("preview has %d lines, over the 8-line codec limit", len(pv.Lines))
	}
	return pv.Headline + "\n" + strings.Join(pv.Lines, "\n")
}

// --- manifest ---

func TestManifestDeclaresReadSendModifyCancel(t *testing.T) {
	m := igAdapter(&fakeBeeper{}).Describe()
	want := []manifest.Verb{manifest.Read, manifest.Send, manifest.Modify, manifest.Cancel}
	for _, v := range want {
		if !m.Allows(v) {
			t.Fatalf("manifest must declare verb %q, has %v", v, m.Verbs)
		}
	}
}

// --- reads ---

// The motivating bug: an unread ask names no conversation, so subject is empty.
// The old adapter returned ErrNoRecipient; the new one runs a network-wide
// unread scan and the preview carries the actual message text.
func TestReadEmptySubjectScansTheNetworksUnread(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{
			{ID: "ig1", Network: "Instagram", Title: "Maya", UnreadCount: 2},
			{ID: "dc1", Network: "Discord", Title: "Guild", UnreadCount: 9},
			{ID: "ig2", Network: "Instagram", Title: "Sam", UnreadCount: 1},
		},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "Maya", Text: "you around?", IsSender: false}},
			"ig2": {{ID: "m2", SenderName: "Sam", Text: "call me back", IsSender: false}},
		},
	}
	plan := mustResolve(t, igAdapter(f), adapter.Intent{Verb: manifest.Read, Subject: ""})
	text := previewText(t, igAdapter(f), plan)
	if !strings.Contains(text, "you around?") || !strings.Contains(text, "call me back") {
		t.Fatalf("unread scan preview should carry both Instagram messages, got:\n%s", text)
	}
	// The Discord unread must not leak into an Instagram adapter's scan.
	if strings.Contains(text, "Guild") {
		t.Fatalf("scan leaked a Discord chat into the Instagram adapter:\n%s", text)
	}
}

// Beeper returns rich message bodies as HTML. A person reading "what's my
// unread Instagram message" expects the words, not "<strong><a href=...>". The
// preview line must carry the visible text with the markup gone and HTML
// entities decoded.
func TestReadStripsHtmlFromMessageText(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "raina", UnreadCount: 1}},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "raina", Text: `<strong><a href="http://flamingo.app">flamingo.app</a> Take the quiz &amp; win 👇</strong>`}},
		},
	}
	text := previewText(t, igAdapter(f), mustResolve(t, igAdapter(f), adapter.Intent{Verb: manifest.Read}))
	for _, tag := range []string{"<strong", "<a ", "href=", "</a>", "&amp;"} {
		if strings.Contains(text, tag) {
			t.Fatalf("preview still shows raw HTML %q:\n%s", tag, text)
		}
	}
	for _, want := range []string{"flamingo.app", "Take the quiz & win", "👇"} {
		if !strings.Contains(text, want) {
			t.Fatalf("preview dropped visible text %q:\n%s", want, text)
		}
	}
}

func TestReadEmptySubjectIsNoLongerANoRecipientError(t *testing.T) {
	f := &fakeBeeper{}
	_, err := igAdapter(f).Resolve(context.Background(), adapter.Intent{Verb: manifest.Read, Subject: ""})
	if errors.Is(err, ErrNoRecipient) {
		t.Fatalf("an empty subject on a read must not be ErrNoRecipient: %v", err)
	}
}

func TestReadNamedConversationListsItsRecentMessages(t *testing.T) {
	f := &fakeBeeper{
		chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}},
		messages: map[string][]beeper.Message{
			"ig1": {
				{ID: "m1", SenderName: "Maya", Text: "dinner friday?", IsSender: false},
				{ID: "m2", SenderName: "You", Text: "sure", IsSender: true},
			},
		},
	}
	plan := mustResolve(t, igAdapter(f), adapter.Intent{Verb: manifest.Read, Subject: "Maya"})
	text := previewText(t, igAdapter(f), plan)
	if !strings.Contains(text, "dinner friday?") {
		t.Fatalf("named read should show the conversation's messages, got:\n%s", text)
	}
}

// A read that finds nothing must still hand back a non-empty, human sentence.
// The phone's wire contract rejects a capability_result whose detail is empty
// (safeDisplayString needs >=1 non-space rune), and a rejected result is
// journaled unacked, so it replays and fails on every reconnect — one empty
// read bricks the whole session. "what's my most recent unread Instagram
// message" with an empty inbox is exactly this path.
func TestReadEmptyUnreadStillHasSendableDetail(t *testing.T) {
	f := &fakeBeeper{}
	a := igAdapter(f)
	plan := mustResolve(t, a, adapter.Intent{Verb: manifest.Read, Subject: ""})
	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if strings.TrimSpace(out.Detail) == "" {
		t.Fatalf("a completed read with no unread must carry a non-empty detail, got %q", out.Detail)
	}
	if !strings.Contains(strings.ToLower(out.Detail), "no unread") || !strings.Contains(out.Detail, "Instagram") {
		t.Fatalf("empty-unread detail should say there are no unread Instagram messages, got %q", out.Detail)
	}
}

func TestReadNamedEmptyThreadStillHasSendableDetail(t *testing.T) {
	f := &fakeBeeper{chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}}}
	a := igAdapter(f)
	plan := mustResolve(t, a, adapter.Intent{Verb: manifest.Read, Subject: "Maya"})
	out, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if strings.TrimSpace(out.Detail) == "" {
		t.Fatalf("a completed named read with no messages must carry a non-empty detail, got %q", out.Detail)
	}
}

// A real Instagram DM often spans multiple lines. The message text then
// carries a literal newline, which is a control character. Rendered straight
// into a preview line it fails the wire contract's safeDisplayString check,
// the phone drops the frame, and the session dies. Every rendered line must be
// a single control-free line, with the message content preserved.
func TestReadPreviewLinesCarryNoControlCharacters(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya", UnreadCount: 1}},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "Maya", Text: "hey\nare you\tfree tonight?\r\ncall me", IsSender: false}},
		},
	}
	plan := mustResolve(t, igAdapter(f), adapter.Intent{Verb: manifest.Read, Subject: ""})
	pv, err := igAdapter(f).Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("Preview returned an error: %v", err)
	}
	for i, line := range pv.Lines {
		if hasControl(line) {
			t.Fatalf("preview line %d carries a control character: %q", i, line)
		}
	}
	joined := strings.Join(pv.Lines, " ")
	for _, want := range []string{"hey", "are you", "free tonight?", "call me"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("sanitized preview dropped message content %q, got:\n%s", want, joined)
		}
	}
}

// The capability_result detail is a single safeDisplayString, not a list, so it
// must not contain the newlines that separate a multi-message render. Any
// two-message unread scan otherwise ships a detail full of newlines and fails
// the contract exactly like the preview does.
func TestReadDetailIsSingleControlFreeLine(t *testing.T) {
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{
			{ID: "ig1", Network: "Instagram", Title: "Maya", UnreadCount: 1},
			{ID: "ig2", Network: "Instagram", Title: "Sam", UnreadCount: 1},
		},
		messages: map[string][]beeper.Message{
			"ig1": {{ID: "m1", SenderName: "Maya", Text: "you around?", IsSender: false}},
			"ig2": {{ID: "m2", SenderName: "Sam", Text: "call me back", IsSender: false}},
		},
	}
	plan := mustResolve(t, igAdapter(f), adapter.Intent{Verb: manifest.Read, Subject: ""})
	out, err := igAdapter(f).Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if hasControl(out.Detail) {
		t.Fatalf("result detail carries a control character: %q", out.Detail)
	}
	if !strings.Contains(out.Detail, "you around?") || !strings.Contains(out.Detail, "call me back") {
		t.Fatalf("detail should carry both messages on one line, got: %q", out.Detail)
	}
}

// The wire contract rejects a capability_preview whose confirmLabel is empty
// or whose headline carries a control character or runs over 256 runes. A read
// preview that omits the confirm label ships an unsendable frame and kills the
// session, so both read shapes must carry a safe label and a safe headline
// even when the conversation title is nasty.
func TestReadPreviewCarriesASafeConfirmLabelAndHeadline(t *testing.T) {
	cases := []struct {
		name    string
		f       *fakeBeeper
		subject string
	}{
		{
			"unread",
			&fakeBeeper{
				unreadChats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya", UnreadCount: 1}},
				messages:    map[string][]beeper.Message{"ig1": {{ID: "m1", SenderName: "Maya", Text: "hi", IsSender: false}}},
			},
			"",
		},
		{
			"named-nasty-title",
			&fakeBeeper{
				chats:    []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya\nSmith" + strings.Repeat("x", 400)}},
				messages: map[string][]beeper.Message{"ig1": {{ID: "m1", SenderName: "Maya", Text: "hi", IsSender: false}}},
			},
			"Maya",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := mustResolve(t, igAdapter(tc.f), adapter.Intent{Verb: manifest.Read, Subject: tc.subject})
			pv, err := igAdapter(tc.f).Preview(context.Background(), plan)
			if err != nil {
				t.Fatalf("Preview returned an error: %v", err)
			}
			if strings.TrimSpace(pv.Confirm) == "" {
				t.Fatalf("a read preview must carry a confirm label")
			}
			if hasControl(pv.Confirm) || utf8.RuneCountInString(pv.Confirm) > 64 {
				t.Fatalf("confirm label is not a safe display string: %q", pv.Confirm)
			}
			if hasControl(pv.Headline) || utf8.RuneCountInString(pv.Headline) > 256 || strings.TrimSpace(pv.Headline) == "" {
				t.Fatalf("headline is not a safe display string: %q", pv.Headline)
			}
		})
	}
}

func TestReadTruncatesOversizedMessagesWithinCodecLimits(t *testing.T) {
	huge := strings.Repeat("x", 5000)
	msgs := make([]beeper.Message, 0, 40)
	for i := 0; i < 40; i++ {
		msgs = append(msgs, beeper.Message{ID: fmt.Sprintf("m%d", i), SenderName: "Maya", Text: huge, IsSender: false})
	}
	f := &fakeBeeper{
		chats:    []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}},
		messages: map[string][]beeper.Message{"ig1": msgs},
	}
	plan := mustResolve(t, igAdapter(f), adapter.Intent{Verb: manifest.Read, Subject: "Maya"})
	// previewText already fails if any line > 1024 chars or there are > 8 lines.
	_ = previewText(t, igAdapter(f), plan)
}

// --- manage: targeting ---

// A three-message thread: incoming, own, then a newer incoming. reply and react
// target the most recent INCOMING; edit and delete target the most recent OWN.
func threadFake() *fakeBeeper {
	return &fakeBeeper{
		chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}},
		messages: map[string][]beeper.Message{
			"ig1": {
				{ID: "in1", SenderName: "Maya", Text: "hi", IsSender: false},
				{ID: "own1", SenderName: "You", Text: "hello", IsSender: true},
				{ID: "in2", SenderName: "Maya", Text: "you there?", IsSender: false},
			},
		},
	}
}

func manageIntent(operation, subject, body string) adapter.Intent {
	return adapter.Intent{Verb: manifest.Modify, Subject: subject, Body: body, Fields: map[string]string{"operation": operation}}
}

func TestReplyTargetsMostRecentIncomingAndExecutes(t *testing.T) {
	f := threadFake()
	a := igAdapter(f)
	plan := mustResolve(t, a, manageIntent("reply", "Maya", "on my way"))
	if plan.Details["message_id"] != "in2" {
		t.Fatalf("reply should target the most recent incoming (in2), got %q", plan.Details["message_id"])
	}
	if _, err := a.Execute(context.Background(), plan); err == nil {
		t.Fatalf("a reply is delivery-pending and must return an outcome-unknown error")
	}
	if len(f.calls) != 1 || f.calls[0] != "Reply ig1 in2 on my way" {
		t.Fatalf("reply dispatched wrong: %v", f.calls)
	}
}

func TestEditTargetsOwnLatest(t *testing.T) {
	f := threadFake()
	a := igAdapter(f)
	plan := mustResolve(t, a, manageIntent("edit", "Maya", "hello!!"))
	if plan.Details["message_id"] != "own1" {
		t.Fatalf("edit should target own-latest (own1), got %q", plan.Details["message_id"])
	}
	if _, err := a.Execute(context.Background(), plan); err != nil {
		t.Fatalf("edit should be a confirmed outcome, got error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "Edit ig1 own1 hello!!" {
		t.Fatalf("edit dispatched wrong: %v", f.calls)
	}
}

func TestDeleteTargetsOwnLatest(t *testing.T) {
	f := threadFake()
	a := igAdapter(f)
	plan := mustResolve(t, a, adapter.Intent{Verb: manifest.Cancel, Subject: "Maya", Fields: map[string]string{"operation": "delete"}})
	if _, err := a.Execute(context.Background(), plan); err != nil {
		t.Fatalf("delete should be a confirmed outcome, got error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "Delete ig1 own1" {
		t.Fatalf("delete dispatched wrong: %v", f.calls)
	}
}

// --- manage: chat-level ops and outcome mapping ---

func TestStateOpsAreConfirmedNotPending(t *testing.T) {
	for _, tc := range []struct {
		op, verb, wantCall string
		cancel             bool
	}{
		{op: "mark_read", wantCall: "MarkRead ig1"},
		{op: "mark_unread", wantCall: "MarkUnread ig1"},
		{op: "archive", wantCall: "Archive ig1 true"},
		{op: "unarchive", wantCall: "Archive ig1 false"},
		{op: "pin", wantCall: "UpdateChat ig1 pinned=true muted=<nil>"},
	} {
		t.Run(tc.op, func(t *testing.T) {
			f := &fakeBeeper{chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}}}
			a := igAdapter(f)
			plan := mustResolve(t, a, manageIntent(tc.op, "Maya", ""))
			out, err := a.Execute(context.Background(), plan)
			if err != nil {
				t.Fatalf("%s should be confirmed, got error: %v", tc.op, err)
			}
			if !out.Done {
				t.Fatalf("%s outcome should be Done", tc.op)
			}
			if len(f.calls) != 1 || f.calls[0] != tc.wantCall {
				t.Fatalf("%s dispatched wrong: %v", tc.op, f.calls)
			}
		})
	}
}

func TestSetReminderDispatchesFromBody(t *testing.T) {
	f := &fakeBeeper{chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}}}
	a := igAdapter(f)
	plan := mustResolve(t, a, manageIntent("set_reminder", "Maya", "2026-08-08T09:00:00Z"))
	if _, err := a.Execute(context.Background(), plan); err != nil {
		t.Fatalf("set_reminder should be confirmed, got error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "SetReminder ig1 2026-08-08T09:00:00Z" {
		t.Fatalf("set_reminder dispatched wrong: %v", f.calls)
	}
}

// --- manage: refusals ---

func TestUnknownOperationAsksForClarification(t *testing.T) {
	f := &fakeBeeper{chats: []beeper.Chat{{ID: "ig1", Network: "Instagram", Title: "Maya"}}}
	_, err := igAdapter(f).Resolve(context.Background(), manageIntent("frobnicate", "Maya", ""))
	var clar *adapter.ClarificationError
	if !errors.As(err, &clar) {
		t.Fatalf("an unknown operation should ask for clarification, got %v", err)
	}
}

func TestManageAmbiguousConversationAsks(t *testing.T) {
	f := &fakeBeeper{chats: []beeper.Chat{
		{ID: "ig1", Network: "Instagram", Title: "Maya A"},
		{ID: "ig2", Network: "Instagram", Title: "Maya B"},
	}}
	_, err := igAdapter(f).Resolve(context.Background(), manageIntent("reply", "Maya", "hi"))
	var clar *adapter.ClarificationError
	if !errors.As(err, &clar) {
		t.Fatalf("two matching conversations should ask, got %v", err)
	}
}

func TestReplyWithEmptyBodyIsRejected(t *testing.T) {
	f := threadFake()
	_, err := igAdapter(f).Resolve(context.Background(), manageIntent("reply", "Maya", ""))
	if !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("a reply with no text should be ErrEmptyMessage, got %v", err)
	}
}
