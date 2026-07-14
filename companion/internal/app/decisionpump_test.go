package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

func TestDecisionPumpRegistersLiveRequestAndPublishesOnlyGenericAttention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan appserver.ServerRequest, 1)
	owner := decisions.NewAppServerOwner(&pumpResponder{})
	router := decisions.NewRouter(owner, nil)
	publisher := &decisionEventPublisher{published: make(chan taskstate.MobileEvent, 1)}
	now := time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC)
	done := make(chan struct{})
	go func() {
		pumpDecisionRequests(ctx, requests, owner, router, decisionTaskLookup{}, publisher, "Aadi Mac", nil, func() time.Time { return now }, false)
		close(done)
	}()
	requests <- appserver.ServerRequest{
		ID: json.RawMessage(`"approval-1"`), Method: "item/commandExecution/requestApproval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1",
		Command: "npm test", AllowedDecisions: []appserver.ApprovalDecision{appserver.DecisionDecline},
	}
	event := <-publisher.published
	if event.TaskID != "thread-1" || event.State != taskstate.WaitingForApproval || event.Summary != "Codex needs your approval" {
		t.Fatalf("event = %#v", event)
	}
	pending := router.Pending("thread-1")
	if len(pending) != 1 || pending[0].ProjectLabel != "Launcher" || pending[0].Command != "npm test" || pending[0].ExpiresAt != now.Add(10*time.Minute) {
		t.Fatalf("pending = %#v", pending)
	}
	cancel()
	<-done
}

func TestDecisionPumpRoutesDesktopOwnedRequestBackToDesktop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan appserver.ServerRequest, 1)
	desktop := &pumpDesktopResponder{decisions: make(chan string, 1)}
	owner := decisions.NewAppServerOwner(&pumpResponder{})
	owner.AttachDesktop(desktop)
	router := decisions.NewRouter(owner, nil)
	publisher := &decisionEventPublisher{published: make(chan taskstate.MobileEvent, 1)}
	go pumpDecisionRequests(ctx, requests, owner, router, decisionTaskLookup{}, publisher, "Aadi Mac", nil, time.Now, true)
	requests <- appserver.ServerRequest{
		ID: json.RawMessage(`"desktop-approval-1"`), Method: "item/commandExecution/requestApproval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1",
		Command: "npm test", AllowedDecisions: []appserver.ApprovalDecision{appserver.DecisionDecline},
	}
	<-publisher.published
	pending := router.Pending("thread-1")
	if len(pending) != 1 {
		t.Fatalf("pending desktop decisions = %#v", pending)
	}
	request := pending[0]
	if err := router.Respond(ctx, decisions.Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: decisions.DecisionDecline}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := <-desktop.decisions; got != "thread-1/desktop-approval-1/decline" {
		t.Fatalf("desktop response = %q", got)
	}
}

type decisionTaskLookup struct{}

func (decisionTaskLookup) CurrentTask(context.Context, string) (taskstate.Task, error) {
	return taskstate.Task{ID: "thread-1", ProjectLabel: "Launcher"}, nil
}

type decisionEventPublisher struct{ published chan taskstate.MobileEvent }

func (publisher *decisionEventPublisher) PublishTaskEvent(_ context.Context, event taskstate.MobileEvent) error {
	publisher.published <- event
	return nil
}

type pumpResponder struct{}

type pumpDesktopResponder struct{ decisions chan string }

func (desktop *pumpDesktopResponder) RouteApprovalDecision(_ context.Context, threadID, requestID, decision string) error {
	desktop.decisions <- threadID + "/" + requestID + "/" + decision
	return nil
}
func (*pumpDesktopResponder) RouteFileApprovalDecision(context.Context, string, string, string) error {
	return nil
}
func (*pumpDesktopResponder) RespondPermissionRequest(context.Context, string, string, desktopipc.PermissionResponse) error {
	return nil
}
func (*pumpDesktopResponder) SubmitUserInput(context.Context, string, string, desktopipc.UserInputResponse) error {
	return nil
}
func (*pumpDesktopResponder) SubmitMCP(context.Context, string, string, desktopipc.MCPResponse) error {
	return nil
}

func (*pumpResponder) RespondCommandApproval(context.Context, string, json.RawMessage, appserver.ApprovalDecision) error {
	return nil
}
func (*pumpResponder) RespondFileApproval(context.Context, string, json.RawMessage, appserver.ApprovalDecision) error {
	return nil
}
func (*pumpResponder) RespondPermissions(context.Context, string, json.RawMessage, json.RawMessage, string) error {
	return nil
}
func (*pumpResponder) RespondUserInput(context.Context, string, json.RawMessage, map[string][]string) error {
	return nil
}
func (*pumpResponder) RespondMCP(context.Context, string, json.RawMessage, string, any) error {
	return nil
}
