package decisions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
)

type DisplayContext struct {
	ComputerName string
	ProjectLabel string
}

type appServerResponder interface {
	RespondCommandApproval(context.Context, string, json.RawMessage, appserver.ApprovalDecision) error
	RespondFileApproval(context.Context, string, json.RawMessage, appserver.ApprovalDecision) error
	RespondPermissions(context.Context, string, json.RawMessage, json.RawMessage, string) error
	RespondUserInput(context.Context, string, json.RawMessage, map[string][]string) error
	RespondMCP(context.Context, string, json.RawMessage, string, any) error
}

type desktopResponder interface {
	RouteApprovalDecision(context.Context, string, string, string) error
	RouteFileApprovalDecision(context.Context, string, string, string) error
	RespondPermissionRequest(context.Context, string, string, desktopipc.PermissionResponse) error
	SubmitUserInput(context.Context, string, string, desktopipc.UserInputResponse) error
	SubmitMCP(context.Context, string, string, desktopipc.MCPResponse) error
}

type requestOwner string

const (
	ownerAppServer requestOwner = "app_server"
	ownerDesktop   requestOwner = "desktop"
)

type ownedAppServerRequest struct {
	rawID       json.RawMessage
	kind        Kind
	threadID    string
	permissions json.RawMessage
	owner       requestOwner
	desktopID   string
}

type AppServerOwner struct {
	mu       sync.Mutex
	client   appServerResponder
	desktop  desktopResponder
	requests map[string]ownedAppServerRequest
}

func NewAppServerOwner(client appServerResponder) *AppServerOwner {
	return &AppServerOwner{client: client, requests: make(map[string]ownedAppServerRequest)}
}

func (owner *AppServerOwner) AttachDesktop(client desktopResponder) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	owner.desktop = client
	owner.mu.Unlock()
}

func (owner *AppServerOwner) Register(source appserver.ServerRequest, display DisplayContext, expiresAt time.Time) (Request, error) {
	return owner.register(source, display, expiresAt, ownerAppServer)
}

func (owner *AppServerOwner) RegisterDesktop(source appserver.ServerRequest, display DisplayContext, expiresAt time.Time) (Request, error) {
	return owner.register(source, display, expiresAt, ownerDesktop)
}

func (owner *AppServerOwner) register(source appserver.ServerRequest, display DisplayContext, expiresAt time.Time, requestOwner requestOwner) (Request, error) {
	if owner == nil || owner.client == nil || !bounded(display.ComputerName, 80) || !bounded(display.ProjectLabel, 128) || expiresAt.IsZero() {
		return Request{}, ErrInvalidRequest
	}
	kind := appServerKind(source.Method)
	id, okay := mobileRequestID(requestOwner, source.ThreadID, source.ID)
	if !okay || kind == "" || !validID(source.ThreadID) {
		return Request{}, ErrInvalidRequest
	}
	desktopID := ""
	if requestOwner == ownerDesktop && (json.Unmarshal(source.ID, &desktopID) != nil || !validID(desktopID)) {
		return Request{}, ErrInvalidRequest
	}
	turnID := source.TurnID
	itemID := source.ItemID
	if kind == KindMCP {
		if turnID == "" {
			turnID = "mcp-turn"
		}
		if itemID == "" {
			itemID = id
		}
	}
	if !validID(turnID) || !validID(itemID) {
		return Request{}, ErrInvalidRequest
	}
	request := Request{
		ID: id, ThreadID: source.ThreadID, TurnID: turnID, ItemID: itemID, Kind: kind,
		ComputerName: display.ComputerName, ProjectLabel: display.ProjectLabel, WorkingDirectory: source.CWD,
		Reason: source.Reason, ExpiresAt: expiresAt,
	}
	switch kind {
	case KindCommand:
		request.Command, request.CommandUnderstandable = RedactCommand(source.Command)
		if request.Command == "" {
			request.Command = "<redacted:unavailable>"
			request.CommandUnderstandable = false
		}
		request.AllowedDecisions = mapAppServerDecisions(source.AllowedDecisions)
	case KindFile:
		request.AllowedDecisions = mapAppServerDecisions(source.AllowedDecisions)
		if source.GrantRoot != "" {
			request.AffectedPaths = []string{source.GrantRoot}
		}
	case KindPermissions:
		if len(source.Permissions) == 0 || !json.Valid(source.Permissions) {
			return Request{}, ErrInvalidRequest
		}
		request.Access = "Requested additional computer access"
		request.AllowedDecisions = []Decision{DecisionAcceptOnce, DecisionAcceptSession, DecisionDecline}
	case KindQuestion:
		request.Questions = make([]Question, 0, len(source.Questions))
		for _, question := range source.Questions {
			request.Questions = append(request.Questions, Question{ID: question.ID, Header: question.Header, Prompt: question.Prompt, Options: append([]string(nil), question.Options...), Secret: question.Secret})
		}
	case KindMCP:
		if source.MCPMode != "form" && source.MCPMode != "openai/form" && source.MCPMode != "url" || !bounded(source.MCPMessage, 4096) {
			return Request{}, ErrInvalidRequest
		}
		request.Reason = source.MCPMessage
		request.Access = "MCP server request"
		request.AllowedDecisions = []Decision{DecisionDecline, DecisionCancel}
	}
	if !validRequest(request) {
		return Request{}, ErrInvalidRequest
	}
	owned := ownedAppServerRequest{rawID: append(json.RawMessage(nil), source.ID...), kind: kind, threadID: source.ThreadID, permissions: append(json.RawMessage(nil), source.Permissions...), owner: requestOwner, desktopID: desktopID}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if previous, exists := owner.requests[id]; exists && string(previous.rawID) != string(source.ID) {
		return Request{}, ErrInvalidRequest
	}
	owner.requests[id] = owned
	return request, nil
}

func (owner *AppServerOwner) Forget(requestID string) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	delete(owner.requests, requestID)
	owner.mu.Unlock()
}

func (owner *AppServerOwner) Respond(ctx context.Context, response Response) error {
	if owner == nil || owner.client == nil {
		return ErrOwnerUnavailable
	}
	owner.mu.Lock()
	owned, exists := owner.requests[response.RequestID]
	owner.mu.Unlock()
	if !exists || owned.kind != response.Kind || owned.threadID != response.ThreadID {
		return ErrOwnerUnavailable
	}
	var err error
	if owned.owner == ownerDesktop {
		err = owner.respondDesktop(ctx, owned, response)
	} else {
		err = owner.respondAppServer(ctx, owned, response)
	}
	if errors.Is(err, appserver.ErrClosed) || errors.Is(err, appserver.ErrNotInitialized) || errors.Is(err, desktopipc.ErrDisconnected) || errors.Is(err, desktopipc.ErrOwnerUnavailable) || errors.Is(err, desktopipc.ErrWriteNotSent) {
		return ErrOwnerUnavailable
	}
	owner.mu.Lock()
	delete(owner.requests, response.RequestID)
	owner.mu.Unlock()
	return err
}

func (owner *AppServerOwner) respondAppServer(ctx context.Context, owned ownedAppServerRequest, response Response) error {
	switch owned.kind {
	case KindCommand:
		return owner.client.RespondCommandApproval(ctx, owned.threadID, owned.rawID, appServerDecision(response.Decision))
	case KindFile:
		return owner.client.RespondFileApproval(ctx, owned.threadID, owned.rawID, appServerDecision(response.Decision))
	case KindPermissions:
		granted := json.RawMessage(`{}`)
		scope := "turn"
		if response.Decision == DecisionAcceptOnce || response.Decision == DecisionAcceptSession {
			granted = owned.permissions
		}
		if response.Decision == DecisionAcceptSession {
			scope = "session"
		}
		return owner.client.RespondPermissions(ctx, owned.threadID, owned.rawID, granted, scope)
	case KindQuestion:
		return owner.client.RespondUserInput(ctx, owned.threadID, owned.rawID, response.Answers)
	case KindMCP:
		action := "decline"
		if response.Decision == DecisionCancel {
			action = "cancel"
		}
		return owner.client.RespondMCP(ctx, owned.threadID, owned.rawID, action, nil)
	default:
		return ErrOwnerUnavailable
	}
}

func (owner *AppServerOwner) respondDesktop(ctx context.Context, owned ownedAppServerRequest, response Response) error {
	owner.mu.Lock()
	desktop := owner.desktop
	owner.mu.Unlock()
	if desktop == nil {
		return ErrOwnerUnavailable
	}
	switch owned.kind {
	case KindCommand:
		return desktop.RouteApprovalDecision(ctx, owned.threadID, owned.desktopID, desktopDecision(response.Decision))
	case KindFile:
		return desktop.RouteFileApprovalDecision(ctx, owned.threadID, owned.desktopID, desktopDecision(response.Decision))
	case KindPermissions:
		granted, scope := json.RawMessage(`{}`), "turn"
		if response.Decision == DecisionAcceptOnce || response.Decision == DecisionAcceptSession {
			granted = owned.permissions
		}
		if response.Decision == DecisionAcceptSession {
			scope = "session"
		}
		return desktop.RespondPermissionRequest(ctx, owned.threadID, owned.desktopID, desktopipc.PermissionResponse{Permissions: granted, Scope: scope})
	case KindQuestion:
		answers := make(map[string]desktopipc.UserInputAnswer, len(response.Answers))
		for id, values := range response.Answers {
			answers[id] = desktopipc.UserInputAnswer{Answers: append([]string(nil), values...)}
		}
		return desktop.SubmitUserInput(ctx, owned.threadID, owned.desktopID, desktopipc.UserInputResponse{Answers: answers})
	case KindMCP:
		action := "decline"
		if response.Decision == DecisionCancel {
			action = "cancel"
		}
		return desktop.SubmitMCP(ctx, owned.threadID, owned.desktopID, desktopipc.MCPResponse{Action: action})
	default:
		return ErrOwnerUnavailable
	}
}

func appServerKind(method string) Kind {
	switch method {
	case "item/commandExecution/requestApproval":
		return KindCommand
	case "item/fileChange/requestApproval":
		return KindFile
	case "item/permissions/requestApproval":
		return KindPermissions
	case "item/tool/requestUserInput":
		return KindQuestion
	case "mcpServer/elicitation/request":
		return KindMCP
	default:
		return ""
	}
}

func mapAppServerDecisions(values []appserver.ApprovalDecision) []Decision {
	result := make([]Decision, 0, len(values))
	for _, value := range values {
		switch value {
		case appserver.DecisionAccept:
			result = append(result, DecisionAcceptOnce)
		case appserver.DecisionAcceptForSession:
			result = append(result, DecisionAcceptSession)
		case appserver.DecisionDecline:
			result = append(result, DecisionDecline)
		case appserver.DecisionCancel:
			result = append(result, DecisionCancel)
		}
	}
	return result
}

func appServerDecision(value Decision) appserver.ApprovalDecision {
	switch value {
	case DecisionAcceptOnce:
		return appserver.DecisionAccept
	case DecisionAcceptSession:
		return appserver.DecisionAcceptForSession
	case DecisionDecline:
		return appserver.DecisionDecline
	case DecisionCancel:
		return appserver.DecisionCancel
	default:
		return ""
	}
}

func desktopDecision(value Decision) string { return string(appServerDecision(value)) }

func mobileRequestID(owner requestOwner, threadID string, raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", false
	}
	digest := sha256.Sum256(append(append([]byte(string(owner)+":"+threadID+":"), raw...), '\n'))
	return "rpc-" + hex.EncodeToString(digest[:12]), true
}

func cloneAnswers(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for id, answers := range source {
		result[id] = append([]string(nil), answers...)
	}
	return result
}
