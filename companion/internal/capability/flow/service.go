// Package flow joins the two router stages to the execution runner. It owns
// pending previews so a phone confirmation can unlock only the exact plan the
// user saw, once.
package flow

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/consent"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage2"
)

var (
	ErrUnknownRequest      = errors.New("capability flow: request has no pending preview")
	ErrRequestExists       = errors.New("capability flow: request already has a pending preview")
	ErrFingerprintMismatch = errors.New("capability flow: confirmation does not match the preview")
)

type QuestionError struct {
	Question string
}

func (e *QuestionError) Error() string { return "capability flow: " + e.Question }

type Preview struct {
	RequestID   string
	AdapterID   string
	Verb        manifest.Verb
	Headline    string
	Lines       []string
	Confirm     string
	Fingerprint string
}

type pendingRequest struct {
	plan    adapter.Plan
	preview execution.Preview
	timer   expiryTimer
}

type expiryTimer interface {
	Stop() bool
}

type Service struct {
	stage1 *stage1.Router
	stage2 *stage2.Resolver
	runner *execution.Runner
	gate   *consent.Store
	logger *slog.Logger
	ttl    time.Duration
	after  func(time.Duration, func()) expiryTimer

	mu           sync.Mutex
	pending      map[string]pendingRequest
	disconnected map[string]bool
}

func New(router *stage1.Router, resolver *stage2.Resolver, runner *execution.Runner, gate *consent.Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		stage1: router, stage2: resolver, runner: runner, gate: gate, logger: logger,
		ttl: 5 * time.Minute,
		after: func(delay time.Duration, run func()) expiryTimer {
			return time.AfterFunc(delay, run)
		},
		pending:      make(map[string]pendingRequest),
		disconnected: make(map[string]bool),
	}
}

func (s *Service) Prepare(ctx context.Context, ownerID, requestID, utterance string) (Preview, error) {
	key := pendingKey(ownerID, requestID)
	s.mu.Lock()
	_, exists := s.pending[key]
	s.mu.Unlock()
	if exists {
		return Preview{}, ErrRequestExists
	}
	s.logger.Info("[capability-flow] prepare", "request_id", requestID, "utterance_bytes", len(utterance))
	route, err := s.stage1.Route(ctx, utterance)
	if err != nil {
		s.logger.Error("[capability-flow] stage1 failed", "request_id", requestID, "error", err)
		return Preview{}, err
	}
	decision, err := s.stage2.Resolve(ctx, route)
	if err != nil {
		return Preview{}, err
	}
	if decision.MustAsk {
		s.logger.Info("[capability-flow] question required", "request_id", requestID, "reason", "route_not_unique")
		return Preview{}, &QuestionError{Question: decision.Question}
	}
	m, err := s.runner.Describe(decision.AdapterID)
	if err != nil {
		return Preview{}, err
	}
	if err := s.gate.Allow(ctx, m); err != nil {
		s.logger.Info("[capability-flow] refused", "adapter_id", decision.AdapterID, "reason", "consent_not_granted")
		return Preview{}, err
	}
	plan, err := s.runner.Resolve(ctx, adapter.Intent{
		AdapterID: decision.AdapterID, Verb: decision.Verb,
		Subject: route.Subject, Handle: decision.Handle, Body: decision.Body,
		Fields: decision.Fields,
	})
	if err != nil {
		return Preview{}, err
	}
	shown, err := s.runner.Preview(ctx, plan)
	if err != nil {
		return Preview{}, err
	}
	fingerprint := shown.Fingerprint()
	s.mu.Lock()
	if _, exists := s.pending[key]; exists {
		s.mu.Unlock()
		return Preview{}, ErrRequestExists
	}
	s.pending[key] = pendingRequest{plan: plan, preview: shown}
	timer := s.after(s.ttl, func() { s.expire(key, fingerprint) })
	pending := s.pending[key]
	pending.timer = timer
	s.pending[key] = pending
	s.mu.Unlock()
	s.logger.Info("[capability-flow] preview ready", "request_id", requestID, "adapter_id", decision.AdapterID, "verb", decision.Verb, "line_count", len(shown.Lines))
	return Preview{
		RequestID: requestID, AdapterID: decision.AdapterID, Verb: decision.Verb,
		Headline: shown.Headline, Lines: append([]string(nil), shown.Lines...), Confirm: shown.Confirm,
		Fingerprint: fingerprint,
	}, nil
}

func (s *Service) Confirm(ctx context.Context, ownerID, requestID, fingerprint string) (adapter.Outcome, error) {
	key := pendingKey(ownerID, requestID)
	s.mu.Lock()
	pending, exists := s.pending[key]
	if !exists {
		s.mu.Unlock()
		return adapter.Outcome{}, ErrUnknownRequest
	}
	if pending.preview.Fingerprint() != fingerprint {
		s.mu.Unlock()
		s.logger.Warn("[capability-flow] confirmation rejected", "request_id", requestID, "reason", "fingerprint_mismatch")
		return adapter.Outcome{}, ErrFingerprintMismatch
	}
	delete(s.pending, key)
	pending.timer.Stop()
	s.mu.Unlock()
	// The gate runs before anything is logged as accepted. A refusal here is
	// a user who withdrew permission while the sheet was still up, and a log
	// that says "confirmation accepted" just above "refused" sends whoever
	// reads it looking for the wrong problem.
	m, err := s.runner.Describe(pending.plan.AdapterID)
	if err != nil {
		return adapter.Outcome{}, err
	}
	if err := s.gate.Allow(ctx, m); err != nil {
		s.logger.Info("[capability-flow] refused", "adapter_id", pending.plan.AdapterID, "reason", "consent_not_granted")
		return adapter.Outcome{}, err
	}
	s.logger.Info("[capability-flow] confirmation accepted", "request_id", requestID, "adapter_id", pending.plan.AdapterID, "verb", pending.plan.Verb)
	outcome, err := s.runner.Execute(ctx, pending.plan, pending.preview.Confirmed())
	if err != nil {
		s.logger.Error("[capability-flow] execute failed", "request_id", requestID, "adapter_id", pending.plan.AdapterID, "verb", pending.plan.Verb, "error", err)
		return adapter.Outcome{}, err
	}
	s.logger.Info("[capability-flow] execute complete", "request_id", requestID, "reached", outcome.Reached, "done", outcome.Done)
	return outcome, nil
}

func (s *Service) Cancel(ownerID, requestID, fingerprint string) error {
	key := pendingKey(ownerID, requestID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pending[key]
	if !exists {
		return ErrUnknownRequest
	}
	if pending.preview.Fingerprint() != fingerprint {
		return ErrFingerprintMismatch
	}
	delete(s.pending, key)
	pending.timer.Stop()
	s.logger.Info("[capability-flow] preview cancelled", "request_id", requestID)
	return nil
}

func (s *Service) Pending(ownerID, requestID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.pending[pendingKey(ownerID, requestID)]
	return exists
}

// Disconnect undoes a connection completely: it is the one method a phone
// action can reach that joins the two halves of a revoke that already
// existed separately — the adapter's own credential drop and the consent
// store's grant/vault wipe — so a single user action undoes both.
//
// Credentials go first. If the consent record were cleared first and
// dropping the credentials then failed, Operator would have thrown away its
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
		s.logger.Info("[capability-flow] disconnect", "adapter_id", adapterID, "result", "already_disconnected")
		return nil
	}

	if err := s.runner.Revoke(ctx, adapterID); err != nil {
		s.logger.Error("[capability-flow] disconnect failed", "adapter_id", adapterID, "step", "credentials", "error", err)
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
		s.logger.Error("[capability-flow] disconnect failed", "adapter_id", adapterID, "step", "consent", "error", err)
		return err
	}

	// Drop every pending preview for this adapter — a sheet still on screen
	// for the app the user just disconnected must not be confirmable
	// afterwards. Previews are keyed by owner+requestID, not by adapter, so
	// this has to scan. Held under s.mu; nothing called in here takes the
	// lock itself.
	s.mu.Lock()
	for key, pending := range s.pending {
		if pending.plan.AdapterID != adapterID {
			continue
		}
		pending.timer.Stop()
		delete(s.pending, key)
	}
	s.mu.Unlock()

	s.logger.Info("[capability-flow] disconnected", "adapter_id", adapterID)
	return nil
}

func (s *Service) expire(key, fingerprint string) {
	s.mu.Lock()
	pending, exists := s.pending[key]
	if exists && pending.preview.Fingerprint() == fingerprint {
		delete(s.pending, key)
	}
	s.mu.Unlock()
	if exists {
		s.logger.Info("[capability-flow] preview expired", "reason", "confirmation_timeout")
	}
}

func pendingKey(ownerID, requestID string) string {
	return ownerID + "\x00" + requestID
}
