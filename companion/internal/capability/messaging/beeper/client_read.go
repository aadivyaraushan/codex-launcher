package beeper

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Message is one message in a chat. Field names are provisional until the B0
// live /v1/spec check confirms them against a running Beeper instance.
type Message struct {
	ID         string `json:"id"`
	ChatID     string `json:"chatID"`
	SenderID   string `json:"senderID"`
	SenderName string `json:"senderName"`
	Text       string `json:"text"`
	Timestamp  string `json:"timestamp"`
	IsSender   bool   `json:"isSender"`
}

// ListChatsOptions narrows a chat listing.
type ListChatsOptions struct {
	Unread bool
	Limit  int
}

const defaultReadLimit = 20

func readLimit(limit int) int {
	if limit <= 0 {
		return defaultReadLimit
	}
	return limit
}

// ListChats returns the caller's conversations, most recently active first.
// It is a read; it works even on a read-only client.
func (c *Client) ListChats(ctx context.Context, opts ListChatsOptions) ([]Chat, error) {
	params := url.Values{"limit": {strconv.Itoa(readLimit(opts.Limit))}}
	if opts.Unread {
		params.Set("unread", "true")
	}
	var page struct {
		Items []Chat `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/chats?"+params.Encode(), nil, &page); err != nil {
		return nil, err
	}
	c.logger.Info("[beeper] chat list complete",
		"unread_only", opts.Unread, "chat_count", len(page.Items))
	return page.Items, nil
}

// ListMessages returns the most recent messages in one chat, oldest-first
// ordering depending on what Beeper returns. It is a read; it works even on a
// read-only client.
func (c *Client) ListMessages(ctx context.Context, chatID string, limit int) ([]Message, error) {
	params := url.Values{"limit": {strconv.Itoa(readLimit(limit))}}
	path := "/v1/chats/" + url.PathEscape(chatID) + "/messages?" + params.Encode()
	var page struct {
		Items []Message `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	c.logger.Info("[beeper] message list complete",
		"chat_id", chatID, "message_count", len(page.Items))
	return page.Items, nil
}

// GetChat fetches one chat by its ID. It is a read; it works even on a
// read-only client.
func (c *Client) GetChat(ctx context.Context, chatID string) (Chat, error) {
	var chat Chat
	path := "/v1/chats/" + url.PathEscape(chatID)
	if err := c.do(ctx, http.MethodGet, path, nil, &chat); err != nil {
		return Chat{}, err
	}
	c.logger.Info("[beeper] chat fetch complete", "chat_id", chat.ID, "network", chat.Network)
	return chat, nil
}

// SearchMessages finds messages matching a free-text query across chats. It is
// a read; it works even on a read-only client.
func (c *Client) SearchMessages(ctx context.Context, query string, limit int) ([]Message, error) {
	// Same rule as SearchChats: a blank ?query= 400s, so an empty search omits
	// the parameter rather than sending it empty.
	params := url.Values{"limit": {strconv.Itoa(readLimit(limit))}}
	if strings.TrimSpace(query) != "" {
		params.Set("query", query)
	}
	var page struct {
		Items []Message `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/messages/search?"+params.Encode(), nil, &page); err != nil {
		return nil, err
	}
	c.logger.Info("[beeper] message search complete",
		"query_length", len(query), "match_count", len(page.Items))
	return page.Items, nil
}
