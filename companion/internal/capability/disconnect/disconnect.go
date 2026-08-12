// Package disconnect undoes a connection completely: it is the one place a
// phone action can reach that joins the two halves of a revoke that
// otherwise live separately — the adapter's own credential drop and the
// consent store's grant/vault wipe — so a single user action undoes both.
//
// It replaces flow.Service.Disconnect for the routed pipeline once that
// pipeline is deleted. Everything about the half-failure rules carries over
// unchanged, because a disconnect that reports success while a token is
// still live is worse than one that plainly fails. What does not carry over
// is the pending-preview scan: flow.Service tracked pending previews keyed
// by owner and request id and had to drop any that pointed at the adapter
// being disconnected; this package has no such thing to track.
package disconnect

import (
	"context"
	"log/slog"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
)

// Service disconnects adapters: it drops their credentials via the
// execution runner and their consent grant via the consent store.
type Service struct {
	runner *execution.Runner
	gate   *consent.Store
	logger *slog.Logger

	mu           sync.Mutex
	disconnected map[string]bool
}

// New builds a Service. A nil logger falls back to slog.Default().
func New(runner *execution.Runner, gate *consent.Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		runner:       runner,
		gate:         gate,
		logger:       logger,
		disconnected: make(map[string]bool),
	}
}

// Disconnect undoes a connection completely.
//
// Credentials go first. If the consent record were cleared first and
// dropping the credentials then failed, the app would have thrown away its
// own memory of the connection while the live token is still out there:
// nothing left to point a retry at, and the user told nothing is wrong. So a
// credential revoke that fails stops here, before the consent store is
// touched at all, and reports the failure.
//
// runner.Revoke unregisters the adapter for good once it succeeds, so a
// second call for the same id can never reach the registry again — the
// registry would report it unknown, indistinguishable from an id this build
// never had. disconnected remembers which ids have already been through
// here, so a repeat call (a user tapping disconnect twice because the
// screen didn't confirm it worked) short-circuits to success instead of
// failing on a lookup that will never come back, while an id this build
// truly does not have still fails at the registry as it should.
func (s *Service) Disconnect(ctx context.Context, adapterID string) error {
	s.mu.Lock()
	already := s.disconnected[adapterID]
	s.mu.Unlock()
	if already {
		s.logger.Info("[capability-disconnect] disconnect", "adapter_id", adapterID, "result", "already_disconnected")
		return nil
	}

	if err := s.runner.Revoke(ctx, adapterID); err != nil {
		s.logger.Error("[capability-disconnect] disconnect failed", "adapter_id", adapterID, "step", "credentials", "error", err)
		return err
	}

	// The credentials are gone, so this id can never be reached through the
	// registry again. Mark it now, before the consent step, so a retry after
	// a consent-step failure below finds it here and skips straight past the
	// registry lookup that would otherwise fail.
	s.mu.Lock()
	s.disconnected[adapterID] = true
	s.mu.Unlock()

	if _, err := s.gate.Revoke(ctx, adapterID); err != nil {
		s.logger.Error("[capability-disconnect] disconnect failed", "adapter_id", adapterID, "step", "consent", "error", err)
		return err
	}

	s.logger.Info("[capability-disconnect] disconnected", "adapter_id", adapterID)
	return nil
}

// Disconnected reports whether adapterID has already been through Disconnect
// here. It is how a caller tells a repeat disconnect (idempotent success)
// apart from an id this build never had.
func (s *Service) Disconnected(adapterID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disconnected[adapterID]
}
