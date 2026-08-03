package adapter

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// The marker an adapter uses to say "I sent it and never learned what
// happened", as opposed to "it did not happen".
//
// It mirrors appserver.OutcomeUnknownError (internal/codex/appserver/client.go
// :131-139), which is the shape this project already uses for exactly this
// distinction on the action path. Keeping the two the same shape means there
// is one idea here, not two.
//
// The bar for using it is deliberately high: the request demonstrably left the
// machine, the reply demonstrably went missing, and the thing being asked for
// changes something in the world. Anything short of all three is a failure and
// should be reported as one.

func TestAnUnknownOutcomeSurvivesBeingWrapped(t *testing.T) {
	// Errors get wrapped on the way up — the runner, the flow service and the
	// handler each add context. If the marker only works when it is the
	// outermost error, it will quietly stop working the first time someone
	// adds a %w. errors.As is what makes it survive.
	original := &OutcomeUnknownError{AdapterID: "slack", Verb: "send", Cause: context.DeadlineExceeded}
	wrapped := fmt.Errorf("executing capability: %w", fmt.Errorf("adapter slack: %w", original))

	var found *OutcomeUnknownError
	if !errors.As(wrapped, &found) {
		t.Fatal("an unknown outcome stopped being recognisable once it was wrapped")
	}
	if found.AdapterID != "slack" || found.Verb != "send" {
		t.Fatalf("the marker lost what it was about: %+v", found)
	}
}

func TestAnUnknownOutcomeKeepsItsCauseReachable(t *testing.T) {
	// Whoever logs this needs the underlying reason, and callers already
	// written against context.DeadlineExceeded must keep matching.
	err := &OutcomeUnknownError{AdapterID: "todoist", Verb: "write", Cause: context.DeadlineExceeded}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("the cause is no longer reachable through the marker")
	}
}

func TestAnUnknownOutcomeSaysWhatItIsInPlainWords(t *testing.T) {
	// This string reaches logs, and someone reading a log at 2am should not
	// have to look up what it means.
	err := &OutcomeUnknownError{AdapterID: "applereminders", Verb: "write", Cause: context.Canceled}

	message := err.Error()
	if message == "" {
		t.Fatal("an error with no message tells no one anything")
	}
	if !containsAll(message, "applereminders", "unknown") {
		t.Fatalf("the message should name the adapter and say the outcome is unknown, got %q", message)
	}
}

// An ordinary error must not be mistaken for an unknown one. This is the guard
// that stops the marker spreading to everything.
func TestAnOrdinaryErrorIsNotAnUnknownOutcome(t *testing.T) {
	var found *OutcomeUnknownError
	if errors.As(errors.New("adapter not connected"), &found) {
		t.Fatal("a plain error was treated as an unknown outcome")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !contains(s, part) {
			return false
		}
	}
	return true
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
