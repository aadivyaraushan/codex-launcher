package mobilesession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

// The same bug the capability path just had, on the path people actually use.
//
// `capability_failure_codes_test.go` fixed the app-action side: every failure
// there was reported as "invalid_action" regardless of cause, so a person who
// simply had not connected an app was told their request was invalid. The
// ordinary task actions — start a task, rename one, answer an approval,
// dismiss a control — still do exactly that. `setActionFailure(result,
// "invalid_action", false)` appears at handler.go:748, :767, :775, :779, :784,
// :800, :807 and :818.
//
// Three of those are a build of the desktop app that cannot do the thing at
// all: no task-management source, no approval router, no prompt queue. The
// user's request was fine; this copy of the companion has nothing to serve it
// with. Telling them "invalid action" sends them off to fix a request that was
// never the problem, and hides the one fact that would explain it — their
// computer needs updating.
//
// One is the whole new-task path. `startNewTask` collapses thirteen different
// endings into a single `newTaskFailed`, and the caller then calls all thirteen
// "invalid_action". Some of them genuinely are the user's request (a model the
// phone no longer offers, a project that does not exist). Most are ours (the
// durable queue would not open, the option catalog would not load, the app
// server refused the start). Starting a task is the most-used thing in the
// product, so this is where the wrong word gets seen most.
//
// The fix is the same as before and it needs no wire change: choose correctly
// among the eleven codes the phone's decoder already accepts
// (ProtocolCodec.kt:618). "desktop_incompatible" for a build that cannot do it,
// "internal" for a break we cannot explain, "invalid_action" only where the
// request really was the problem.
//
// The controls below matter as much as the rest. "Never say invalid_action"
// passes every other test in this file and ships something worse than the bug.

// A build with no task-management source cannot rename, archive or fork
// anything. That is a fact about the computer, not about what was asked.
func TestABuildThatCannotManageTasksSaysSoInsteadOfBlamingTheRequest(t *testing.T) {
	result := failedActionResult(t, newTestHandlerAction(t,
		`{"version":{"major":1,"minor":0},"messageId":"rename","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"rename_task","taskId":"thread-1","title":"Renamed"}}`))

	if code := errorCodeOf(t, result); code != "desktop_incompatible" {
		t.Fatalf("expected desktop_incompatible, got %q: %s", code, result.Body)
	}
}

// Same fact, different door: no approval router means no approvals, ever.
func TestABuildThatCannotRouteApprovalsSaysSoInsteadOfBlamingTheRequest(t *testing.T) {
	result := failedActionResult(t, newTestHandlerAction(t,
		`{"version":{"major":1,"minor":0},"messageId":"approve","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"approval","taskId":"thread-1","requestId":"approval-1","requestKind":"command","decision":"accept"}}`))

	if code := errorCodeOf(t, result); code != "desktop_incompatible" {
		t.Fatalf("expected desktop_incompatible, got %q: %s", code, result.Body)
	}
}

// And a third: no prompt queue means nothing to dismiss.
func TestABuildWithNoPromptQueueSaysSoInsteadOfBlamingTheRequest(t *testing.T) {
	result := failedActionResult(t, newTestHandlerAction(t,
		`{"version":{"major":1,"minor":0},"messageId":"dismiss","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"dismiss_unknown_control","targetActionId":"unknown-1"}}`))

	if code := errorCodeOf(t, result); code != "desktop_incompatible" {
		t.Fatalf("expected desktop_incompatible, got %q: %s", code, result.Body)
	}
}

// The most-used action in the product. A companion with no way to start a task
// is not a person typing something wrong.
func TestABuildThatCannotStartTasksSaysSoInsteadOfBlamingTheRequest(t *testing.T) {
	result := failedActionResult(t, newTestHandlerAction(t,
		`{"version":{"major":1,"minor":0},"messageId":"start","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`))

	if code := errorCodeOf(t, result); code != "desktop_incompatible" {
		t.Fatalf("expected desktop_incompatible, got %q: %s", code, result.Body)
	}
}

// The app server was there and refused. We cannot say why, so we say that,
// rather than pinning it on the request.
func TestATaskTheComputerRefusedToStartIsNotCalledAnInvalidRequest(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog(), startErr: errors.New("app server closed the connection")}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	greet(t, handler, sender)
	sender.sent = make(chan contract.Message, 2)

	if err := handler.Handle(context.Background(), sender, decode(t,
		`{"version":{"major":1,"minor":0},"messageId":"start","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`)); err != nil {
		t.Fatal(err)
	}

	result := awaitSentMessage(t, sender.sent)
	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("expected a failure at all: %s", result.Body)
	}
	if code := errorCodeOf(t, result); code != "internal" {
		t.Fatalf("expected internal, got %q: %s", code, result.Body)
	}
}

// First control. A model the phone offered but the computer no longer has
// really is a request that cannot be served as asked, and "invalid_action" is
// the truth. Without this test, answering "desktop_incompatible" to everything
// passes all four tests above.
func TestAModelTheComputerNoLongerOffersIsStillAnInvalidRequest(t *testing.T) {
	source := &newTaskSource{catalog: testTaskOptionsCatalog()}
	handler, sender := newTestHandlerWithTaskQueue(t, source, promptqueue.NewMemoryStore())
	greet(t, handler, sender)
	sender.sent = make(chan contract.Message, 2)

	if err := handler.Handle(context.Background(), sender, decode(t,
		`{"version":{"major":1,"minor":0},"messageId":"start","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"model-that-went-away","reasoningId":"high","permissionModeId":"workspace-write"}}`)); err != nil {
		t.Fatal(err)
	}

	result := awaitSentMessage(t, sender.sent)
	if code := errorCodeOf(t, result); code != "invalid_action" {
		t.Fatalf("a model that does not exist really is an invalid request, got %q: %s", code, result.Body)
	}
}

// Second control, on a different action so the first one cannot be satisfied by
// a special case. Naming a project this computer does not have is the user's
// request being wrong, and must stay "invalid_action".
func TestAProjectThisComputerDoesNotHaveIsStillAnInvalidRequest(t *testing.T) {
	result := failedActionResult(t, newTestHandlerAction(t,
		`{"version":{"major":1,"minor":0},"messageId":"project","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"set_project","projectId":"not-a-project"}}`))

	if code := errorCodeOf(t, result); code != "invalid_action" {
		t.Fatalf("an unknown project really is an invalid request, got %q: %s", code, result.Body)
	}
}

// Third control, and the one that stops this quietly breaking later. The phone
// throws away any envelope carrying a code outside its fixed set, so a
// well-meant new word leaves the user with no message at all — worse than the
// wrong word.
func TestEveryTaskActionFailureCodeIsOneThePhoneAccepts(t *testing.T) {
	accepted := map[string]bool{
		"computer_offline": true, "connection_lost": true, "desktop_incompatible": true,
		"owner_unavailable": true, "invalid_action": true, "outcome_unknown": true,
		"sequence_gap": true, "unauthorized": true, "quota_exceeded": true,
		"attachment_invalid": true, "internal": true,
	}
	actions := map[string]string{
		"rename with no manager":   `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"rename_task","taskId":"thread-1","title":"Renamed"}}`,
		"archive with no manager":  `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"archive_task","taskId":"thread-1"}}`,
		"fork with no manager":     `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"fork_task","taskId":"thread-1"}}`,
		"approval with no router":  `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"approval","taskId":"thread-1","requestId":"approval-1","requestKind":"command","decision":"accept"}}`,
		"question with no router":  `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"question_response","taskId":"thread-1","requestId":"question-1","answers":{"question-1":["yes"]}}}`,
		"dismiss with no queue":    `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"dismiss_unknown_control","targetActionId":"unknown-1"}}`,
		"new task with no sources": `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"main","text":"Fix it","modelId":"public-model","reasoningId":"high","permissionModeId":"workspace-write"}}`,
		"unknown project":          `{"version":{"major":1,"minor":0},"messageId":"m","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"set_project","projectId":"not-a-project"}}`,
	}

	for name, frame := range actions {
		result := failedActionResult(t, newTestHandlerAction(t, frame))
		code := errorCodeOf(t, result)
		if code == "" {
			t.Fatalf("%s: failure carried no error code at all: %s", name, result.Body)
		}
		if !accepted[code] {
			t.Fatalf("%s: code %q is not one the phone accepts, so it would drop the whole message", name, code)
		}
	}
}

// --- harness ---

// newTestHandlerAction runs one action against a handler with nothing wired in
// — no task-management source, no approval router, no prompt queue — which is
// exactly the "this build cannot do that" situation under test.
func newTestHandlerAction(t *testing.T, frame string) contract.Message {
	t.Helper()
	handler, sender := newTestHandler(t)
	greet(t, handler, sender)
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, frame)); err != nil {
		t.Fatal(err)
	}
	return awaitSentMessage(t, sender.sent)
}

func failedActionResult(t *testing.T, result contract.Message) contract.Message {
	t.Helper()
	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("expected a failed action result so the error object is allowed at all: %s", result.Body)
	}
	return result
}

// errorCodeOf pulls the error code out of an action_result body, or returns ""
// if there is no error object.
func errorCodeOf(t *testing.T, result contract.Message) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.Body, &decoded); err != nil {
		t.Fatalf("could not read the result body: %v (%s)", err, result.Body)
	}
	return decoded.Error.Code
}

func greet(t *testing.T, handler *Handler, sender *recordingSender) {
	t.Helper()
	if err := handler.Handle(context.Background(), sender, decode(t,
		`{"version":{"major":1,"minor":0},"messageId":"hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil
}
