package decisions

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRouterKeepsArrivalOrderAndRoutesOneExactOfferedDecision(t *testing.T) {
	responder := &recordingResponder{}
	router := NewRouter(responder, nil)
	now := time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC)
	first := Request{ID: "request-1", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Kind: KindCommand, AllowedDecisions: []Decision{DecisionAcceptOnce, DecisionDecline}, ExpiresAt: now.Add(time.Minute)}
	second := Request{ID: "request-2", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-2", Kind: KindFile, AllowedDecisions: []Decision{DecisionDecline}, ExpiresAt: now.Add(time.Minute)}
	if err := router.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(second); err != nil {
		t.Fatal(err)
	}
	if got := router.Pending("thread-1"); !reflect.DeepEqual(got, []Request{first, second}) {
		t.Fatalf("pending = %#v", got)
	}

	if err := router.Respond(context.Background(), Response{RequestID: first.ID, ThreadID: first.ThreadID, TurnID: first.TurnID, ItemID: first.ItemID, Kind: KindFile, Decision: DecisionDecline}, now); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("cross-kind response error = %v", err)
	}
	if err := router.Respond(context.Background(), Response{RequestID: first.ID, ThreadID: first.ThreadID, TurnID: first.TurnID, ItemID: first.ItemID, Kind: first.Kind, Decision: DecisionAcceptSession}, now); !errors.Is(err, ErrDecisionNotOffered) {
		t.Fatalf("unoffered response error = %v", err)
	}
	want := Response{RequestID: first.ID, ThreadID: first.ThreadID, TurnID: first.TurnID, ItemID: first.ItemID, Kind: first.Kind, Decision: DecisionDecline}
	if err := router.Respond(context.Background(), want, now); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(responder.responses, []Response{want}) {
		t.Fatalf("responses = %#v", responder.responses)
	}
	if err := router.Respond(context.Background(), want, now); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("duplicate response error = %v", err)
	}
	if got := router.Pending("thread-1"); !reflect.DeepEqual(got, []Request{second}) {
		t.Fatalf("remaining = %#v", got)
	}
}

func TestRouterRejectsEveryAllowScopeForAnUnclearCommand(t *testing.T) {
	responder := &recordingResponder{}
	router := NewRouter(responder, nil)
	now := time.Now()
	request := Request{ID: "approval", ThreadID: "thread", TurnID: "turn", ItemID: "item", Kind: KindCommand, Command: "sh -c <redacted:secret>", CommandUnderstandable: false, AllowedDecisions: []Decision{DecisionAcceptOnce, DecisionAcceptSession, DecisionDecline}, ExpiresAt: now.Add(time.Minute)}
	if err := router.Add(request); err != nil {
		t.Fatal(err)
	}
	for _, decision := range []Decision{DecisionAcceptOnce, DecisionAcceptSession} {
		response := Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: decision}
		if err := router.Respond(context.Background(), response, now); !errors.Is(err, ErrDecisionNotOffered) {
			t.Fatalf("unsafe %s response error = %v", decision, err)
		}
	}
	if len(responder.responses) != 0 {
		t.Fatalf("unsafe allow reached owner: %#v", responder.responses)
	}
}

func TestRouterDisconnectExpiryAndSecretQuestionNeverGrantOrTransmit(t *testing.T) {
	responder := &recordingResponder{err: ErrOwnerUnavailable}
	router := NewRouter(responder, nil)
	now := time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC)
	approval := Request{ID: "approval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Kind: KindPermissions, AllowedDecisions: []Decision{DecisionAcceptOnce, DecisionDecline}, ExpiresAt: now.Add(time.Minute)}
	question := Request{ID: "question", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-2", Kind: KindQuestion, Questions: []Question{{ID: "secret", Prompt: "Password?", Secret: true}}, ExpiresAt: now.Add(time.Minute)}
	if err := router.Add(approval); err != nil {
		t.Fatal(err)
	}
	if err := router.Add(question); err != nil {
		t.Fatal(err)
	}
	response := Response{RequestID: approval.ID, ThreadID: approval.ThreadID, TurnID: approval.TurnID, ItemID: approval.ItemID, Kind: approval.Kind, Decision: DecisionAcceptOnce}
	if err := router.Respond(context.Background(), response, now); !errors.Is(err, ErrOwnerUnavailable) {
		t.Fatalf("offline response error = %v", err)
	}
	if len(router.Pending("thread-1")) != 2 {
		t.Fatal("offline response removed pending request")
	}
	if err := router.Respond(context.Background(), Response{RequestID: question.ID, ThreadID: question.ThreadID, TurnID: question.TurnID, ItemID: question.ItemID, Kind: question.Kind, Answers: map[string][]string{"secret": {"do not send"}}}, now); !errors.Is(err, ErrSecretAnswer) {
		t.Fatalf("secret answer error = %v", err)
	}
	if len(responder.responses) != 1 {
		t.Fatalf("secret answer reached responder: %#v", responder.responses)
	}
	router.Expire(now.Add(2 * time.Minute))
	if len(router.Pending("thread-1")) != 0 || len(responder.responses) != 1 {
		t.Fatalf("expiry sent a response or retained request: %#v", responder.responses)
	}
}

func TestRouterRemovesRequestWhenResponseOutcomeIsUnknown(t *testing.T) {
	responder := &recordingResponder{err: errors.New("connection lost after write")}
	router := NewRouter(responder, nil)
	now := time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC)
	request := Request{ID: "approval", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Kind: KindCommand, AllowedDecisions: []Decision{DecisionDecline}, ExpiresAt: now.Add(time.Minute)}
	if err := router.Add(request); err != nil {
		t.Fatal(err)
	}
	response := Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: DecisionDecline}
	err := router.Respond(context.Background(), response, now)
	if !errors.Is(err, ErrResponseOutcomeUnknown) {
		t.Fatalf("response error = %v", err)
	}
	if pending := router.Pending(request.ThreadID); len(pending) != 0 {
		t.Fatalf("outcome-unknown response remained retryable: %#v", pending)
	}
}

func TestRouterConcurrentDuplicateResponseCallsOwnerOnce(t *testing.T) {
	responder := &recordingResponder{}
	router := NewRouter(responder, nil)
	now := time.Now()
	request := Request{ID: "approval", ThreadID: "thread", TurnID: "turn", ItemID: "item", Kind: KindCommand, AllowedDecisions: []Decision{DecisionDecline}, ExpiresAt: now.Add(time.Minute)}
	if err := router.Add(request); err != nil {
		t.Fatal(err)
	}
	response := Response{RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: DecisionDecline}
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() { defer wait.Done(); _ = router.Respond(context.Background(), response, now) }()
	}
	wait.Wait()
	if len(responder.responses) != 1 {
		t.Fatalf("owner call count = %d", len(responder.responses))
	}
}

func TestCommandRedactionPreservesOrderAndDisablesUnclearCommands(t *testing.T) {
	redacted, understandable := RedactCommand(`env API_TOKEN=abc npm deploy --password hunter2 --branch main`)
	if redacted != `env API_TOKEN=<redacted:secret> npm deploy --password <redacted:secret> --branch main` || !understandable {
		t.Fatalf("redacted = %q, understandable = %v", redacted, understandable)
	}
	redacted, understandable = RedactCommand(`sh -c "$DEPLOY_SECRET"`)
	if redacted != `sh -c <redacted:secret>` || understandable {
		t.Fatalf("unclear redaction = %q, %v", redacted, understandable)
	}
	if redacted, understandable = RedactCommand("echo ok\nrm -rf /tmp/x"); redacted != "" || understandable {
		t.Fatalf("control command = %q, %v", redacted, understandable)
	}
	redacted, understandable = RedactCommand(`curl --api-key=abc --url example.com`)
	if redacted != `curl --api-key=<redacted:secret> --url example.com` || !understandable {
		t.Fatalf("inline flag redaction = %q, %v", redacted, understandable)
	}
}

type recordingResponder struct {
	mu        sync.Mutex
	responses []Response
	err       error
}

func (responder *recordingResponder) Respond(_ context.Context, response Response) error {
	responder.mu.Lock()
	defer responder.mu.Unlock()
	responder.responses = append(responder.responses, response)
	return responder.err
}

func (responder *recordingResponder) last() Response {
	responder.mu.Lock()
	defer responder.mu.Unlock()
	if len(responder.responses) == 0 {
		return Response{}
	}
	return responder.responses[len(responder.responses)-1]
}

// Answers must survive the router's defensive copy on the way to the request
// owner. They were previously dropped, which silently emptied every answer the
// owner received for a question.
func TestRespondDeliversAnswersToTheOwner(t *testing.T) {
	responder := &recordingResponder{}
	router := NewRouter(responder, nil)
	expires := time.Now().Add(time.Minute)
	request := Request{
		ID: "req-1", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Kind: KindQuestion,
		ComputerName: "mac", ProjectLabel: "launcher", ExpiresAt: expires,
		Questions: []Question{{ID: "q0", Header: "Database", Prompt: "Which database?", Options: []string{"Postgres", "SQLite"}}},
	}
	if err := router.Add(request); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	response := Response{
		RequestID: "req-1", ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", Kind: KindQuestion,
		Answers: map[string][]string{"q0": {"Postgres"}},
	}
	if err := router.Respond(context.Background(), response, time.Now()); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	delivered := responder.last()
	if len(delivered.Answers) != 1 {
		t.Fatalf("owner received %d answers; want 1", len(delivered.Answers))
	}
	if got := delivered.Answers["q0"]; len(got) != 1 || got[0] != "Postgres" {
		t.Fatalf("owner received answers = %#v", delivered.Answers)
	}
	// Still a copy: mutating the caller's slice must not reach the owner.
	response.Answers["q0"][0] = "mutated"
	if delivered.Answers["q0"][0] != "Postgres" {
		t.Fatal("router handed the owner the caller's own slice")
	}
}
