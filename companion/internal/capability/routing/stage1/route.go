// Package stage1 is the cloud half of the router: it turns an utterance
// into a Route by asking a model, and nothing else. A Route can carry a
// verb, which app class and (if the user said so) which app, a subject
// name, a body, and a confidence — and structurally nothing else. It can
// never carry a resolved handle, because resolving a subject to a handle
// is the on-device half's job (see stage2), and the cloud never sees the
// contact graph that would let it do that job itself.
package stage1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// ConfidenceFloor is the lowest confidence a route may act on. Below it,
// the route always asks rather than executes.
const ConfidenceFloor = 0.5

// ErrHandleInRoute is returned when a model reply carries a field that
// could hold a resolved handle (a phone number, an email, a thread id). It
// is rejected rather than silently dropped, so the first time a prompt
// change causes it, it is visible rather than hidden.
var ErrHandleInRoute = errors.New("stage1: reply carries a resolved handle")

// forbiddenKeys names the substrings no reply field may contain. Checked
// against the raw JSON keys before the reply is even decoded, so nothing
// pattern-matching this list can reach a Route field.
var forbiddenKeys = []string{"handle", "phone", "number", "email", "address", "thread", "contact", "recipient"}

// Route is everything stage 1 is allowed to hand to the on-device half. No
// field's name may contain a forbidden substring — enforced structurally
// by a reflection test, not just by convention.
type Route struct {
	Verb       manifest.Verb
	AppClass   string
	AppNamed   string
	Subject    string
	Body       string
	Confidence float64
}

// MustAsk reports whether this route is too weak to act on: either its
// confidence is below the floor, or it names no app class at all.
func (r Route) MustAsk() bool {
	return r.Confidence < ConfidenceFloor || r.AppClass == ""
}

// replyPayload is the wire shape of a stage-1 model reply.
type replyPayload struct {
	Verb       string  `json:"verb"`
	AppClass   string  `json:"app_class"`
	AppNamed   string  `json:"app_named"`
	Subject    string  `json:"subject"`
	Body       string  `json:"body"`
	Confidence float64 `json:"confidence"`
}

// ParseRoute decodes a model reply into a Route, refusing anything that
// carries a resolved handle, an unknown or missing verb, or is not valid
// JSON at all.
func ParseRoute(data []byte) (Route, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Route{}, fmt.Errorf("stage1: reply is not a JSON object: %w", err)
	}

	for key := range raw {
		lower := strings.ToLower(key)
		for _, bad := range forbiddenKeys {
			if strings.Contains(lower, bad) {
				return Route{}, fmt.Errorf("stage1: reply field %q: %w", key, ErrHandleInRoute)
			}
		}
	}

	var payload replyPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return Route{}, fmt.Errorf("stage1: reply does not match the expected shape: %w", err)
	}

	verb, err := manifest.ParseVerb(payload.Verb)
	if err != nil {
		return Route{}, fmt.Errorf("stage1: %w", err)
	}

	return Route{
		Verb:       verb,
		AppClass:   payload.AppClass,
		AppNamed:   payload.AppNamed,
		Subject:    payload.Subject,
		Body:       payload.Body,
		Confidence: payload.Confidence,
	}, nil
}

// ModelFunc asks the model for a raw reply to an utterance. Every caller in
// this repo injects a recorded or stubbed one, so routing tests run
// offline; only a real ModelFunc implementation reaches the network.
type ModelFunc func(ctx context.Context, utterance string) ([]byte, error)

// Router drives an utterance through a model call and into a Route.
type Router struct {
	model ModelFunc
}

// New returns a Router backed by the given model function.
func New(model ModelFunc) *Router {
	return &Router{model: model}
}

// Route asks the model for a reply to utterance and parses it into a
// Route. A model error is returned unwrapped, so a caller can test for it
// with errors.Is rather than the router silently falling back to
// something.
func (r *Router) Route(ctx context.Context, utterance string) (Route, error) {
	raw, err := r.model(ctx, utterance)
	if err != nil {
		return Route{}, err
	}
	return ParseRoute(raw)
}
