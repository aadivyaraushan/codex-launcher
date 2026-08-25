package mobilesession

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
)

// Workstream A step 6. The phone's stage-1 router now fails closed with two
// typed errors (openai/client.go): ErrRouterNotProvisioned when the broker
// holds no key, ErrRouterUnreachable when the broker can't be reached. Today
// both flow up as a bare Prepare error and collapse to code "internal", which
// CapabilityInteraction.kt paints as "the router returned an unexpected
// result" under an "App action failed" dialog — blaming the router for a setup
// gap. These map onto the same cancelled+sentence mechanism the disambiguation
// question already uses, so the user reads a true explanation. Only these two
// sentinels map; every other error stays a generic failure.
//
// The expected sentences are the production consts (routerNotProvisionedSentence,
// routerUnreachableSentence) defined in handler.go, referenced here so the test
// and the wire text can never drift.

type routerNotProvisionedFlow struct{ recordingCapabilityFlow }

func (f *routerNotProvisionedFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, fmt.Errorf("stage1: %w", stage1openai.ErrRouterNotProvisioned)
}

type routerUnreachableFlow struct{ recordingCapabilityFlow }

func (f *routerUnreachableFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, fmt.Errorf("stage1: %w", stage1openai.ErrRouterUnreachable)
}

func TestUnprovisionedRouterTellsTheUserToFinishSetup(t *testing.T) {
	result := requestAndReadResult(t, &routerNotProvisionedFlow{})

	if !bytes.Contains(result.Body, []byte(`"question":"`+routerNotProvisionedSentence+`"`)) {
		t.Fatalf("the setup sentence was dropped on the way to the phone: %s", result.Body)
	}
}

func TestUnreachableRouterTellsTheUserToRetry(t *testing.T) {
	result := requestAndReadResult(t, &routerUnreachableFlow{})

	if !bytes.Contains(result.Body, []byte(`"question":"`+routerUnreachableSentence+`"`)) {
		t.Fatalf("the retry sentence was dropped on the way to the phone: %s", result.Body)
	}
}

// A router failure stops without acting and we know why, so it takes the same
// honest shape as the question: state "cancelled", no error object the phone
// would reject. It must never surface as "failed"/"internal" (the malfunction
// dialog) nor "invalid_action" (the request was fine).
func TestRouterFailureUsesTheCancelledShapeNotAMalfunction(t *testing.T) {
	result := requestAndReadResult(t, &routerNotProvisionedFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"cancelled"`)) {
		t.Fatalf("expected state cancelled, got: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"error"`)) {
		t.Fatalf("a cancelled result carried an error object the phone will reject: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("a setup gap was blamed on the router as an internal failure: %s", result.Body)
	}
}

// The guard: only the two router sentinels map. An ordinary Prepare error must
// still report as a genuine failure so real breakage is never silenced.
func TestANonRouterFailureIsStillAGenuineFailure(t *testing.T) {
	result := requestAndReadResult(t, &prepareFailureFlow{})

	if !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a genuine failure must still be reported as failed: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"question"`)) {
		t.Fatalf("a non-router failure grew a router sentence: %s", result.Body)
	}
}
