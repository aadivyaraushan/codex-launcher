package mobilesession

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// The wire half of "every official account connection can be revoked by the
// user". flow.Disconnect exists one layer down; without a message that reaches
// it, the only callers are still three proof commands, and the promise is
// still not kept.
//
// The rule that matters here is what the phone is told when it goes wrong. A
// disconnect that half-worked must come back as failed, because the screen the
// user is looking at will otherwise move the app into "not connected" and they
// will stop trying.

func (f *recordingCapabilityFlow) Disconnect(_ context.Context, adapterID string) error {
	f.disconnected = append(f.disconnected, adapterID)
	if f.disconnectErr != nil {
		return f.disconnectErr
	}
	return nil
}

func TestADisconnectActionReachesTheFlowAndIsConfirmed(t *testing.T) {
	handler, sender := newTestHandler(t)
	flow := &recordingCapabilityFlow{}
	handler.EnableCapabilities(flow)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-cap","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"cap-disconnect","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"todoist"}}`)); err != nil {
		t.Fatal(err)
	}

	result := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"confirmed"`)) {
		t.Fatalf("result=%+v", result)
	}
	if len(flow.disconnected) != 1 || flow.disconnected[0] != "todoist" {
		t.Fatalf("the flow was asked to disconnect %v", flow.disconnected)
	}
}

// The one that keeps the screen honest. A revoke that could not finish comes
// back as failed, so the phone leaves the app showing as still connected and
// the user tries again rather than believing a token is gone when it is not.
func TestADisconnectThatFailedIsNotReportedAsConfirmed(t *testing.T) {
	handler, sender := newTestHandler(t)
	flow := &recordingCapabilityFlow{disconnectErr: errors.New("token endpoint is down")}
	handler.EnableCapabilities(flow)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-cap","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"cap-disconnect","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"todoist"}}`)); err != nil {
		t.Fatal(err)
	}

	result := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a failed disconnect came back as %s", result.Body)
	}
}

// A companion with no capability flow at all has nothing to disconnect from,
// and saying "confirmed" would tell the user a connection was undone that was
// never even looked at.
func TestADisconnectWithNoCapabilityFlowIsAFailureNotASilentYes(t *testing.T) {
	handler, sender := newTestHandler(t)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-cap","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.sent = make(chan contract.Message, 2)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"cap-disconnect","sender":"phone","type":"action","body":{"actionId":"cap-disconnect-1","kind":"capability_disconnect","adapterId":"todoist"}}`)); err != nil {
		t.Fatal(err)
	}

	result := awaitSentMessage(t, sender.sent)
	if result.Type != "action_result" || !bytes.Contains(result.Body, []byte(`"state":"failed"`)) {
		t.Fatalf("a disconnect with no capability flow came back as %s", result.Body)
	}
}
