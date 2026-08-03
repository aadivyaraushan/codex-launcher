// Package instagram is Operator's Wave-1 Instagram DM hand-off adapter.
// It prepares draft text and opens Instagram on the phone; it never logs
// in, never reads the account, and never claims a message was sent.
package instagram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/handoff"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const (
	ID             = "instagram"
	AndroidPackage = "com.instagram.android"
	AppName        = "Instagram"
)

var ErrEmptyDraft = errors.New("instagram: draft text must not be empty")

type Adapter struct {
	logger *slog.Logger
}

var _ adapter.Adapter = (*Adapter)(nil)

func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{logger: logger}
}

func (a *Adapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID: ID, Runtime: manifest.RT4,
		Verbs:   []manifest.Verb{manifest.Compose},
		Ceiling: manifest.HandsOff, Consent: manifest.ConsentA,
		Auth: manifest.AuthNone, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "instagram_draft_open_smoke",
	}
}

func (a *Adapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	a.logger.Info("[instagram] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))
	if in.Verb != manifest.Compose {
		return adapter.Plan{}, fmt.Errorf("instagram: verb %q is not supported; use compose", in.Verb)
	}
	draft := strings.TrimSpace(in.Body)
	if draft == "" {
		return adapter.Plan{}, ErrEmptyDraft
	}
	details := map[string]string{
		"draft":           draft,
		"android_package": AndroidPackage,
	}
	if subject := strings.TrimSpace(in.Subject); subject != "" {
		details["subject_hint"] = subject
	}
	a.logger.Info("[instagram] resolve ready", "draft_length", len(draft), "has_subject_hint", details["subject_hint"] != "")
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Compose,
		Summary: "Prepare an Instagram draft",
		Details: details,
	}, nil
}

func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	draft := plan.Details["draft"]
	a.logger.Info("[instagram] preview", "draft_length", len(draft), "has_subject_hint", plan.Details["subject_hint"] != "")
	lines := make([]string, 0, 3)
	if hint := plan.Details["subject_hint"]; hint != "" {
		lines = append(lines, "For: "+hint+" (you choose the thread in Instagram)")
	}
	lines = append(lines, draft)
	lines = append(lines, "Operator opens Instagram only. You paste and finish there.")
	return adapter.Preview{
		Plan:     plan,
		Headline: plan.Summary,
		Lines:    lines,
		Confirm:  "Open Instagram",
	}, nil
}

func (a *Adapter) Execute(_ context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	draft := plan.Details["draft"]
	a.logger.Info("[instagram] execute", "draft_length", len(draft), "handed_off_to", AppName)
	out := handoff.DraftOutcome(AppName, draft)
	a.logger.Info("[instagram] execute complete", "reached", out.Reached, "done", out.Done, "handed_off_to", out.HandedOffTo)
	return out, nil
}

func (a *Adapter) Revoke(context.Context) error {
	a.logger.Info("[instagram] revoke", "decision", "noop_no_credentials")
	return nil
}
