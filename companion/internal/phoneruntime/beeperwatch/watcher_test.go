package beeperwatch

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testToken = "beeper-test-token"

// --- fake Beeper server ---
//
// Speaks just enough of the Beeper Client API WebSocket protocol
// (GET /v1/spec, "## WebSocket"): accept on /v1/ws with a Bearer token,
// send ready, read the client's subscriptions.set, reply
// subscriptions.updated, then let the test push domain-event frames.

type beeperSession struct {
	conn      *websocket.Conn
	subscribe json.RawMessage
	done      chan struct{}
	once      sync.Once
}

func (s *beeperSession) write(t *testing.T, frame string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.conn.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
		t.Fatalf("push frame to watcher: %v", err)
	}
}

func (s *beeperSession) drop() {
	s.once.Do(func() {
		s.conn.Close(websocket.StatusGoingAway, "drop")
		close(s.done)
	})
}

type fakeBeeper struct {
	server   *httptest.Server
	sessions chan *beeperSession
}

func newFakeBeeper(t *testing.T) *fakeBeeper {
	t.Helper()
	fake := &fakeBeeper{sessions: make(chan *beeperSession, 4)}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ws" {
			t.Errorf("watcher dialed %q, want /v1/ws", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("authorization = %q, want the configured bearer token", got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := r.Context()
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ready","version":1,"chatIDs":[]}`)); err != nil {
			return
		}
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"subscriptions.updated","chatIDs":["*"]}`)); err != nil {
			return
		}
		session := &beeperSession{conn: conn, subscribe: raw, done: make(chan struct{})}
		fake.sessions <- session
		// Keep the handler (and with it the hijacked socket) alive until
		// the test drops the session or the suite tears the server down.
		<-session.done
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeBeeper) nextSession(t *testing.T) *beeperSession {
	t.Helper()
	select {
	case session := <-f.sessions:
		t.Cleanup(session.drop)
		return session
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher never completed a subscribe handshake")
		return nil
	}
}

// --- trigger recording ---

type triggerRecorder struct {
	ch chan Message
}

func (r *triggerRecorder) notify(_ context.Context, msg Message) {
	r.ch <- msg
}

func (r *triggerRecorder) next(t *testing.T) Message {
	t.Helper()
	select {
	case msg := <-r.ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("no trigger arrived in time")
		return Message{}
	}
}

func (r *triggerRecorder) quiet(t *testing.T, wait time.Duration) {
	t.Helper()
	select {
	case msg := <-r.ch:
		t.Fatalf("unexpected trigger fired: %+v", msg)
	case <-time.After(wait):
	}
}

type loaderFunc func(ctx context.Context, chatID, messageID string) (Message, error)

func (f loaderFunc) LoadMessage(ctx context.Context, chatID, messageID string) (Message, error) {
	return f(ctx, chatID, messageID)
}

func startWatcher(t *testing.T, fake *fakeBeeper, loader MessageLoader) *triggerRecorder {
	t.Helper()
	recorder := &triggerRecorder{ch: make(chan Message, 16)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go Run(ctx, Config{
		BaseURL:    fake.server.URL,
		Token:      testToken,
		Loader:     loader,
		Notify:     recorder.notify,
		Logger:     slog.New(slog.DiscardHandler),
		RetryDelay: 20 * time.Millisecond,
	})
	return recorder
}

// --- tests ---

func TestWatcherSubscribesToAllChatsAfterReady(t *testing.T) {
	fake := newFakeBeeper(t)
	startWatcher(t, fake, nil)

	session := fake.nextSession(t)
	var sub struct {
		Type    string   `json:"type"`
		ChatIDs []string `json:"chatIDs"`
		App     struct {
			State bool `json:"state"`
		} `json:"app"`
	}
	if err := json.Unmarshal(session.subscribe, &sub); err != nil {
		t.Fatalf("decode subscribe frame: %v", err)
	}
	if sub.Type != "subscriptions.set" {
		t.Fatalf("first client frame = %q, want subscriptions.set", sub.Type)
	}
	if len(sub.ChatIDs) != 1 || sub.ChatIDs[0] != "*" {
		t.Fatalf("chatIDs = %v, want the wildcard subscription", sub.ChatIDs)
	}
	if !sub.App.State {
		t.Fatal("app.state must be subscribed so setup regressions surface")
	}
}

func TestAnInboundMessageBecomesATrigger(t *testing.T) {
	fake := newFakeBeeper(t)
	recorder := startWatcher(t, fake, nil)
	session := fake.nextSession(t)

	session.write(t, `{"type":"message.upserted","seq":1,"ts":"2026-08-12T10:00:00Z","chatID":"chat-1","ids":["msg-1"],"entries":[{"id":"msg-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","senderName":"Maya","timestamp":"2026-08-12T10:00:00Z","sortKey":"1","text":"you around?","isSender":false,"isUnread":true}]}`)

	got := recorder.next(t)
	want := Message{
		ID:         "msg-1",
		ChatID:     "chat-1",
		SenderID:   "instagram:maya",
		SenderName: "Maya",
		Text:       "you around?",
		Timestamp:  "2026-08-12T10:00:00Z",
	}
	if got != want {
		t.Fatalf("trigger = %+v, want %+v", got, want)
	}
}

func TestOwnHiddenAndDeletedMessagesDoNotTrigger(t *testing.T) {
	fake := newFakeBeeper(t)
	recorder := startWatcher(t, fake, nil)
	session := fake.nextSession(t)

	// One frame carrying everything the watcher must ignore: our own
	// outbound message, a hidden one, and a deleted one.
	session.write(t, `{"type":"message.upserted","seq":1,"ts":"2026-08-12T10:00:00Z","chatID":"chat-1","ids":["own-1","hidden-1","gone-1"],"entries":[{"id":"own-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:me","timestamp":"2026-08-12T10:00:00Z","sortKey":"1","text":"on my way","isSender":true},{"id":"hidden-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","timestamp":"2026-08-12T10:00:01Z","sortKey":"2","text":"spam","isSender":false,"isHidden":true},{"id":"gone-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","timestamp":"2026-08-12T10:00:02Z","sortKey":"3","isSender":false,"isDeleted":true}]}`)

	recorder.quiet(t, 250*time.Millisecond)
}

func TestAMissingEntryIsLoadedOverHTTP(t *testing.T) {
	fake := newFakeBeeper(t)
	var mu sync.Mutex
	var loads [][2]string
	loader := loaderFunc(func(_ context.Context, chatID, messageID string) (Message, error) {
		mu.Lock()
		loads = append(loads, [2]string{chatID, messageID})
		mu.Unlock()
		if messageID == "own-9" {
			return Message{ID: "own-9", ChatID: chatID, SenderID: "instagram:me", Text: "sent from my other device", Timestamp: "2026-08-12T10:01:00Z", IsSender: true}, nil
		}
		return Message{ID: messageID, ChatID: chatID, SenderID: "instagram:maya", SenderName: "Maya", Text: "loaded over http", Timestamp: "2026-08-12T10:01:00Z"}, nil
	})
	recorder := startWatcher(t, fake, loader)
	session := fake.nextSession(t)

	// The spec marks entries optional: an event may carry only ids, and
	// at-most-once delivery means skipping it would lose the trigger.
	session.write(t, `{"type":"message.upserted","seq":1,"ts":"2026-08-12T10:01:00Z","chatID":"chat-1","ids":["msg-9"]}`)

	got := recorder.next(t)
	if got.ID != "msg-9" || got.Text != "loaded over http" {
		t.Fatalf("trigger = %+v, want the loader's message", got)
	}
	mu.Lock()
	firstLoad := loads[0]
	mu.Unlock()
	if firstLoad != [2]string{"chat-1", "msg-9"} {
		t.Fatalf("loader saw %v, want chat-1/msg-9", firstLoad)
	}

	// A loaded message that turns out to be our own must stay silent.
	session.write(t, `{"type":"message.upserted","seq":2,"ts":"2026-08-12T10:02:00Z","chatID":"chat-1","ids":["own-9"]}`)
	recorder.quiet(t, 250*time.Millisecond)
}

func TestAnEditedMessageDoesNotFireTwice(t *testing.T) {
	fake := newFakeBeeper(t)
	recorder := startWatcher(t, fake, nil)
	session := fake.nextSession(t)

	session.write(t, `{"type":"message.upserted","seq":1,"ts":"2026-08-12T10:00:00Z","chatID":"chat-1","ids":["msg-1"],"entries":[{"id":"msg-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","senderName":"Maya","timestamp":"2026-08-12T10:00:00Z","sortKey":"1","text":"you around","isSender":false}]}`)
	if got := recorder.next(t); got.ID != "msg-1" {
		t.Fatalf("first upsert should trigger, got %+v", got)
	}

	// An edit arrives as another message.upserted for the same id; waking
	// the agent again for it would double-handle one message.
	session.write(t, `{"type":"message.upserted","seq":2,"ts":"2026-08-12T10:00:30Z","chatID":"chat-1","ids":["msg-1"],"entries":[{"id":"msg-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","senderName":"Maya","timestamp":"2026-08-12T10:00:00Z","sortKey":"1","text":"you around? (edited)","isSender":false,"editedTimestamp":"2026-08-12T10:00:30Z"}]}`)
	recorder.quiet(t, 250*time.Millisecond)
}

func TestReconnectResubscribesAndRemembersWhatItSaw(t *testing.T) {
	fake := newFakeBeeper(t)
	recorder := startWatcher(t, fake, nil)
	first := fake.nextSession(t)

	entry := `{"id":"msg-1","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","senderName":"Maya","timestamp":"2026-08-12T10:00:00Z","sortKey":"1","text":"you around?","isSender":false}`
	first.write(t, `{"type":"message.upserted","seq":1,"ts":"2026-08-12T10:00:00Z","chatID":"chat-1","ids":["msg-1"],"entries":[`+entry+`]}`)
	recorder.next(t)

	first.drop()

	// The redial must run the full handshake again — subscriptions are
	// per-connection and start empty (spec: "Initial subscription state
	// is empty"), so a reconnect that skips subscriptions.set hears
	// nothing forever.
	second := fake.nextSession(t)
	var sub struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(second.subscribe, &sub); err != nil || sub.Type != "subscriptions.set" {
		t.Fatalf("second connection's first frame = %s (err %v), want subscriptions.set", second.subscribe, err)
	}

	// At-most-once with no replay means the same message can be pushed
	// again after reconnect reconciliation; memory must span connections.
	second.write(t, `{"type":"message.upserted","seq":1,"ts":"2026-08-12T10:00:00Z","chatID":"chat-1","ids":["msg-1"],"entries":[`+entry+`]}`)
	recorder.quiet(t, 250*time.Millisecond)

	second.write(t, `{"type":"message.upserted","seq":2,"ts":"2026-08-12T10:03:00Z","chatID":"chat-1","ids":["msg-2"],"entries":[{"id":"msg-2","chatID":"chat-1","accountID":"acc-1","senderID":"instagram:maya","senderName":"Maya","timestamp":"2026-08-12T10:03:00Z","sortKey":"2","text":"ping","isSender":false}]}`)
	if got := recorder.next(t); got.ID != "msg-2" {
		t.Fatalf("fresh message after reconnect = %+v, want msg-2", got)
	}
}
