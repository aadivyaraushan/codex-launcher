package beepermessage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
)

// This is the lever. Every earlier field bug in the read path (empty detail,
// control chars in a line, an empty confirm label) only ever showed up on the
// real phone, because the package's own tests checked shape, not the wire
// contract. This test builds the exact JSON bodies the mobile-session handler
// puts on the wire for a capability_preview and a capability_result, wraps each
// in a real contract.Message, and runs it through the real contract encoder.
// The encoder round-trips through the decoder, so anything the phone would
// reject fails here first, on a laptop, in milliseconds.
//
// The message text carries newlines, tabs, and a carriage return on purpose:
// a real DM is multi-line, and the wire contract rejects any control character
// in a display string. If the adapter ever stops folding those out, this test
// goes red instead of the phone's session dying.
func buildUnreadScanFixture(t *testing.T) (adapter.Preview, adapter.Outcome) {
	t.Helper()
	f := &fakeBeeper{
		unreadChats: []beeper.Chat{
			{ID: "ig1", Network: "Instagram", Title: "Maya", UnreadCount: 2},
			{ID: "ig2", Network: "Instagram", Title: "Devon", UnreadCount: 1},
		},
		messages: map[string][]beeper.Message{
			"ig1": {{SenderName: "Maya", Text: "hey\nare you\tfree tonight?\r\ncall me"}},
			"ig2": {{SenderName: "Devon", Text: "sent the deck"}},
		},
	}
	a := igAdapter(f)
	plan := mustResolve(t, a, adapter.Intent{Verb: "read"})
	preview, err := a.Preview(context.Background(), plan)
	if err != nil {
		t.Fatalf("Preview returned an error: %v", err)
	}
	outcome, err := a.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	return preview, outcome
}

func TestReadPreviewBodyPassesTheRealWireContract(t *testing.T) {
	preview, _ := buildUnreadScanFixture(t)

	body, err := json.Marshal(struct {
		RequestID    string   `json:"requestId"`
		AdapterID    string   `json:"adapterId"`
		Verb         string   `json:"verb"`
		Headline     string   `json:"headline"`
		Lines        []string `json:"lines"`
		ConfirmLabel string   `json:"confirmLabel"`
		Fingerprint  string   `json:"fingerprint"`
	}{"req-test-1", preview.Plan.AdapterID, string(preview.Plan.Verb), preview.Headline, preview.Lines, preview.Confirm, preview.Fingerprint()})
	if err != nil {
		t.Fatalf("marshalling the preview body failed: %v", err)
	}

	msg := contract.Message{
		Version:   contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor},
		MessageID: "msg-preview-1",
		Sender:    "companion",
		Type:      "capability_preview",
		Body:      body,
	}
	if _, err := contract.EncodeText(msg); err != nil {
		t.Fatalf("the preview frame the phone would receive is invalid: %v\nheadline=%q confirm=%q lines=%q", err, preview.Headline, preview.Confirm, preview.Lines)
	}
}

func TestReadResultBodyPassesTheRealWireContract(t *testing.T) {
	_, outcome := buildUnreadScanFixture(t)

	body, err := json.Marshal(struct {
		RequestID   string `json:"requestId"`
		Ceiling     string `json:"ceiling"`
		Done        bool   `json:"done"`
		Detail      string `json:"detail"`
		HandedOffTo string `json:"handedOffTo"`
	}{"req-test-1", string(outcome.Reached), outcome.Done, outcome.Detail, outcome.HandedOffTo})
	if err != nil {
		t.Fatalf("marshalling the result body failed: %v", err)
	}

	seq := uint64(1)
	msg := contract.Message{
		Version:   contract.Version{Major: contract.ProtocolMajor, Minor: contract.ProtocolMinor},
		MessageID: "msg-result-1",
		Sender:    "companion",
		Type:      "capability_result",
		Sequence:  &seq,
		Body:      body,
	}
	if _, err := contract.EncodeText(msg); err != nil {
		t.Fatalf("the result frame the phone would receive is invalid: %v\ndetail=%q", err, outcome.Detail)
	}
}
