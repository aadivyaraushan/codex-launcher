package beeper

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// Workstream B1 (write side). Field and endpoint shapes are provisional until
// the B0 live /v1/spec check confirms them against a running Beeper instance.
// Every method here mutates state, so each refuses on a read-only client
// exactly like Send: the read-only guard is the phone's fail-safe when the
// user has not enabled writes, and skipping it here would be a silent hole.

// ChatState is a partial update to a chat's pin/mute flags. Only the fields
// set (non-nil) are sent; the rest are left alone.
type ChatState struct {
	Pinned *bool `json:"pinned,omitempty"`
	Muted  *bool `json:"muted,omitempty"`
}

func messagePath(chatID, messageID string) string {
	return "/v1/chats/" + url.PathEscape(chatID) + "/messages/" + url.PathEscape(messageID)
}

// Reply sends text into a chat as a reply to one earlier message.
func (c *Client) Reply(ctx context.Context, chatID, replyToMessageID, text string) (Sent, error) {
	if strings.TrimSpace(text) == "" {
		return Sent{}, ErrEmptyMessage
	}
	if c.readOnly {
		c.logger.Warn("[beeper] reply refused, client is read-only", "chat_id", chatID)
		return Sent{}, ErrReadOnly
	}
	var sent Sent
	path := "/v1/chats/" + url.PathEscape(chatID) + "/messages"
	body := map[string]any{"text": text, "replyToMessageID": replyToMessageID}
	if err := c.do(ctx, http.MethodPost, path, body, &sent); err != nil {
		return Sent{}, err
	}
	c.logger.Info("[beeper] reply accepted",
		"chat_id", sent.ChatID, "pending_message_id", sent.PendingMessageID,
		"reply_to_message_id", replyToMessageID, "text_length", len(text))
	return sent, nil
}

// EditMessage replaces the text of an already-sent message.
func (c *Client) EditMessage(ctx context.Context, chatID, messageID, text string) error {
	if strings.TrimSpace(text) == "" {
		return ErrEmptyMessage
	}
	if c.readOnly {
		c.logger.Warn("[beeper] edit message refused, client is read-only",
			"chat_id", chatID, "message_id", messageID)
		return ErrReadOnly
	}
	body := map[string]any{"text": text}
	if err := c.do(ctx, http.MethodPut, messagePath(chatID, messageID), body, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] message edited", "chat_id", chatID, "message_id", messageID, "text_length", len(text))
	return nil
}

// DeleteMessage removes a message from a chat.
func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] delete message refused, client is read-only",
			"chat_id", chatID, "message_id", messageID)
		return ErrReadOnly
	}
	if err := c.do(ctx, http.MethodDelete, messagePath(chatID, messageID), nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] message deleted", "chat_id", chatID, "message_id", messageID)
	return nil
}

// React adds an emoji reaction to a message.
func (c *Client) React(ctx context.Context, chatID, messageID, key string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] react refused, client is read-only",
			"chat_id", chatID, "message_id", messageID)
		return ErrReadOnly
	}
	path := messagePath(chatID, messageID) + "/reactions"
	body := map[string]any{"key": key}
	if err := c.do(ctx, http.MethodPost, path, body, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] reaction added", "chat_id", chatID, "message_id", messageID)
	return nil
}

// Unreact removes a previously added emoji reaction from a message.
func (c *Client) Unreact(ctx context.Context, chatID, messageID, key string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] unreact refused, client is read-only",
			"chat_id", chatID, "message_id", messageID)
		return ErrReadOnly
	}
	path := messagePath(chatID, messageID) + "/reactions/" + url.PathEscape(key)
	if err := c.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] reaction removed", "chat_id", chatID, "message_id", messageID)
	return nil
}

// MarkRead marks a chat as read.
func (c *Client) MarkRead(ctx context.Context, chatID string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] mark read refused, client is read-only", "chat_id", chatID)
		return ErrReadOnly
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/read"
	if err := c.do(ctx, http.MethodPost, path, nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] chat marked read", "chat_id", chatID)
	return nil
}

// MarkUnread marks a chat as unread.
func (c *Client) MarkUnread(ctx context.Context, chatID string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] mark unread refused, client is read-only", "chat_id", chatID)
		return ErrReadOnly
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/unread"
	if err := c.do(ctx, http.MethodPost, path, nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] chat marked unread", "chat_id", chatID)
	return nil
}

// Archive sets or clears a chat's archived flag.
func (c *Client) Archive(ctx context.Context, chatID string, archived bool) error {
	if c.readOnly {
		c.logger.Warn("[beeper] archive refused, client is read-only", "chat_id", chatID)
		return ErrReadOnly
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/archive"
	body := map[string]any{"archived": archived}
	if err := c.do(ctx, http.MethodPost, path, body, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] chat archive flag set", "chat_id", chatID, "archived", archived)
	return nil
}

// UpdateChat applies a partial pin/mute update to a chat. Only the fields set
// in state are sent.
func (c *Client) UpdateChat(ctx context.Context, chatID string, state ChatState) error {
	if c.readOnly {
		c.logger.Warn("[beeper] update chat refused, client is read-only", "chat_id", chatID)
		return ErrReadOnly
	}
	path := "/v1/chats/" + url.PathEscape(chatID)
	if err := c.do(ctx, http.MethodPatch, path, state, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] chat updated", "chat_id", chatID)
	return nil
}

// SetReminder schedules a reminder on a chat for the given time.
func (c *Client) SetReminder(ctx context.Context, chatID, remindAt string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] set reminder refused, client is read-only", "chat_id", chatID)
		return ErrReadOnly
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/reminders"
	body := map[string]any{"remindAt": remindAt}
	if err := c.do(ctx, http.MethodPost, path, body, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] reminder set", "chat_id", chatID)
	return nil
}

// ClearReminder removes any reminder set on a chat.
func (c *Client) ClearReminder(ctx context.Context, chatID string) error {
	if c.readOnly {
		c.logger.Warn("[beeper] clear reminder refused, client is read-only", "chat_id", chatID)
		return ErrReadOnly
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/reminders"
	if err := c.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] reminder cleared", "chat_id", chatID)
	return nil
}
