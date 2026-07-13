package mobilesession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

var sessionNow = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

func TestColdHelloSendsWelcomeAndSafeProjectSnapshot(t *testing.T) {
	handler, sender := newTestHandler(t)

	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}

	if len(sender.messages) != 2 || sender.messages[0].Type != "welcome" || sender.messages[1].Type != "snapshot" {
		t.Fatalf("outputs = %#v", sender.messages)
	}
	var snapshot struct {
		BaseSequence uint64 `json:"baseSeq"`
		ComputerName string `json:"computerName"`
		Projects     []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"projects"`
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(sender.messages[1].Body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.BaseSequence != 1 || snapshot.ComputerName != "Studio Mac" || len(snapshot.Projects) != 1 || snapshot.Projects[0].ID != "main" || snapshot.Projects[0].DisplayName != "Main" || len(snapshot.Tasks) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if bytes.Contains(sender.messages[1].Body, []byte(sender.projectPath)) {
		t.Fatal("snapshot exposed a configured path")
	}
}

func TestSetProjectReturnsSequencedConfirmedOrFailedResult(t *testing.T) {
	handler, sender := newTestHandler(t)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	sender.messages = nil

	for _, test := range []struct {
		actionID string
		project  string
		state    string
		error    string
	}{
		{actionID: "action-ok", project: "main", state: "confirmed"},
		{actionID: "action-missing", project: "removed", state: "failed", error: "invalid_action"},
	} {
		body := `{"actionId":"` + test.actionID + `","kind":"set_project","projectId":"` + test.project + `"}`
		message := contract.Message{Version: contract.Version{Major: 1}, MessageID: "request-" + test.actionID, Sender: "phone", Type: "action", Body: json.RawMessage(body)}
		if err := handler.Handle(context.Background(), sender, message); err != nil {
			t.Fatal(err)
		}
		result := sender.messages[len(sender.messages)-1]
		if result.Type != "action_result" || result.Sequence == nil {
			t.Fatalf("result = %#v", result)
		}
		var resultBody struct {
			ActionID string `json:"actionId"`
			State    string `json:"state"`
			Error    *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(result.Body, &resultBody)
		if resultBody.ActionID != test.actionID || resultBody.State != test.state || test.error != "" && (resultBody.Error == nil || resultBody.Error.Code != test.error) {
			t.Fatalf("result body = %#v", resultBody)
		}
	}
	if *sender.messages[0].Sequence != 2 || *sender.messages[1].Sequence != 3 {
		t.Fatalf("result sequences = %d, %d", *sender.messages[0].Sequence, *sender.messages[1].Sequence)
	}
}

func TestAcknowledgementIsStoredPerAuthenticatedDevice(t *testing.T) {
	handler, sender := newTestHandler(t)
	if err := handler.Handle(context.Background(), sender, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	ack := contract.Message{Version: contract.Version{Major: 1}, MessageID: "ack-1", Sender: "phone", Type: "ack", Body: json.RawMessage(`{"throughSeq":1}`)}
	if err := handler.Handle(context.Background(), sender, ack); err != nil {
		t.Fatal(err)
	}
	through, err := sender.store.Acknowledged(context.Background(), sender.DeviceID())
	if err != nil || through != 1 {
		t.Fatalf("acknowledged = %d, %v", through, err)
	}
}

func TestNewHelloReplacesTheOlderDeviceSessionBeforeMoreEventsAreApplied(t *testing.T) {
	handler, first := newTestHandler(t)
	if err := handler.Handle(context.Background(), first, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-1","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	second := &recordingSender{deviceID: first.deviceID, sessionID: "session-2", connectionID: 2, projectPath: first.projectPath, store: first.store}
	if err := handler.Handle(context.Background(), second, decode(t, `{"version":{"major":1,"minor":0},"messageId":"hello-2","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)); err != nil {
		t.Fatal(err)
	}
	if !first.closed {
		t.Fatal("older device session remained open")
	}
	action := contract.Message{Version: contract.Version{Major: 1}, MessageID: "request-old", Sender: "phone", Type: "action", Body: json.RawMessage(`{"actionId":"action-old","kind":"set_project","projectId":"main"}`)}
	if err := handler.Handle(context.Background(), first, action); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("older session action error = %v", err)
	}
	action.MessageID = "request-current"
	action.Body = json.RawMessage(`{"actionId":"action-current","kind":"set_project","projectId":"main"}`)
	if err := handler.Handle(context.Background(), second, action); err != nil {
		t.Fatal(err)
	}
	result := second.messages[len(second.messages)-1]
	if result.Sequence == nil || *result.Sequence != 2 {
		t.Fatalf("current session result sequence = %v, want 2", result.Sequence)
	}
}

type recordingSender struct {
	deviceID     string
	sessionID    string
	messages     []contract.Message
	projectPath  string
	store        *eventjournal.MemoryStore
	connectionID uint64
	closed       bool
}

func (sender *recordingSender) DeviceID() string     { return sender.deviceID }
func (sender *recordingSender) SessionID() string    { return sender.sessionID }
func (sender *recordingSender) ConnectionID() uint64 { return sender.connectionID }
func (sender *recordingSender) Close()               { sender.closed = true }
func (sender *recordingSender) Send(_ context.Context, message contract.Message) error {
	encoded, err := contract.EncodeText(message)
	if err != nil {
		return err
	}
	decoded, err := contract.DecodeText(encoded)
	if err == nil {
		sender.messages = append(sender.messages, decoded)
	}
	return err
}

func newTestHandler(t *testing.T) (*Handler, *recordingSender) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.New([]projects.Config{{ID: "main", DisplayName: "Main", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	store := eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 16, MaxBytes: 64 * 1024})
	journal := eventjournal.New(store, nil)
	handler, err := New(context.Background(), "Studio Mac", projectService, journal, func() time.Time { return sessionNow })
	if err != nil {
		t.Fatal(err)
	}
	return handler, &recordingSender{deviceID: "pixel-9", sessionID: "session-1", connectionID: 1, projectPath: root, store: store}
}

func decode(t *testing.T, frame string) contract.Message {
	t.Helper()
	message, err := contract.DecodeText([]byte(frame))
	if err != nil {
		t.Fatal(err)
	}
	return message
}
