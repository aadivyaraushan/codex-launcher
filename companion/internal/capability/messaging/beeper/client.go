// Package beeper talks to a Beeper Client API — the one Beeper Desktop and the
// headless Beeper Server both serve on 127.0.0.1:23373. One Beeper account
// bridges Instagram, Discord, Google Messages, WhatsApp, Signal and the rest,
// so this is a single client for all of them rather than one adapter per app.
//
// Where that API is running is a deployment choice, not a code one: the same
// client works against Beeper Desktop on a paired Mac, a self-hosted Beeper
// Server, or a remote one reached over a tunnel. Point baseURL at whichever.
//
// The contract here was read off a live instance's own OpenAPI document
// (GET /v1/spec) rather than the docs site, whose reference page 404s.
package beeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DefaultBaseURL is where Beeper Desktop and Beeper Server both listen.
const DefaultBaseURL = "http://127.0.0.1:23373"

var (
	// ErrReadOnly is returned instead of sending when the client is read-only.
	ErrReadOnly = errors.New("beeper: client is read-only, refusing to send")
	// ErrNoMatch means the search found nobody, which is not an empty success.
	ErrNoMatch = errors.New("beeper: no chat matched")
	// ErrEmptyMessage means there was nothing to send.
	ErrEmptyMessage   = errors.New("beeper: refusing to send an empty message")
	ErrMissingAccount = errors.New("beeper: account id is required to start a chat")
	ErrMissingUser    = errors.New("beeper: phone number is required to start a chat")
)

// Chat is one conversation on one bridged network.
type Chat struct {
	ID           string           `json:"id"`
	LocalChatID  string           `json:"localChatID"`
	AccountID    string           `json:"accountID"`
	Network      string           `json:"network"`
	Title        string           `json:"title"`
	Type         string           `json:"type"`
	UnreadCount  int              `json:"unreadCount"`
	LastActivity string           `json:"lastActivity"`
	IsArchived   bool             `json:"isArchived"`
	IsPinned     bool             `json:"isPinned"`
	IsMuted      bool             `json:"isMuted"`
	Capabilities ChatCapabilities `json:"capabilities"`
}

// ChatCapabilities is the per-chat support matrix from Beeper Desktop.
// Support levels use Beeper's scale: -2 rejected, -1 dropped, 0 unsupported,
// 1 partial, 2 full. Boolean flags are true when the network advertises the op.
type ChatCapabilities struct {
	Reply        int  `json:"reply"`
	Edit         int  `json:"edit"`
	Delete       int  `json:"delete"`
	Reaction     int  `json:"reaction"`
	Archive      bool `json:"archive"`
	MarkAsUnread bool `json:"markAsUnread"`
}

// Supports reports whether a capability level is usable (partial or full).
func Supports(level int) bool { return level >= 1 }

// Message is one chat message from Beeper Desktop.
type Message struct {
	ID          string              `json:"id"`
	ChatID      string              `json:"chatID"`
	AccountID   string              `json:"accountID"`
	SenderID    string              `json:"senderID"`
	SenderName  string              `json:"senderName"`
	Timestamp   string              `json:"timestamp"`
	Text        string              `json:"text"`
	IsSender    bool                `json:"isSender"`
	IsUnread    bool                `json:"isUnread"`
	Attachments []MessageAttachment `json:"attachments"`
}

// MessageAttachment is a lightweight attachment hint for read previews.
// Sending attachments is out of scope; reads only describe them.
type MessageAttachment struct {
	Type string `json:"type"`
}

// ChatPage is one cursor page of chats.
type ChatPage struct {
	Items        []Chat `json:"items"`
	HasMore      bool   `json:"hasMore"`
	OldestCursor string `json:"oldestCursor"`
	NewestCursor string `json:"newestCursor"`
}

// MessagePage is one cursor page of messages.
type MessagePage struct {
	Items        []Message `json:"items"`
	HasMore      bool      `json:"hasMore"`
	OldestCursor string    `json:"oldestCursor"`
	NewestCursor string    `json:"newestCursor"`
}

// ListChatsOptions filters GET /v1/chats. Live Desktop 5.0.0 has no unread
// query filter — callers filter UnreadCount themselves after paging.
type ListChatsOptions struct {
	AccountIDs []string
	Cursor     string
	Direction  string
}

// MessageListOptions pages GET /v1/chats/{id}/messages.
type MessageListOptions struct {
	Cursor    string
	Direction string
}

// SearchMessagesOptions filters GET /v1/messages/search.
type SearchMessagesOptions struct {
	Query      string
	AccountIDs []string
	ChatIDs    []string
	Limit      int
	Cursor     string
	Direction  string
}

// UpdateChatOptions is the pin/mute subset of PATCH /v1/chats/{id}.
type UpdateChatOptions struct {
	Pinned *bool
	Muted  *bool
}

// Account identifies one connected bridge. Account IDs are assigned by
// Beeper and must be discovered rather than guessed from a network name.
type Account struct {
	ID      string `json:"accountID"`
	Network string `json:"network"`
	Status  string `json:"status"`
}

// Sent is what Beeper hands back after accepting a message. The message is
// queued at this point, not confirmed delivered — hence "pending".
type Sent struct {
	ChatID           string `json:"chatID"`
	PendingMessageID string `json:"pendingMessageID"`
}

// AmbiguousError means the name matched more than one conversation. The caller
// must ask the user which one; it must never pick for them.
type AmbiguousError struct {
	Query      string
	Candidates []Chat
}

func (e *AmbiguousError) Error() string {
	described := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		described = append(described, fmt.Sprintf("%s (%s)", c.Title, c.Network))
	}
	return fmt.Sprintf("beeper: %q matches %d chats: %s",
		e.Query, len(e.Candidates), strings.Join(described, ", "))
}

// TokenSource supplies the Beeper access token.
type TokenSource interface {
	AccessToken(context.Context) (string, error)
}

// StaticToken is a token already in hand, e.g. from BEEPER_ACCESS_TOKEN.
type StaticToken string

func (t StaticToken) AccessToken(context.Context) (string, error) { return string(t), nil }

// Client is a Beeper Client API client.
type Client struct {
	baseURL  string
	tokens   TokenSource
	http     *http.Client
	logger   *slog.Logger
	readOnly bool
}

// NewClient builds a client. An empty baseURL means the local default.
func NewClient(baseURL string, tokens TokenSource, httpClient *http.Client, logger *slog.Logger) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		tokens:  tokens,
		http:    httpClient,
		logger:  logger,
	}
}

// ReadOnly returns a copy of the client that refuses every send. Use it when
// pointing at a live Beeper account you do not intend to write to.
func (c *Client) ReadOnly() *Client {
	copied := *c
	copied.readOnly = true
	return &copied
}

// SearchChats finds conversations matching a free-text query — usually a
// person's name.
func (c *Client) SearchChats(ctx context.Context, query string) ([]Chat, error) {
	// A real Beeper 400s on a blank ?query= ("String must contain at least 1
	// character(s)"), so an empty search omits the parameter rather than
	// sending it empty.
	params := url.Values{"limit": {"20"}}
	if strings.TrimSpace(query) != "" {
		params.Set("query", query)
	}
	var page struct {
		Items []Chat `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/chats/search?"+params.Encode(), nil, &page); err != nil {
		return nil, err
	}
	c.logger.Info("[beeper] chat search complete",
		"query_length", len(query), "match_count", len(page.Items))
	return page.Items, nil
}

// Accounts returns the bridge accounts currently known to Beeper.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	if err := c.do(ctx, http.MethodGet, "/v1/accounts", nil, &accounts); err != nil {
		return nil, err
	}
	c.logger.Info("[beeper] accounts listed", "account_count", len(accounts))
	return accounts, nil
}

// StartChat opens or reuses a direct conversation for an exact phone number on
// one selected Beeper account. It is used only after ordinary chat search has
// found no match.
func (c *Client) StartChat(ctx context.Context, accountID, phoneNumber string) (Chat, error) {
	accountID = strings.TrimSpace(accountID)
	phoneNumber = strings.TrimSpace(phoneNumber)
	if accountID == "" {
		return Chat{}, ErrMissingAccount
	}
	if phoneNumber == "" {
		return Chat{}, ErrMissingUser
	}
	if c.readOnly {
		c.logger.Warn("[beeper] start chat refused, client is read-only", "account_id", accountID)
		return Chat{}, ErrReadOnly
	}
	var chat Chat
	body := map[string]any{
		"accountID": accountID,
		"user":      map[string]string{"phoneNumber": phoneNumber},
	}
	if err := c.do(ctx, http.MethodPost, "/v1/chats/start", body, &chat); err != nil {
		return Chat{}, err
	}
	c.logger.Info("[beeper] chat start complete",
		"account_id", accountID, "chat_id", chat.ID, "network", chat.Network,
		"phone_length", len(phoneNumber))
	return chat, nil
}

// ResolveOne turns a name into exactly one chat, or an error explaining why it
// could not. It never guesses between people.
func (c *Client) ResolveOne(ctx context.Context, name string) (Chat, error) {
	matches, err := c.SearchChats(ctx, name)
	if err != nil {
		return Chat{}, err
	}
	switch len(matches) {
	case 0:
		c.logger.Info("[beeper] resolve found nobody", "query_length", len(name))
		return Chat{}, fmt.Errorf("%w for %q", ErrNoMatch, name)
	case 1:
		c.logger.Info("[beeper] resolve matched one chat",
			"chat_id", matches[0].ID, "network", matches[0].Network)
		return matches[0], nil
	default:
		c.logger.Info("[beeper] resolve was ambiguous, asking the user",
			"query_length", len(name), "candidate_count", len(matches))
		return Chat{}, &AmbiguousError{Query: name, Candidates: matches}
	}
}

// Send delivers text to one chat. Final delivery is pending at this point.
func (c *Client) Send(ctx context.Context, chatID, text string) (Sent, error) {
	return c.SendReply(ctx, chatID, text, "")
}

// SendReply delivers text, optionally as a reply to an existing message.
func (c *Client) SendReply(ctx context.Context, chatID, text, replyToMessageID string) (Sent, error) {
	if strings.TrimSpace(text) == "" {
		return Sent{}, ErrEmptyMessage
	}
	if c.readOnly {
		c.logger.Warn("[beeper] send refused, client is read-only", "chat_id", chatID)
		return Sent{}, ErrReadOnly
	}
	var sent Sent
	path := "/v1/chats/" + url.PathEscape(chatID) + "/messages"
	body := map[string]any{"text": text}
	if replyTo := strings.TrimSpace(replyToMessageID); replyTo != "" {
		body["replyToMessageID"] = replyTo
	}
	if err := c.do(ctx, http.MethodPost, path, body, &sent); err != nil {
		return Sent{}, err
	}
	c.logger.Info("[beeper] message accepted",
		"chat_id", sent.ChatID, "pending_message_id", sent.PendingMessageID,
		"text_length", len(text), "has_reply_to", strings.TrimSpace(replyToMessageID) != "")
	return sent, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, result any) error {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("beeper: load access token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("beeper: encode request: %w", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("beeper: build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	cleanPath := strings.Split(path, "?")[0]
	c.logger.Info("[beeper] request", "method", method, "path", cleanPath)

	response, err := c.http.Do(request)
	if err != nil {
		c.logger.Error("[beeper] request failed", "method", method, "path", cleanPath, "error", err)
		return fmt.Errorf("beeper: %s %s failed: %w", method, cleanPath, err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		c.logger.Error("[beeper] response rejected",
			"method", method, "path", cleanPath, "status", response.StatusCode)
		return fmt.Errorf("beeper: %s %s returned status %s",
			method, cleanPath, strconv.Itoa(response.StatusCode))
	}
	// State ops (archive/delete/reminders) return 204 with an empty body.
	if result == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("beeper: read %s response: %w", method, err)
	}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, result); err != nil {
		return fmt.Errorf("beeper: decode %s response: %w", method, err)
	}
	return nil
}
