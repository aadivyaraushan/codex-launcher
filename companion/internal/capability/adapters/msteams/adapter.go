// Package msteams is Operator's Wave 1 Microsoft Graph Teams adapter for
// work/school accounts. Personal Teams stays on deeplink ID "teams".
//
// Docs (Context7 /websites/learn_microsoft_en-us_graph):
// GET /me/chats and POST /chats/{id}/messages — personal Microsoft accounts
// not supported; use work or school delegated Chat.ReadWrite.
package msteams

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "msteams"

var (
	ErrAmbiguousChat = errors.New("msteams: more than one chat matches; ask the user which one")
	ErrNoChat        = errors.New("msteams: no chat matches")
	ErrNotConnected  = errors.New("msteams: adapter is not connected")
	ErrEmptyBody     = errors.New("msteams: message body must not be empty")
	ErrEmptyChat     = errors.New("msteams: chat name must not be empty")
)

// API is the small part of Graph chat the adapter needs.
type API interface {
	ListChats(context.Context) ([]Chat, error)
	SendMessage(context.Context, SendMessage) (SentMessage, error)
	Clear(context.Context) error
}

type Chat struct {
	ID    string
	Topic string
}

type SendMessage struct {
	ChatID string
	Body   string
}

type SentMessage struct {
	ID     string
	ChatID string
}

type Adapter struct {
	api    API
	logger *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(api API, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{api: api, logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read, manifest.Send},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "msteams_graph_work_oauth_chat_read_send",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[msteams] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	switch in.Verb {
	case manifest.Read:
		return a.findChat(ctx, in.Subject, manifest.Read, "")
	case manifest.Send:
		text := strings.TrimSpace(in.Body)
		if text == "" {
			return adapter.Plan{}, ErrEmptyBody
		}
		plan, err := a.findChat(ctx, in.Subject, manifest.Send, text)
		if err != nil {
			return adapter.Plan{}, err
		}
		plan.Summary = "Send a Teams work chat message"
		plan.Details["text"] = text
		return plan, nil
	default:
		return adapter.Plan{}, fmt.Errorf("msteams: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) findChat(ctx context.Context, query string, verb manifest.Verb, text string) (adapter.Plan, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return adapter.Plan{}, ErrEmptyChat
	}
	chats, err := a.api.ListChats(ctx)
	if err != nil {
		a.logger.Error("[msteams] list chats failed", "error", err)
		return adapter.Plan{}, err
	}
	var exact, partial []Chat
	for _, chat := range chats {
		topic := strings.ToLower(strings.TrimSpace(chat.Topic))
		id := strings.ToLower(strings.TrimSpace(chat.ID))
		if topic == needle || id == needle {
			exact = append(exact, chat)
			continue
		}
		if topic != "" && strings.Contains(topic, needle) {
			partial = append(partial, chat)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return adapter.Plan{}, ErrNoChat
	}
	if len(matches) > 1 {
		return adapter.Plan{}, &adapter.ClarificationError{Question: "Which matching Microsoft Teams chat did you mean?", Cause: ErrAmbiguousChat}
	}
	chat := matches[0]
	summary := "Read a Teams work chat"
	if verb == manifest.Send {
		summary = "Send a Teams work chat message"
	}
	details := map[string]string{
		"chat_id":    chat.ID,
		"chat_topic": chat.Topic,
	}
	if text != "" {
		details["text"] = text
	}
	return adapter.Plan{
		AdapterID: ID, Verb: verb, Handle: chat.ID, Summary: summary, Details: details,
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	topic := plan.Details["chat_topic"]
	if topic == "" {
		topic = plan.Details["chat_id"]
	}
	lines := []string{topic}
	confirm := "Read from Teams"
	if plan.Verb == manifest.Send {
		lines = append(lines, plan.Details["text"])
		confirm = "Send message"
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[msteams] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		detail := fmt.Sprintf("%s (%s)", plan.Details["chat_topic"], plan.Details["chat_id"])
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: detail}, nil
	case manifest.Send:
		sent, err := a.api.SendMessage(ctx, SendMessage{
			ChatID: plan.Details["chat_id"], Body: plan.Details["text"],
		})
		if err != nil {
			a.logger.Error("[msteams] send message failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[msteams] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("sent to %s message_id=%s", plan.Details["chat_topic"], sent.ID),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("msteams: verb %q is not supported", plan.Verb)
	}
}

// Revoke of an already-disconnected adapter reports success, not
// ErrNotConnected — see the comment on todoist's Revoke for why (a retried
// revoke should never look like a failed disconnect).
func (a *Adapter) Revoke(ctx context.Context) error {
	if a.api == nil {
		return nil
	}
	if err := a.api.Clear(ctx); err != nil {
		a.logger.Error("[msteams] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[msteams] revoked")
	return nil
}
