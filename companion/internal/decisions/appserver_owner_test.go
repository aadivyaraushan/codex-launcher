package decisions

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
)

func TestAppServerOwnerProjectsSafeCommandAndRoutesExactDecision(t *testing.T) {
	client := &fakeAppServerResponder{}
	owner := NewAppServerOwner(client)
	expires := time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC)
	projected, err := owner.Register(appserver.ServerRequest{
		ID: json.RawMessage(`17`), Method: "item/commandExecution/requestApproval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1",
		Command: `env API_TOKEN=secret npm test`, CWD: "/work", Reason: "Run tests", AllowedDecisions: []appserver.ApprovalDecision{appserver.DecisionAccept, appserver.DecisionDecline},
	}, DisplayContext{ComputerName: "Aadi Mac", ProjectLabel: "Launcher"}, expires)
	if err != nil {
		t.Fatal(err)
	}
	if projected.ID == "" || projected.Command != `env API_TOKEN=<redacted:secret> npm test` || projected.WorkingDirectory != "/work" || projected.ExpiresAt != expires ||
		!reflect.DeepEqual(projected.AllowedDecisions, []Decision{DecisionAcceptOnce, DecisionDecline}) {
		t.Fatalf("projected = %#v", projected)
	}
	response := Response{RequestID: projected.ID, ThreadID: projected.ThreadID, TurnID: projected.TurnID, ItemID: projected.ItemID, Kind: projected.Kind, Decision: DecisionDecline}
	if err := owner.Respond(context.Background(), response); err != nil {
		t.Fatal(err)
	}
	if len(client.command) != 1 || string(client.command[0].id) != "17" || client.command[0].decision != appserver.DecisionDecline {
		t.Fatalf("command responses = %#v", client.command)
	}
	if err := owner.Respond(context.Background(), response); !errors.Is(err, ErrOwnerUnavailable) {
		t.Fatalf("duplicate owner response error = %v", err)
	}
}

func TestAppServerOwnerProjectsQuestionsAndMapsPermissionDenyToEmptyGrant(t *testing.T) {
	client := &fakeAppServerResponder{}
	owner := NewAppServerOwner(client)
	expires := time.Now().Add(time.Minute)
	question, err := owner.Register(appserver.ServerRequest{ID: json.RawMessage(`"q-1"`), Method: "item/tool/requestUserInput", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Questions: []appserver.ServerQuestion{{ID: "password", Header: "Secret", Prompt: "Password?", Secret: true}}}, DisplayContext{ComputerName: "Mac", ProjectLabel: "Project"}, expires)
	if err != nil || len(question.Questions) != 1 || !question.Questions[0].Secret {
		t.Fatalf("question = %#v, err = %v", question, err)
	}
	permission, err := owner.Register(appserver.ServerRequest{ID: json.RawMessage(`"p-1"`), Method: "item/permissions/requestApproval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-2", CWD: "/work", Permissions: json.RawMessage(`{"network":{"enabled":true}}`)}, DisplayContext{ComputerName: "Mac", ProjectLabel: "Project"}, expires)
	if err != nil {
		t.Fatal(err)
	}
	response := Response{RequestID: permission.ID, ThreadID: permission.ThreadID, TurnID: permission.TurnID, ItemID: permission.ItemID, Kind: permission.Kind, Decision: DecisionDecline}
	if err := owner.Respond(context.Background(), response); err != nil {
		t.Fatal(err)
	}
	if len(client.permissions) != 1 || string(client.permissions[0].granted) != `{}` || client.permissions[0].scope != "turn" {
		t.Fatalf("permission responses = %#v", client.permissions)
	}
}

func TestAppServerOwnerKeepsMcpElicitationFailClosedWhenItHasNoTurnOrItem(t *testing.T) {
	client := &fakeAppServerResponder{}
	owner := NewAppServerOwner(client)
	projected, err := owner.Register(appserver.ServerRequest{
		ID: json.RawMessage(`23`), Method: "mcpServer/elicitation/request", ThreadID: "thread-1", MCPMode: "form", MCPMessage: "Choose access",
	}, DisplayContext{ComputerName: "Mac", ProjectLabel: "Project"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if projected.Kind != KindMCP || projected.TurnID == "" || projected.ItemID == "" || projected.Reason != "Choose access" ||
		!reflect.DeepEqual(projected.AllowedDecisions, []Decision{DecisionDecline, DecisionCancel}) {
		t.Fatalf("projected MCP request = %#v", projected)
	}
	response := Response{RequestID: projected.ID, ThreadID: projected.ThreadID, TurnID: projected.TurnID, ItemID: projected.ItemID, Kind: projected.Kind, Decision: DecisionDecline}
	if err := owner.Respond(context.Background(), response); err != nil {
		t.Fatal(err)
	}
	if len(client.mcp) != 1 || client.mcp[0].action != "decline" || client.mcp[0].content != nil {
		t.Fatalf("MCP responses = %#v", client.mcp)
	}
}

func TestAppServerOwnerRoutesDesktopOwnedDecisionBackThroughDesktopIPC(t *testing.T) {
	appClient := &fakeAppServerResponder{}
	desktopClient := &fakeDesktopResponder{}
	owner := NewAppServerOwner(appClient)
	owner.AttachDesktop(desktopClient)
	projected, err := owner.RegisterDesktop(appserver.ServerRequest{
		ID: json.RawMessage(`"desktop-approval-1"`), Method: "item/commandExecution/requestApproval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1",
		Command: "npm test", CWD: "/work", AllowedDecisions: []appserver.ApprovalDecision{appserver.DecisionDecline},
	}, DisplayContext{ComputerName: "Mac", ProjectLabel: "Project"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	response := Response{RequestID: projected.ID, ThreadID: projected.ThreadID, TurnID: projected.TurnID, ItemID: projected.ItemID, Kind: projected.Kind, Decision: DecisionDecline}
	if err := owner.Respond(context.Background(), response); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(desktopClient.approvals, []desktopApprovalCall{{threadID: "thread-1", requestID: "desktop-approval-1", decision: "decline"}}) || len(appClient.command) != 0 {
		t.Fatalf("desktop approvals = %#v, app approvals = %#v", desktopClient.approvals, appClient.command)
	}
}

type desktopApprovalCall struct{ threadID, requestID, decision string }
type fakeDesktopResponder struct{ approvals []desktopApprovalCall }

func (fake *fakeDesktopResponder) RouteApprovalDecision(_ context.Context, threadID, requestID, decision string) error {
	fake.approvals = append(fake.approvals, desktopApprovalCall{threadID, requestID, decision})
	return nil
}
func (*fakeDesktopResponder) RouteFileApprovalDecision(context.Context, string, string, string) error {
	return nil
}
func (*fakeDesktopResponder) RespondPermissionRequest(context.Context, string, string, desktopipc.PermissionResponse) error {
	return nil
}
func (*fakeDesktopResponder) SubmitUserInput(context.Context, string, string, desktopipc.UserInputResponse) error {
	return nil
}
func (*fakeDesktopResponder) SubmitMCP(context.Context, string, string, desktopipc.MCPResponse) error {
	return nil
}

type fakeAppServerResponder struct {
	command     []approvalCall
	file        []approvalCall
	permissions []permissionCall
	questions   []questionCall
	mcp         []mcpCall
	err         error
}

type approvalCall struct {
	threadID string
	id       json.RawMessage
	decision appserver.ApprovalDecision
}
type permissionCall struct {
	threadID string
	id       json.RawMessage
	granted  json.RawMessage
	scope    string
}
type questionCall struct {
	threadID string
	id       json.RawMessage
	answers  map[string][]string
}
type mcpCall struct {
	threadID string
	id       json.RawMessage
	action   string
	content  any
}

func (fake *fakeAppServerResponder) RespondCommandApproval(_ context.Context, threadID string, id json.RawMessage, decision appserver.ApprovalDecision) error {
	fake.command = append(fake.command, approvalCall{threadID, append(json.RawMessage(nil), id...), decision})
	return fake.err
}
func (fake *fakeAppServerResponder) RespondFileApproval(_ context.Context, threadID string, id json.RawMessage, decision appserver.ApprovalDecision) error {
	fake.file = append(fake.file, approvalCall{threadID, append(json.RawMessage(nil), id...), decision})
	return fake.err
}
func (fake *fakeAppServerResponder) RespondPermissions(_ context.Context, threadID string, id json.RawMessage, granted json.RawMessage, scope string) error {
	fake.permissions = append(fake.permissions, permissionCall{threadID, append(json.RawMessage(nil), id...), append(json.RawMessage(nil), granted...), scope})
	return fake.err
}
func (fake *fakeAppServerResponder) RespondUserInput(_ context.Context, threadID string, id json.RawMessage, answers map[string][]string) error {
	fake.questions = append(fake.questions, questionCall{threadID, append(json.RawMessage(nil), id...), cloneAnswers(answers)})
	return fake.err
}
func (fake *fakeAppServerResponder) RespondMCP(_ context.Context, threadID string, id json.RawMessage, action string, content any) error {
	fake.mcp = append(fake.mcp, mcpCall{threadID, append(json.RawMessage(nil), id...), action, content})
	return fake.err
}
