package agentbridge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/devicework"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/execution"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/agentbridge/gates"
)

// Some adapters cannot finish where they started: a notification reply's
// reply box lives inside an Android notification only the phone can reach,
// so Execute refuses with a DeviceWorkError instead of an Outcome. The
// bridge is the agent's only caller, so the bridge must catch that refusal,
// hand the ask to the phone through its DeviceWorker, and wait for the
// answer inside the same tool call. Without a phone, the honest answer is
// a failure that says so.

// handOffAdapter is a fake whose Execute always refuses with device work,
// the way notificationreply and youtube do.
type handOffAdapter struct {
	*fake
	work *adapter.DeviceWorkError
}

func (a *handOffAdapter) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	a.calls = append(a.calls, "execute")
	return adapter.Outcome{}, a.work
}

// fakeDeviceWorker records what the bridge asked and answers with a canned
// result, standing in for the mobilesession handler.
type fakeDeviceWorker struct {
	asks        []devicework.Ask
	hadDeadline bool
	result      devicework.Result
	err         error
}

func (w *fakeDeviceWorker) RunOnDevice(ctx context.Context, ask devicework.Ask) (devicework.Result, error) {
	w.asks = append(w.asks, ask)
	_, w.hadDeadline = ctx.Deadline()
	return w.result, w.err
}

// deviceWorkBridge builds a bridge whose gate store already knows the one
// recipient these tests reply to, with the given worker wired in (nil means
// no phone).
func deviceWorkBridge(t *testing.T, a adapter.Adapter, worker agentbridge.DeviceWorker) *httptest.Server {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}
	store := newUngatedStore()
	store.known[a.Describe().ID+"\x00maya"] = true
	serial := 0
	deps := agentbridge.GateDeps{
		Policy: gates.New(store, func() string {
			serial++
			return "gate-" + string(rune('0'+serial))
		}),
		Store:        store,
		AllowListed:  func(string) bool { return true },
		DeviceWorker: worker,
	}
	b := agentbridge.New(reg, execution.New(reg), testToken, nil, deps)
	server := httptest.NewServer(b.Handler())
	t.Cleanup(server.Close)
	return server
}

func newHandOffAdapter() *handOffAdapter {
	base := newFake("notification_reply", manifest.Send)
	return &handOffAdapter{fake: base, work: &adapter.DeviceWorkError{
		AdapterID: "notification_reply", Kind: "notification_reply",
		Handle: "maya", Text: "on my way", Ceiling: manifest.Completes,
	}}
}

func deviceWorkCall(t *testing.T, server *httptest.Server) (*http.Response, agentbridge.ToolCallResult) {
	t.Helper()
	body := callBody(t, agentbridge.ToolCallRequest{Adapter: "notification_reply", Verb: "send", Handle: "maya", Body: "on my way"})
	resp, raw := do(t, http.MethodPost, server.URL+"/v1/agent-tools/call", testToken, body)
	var result agentbridge.ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("call response is not a ToolCallResult: %v: %s", err, raw)
	}
	return resp, result
}

func TestDeviceWorkIsHandedToThePhoneAndItsAnswerReturned(t *testing.T) {
	worker := &fakeDeviceWorker{result: devicework.Result{
		Answered: true, Reached: manifest.Completes, Done: true,
		Detail: "Handed to the app — we can't see whether it reached them.",
	}}
	server := deviceWorkBridge(t, newHandOffAdapter(), worker)

	resp, result := deviceWorkCall(t, server)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !result.OK || !result.Done || result.Reached != "completes" {
		t.Fatalf("the phone's answer was not returned to the agent: %+v", result)
	}
	if result.Detail != "Handed to the app — we can't see whether it reached them." {
		t.Fatalf("the phone's explanation was dropped: %q", result.Detail)
	}

	if len(worker.asks) != 1 {
		t.Fatalf("the phone was asked %d times, want exactly once", len(worker.asks))
	}
	ask := worker.asks[0]
	if ask.AdapterID != "notification_reply" || ask.Kind != "notification_reply" || ask.Handle != "maya" || ask.Text != "on my way" || ask.Ceiling != manifest.Completes {
		t.Fatalf("the ask lost what the adapter had decided: %+v", ask)
	}
	if !worker.hadDeadline {
		t.Fatal("the ask was sent with no deadline — an unanswered phone would hang the agent's tool call forever")
	}
}

func TestDeviceWorkWithoutAPhoneIsAdapterFailed(t *testing.T) {
	server := deviceWorkBridge(t, newHandOffAdapter(), nil)

	resp, result := deviceWorkCall(t, server)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if result.OK || result.Error == nil || result.Error.Code != "adapter_failed" {
		t.Fatalf("no phone must be an adapter failure: %+v", result)
	}
	if !strings.Contains(strings.ToLower(result.Error.Message), "phone") {
		t.Fatalf("the failure does not tell the agent what is missing: %q", result.Error.Message)
	}
}

func TestADeviceWorkerFailureIsAdapterFailed(t *testing.T) {
	worker := &fakeDeviceWorker{err: context.DeadlineExceeded}
	server := deviceWorkBridge(t, newHandOffAdapter(), worker)

	resp, result := deviceWorkCall(t, server)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if result.OK || result.Error == nil || result.Error.Code != "adapter_failed" {
		t.Fatalf("a device-worker error must be an adapter failure: %+v", result)
	}
}

func TestAnUnansweredAskIsReportedNotFailed(t *testing.T) {
	// The phone never answered in time. That is an answer — outcome
	// unknown — not an error: the reply may already sit in someone's chat,
	// and a "failed" here would invite the agent to send it again.
	worker := &fakeDeviceWorker{result: devicework.Result{
		Answered: false, Done: false,
		Detail: "The phone did not answer in time; whether the reply went out is unknown.",
	}}
	server := deviceWorkBridge(t, newHandOffAdapter(), worker)

	resp, result := deviceWorkCall(t, server)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !result.OK || result.Done {
		t.Fatalf("an unknown outcome was reported as something stronger: %+v", result)
	}
	if result.Detail == "" {
		t.Fatal("an unknown outcome must explain itself to the agent")
	}
	if result.Error != nil {
		t.Fatalf("outcome-unknown is an answer, not an error: %+v", result.Error)
	}
}
