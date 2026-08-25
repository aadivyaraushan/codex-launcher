// Package beeperwatch watches the Beeper Client API WebSocket for new
// inbound messages and turns them into trigger callbacks. It is the phone
// side of "a small watcher in phoneruntime forwards new inbound messages to
// the Gateway as events": it owns the connection to Beeper, not the
// connection to the Gateway (that's turnproxy).
//
// Delivery is at-most-once by design: a message that fails to load or
// arrives while the socket is down is logged and dropped rather than
// retried, so a flaky network never blocks or crashes the watcher.
package beeperwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// Message is one chat message as the Beeper Client API represents it, both
// inline in a message.upserted entry and from the GET-message loader. JSON
// tags follow the wire field names exactly (OpenAPI spec, GET /v1/spec).
type Message struct {
	ID         string `json:"id"`
	ChatID     string `json:"chatID"`
	SenderID   string `json:"senderID"`
	SenderName string `json:"senderName"`
	Text       string `json:"text"`
	Timestamp  string `json:"timestamp"`
	IsSender   bool   `json:"isSender"`
	IsHidden   bool   `json:"isHidden"`
	IsDeleted  bool   `json:"isDeleted"`
}

// MessageLoader fetches one message by id. The watcher calls it when a
// message.upserted event references an id that isn't inlined in entries —
// the spec marks entries optional.
type MessageLoader interface {
	LoadMessage(ctx context.Context, chatID, messageID string) (Message, error)
}

// Config configures Run.
type Config struct {
	BaseURL string
	Token   string

	// Loader fills in messages a message.upserted event didn't inline. May
	// be nil; ids that need it are then skipped.
	Loader MessageLoader

	// Notify is called synchronously from the read loop for every new,
	// non-filtered inbound message.
	Notify func(context.Context, Message)

	Logger *slog.Logger

	// RetryDelay is how long Run waits before redialing after a dropped or
	// refused connection. Defaults to 5s.
	RetryDelay time.Duration
}

const defaultRetryDelay = 5 * time.Second

// maxSeen bounds the dedupe memory so a long-lived watcher doesn't grow
// without limit; the oldest ids are evicted once the cap is hit.
const maxSeen = 4096

// Run dials the Beeper Client API WebSocket, performs the ready/subscribe
// handshake, and delivers message.upserted events to cfg.Notify until ctx
// is done. On a dropped or refused connection it redials after
// cfg.RetryDelay — the handshake runs fresh on every connection because
// subscriptions are per-connection state on the server (initial
// subscription state is empty on each new socket). Message-id dedupe
// survives reconnects so a redial can't re-fire a trigger the watcher
// already delivered.
func Run(ctx context.Context, cfg Config) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultRetryDelay
	}

	seen := newSeenSet(maxSeen)

	for ctx.Err() == nil {
		if err := runOnce(ctx, cfg, logger, seen); err != nil && ctx.Err() == nil {
			logger.Info("[beeperwatch] connection dropped", "error", err)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}

// runOnce owns a single connection end to end: dial, handshake, subscribe,
// then read frames until the socket errors or ctx is done.
func runOnce(ctx context.Context, cfg Config, logger *slog.Logger, seen *seenSet) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+cfg.Token)

	conn, _, err := websocket.Dial(ctx, cfg.BaseURL+"/v1/ws", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return fmt.Errorf("beeperwatch: dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "beeperwatch closing")

	if err := awaitReady(ctx, conn); err != nil {
		return fmt.Errorf("beeperwatch: await ready: %w", err)
	}
	if err := subscribeAll(ctx, conn); err != nil {
		return fmt.Errorf("beeperwatch: subscribe: %w", err)
	}
	logger.Debug("[beeperwatch] connected and subscribed")

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("beeperwatch: read: %w", err)
		}
		handleFrame(ctx, cfg, logger, seen, data)
	}
}

// awaitReady reads the server's opening frame. The spec guarantees it's
// "ready" first on every connection; anything else means the server isn't
// speaking the protocol this watcher expects.
func awaitReady(ctx context.Context, conn *websocket.Conn) error {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	var frame struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		return fmt.Errorf("decode opening frame: %w", err)
	}
	if frame.Type != "ready" {
		return fmt.Errorf("opening frame type = %q, want ready", frame.Type)
	}
	return nil
}

type subscribeRequest struct {
	Type    string       `json:"type"`
	ChatIDs []string     `json:"chatIDs"`
	App     subscribeApp `json:"app"`
}

type subscribeApp struct {
	State bool `json:"state"`
}

// subscribeAll asks for every chat plus app-state updates, then waits for
// the server's acknowledgement so a slow subscribe can't race the first
// domain event landing in the main read loop.
func subscribeAll(ctx context.Context, conn *websocket.Conn) error {
	req := subscribeRequest{Type: "subscriptions.set", ChatIDs: []string{"*"}, App: subscribeApp{State: true}}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode subscriptions.set: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("send subscriptions.set: %w", err)
	}
	if _, _, err := conn.Read(ctx); err != nil {
		return fmt.Errorf("await subscriptions.updated: %w", err)
	}
	return nil
}

// upsertedFrame is the message.upserted event shape. Fields the watcher
// doesn't need (seq, ts) are dropped rather than modeled.
type upsertedFrame struct {
	Type    string    `json:"type"`
	ChatID  string    `json:"chatID"`
	IDs     []string  `json:"ids"`
	Entries []Message `json:"entries"`
}

// handleFrame decodes one server frame and dispatches it. Everything but
// message.upserted is ignored — chat.upserted, chat.deleted,
// message.deleted, and app.state.updated carry nothing this watcher acts
// on — and an unparsable frame is logged and dropped rather than treated
// as fatal, since one bad frame shouldn't take down the connection.
func handleFrame(ctx context.Context, cfg Config, logger *slog.Logger, seen *seenSet, data []byte) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		logger.Warn("[beeperwatch] dropped unparsable frame")
		return
	}
	if envelope.Type != "message.upserted" {
		logger.Debug("[beeperwatch] ignoring frame", "type", envelope.Type)
		return
	}

	var frame upsertedFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		logger.Warn("[beeperwatch] dropped unparsable message.upserted frame")
		return
	}
	byID := make(map[string]Message, len(frame.Entries))
	for _, entry := range frame.Entries {
		byID[entry.ID] = entry
	}
	for _, id := range frame.IDs {
		handleUpsert(ctx, cfg, logger, seen, frame.ChatID, id, byID)
	}
}

// handleUpsert resolves one upserted message id to a Message (inline or
// loaded), filters it, and notifies. Dedupe happens by id up front so an
// edit — the same id upserted again — never re-triggers.
func handleUpsert(ctx context.Context, cfg Config, logger *slog.Logger, seen *seenSet, chatID, id string, byID map[string]Message) {
	if seen.seenBefore(id) {
		logger.Debug("[beeperwatch] skipping already-seen message", "id", id, "chatID", chatID)
		return
	}

	msg, ok := byID[id]
	if !ok {
		if cfg.Loader == nil {
			logger.Debug("[beeperwatch] no loader configured, skipping entry-less message", "id", id, "chatID", chatID)
			return
		}
		loaded, err := cfg.Loader.LoadMessage(ctx, chatID, id)
		if err != nil {
			logger.Warn("[beeperwatch] failed to load message, skipping", "id", id, "chatID", chatID, "error", err)
			return
		}
		msg = loaded
	}

	seen.markSeen(id)

	if msg.IsSender || msg.IsHidden || msg.IsDeleted {
		logger.Debug("[beeperwatch] skipping filtered message", "id", id, "chatID", chatID, "isSender", msg.IsSender, "isHidden", msg.IsHidden, "isDeleted", msg.IsDeleted)
		return
	}
	if cfg.Notify != nil {
		cfg.Notify(ctx, msg)
	}
}

// seenSet is a bounded, insertion-ordered set of message ids used to dedupe
// message.upserted deliveries across the whole Run lifetime, including
// reconnects. Not safe for concurrent use — Run drives it from a single
// goroutine (one connection's read loop at a time), which is all it needs.
type seenSet struct {
	max   int
	set   map[string]struct{}
	order []string
}

func newSeenSet(max int) *seenSet {
	return &seenSet{max: max, set: make(map[string]struct{}, max)}
}

func (s *seenSet) seenBefore(id string) bool {
	_, ok := s.set[id]
	return ok
}

func (s *seenSet) markSeen(id string) {
	if _, ok := s.set[id]; ok {
		return
	}
	s.set[id] = struct{}{}
	s.order = append(s.order, id)
	if len(s.order) > s.max {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.set, oldest)
	}
}
