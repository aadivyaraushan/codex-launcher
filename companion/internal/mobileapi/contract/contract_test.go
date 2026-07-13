package contract

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGoldenFixturesDecodeThroughProductionContract(t *testing.T) {
	for _, name := range []string{"session.jsonl", "approval.jsonl", "reconnect.jsonl"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "protocol", "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		for lineNumber, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			message, err := DecodeText([]byte(line))
			if err != nil {
				t.Fatalf("%s:%d: %v", name, lineNumber+1, err)
			}
			if message.Type == "" || message.MessageID == "" {
				t.Fatalf("%s:%d decoded empty envelope", name, lineNumber+1)
			}
		}
	}
}

func TestSessionRejectsReplayGapsAndUnsafeResume(t *testing.T) {
	tests := []struct {
		name string
		feed []string
		want error
	}{
		{
			name: "duplicate action ID",
			feed: []string{helloNoState("h-1"), action("m-1", "duplicate"), action("m-2", "duplicate")},
			want: ErrDuplicateAction,
		},
		{
			name: "gapped sequence",
			feed: []string{helloNoState("h-1"), welcome("w-1"), snapshot("s-1", 4), event("e-1", 6)},
			want: ErrSequenceGap,
		},
		{
			name: "ack beyond received sequence",
			feed: []string{helloNoState("h-1"), welcome("w-1"), snapshot("s-1", 4), ack("a-1", 5)},
			want: ErrInvalidAck,
		},
		{
			name: "cold start with cursor",
			feed: []string{`{"version":{"major":1,"minor":0},"messageId":"h-1","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"no_local_state","lastAck":9}}}`},
			want: ErrColdResumeCursor,
		},
		{
			name: "cold start with upload cursor",
			feed: []string{`{"version":{"major":1,"minor":0},"messageId":"h-uploads","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"no_local_state","uploads":{"upload-1":2}}}}`},
			want: ErrColdResumeCursor,
		},
		{
			name: "unsupported major",
			feed: []string{`{"version":{"major":2,"minor":0},"messageId":"h-1","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[2],"resume":{"mode":"no_local_state"}}}`},
			want: ErrUnsupportedVersion,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := NewSession()
			var err error
			for _, frame := range test.feed {
				_, err = session.AcceptText([]byte(frame))
				if err != nil {
					break
				}
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("AcceptText() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSessionStaysClosedAfterProtocolViolation(t *testing.T) {
	session := NewSession()
	if _, err := session.AcceptText([]byte(ack("bad", 1))); !errors.Is(err, ErrInvalidAck) {
		t.Fatalf("first error = %v", err)
	}
	if _, err := session.AcceptText([]byte(snapshot("later", 1))); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("second error = %v", err)
	}
}

func TestCursorsNeverMoveBackward(t *testing.T) {
	for _, feed := range [][]string{
		{snapshot("s-1", 4), ack("a-1", 4), ack("a-2", 3)},
		{snapshot("s-1", 4), snapshot("s-2", 3)},
	} {
		session := NewSession()
		var err error
		for _, frame := range feed {
			_, err = session.AcceptText([]byte(frame))
			if err != nil {
				break
			}
		}
		if err == nil {
			t.Fatal("session accepted a backward cursor")
		}
	}
}

func TestWarmResumeCursorIsAlsoTheAcknowledgementFloor(t *testing.T) {
	session := NewSession()
	frames := []string{
		`{"version":{"major":1,"minor":0},"messageId":"h-warm","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","lastAck":9}}}`,
		ack("a-backward", 8),
	}
	for index, frame := range frames {
		_, err := session.AcceptText([]byte(frame))
		if index == 1 && !errors.Is(err, ErrInvalidAck) {
			t.Fatalf("backward acknowledgement error = %v", err)
		}
	}
}

func TestContractRejectsMalformedActionsRawPathsAndOversizedFrames(t *testing.T) {
	for _, test := range []struct {
		name, frame string
		want        error
	}{
		{name: "malformed approval", frame: `{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"approval","taskId":"task-1","decision":"yes"}}`, want: ErrInvalidAction},
		{name: "raw project path", frame: `{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"set_project","projectPath":"/etc"}}`, want: ErrInvalidAction},
		{name: "oversized", frame: strings.Repeat("x", MaxJSONFrameBytes+1), want: ErrFrameTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeText([]byte(test.frame))
			if !errors.Is(err, test.want) {
				t.Fatalf("DecodeText() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestContractAcceptsOnlyOpaqueApprovedProjectIdentifiers(t *testing.T) {
	frame := `{"version":{"major":1,"minor":0},"messageId":"m-project","sender":"phone","type":"action","body":{"actionId":"a-project","kind":"set_project","projectId":"project-main"}}`
	if _, err := DecodeText([]byte(frame)); err != nil {
		t.Fatalf("opaque project identifier was rejected: %v", err)
	}
	invalid := `{"version":{"major":1,"minor":0},"messageId":"m-project-invalid","sender":"phone","type":"action","body":{"actionId":"a-project","kind":"set_project","projectId":"project:main"}}`
	if _, err := DecodeText([]byte(invalid)); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("non-project identifier error = %v, want %v", err, ErrInvalidAction)
	}
}

func TestContractAcceptsNewTaskStartWithHostOptionIdentifiers(t *testing.T) {
	valid := `{"version":{"major":1,"minor":0},"messageId":"new-task","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"project-main","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}`
	if _, err := DecodeText([]byte(valid)); err != nil {
		t.Fatalf("valid new-task action was rejected: %v", err)
	}

	invalid := []string{
		`{"version":{"major":1,"minor":0},"messageId":"missing-project","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"mixed-targets","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","taskId":"task-1","projectId":"project-main","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"raw-path","sender":"phone","type":"action","body":{"actionId":"action-1","kind":"start_turn","projectId":"project-main","projectPath":"/tmp/private","text":"Fix the tests","modelId":"gpt-5.4","reasoningId":"high","permissionModeId":"workspace-write"}}`,
	}
	for _, frame := range invalid {
		if _, err := DecodeText([]byte(frame)); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("invalid new-task action error = %v, want %v", err, ErrInvalidAction)
		}
	}
}

func TestContractAcceptsOnlyBoundedTaskManagementActions(t *testing.T) {
	valid := []string{
		`{"version":{"major":1,"minor":0},"messageId":"rename","sender":"phone","type":"action","body":{"actionId":"a-rename","kind":"rename_task","taskId":"thread-1","title":"Launcher follow-up"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"archive","sender":"phone","type":"action","body":{"actionId":"a-archive","kind":"archive_task","taskId":"thread-1"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"fork","sender":"phone","type":"action","body":{"actionId":"a-fork","kind":"fork_task","taskId":"thread-1"}}`,
	}
	for _, frame := range valid {
		if _, err := DecodeText([]byte(frame)); err != nil {
			t.Fatalf("valid task action was rejected: %v", err)
		}
	}

	invalid := []string{
		`{"version":{"major":1,"minor":0},"messageId":"blank","sender":"phone","type":"action","body":{"actionId":"a-rename","kind":"rename_task","taskId":"thread-1","title":"   "}}`,
		`{"version":{"major":1,"minor":0},"messageId":"control","sender":"phone","type":"action","body":{"actionId":"a-rename","kind":"rename_task","taskId":"thread-1","title":"bad\nname"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"extra","sender":"phone","type":"action","body":{"actionId":"a-archive","kind":"archive_task","taskId":"thread-1","title":"hidden"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"bad-task","sender":"phone","type":"action","body":{"actionId":"a-fork","kind":"fork_task","taskId":"bad id!"}}`,
	}
	for _, frame := range invalid {
		if _, err := DecodeText([]byte(frame)); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("invalid task action error = %v, want %v", err, ErrInvalidAction)
		}
	}
}

func TestContractAcceptsOnlyExactUnknownControlDismissal(t *testing.T) {
	valid := `{"version":{"major":1,"minor":0},"messageId":"dismiss","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control","taskId":"thread-1","targetActionId":"unknown-1"}}`
	if _, err := DecodeText([]byte(valid)); err != nil {
		t.Fatalf("valid dismissal was rejected: %v", err)
	}
	invalid := []string{
		`{"version":{"major":1,"minor":0},"messageId":"missing","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control","taskId":"thread-1"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"extra","sender":"phone","type":"action","body":{"actionId":"dismiss-1","kind":"dismiss_unknown_control","taskId":"thread-1","targetActionId":"unknown-1","text":"hidden"}}`,
	}
	for _, frame := range invalid {
		if _, err := DecodeText([]byte(frame)); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("invalid dismissal error = %v", err)
		}
	}
}

func TestContractAcceptsBoundedUnsequencedTaskTranscriptPages(t *testing.T) {
	read := `{"version":{"major":1,"minor":0},"messageId":"read-1","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":32,"beforeEntryId":"agent-2"}}`
	if _, err := DecodeText([]byte(read)); err != nil {
		t.Fatalf("task read was rejected: %v", err)
	}
	page := `{"version":{"major":1,"minor":0},"messageId":"page-1","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[
		{"id":"user-1","turnId":"turn-1","kind":"user","text":"Fix it"},
		{"id":"command-1","turnId":"turn-1","kind":"command","status":"completed","command":"go test ./...","output":"ok"},
		{"id":"file-1","turnId":"turn-1","kind":"file_change","status":"completed","changes":[{"path":"src/main.go","kind":"update","diff":"@@"}]}
	],"earlierCursor":"user-1","truncated":false}}`
	message, err := DecodeText([]byte(page))
	if err != nil {
		t.Fatalf("task page was rejected: %v", err)
	}
	if message.Sequence != nil {
		t.Fatalf("task page unexpectedly entered durable sequence: %v", *message.Sequence)
	}
}

func TestContractRejectsTranscriptInternalsAndMalformedPages(t *testing.T) {
	frames := map[string]string{
		"sequenced page":           `{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","seq":2,"body":{"requestId":"request-1","taskId":"thread-1","entries":[],"truncated":false}}`,
		"raw cwd":                  `{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"command-1","turnId":"turn-1","kind":"command","status":"completed","command":"pwd","cwd":"/private"}],"truncated":false}}`,
		"hidden reasoning content": `{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"reason-1","turnId":"turn-1","kind":"reasoning","text":"summary","content":"hidden"}],"truncated":false}}`,
		"unknown entry kind":       `{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"item-1","turnId":"turn-1","kind":"raw","text":"private"}],"truncated":false}}`,
		"null entries":             `{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":null,"truncated":false}}`,
		"null file changes":        `{"version":{"major":1,"minor":0},"messageId":"page","sender":"companion","type":"task_page","body":{"requestId":"request-1","taskId":"thread-1","entries":[{"id":"file-1","turnId":"turn-1","kind":"file_change","status":"inProgress","changes":null}],"truncated":false}}`,
		"invalid read limit":       `{"version":{"major":1,"minor":0},"messageId":"read","sender":"phone","type":"task_read","body":{"requestId":"request-1","taskId":"thread-1","limit":65}}`,
	}
	for name, frame := range frames {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeText([]byte(frame)); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("error = %v, want %v", err, ErrInvalidEnvelope)
			}
		})
	}
}

func TestSnapshotCarriesOnlySafeComputerAndOpaqueProjectChoices(t *testing.T) {
	valid := `{"version":{"major":1,"minor":0},"messageId":"snapshot-projects","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Aadi's Mac","projects":[{"id":"project-main","displayName":"Codex Launcher"}],"tasks":[]}}`
	if _, err := DecodeText([]byte(valid)); err != nil {
		t.Fatalf("safe project snapshot was rejected: %v", err)
	}
	for name, frame := range map[string]string{
		"missing choices":   `{"version":{"major":1,"minor":0},"messageId":"missing","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"tasks":[]}}`,
		"raw path":          `{"version":{"major":1,"minor":0},"messageId":"path","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project-main","displayName":"Main","path":"/private"}],"tasks":[]}}`,
		"duplicate id":      `{"version":{"major":1,"minor":0},"messageId":"duplicate","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"same","displayName":"One"},{"id":"same","displayName":"Two"}],"tasks":[]}}`,
		"control character": `{"version":{"major":1,"minor":0},"messageId":"control","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project-main","displayName":"Main\nInjected"}],"tasks":[]}}`,
		"c1 control":        `{"version":{"major":1,"minor":0},"messageId":"c1-control","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project-main","displayName":"Main\u0085Injected"}],"tasks":[]}}`,
		"non-project id":    `{"version":{"major":1,"minor":0},"messageId":"project-id","sender":"companion","type":"snapshot","seq":1,"body":{"baseSeq":1,"computerName":"Mac","projects":[{"id":"project:main","displayName":"Main"}],"tasks":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeText([]byte(frame)); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("DecodeText() error = %v, want %v", err, ErrInvalidEnvelope)
			}
		})
	}
}

func TestAttachmentChunksRequireOrderSizeDigestAndAuthentication(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	quota := NewAttachmentQuota(DefaultAttachmentLimits())
	session := NewSessionWithAttachments(key, quota, "session-1", "phone-1")
	offer := AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 4, SHA256: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"}
	if err := session.OfferAttachment(offer); err != nil {
		t.Fatal(err)
	}
	chunk := AttachmentChunk{SessionID: "session-1", UploadID: "upload-1", Chunk: 0, Offset: 0, DeclaredTotal: 4, Final: true, Payload: []byte("data")}
	frame, err := EncodeAttachmentFrame(chunk, key)
	if err != nil {
		t.Fatal(err)
	}
	wantHex, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "protocol", "fixtures", "attachments", "authenticated-frame.hex"))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) != strings.TrimSpace(string(wantHex)) {
		t.Fatal("Go attachment encoding drifted from the shared golden frame")
	}
	if err := session.AcceptAttachmentFrame(frame); err != nil {
		t.Fatal(err)
	}
	ack, err := session.CompleteAttachment("upload-1")
	if err != nil || ack.ReceivedBytes != 4 || ack.SHA256 != offer.SHA256 {
		t.Fatalf("CompleteAttachment() = %#v, %v", ack, err)
	}

	tampered := append([]byte(nil), frame...)
	tampered[len(tampered)-33] ^= 0xff
	if _, err := DecodeAttachmentFrame(tampered, key); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("tampered frame error = %v", err)
	}
	if _, err := DecodeAttachmentFrame(frame, []byte("wrong-key-wrong-key-wrong-key-12")); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("wrong key error = %v", err)
	}
	replayed, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "old-session", UploadID: "upload-1", Chunk: 0, Offset: 0, DeclaredTotal: 4, Final: true, Payload: []byte("data")}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AcceptAttachmentFrame(replayed); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("cross-session replay error = %v", err)
	}
}

func TestAttachmentOfferRejectsQuotasBeforeAllocation(t *testing.T) {
	limits := DefaultAttachmentLimits()
	limits.MaxDeviceUploads = 1
	limits.MaxGlobalUploads = 1
	quota := NewAttachmentQuota(limits)
	first := NewSessionWithAttachments([]byte("0123456789abcdef0123456789abcdef"), quota, "session-1", "phone-1")
	second := NewSessionWithAttachments([]byte("abcdef0123456789abcdef0123456789"), quota, "session-2", "phone-2")
	valid := AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 4, SHA256: strings.Repeat("a", 64)}
	if err := first.OfferAttachment(valid); err != nil {
		t.Fatal(err)
	}
	if err := first.OfferAttachment(AttachmentOffer{UploadID: "upload-2", DeclaredTotal: 4, SHA256: strings.Repeat("b", 64)}); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("device quota error = %v", err)
	}
	if err := second.OfferAttachment(AttachmentOffer{UploadID: "upload-3", DeclaredTotal: 4, SHA256: strings.Repeat("c", 64)}); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("global quota error = %v", err)
	}
	first.CancelAttachment("upload-1")
	if err := second.OfferAttachment(AttachmentOffer{UploadID: "upload-3", DeclaredTotal: 4, SHA256: strings.Repeat("c", 64)}); err != nil {
		t.Fatalf("quota was not released after cancel: %v", err)
	}
}

func TestQuotaConfigurationCannotExceedProtocolCaps(t *testing.T) {
	limits := AttachmentLimits{MaxAttachmentBytes: MaxAttachmentBytes + 1, MaxDeviceUploads: MaxDeviceUploads + 1, MaxGlobalUploads: MaxGlobalUploads + 1, MaxTemporaryBytes: MaxTemporaryBytes + 1}
	session := NewSessionWithAttachments(nil, NewAttachmentQuota(limits), "session-1", "phone-1")
	if err := session.OfferAttachment(AttachmentOffer{UploadID: "too-large", DeclaredTotal: MaxAttachmentBytes + 1, SHA256: strings.Repeat("a", 64)}); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("oversized configured offer error = %v", err)
	}
	for index := 0; index < MaxDeviceUploads+1; index++ {
		err := session.OfferAttachment(AttachmentOffer{UploadID: fmt.Sprintf("upload-%d", index), DeclaredTotal: 1, SHA256: strings.Repeat("a", 64)})
		if index == MaxDeviceUploads && !errors.Is(err, ErrAttachmentQuota) {
			t.Fatalf("offer %d exceeded protocol device cap: %v", index, err)
		}
	}
}

func TestDeviceQuotaSpansReconnectSessions(t *testing.T) {
	quota := NewAttachmentQuota(DefaultAttachmentLimits())
	first := NewSessionWithAttachments(nil, quota, "session-1", "phone-1")
	second := NewSessionWithAttachments(nil, quota, "session-2", "phone-1")
	for index, session := range []*Session{first, second} {
		if err := session.OfferAttachment(AttachmentOffer{UploadID: fmt.Sprintf("upload-%d", index), DeclaredTotal: 1, SHA256: strings.Repeat("a", 64)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := second.OfferAttachment(AttachmentOffer{UploadID: "upload-3", DeclaredTotal: 1, SHA256: strings.Repeat("a", 64)}); !errors.Is(err, ErrAttachmentQuota) {
		t.Fatalf("same device crossed session quota: %v", err)
	}
}

func TestAttachmentReservationsExpireAndReleaseQuota(t *testing.T) {
	limits := DefaultAttachmentLimits()
	limits.MaxGlobalUploads = 1
	quota := NewAttachmentQuota(limits)
	first := NewSessionWithAttachments([]byte("0123456789abcdef0123456789abcdef"), quota, "session-1", "phone-1")
	second := NewSessionWithAttachments([]byte("abcdef0123456789abcdef0123456789"), quota, "session-2", "phone-2")
	offer := AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 4, SHA256: strings.Repeat("a", 64)}
	started := time.Unix(1_000, 0)
	if err := first.OfferAttachmentAt(offer, started); err != nil {
		t.Fatal(err)
	}
	if err := second.OfferAttachmentAt(AttachmentOffer{UploadID: "upload-2", DeclaredTotal: 4, SHA256: strings.Repeat("b", 64)}, started.Add(time.Duration(UploadExpirySeconds+1)*time.Second)); err != nil {
		t.Fatalf("expired quota was not released: %v", err)
	}
}

func TestProductionValidationRejectsFieldsOutsideSchema(t *testing.T) {
	frames := []string{
		`{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"interrupt_turn","taskId":"task-1","text":"hidden"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-2","sender":"phone","type":"ack","body":{"throughSeq":1,"extra":true}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-3","sender":"phone","type":"ack","body":{"throughSeq":1}} {}`,
		`{"version":{"major":1,"minor":0},"messageId":"bad id!","sender":"phone","type":"ack","body":{"throughSeq":1}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-5","sender":"phone","type":"action","body":{"actionId":"a-5","kind":"start_turn","taskId":"task-1","text":"go","attachmentIds":["valid","bad id!"]}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-6","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["same","same"],"limits":{"maxJsonBytes":262145,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-7","sender":"companion","type":"event","seq":1,"body":{"taskId":"task-1","event":"invented","state":"working","summary":"Working"}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-8","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","uploads":[{"uploadId":"u-1","nextChunk":"two"}]}}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-9","sender":"phone","type":"ack","seq":1,"body":{"throughSeq":1}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-10","sender":"companion","type":"snapshot","seq":0,"body":{"baseSeq":0,"tasks":[]}}`,
		`{"version":{"major":1,"minor":0},"messageId":"m-11","sender":"phone","type":"ack","body":{"throughSeq":0}}`,
	}
	for _, frame := range frames {
		if _, err := DecodeText([]byte(frame)); err == nil {
			t.Fatalf("DecodeText accepted schema drift: %s", frame)
		}
	}
}

func TestWelcomeAcceptsStrictNewTaskOptionCatalog(t *testing.T) {
	frame := `{"version":{"major":1,"minor":0},"messageId":"welcome-options","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Balances speed and depth."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Can change the selected project.","isDefault":true}]}}}`
	if _, err := DecodeText([]byte(frame)); err != nil {
		t.Fatalf("valid new task options were rejected: %v", err)
	}
}

func TestWelcomeRejectsUnsafeNewTaskOptionCatalogs(t *testing.T) {
	frames := []string{
		`{"version":{"major":1,"minor":0},"messageId":"bad-options-1","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[],"permissionModes":[]}}}`,
		`{"version":{"major":1,"minor":0},"messageId":"bad-options-2","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"high","reasoning":[{"id":"medium","displayName":"Medium","description":"Safe."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Safe.","isDefault":true}]}}}`,
		`{"version":{"major":1,"minor":0},"messageId":"bad-options-3","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":[],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Safe."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Safe.","isDefault":true}]}}}`,
		`{"version":{"major":1,"minor":0},"messageId":"bad-options-4","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":null,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Safe."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Safe.","isDefault":true}]}}}`,
		`{"version":{"major":1,"minor":0},"messageId":"bad-options-5","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Safe."}]}],"permissionModes":[{"id":"workspace-write","displayName":"Workspace","description":"Safe.","isDefault":true},{"id":"read-only","displayName":"Read only","description":"Safe.","isDefault":null}]}}}`,
	}
	for _, frame := range frames {
		if _, err := DecodeText([]byte(frame)); err == nil {
			t.Fatalf("DecodeText accepted unsafe new task options: %s", frame)
		}
	}
}

func TestExpiredAttachmentsCannotReceiveOrComplete(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	for _, operation := range []string{"receive", "complete"} {
		session := NewSessionWithAttachments(key, nil, "session-1", "phone-1")
		offer := AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 1, SHA256: "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"}
		if err := session.OfferAttachmentAt(offer, time.Unix(1_000, 0)); err != nil {
			t.Fatal(err)
		}
		frame, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "session-1", UploadID: "upload-1", DeclaredTotal: 1, Final: true, Payload: []byte("x")}, key)
		if err != nil {
			t.Fatal(err)
		}
		if operation == "receive" {
			err = session.AcceptAttachmentFrame(frame)
		} else {
			_, err = session.CompleteAttachment("upload-1")
		}
		if !errors.Is(err, ErrInvalidAttachment) {
			t.Fatalf("expired %s error = %v", operation, err)
		}
	}
}

func TestAttachmentChunksAdvanceOneAtATimeAndRejectUnknownFlags(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	if _, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "session-1", UploadID: "overflow", Offset: int64(1<<63 - 1), DeclaredTotal: 1, Payload: []byte("x")}, key); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("overflowing offset error = %v", err)
	}
	session := NewSessionWithAttachments(key, nil, "session-1", "phone-1")
	offer := AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 4, SHA256: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"}
	if err := session.OfferAttachment(offer); err != nil {
		t.Fatal(err)
	}
	premature, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "session-1", UploadID: "upload-1", Chunk: 0, Offset: 0, DeclaredTotal: 4, Final: true, Payload: []byte("da")}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AcceptAttachmentFrame(premature); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("premature final error = %v", err)
	}
	for _, chunk := range []AttachmentChunk{
		{SessionID: "session-1", UploadID: "upload-1", Chunk: 0, Offset: 0, DeclaredTotal: 4, Payload: []byte("da")},
		{SessionID: "session-1", UploadID: "upload-1", Chunk: 1, Offset: 2, DeclaredTotal: 4, Final: true, Payload: []byte("ta")},
	} {
		frame, err := EncodeAttachmentFrame(chunk, key)
		if err != nil {
			t.Fatal(err)
		}
		if err := session.AcceptAttachmentFrame(frame); err != nil {
			t.Fatalf("chunk %d was rejected: %v", chunk.Chunk, err)
		}
	}
	if _, err := session.CompleteAttachment("upload-1"); err != nil {
		t.Fatal(err)
	}
	frame, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "session-1", UploadID: "u", DeclaredTotal: 1, Final: true, Payload: []byte("x")}, key)
	if err != nil {
		t.Fatal(err)
	}
	frame[5] = 2
	if _, err := DecodeAttachmentFrame(frame, key); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("unknown flag error = %v", err)
	}
}

func TestDigestFailureReleasesAttachmentQuota(t *testing.T) {
	limits := DefaultAttachmentLimits()
	limits.MaxDeviceUploads = 1
	quota := NewAttachmentQuota(limits)
	key := []byte("0123456789abcdef0123456789abcdef")
	session := NewSessionWithAttachments(key, quota, "session-1", "phone-1")
	if err := session.OfferAttachment(AttachmentOffer{UploadID: "bad", DeclaredTotal: 1, SHA256: strings.Repeat("0", 64)}); err != nil {
		t.Fatal(err)
	}
	frame, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "session-1", UploadID: "bad", DeclaredTotal: 1, Final: true, Payload: []byte("x")}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AcceptAttachmentFrame(frame); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CompleteAttachment("bad"); !errors.Is(err, ErrInvalidAttachment) {
		t.Fatalf("digest mismatch error = %v", err)
	}
	if err := session.OfferAttachment(AttachmentOffer{UploadID: "next", DeclaredTotal: 1, SHA256: strings.Repeat("a", 64)}); err != nil {
		t.Fatalf("digest failure kept quota reserved: %v", err)
	}
}

func TestAttachmentCompleteMessageEnforcesDigestAndReleasesQuota(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	session := NewSessionWithAttachments(key, nil, "session-1", "phone-1")
	offer := AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 1, SHA256: "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"}
	if err := session.OfferAttachment(offer); err != nil {
		t.Fatal(err)
	}
	frame, err := EncodeAttachmentFrame(AttachmentChunk{SessionID: "session-1", UploadID: "upload-1", DeclaredTotal: 1, Final: true, Payload: []byte("x")}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.AcceptAttachmentFrame(frame); err != nil {
		t.Fatal(err)
	}
	complete := `{"version":{"major":1,"minor":0},"messageId":"complete-1","sender":"phone","type":"attachment_complete","body":{"uploadId":"upload-1"}}`
	if _, err := session.AcceptText([]byte(complete)); err != nil {
		t.Fatal(err)
	}
	ack, okay := session.CompletedAttachment("upload-1")
	if !okay || ack.ReceivedBytes != 1 || ack.SHA256 != offer.SHA256 {
		t.Fatalf("completed attachment = %#v, found=%v", ack, okay)
	}
}

func TestProductionRejectsEverySharedInvalidFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "protocol", "fixtures", "invalid", "schema-drift.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if _, err := DecodeText([]byte(line)); err == nil {
			t.Fatalf("invalid fixture line %d was accepted", lineNumber+1)
		}
	}
}

func TestActionResultStateCannotMoveBackwardAfterCrash(t *testing.T) {
	session := NewSession()
	for _, frame := range []string{snapshot("s-1", 8), actionResult("r-1", 9, "a-1", "queued"), actionResult("r-2", 10, "a-1", "confirmed")} {
		if _, err := session.AcceptText([]byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.AcceptText([]byte(actionResult("r-3", 11, "a-1", "sent"))); !errors.Is(err, ErrInvalidActionState) {
		t.Fatalf("terminal action transition error = %v", err)
	}
}

func TestKilledPhoneReconnectRequiresFreshSnapshot(t *testing.T) {
	lines := fixtureLines(t, "reconnect.jsonl")
	warm := NewSessionWithAttachments(nil, NewAttachmentQuota(DefaultAttachmentLimits()), "", "phone-1")
	retained := RetainedAttachment{
		Offer:    AttachmentOffer{UploadID: "upload-resume", DeclaredTotal: 4, SHA256: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"},
		Received: []byte("da"), NextChunk: 2, ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := warm.RestoreAttachment(retained); err != nil {
		t.Fatal(err)
	}
	for _, line := range lines[:4] {
		if _, err := warm.AcceptText([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if chunk, okay := warm.AcceptedResumeChunk("upload-resume"); !okay || chunk != 2 {
		t.Fatalf("warm upload resume chunk = %d, accepted=%v", chunk, okay)
	}
	cold := NewSession()
	for _, line := range lines[4:] {
		if _, err := cold.AcceptText([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if !cold.HasFreshSnapshot() || cold.LastSequence() != 30 {
		t.Fatalf("cold reconnect snapshot=%v sequence=%d", cold.HasFreshSnapshot(), cold.LastSequence())
	}
}

func TestUploadResumeUsesOnlyRetainedAuthenticatedState(t *testing.T) {
	claim := `{"version":{"major":1,"minor":0},"messageId":"resume","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","lastAck":1,"uploads":{"upload-1":2}}}}`
	unretained := NewSession()
	if _, err := unretained.AcceptText([]byte(claim)); err != nil {
		t.Fatal(err)
	}
	if _, okay := unretained.AcceptedResumeChunk("upload-1"); okay {
		t.Fatal("client-only resume claim was trusted")
	}

	retained := NewSessionWithAttachments(nil, NewAttachmentQuota(DefaultAttachmentLimits()), "", "phone-1")
	err := retained.RestoreAttachment(RetainedAttachment{
		Offer:    AttachmentOffer{UploadID: "upload-1", DeclaredTotal: 4, SHA256: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"},
		Received: []byte("da"), NextChunk: 2, ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retained.AcceptText([]byte(claim)); err != nil {
		t.Fatal(err)
	}
	if chunk, okay := retained.AcceptedResumeChunk("upload-1"); !okay || chunk != 2 {
		t.Fatalf("retained resume chunk = %d, accepted=%v", chunk, okay)
	}
}

func helloNoState(id string) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"phone","type":"hello","body":{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`
}
func welcome(id string) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":[],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}}`
}
func action(id, actionID string) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"phone","type":"action","body":{"actionId":"` + actionID + `","kind":"interrupt_turn","taskId":"task-1"}}`
}
func snapshot(id string, seq uint64) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"companion","type":"snapshot","seq":` + uintString(seq) + `,"body":{"baseSeq":` + uintString(seq) + `,"computerName":"Test computer","projects":[],"tasks":[]}}`
}
func event(id string, seq uint64) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"companion","type":"event","seq":` + uintString(seq) + `,"body":{"taskId":"task-1","event":"activity","state":"working","summary":"Working"}}`
}
func ack(id string, seq uint64) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"phone","type":"ack","body":{"throughSeq":` + uintString(seq) + `}}`
}
func actionResult(id string, seq uint64, actionID, state string) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"companion","type":"action_result","seq":` + uintString(seq) + `,"body":{"actionId":"` + actionID + `","state":"` + state + `"}}`
}
func uintString(value uint64) string { return fmt.Sprintf("%d", value) }

func fixtureLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "protocol", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}
