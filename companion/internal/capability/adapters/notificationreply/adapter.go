// Package notificationreply is the adapter that answers a live message
// thread through the reply box Android puts inside a notification.
//
// Every other adapter in this tree finishes the whole job itself, on the
// Mac. This one cannot: only the phone knows which conversations still have
// a live notification to reply into, and only the phone can type into that
// reply box. So Resolve does the ordinary job of checking the request makes
// sense, and Execute does something this codebase otherwise never does — it
// refuses to act, and instead hands the decision to the phone by returning
// an *adapter.DeviceWorkError. Everything downstream of that error already
// exists and already works (see adapter_test.go for the full chain); this
// file is what starts it.
package notificationreply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

const (
	// ID is how the registry, the router and the inventory all name this
	// adapter.
	ID = "notification_reply"

	// Class is the routing class this adapter answers. It holds exactly one
	// adapter — only the phone can ever carry out a reply, so there is never
	// a second surface to choose between.
	Class = "notification_reply"

	// deviceWorkKind is the value the phone matches against when it decides
	// what kind of device work it was handed (contract/validation.go:1100).
	// It is written down separately from ID even though the two happen to
	// share a spelling today, because they answer different questions: ID is
	// how this adapter is addressed on the Mac, deviceWorkKind is a word on
	// the wire the phone depends on staying exactly this.
	deviceWorkKind = "notification_reply"
)

// ErrNoName is returned when a request names nobody to reply to. A blank
// name would ask the phone to match against every open conversation instead
// of the one the user meant, so this is refused before it ever reaches the
// device.
var ErrNoName = errors.New("notificationreply: no name to reply to")

// ErrEmptyReply is returned when the message body is blank. A blank
// RemoteInput would post an empty message into a real person's chat, which
// cannot be taken back, so this is refused before it ever reaches the
// device.
var ErrEmptyReply = errors.New("notificationreply: reply text must not be empty")

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
		Verbs: []manifest.Verb{manifest.Send},
		// Completes, not a lower ceiling: once the phone fires the reply
		// there is nothing left for the user to do. The open question is
		// whether the recipient actually got it, and that is a question of
		// certainty, not effort — this codebase already has a separate word
		// for that doubt (outcome_unknown) and the ceiling is not it.
		Ceiling: manifest.Completes, Consent: manifest.ConsentA,
		Auth: manifest.AuthNone, Cost: manifest.CostFree,
		Gates:    []manifest.Gate{manifest.GateNone},
		Capacity: manifest.Capacity{Kind: manifest.CapacityNone},
		Region:   []string{"global"}, Platform: manifest.PlatformAndroid,
		ProvesCeiling: "notification_reply_remote_input_accepted_smoke",
	}
}

// Resolve checks the request makes sense and builds the plan Execute will
// hand to the phone. The name is kept exactly as the router typed it — see
// the package doc — so only whitespace-only names are rejected; nothing
// here lowercases, trims content, or maps it to any canonical form.
func (a *Adapter) Resolve(_ context.Context, in adapter.Intent) (adapter.Plan, error) {
	a.logger.Info("[notification_reply] resolve", "verb", in.Verb, "subject_length", len(in.Subject), "body_length", len(in.Body))

	if in.Verb != manifest.Send {
		return adapter.Plan{}, fmt.Errorf("notificationreply: verb %q is not supported; use send", in.Verb)
	}
	if strings.TrimSpace(in.Subject) == "" {
		a.logger.Info("[notification_reply] resolve refused", "reason", "no_name")
		return adapter.Plan{}, ErrNoName
	}
	if strings.TrimSpace(in.Body) == "" {
		a.logger.Info("[notification_reply] resolve refused", "reason", "empty_reply")
		return adapter.Plan{}, ErrEmptyReply
	}

	details := map[string]string{
		"handle": in.Subject,
		"text":   in.Body,
	}
	a.logger.Info("[notification_reply] resolve ready", "handle_length", len(in.Subject), "text_length", len(in.Body))
	return adapter.Plan{
		AdapterID: ID, Verb: manifest.Send,
		Handle:  in.Subject,
		Summary: "Reply on the phone",
		Details: details,
	}, nil
}

// Preview shows exactly who the reply goes to and exactly what it says —
// nothing summarized or shortened — because a confirm step that hides
// either one is not consent to anything.
func (a *Adapter) Preview(_ context.Context, plan adapter.Plan) (adapter.Preview, error) {
	handle := plan.Details["handle"]
	text := plan.Details["text"]
	a.logger.Info("[notification_reply] preview", "handle_length", len(handle), "text_length", len(text))
	return adapter.Preview{
		Plan:     plan,
		Headline: "Reply to " + handle,
		Lines:    []string{text},
		Confirm:  "Send reply",
	}, nil
}

// Execute never talks to anything itself. It hands the plan to the phone by
// returning a DeviceWorkError — the signal handleDeviceAction on the phone
// is already built to receive — carrying the declared ceiling along with it
// so the result path never has to guess or hardcode one.
func (a *Adapter) Execute(_ context.Context, plan adapter.Plan) (adapter.Outcome, error) {
	handle := plan.Details["handle"]
	text := plan.Details["text"]
	a.logger.Info("[notification_reply] execute", "handle_length", len(handle), "text_length", len(text), "decision", "hand_off_to_phone")
	return adapter.Outcome{}, &adapter.DeviceWorkError{
		AdapterID: ID,
		Kind:      deviceWorkKind,
		Handle:    handle,
		Text:      text,
		Ceiling:   a.Describe().Ceiling,
	}
}

func (a *Adapter) Revoke(context.Context) error {
	a.logger.Info("[notification_reply] revoke", "decision", "noop_no_credentials")
	return nil
}
