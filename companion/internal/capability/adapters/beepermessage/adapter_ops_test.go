package beepermessage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

func TestManifestOffersReadSendModifyCancel(t *testing.T) {
	a := New(Spec{ID: "instagram", Network: "Instagram"}, &fakeBeeper{}, testLogger())
	m := a.Describe()
	want := map[manifest.Verb]bool{manifest.Read: true, manifest.Send: true, manifest.Modify: true, manifest.Cancel: true}
	if len(m.Verbs) != 4 {
		t.Fatalf("verbs=%v", m.Verbs)
	}
	for _, v := range m.Verbs {
		if !want[v] {
			t.Fatalf("unexpected verb %q in %v", v, m.Verbs)
		}
	}
}

// Fact-force (edit): callers=go test beepermessage; schemas=none;
// user: "Continue OpenAI+Beeper — **SLICE 4: B4 + B5**."
func TestReadOnlyManifestOffersReadVerbOnly(t *testing.T) {
	a := NewReadOnlyWithRevoke(Spec{ID: "instagram", Network: "Instagram"}, &fakeBeeper{}, nil, testLogger())
	m := a.Describe()
	if len(m.Verbs) != 1 || m.Verbs[0] != manifest.Read {
		t.Fatalf("read-only verbs=%v, want [read]", m.Verbs)
	}
}

func TestUnreadAskWithEmptySubjectReturnsNewestUnreadMessages(t *testing.T) {
	api := &fakeBeeper{
		chatPages: []beeper.ChatPage{{
			Items: []beeper.Chat{
				{ID: "old", Network: "Instagram", Title: "Old", UnreadCount: 2, LastActivity: "2026-08-01T00:00:00Z"},
				{ID: "new", Network: "Instagram", Title: "Maya", UnreadCount: 1, LastActivity: "2026-08-06T12:00:00Z"},
				{ID: "zero", Network: "Instagram", Title: "Zero", UnreadCount: 0, LastActivity: "2026-08-06T13:00:00Z"},
				{ID: "other", Network: "Discord", Title: "Maya", UnreadCount: 9, LastActivity: "2026-08-06T14:00:00Z"},
			},
		}},
		messagesByChat: map[string][]beeper.Message{
			"new": {
				{ID: "m1", SenderName: "Maya", Text: "are you free?", IsSender: false, IsUnread: true},
			},
			"old": {
				{ID: "o1", SenderName: "Old", Text: "older unread", IsSender: false, IsUnread: true},
			},
		},
	}
	a := New(Spec{ID: "instagram", Network: "Instagram"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{AdapterID: "instagram", Verb: manifest.Read})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	preview, err := a.Preview(t.Context(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	joined := strings.Join(preview.Lines, "\n")
	if !strings.Contains(joined, "Maya:") || !strings.Contains(joined, "are you free?") {
		t.Fatalf("preview missing newest unread text: %v", preview.Lines)
	}
	if strings.Contains(joined, "Old:") && strings.Index(joined, "Maya:") > strings.Index(joined, "Old:") {
		t.Fatalf("expected Maya (newer) before Old, got %v", preview.Lines)
	}
	outcome, err := a.Execute(t.Context(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Done || outcome.Detail == "" {
		t.Fatalf("read outcome should be confirmed with detail, got %+v", outcome)
	}
}

func TestNamedConversationReadListsRecentMessages(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Instagram", Title: "Maya"}},
		messagesByChat: map[string][]beeper.Message{
			"c1": {
				{ID: "m2", SenderName: "me", Text: "hey", IsSender: true},
				{ID: "m1", SenderName: "Maya", Text: "hi back", IsSender: false},
			},
		},
	}
	a := New(Spec{ID: "instagram", Network: "Instagram"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "instagram", Verb: manifest.Read, Subject: "Maya",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Handle != "c1" {
		t.Fatalf("handle=%q", plan.Handle)
	}
	preview, err := a.Preview(t.Context(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !strings.Contains(strings.Join(preview.Lines, "\n"), "Maya: hi back") {
		t.Fatalf("preview=%v", preview.Lines)
	}
}

func TestSearchReadUsesMessageSearch(t *testing.T) {
	api := &fakeBeeper{
		searchResults: []beeper.Message{
			{ID: "m9", ChatID: "c1", SenderName: "Maya", Text: "Friday dinner", IsSender: false},
		},
	}
	a := New(Spec{ID: "instagram", Network: "Instagram"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "instagram", Verb: manifest.Read, Body: "Friday",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Details["mode"] != "search" {
		t.Fatalf("details=%v", plan.Details)
	}
	if len(api.searchCalls) != 1 || api.searchCalls[0].Query != "Friday" {
		t.Fatalf("searchCalls=%+v", api.searchCalls)
	}
}

func TestOversizedMessagesTruncateWithinCodecLimits(t *testing.T) {
	huge := strings.Repeat("x", 500)
	msgs := make([]beeper.Message, 0, 10)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, beeper.Message{ID: string(rune('a' + i)), SenderName: "Maya", Text: huge, IsSender: false})
	}
	api := &fakeBeeper{
		chats:          []beeper.Chat{{ID: "c1", Network: "Instagram", Title: "Maya"}},
		messagesByChat: map[string][]beeper.Message{"c1": msgs},
	}
	a := New(Spec{ID: "instagram", Network: "Instagram"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{AdapterID: "instagram", Verb: manifest.Read, Subject: "Maya"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	preview, err := a.Preview(t.Context(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(preview.Lines) > 8 {
		t.Fatalf("preview lines=%d, codec max is 8", len(preview.Lines))
	}
	for i, line := range preview.Lines {
		if utf8.RuneCountInString(line) > 1024 {
			t.Fatalf("line %d length %d exceeds codec 1024", i, utf8.RuneCountInString(line))
		}
	}
}

func TestUnknownOperationAsksClarification(t *testing.T) {
	api := &fakeBeeper{chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "Maya"}}}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())
	_, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Modify, Subject: "Maya", Body: "hi",
		Fields: map[string]string{"operation": "explode"},
	})
	var question *adapter.ClarificationError
	if !errors.As(err, &question) {
		t.Fatalf("got %v, want ClarificationError", err)
	}
}

func TestMissingOperationOnModifyAsksClarification(t *testing.T) {
	api := &fakeBeeper{chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "Maya"}}}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())
	_, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Modify, Subject: "Maya", Body: "hi",
	})
	var question *adapter.ClarificationError
	if !errors.As(err, &question) {
		t.Fatalf("got %v, want ClarificationError", err)
	}
}

func TestCapabilityMissingEditRefusesWithPlainSentence(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Instagram", Title: "Maya"}},
		chatByID: map[string]beeper.Chat{
			"c1": {ID: "c1", Network: "Instagram", Title: "Maya", Capabilities: beeper.ChatCapabilities{Edit: 0}},
		},
		messagesByChat: map[string][]beeper.Message{
			"c1": {{ID: "m1", Text: "my last", IsSender: true}},
		},
	}
	a := New(Spec{ID: "instagram", Network: "Instagram"}, api, testLogger())
	_, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "instagram", Verb: manifest.Modify, Subject: "Maya", Body: "new text",
		Fields: map[string]string{"operation": "edit"},
	})
	var question *adapter.ClarificationError
	if !errors.As(err, &question) {
		t.Fatalf("got %T %v, want ClarificationError naming missing capability", err, err)
	}
	if !strings.Contains(strings.ToLower(question.Question), "edit") {
		t.Fatalf("question=%q", question.Question)
	}
}

func TestEditTargetsOwnLatestAndPinsMessageID(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "Maya"}},
		chatByID: map[string]beeper.Chat{
			"c1": {ID: "c1", Network: "Discord", Title: "Maya", Capabilities: beeper.ChatCapabilities{Edit: 2}},
		},
		messagesByChat: map[string][]beeper.Message{
			"c1": {
				{ID: "incoming", Text: "their latest", IsSender: false},
				{ID: "mine", Text: "my latest", IsSender: true},
				{ID: "older-mine", Text: "older", IsSender: true},
			},
		},
	}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Modify, Subject: "Maya", Body: "fixed text",
		Fields: map[string]string{"operation": "edit"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Details["message_id"] != "mine" || plan.Details["target_text"] != "my latest" {
		t.Fatalf("details=%v", plan.Details)
	}
	preview, err := a.Preview(t.Context(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !strings.Contains(strings.Join(preview.Lines, "\n"), "my latest") {
		t.Fatalf("preview must show pinned text: %v", preview.Lines)
	}
	outcome, err := a.Execute(t.Context(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Done {
		t.Fatalf("edit should confirm done, got %+v", outcome)
	}
	if len(api.edits) != 1 || api.edits[0].messageID != "mine" || api.edits[0].text != "fixed text" {
		t.Fatalf("edits=%+v", api.edits)
	}
}

func TestReplyAndReactTargetIncomingLatest(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "Maya"}},
		chatByID: map[string]beeper.Chat{
			"c1": {ID: "c1", Network: "Discord", Title: "Maya", Capabilities: beeper.ChatCapabilities{Reply: 2, Reaction: 2}},
		},
		messagesByChat: map[string][]beeper.Message{
			"c1": {
				{ID: "mine", Text: "my note", IsSender: true},
				{ID: "incoming", Text: "their ask", IsSender: false},
			},
		},
	}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())

	replyPlan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Send, Subject: "Maya", Body: "sure",
		Fields: map[string]string{"operation": "reply"},
	})
	if err != nil {
		t.Fatalf("reply Resolve: %v", err)
	}
	if replyPlan.Details["message_id"] != "incoming" {
		t.Fatalf("reply target=%v", replyPlan.Details)
	}
	_, err = a.Execute(t.Context(), replyPlan)
	var unknown *adapter.OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("reply Execute=%v, want pending unknown", err)
	}
	if len(api.sent) != 1 || api.sent[0].replyTo != "incoming" {
		t.Fatalf("sent=%+v", api.sent)
	}

	reactPlan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Modify, Subject: "Maya", Body: "👍",
		Fields: map[string]string{"operation": "react"},
	})
	if err != nil {
		t.Fatalf("react Resolve: %v", err)
	}
	if reactPlan.Details["message_id"] != "incoming" {
		t.Fatalf("react target=%v", reactPlan.Details)
	}
	outcome, err := a.Execute(t.Context(), reactPlan)
	if err != nil || !outcome.Done {
		t.Fatalf("react Execute outcome=%+v err=%v", outcome, err)
	}
	if len(api.reacts) != 1 || api.reacts[0].messageID != "incoming" {
		t.Fatalf("reacts=%+v", api.reacts)
	}
}

func TestDeleteTargetsOwnLatest(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "Maya"}},
		chatByID: map[string]beeper.Chat{
			"c1": {ID: "c1", Network: "Discord", Title: "Maya", Capabilities: beeper.ChatCapabilities{Delete: 2}},
		},
		messagesByChat: map[string][]beeper.Message{
			"c1": {
				{ID: "incoming", Text: "theirs", IsSender: false},
				{ID: "mine", Text: "delete me", IsSender: true},
			},
		},
	}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Cancel, Subject: "Maya",
		Fields: map[string]string{"operation": "delete"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Details["message_id"] != "mine" {
		t.Fatalf("details=%v", plan.Details)
	}
	outcome, err := a.Execute(t.Context(), plan)
	if err != nil || !outcome.Done {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if len(api.deletes) != 1 || api.deletes[0] != "mine" {
		t.Fatalf("deletes=%v", api.deletes)
	}
}

func TestQuotedBodyMatchesRecentMessageInsteadOfRecency(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "Maya"}},
		chatByID: map[string]beeper.Chat{
			"c1": {ID: "c1", Network: "Discord", Title: "Maya", Capabilities: beeper.ChatCapabilities{Delete: 2}},
		},
		messagesByChat: map[string][]beeper.Message{
			"c1": {
				{ID: "newer-mine", Text: "newest own", IsSender: true},
				{ID: "quoted", Text: "please remove this one", IsSender: true},
			},
		},
	}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Cancel, Subject: "Maya",
		Body:   "please remove this one",
		Fields: map[string]string{"operation": "delete"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Details["message_id"] != "quoted" {
		t.Fatalf("expected quoted match, got %v", plan.Details)
	}
}

func TestArchiveConfirmedOutcome(t *testing.T) {
	api := &fakeBeeper{
		chats: []beeper.Chat{{ID: "c1", Network: "Discord", Title: "crew"}},
		chatByID: map[string]beeper.Chat{
			"c1": {ID: "c1", Network: "Discord", Title: "crew", Capabilities: beeper.ChatCapabilities{Archive: true}},
		},
	}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())
	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Modify, Subject: "crew",
		Fields: map[string]string{"operation": "archive"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	outcome, err := a.Execute(t.Context(), plan)
	if err != nil || !outcome.Done {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if len(api.archives) != 1 || !api.archives[0].archived {
		t.Fatalf("archives=%+v", api.archives)
	}
}

func TestSendStillRequiresRecipient(t *testing.T) {
	a := New(Spec{ID: "discord", Network: "Discord"}, &fakeBeeper{}, testLogger())
	_, err := a.Resolve(t.Context(), adapter.Intent{AdapterID: "discord", Verb: manifest.Send, Body: "hi"})
	if !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("got %v, want ErrNoRecipient", err)
	}
}

var _ = context.Background

// (ops tests live in adapter_ops_test.go)
