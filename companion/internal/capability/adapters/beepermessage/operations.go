package beepermessage

import (
	"context"
	"fmt"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

// messageTarget names which message within a resolved chat a manage
// operation acts on. Some operations act on the chat itself and need no
// message at all.
type messageTarget int

const (
	targetChat messageTarget = iota
	targetOwnLatest
	targetIncomingLatest
)

// manageOpSpec is one row of the manage-operation registry: what it targets,
// whether it needs body text, whether its outcome is delivery-pending, and
// how it calls the Beeper API. Keeping every operation's shape in this table
// (instead of a growing switch per concern) is what lets resolveManage and
// executeManage stay two short, uniform functions no matter how many
// operations the table grows to.
type manageOpSpec struct {
	target       messageTarget
	bodyRequired bool
	pending      bool
	dispatch     func(ctx context.Context, api API, chatID, messageID, body string) error
}

func boolPtr(b bool) *bool { return &b }

var manageOps = map[string]manageOpSpec{
	"reply": {
		target: targetIncomingLatest, bodyRequired: true, pending: true,
		dispatch: func(ctx context.Context, api API, chatID, messageID, body string) error {
			_, err := api.Reply(ctx, chatID, messageID, body)
			return err
		},
	},
	"edit": {
		target: targetOwnLatest, bodyRequired: true,
		dispatch: func(ctx context.Context, api API, chatID, messageID, body string) error {
			return api.EditMessage(ctx, chatID, messageID, body)
		},
	},
	"delete": {
		target: targetOwnLatest,
		dispatch: func(ctx context.Context, api API, chatID, messageID, _ string) error {
			return api.DeleteMessage(ctx, chatID, messageID)
		},
	},
	"react": {
		target: targetIncomingLatest, bodyRequired: true,
		dispatch: func(ctx context.Context, api API, chatID, messageID, body string) error {
			return api.React(ctx, chatID, messageID, body)
		},
	},
	"unreact": {
		target: targetIncomingLatest, bodyRequired: true,
		dispatch: func(ctx context.Context, api API, chatID, messageID, body string) error {
			return api.Unreact(ctx, chatID, messageID, body)
		},
	},
	"mark_read": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.MarkRead(ctx, chatID)
		},
	},
	"mark_unread": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.MarkUnread(ctx, chatID)
		},
	},
	"archive": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.Archive(ctx, chatID, true)
		},
	},
	"unarchive": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.Archive(ctx, chatID, false)
		},
	},
	"pin": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.UpdateChat(ctx, chatID, beeper.ChatState{Pinned: boolPtr(true)})
		},
	},
	"mute": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.UpdateChat(ctx, chatID, beeper.ChatState{Muted: boolPtr(true)})
		},
	},
	"set_reminder": {
		target: targetChat, bodyRequired: true,
		dispatch: func(ctx context.Context, api API, chatID, _, body string) error {
			return api.SetReminder(ctx, chatID, body)
		},
	},
	"clear_reminder": {
		target: targetChat,
		dispatch: func(ctx context.Context, api API, chatID, _, _ string) error {
			return api.ClearReminder(ctx, chatID)
		},
	},
}

// resolveManage answers a Modify/Cancel intent (or any intent carrying an
// "operation" field): look up the operation, resolve its conversation the
// same way send does, then pin down which message (if any) it targets by
// recency, so Execute can dispatch without re-resolving anything.
func (a *Adapter) resolveManage(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	operation := strings.TrimSpace(in.Fields["operation"])
	spec, ok := manageOps[operation]
	if !ok {
		return adapter.Plan{}, &adapter.ClarificationError{Question: fmt.Sprintf("I'm not sure what you want me to do with that %s message — try reply, edit, delete, react, mark read/unread, archive, pin, mute, or a reminder.", a.spec.Network)}
	}

	subject := strings.TrimSpace(in.Subject)
	if subject == "" {
		return adapter.Plan{}, ErrNoRecipient
	}
	body := strings.TrimSpace(in.Body)
	if spec.bodyRequired && body == "" {
		return adapter.Plan{}, ErrEmptyMessage
	}

	chat, err := a.resolveOneChat(ctx, subject)
	if err != nil {
		return adapter.Plan{}, err
	}

	details := map[string]string{
		"network": a.spec.Network, "operation": operation, "conversation": chat.Title,
	}
	if body != "" {
		details["body"] = body
	}

	if spec.target != targetChat {
		msgs, err := a.api.ListMessages(ctx, chat.ID, 20)
		if err != nil {
			a.logger.Error("[beeper-message] manage message list failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "operation", operation, "chat_id", chat.ID, "error", err)
			return adapter.Plan{}, err
		}
		msg, found := lastMatching(msgs, spec.target == targetOwnLatest)
		if !found {
			return adapter.Plan{}, &adapter.ClarificationError{Question: fmt.Sprintf("I couldn't find a message to %s in that conversation.", operation)}
		}
		details["message_id"] = msg.ID
		details["target_text"] = msg.Text
	}

	a.logger.Info("[beeper-message] manage resolve ready", "adapter_id", a.spec.ID, "network", a.spec.Network, "operation", operation, "chat_id", chat.ID, "message_id", details["message_id"])
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: in.Verb, Handle: chat.ID,
		Summary: manageHeadline(operation, a.spec.Network, chat.Title),
		Details: details,
	}, nil
}

// lastMatching returns the most recent message with IsSender == wantSender.
// Beeper returns messages oldest-first, so the most recent match is found by
// scanning from the end.
func lastMatching(msgs []beeper.Message, wantSender bool) (beeper.Message, bool) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].IsSender == wantSender {
			return msgs[i], true
		}
	}
	return beeper.Message{}, false
}

func manageHeadline(operation, network, conversation string) string {
	switch operation {
	case "reply":
		return fmt.Sprintf("Reply to %s on %s", conversation, network)
	case "edit":
		return fmt.Sprintf("Edit your last message to %s on %s", conversation, network)
	case "delete":
		return fmt.Sprintf("Delete your last %s message", network)
	case "react":
		return fmt.Sprintf("React to %s's message on %s", conversation, network)
	case "unreact":
		return fmt.Sprintf("Remove your reaction on %s's message on %s", conversation, network)
	case "mark_read":
		return fmt.Sprintf("Mark %s as read on %s", conversation, network)
	case "mark_unread":
		return fmt.Sprintf("Mark %s as unread on %s", conversation, network)
	case "archive":
		return fmt.Sprintf("Archive %s on %s", conversation, network)
	case "unarchive":
		return fmt.Sprintf("Unarchive %s on %s", conversation, network)
	case "pin":
		return fmt.Sprintf("Pin %s on %s", conversation, network)
	case "mute":
		return fmt.Sprintf("Mute %s on %s", conversation, network)
	case "set_reminder":
		return fmt.Sprintf("Set a reminder for %s on %s", conversation, network)
	case "clear_reminder":
		return fmt.Sprintf("Clear the reminder for %s on %s", conversation, network)
	default:
		return fmt.Sprintf("%s %s on %s", operation, conversation, network)
	}
}

func manageConfirm(operation string) string {
	switch operation {
	case "reply":
		return "Send reply"
	case "edit":
		return "Save edit"
	case "delete":
		return "Delete message"
	case "react":
		return "Add reaction"
	case "unreact":
		return "Remove reaction"
	case "mark_read":
		return "Mark as read"
	case "mark_unread":
		return "Mark as unread"
	case "archive":
		return "Archive"
	case "unarchive":
		return "Unarchive"
	case "pin":
		return "Pin"
	case "mute":
		return "Mute"
	case "set_reminder":
		return "Set reminder"
	case "clear_reminder":
		return "Clear reminder"
	default:
		return "Confirm"
	}
}

func manageConfirmedDetail(operation string) string {
	switch operation {
	case "edit":
		return "Updated your message"
	case "delete":
		return "Deleted your message"
	case "react":
		return "Added your reaction"
	case "unreact":
		return "Removed your reaction"
	case "mark_read":
		return "Marked as read"
	case "mark_unread":
		return "Marked as unread"
	case "archive":
		return "Archived the conversation"
	case "unarchive":
		return "Unarchived the conversation"
	case "pin":
		return "Pinned the conversation"
	case "mute":
		return "Muted the conversation"
	case "set_reminder":
		return "Set a reminder"
	case "clear_reminder":
		return "Cleared the reminder"
	default:
		return "Done"
	}
}

func (a *Adapter) previewManage(plan adapter.Plan) (adapter.Preview, error) {
	operation := plan.Details["operation"]
	lines := []string{
		"Network: " + plan.Details["network"],
		"Conversation: " + plan.Details["conversation"],
	}
	if target := plan.Details["target_text"]; target != "" {
		lines = append(lines, "Message: "+clipRunes(target, maxPreviewMessageRunes))
	}
	if body := plan.Details["body"]; body != "" {
		lines = append(lines, body)
	}
	a.logger.Info("[beeper-message] preview", "adapter_id", a.spec.ID, "network", a.spec.Network, "verb", plan.Verb, "operation", operation, "chat_id", plan.Handle)
	return adapter.Preview{
		Plan:     plan,
		Headline: manageHeadline(operation, plan.Details["network"], plan.Details["conversation"]),
		Lines:    lines,
		Confirm:  manageConfirm(operation),
	}, nil
}

// executeManage dispatches a resolved manage plan to its Beeper API call. A
// reply's outcome is delivery-pending, exactly like send, because Beeper has
// accepted it but final thread visibility is not yet confirmed. Every other
// operation is a plain state change: it either took effect or it didn't, so
// its outcome is confirmed on success and its raw error on failure.
func (a *Adapter) executeManage(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	operation := plan.Details["operation"]
	spec, ok := manageOps[operation]
	if !ok {
		return adapter.Outcome{}, fmt.Errorf("beeper message: unknown manage operation %q", operation)
	}
	chatID := plan.Handle
	messageID := plan.Details["message_id"]
	body := plan.Details["body"]
	a.logger.Info("[beeper-message] execute", "adapter_id", a.spec.ID, "network", a.spec.Network, "verb", plan.Verb, "operation", operation, "chat_id", chatID, "message_id", messageID)

	if err := spec.dispatch(ctx, a.api, chatID, messageID, body); err != nil {
		a.logger.Error("[beeper-message] manage execute failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "operation", operation, "chat_id", chatID, "error", err)
		if spec.pending {
			return adapter.Outcome{}, &adapter.OutcomeUnknownError{AdapterID: a.spec.ID, Verb: string(plan.Verb), Cause: err}
		}
		return adapter.Outcome{}, err
	}

	if spec.pending {
		a.logger.Info("[beeper-message] reply accepted, pending", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", chatID, "message_id", messageID)
		return adapter.Outcome{}, &adapter.OutcomeUnknownError{AdapterID: a.spec.ID, Verb: string(plan.Verb), Cause: ErrDeliveryPending}
	}

	a.logger.Info("[beeper-message] manage execute confirmed", "adapter_id", a.spec.ID, "network", a.spec.Network, "operation", operation, "chat_id", chatID)
	return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: manageConfirmedDetail(operation)}, nil
}
