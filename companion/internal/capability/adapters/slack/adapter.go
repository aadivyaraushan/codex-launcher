// Package slack is Operator's Wave 1 Slack adapter. It uses a Slack user
// OAuth token and exposes only read (resolve a channel) and send (post a
// message the user confirmed).
package slack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "slack"

var (
	ErrAmbiguousChannel = errors.New("slack: more than one channel matches; ask the user which one")
	ErrNoChannel        = errors.New("slack: no channel matches")
	ErrNotConnected     = errors.New("slack: adapter is not connected")
	ErrEmptyMessage     = errors.New("slack: message text must not be empty")
	ErrEmptyChannel     = errors.New("slack: channel name must not be empty")
)

// API is the small part of Slack Web API the adapter needs.
type API interface {
	ListChannels(context.Context) ([]Channel, error)
	PostMessage(context.Context, PostMessage) (PostedMessage, error)
	Clear(context.Context) error
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
		ProvesCeiling: "slack_user_oauth_channel_read_send",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[slack] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	switch in.Verb {
	case manifest.Read:
		return a.resolveChannel(ctx, in.Subject, manifest.Read, "")
	case manifest.Send:
		text := strings.TrimSpace(in.Body)
		if text == "" {
			return adapter.Plan{}, ErrEmptyMessage
		}
		plan, err := a.resolveChannel(ctx, in.Subject, manifest.Send, text)
		if err != nil {
			return adapter.Plan{}, err
		}
		plan.Summary = "Send a Slack message"
		plan.Details["text"] = text
		return plan, nil
	default:
		return adapter.Plan{}, fmt.Errorf("slack: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveChannel(ctx context.Context, query string, verb manifest.Verb, text string) (adapter.Plan, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return adapter.Plan{}, ErrEmptyChannel
	}
	channels, err := a.api.ListChannels(ctx)
	if err != nil {
		a.logger.Error("[slack] list channels failed", "error", err)
		return adapter.Plan{}, err
	}
	var exact, partial []Channel
	for _, channel := range channels {
		if channel.Archived {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(channel.Name))
		id := strings.ToLower(strings.TrimSpace(channel.ID))
		if name == needle || id == needle {
			exact = append(exact, channel)
			continue
		}
		if strings.Contains(name, needle) {
			partial = append(partial, channel)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return adapter.Plan{}, ErrNoChannel
	}
	if len(matches) > 1 {
		return adapter.Plan{}, ErrAmbiguousChannel
	}
	channel := matches[0]
	summary := "Read a Slack channel"
	if verb == manifest.Send {
		summary = "Send a Slack message"
	}
	details := map[string]string{
		"channel_id":   channel.ID,
		"channel_name": channel.Name,
	}
	if text != "" {
		details["text"] = text
	}
	return adapter.Plan{
		AdapterID: ID, Verb: verb, Handle: channel.ID, Summary: summary, Details: details,
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	lines := []string{"#" + plan.Details["channel_name"]}
	confirm := "Read from Slack"
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
	a.logger.Info("[slack] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		detail := fmt.Sprintf("#%s (%s)", plan.Details["channel_name"], plan.Details["channel_id"])
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: detail}, nil
	case manifest.Send:
		posted, err := a.api.PostMessage(ctx, PostMessage{
			Channel: plan.Details["channel_id"], Text: plan.Details["text"],
		})
		if err != nil {
			a.logger.Error("[slack] post message failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[slack] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("posted to #%s ts=%s", plan.Details["channel_name"], posted.Timestamp),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("slack: verb %q is not supported", plan.Verb)
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
		a.logger.Error("[slack] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[slack] revoked")
	return nil
}
