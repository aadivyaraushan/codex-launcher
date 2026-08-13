package turnproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/tasktranscript"
)

// EventPublisher delivers MobileEvents to the phone. It is the launcher's
// event pump (internal/app/eventpump.go), not the gateway.
type EventPublisher interface {
	PublishTaskEvent(context.Context, taskstate.MobileEvent) error
}

const (
	// HomeComposeTaskID is the virtual inbox Home Send targets. It is never
	// a real OpenClaw session: StartExistingTurn on this id allocates a new
	// phone-chat task and a new agent:main:phone-* session so each Home
	// prompt is a fresh conversation. Beeper/inbound keeps Config.TaskID
	// (phone-agent) and Config.SessionKey (agent:main:main).
	HomeComposeTaskID      = "phone-home"
	thinkingEventsCap      = "thinking-events"
	sessionScopedEventsCap = "session-scoped-events"
	homeChatPrefix         = "phone-chat-"
	homeSessionPrefix      = "agent:main:phone-"
	userThinkingLevel      = "max"
)

// Config configures one Source: which gateway to dial, which inbound task
// and gateway session Beeper uses, and where MobileEvents go.
type Config struct {
	URL        string
	Token      string
	TaskID     string
	SessionKey string
	Publisher  EventPublisher
	Logger     *slog.Logger

	// InitialLastMessage carries forward what the previous connection last
	// knew, so a redial doesn't blank the Home preview before the next turn.
	InitialLastMessage taskstate.LastMessage
}

// Source is a TaskSource/ExistingTaskSource (internal/app/mobilesession)
// backed by one OpenClaw Gateway connection. The inbound phone-agent
// session is pinned for Beeper; Home compose allocates extra tasks.
type Source struct {
	cfg    Config
	client *gatewayClient
	logger *slog.Logger

	// mu guards conversations. handleEvent (the client's reader goroutine)
	// is the only writer of run state; CurrentTask and ListRecent are readers.
	mu            sync.Mutex
	conversations map[string]*conversation
	bySession     map[string]*conversation
}

type conversation struct {
	taskID       string
	sessionKey   string
	title        string
	mapper       *TurnMapper
	state        taskstate.State
	activeTurnID string
	pendingRunID string
	sentRunID    string
	updatedAt    int64
	lastMessage  taskstate.LastMessage
	entries      []tasktranscript.Entry
	nextEntry    uint64
}

type chatSendParams struct {
	SessionKey     string `json:"sessionKey"`
	Message        string `json:"message"`
	IdempotencyKey string `json:"idempotencyKey"`
	Thinking       string `json:"thinking,omitempty"`
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
	inbound := newConversation(cfg.TaskID, cfg.SessionKey, "Phone agent")
	inbound.lastMessage = taskstate.SafeLastMessage(cfg.InitialLastMessage)
	if cfg.InitialLastMessage.Text != "" {
		inbound.appendTranscript(kindFromSpeaker(cfg.InitialLastMessage.From), cfg.InitialLastMessage.Text, "seed")
	}
	source := &Source{
		cfg:           cfg,
		logger:        logger,
		conversations: map[string]*conversation{cfg.TaskID: inbound},
		bySession:     map[string]*conversation{cfg.SessionKey: inbound},
	}
	client, err := connectClient(ctx, cfg.URL, cfg.Token, logger, source.handleEvent)
	if err != nil {
		return nil, err
	}
	source.client = client
	return source, nil
}

func newConversation(taskID, sessionKey, title string) *conversation {
	return &conversation{
		taskID:      taskID,
		sessionKey:  sessionKey,
		title:       title,
		mapper:      NewTurnMapper(taskID, sessionKey),
		state:       taskstate.IdleAfterReply,
		updatedAt:   time.Now().Unix(),
		lastMessage: taskstate.LastMessage{},
		entries:     []tasktranscript.Entry{},
	}
}

// handleEvent runs on the client's reader goroutine for every "event"
// frame. Chat replies and agent reasoning (thinking, item, or assistant
// thinking content) for known sessions are applied; everything else is ignored.
func (source *Source) handleEvent(event string, payload json.RawMessage) {
	switch event {
	case "chat":
		source.handleChatEvent(payload)
	case "agent":
		source.handleAgentEvent(event, payload)
	case "connect.challenge":
		return
	default:
		source.logger.Info("[turnproxy] ignored gateway event", "event", event)
	}
}

func (source *Source) handleChatEvent(payload json.RawMessage) {
	var chatPayload ChatEventPayload
	if err := json.Unmarshal(payload, &chatPayload); err != nil {
		source.logger.Warn("[turnproxy] dropped unparsable chat event")
		return
	}

	source.mu.Lock()
	conv := source.bySession[chatPayload.SessionKey]
	if conv == nil {
		source.mu.Unlock()
		return
	}
	mobileEvents := conv.mapper.Apply(chatPayload)
	conv.applyRunState(chatPayload.State, chatPayload.RunID)
	reasoning := conv.mapper.ReasoningDisplay(chatPayload.RunID)
	if reasoning != "" {
		conv.upsertTranscript(tasktranscript.KindReasoning, reasoning, chatPayload.RunID)
	}
	reply := conv.mapper.ReplyDisplay(chatPayload.RunID)
	for _, mobileEvent := range mobileEvents {
		switch mobileEvent.Kind {
		case "reply":
			conv.lastMessage = taskstate.SafeLastMessage(taskstate.LastMessage{From: taskstate.SpeakerAgent, Text: mobileEvent.Summary})
			if reply == "" {
				reply = mobileEvent.Summary
			}
		case "failure", "interrupted":
			conv.appendTranscript(tasktranscript.KindActivity, mobileEvent.Summary, chatPayload.RunID)
		}
	}
	if reply != "" {
		conv.upsertTranscript(tasktranscript.KindAgent, reply, chatPayload.RunID)
	}
	source.logger.Info("[turnproxy] applied chat event", "gateway_state", chatPayload.State, "run_id", chatPayload.RunID, "emitted", len(mobileEvents), "task_state", string(conv.state), "task_id", conv.taskID, "reply_runes", utf8.RuneCountInString(reply), "reasoning_runes", utf8.RuneCountInString(reasoning))
	source.mu.Unlock()

	source.publish(mobileEvents)
}

func (source *Source) handleAgentEvent(event string, payload json.RawMessage) {
	agentPayload, ok := NormalizeAgentEvent(payload)
	sessionKeyPresent := ok && agentPayload.SessionKey != ""
	if !ok {
		source.logAgentEvent(event, "", "", false, false, "unparsable", 0, "", "", "", 0)
		return
	}
	if agentPayload.RunID == "" {
		source.logAgentEvent(event, agentPayload.Stream, "", sessionKeyPresent, false, "missing_run_id", 0, "", agentPayload.ItemType, agentPayload.DataType, 0)
		return
	}
	if !agentPayload.carriesReasoning() {
		source.logAgentEvent(event, agentPayload.Stream, agentPayload.RunID, sessionKeyPresent, false, "not_reasoning", 0, "", agentPayload.ItemType, agentPayload.DataType, 0)
		return
	}

	source.mu.Lock()
	conv, dropReason := source.conversationForThinkingLocked(agentPayload.SessionKey, agentPayload.RunID)
	if conv == nil {
		source.mu.Unlock()
		source.logAgentEvent(event, agentPayload.Stream, agentPayload.RunID, sessionKeyPresent, false, dropReason, 0, "", agentPayload.ItemType, agentPayload.DataType, 0)
		return
	}
	agentPayload.SessionKey = conv.sessionKey
	mobileEvents := conv.mapper.ApplyAgent(agentPayload)
	if len(mobileEvents) > 0 {
		conv.state = taskstate.Working
		conv.activeTurnID = agentPayload.RunID
		conv.updatedAt = time.Now().Unix()
	}
	reasoning := conv.mapper.ReasoningDisplay(agentPayload.RunID)
	if reasoning != "" {
		conv.upsertTranscript(tasktranscript.KindReasoning, reasoning, agentPayload.RunID)
	}
	taskID := conv.taskID
	emitted := len(mobileEvents)
	applied := emitted > 0
	if !applied {
		if conv.mapper.Finished(agentPayload.RunID) {
			dropReason = "finished_run"
		} else if reasoning == "" {
			dropReason = "no_text"
		} else {
			dropReason = "unchanged"
		}
	} else {
		dropReason = ""
	}
	reasoningRunes := utf8.RuneCountInString(reasoning)
	source.mu.Unlock()

	source.logAgentEvent(event, agentPayload.Stream, agentPayload.RunID, sessionKeyPresent, applied, dropReason, emitted, taskID, agentPayload.ItemType, agentPayload.DataType, reasoningRunes)
	source.publish(mobileEvents)
}

func (source *Source) logAgentEvent(event, stream, runID string, sessionKeyPresent, applied bool, dropReason string, emitted int, taskID, itemType, dataType string, reasoningRunes int) {
	source.logger.Info("[turnproxy] received agent event",
		"event", event,
		"stream", stream,
		"item_type", itemType,
		"data_type", dataType,
		"session_key_present", sessionKeyPresent,
		"run_id", runID,
		"applied", applied,
		"drop_reason", dropReason,
		"emitted", emitted,
		"task_id", taskID,
		"reasoning_runes", reasoningRunes,
	)
}

func (source *Source) conversationForThinkingLocked(sessionKey, runID string) (*conversation, string) {
	if runID != "" {
		var match *conversation
		for _, conv := range source.conversations {
			if !conv.ownsRun(runID) {
				continue
			}
			if match != nil {
				return nil, "ambiguous_run"
			}
			match = conv
		}
		if match != nil {
			return match, ""
		}
		if sessionKey == "" {
			return nil, "unknown_run"
		}
		// A sessionKey hit is not enough when runId is present but matches
		// no in-flight turn: that is how home-chat thinking stamped
		// agent:main:main used to land on the inbound phone-agent row.
		if source.bySession[sessionKey] == nil {
			return nil, "unknown_session"
		}
		return nil, "unknown_run"
	}
	if sessionKey != "" {
		if conv := source.bySession[sessionKey]; conv != nil {
			return conv, ""
		}
		return nil, "unknown_session"
	}
	return nil, "unknown_run"
}

func (conv *conversation) ownsRun(runID string) bool {
	if runID == "" {
		return false
	}
	if conv.activeTurnID == runID || conv.pendingRunID == runID || conv.sentRunID == runID || conv.mapper.KnowsRun(runID) {
		return true
	}
	start := len(conv.entries) - 4
	if start < 0 {
		start = 0
	}
	for index := len(conv.entries) - 1; index >= start; index-- {
		if conv.entries[index].TurnID == runID {
			return true
		}
	}
	return false
}

func (source *Source) publish(mobileEvents []taskstate.MobileEvent) {
	for _, mobileEvent := range mobileEvents {
		if err := source.cfg.Publisher.PublishTaskEvent(context.Background(), mobileEvent); err != nil {
			source.logger.Error("[turnproxy] publish task event failed", "kind", mobileEvent.Kind, "error", err.Error())
		}
	}
}

func (conv *conversation) applyRunState(state, runID string) {
	switch state {
	case "delta":
		conv.state = taskstate.Working
		conv.activeTurnID = runID
	case "final":
		conv.state = taskstate.IdleAfterReply
		conv.activeTurnID = ""
		if conv.pendingRunID == runID {
			conv.pendingRunID = ""
		}
	case "error":
		conv.state = taskstate.Failed
		conv.activeTurnID = ""
		if conv.pendingRunID == runID {
			conv.pendingRunID = ""
		}
	case "aborted":
		conv.state = taskstate.Interrupted
		conv.activeTurnID = ""
		if conv.pendingRunID == runID {
			conv.pendingRunID = ""
		}
	default:
		return
	}
	conv.updatedAt = time.Now().Unix()
}

// StartExistingTurn sends chat.send. The inbound phone-agent task continues
// that session. HomeComposeTaskID allocates a new phone-chat task/session
// so Home compose does not append to the inbound transcript.
func (source *Source) StartExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	return source.sendChat(ctx, taskID, prompt, taskstate.LastMessage{From: taskstate.SpeakerUser, Text: prompt}, true)
}

// StartTriggeredTurn sends chat.send the same way StartExistingTurn does,
// but for a turn the Beeper watcher started on the owner's behalf rather
// than the owner typing it: the Home preview shows preview (a plain,
// speakerless line) instead of the trigger prompt itself, since the prompt
// is never something the owner said.
func (source *Source) StartTriggeredTurn(ctx context.Context, taskID, prompt, preview string) (taskadapter.ExistingTaskResult, error) {
	return source.sendChat(ctx, taskID, prompt, taskstate.LastMessage{From: taskstate.SpeakerPlain, Text: preview}, false)
}

// sendChat is the shared chat.send path StartExistingTurn and
// StartTriggeredTurn both use; only the LastMessage they stamp afterward
// and whether thinking is requested differ.
func (source *Source) sendChat(ctx context.Context, taskID, prompt string, lastMessage taskstate.LastMessage, requestThinking bool) (taskadapter.ExistingTaskResult, error) {
	source.mu.Lock()
	conv, err := source.conversationForSendLocked(taskID, prompt)
	source.mu.Unlock()
	if err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	idempotencyKey := newIdempotencyKey()
	source.mu.Lock()
	conv.pendingRunID = idempotencyKey
	conv.sentRunID = idempotencyKey
	source.mu.Unlock()
	params := chatSendParams{
		SessionKey:     conv.sessionKey,
		Message:        prompt,
		IdempotencyKey: idempotencyKey,
	}
	if requestThinking {
		params.Thinking = userThinkingLevel
	}
	payload, err := source.client.request(ctx, "chat.send", params)
	if err != nil {
		source.mu.Lock()
		if conv.pendingRunID == idempotencyKey {
			conv.pendingRunID = ""
			conv.sentRunID = ""
		}
		source.mu.Unlock()
		return taskadapter.ExistingTaskResult{}, err
	}
	turnID := idempotencyKey
	if serverRunID := serverAssignedRunID(payload); serverRunID != "" {
		turnID = serverRunID
	}
	source.mu.Lock()
	conv.pendingRunID = turnID
	conv.sentRunID = turnID
	conv.lastMessage = taskstate.SafeLastMessage(lastMessage)
	conv.appendTranscript(kindFromSpeaker(lastMessage.From), lastMessage.Text, turnID)
	conv.state = taskstate.Working
	conv.activeTurnID = turnID
	conv.updatedAt = time.Now().Unix()
	working := conv.mapper.StartRun(turnID)
	source.mu.Unlock()
	source.logger.Info("[turnproxy] chat.send accepted", "task_id", conv.taskID, "session_key", conv.sessionKey, "turn_id", turnID, "thinking", requestThinking)
	source.publish(working)
	return taskadapter.ExistingTaskResult{ThreadID: conv.taskID, TurnID: turnID}, nil
}

func (source *Source) conversationForSendLocked(taskID, prompt string) (*conversation, error) {
	if taskID == HomeComposeTaskID {
		conv := source.allocateHomeChatLocked(prompt)
		source.logger.Info("[turnproxy] allocated home conversation", "task_id", conv.taskID, "session_key", conv.sessionKey)
		return conv, nil
	}
	conv := source.conversations[taskID]
	if conv == nil {
		return nil, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	return conv, nil
}

func (source *Source) allocateHomeChatLocked(prompt string) *conversation {
	taskID, sessionKey := newHomeConversationIDs()
	title := taskstate.SafeDisplay(prompt, "Phone chat", 256)
	conv := newConversation(taskID, sessionKey, title)
	source.conversations[taskID] = conv
	source.bySession[sessionKey] = conv
	return conv
}

func newHomeConversationIDs() (taskID, sessionKey string) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		return homeChatPrefix + suffix, homeSessionPrefix + suffix
	}
	suffix := hex.EncodeToString(buf)
	return homeChatPrefix + suffix, homeSessionPrefix + suffix
}

// serverAssignedRunID extracts a non-empty "runId" string field from a
// chat.send ack payload, or "" if the payload carries none. The protocol
// doc marks this ack shape as unknown, so a missing or malformed field is
// not an error — the caller falls back to the idempotency key it generated.
func serverAssignedRunID(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var body struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	return body.RunID
}

// RedirectExistingTurn sends sessions.steer, which the gateway treats as
// chat.send with interruptIfActive: true. Its session field is "key", not
// "sessionKey" — the gateway validates that strictly.
func (source *Source) RedirectExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	source.mu.Lock()
	conv := source.conversations[taskID]
	source.mu.Unlock()
	if conv == nil {
		return taskadapter.ExistingTaskResult{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	params := steerParams{Key: conv.sessionKey, Message: prompt}
	if _, err := source.client.request(ctx, "sessions.steer", params); err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	source.mu.Lock()
	conv.lastMessage = taskstate.SafeLastMessage(taskstate.LastMessage{From: taskstate.SpeakerUser, Text: prompt})
	conv.appendTranscript(tasktranscript.KindUser, prompt, "steer")
	source.mu.Unlock()
	return taskadapter.ExistingTaskResult{ThreadID: conv.taskID}, nil
}

// InterruptExistingTurn sends chat.abort for the session's active run.
func (source *Source) InterruptExistingTurn(ctx context.Context, taskID string) (taskadapter.ExistingTaskResult, error) {
	source.mu.Lock()
	conv := source.conversations[taskID]
	source.mu.Unlock()
	if conv == nil {
		return taskadapter.ExistingTaskResult{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	params := abortParams{SessionKey: conv.sessionKey}
	if _, err := source.client.request(ctx, "chat.abort", params); err != nil {
		return taskadapter.ExistingTaskResult{}, err
	}
	return taskadapter.ExistingTaskResult{ThreadID: conv.taskID}, nil
}

// CurrentTask returns a known phone conversation, or the virtual Home
// compose inbox used only as a start_turn target.
func (source *Source) CurrentTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	if taskID == HomeComposeTaskID {
		return source.composeInbox(), nil
	}
	source.mu.Lock()
	conv := source.conversations[taskID]
	source.mu.Unlock()
	if conv == nil {
		return taskstate.Task{}, fmt.Errorf("turnproxy: unknown task %q", taskID)
	}
	return conv.snapshot(), nil
}

func (source *Source) composeInbox() taskstate.Task {
	return taskstate.Task{
		ID:            HomeComposeTaskID,
		Title:         "New chat",
		State:         taskstate.IdleAfterReply,
		UpdatedAtUnix: time.Now().Unix(),
		Source:        taskstate.SourceAppServer,
	}
}

// ListRecent returns the inbound phone-agent task plus recent Home chats.
// The compose inbox is not listed — it is only a send target.
func (source *Source) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	if limit < 1 {
		limit = taskstate.MaxHomeTasks
	}
	if limit > taskstate.MaxHomeTasks {
		limit = taskstate.MaxHomeTasks
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	inbound := source.conversations[source.cfg.TaskID]
	chats := make([]*conversation, 0, len(source.conversations))
	for id, conv := range source.conversations {
		if id == source.cfg.TaskID {
			continue
		}
		chats = append(chats, conv)
	}
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].updatedAt == chats[j].updatedAt {
			return chats[i].taskID > chats[j].taskID
		}
		return chats[i].updatedAt > chats[j].updatedAt
	})
	tasks := make([]taskstate.Task, 0, limit)
	for _, conv := range chats {
		if len(tasks) >= limit-1 && inbound != nil {
			break
		}
		if len(tasks) >= limit {
			break
		}
		tasks = append(tasks, conv.snapshot())
	}
	if inbound != nil && len(tasks) < limit {
		tasks = append(tasks, inbound.snapshot())
	}
	return tasks, nil
}

// ReadTranscript returns the last known user/agent/reasoning lines this
// Source has seen in this connection (seeded last message, prompts sent,
// replies streamed). It is not a full gateway history.
func (source *Source) ReadTranscript(_ context.Context, taskID string, options tasktranscript.PageOptions) (tasktranscript.Page, error) {
	if options.TaskID != "" && options.TaskID != taskID {
		return tasktranscript.Page{}, tasktranscript.ErrTaskMismatch
	}
	if taskID == HomeComposeTaskID {
		return tasktranscript.Page{}, tasktranscript.ErrTaskMismatch
	}
	if options.Limit < 1 || options.Limit > tasktranscript.MaxPageEntries {
		return tasktranscript.Page{}, tasktranscript.ErrInvalidTranscript
	}
	if options.BeforeEntryID != "" && !validTranscriptID(options.BeforeEntryID) {
		return tasktranscript.Page{}, tasktranscript.ErrInvalidTranscript
	}

	source.mu.Lock()
	conv := source.conversations[taskID]
	if conv == nil {
		source.mu.Unlock()
		return tasktranscript.Page{}, tasktranscript.ErrTaskMismatch
	}
	entries := append([]tasktranscript.Entry(nil), conv.entries...)
	source.mu.Unlock()

	end := len(entries)
	if options.BeforeEntryID != "" {
		end = -1
		for index := range entries {
			if entries[index].ID == options.BeforeEntryID {
				end = index
				break
			}
		}
		if end < 0 {
			return tasktranscript.Page{}, tasktranscript.ErrUnknownCursor
		}
	}
	start := end - options.Limit
	if start < 0 {
		start = 0
	}
	page := tasktranscript.Page{
		TaskID:    taskID,
		Entries:   append([]tasktranscript.Entry(nil), entries[start:end]...),
		Truncated: false,
	}
	if page.Entries == nil {
		page.Entries = []tasktranscript.Entry{}
	}
	if start > 0 && len(page.Entries) != 0 {
		page.EarlierCursor = page.Entries[0].ID
	}
	source.logger.Info("[turnproxy] transcript read", "task_id", taskID, "entry_count", len(page.Entries), "has_earlier", page.EarlierCursor != "")
	return page, nil
}

func (conv *conversation) snapshot() taskstate.Task {
	activeTurnID := ""
	if conv.state == taskstate.Working {
		activeTurnID = conv.activeTurnID
	}
	return taskstate.Task{
		ID:            conv.taskID,
		Title:         conv.title,
		State:         conv.state,
		ActiveTurnID:  activeTurnID,
		CanRedirect:   conv.state == taskstate.Working,
		UpdatedAtUnix: conv.updatedAt,
		Source:        taskstate.SourceAppServer,
		LastMessage:   taskstate.SafeLastMessage(conv.lastMessage),
	}
}

// Done returns a channel that closes when the underlying gateway connection
// drops — the reader goroutine's readLoop exiting, whether from a clean
// Close or the socket dying underneath it. The runtime selects on it to
// notice a dead connection and redial.
func (source *Source) Done() <-chan struct{} {
	return source.client.done
}

// Close stops the reader goroutine and closes the gateway connection.
// Idempotent, so it is safe to defer in tests via t.Cleanup.
func (source *Source) Close() error {
	if source.client == nil {
		return nil
	}
	return source.client.Close()
}

func (conv *conversation) upsertTranscript(kind tasktranscript.Kind, text, turnID string) {
	text = boundTranscriptText(text)
	if text == "" {
		return
	}
	if !validTranscriptID(turnID) {
		turnID = fmt.Sprintf("turn-%d", conv.nextEntry+1)
	}
	for index := len(conv.entries) - 1; index >= 0; index-- {
		entry := conv.entries[index]
		if entry.TurnID == turnID && entry.Kind == kind {
			conv.entries[index].Text = text
			return
		}
	}
	conv.appendTranscript(kind, text, turnID)
}

func (conv *conversation) appendTranscript(kind tasktranscript.Kind, text, turnID string) {
	text = boundTranscriptText(text)
	if text == "" {
		return
	}
	conv.nextEntry++
	entryID := fmt.Sprintf("entry-%d", conv.nextEntry)
	if !validTranscriptID(turnID) {
		turnID = fmt.Sprintf("turn-%d", conv.nextEntry)
	}
	conv.entries = append(conv.entries, tasktranscript.Entry{
		ID:     entryID,
		TurnID: turnID,
		Kind:   kind,
		Text:   text,
	})
	if len(conv.entries) > tasktranscript.MaxPageEntries {
		conv.entries = conv.entries[len(conv.entries)-tasktranscript.MaxPageEntries:]
	}
}

func kindFromSpeaker(from string) tasktranscript.Kind {
	switch from {
	case taskstate.SpeakerUser:
		return tasktranscript.KindUser
	case taskstate.SpeakerAgent:
		return tasktranscript.KindAgent
	default:
		return tasktranscript.KindActivity
	}
}

func boundTranscriptText(text string) string {
	if utf8.RuneCountInString(text) <= tasktranscript.MaxEntryRunes {
		return text
	}
	return string([]rune(text)[:tasktranscript.MaxEntryRunes])
}

func validTranscriptID(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
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
