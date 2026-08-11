package turnproxy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

// EventPublisher delivers MobileEvents to the phone. It is the launcher's
// event pump (internal/app/eventpump.go), not the gateway.
type EventPublisher interface {
	PublishTaskEvent(context.Context, taskstate.MobileEvent) error
}

// Config configures one Source: which gateway to dial, which task and
// gateway session it speaks for, and where its MobileEvents go.
type Config struct {
	URL        string
	Token      string
	TaskID     string
	SessionKey string
	Publisher  EventPublisher
	Logger     *slog.Logger
}

// Source is a TaskSource/ExistingTaskSource (internal/app/mobilesession)
// backed by one OpenClaw Gateway session pinned to a single phone-agent
// task. It reuses TurnMapper to turn that session's "chat" events into
// MobileEvents.
type Source struct {
	cfg    Config
	client *gatewayClient
	logger *slog.Logger

	// mu guards mapper and the derived state below. handleEvent (driven by
	// the client's reader goroutine) is the only writer; CurrentTask and
	// ListRecent are readers.
	mu           sync.Mutex
	mapper       *TurnMapper
	state        taskstate.State
	activeTurnID string
	updatedAt    int64
}

type chatSendParams struct {
	SessionKey     string `json:"sessionKey"`
	Message        string `json:"message"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type steerParams struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

type abortParams struct {
	SessionKey string `json:"sessionKey"`
}

// Connect dials the gateway and performs the operator handshake before
// returning. A refused or dropped handshake returns an error and no
// Source, so there is never a half-open Source in the wild.
func Connect(ctx context.Context, cfg Config) (*Source, error) {
	if cfg.Publisher == nil {
		return nil, fmt.Errorf("turnproxy: publisher is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	source := &Source{
		cfg:       cfg,
		logger:    logger,
		mapper:    NewTurnMapper(cfg.TaskID, cfg.SessionKey),
		state:     taskstate.IdleAfterReply,
		updatedAt: time.Now().Unix(),
	}
	client, err := connectClient(ctx, cfg.URL, cfg.Token, logger, source.handleEvent)
	if err != nil {
		return nil, err
	}
	source.client = client
	return source, nil
}

// handleEvent runs on the client's reader goroutine for every "event"
// frame; it ignores everything but "chat".
func (source *Source) handleEvent(event string, payload json.RawMessage) {
	if event != "chat" {
		return
	}
	var chatPayload ChatEventPayload
	if err := json.Unmarshal(payload, &chatPayload); err != nil {
		source.logger.Warn("[turnproxy] dropped unparsable chat event")
		return
	}
	if chatPayload.SessionKey != source.cfg.SessionKey {
		return
	}

	source.mu.Lock()
	mobileEvents := source.mapper.Apply(chatPayload)
	source.applyRunStateLocked(chatPayload)
	source.mu.Unlock()

	for _, mobileEvent := range mobileEvents {
		if err := source.cfg.Publisher.PublishTaskEvent(context.Background(), mobileEvent); err != nil {
			source.logger.Error("[turnproxy] publish task event failed", "kind", mobileEvent.Kind, "error", err.Error())
		}
	}
}

// applyRunStateLocked tracks the task's current State and ActiveTurnID from
// the raw chat event, independent of which MobileEvents the mapper
// produced for it. Caller holds source.mu.
func (source *Source) applyRunStateLocked(chatPayload ChatEventPayload) {
	switch chatPayload.State {
	case "delta":
		source.state = taskstate.Working
		source.activeTurnID = chatPayload.RunID
	case "final":
		source.state = taskstate.IdleAfterReply
		source.activeTurnID = ""
	case "error":
		source.state = taskstate.Failed
		source.activeTurnID = ""
	case "aborted":
		source.state = taskstate.Interrupted
		source.activeTurnID = ""
	default:
		return
	}
	source.updatedAt = time.Now().Unix()
}

// StartExistingTurn sends chat.send for the phone agent's own task and
// returns once the gateway acknowledges the request; the reply itself
// streams back later as "chat" events.
func (source *Source) StartExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	if taskID != source.cfg.TaskID {
		return taskadapter.ExistingTaskResult{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	idempotencyKey := newIdempotencyKey()
	params := chatSendParams{
		SessionKey:     source.cfg.SessionKey,
		Message:        prompt,
		IdempotencyKey: idempotencyKey,
	}
	if _, err := source.client.request(ctx, "chat.send", params); err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	return taskadapter.ExistingTaskResult{ThreadID: source.cfg.TaskID, TurnID: idempotencyKey}, nil
}

// RedirectExistingTurn sends sessions.steer, which the gateway treats as
// chat.send with interruptIfActive: true. Its session field is "key", not
// "sessionKey" — the gateway validates that strictly.
func (source *Source) RedirectExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	if taskID != source.cfg.TaskID {
		return taskadapter.ExistingTaskResult{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	params := steerParams{Key: source.cfg.SessionKey, Message: prompt}
	if _, err := source.client.request(ctx, "sessions.steer", params); err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	return taskadapter.ExistingTaskResult{ThreadID: source.cfg.TaskID}, nil
}

// InterruptExistingTurn sends chat.abort for the session's active run.
func (source *Source) InterruptExistingTurn(ctx context.Context, taskID string) (taskadapter.ExistingTaskResult, error) {
	if taskID != source.cfg.TaskID {
		return taskadapter.ExistingTaskResult{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	params := abortParams{SessionKey: source.cfg.SessionKey}
	if _, err := source.client.request(ctx, "chat.abort", params); err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	return taskadapter.ExistingTaskResult{ThreadID: source.cfg.TaskID}, nil
}

// CurrentTask returns the phone agent's one task, refusing any other id.
func (source *Source) CurrentTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	if taskID != source.cfg.TaskID {
		return taskstate.Task{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	return source.snapshot(), nil
}

// ListRecent always returns the single phone-agent task this Source speaks
// for; limit is accepted for interface compatibility and otherwise unused.
func (source *Source) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	return []taskstate.Task{source.snapshot()}, nil
}

func (source *Source) snapshot() taskstate.Task {
	source.mu.Lock()
	defer source.mu.Unlock()
	activeTurnID := ""
	if source.state == taskstate.Working {
		activeTurnID = source.activeTurnID
	}
	return taskstate.Task{
		ID:            source.cfg.TaskID,
		Title:         "Phone agent",
		State:         source.state,
		ActiveTurnID:  activeTurnID,
		CanRedirect:   source.state == taskstate.Working,
		UpdatedAtUnix: source.updatedAt,
		Source:        taskstate.SourceAppServer,
	}
}

// Close stops the reader goroutine and closes the gateway connection.
// Idempotent, so it is safe to defer in tests via t.Cleanup.
func (source *Source) Close() error {
	if source.client == nil {
		return nil
	}
	return source.client.Close()
}

func newIdempotencyKey() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
