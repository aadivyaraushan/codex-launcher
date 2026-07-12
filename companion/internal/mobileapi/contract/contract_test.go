package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestContractRejectsMalformedActionsPathsAndOversizedFrames(t *testing.T) {
	for _, test := range []struct {
		name, frame string
		want        error
	}{
		{name: "malformed approval", frame: `{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"approval","taskId":"task-1","decision":"yes"}}`, want: ErrInvalidAction},
		{name: "relative project", frame: `{"version":{"major":1,"minor":0},"messageId":"m-1","sender":"phone","type":"action","body":{"actionId":"a-1","kind":"set_project","projectPath":"../private"}}`, want: ErrUnsafeProjectPath},
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

func TestAttachmentChunksRequireOrderSizeDigestAndAuthentication(t *testing.T) {
	session := NewSession()
	valid := AttachmentChunk{UploadID: "upload-1", Chunk: 0, DeclaredTotal: 4, Payload: []byte("data"), SHA256: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7", AuthTag: strings.Repeat("a", 64)}
	if err := session.AcceptAttachment(valid); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []AttachmentChunk{
		{UploadID: "upload-2", Chunk: 1, DeclaredTotal: 4, Payload: []byte("data"), SHA256: valid.SHA256, AuthTag: valid.AuthTag},
		{UploadID: "upload-3", Chunk: 0, DeclaredTotal: 3, Payload: []byte("data"), SHA256: valid.SHA256, AuthTag: valid.AuthTag},
		{UploadID: "upload-4", Chunk: 0, DeclaredTotal: 4, Payload: []byte("data"), SHA256: strings.Repeat("0", 64), AuthTag: valid.AuthTag},
		{UploadID: "upload-5", Chunk: 0, DeclaredTotal: 4, Payload: []byte("data"), SHA256: valid.SHA256},
	} {
		if err := session.AcceptAttachment(changed); !errors.Is(err, ErrInvalidAttachment) {
			t.Fatalf("AcceptAttachment(%#v) error = %v", changed, err)
		}
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
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"companion","type":"snapshot","seq":` + uintString(seq) + `,"body":{"baseSeq":` + uintString(seq) + `,"tasks":[]}}`
}
func event(id string, seq uint64) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"companion","type":"event","seq":` + uintString(seq) + `,"body":{"taskId":"task-1","event":"activity","state":"working","summary":"Working"}}`
}
func ack(id string, seq uint64) string {
	return `{"version":{"major":1,"minor":0},"messageId":"` + id + `","sender":"phone","type":"ack","body":{"throughSeq":` + uintString(seq) + `}}`
}
func uintString(value uint64) string { return fmt.Sprintf("%d", value) }
