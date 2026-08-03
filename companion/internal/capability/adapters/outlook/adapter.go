// Package outlook is Operator's Wave 1 Microsoft Graph mail adapter. It uses a
// personal Microsoft account user OAuth token and exposes read plus
// preview-gated write (draft) and send.
package outlook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "outlook"

var (
	ErrAmbiguousMessage = errors.New("outlook: more than one message matches; ask the user which one")
	ErrNoMessage        = errors.New("outlook: no message matches")
	ErrNotConnected     = errors.New("outlook: adapter is not connected")
	ErrEmptySubject     = errors.New("outlook: subject must not be empty")
	ErrEmptyRecipient   = errors.New("outlook: recipient address must not be empty")
	ErrEmptyBody        = errors.New("outlook: message body must not be empty")
)

// API is the small part of Graph mail the adapter needs.
type API interface {
	ListMessages(context.Context, string) ([]Message, error)
	CreateDraft(context.Context, CreateDraft) (Message, error)
	SendMail(context.Context, SendMail) error
	Clear(context.Context) error
}

type Message struct {
	ID      string
	Subject string
	From    string
	Preview string
}

type CreateDraft struct {
	Subject string
	Body    string
	To      string
}

type SendMail struct {
	To      string
	Subject string
	Body    string
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
		Verbs:   []manifest.Verb{manifest.Read, manifest.Write, manifest.Send},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "outlook_graph_user_oauth_mail_read_write_send",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[outlook] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	switch in.Verb {
	case manifest.Read:
		return a.resolveRead(ctx, in.Subject)
	case manifest.Write:
		subject := strings.TrimSpace(in.Subject)
		if subject == "" {
			return adapter.Plan{}, ErrEmptySubject
		}
		return adapter.Plan{
			AdapterID: ID, Verb: manifest.Write, Summary: "Create an Outlook draft",
			Details: map[string]string{"subject": subject, "body": in.Body},
		}, nil
	case manifest.Send:
		to := strings.TrimSpace(in.Subject)
		body := strings.TrimSpace(in.Body)
		if to == "" {
			return adapter.Plan{}, ErrEmptyRecipient
		}
		if body == "" {
			return adapter.Plan{}, ErrEmptyBody
		}
		return adapter.Plan{
			AdapterID: ID, Verb: manifest.Send, Summary: "Send an Outlook message",
			Details: map[string]string{
				"to": to, "subject": "Message from Operator", "body": body,
			},
		}, nil
	default:
		return adapter.Plan{}, fmt.Errorf("outlook: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveRead(ctx context.Context, query string) (adapter.Plan, error) {
	messages, err := a.api.ListMessages(ctx, query)
	if err != nil {
		a.logger.Error("[outlook] list messages failed", "error", err)
		return adapter.Plan{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []Message
	for _, message := range messages {
		subject := strings.ToLower(strings.TrimSpace(message.Subject))
		id := strings.ToLower(strings.TrimSpace(message.ID))
		if subject == needle || id == needle {
			exact = append(exact, message)
			continue
		}
		if needle != "" && strings.Contains(subject, needle) {
			partial = append(partial, message)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return adapter.Plan{}, ErrNoMessage
	}
	if len(matches) > 1 {
		return adapter.Plan{}, ErrAmbiguousMessage
	}
	message := matches[0]
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Handle: message.ID, Summary: "Read an Outlook message",
		Details: map[string]string{
			"message_id": message.ID, "subject": message.Subject,
			"from": message.From, "preview": message.Preview,
		},
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	lines := []string{plan.Details["subject"]}
	confirm := "Read from Outlook"
	switch plan.Verb {
	case manifest.Write:
		if body := plan.Details["body"]; body != "" {
			lines = append(lines, body)
		}
		confirm = "Create draft"
	case manifest.Send:
		lines = []string{"To: " + plan.Details["to"], plan.Details["subject"], plan.Details["body"]}
		confirm = "Send message"
	case manifest.Read:
		if from := plan.Details["from"]; from != "" {
			lines = append(lines, "From: "+from)
		}
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[outlook] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: plan.Details["subject"]}, nil
	case manifest.Write:
		created, err := a.api.CreateDraft(ctx, CreateDraft{
			Subject: plan.Details["subject"], Body: plan.Details["body"],
		})
		if err != nil {
			a.logger.Error("[outlook] create draft failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[outlook] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("created Outlook draft %s", created.ID),
		}, nil
	case manifest.Send:
		err := a.api.SendMail(ctx, SendMail{
			To: plan.Details["to"], Subject: plan.Details["subject"], Body: plan.Details["body"],
		})
		if err != nil {
			a.logger.Error("[outlook] send mail failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[outlook] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("sent Outlook message to %s", plan.Details["to"]),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("outlook: verb %q is not supported", plan.Verb)
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
		a.logger.Error("[outlook] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[outlook] revoked")
	return nil
}
