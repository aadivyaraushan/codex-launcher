// Package todoist is Operator's first production RT-2 adapter. It uses
// Todoist's public API v1 and exposes only the read and write verbs declared
// in its manifest.
package todoist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const ID = "todoist"

var (
	ErrAmbiguousTask = errors.New("todoist: more than one task matches; ask the user which one")
	ErrNoTask        = errors.New("todoist: no task matches")
	ErrNotConnected  = errors.New("todoist: adapter is not connected")
	ErrEmptyContent  = errors.New("todoist: task content must not be empty")
)

// API is the small part of Todoist the adapter needs. HTTPClient is the real
// implementation; tests use a recording fake.
type API interface {
	ListTasks(context.Context) ([]Task, error)
	CreateTask(context.Context, CreateTask) (Task, error)
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
		Verbs:   []manifest.Verb{manifest.Read, manifest.Write},
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthOAuth, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "todoist_write_roundtrip_smoke",
	}
}

func (a *Adapter) Resolve(ctx context.Context, in adapter.Intent) (adapter.Plan, error) {
	if a.api == nil {
		return adapter.Plan{}, ErrNotConnected
	}
	a.logger.Info("[todoist] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	switch in.Verb {
	case manifest.Write:
		content := strings.TrimSpace(in.Subject)
		if content == "" {
			return adapter.Plan{}, ErrEmptyContent
		}
		return adapter.Plan{
			AdapterID: ID, Verb: manifest.Write, Summary: "Create a Todoist task",
			Details: map[string]string{"content": content, "description": in.Body},
		}, nil
	case manifest.Read:
		return a.resolveRead(ctx, in.Subject)
	default:
		return adapter.Plan{}, fmt.Errorf("todoist: verb %q is not supported", in.Verb)
	}
}

func (a *Adapter) resolveRead(ctx context.Context, query string) (adapter.Plan, error) {
	tasks, err := a.api.ListTasks(ctx)
	if err != nil {
		a.logger.Error("[todoist] list tasks failed", "error", err)
		return adapter.Plan{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []Task
	for _, task := range tasks {
		content := strings.ToLower(task.Content)
		if content == needle {
			exact = append(exact, task)
		} else if needle != "" && strings.Contains(content, needle) {
			partial = append(partial, task)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) == 0 {
		return adapter.Plan{}, ErrNoTask
	}
	if len(matches) > 1 {
		return adapter.Plan{}, &adapter.ClarificationError{Question: "Which matching Todoist task did you mean?", Cause: ErrAmbiguousTask}
	}
	task := matches[0]
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Read, Handle: task.ID, Summary: "Read a Todoist task",
		Details: map[string]string{"task_id": task.ID, "content": task.Content, "description": task.Description},
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	if a.api == nil {
		return adapter.Preview{}, ErrNotConnected
	}
	lines := []string{plan.Details["content"]}
	if description := plan.Details["description"]; description != "" {
		lines = append(lines, description)
	}
	confirm := "Read from Todoist"
	if plan.Verb == manifest.Write {
		confirm = "Create task"
	}
	return adapter.Preview{Plan: plan, Headline: plan.Summary, Lines: lines, Confirm: confirm}, nil
}

func (a *Adapter) Execute(ctx context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	if a.api == nil {
		return adapter.Outcome{}, ErrNotConnected
	}
	a.logger.Info("[todoist] execute", "verb", plan.Verb)
	switch plan.Verb {
	case manifest.Read:
		return adapter.Outcome{Reached: manifest.Completes, Done: true, Detail: plan.Details["content"]}, nil
	case manifest.Write:
		created, err := a.api.CreateTask(ctx, CreateTask{
			Content: plan.Details["content"], Description: plan.Details["description"],
		})
		if err != nil {
			a.logger.Error("[todoist] create task failed", "error", err)
			return adapter.Outcome{}, err
		}
		a.logger.Info("[todoist] execute complete", "verb", plan.Verb, "done", true)
		return adapter.Outcome{
			Reached: manifest.Completes, Done: true,
			Detail: fmt.Sprintf("created Todoist task %s", created.ID),
		}, nil
	default:
		return adapter.Outcome{}, fmt.Errorf("todoist: verb %q is not supported", plan.Verb)
	}
}

// Revoke is the user taking their access back, and the phone can retry a
// request whose reply got lost on the way. If we already have nothing
// connected, a retried revoke should say "done" (nil), not "not connected"
// (an error) — an error here reads to the user as "disconnecting failed",
// which makes them think they are still connected when they are not.
// Asking to disconnect something already disconnected has got what it
// asked for. A real failure to clear the connection still returns its
// error below, because that one genuinely did not work.
func (a *Adapter) Revoke(ctx context.Context) error {
	if a.api == nil {
		return nil
	}
	if err := a.api.Clear(ctx); err != nil {
		a.logger.Error("[todoist] revoke failed", "error", err)
		return err
	}
	a.api = nil
	a.logger.Info("[todoist] revoked")
	return nil
}
