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
	"unicode"

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

// API is the Beeper surface the adapter needs across send, read, and manage.
type API interface {
	SearchChats(context.Context, string) ([]beeper.Chat, error)
	Accounts(context.Context) ([]beeper.Account, error)
	StartChat(context.Context, string, string) (beeper.Chat, error)
	Send(context.Context, string, string) (beeper.Sent, error)

	ListChats(context.Context, beeper.ListChatsOptions) ([]beeper.Chat, error)
	ListMessages(context.Context, string, int) ([]beeper.Message, error)
	Reply(context.Context, string, string, string) (beeper.Sent, error)
	EditMessage(context.Context, string, string, string) error
	DeleteMessage(context.Context, string, string) error
	React(context.Context, string, string, string) error
	Unreact(context.Context, string, string, string) error
	MarkRead(context.Context, string) error
	MarkUnread(context.Context, string) error
	Archive(context.Context, string, bool) error
	UpdateChat(context.Context, string, beeper.ChatState) error
	SetReminder(context.Context, string, string) error
	ClearReminder(context.Context, string) error
}

// Spec binds one Operator adapter id to one Beeper network. The production
// set is deliberately closed to the networks the owner approved for this run.
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

type Adapter struct {
	spec    Spec
	api     API
	revoke  func(context.Context) error
	revoked bool
	logger  *slog.Logger
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

func (a *Adapter) Describe() manifest.Manifest {
	auth := a.spec.Auth
	if auth == "" {
		auth = manifest.AuthOAuth
	}
	return manifest.Manifest{
		ID: a.spec.ID, Runtime: manifest.RT2, Verbs: []manifest.Verb{manifest.Read, manifest.Send, manifest.Modify, manifest.Cancel},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: auth, Cost: manifest.CostFree,
		Gates: []manifest.Gate{manifest.GateNone}, Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region: []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "beeper_" + a.spec.ID + "_confirmed_send_visible_in_thread",
		Unshipped:     a.spec.Unshipped,
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	a.logger.Info("[beeper-message] resolve", "adapter_id", a.spec.ID, "network", a.spec.Network, "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	switch in.Verb {
	case manifest.Send:
		return a.resolveSend(ctx, in)
	case manifest.Read:
		return a.resolveRead(ctx, in)
	default:
		return a.resolveManage(ctx, in)
	}
}

// matchingChats runs a Beeper chat search and narrows the results to this
// adapter's network, sorted by title. It is the one place send, read, and
// manage all go through to turn a spoken name into candidate chats, so the
// matching rule (and its network filter) can never drift between them.
func (a *Adapter) matchingChats(ctx context.Context, subject string) ([]beeper.Chat, error) {
	chats, err := a.api.SearchChats(ctx, subject)
	if err != nil {
		a.logger.Error("[beeper-message] chat search failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "error", err)
		return nil, err
	}
	matches := make([]beeper.Chat, 0, len(chats))
	for _, chat := range chats {
		if !strings.EqualFold(strings.TrimSpace(chat.Network), a.spec.Network) {
			continue
		}
		// Beeper searches participant identities as well as conversation
		// titles. A valid direct-message result therefore need not repeat the
		// searched name in its title. Trust Beeper's match, then enforce the
		// selected network and require exactly one result below.
		matches = append(matches, chat)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Title < matches[j].Title })
	return matches, nil
}

func clarifyNoMatch(network, subject string) error {
	return &adapter.ClarificationError{Question: fmt.Sprintf("I couldn't find a %s conversation matching %s. Which conversation did you mean?", network, subject)}
}

func clarifyAmbiguous(network, subject string, matches []beeper.Chat) error {
	titles := make([]string, len(matches))
	for i, chat := range matches {
		titles[i] = chat.Title
	}
	return &adapter.ClarificationError{Question: fmt.Sprintf("I found %d %s conversations matching %s: %s. Which one did you mean?", len(matches), network, subject, strings.Join(titles, ", "))}
}

// resolveOneChat resolves a subject to exactly one chat on this adapter's
// network, or a ClarificationError when there is no match or more than one.
// It is the matching rule read and manage share; send keeps its own inline
// version because of the phone-start branch that runs before its
// zero-match clarification.
func (a *Adapter) resolveOneChat(ctx context.Context, subject string) (beeper.Chat, error) {
	matches, err := a.matchingChats(ctx, subject)
	if err != nil {
		return beeper.Chat{}, err
	}
	if len(matches) == 0 {
		return beeper.Chat{}, clarifyNoMatch(a.spec.Network, subject)
	}
	if len(matches) > 1 {
		return beeper.Chat{}, clarifyAmbiguous(a.spec.Network, subject, matches)
	}
	return matches[0], nil
}

func (a *Adapter) resolveSend(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	recipient := strings.TrimSpace(in.Subject)
	if recipient == "" {
		return adapter.Plan{}, ErrNoRecipient
	}
	message := strings.TrimSpace(in.Body)
	if message == "" {
		return adapter.Plan{}, ErrEmptyMessage
	}

	matches, err := a.matchingChats(ctx, recipient)
	if err != nil {
		return adapter.Plan{}, err
	}
	if len(matches) == 0 && a.spec.StartByPhone {
		phoneNumber, ok := normalizedE164(recipient)
		if ok {
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
					"network": a.spec.Network, "conversation": recipient, "text": message,
					"start_account_id": connected[0].ID, "start_phone": phoneNumber,
				},
			}, nil
		}
	}
	if len(matches) == 0 {
		a.logger.Info("[beeper-message] resolve needs clarification", "adapter_id", a.spec.ID, "network", a.spec.Network, "reason", "no_match")
		return adapter.Plan{}, clarifyNoMatch(a.spec.Network, recipient)
	}
	if len(matches) > 1 {
		a.logger.Info("[beeper-message] resolve needs clarification", "adapter_id", a.spec.ID, "network", a.spec.Network, "reason", "multiple_matches", "match_count", len(matches))
		return adapter.Plan{}, clarifyAmbiguous(a.spec.Network, recipient, matches)
	}

	chat := matches[0]
	details := map[string]string{
		"network": a.spec.Network, "conversation": chat.Title, "text": message,
	}
	a.logger.Info("[beeper-message] resolve ready", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", chat.ID, "text_length", len(message))
	return adapter.Plan{
		AdapterID: a.spec.ID, Verb: manifest.Send, Handle: chat.ID,
		Summary: "Send a " + a.spec.Network + " message", Details: details,
	}, nil
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

// Preview dispatches on what kind of plan Resolve produced: a read plan
// carries "kind", a manage plan carries "operation", and a send plan carries
// neither.
func (a *Adapter) Preview(ctx context.Context, plan adapter.Plan) (adapter.Preview, error) {
	switch {
	case plan.Details["kind"] != "":
		return a.previewRead(plan)
	case plan.Details["operation"] != "":
		return a.previewManage(plan)
	default:
		return a.previewSend(plan)
	}
}

func (a *Adapter) previewSend(plan adapter.Plan) (adapter.Preview, error) {
	a.logger.Info("[beeper-message] preview", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", plan.Handle, "text_length", len(plan.Details["text"]))
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

// Execute dispatches the same way Preview does: by verb for a read plan, by
// the presence of "operation" for a manage plan, and send otherwise.
func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	switch {
	case plan.Verb == manifest.Read:
		return a.executeRead(plan)
	case plan.Details["operation"] != "":
		return a.executeManage(ctx, plan)
	default:
		return a.executeSend(ctx, plan)
	}
}

func (a *Adapter) executeSend(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	a.logger.Info("[beeper-message] execute", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", plan.Handle, "text_length", len(plan.Details["text"]))
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
	sent, err := a.api.Send(ctx, chatID, plan.Details["text"])
	if err != nil {
		a.logger.Error("[beeper-message] send failed", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", plan.Handle, "error", err)
		return adapter.Outcome{}, &adapter.OutcomeUnknownError{AdapterID: a.spec.ID, Verb: string(manifest.Send), Cause: err}
	}
	a.logger.Info("[beeper-message] send accepted", "adapter_id", a.spec.ID, "network", a.spec.Network, "chat_id", sent.ChatID, "pending_message_id", sent.PendingMessageID)
	return adapter.Outcome{}, &adapter.OutcomeUnknownError{
		AdapterID: a.spec.ID, Verb: string(manifest.Send), Cause: ErrDeliveryPending,
	}
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
	// The shared Beeper connection belongs to the companion, not one network
	// adapter. The persistent marker and runner remove this network from routing;
	// they must not log the user out of every other linked network as a side effect.
	a.logger.Info("[beeper-message] revoked", "adapter_id", a.spec.ID, "decision", "routing_removed_shared_beeper_session_kept")
	return nil
}
