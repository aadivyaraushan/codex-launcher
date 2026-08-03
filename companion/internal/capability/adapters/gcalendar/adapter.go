// Package gcalendar is Operator's Wave 1 Google Calendar adapter. It uses a
// user OAuth token with calendar.events and exposes read + preview-gated write.
package gcalendar

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "gcalendar"

var (
	ErrAmbiguousEvent = errors.New("gcalendar: more than one event matches; ask the user which one")
	ErrNoEvent        = errors.New("gcalendar: no event matches")
	ErrNotConnected   = errors.New("gcalendar: adapter is not connected")
	ErrEmptySummary   = errors.New("gcalendar: event summary must not be empty")
)

// API is the small part of Calendar the adapter needs.
type API interface {
	ListEvents(context.Context, string) ([]Event, error)
	CreateEvent(context.Context, CreateEvent) (Event, error)
	Clear(context.Context) error
}

type Event struct {
	ID          string
	Summary     string
	Description string
	Start       string
	End         string
}

type CreateEvent struct {
	Summary     string
	Description string
	Start       string
	End         string
}

type Adapter struct {
	api    API
	logger *slog.Logger
	now    func() time.Time
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(api API, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{api: api, logger: logger, now: time.Now}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT2,
		Verbs:   []manifest.Verb{manifest.Read, manifest.Write},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "gcalendar_events_oauth_read_write",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[gcalendar] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	switch in.Verb {
	case manifest.Write:
		summary := strings.TrimSpace(in.Subject)
		if summary == "" {
			return adapter.Plan{}, ErrEmptySummary
		}
		start, end := a.defaultWindow()
		return adapter.Plan{
			AdapterID: ID, Verb: manifest.Write, Summary: "Create a Calendar event",
			Details: map[string]string{
				"summary": summary, "description": in.Body, "start": start, "end": end,
			},
		}, nil
	case manifest.Read:
		return a.resolveRead(ctx, in.Subject)
	default:
		return adapter.Plan{}, fmt.Errorf("gcalendar: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveRead(ctx context.Context, query string) (adapter.Plan, error) {
	events, err := a.api.ListEvents(ctx, query)
	if err != nil {
		a.logger.Error("[gcalendar] list events failed", "error", err)
		return adapter.Plan{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []Event
	for _, event := range events {
		summary := strings.ToLower(strings.TrimSpace(event.Summary))
		id := strings.ToLower(strings.TrimSpace(event.ID))
		if summary == needle || id == needle {
			exact = append(exact, event)
			continue
		}
		if needle != "" && strings.Contains(summary, needle) {
			partial = append(partial, event)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return adapter.Plan{}, ErrNoEvent
	}
	if len(matches) > 1 {
		return adapter.Plan{}, ErrAmbiguousEvent
	}
	event := matches[0]
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Handle: event.ID, Summary: "Read a Calendar event",
		Details: map[string]string{
			"event_id": event.ID, "summary": event.Summary, "description": event.Description,
			"start": event.Start, "end": event.End,
		},
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	lines := []string{plan.Details["summary"]}
	if description := plan.Details["description"]; description != "" {
		lines = append(lines, description)
	}
	if start := plan.Details["start"]; start != "" {
		lines = append(lines, start+" → "+plan.Details["end"])
	}
	confirm := "Read from Calendar"
	if plan.Verb == manifest.Write {
		confirm = "Create event"
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[gcalendar] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: plan.Details["summary"]}, nil
	case manifest.Write:
		created, err := a.api.CreateEvent(ctx, CreateEvent{
			Summary: plan.Details["summary"], Description: plan.Details["description"],
			Start: plan.Details["start"], End: plan.Details["end"],
		})
		if err != nil {
			a.logger.Error("[gcalendar] create event failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[gcalendar] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("created Calendar event %s", created.ID),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("gcalendar: verb %q is not supported", plan.Verb)
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
		a.logger.Error("[gcalendar] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[gcalendar] revoked")
	return nil
}

func (a *Adapter) defaultWindow() (string, string) {
	now := a.now
	if now == nil {
		now = time.Now
	}
	start := now().UTC().Truncate(time.Hour).Add(time.Hour)
	end := start.Add(time.Hour)
	return start.Format(time.RFC3339), end.Format(time.RFC3339)
}
