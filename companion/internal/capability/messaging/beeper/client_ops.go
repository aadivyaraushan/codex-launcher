package beeper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ListChats returns one page of the unified inbox (GET /v1/chats).
func (c *Client) ListChats(ctx context.Context, opts ListChatsOptions) (ChatPage, error) {
	params := url.Values{}
	for _, id := range opts.AccountIDs {
		if id = strings.TrimSpace(id); id != "" {
			params.Add("accountIDs", id)
		}
	}
	if cursor := strings.TrimSpace(opts.Cursor); cursor != "" {
		params.Set("cursor", cursor)
	}
	if direction := strings.TrimSpace(opts.Direction); direction != "" {
		params.Set("direction", direction)
	}
	path := "/v1/chats"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var page ChatPage
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return ChatPage{}, err
	}
	c.logger.Info("[beeper] chats listed",
		"item_count", len(page.Items), "has_more", page.HasMore,
		"account_filter_count", len(opts.AccountIDs))
	return page, nil
}

// GetChat returns one chat including its capabilities matrix.
func (c *Client) GetChat(ctx context.Context, chatID string) (Chat, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return Chat{}, fmt.Errorf("beeper: chat id is required")
	}
	var chat Chat
	path := "/v1/chats/" + url.PathEscape(chatID)
	if err := c.do(ctx, http.MethodGet, path, nil, &chat); err != nil {
		return Chat{}, err
	}
	c.logger.Info("[beeper] chat fetched",
		"chat_id", chat.ID, "network", chat.Network, "unread_count", chat.UnreadCount)
	return chat, nil
}

// ListMessages returns one page of messages in a chat.
func (c *Client) ListMessages(ctx context.Context, chatID string, opts MessageListOptions) (MessagePage, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return MessagePage{}, fmt.Errorf("beeper: chat id is required")
	}
	params := url.Values{}
	if cursor := strings.TrimSpace(opts.Cursor); cursor != "" {
		params.Set("cursor", cursor)
	}
	if direction := strings.TrimSpace(opts.Direction); direction != "" {
		params.Set("direction", direction)
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/messages"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var page MessagePage
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return MessagePage{}, err
	}
	c.logger.Info("[beeper] messages listed",
		"chat_id", chatID, "item_count", len(page.Items), "has_more", page.HasMore)
	return page, nil
}

// SearchMessages searches message text across chats.
func (c *Client) SearchMessages(ctx context.Context, opts SearchMessagesOptions) (MessagePage, error) {
	params := url.Values{}
	if query := strings.TrimSpace(opts.Query); query != "" {
		params.Set("query", query)
	}
	for _, id := range opts.AccountIDs {
		if id = strings.TrimSpace(id); id != "" {
			params.Add("accountIDs", id)
		}
	}
	for _, id := range opts.ChatIDs {
		if id = strings.TrimSpace(id); id != "" {
			params.Add("chatIDs", id)
		}
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if cursor := strings.TrimSpace(opts.Cursor); cursor != "" {
		params.Set("cursor", cursor)
	}
	if direction := strings.TrimSpace(opts.Direction); direction != "" {
		params.Set("direction", direction)
	}
	path := "/v1/messages/search"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var page MessagePage
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return MessagePage{}, err
	}
	c.logger.Info("[beeper] messages searched",
		"query_length", len(strings.TrimSpace(opts.Query)),
		"item_count", len(page.Items), "has_more", page.HasMore)
	return page, nil
}

// EditMessage replaces the text of an existing message (PUT).
func (c *Client) EditMessage(ctx context.Context, chatID, messageID, text string) (Message, error) {
	if err := c.refuseWrite("edit"); err != nil {
		return Message{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Message{}, ErrEmptyMessage
	}
	var msg Message
	path := messagePath(chatID, messageID)
	if err := c.do(ctx, http.MethodPut, path, map[string]any{"text": text}, &msg); err != nil {
		return Message{}, err
	}
	c.logger.Info("[beeper] message edited",
		"chat_id", chatID, "message_id", messageID, "text_length", len(text))
	return msg, nil
}

// DeleteMessage deletes a message (DELETE, 204).
func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID string) error {
	if err := c.refuseWrite("delete"); err != nil {
		return err
	}
	if err := c.do(ctx, http.MethodDelete, messagePath(chatID, messageID), nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] message deleted", "chat_id", chatID, "message_id", messageID)
	return nil
}

// React adds a reaction to a message.
func (c *Client) React(ctx context.Context, chatID, messageID, reactionKey string) error {
	if err := c.refuseWrite("react"); err != nil {
		return err
	}
	reactionKey = strings.TrimSpace(reactionKey)
	if reactionKey == "" {
		return fmt.Errorf("beeper: reaction key is required")
	}
	path := messagePath(chatID, messageID) + "/reactions"
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, path, map[string]any{"reactionKey": reactionKey}, &out); err != nil {
		return err
	}
	c.logger.Info("[beeper] reaction added",
		"chat_id", chatID, "message_id", messageID, "reaction_length", len(reactionKey))
	return nil
}

// Unreact removes a reaction from a message.
func (c *Client) Unreact(ctx context.Context, chatID, messageID, reactionKey string) error {
	if err := c.refuseWrite("unreact"); err != nil {
		return err
	}
	reactionKey = strings.TrimSpace(reactionKey)
	if reactionKey == "" {
		return fmt.Errorf("beeper: reaction key is required")
	}
	path := messagePath(chatID, messageID) + "/reactions/" + url.PathEscape(reactionKey)
	var out map[string]any
	if err := c.do(ctx, http.MethodDelete, path, nil, &out); err != nil {
		return err
	}
	c.logger.Info("[beeper] reaction removed",
		"chat_id", chatID, "message_id", messageID, "reaction_length", len(reactionKey))
	return nil
}

// MarkRead marks a chat read. Optional messageID marks read through that message.
// Live Desktop 5.0.0 returns 200 + Chat (not 204).
func (c *Client) MarkRead(ctx context.Context, chatID, messageID string) (Chat, error) {
	if err := c.refuseWrite("mark_read"); err != nil {
		return Chat{}, err
	}
	var chat Chat
	path := "/v1/chats/" + url.PathEscape(chatID) + "/read"
	body := map[string]any{}
	if messageID = strings.TrimSpace(messageID); messageID != "" {
		body["messageID"] = messageID
	}
	if err := c.do(ctx, http.MethodPost, path, body, &chat); err != nil {
		return Chat{}, err
	}
	c.logger.Info("[beeper] chat marked read", "chat_id", chatID, "has_message_id", messageID != "")
	return chat, nil
}

// MarkUnread marks a chat unread. Live Desktop 5.0.0 returns 200 + Chat.
func (c *Client) MarkUnread(ctx context.Context, chatID, messageID string) (Chat, error) {
	if err := c.refuseWrite("mark_unread"); err != nil {
		return Chat{}, err
	}
	var chat Chat
	path := "/v1/chats/" + url.PathEscape(chatID) + "/unread"
	body := map[string]any{}
	if messageID = strings.TrimSpace(messageID); messageID != "" {
		body["messageID"] = messageID
	}
	if err := c.do(ctx, http.MethodPost, path, body, &chat); err != nil {
		return Chat{}, err
	}
	c.logger.Info("[beeper] chat marked unread", "chat_id", chatID, "has_message_id", messageID != "")
	return chat, nil
}

// Archive archives or unarchives a chat (POST, 204).
func (c *Client) Archive(ctx context.Context, chatID string, archived bool) error {
	if err := c.refuseWrite("archive"); err != nil {
		return err
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/archive"
	if err := c.do(ctx, http.MethodPost, path, map[string]any{"archived": archived}, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] chat archive updated", "chat_id", chatID, "archived", archived)
	return nil
}

// UpdateChat patches pin/mute state (PATCH).
func (c *Client) UpdateChat(ctx context.Context, chatID string, opts UpdateChatOptions) (Chat, error) {
	if err := c.refuseWrite("update_chat"); err != nil {
		return Chat{}, err
	}
	body := map[string]any{}
	if opts.Pinned != nil {
		body["isPinned"] = *opts.Pinned
	}
	if opts.Muted != nil {
		body["isMuted"] = *opts.Muted
	}
	if len(body) == 0 {
		return Chat{}, fmt.Errorf("beeper: update chat requires pin or mute")
	}
	var chat Chat
	path := "/v1/chats/" + url.PathEscape(chatID)
	if err := c.do(ctx, http.MethodPatch, path, body, &chat); err != nil {
		return Chat{}, err
	}
	c.logger.Info("[beeper] chat updated",
		"chat_id", chatID, "set_pinned", opts.Pinned != nil, "set_muted", opts.Muted != nil)
	return chat, nil
}

// SetReminder creates a chat reminder (POST, 204).
func (c *Client) SetReminder(ctx context.Context, chatID string, remindAt time.Time, dismissOnIncoming bool) error {
	if err := c.refuseWrite("set_reminder"); err != nil {
		return err
	}
	if remindAt.IsZero() {
		return fmt.Errorf("beeper: reminder time is required")
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/reminders"
	body := map[string]any{
		"reminder": map[string]any{
			"remindAt":                 remindAt.UTC().Format(time.RFC3339),
			"dismissOnIncomingMessage": dismissOnIncoming,
		},
	}
	if err := c.do(ctx, http.MethodPost, path, body, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] reminder set", "chat_id", chatID, "dismiss_on_incoming", dismissOnIncoming)
	return nil
}

// ClearReminder deletes a chat reminder (DELETE, 204).
func (c *Client) ClearReminder(ctx context.Context, chatID string) error {
	if err := c.refuseWrite("clear_reminder"); err != nil {
		return err
	}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/reminders"
	if err := c.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return err
	}
	c.logger.Info("[beeper] reminder cleared", "chat_id", chatID)
	return nil
}

func (c *Client) refuseWrite(op string) error {
	if !c.readOnly {
		return nil
	}
	c.logger.Warn("[beeper] write refused, client is read-only", "op", op)
	return ErrReadOnly
}

func messagePath(chatID, messageID string) string {
	return "/v1/chats/" + url.PathEscape(strings.TrimSpace(chatID)) +
		"/messages/" + url.PathEscape(strings.TrimSpace(messageID))
}
