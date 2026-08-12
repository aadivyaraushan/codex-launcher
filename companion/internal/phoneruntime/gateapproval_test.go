package phoneruntime

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

type recordingSink struct {
	requests []decisions.Request
	err      error
}

func (s *recordingSink) Add(request decisions.Request) error {
	if s.err != nil {
		return s.err
	}
	s.requests = append(s.requests, request)
	return nil
}

type recordingPublisher struct {
	events []taskstate.MobileEvent
}

func (p *recordingPublisher) PublishTaskEvent(_ context.Context, event taskstate.MobileEvent) error {
	p.events = append(p.events, event)
	return nil
}

type recordingReleaser struct {
	approved   []string
	denied     []string
	approveErr error
}

func (r *recordingReleaser) ApproveGate(_ context.Context, gateID string) (agentbridge.ToolCallResult, error) {
	if r.approveErr != nil {
		return agentbridge.ToolCallResult{}, r.approveErr
	}
	r.approved = append(r.approved, gateID)
	return agentbridge.ToolCallResult{OK: true, Done: true}, nil
}

func (r *recordingReleaser) DenyGate(gateID string) error {
	r.denied = append(r.denied, gateID)
	return nil
}

func testClock() func() time.Time {
	at := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	return func() time.Time { return at }
}

func newTestGateApprovals(sink *recordingSink, publisher *recordingPublisher, releaser *recordingReleaser) *gateApprovals {
	return newGateApprovals(slog.New(slog.DiscardHandler), testClock(), sink, publisher, releaser)
}

func sampleGate() (gates.Gate, agentbridge.PreviewSummary) {
	gate := gates.Gate{
		ID:        "gate-1",
		Kind:      gates.KindFirstContact,
		Adapter:   "beeper.message",
		Verb:      "send",
		Recipient: "+15550000001",
	}
	preview := agentbridge.PreviewSummary{Headline: "send \"hi\" to +15550000001", Lines: []string{"to: +15550000001"}, Confirm: "Send"}
	return gate, preview
}

func TestGateRaisedRegistersApprovalAndTellsPhone(t *testing.T) {
	sink := &recordingSink{}
	publisher := &recordingPublisher{}
	approvals := newTestGateApprovals(sink, publisher, &recordingReleaser{})

	gate, preview := sampleGate()
	approvals.GateRaised(gate, preview)

	if len(sink.requests) != 1 {
		t.Fatalf("raised gate must register exactly one pending decision, got %d", len(sink.requests))
	}
	request := sink.requests[0]
	if request.ID != "gate-1" {
		t.Fatalf("decision id must be the gate id, got %q", request.ID)
	}
	if request.ThreadID != agentGateThreadID {
		t.Fatalf("decision thread = %q, want the agent's own thread %q", request.ThreadID, agentGateThreadID)
	}
	if request.Kind != decisions.KindPermissions {
		t.Fatalf("decision kind = %q, want %q", request.Kind, decisions.KindPermissions)
	}
	// The router refuses requests without TurnID/ItemID, and the phone echoes
	// them back verbatim in its response (mobilesession handler builds the
	// Response from the pending request's fields) — so both must be set.
	if request.TurnID == "" || request.ItemID == "" {
		t.Fatalf("request must carry routable TurnID/ItemID, got turn=%q item=%q", request.TurnID, request.ItemID)
	}
	if request.Reason == "" {
		t.Fatal("the owner's sheet must say what the agent is asking to do")
	}
	if request.ComputerName == "" || request.ProjectLabel == "" {
		t.Fatalf("decision_page requires non-empty computerName/projectLabel, got computer=%q project=%q", request.ComputerName, request.ProjectLabel)
	}
	wantDecisions := []decisions.Decision{decisions.DecisionAcceptOnce, decisions.DecisionDecline}
	if len(request.AllowedDecisions) != len(wantDecisions) {
		t.Fatalf("allowed decisions = %v, want exactly accept and decline — a gate releases once, never for a whole session", request.AllowedDecisions)
	}
	for i, want := range wantDecisions {
		if request.AllowedDecisions[i] != want {
			t.Fatalf("allowed decisions = %v, want %v", request.AllowedDecisions, wantDecisions)
		}
	}
	if !request.ExpiresAt.After(testClock()()) {
		t.Fatalf("decision must expire after now, got %v", request.ExpiresAt)
	}

	if len(publisher.events) != 1 {
		t.Fatalf("raised gate must publish exactly one phone event, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.TaskID != agentGateThreadID || event.Kind != "approval" {
		t.Fatalf("phone event = %+v, want kind approval on thread %q", event, agentGateThreadID)
	}
}

func TestGateRaisedSinkFailureDoesNotNotifyPhone(t *testing.T) {
	sink := &recordingSink{err: errors.New("router full")}
	publisher := &recordingPublisher{}
	approvals := newTestGateApprovals(sink, publisher, &recordingReleaser{})

	gate, preview := sampleGate()
	approvals.GateRaised(gate, preview)

	if len(publisher.events) != 0 {
		t.Fatalf("phone must not be told about a sheet that was never registered, got %+v", publisher.events)
	}
}

func TestAcceptReleasesExactlyThatGate(t *testing.T) {
	releaser := &recordingReleaser{}
	approvals := newTestGateApprovals(&recordingSink{}, &recordingPublisher{}, releaser)

	err := approvals.Respond(context.Background(), decisions.Response{
		RequestID: "gate-1", ThreadID: agentGateThreadID, Kind: decisions.KindPermissions,
		Decision: decisions.DecisionAcceptOnce,
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if len(releaser.approved) != 1 || releaser.approved[0] != "gate-1" {
		t.Fatalf("accept must approve exactly gate-1, got %v", releaser.approved)
	}
	if len(releaser.denied) != 0 {
		t.Fatalf("accept must not deny anything, got %v", releaser.denied)
	}
}

func TestDeclineDeniesTheGate(t *testing.T) {
	releaser := &recordingReleaser{}
	approvals := newTestGateApprovals(&recordingSink{}, &recordingPublisher{}, releaser)

	err := approvals.Respond(context.Background(), decisions.Response{
		RequestID: "gate-1", ThreadID: agentGateThreadID, Kind: decisions.KindPermissions,
		Decision: decisions.DecisionDecline,
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if len(releaser.denied) != 1 || releaser.denied[0] != "gate-1" {
		t.Fatalf("decline must deny exactly gate-1, got %v", releaser.denied)
	}
	if len(releaser.approved) != 0 {
		t.Fatalf("decline must not approve anything, got %v", releaser.approved)
	}
}

func TestCancelReleasesNothing(t *testing.T) {
	releaser := &recordingReleaser{}
	approvals := newTestGateApprovals(&recordingSink{}, &recordingPublisher{}, releaser)

	err := approvals.Respond(context.Background(), decisions.Response{
		RequestID: "gate-1", ThreadID: agentGateThreadID, Kind: decisions.KindPermissions,
		Decision: decisions.DecisionCancel,
	})
	if err != nil {
		t.Fatalf("dismissing the sheet is not an answer and must not error, got %v", err)
	}
	if len(releaser.approved) != 0 || len(releaser.denied) != 0 {
		t.Fatalf("cancel must neither approve nor deny, got approved=%v denied=%v", releaser.approved, releaser.denied)
	}
}

func TestUnexpectedDecisionReleasesNothing(t *testing.T) {
	releaser := &recordingReleaser{}
	approvals := newTestGateApprovals(&recordingSink{}, &recordingPublisher{}, releaser)

	err := approvals.Respond(context.Background(), decisions.Response{
		RequestID: "gate-1", ThreadID: agentGateThreadID, Kind: decisions.KindPermissions,
		Decision: decisions.DecisionAcceptSession,
	})
	if err == nil {
		t.Fatal("a decision outside accept/decline/cancel must be refused — a gate never releases for a whole session")
	}
	if len(releaser.approved) != 0 || len(releaser.denied) != 0 {
		t.Fatalf("refused decision must release nothing, got approved=%v denied=%v", releaser.approved, releaser.denied)
	}
}

func TestApproveFailureSurfacesError(t *testing.T) {
	releaser := &recordingReleaser{approveErr: errors.New("gate not pending")}
	approvals := newTestGateApprovals(&recordingSink{}, &recordingPublisher{}, releaser)

	err := approvals.Respond(context.Background(), decisions.Response{
		RequestID: "gate-1", ThreadID: agentGateThreadID, Kind: decisions.KindPermissions,
		Decision: decisions.DecisionAcceptOnce,
	})
	if err == nil {
		t.Fatal("a failed release must surface, so the sheet is not silently dropped")
	}
}

// The full loop through a real decisions.Router: GateRaised registers the
// pending decision, the phone's structured approval action resolves it via
// Router.Respond, and the same gate can never resolve twice.
func TestRealRouterLoopApprovesOnce(t *testing.T) {
	publisher := &recordingPublisher{}
	releaser := &recordingReleaser{}
	logger := slog.New(slog.DiscardHandler)
	approvals := newGateApprovals(logger, testClock(), nil, publisher, releaser)
	router := decisions.NewRouter(approvals, logger)
	approvals.sink = router

	gate, preview := sampleGate()
	approvals.GateRaised(gate, preview)

	pending := router.Pending(agentGateThreadID)
	if len(pending) != 1 {
		t.Fatalf("router must hold exactly the raised gate, got %d pending", len(pending))
	}
	// The phone builds its response by echoing the pending request's fields
	// (mobilesession handleAction "approval") — mirror that here.
	response := decisions.Response{
		RequestID: pending[0].ID, ThreadID: pending[0].ThreadID,
		TurnID: pending[0].TurnID, ItemID: pending[0].ItemID, Kind: pending[0].Kind,
		Decision: decisions.DecisionAcceptOnce,
	}
	if err := router.Respond(context.Background(), response, testClock()()); err != nil {
		t.Fatalf("Respond through the real router: %v", err)
	}
	if len(releaser.approved) != 1 || releaser.approved[0] != "gate-1" {
		t.Fatalf("router approval must release exactly gate-1, got %v", releaser.approved)
	}

	if err := router.Respond(context.Background(), response, testClock()()); err == nil {
		t.Fatal("a resolved decision must not resolve again")
	}
	if len(releaser.approved) != 1 {
		t.Fatalf("second response released the gate again, got %v", releaser.approved)
	}
}
