package phoneruntime

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

// agentGateThreadID is the thread the on-phone agent's gate approvals live
// on. The chat UI for this thread lands in a later phase; until then it is
// just the id gate decisions are filed under.
const agentGateThreadID = "phone-agent"

// gateApprovalTTL is how long a gate's approval sheet stays valid before it
// expires unanswered.
const gateApprovalTTL = 15 * time.Minute

// decisionSink registers a pending decision for the phone to answer.
// Satisfied by *decisions.Router.
type decisionSink interface {
	Add(decisions.Request) error
}

// eventPublisher tells the phone a task-level event happened. Satisfied by
// *mobilesession.Handler.
type eventPublisher interface {
	PublishTaskEvent(context.Context, taskstate.MobileEvent) error
}

// gateReleaser resolves a pending gate one way or the other. Satisfied by
// *agentbridge.Bridge.
type gateReleaser interface {
	ApproveGate(context.Context, string) (agentbridge.ToolCallResult, error)
	DenyGate(string) error
}

// gateApprovals is the glue between the agent bridge's hard gates and the
// launcher's approval sheet: it turns a raised gate into a decision the
// phone can see, and turns the phone's answer back into a gate release.
type gateApprovals struct {
	logger    *slog.Logger
	now       func() time.Time
	sink      decisionSink
	publisher eventPublisher
	releaser  gateReleaser
}

// newGateApprovals builds a gateApprovals. sink and releaser may be nil at
// construction and assigned afterward — the router needs approvals to
// exist before it can be built, and the bridge needs the router to exist
// before it can be built, so the cycle resolves by late-binding both.
func newGateApprovals(logger *slog.Logger, now func() time.Time, sink decisionSink, publisher eventPublisher, releaser gateReleaser) *gateApprovals {
	return &gateApprovals{logger: logger, now: now, sink: sink, publisher: publisher, releaser: releaser}
}

// GateRaised implements agentbridge.ApprovalNotifier. It registers the gate
// as a pending decision and, only once that registration succeeds, tells
// the phone about it — a phone told about a sheet that was never
// registered would have no way to ever resolve it.
func (approvals *gateApprovals) GateRaised(gate gates.Gate, preview agentbridge.PreviewSummary) {
	request := decisions.Request{
		ID:               gate.ID,
		ThreadID:         agentGateThreadID,
		TurnID:           gate.ID,
		ItemID:           gate.ID,
		Kind:             decisions.KindPermissions,
		Reason:           gateReason(gate.Kind, preview),
		AllowedDecisions: []decisions.Decision{decisions.DecisionAcceptOnce, decisions.DecisionDecline},
		ExpiresAt:        approvals.now().Add(gateApprovalTTL),
	}
	if err := approvals.sink.Add(request); err != nil {
		approvals.logger.Error("[phone-runtime] gate approval registration failed", "gate_id", gate.ID, "adapter", gate.Adapter, "verb", gate.Verb, "gate_kind", gate.Kind, "error", err.Error())
		return
	}
	event := taskstate.MobileEvent{
		TaskID:  agentGateThreadID,
		Kind:    "approval",
		State:   taskstate.WaitingForApproval,
		Summary: "Codex needs your approval",
	}
	if err := approvals.publisher.PublishTaskEvent(context.Background(), event); err != nil {
		approvals.logger.Error("[phone-runtime] gate approval phone event failed", "gate_id", gate.ID, "adapter", gate.Adapter, "verb", gate.Verb, "gate_kind", gate.Kind, "error", err.Error())
	}
}

// gateReason builds the short line the owner's sheet shows for why the
// agent is asking, reusing the same kind wording bridge.go sends back to
// the agent, prefixed to the preview's headline of what it wants to do.
func gateReason(kind gates.Kind, preview agentbridge.PreviewSummary) string {
	return fmt.Sprintf("%s: %s", gateKindPhrase(kind), preview.Headline)
}

// gateKindPhrase mirrors gateMessage in bridge.go, phrased as a short label
// rather than a full sentence back to the agent.
func gateKindPhrase(kind gates.Kind) string {
	switch kind {
	case gates.KindFirstContact:
		return "First message to this recipient"
	case gates.KindRevoke:
		return "Revoking access"
	case gates.KindIrreversible:
		return "Irreversible action"
	case gates.KindExfiltration:
		return "Sending after reading another adapter this turn"
	default:
		return "This call"
	}
}

// Respond implements decisions.Responder. A gate releases exactly once, for
// exactly the decision the owner made — never for a whole session, since a
// hard gate exists precisely to require a fresh look each time.
func (approvals *gateApprovals) Respond(ctx context.Context, response decisions.Response) error {
	switch response.Decision {
	case decisions.DecisionAcceptOnce:
		_, err := approvals.releaser.ApproveGate(ctx, response.RequestID)
		if err != nil {
			return err
		}
		approvals.logger.Info("[phone-runtime] gate approved", "gate_id", response.RequestID)
		return nil
	case decisions.DecisionDecline:
		return approvals.releaser.DenyGate(response.RequestID)
	case decisions.DecisionCancel:
		// Dismissing the sheet is not an answer: the gate stays pending for
		// a later response instead of being released either way.
		return nil
	default:
		return fmt.Errorf("phoneruntime: gate %q cannot resolve with decision %q — a gate never releases for a whole session", response.RequestID, response.Decision)
	}
}
