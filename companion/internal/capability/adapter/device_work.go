package adapter

import (
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

// DeviceWorkError marks a capability action that Execute cannot finish by
// itself, because the acting half of it has to happen on the phone.
//
// This is not an OutcomeUnknownError. Every other adapter decides and acts
// inside the one call to Execute, and the result is already known by the
// time it returns. A notification reply cannot work that way: the reply box
// it has to be typed into lives inside an Android notification, which only
// the phone can reach. So Execute returns this error to say "the decision
// is made, but somebody else still has to carry it out" — not to say that
// anything has gone wrong or that any outcome is in doubt. The agent bridge
// (phoneruntime/agentbridge) is what catches it, hands the ask to the phone
// through mobilesession.Handler.RunOnDevice, and waits for the answer
// inside the same tool call.
//
// Handle and Text are things a real person wrote — a contact's name and the
// words routed to them — so Error() deliberately leaves them out. Error
// strings end up in logs, and this codebase never puts user content there.
// Ceiling is the same promise every other adapter makes through
// execution.Runner's clamp (runner.go:215): the most this adapter will ever
// claim to have done. Device work skips that runner, so this is the only
// place that promise is written down before the phone answers. Leaving it
// unset is not a way to opt out of the promise — an empty Ceiling is read as
// the most modest one, hands_off, never as the strongest.
type DeviceWorkError struct {
	AdapterID string
	Kind      string
	Handle    string
	Text      string
	Ceiling   manifest.Ceiling
}

func (err *DeviceWorkError) Error() string {
	return fmt.Sprintf("%s %s has to be carried out on the phone", err.AdapterID, err.Kind)
}
