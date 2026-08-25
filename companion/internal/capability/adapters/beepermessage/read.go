package beepermessage

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

// Beeper sends rich message bodies as HTML. htmlTag matches a single well-formed
// tag so plainText can drop the markup and leave the words a person came to read.
// It needs a closing '>', so a bare "<3" in a plain message is left untouched.
var htmlTag = regexp.MustCompile(`<[^>]+>`)

// A rendered message preview never shows more than this many message lines
// before folding the rest into a "…and N more" line, and never clips a
// single message past this many runes. Both numbers exist to keep every
// preview inside the codec's 8-line / 1024-char-per-line limits, which
// previewText in the tests enforces directly.
const (
	maxPreviewMessageLines = 6
	maxPreviewMessageRunes = 200
)

// resolveRead answers a Read intent. An empty subject means "what's unread
// across this network right now"; a named subject means "show me that one
// conversation". Neither path treats an empty subject as ErrNoRecipient —
// that was the send-only adapter's rule, and reads have no recipient to
// require.
func (a *Adapter) resolveRead(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	subject := strings.TrimSpace(in.Subject)
	if subject == "" {
		return a.resolveUnreadScan(ctx)
	}
	return a.resolveNamedThread(ctx, subject)
}

func (a *Adapter) resolveUnreadScan(ctx context.Context) (adapter.Plan, error) {
	chats, err := a.api.ListChats(ctx, beeper.ListChatsOptions{Unread: true, Limit: 20})
	if err != nil {
		a.logger.Error("[beeper-message] unread scan failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "error", err)
		return adapter.Plan{}, err
	}

	kept := make([]beeper.Chat, 0, 3)
	for _, chat := range chats {
		if !strings.EqualFold(strings.TrimSpace(chat.Network), a.spec.Network) {
			continue
		}
		if chat.UnreadCount <= 0 {
			continue
		}
		kept = append(kept, chat)
		if len(kept) == 3 {
			break
		}
	}

	var allMessages []beeper.Message
	for _, chat := range kept {
		msgs, err := a.api.ListMessages(ctx, chat.ID, 5)
		if err != nil {
			a.logger.Error("[beeper-message] unread message fetch failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", chat.ID, "error", err)
			return adapter.Plan{}, err
		}
		allMessages = append(allMessages, msgs...)
	}

	preview := renderMessages(allMessages)
	if strings.TrimSpace(preview) == "" {
		preview = "No unread " + a.spec.Network + " messages."
	}
	a.logger.Info("[beeper-message] unread scan ready", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_count", len(kept), "message_count", len(allMessages))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Read, Handle: "",
		Summary: "Read " + a.spec.Network + " unread messages",
		Details: map[string]string{
			"network": a.spec.Network, "kind": "unread_scan", "preview": preview,
			"messages": encodeStructuredMessages(allMessages),
		},
	}, nil
}

func (a *Adapter) resolveNamedThread(ctx context.Context, subject string) (adapter.Plan, error) {
	chat, err := a.resolveOneChat(ctx, subject)
	if err != nil {
		return adapter.Plan{}, err
	}
	msgs, err := a.api.ListMessages(ctx, chat.ID, 10)
	if err != nil {
		a.logger.Error("[beeper-message] thread read failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", chat.ID, "error", err)
		return adapter.Plan{}, err
	}

	preview := renderMessages(msgs)
	if strings.TrimSpace(preview) == "" {
		preview = "No recent messages in this " + a.spec.Network + " conversation."
	}
	a.logger.Info("[beeper-message] thread read ready", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", chat.ID, "message_count", len(msgs))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Read, Handle: chat.ID,
		Summary: "Read " + a.spec.Network + " conversation",
		Details: map[string]string{
			"network": a.spec.Network, "kind": "thread", "conversation": chat.Title, "preview": preview,
			"messages": encodeStructuredMessages(msgs),
		},
	}, nil
}

// encodeStructuredMessages carries the received (non-own) messages behind a
// preview through to Execute, where they become the outcome's structured
// rows. Plan.Details is a map[string]string, so the slice travels as JSON;
// own sends (IsSender true) are dropped here rather than in Execute, since
// this is the one place both read paths funnel through.
func encodeStructuredMessages(messages []beeper.Message) string {
	received := make([]beeper.Message, 0, len(messages))
	for _, m := range messages {
		if m.IsSender {
			continue
		}
		received = append(received, m)
	}
	encoded, err := json.Marshal(received)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// decodeStructuredMessages turns the JSON stashed by encodeStructuredMessages
// back into the outcome rows a thread UI renders. Text keeps its newlines
// (only tags and entities are stripped, never sanitizeToLine's line-folding,
// which is for the flat Detail sentence); Sender is folded to a single
// control-free line because the wire rejects control characters anywhere in
// a message entry. A timestamp that fails RFC3339 parsing becomes a zero
// time rather than an invented one.
func decodeStructuredMessages(encoded string) []adapter.OutcomeMessage {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}
	var raw []beeper.Message
	if err := json.Unmarshal([]byte(encoded), &raw); err != nil {
		return nil
	}
	out := make([]adapter.OutcomeMessage, 0, len(raw))
	for _, m := range raw {
		sentAt, err := time.Parse(time.RFC3339, m.Timestamp)
		if err != nil {
			sentAt = time.Time{}
		}
		out = append(out, adapter.OutcomeMessage{
			Sender: sanitizeToLine(m.SenderName),
			Text:   plainText(m.Text),
			SentAt: sentAt,
		})
	}
	return out
}

// renderMessages turns a list of messages into the newline-joined text that
// both Preview (by splitting it back apart) and Execute (as the read's
// Detail) hand back to the caller. It applies the truncation rule once, at
// the source, so nothing downstream has to re-check line or rune limits.
func renderMessages(messages []beeper.Message) string {
	shown := len(messages)
	if shown > maxPreviewMessageLines {
		shown = maxPreviewMessageLines
	}
	lines := make([]string, 0, shown+1)
	for _, m := range messages[:shown] {
		lines = append(lines, renderMessageLine(m))
	}
	if len(messages) > shown {
		lines = append(lines, "…and "+strconv.Itoa(len(messages)-shown)+" more")
	}
	return strings.Join(lines, "\n")
}

func renderMessageLine(m beeper.Message) string {
	return sanitizeToLine(m.SenderName) + ": " + clipRunes(sanitizeToLine(plainText(m.Text)), maxPreviewMessageRunes)
}

// plainText turns an HTML message body into the visible text: it drops every
// tag and decodes HTML entities (so "&amp;" becomes "&"). Only the message text
// gets this; a sender name or conversation title is already plain. The tags go
// before the entities so a decoded "<" can never be mistaken for markup.
func plainText(s string) string {
	return html.UnescapeString(htmlTag.ReplaceAllString(s, " "))
}

// sanitizeToLine folds a message into a single display line. The mobile wire
// contract rejects any control character (a multi-line DM carries a newline,
// which is one), so every control rune becomes a space and runs of whitespace
// collapse to one. A message that renders to nothing after this returns the
// empty string, which the callers replace with a "no messages" sentence.
func sanitizeToLine(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsControl(r) {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// clipRunes truncates s to at most n runes, appending an ellipsis when it
// clips. It counts runes rather than bytes so multi-byte characters are
// never split mid-rune.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (a *Adapter) previewRead(plan adapter.Plan) (adapter.Preview, error) {
	// The wire contract needs a non-empty confirm label and a control-free
	// headline no longer than 256 runes. The conversation title comes from the
	// network and can be multi-line or very long, so it is sanitized and
	// clipped before it goes into the headline.
	var headline, confirm string
	switch plan.Details["kind"] {
	case "unread_scan":
		headline = "Unread " + plan.Details["network"] + " messages"
		confirm = "Show unread"
	default:
		headline = plan.Details["network"] + " conversation: " + clipRunes(sanitizeToLine(plan.Details["conversation"]), maxPreviewMessageRunes)
		confirm = "Show conversation"
	}
	a.logger.Info("[beeper-message] preview", "adapter_id", a.spec.ID, "network", a.spec.Network, "verb", manifest.Read, "kind", plan.Details["kind"])
	return adapter.Preview{
		Plan:     plan,
		Headline: headline,
		Lines:    strings.Split(plan.Details["preview"], "\n"),
		Confirm:  confirm,
	}, nil
}

func (a *Adapter) executeRead(plan adapter.Plan) (adapter.Outcome, error) {
	a.logger.Info("[beeper-message] execute", "adapter_id", a.spec.ID, "network", a.spec.Network, "verb", manifest.Read, "kind", plan.Details["kind"])
	// The result detail is a single display string, not a list, so the newlines
	// that separate the rendered messages have to become an inline separator.
	detail := strings.ReplaceAll(plan.Details["preview"], "\n", " · ")
	return adapter.Outcome{
		Reached: manifest.Completes, Done: true, Detail: detail,
		Messages: decodeStructuredMessages(plan.Details["messages"]),
	}, nil
}
