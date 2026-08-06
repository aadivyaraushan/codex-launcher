// Fact-force:
// 1) Importers/callers: go test ./companion/internal/app/mobilesession -run Router;
//    production = handler.go capability_request Prepare error mapping.
// 2) Affected API: action_result JSON {state:"cancelled", question:"…"} for
//    ErrRouterNotProvisioned / ErrRouterUnreachable (not code internal).
// 3) No data schemas/files.
// 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only
//    (survive timeouts by shipping a durable checkpoint)."
package mobilesession

import (
	"bytes"
	"context"
	"testing"

	capabilityflow "github.com/codex-launcher/codex-launcher/companion/internal/capability/flow"
	stage1openai "github.com/codex-launcher/codex-launcher/companion/internal/capability/routing/stage1/openai"
)

type routerNotProvisionedFlow struct{ recordingCapabilityFlow }

func (f *routerNotProvisionedFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, stage1openai.ErrRouterNotProvisioned
}

type routerUnreachableFlow struct{ recordingCapabilityFlow }

func (f *routerUnreachableFlow) Prepare(_ context.Context, _, _, _ string) (capabilityflow.Preview, error) {
	return capabilityflow.Preview{}, stage1openai.ErrRouterUnreachable
}

func TestRouterNotProvisionedReachesPhoneAsSetupQuestion(t *testing.T) {
	result := requestAndReadResult(t, &routerNotProvisionedFlow{})

	if bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("no-key router reported as failure: %s", result.Body)
	}
	if bytes.Contains(result.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("no-key router collapsed to internal: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"state":"cancelled"`)) {
		t.Fatalf("expected cancelled, got: %s", result.Body)
	}
	want := "Operator's router isn't set up on this phone yet. Ask the operator to finish setup."
	if !bytes.Contains(result.Body, []byte(`"question":"`+want+`"`)) {
		t.Fatalf("setup sentence missing: %s", result.Body)
	}
}

func TestRouterUnreachableReachesPhoneAsRetryQuestion(t *testing.T) {
	result := requestAndReadResult(t, &routerUnreachableFlow{})

	if bytes.Contains(result.Body, []byte(`"code":"internal"`)) {
		t.Fatalf("unreachable router collapsed to internal: %s", result.Body)
	}
	if !bytes.Contains(result.Body, []byte(`"state":"cancelled"`)) {
		t.Fatalf("expected cancelled, got: %s", result.Body)
	}
	want := "I couldn't reach the router just now. Try again in a moment."
	if !bytes.Contains(result.Body, []byte(`"question":"`+want+`"`)) {
		t.Fatalf("unreachable sentence missing: %s", result.Body)
	}
}
