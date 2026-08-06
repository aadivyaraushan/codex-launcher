package beepermessage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

type fakeBeeper struct {
	chats          []beeper.Chat
	accounts       []beeper.Account
	started        []startCall
	startReply     beeper.Chat
	sent           []sentCall
	chatPages      []beeper.ChatPage
	chatPageIdx    int
	chatByID       map[string]beeper.Chat
	messagesByChat map[string][]beeper.Message
	searchResults  []beeper.Message
	searchCalls    []beeper.SearchMessagesOptions
	edits          []editCall
	deletes        []string
	reacts         []reactCall
	archives       []archiveCall
}

type startCall struct {
	accountID   string
	phoneNumber string
}

type sentCall struct {
	chatID  string
	text    string
	replyTo string
}

type editCall struct {
	chatID    string
	messageID string
	text      string
}

type reactCall struct {
	chatID      string
	messageID   string
	reactionKey string
}

type archiveCall struct {
	chatID   string
	archived bool
}

func (f *fakeBeeper) SearchChats(context.Context, string) ([]beeper.Chat, error) {
	return append([]beeper.Chat(nil), f.chats...), nil
}

func (f *fakeBeeper) Accounts(context.Context) ([]beeper.Account, error) {
	return append([]beeper.Account(nil), f.accounts...), nil
}

func (f *fakeBeeper) StartChat(_ context.Context, accountID, phoneNumber string) (beeper.Chat, error) {
	f.started = append(f.started, startCall{accountID: accountID, phoneNumber: phoneNumber})
	return f.startReply, nil
}

func (f *fakeBeeper) Send(ctx context.Context, chatID, text string) (beeper.Sent, error) {
	return f.SendReply(ctx, chatID, text, "")
}

func (f *fakeBeeper) SendReply(_ context.Context, chatID, text, replyTo string) (beeper.Sent, error) {
	f.sent = append(f.sent, sentCall{chatID: chatID, text: text, replyTo: replyTo})
	return beeper.Sent{ChatID: chatID, PendingMessageID: "pending-1"}, nil
}

func (f *fakeBeeper) ListChats(context.Context, beeper.ListChatsOptions) (beeper.ChatPage, error) {
	if len(f.chatPages) == 0 {
		return beeper.ChatPage{}, nil
	}
	idx := f.chatPageIdx
	if idx >= len(f.chatPages) {
		idx = len(f.chatPages) - 1
	}
	f.chatPageIdx++
	return f.chatPages[idx], nil
}

func (f *fakeBeeper) GetChat(_ context.Context, chatID string) (beeper.Chat, error) {
	if f.chatByID != nil {
		if chat, ok := f.chatByID[chatID]; ok {
			return chat, nil
		}
	}
	for _, chat := range f.chats {
		if chat.ID == chatID {
			return chat, nil
		}
	}
	return beeper.Chat{ID: chatID}, nil
}

func (f *fakeBeeper) ListMessages(_ context.Context, chatID string, _ beeper.MessageListOptions) (beeper.MessagePage, error) {
	return beeper.MessagePage{Items: append([]beeper.Message(nil), f.messagesByChat[chatID]...)}, nil
}

func (f *fakeBeeper) SearchMessages(_ context.Context, opts beeper.SearchMessagesOptions) (beeper.MessagePage, error) {
	f.searchCalls = append(f.searchCalls, opts)
	return beeper.MessagePage{Items: append([]beeper.Message(nil), f.searchResults...)}, nil
}

func (f *fakeBeeper) EditMessage(_ context.Context, chatID, messageID, text string) (beeper.Message, error) {
	f.edits = append(f.edits, editCall{chatID: chatID, messageID: messageID, text: text})
	return beeper.Message{ID: messageID, ChatID: chatID, Text: text, IsSender: true}, nil
}

func (f *fakeBeeper) DeleteMessage(_ context.Context, _, messageID string) error {
	f.deletes = append(f.deletes, messageID)
	return nil
}

func (f *fakeBeeper) React(_ context.Context, chatID, messageID, reactionKey string) error {
	f.reacts = append(f.reacts, reactCall{chatID: chatID, messageID: messageID, reactionKey: reactionKey})
	return nil
}

func (f *fakeBeeper) Unreact(context.Context, string, string, string) error { return nil }

func (f *fakeBeeper) MarkRead(_ context.Context, chatID, _ string) (beeper.Chat, error) {
	return beeper.Chat{ID: chatID}, nil
}

func (f *fakeBeeper) MarkUnread(_ context.Context, chatID, _ string) (beeper.Chat, error) {
	return beeper.Chat{ID: chatID}, nil
}

func (f *fakeBeeper) Archive(_ context.Context, chatID string, archived bool) error {
	f.archives = append(f.archives, archiveCall{chatID: chatID, archived: archived})
	return nil
}

func (f *fakeBeeper) UpdateChat(_ context.Context, chatID string, _ beeper.UpdateChatOptions) (beeper.Chat, error) {
	return beeper.Chat{ID: chatID}, nil
}

func (f *fakeBeeper) SetReminder(context.Context, string, time.Time, bool) error { return nil }

func (f *fakeBeeper) ClearReminder(context.Context, string) error { return nil }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSendNeedsAnExactNetworkAndShowsTheWholeIrreversibleAction(t *testing.T) {
	api := &fakeBeeper{chats: []beeper.Chat{
		{ID: "discord-1", Network: "Discord", Title: "Aadivya"},
		{ID: "instagram-1", Network: "Instagram", Title: "Aadivya"},
	}}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())

	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Send, Subject: "Aadivya", Body: "Operator verification 2026-08-04",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Handle != "discord-1" {
		t.Fatalf("resolved handle=%q, want the Discord chat", plan.Handle)
	}
	preview, err := a.Preview(t.Context(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Headline != "Send a Discord message to Aadivya" {
		t.Fatalf("headline=%q", preview.Headline)
	}
	wantLines := []string{"Network: Discord", "Conversation: Aadivya", "Operator verification 2026-08-04"}
	if len(preview.Lines) != len(wantLines) {
		t.Fatalf("preview lines=%v", preview.Lines)
	}
	for i := range wantLines {
		if preview.Lines[i] != wantLines[i] {
			t.Fatalf("preview line %d=%q, want %q", i, preview.Lines[i], wantLines[i])
		}
	}

	_, err = a.Execute(t.Context(), plan)
	var unknown *adapter.OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("Execute error = %v, want OutcomeUnknownError while Beeper delivery is pending", err)
	}
	if len(api.sent) != 1 || api.sent[0].chatID != "discord-1" || api.sent[0].text != "Operator verification 2026-08-04" {
		t.Fatalf("sent=%+v", api.sent)
	}
}

func TestAmbiguousConversationAsksInsteadOfPicking(t *testing.T) {
	api := &fakeBeeper{chats: []beeper.Chat{
		{ID: "1", Network: "Discord", Title: "Raina"},
		{ID: "2", Network: "Discord", Title: "Raina family"},
	}}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())

	_, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Send, Subject: "Raina", Body: "hello",
	})
	var question *adapter.ClarificationError
	if !errors.As(err, &question) {
		t.Fatalf("ambiguous chat returned %T %v, want ClarificationError", err, err)
	}
	if question.Question == "" {
		t.Fatal("clarification has no user-facing question")
	}
	if len(api.sent) != 0 {
		t.Fatalf("ambiguous resolve sent %d messages", len(api.sent))
	}
}

func TestSearchCanMatchAParticipantWhoseNameIsNotTheConversationTitle(t *testing.T) {
	api := &fakeBeeper{chats: []beeper.Chat{
		{ID: "discord-1", Network: "Discord", Title: "Unrelated display title"},
	}}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())

	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Send, Subject: "Aadivya", Body: "hello",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Handle != "discord-1" {
		t.Fatalf("resolved handle=%q, want Beeper's one Discord search result", plan.Handle)
	}
}

func TestNoConversationOnTheNamedNetworkAsksInsteadOfCrossingNetworks(t *testing.T) {
	api := &fakeBeeper{chats: []beeper.Chat{{ID: "1", Network: "Instagram", Title: "Aadivya"}}}
	a := New(Spec{ID: "discord", Network: "Discord"}, api, testLogger())

	_, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "discord", Verb: manifest.Send, Subject: "Aadivya", Body: "hello",
	})
	var question *adapter.ClarificationError
	if !errors.As(err, &question) {
		t.Fatalf("wrong-network-only search returned %T %v", err, err)
	}
	if len(api.sent) != 0 {
		t.Fatalf("wrong-network resolve sent %d messages", len(api.sent))
	}
}

func TestGoogleMessagesCanResolveAnApprovedPhoneThroughStartChat(t *testing.T) {
	api := &fakeBeeper{accounts: []beeper.Account{{ID: "google-account-live", Network: "Google Messages", Status: "connected"}}, startReply: beeper.Chat{
		ID: "google-chat-1", AccountID: "gmessages", Network: "Google Messages", Title: "wife", Type: "single",
	}}
	a := New(Spec{ID: "messages", Network: "Google Messages", StartByPhone: true}, api, testLogger())

	plan, err := a.Resolve(t.Context(), adapter.Intent{
		AdapterID: "messages", Verb: manifest.Send, Subject: "+1 224-322-8828", Body: "Operator verification 2026-08-04",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(api.started) != 0 {
		t.Fatalf("resolve changed external state before confirmation: %+v", api.started)
	}
	if plan.Handle != "" || plan.Details["start_phone"] != "+12243228828" {
		t.Fatalf("plan = %+v", plan)
	}
	preview, err := a.Preview(t.Context(), plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Headline != "Send a Google Messages message to +1 224-322-8828" {
		t.Fatalf("headline = %q", preview.Headline)
	}
	_, err = a.Execute(t.Context(), plan)
	var unknown *adapter.OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("Execute error = %v, want OutcomeUnknownError while delivery is pending", err)
	}
	if len(api.started) != 1 || api.started[0].accountID != "google-account-live" || api.started[0].phoneNumber != "+12243228828" {
		t.Fatalf("confirmed start calls = %+v", api.started)
	}
}

func TestRevokePersistsTheNetworkDisconnect(t *testing.T) {
	revokes := 0
	a := NewWithRevoke(Spec{ID: "discord", Network: "Discord"}, &fakeBeeper{}, func(context.Context) error {
		revokes++
		return nil
	}, testLogger())
	if err := a.Revoke(t.Context()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revokes != 1 {
		t.Fatalf("persistent revoke calls = %d, want 1", revokes)
	}
}
