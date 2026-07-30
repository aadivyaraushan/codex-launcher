package decisions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidRequest         = errors.New("decision request is invalid")
	ErrRequestMismatch        = errors.New("decision response does not match a pending request")
	ErrDecisionNotOffered     = errors.New("decision was not offered by Codex")
	ErrSecretAnswer           = errors.New("secret questions must be answered on the computer")
	ErrOwnerUnavailable       = errors.New("Codex request owner is unavailable")
	ErrResponseOutcomeUnknown = errors.New("decision response outcome is unknown")
	ErrResponseInFlight       = errors.New("decision response is already in flight")
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Kind string

const (
	KindCommand     Kind = "command"
	KindFile        Kind = "file"
	KindPermissions Kind = "permissions"
	KindQuestion    Kind = "question"
	KindMCP         Kind = "mcp_elicitation"
)

type Decision string

const (
	DecisionAcceptOnce    Decision = "accept"
	DecisionAcceptSession Decision = "accept_for_session"
	DecisionDecline       Decision = "decline"
	DecisionCancel        Decision = "cancel"
)

type Question struct {
	ID      string
	Header  string
	Prompt  string
	Options []string
	Secret  bool
}

type Request struct {
	ID                    string
	ThreadID              string
	TurnID                string
	ItemID                string
	Kind                  Kind
	ComputerName          string
	ProjectLabel          string
	WorkingDirectory      string
	Reason                string
	Access                string
	Command               string
	CommandUnderstandable bool
	AffectedPaths         []string
	AllowedDecisions      []Decision
	Questions             []Question
	ExpiresAt             time.Time
}

type Response struct {
	RequestID string
	ThreadID  string
	TurnID    string
	ItemID    string
	Kind      Kind
	Decision  Decision
	Answers   map[string][]string
}

type Responder interface {
	Respond(context.Context, Response) error
}

type pendingRequest struct {
	request  Request
	inFlight bool
}

type Router struct {
	mu        sync.Mutex
	responder Responder
	logger    *slog.Logger
	pending   map[string]*pendingRequest
	order     []string
}

func NewRouter(responder Responder, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{responder: responder, logger: logger, pending: make(map[string]*pendingRequest)}
}

func (router *Router) Add(request Request) error {
	if router == nil || !validRequest(request) {
		return ErrInvalidRequest
	}
	request = cloneRequest(request)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.pending[request.ID]; exists {
		return ErrInvalidRequest
	}
	router.pending[request.ID] = &pendingRequest{request: request}
	router.order = append(router.order, request.ID)
	router.logger.Info("[decisions] request registered", "request_id", request.ID, "thread_id", request.ThreadID, "request_kind", request.Kind, "decision", "wait_for_exact_response")
	return nil
}

func (router *Router) Pending(threadID string) []Request {
	if router == nil || !validID(threadID) {
		return nil
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	result := make([]Request, 0)
	for _, id := range router.order {
		pending := router.pending[id]
		if pending != nil && pending.request.ThreadID == threadID {
			result = append(result, cloneRequest(pending.request))
		}
	}
	return result
}

func (router *Router) Respond(ctx context.Context, response Response, now time.Time) error {
	if router == nil || router.responder == nil || ctx == nil || now.IsZero() || !validResponseShape(response) {
		return ErrRequestMismatch
	}
	router.mu.Lock()
	pending := router.pending[response.RequestID]
	if pending == nil || !matches(pending.request, response) || !now.Before(pending.request.ExpiresAt) {
		router.mu.Unlock()
		return ErrRequestMismatch
	}
	if pending.inFlight {
		router.mu.Unlock()
		return ErrResponseInFlight
	}
	if err := authorizeResponse(pending.request, response); err != nil {
		router.mu.Unlock()
		return err
	}
	pending.inFlight = true
	router.mu.Unlock()

	err := router.responder.Respond(ctx, cloneResponse(response))
	router.mu.Lock()
	defer router.mu.Unlock()
	current := router.pending[response.RequestID]
	if current == nil || current != pending {
		return ErrRequestMismatch
	}
	if err != nil {
		if errors.Is(err, ErrOwnerUnavailable) {
			pending.inFlight = false
			router.logger.Error("[decisions] response delivery failed", "request_id", response.RequestID, "thread_id", response.ThreadID, "request_kind", response.Kind, "error_class", errorClass(err), "decision", "retain_pending")
			return err
		}
		delete(router.pending, response.RequestID)
		router.removeOrderLocked(response.RequestID)
		router.logger.Error("[decisions] response outcome unknown", "request_id", response.RequestID, "thread_id", response.ThreadID, "request_kind", response.Kind, "error_class", errorClass(err), "decision", "remove_to_prevent_retry")
		return errors.Join(ErrResponseOutcomeUnknown, err)
	}
	delete(router.pending, response.RequestID)
	router.removeOrderLocked(response.RequestID)
	router.logger.Info("[decisions] response delivered", "request_id", response.RequestID, "thread_id", response.ThreadID, "request_kind", response.Kind, "decision", "remove_exact_request")
	return nil
}

func (router *Router) Expire(now time.Time) {
	if router == nil || now.IsZero() {
		return
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, id := range append([]string(nil), router.order...) {
		pending := router.pending[id]
		if pending != nil && !pending.inFlight && !now.Before(pending.request.ExpiresAt) {
			delete(router.pending, id)
			router.removeOrderLocked(id)
			router.logger.Info("[decisions] request expired", "request_id", id, "thread_id", pending.request.ThreadID, "request_kind", pending.request.Kind, "decision", "remove_without_response")
		}
	}
}

func (router *Router) removeOrderLocked(id string) {
	index := slices.Index(router.order, id)
	if index >= 0 {
		router.order = append(router.order[:index], router.order[index+1:]...)
	}
}

func validRequest(request Request) bool {
	if !validID(request.ID) || !validID(request.ThreadID) || !validID(request.TurnID) || !validID(request.ItemID) || request.ExpiresAt.IsZero() || !validKind(request.Kind) {
		return false
	}
	if request.Kind == KindQuestion {
		if len(request.Questions) == 0 || len(request.Questions) > 32 || len(request.AllowedDecisions) != 0 {
			return false
		}
		seen := make(map[string]bool)
		for _, question := range request.Questions {
			if !validID(question.ID) || !bounded(question.Prompt, 4096) || len(question.Options) > 32 || seen[question.ID] {
				return false
			}
			seen[question.ID] = true
		}
		return true
	}
	if len(request.Questions) != 0 || len(request.AllowedDecisions) == 0 || len(request.AllowedDecisions) > 4 {
		return false
	}
	seen := make(map[Decision]bool)
	for _, decision := range request.AllowedDecisions {
		if !validDecision(decision) || seen[decision] {
			return false
		}
		seen[decision] = true
	}
	return true
}

func validResponseShape(response Response) bool {
	return validID(response.RequestID) && validID(response.ThreadID) && validID(response.TurnID) && validID(response.ItemID) && validKind(response.Kind)
}

func matches(request Request, response Response) bool {
	return request.ID == response.RequestID && request.ThreadID == response.ThreadID && request.TurnID == response.TurnID && request.ItemID == response.ItemID && request.Kind == response.Kind
}

func authorizeResponse(request Request, response Response) error {
	if request.Kind != KindQuestion {
		if len(response.Answers) != 0 || !slices.Contains(request.AllowedDecisions, response.Decision) {
			return ErrDecisionNotOffered
		}
		if request.Kind == KindCommand && !request.CommandUnderstandable && (response.Decision == DecisionAcceptOnce || response.Decision == DecisionAcceptSession) {
			return ErrDecisionNotOffered
		}
		return nil
	}
	if response.Decision != "" || len(response.Answers) == 0 || len(response.Answers) != len(request.Questions) {
		return ErrRequestMismatch
	}
	questions := make(map[string]Question, len(request.Questions))
	for _, question := range request.Questions {
		questions[question.ID] = question
	}
	total := 0
	for id, answers := range response.Answers {
		question, exists := questions[id]
		if !exists || len(answers) == 0 || len(answers) > 32 {
			return ErrRequestMismatch
		}
		if question.Secret {
			return ErrSecretAnswer
		}
		for _, answer := range answers {
			total += len(answer)
			if !bounded(answer, 131072) || total > 131072 {
				return ErrRequestMismatch
			}
		}
	}
	return nil
}

func validID(value string) bool { return idPattern.MatchString(value) }
func validKind(kind Kind) bool {
	return kind == KindCommand || kind == KindFile || kind == KindPermissions || kind == KindQuestion || kind == KindMCP
}
func validDecision(decision Decision) bool {
	return decision == DecisionAcceptOnce || decision == DecisionAcceptSession || decision == DecisionDecline || decision == DecisionCancel
}
func bounded(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func cloneRequest(request Request) Request {
	request.AffectedPaths = append([]string(nil), request.AffectedPaths...)
	request.AllowedDecisions = append([]Decision(nil), request.AllowedDecisions...)
	request.Questions = append([]Question(nil), request.Questions...)
	for index := range request.Questions {
		request.Questions[index].Options = append([]string(nil), request.Questions[index].Options...)
	}
	return request
}

func cloneResponse(response Response) Response {
	if response.Answers == nil {
		return response
	}
	// Build into a separate map. Assigning the new map to response.Answers
	// first and then ranging over it copied nothing, so every answer was
	// silently dropped on the way to the request owner.
	response.Answers = cloneAnswers(response.Answers)
	return response
}

func errorClass(err error) string {
	if err == nil {
		return "none"
	}
	return regexp.MustCompile(`[^A-Za-z0-9_.-]`).ReplaceAllString(fmt.Sprintf("%T", err), "_")
}
