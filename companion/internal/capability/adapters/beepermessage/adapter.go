// Package beepermessage starts confirmed conversations through the user's
// connected Beeper account. It is separate from notification reply: Beeper can
// start from the chat list, while notification reply can only answer a thread
// that currently has an Android reply box.
package beepermessage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

var (
	ErrNotConnected    = errors.New("beeper message: Beeper is not connected")
	ErrNoRecipient     = errors.New("beeper message: recipient must not be empty")
	ErrEmptyMessage    = errors.New("beeper message: message must not be empty")
	ErrDeliveryPending = errors.New("beeper message: Beeper accepted the message but final thread visibility is not yet confirmed")
)

// Phone preview codec limits (ProtocolCodec.kt): at most 8 lines × 1024 chars.
const (
	previewMaxLines   = 6
	previewMsgChars   = 200
	previewCodecLines = 8
	previewCodecChars = 1024
	unreadChatCap     = 3
	unreadMsgsPerChat = 5
	namedChatMsgCap   = 10
	detailMaxChars    = 2048
)

var closedOperations = map[string]bool{
	"reply": true, "edit": true, "delete": true, "react": true, "unreact": true,
	"mark_read": true, "mark_unread": true, "archive": true, "unarchive": true,
	"pin": true, "mute": true, "set_reminder": true, "clear_reminder": true,
}

// API is the Beeper surface the adapter needs for text messaging ops.
type API interface {
	SearchChats(context.Context, string) ([]beeper.Chat, error)
	Accounts(context.Context) ([]beeper.Account, error)
	StartChat(context.Context, string, string) (beeper.Chat, error)
	Send(context.Context, string, string) (beeper.Sent, error)
	SendReply(context.Context, string, string, string) (beeper.Sent, error)
	ListChats(context.Context, beeper.ListChatsOptions) (beeper.ChatPage, error)
	GetChat(context.Context, string) (beeper.Chat, error)
	ListMessages(context.Context, string, beeper.MessageListOptions) (beeper.MessagePage, error)
	SearchMessages(context.Context, beeper.SearchMessagesOptions) (beeper.MessagePage, error)
	EditMessage(context.Context, string, string, string) (beeper.Message, error)
	DeleteMessage(context.Context, string, string) error
	React(context.Context, string, string, string) error
	Unreact(context.Context, string, string, string) error
	MarkRead(context.Context, string, string) (beeper.Chat, error)
	MarkUnread(context.Context, string, string) (beeper.Chat, error)
	Archive(context.Context, string, bool) error
	UpdateChat(context.Context, string, beeper.UpdateChatOptions) (beeper.Chat, error)
	SetReminder(context.Context, string, time.Time, bool) error
	ClearReminder(context.Context, string) error
}

// Spec binds one Operator adapter id to one Beeper network.
type Spec struct {
	ID           string
	Network      string
	Auth         manifest.Auth
	Unshipped    string
	StartByPhone bool
}

func ProductionSpecs() []Spec {
	return []Spec{
		{ID: "instagram", Network: "Instagram"},
		{ID: "discord", Network: "Discord"},
		{ID: "messages", Network: "Google Messages", StartByPhone: true},
	}
}

// Fact-force (edit): callers=runtime/production.go NewProduction Beeper
// branch + adapter tests; existing adapter.go (not new). User: Slice 4 B4+B5.
type Adapter struct {
	spec     Spec
	api      API
	revoke   func(context.Context) error
	revoked  bool
	readOnly bool
	logger   *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(spec Spec, api API, logger *slog.Logger) *Adapter {
	return NewWithRevoke(spec, api, nil, logger)
}

func NewWithRevoke(spec Spec, api API, revoke func(context.Context) error, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{spec: spec, api: api, revoke: revoke, logger: logger}
}

// NewReadOnlyWithRevoke registers the same network adapter with Verbs:[read]
// only (BEEPER_READONLY). Writes are refused by stage-2 verb filter and by a
// read-only Beeper client when callers pass one.
func NewReadOnlyWithRevoke(spec Spec, api API, revoke func(context.Context) error, logger *slog.Logger) *Adapter {
	a := NewWithRevoke(spec, api, revoke, logger)
	a.readOnly = true
	return a
}

func (a *Adapter) Describe() manifest.Manifest {
	auth := a.spec.Auth
	if auth == "" {
		auth = manifest.AuthOAuth
	}
	verbs := []manifest.Verb{manifest.Read, manifest.Send, manifest.Modify, manifest.Cancel}
	if a.readOnly {
		verbs = []manifest.Verb{manifest.Read}
	}
	return manifest.Manifest{
		ID: a.spec.ID, Runtime: manifest.RT2,
		Verbs:   verbs,
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: auth, Cost: manifest.CostFree,
		Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region: []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "beeper_" + a.spec.ID + "_confirmed_send_visible_in_thread",
		Unshipped:     a.spec.Unshipped,
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	recipient, recipientSource := recipientFromIntent(in)
	a.logger.Info("[beeper-message] resolve",
		"adapter_id", a.spec.ID, "network", a.spec.Network, "verb", in.Verb,
		"subject_length", len(in.Subject), "handle_length", len(in.Handle),
		"body_length", len(in.Body), "recipient_length", len(recipient),
		"recipient_source", recipientSource,
		"operation", strings.TrimSpace(in.Fields["operation"]))
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	switch in.Verb {
	case manifest.Read:
		return a.resolveRead(ctx, in)
	case manifest.Send:
		return a.resolveSend(ctx, in)
	case manifest.Modify, manifest.Cancel:
		return a.resolveManage(ctx, in)
	default:
		return adapter.Plan{}, fmt.Errorf("beeper message: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveRead(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	recipient, _ := recipientFromIntent(in)
	body := strings.TrimSpace(in.Body)
	switch {
	case recipient == "" && body != "":
		return a.resolveSearchRead(ctx, body)
	case recipient == "":
		return a.resolveUnreadScan(ctx)
	default:
		return a.resolveNamedRead(ctx, recipient)
	}
}

func (a *Adapter) resolveSearchRead(ctx context.Context, query string) (adapter.Plan, error) {
	page, err := a.api.SearchMessages(ctx, beeper.SearchMessagesOptions{Query: query, Limit: namedChatMsgCap})
	if err != nil {
		return adapter.Plan{}, err
	}
	lines, more := renderMessageLines(page.Items, previewMaxLines)
	details := map[string]string{
		"network": a.spec.Network, "mode": "search", "query": query,
		"preview_lines": strings.Join(lines, "\n"),
		"more_count":    fmt.Sprintf("%d", more),
	}
	a.logger.Info("[beeper-message] search read ready",
		"adapter_id", a.spec.ID, "hit_count", len(page.Items), "query_length", len(query))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Read,
		Summary: "Search " + a.spec.Network + " messages",
		Details: details,
	}, nil
}

func (a *Adapter) resolveUnreadScan(ctx context.Context) (adapter.Plan, error) {
	unread, err := a.collectUnreadChats(ctx)
	if err != nil {
		return adapter.Plan{}, err
	}
	if len(unread) == 0 {
		return adapter.Plan{
			AdapterID: a.spec.ID, Verb: manifest.Read,
			Summary: "No unread " + a.spec.Network + " messages",
			Details: map[string]string{
				"network": a.spec.Network, "mode": "unread_scan",
				"preview_lines": "No unread " + a.spec.Network + " conversations.",
				"more_count":    "0",
			},
		}, nil
	}
	var lines []string
	for _, chat := range unread {
		remaining := previewMaxLines - len(lines)
		if remaining <= 0 {
			break
		}
		page, listErr := a.api.ListMessages(ctx, chat.ID, beeper.MessageListOptions{})
		if listErr != nil {
			return adapter.Plan{}, listErr
		}
		msgs := filterUnreadPrefer(page.Items, unreadMsgsPerChat)
		lines = append(lines, renderChatMessages(chat.Title, msgs, remaining)...)
	}
	details := map[string]string{
		"network": a.spec.Network, "mode": "unread_scan",
		"preview_lines": strings.Join(lines, "\n"),
		"more_count":    "0",
		"chat_count":    fmt.Sprintf("%d", len(unread)),
	}
	a.logger.Info("[beeper-message] unread scan ready",
		"adapter_id", a.spec.ID, "unread_chats", len(unread), "line_count", len(lines))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Read,
		Summary: "Read unread " + a.spec.Network + " messages",
		Details: details,
	}, nil
}

func (a *Adapter) resolveNamedRead(ctx context.Context, subject string) (adapter.Plan, error) {
	chat, err := a.resolveNetworkChat(ctx, subject)
	if err != nil {
		return adapter.Plan{}, err
	}
	page, err := a.api.ListMessages(ctx, chat.ID, beeper.MessageListOptions{})
	if err != nil {
		return adapter.Plan{}, err
	}
	msgs := page.Items
	if len(msgs) > namedChatMsgCap {
		msgs = msgs[:namedChatMsgCap]
	}
	lines, more := renderMessageLines(msgs, previewMaxLines)
	details := map[string]string{
		"network": a.spec.Network, "mode": "named", "conversation": chat.Title,
		"preview_lines": strings.Join(lines, "\n"),
		"more_count":    fmt.Sprintf("%d", more),
	}
	a.logger.Info("[beeper-message] named read ready",
		"adapter_id", a.spec.ID, "chat_id", chat.ID, "message_count", len(msgs))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Read, Handle: chat.ID,
		Summary: "Read " + a.spec.Network + " conversation",
		Details: details,
	}, nil
}

func (a *Adapter) collectUnreadChats(ctx context.Context) ([]beeper.Chat, error) {
	var unread []beeper.Chat
	cursor := ""
	for pages := 0; pages < 10 && len(unread) < unreadChatCap; pages++ {
		page, err := a.api.ListChats(ctx, beeper.ListChatsOptions{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, chat := range page.Items {
			if !strings.EqualFold(strings.TrimSpace(chat.Network), a.spec.Network) {
				continue
			}
			if chat.UnreadCount <= 0 {
				continue
			}
			unread = append(unread, chat)
		}
		if !page.HasMore || page.OldestCursor == "" || page.OldestCursor == cursor {
			break
		}
		cursor = page.OldestCursor
	}
	sort.SliceStable(unread, func(i, j int) bool {
		return unread[i].LastActivity > unread[j].LastActivity
	})
	if len(unread) > unreadChatCap {
		unread = unread[:unreadChatCap]
	}
	return unread, nil
}

func (a *Adapter) resolveSend(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	recipient, source := recipientFromIntent(in)
	if recipient == "" {
		a.logger.Info("[beeper-message] send refused: no recipient",
			"adapter_id", a.spec.ID, "network", a.spec.Network)
		return adapter.Plan{}, ErrNoRecipient
	}
	a.logger.Info("[beeper-message] send recipient resolved",
		"adapter_id", a.spec.ID, "network", a.spec.Network,
		"recipient_source", source, "recipient_length", len(recipient))
	message := strings.TrimSpace(in.Body)
	if message == "" {
		return adapter.Plan{}, ErrEmptyMessage
	}
	operation := strings.TrimSpace(in.Fields["operation"])
	if operation == "reply" {
		return a.resolveReply(ctx, recipient, message)
	}
	if operation != "" {
		return adapter.Plan{}, &adapter.ClarificationError{
			Question: fmt.Sprintf("I don't recognize the messaging operation %q for a send.", operation),
		}
	}
	return a.resolvePlainSend(ctx, recipient, message)
}

func (a *Adapter) resolvePlainSend(ctx context.Context, recipient, message string) (adapter.Plan, error) {
	matches, err := a.searchNetwork(ctx, recipient)
	if err != nil {
		return adapter.Plan{}, err
	}
	if len(matches) == 0 && a.spec.StartByPhone {
		return a.resolveStartByPhone(ctx, recipient, message)
	}
	chat, err := exactlyOne(matches, a.spec.Network, recipient)
	if err != nil {
		return adapter.Plan{}, err
	}
	details := map[string]string{
		"network": a.spec.Network, "conversation": chat.Title, "text": message, "mode": "send",
	}
	a.logger.Info("[beeper-message] resolve ready", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", chat.ID, "text_length", len(message))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Send, Handle: chat.ID,
		Summary: "Send a " + a.spec.Network + " message", Details: details,
	}, nil
}

func (a *Adapter) resolveStartByPhone(ctx context.Context, recipient, message string) (adapter.Plan, error) {
	phoneNumber, ok := normalizedE164(recipient)
	if !ok {
		return adapter.Plan{}, &adapter.ClarificationError{Question: fmt.Sprintf("I couldn't find a %s conversation matching %s. Which conversation did you mean?", a.spec.Network, recipient)}
	}
	accounts, accountErr := a.api.Accounts(ctx)
	if accountErr != nil {
		a.logger.Error("[beeper-message] account discovery failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "error", accountErr)
		return adapter.Plan{}, accountErr
	}
	connected := make([]beeper.Account, 0, 1)
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Network), a.spec.Network) && strings.EqualFold(strings.TrimSpace(account.Status), "connected") {
			connected = append(connected, account)
		}
	}
	if len(connected) != 1 {
		return adapter.Plan{}, &adapter.ClarificationError{Question: fmt.Sprintf("I found %d connected %s accounts in Beeper. Please connect or select exactly one before I start this conversation.", len(connected), a.spec.Network)}
	}
	a.logger.Info("[beeper-message] exact phone chat ready for confirmed execution",
		"adapter_id", a.spec.ID, "network", a.spec.Network,
		"account_id", connected[0].ID, "phone_length", len(phoneNumber))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Send,
		Summary: "Send a " + a.spec.Network + " message",
		Details: map[string]string{
			"network": a.spec.Network, "conversation": recipient, "text": message, "mode": "send",
			"start_account_id": connected[0].ID, "start_phone": phoneNumber,
		},
	}, nil
}

func (a *Adapter) resolveReply(ctx context.Context, recipient, message string) (adapter.Plan, error) {
	chat, err := a.resolveNetworkChat(ctx, recipient)
	if err != nil {
		return adapter.Plan{}, err
	}
	full, err := a.api.GetChat(ctx, chat.ID)
	if err != nil {
		return adapter.Plan{}, err
	}
	if !beeper.Supports(full.Capabilities.Reply) {
		return adapter.Plan{}, &adapter.ClarificationError{
			Question: fmt.Sprintf("%s doesn't support replying to messages.", a.spec.Network),
		}
	}
	target, err := a.pinTargetMessage(ctx, chat.ID, "", targetIncoming)
	if err != nil {
		return adapter.Plan{}, err
	}
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Send, Handle: chat.ID,
		Summary: "Reply on " + a.spec.Network,
		Details: map[string]string{
			"network": a.spec.Network, "conversation": chat.Title, "text": message,
			"mode": "reply", "operation": "reply",
			"message_id": target.ID, "target_text": displayText(target),
		},
	}, nil
}

func (a *Adapter) resolveManage(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	operation := strings.TrimSpace(in.Fields["operation"])
	if operation == "" {
		return adapter.Plan{}, &adapter.ClarificationError{
			Question: "Which messaging change did you mean — edit, delete, react, mark read, archive, pin, mute, or set a reminder?",
		}
	}
	if !closedOperations[operation] {
		return adapter.Plan{}, &adapter.ClarificationError{
			Question: fmt.Sprintf("I don't recognize the messaging operation %q.", operation),
		}
	}
	recipient, source := recipientFromIntent(in)
	if recipient == "" {
		a.logger.Info("[beeper-message] manage refused: no recipient",
			"adapter_id", a.spec.ID, "operation", operation)
		return adapter.Plan{}, ErrNoRecipient
	}
	a.logger.Info("[beeper-message] manage recipient resolved",
		"adapter_id", a.spec.ID, "operation", operation,
		"recipient_source", source, "recipient_length", len(recipient))
	chat, err := a.resolveNetworkChat(ctx, recipient)
	if err != nil {
		return adapter.Plan{}, err
	}
	full, err := a.api.GetChat(ctx, chat.ID)
	if err != nil {
		return adapter.Plan{}, err
	}
	if err := a.checkCapability(operation, full); err != nil {
		return adapter.Plan{}, err
	}

	details := map[string]string{
		"network": a.spec.Network, "conversation": chat.Title,
		"mode": "manage", "operation": operation, "text": strings.TrimSpace(in.Body),
	}
	switch operation {
	case "edit":
		if strings.TrimSpace(in.Body) == "" {
			return adapter.Plan{}, ErrEmptyMessage
		}
		target, pinErr := a.pinTargetMessage(ctx, chat.ID, "", targetOwn)
		if pinErr != nil {
			return adapter.Plan{}, pinErr
		}
		details["message_id"] = target.ID
		details["target_text"] = displayText(target)
	case "delete":
		target, pinErr := a.pinTargetMessage(ctx, chat.ID, strings.TrimSpace(in.Body), targetOwn)
		if pinErr != nil {
			return adapter.Plan{}, pinErr
		}
		details["message_id"] = target.ID
		details["target_text"] = displayText(target)
		details["text"] = ""
	case "react", "unreact":
		if strings.TrimSpace(in.Body) == "" {
			return adapter.Plan{}, &adapter.ClarificationError{Question: "Which reaction should I use?"}
		}
		target, pinErr := a.pinTargetMessage(ctx, chat.ID, "", targetIncoming)
		if pinErr != nil {
			return adapter.Plan{}, pinErr
		}
		details["message_id"] = target.ID
		details["target_text"] = displayText(target)
	case "set_reminder":
		if strings.TrimSpace(in.Body) == "" {
			return adapter.Plan{}, &adapter.ClarificationError{Question: "When should I set the reminder?"}
		}
	}

	a.logger.Info("[beeper-message] manage resolve ready",
		"adapter_id", a.spec.ID, "operation", operation, "chat_id", chat.ID,
		"has_message_id", details["message_id"] != "")
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: in.Verb, Handle: chat.ID,
		Summary: fmt.Sprintf("%s on %s", operationLabel(operation), a.spec.Network),
		Details: details,
	}, nil
}

type targetKind int

const (
	targetOwn targetKind = iota
	targetIncoming
)

func (a *Adapter) pinTargetMessage(ctx context.Context, chatID, quote string, kind targetKind) (beeper.Message, error) {
	page, err := a.api.ListMessages(ctx, chatID, beeper.MessageListOptions{})
	if err != nil {
		return beeper.Message{}, err
	}
	quote = strings.TrimSpace(quote)
	if quote != "" {
		for _, msg := range page.Items {
			if kind == targetOwn && !msg.IsSender {
				continue
			}
			if kind == targetIncoming && msg.IsSender {
				continue
			}
			if strings.Contains(strings.ToLower(displayText(msg)), strings.ToLower(quote)) {
				return msg, nil
			}
		}
		return beeper.Message{}, &adapter.ClarificationError{
			Question: "I couldn't find a recent message matching that text. Which one did you mean?",
		}
	}
	for _, msg := range page.Items {
		if kind == targetOwn && msg.IsSender {
			return msg, nil
		}
		if kind == targetIncoming && !msg.IsSender {
			return msg, nil
		}
	}
	if kind == targetOwn {
		return beeper.Message{}, &adapter.ClarificationError{Question: "I couldn't find one of your recent messages in that conversation."}
	}
	return beeper.Message{}, &adapter.ClarificationError{Question: "I couldn't find a recent incoming message in that conversation."}
}

func (a *Adapter) checkCapability(operation string, chat beeper.Chat) error {
	caps := chat.Capabilities
	switch operation {
	case "edit":
		if !beeper.Supports(caps.Edit) {
			return &adapter.ClarificationError{Question: fmt.Sprintf("%s doesn't support editing messages.", a.spec.Network)}
		}
	case "delete":
		if !beeper.Supports(caps.Delete) {
			return &adapter.ClarificationError{Question: fmt.Sprintf("%s doesn't support deleting messages.", a.spec.Network)}
		}
	case "react", "unreact":
		if !beeper.Supports(caps.Reaction) {
			return &adapter.ClarificationError{Question: fmt.Sprintf("%s doesn't support reactions.", a.spec.Network)}
		}
	case "reply":
		if !beeper.Supports(caps.Reply) {
			return &adapter.ClarificationError{Question: fmt.Sprintf("%s doesn't support replying to messages.", a.spec.Network)}
		}
	case "archive", "unarchive":
		if !caps.Archive {
			return &adapter.ClarificationError{Question: fmt.Sprintf("%s doesn't support archiving chats.", a.spec.Network)}
		}
	case "mark_unread":
		if !caps.MarkAsUnread {
			return &adapter.ClarificationError{Question: fmt.Sprintf("%s doesn't support marking chats unread.", a.spec.Network)}
		}
	}
	return nil
}

func (a *Adapter) resolveNetworkChat(ctx context.Context, recipient string) (beeper.Chat, error) {
	matches, err := a.searchNetwork(ctx, recipient)
	if err != nil {
		return beeper.Chat{}, err
	}
	return exactlyOne(matches, a.spec.Network, recipient)
}

func (a *Adapter) searchNetwork(ctx context.Context, recipient string) ([]beeper.Chat, error) {
	chats, err := a.api.SearchChats(ctx, recipient)
	if err != nil {
		a.logger.Error("[beeper-message] chat search failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "error", err)
		return nil, err
	}
	matches := make([]beeper.Chat, 0, len(chats))
	for _, chat := range chats {
		if strings.EqualFold(strings.TrimSpace(chat.Network), a.spec.Network) {
			matches = append(matches, chat)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Title < matches[j].Title })
	return matches, nil
}

// recipientFromIntent returns the person or chat to act on, and which
// intent field it came from. Empty means fail closed.
func recipientFromIntent(in adapter.Intent) (string, string) {
	if s := strings.TrimSpace(in.Subject); s != "" {
		return s, "subject"
	}
	if s := strings.TrimSpace(in.Handle); s != "" {
		return s, "handle"
	}
	for _, key := range []string{"to", "recipient", "chat_id"} {
		if s := strings.TrimSpace(in.Fields[key]); s != "" {
			return s, "fields." + key
		}
	}
	return "", ""
}

func exactlyOne(matches []beeper.Chat, network, recipient string) (beeper.Chat, error) {
	if len(matches) == 0 {
		return beeper.Chat{}, &adapter.ClarificationError{Question: fmt.Sprintf("I couldn't find a %s conversation matching %s. Which conversation did you mean?", network, recipient)}
	}
	if len(matches) > 1 {
		titles := make([]string, len(matches))
		for i, chat := range matches {
			titles[i] = chat.Title
		}
		return beeper.Chat{}, &adapter.ClarificationError{Question: fmt.Sprintf("I found %d %s conversations matching %s: %s. Which one did you mean?", len(matches), network, recipient, strings.Join(titles, ", "))}
	}
	return matches[0], nil
}

func normalizedE164(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "+") {
		return "", false
	}
	var digits strings.Builder
	for _, r := range raw[1:] {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case unicode.IsSpace(r) || strings.ContainsRune("-().", r):
			continue
		default:
			return "", false
		}
	}
	if digits.Len() < 8 || digits.Len() > 15 {
		return "", false
	}
	return "+" + digits.String(), true
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	mode := plan.Details["mode"]
	a.logger.Info("[beeper-message] preview",
		"adapter_id", a.spec.ID, "mode", mode, "operation", plan.Details["operation"],
		"chat_id", plan.Handle)
	switch mode {
	case "unread_scan", "named", "search":
		lines := splitPreviewLines(plan.Details["preview_lines"])
		if more := plan.Details["more_count"]; more != "" && more != "0" {
			lines = append(lines, "…and "+more+" more")
		}
		lines = clampPreview(lines)
		headline := "Read " + plan.Details["network"] + " messages"
		if mode == "named" {
			headline = fmt.Sprintf("Read %s conversation with %s", plan.Details["network"], plan.Details["conversation"])
		}
		if mode == "search" {
			headline = fmt.Sprintf("Search %s for %q", plan.Details["network"], plan.Details["query"])
		}
		return adapter.Preview{Plan: plan, Headline: headline, Lines: lines, Confirm: "Got it"}, nil
	case "reply":
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Reply on %s to %s", plan.Details["network"], plan.Details["conversation"]),
			Lines: []string{
				"Network: " + plan.Details["network"],
				"Conversation: " + plan.Details["conversation"],
				"Replying to: " + clip(plan.Details["target_text"], previewMsgChars),
				plan.Details["text"],
			},
			Confirm: "Send reply",
		}, nil
	case "manage":
		return managePreview(plan), nil
	default:
		return adapter.Preview{
			Plan:     plan,
			Headline: fmt.Sprintf("Send a %s message to %s", plan.Details["network"], plan.Details["conversation"]),
			Lines: []string{
				"Network: " + plan.Details["network"],
				"Conversation: " + plan.Details["conversation"],
				plan.Details["text"],
			},
			Confirm: "Send message",
		}, nil
	}
}

func managePreview(plan adapter.Plan) adapter.Preview {
	op := plan.Details["operation"]
	lines := []string{
		"Network: " + plan.Details["network"],
		"Conversation: " + plan.Details["conversation"],
		"Operation: " + op,
	}
	if plan.Details["target_text"] != "" {
		lines = append(lines, "Message: "+clip(plan.Details["target_text"], previewMsgChars))
	}
	if plan.Details["text"] != "" && op != "delete" {
		lines = append(lines, plan.Details["text"])
	}
	return adapter.Preview{
		Plan:     plan,
		Headline: fmt.Sprintf("%s on %s (%s)", operationLabel(op), plan.Details["network"], plan.Details["conversation"]),
		Lines:    clampPreview(lines),
		Confirm:  "Confirm",
	}
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	mode := plan.Details["mode"]
	a.logger.Info("[beeper-message] execute",
		"adapter_id", a.spec.ID, "mode", mode, "operation", plan.Details["operation"], "chat_id", plan.Handle)
	switch mode {
	case "unread_scan", "named", "search":
		detail := plan.Details["preview_lines"]
		if more := plan.Details["more_count"]; more != "" && more != "0" {
			detail += "\n…and " + more + " more"
		}
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: clip(detail, detailMaxChars),
		}, nil
	case "reply":
		return a.executeSend(ctx, plan, plan.Details["message_id"])
	case "manage":
		return a.executeManage(ctx, plan)
	default:
		return a.executeSend(ctx, plan, "")
	}
}

func (a *Adapter) executeSend(ctx context.Context, plan adapter.Plan, replyTo string) (adapter.Outcome, error) {
	chatID := plan.Handle
	if chatID == "" && plan.Details["start_phone"] != "" {
		chat, err := a.api.StartChat(ctx, plan.Details["start_account_id"], plan.Details["start_phone"])
		if err != nil {
			return adapter.Outcome{}, err
		}
		if chat.ID == "" || !strings.EqualFold(strings.TrimSpace(chat.Network), a.spec.Network) {
			return adapter.Outcome{}, fmt.Errorf("beeper message: started chat did not match selected %s network", a.spec.Network)
		}
		chatID = chat.ID
	}
	var (
		sent beeper.Sent
		err  error
	)
	if replyTo != "" {
		sent, err = a.api.SendReply(ctx, chatID, plan.Details["text"], replyTo)
	} else {
		sent, err = a.api.Send(ctx, chatID, plan.Details["text"])
	}
	if err != nil {
		a.logger.Error("[beeper-message] send failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", plan.Handle, "error", err)
		return adapter.Outcome{}, &adapter.OutcomeUnknownError{AdapterID: a.spec.ID, Verb: string(manifest.Send), Cause: err}
	}
	a.logger.Info("[beeper-message] send accepted", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", sent.ChatID, "pending_message_id", sent.PendingMessageID)
	return adapter.Outcome{}, &adapter.OutcomeUnknownError{
		AdapterID: a.spec.ID, Verb: string(manifest.Send), Cause: ErrDeliveryPending,
	}
}

func (a *Adapter) executeManage(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	op := plan.Details["operation"]
	chatID := plan.Handle
	messageID := plan.Details["message_id"]
	var err error
	switch op {
	case "edit":
		_, err = a.api.EditMessage(ctx, chatID, messageID, plan.Details["text"])
	case "delete":
		err = a.api.DeleteMessage(ctx, chatID, messageID)
	case "react":
		err = a.api.React(ctx, chatID, messageID, plan.Details["text"])
	case "unreact":
		err = a.api.Unreact(ctx, chatID, messageID, plan.Details["text"])
	case "mark_read":
		_, err = a.api.MarkRead(ctx, chatID, "")
	case "mark_unread":
		_, err = a.api.MarkUnread(ctx, chatID, "")
	case "archive":
		err = a.api.Archive(ctx, chatID, true)
	case "unarchive":
		err = a.api.Archive(ctx, chatID, false)
	case "pin":
		pinned := true
		_, err = a.api.UpdateChat(ctx, chatID, beeper.UpdateChatOptions{Pinned: &pinned})
	case "mute":
		muted := true
		_, err = a.api.UpdateChat(ctx, chatID, beeper.UpdateChatOptions{Muted: &muted})
	case "set_reminder":
		when, parseErr := time.Parse(time.RFC3339, plan.Details["text"])
		if parseErr != nil {
			return adapter.Outcome{}, &adapter.ClarificationError{Question: "I need a reminder time like 2026-08-07T12:00:00Z."}
		}
		err = a.api.SetReminder(ctx, chatID, when, true)
	case "clear_reminder":
		err = a.api.ClearReminder(ctx, chatID)
	default:
		return adapter.Outcome{}, fmt.Errorf("beeper message: unsupported operation %q", op)
	}
	if err != nil {
		a.logger.Error("[beeper-message] manage failed", "adapter_id", a.spec.ID, "operation", op, "error", err)
		return adapter.Outcome{}, err
	}
	detail := fmt.Sprintf("Confirmed %s on %s (%s).", operationLabel(op), a.spec.Network, plan.Details["conversation"])
	a.logger.Info("[beeper-message] manage confirmed", "adapter_id", a.spec.ID, "operation", op, "chat_id", chatID)
	return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: detail}, nil
}

func (a *Adapter) Revoke(ctx context.Context) error {
	if a.revoked {
		return nil
	}
	if a.revoke != nil {
		if err := a.revoke(ctx); err != nil {
			a.logger.Error("[beeper-message] persistent revoke failed", "adapter_id", a.spec.ID, "error", err)
			return err
		}
	}
	a.revoked = true
	a.logger.Info("[beeper-message] revoked", "adapter_id", a.spec.ID, "decision", "routing_removed_shared_beeper_session_kept")
	return nil
}

func filterUnreadPrefer(msgs []beeper.Message, capN int) []beeper.Message {
	var unread []beeper.Message
	for _, m := range msgs {
		if m.IsUnread {
			unread = append(unread, m)
		}
	}
	if len(unread) == 0 {
		unread = msgs
	}
	if len(unread) > capN {
		unread = unread[:capN]
	}
	return unread
}

func renderChatMessages(title string, msgs []beeper.Message, maxLines int) []string {
	lines, _ := renderMessageLines(msgs, maxLines)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, title+" · "+line)
	}
	return out
}

func renderMessageLines(msgs []beeper.Message, maxLines int) ([]string, int) {
	if maxLines <= 0 {
		return nil, len(msgs)
	}
	n := len(msgs)
	if n > maxLines {
		n = maxLines
	}
	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lines = append(lines, formatMessageLine(msgs[i]))
	}
	return lines, len(msgs) - n
}

func formatMessageLine(msg beeper.Message) string {
	sender := strings.TrimSpace(msg.SenderName)
	if sender == "" {
		if msg.IsSender {
			sender = "You"
		} else {
			sender = "Someone"
		}
	}
	return sender + ": " + clip(displayText(msg), previewMsgChars)
}

func displayText(msg beeper.Message) string {
	text := strings.TrimSpace(msg.Text)
	if text != "" {
		return text
	}
	if len(msg.Attachments) > 0 {
		kind := strings.TrimSpace(msg.Attachments[0].Type)
		if kind == "" {
			kind = "file"
		}
		return "[attachment: " + kind + "]"
	}
	return "[empty message]"
}

func splitPreviewLines(joined string) []string {
	joined = strings.TrimSpace(joined)
	if joined == "" {
		return []string{"(no messages)"}
	}
	return strings.Split(joined, "\n")
}

func clampPreview(lines []string) []string {
	if len(lines) > previewCodecLines {
		lines = lines[:previewCodecLines]
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = clip(line, previewCodecChars)
	}
	return out
}

func clip(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max < 2 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func operationLabel(op string) string {
	return strings.ReplaceAll(op, "_", " ")
}
