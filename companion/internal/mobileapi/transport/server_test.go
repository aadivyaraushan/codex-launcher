package transport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/codex-launcher/codex-launcher/companion/internal/attachments"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

func TestAttachmentSessionPersistsCompletesResumesAndCancels(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	key := []byte("0123456789abcdef0123456789abcdef")
	payload := []byte("attachment")
	digest := sha256.Sum256(payload)
	digestHex := fmt.Sprintf("%x", digest[:])
	store, err := attachments.Open(filepath.Join(t.TempDir(), "attachments"), attachments.DefaultLimits(), nil)
	if err != nil {
		t.Fatal(err)
	}

	firstProtocol := contract.NewSessionWithAttachments(key, nil, "session-1", "phone-1")
	first := newAttachmentSession("phone-1", firstProtocol, store, func() time.Time { return now })
	offer := phoneAttachmentMessage(t, firstProtocol, "offer-1", "attachment_offer", fmt.Sprintf(`{"uploadId":"upload-1","declaredTotal":%d,"sha256":"%s"}`, len(payload), digestHex))
	event, err := first.acceptText(offer)
	if err != nil || event.State != "accepted" || event.NextChunk != 0 || event.ReceivedBytes != 0 {
		t.Fatalf("offer event = %#v, %v", event, err)
	}
	frame, err := contract.EncodeAttachmentFrame(contract.AttachmentChunk{
		SessionID: "session-1", UploadID: "upload-1", Chunk: 0, Offset: 0, DeclaredTotal: int64(len(payload)), Payload: payload[:4],
	}, key)
	if err != nil || first.acceptBinary(frame) != nil {
		t.Fatalf("first chunk error = %v", err)
	}
	firstProtocol.Close()

	secondProtocol := contract.NewSessionWithAttachments(key, nil, "session-2", "phone-1")
	phoneAttachmentMessage(t, secondProtocol, "hello-2", "hello", `{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","lastAck":1,"uploads":{"upload-1":1}}}`)
	second := newAttachmentSession("phone-1", secondProtocol, store, func() time.Time { return now.Add(time.Second) })
	events, err := second.restoreRequested()
	if err != nil || len(events) != 1 || events[0].NextChunk != 1 || events[0].ReceivedBytes != 4 {
		t.Fatalf("resume events = %#v, %v", events, err)
	}
	frame, err = contract.EncodeAttachmentFrame(contract.AttachmentChunk{
		SessionID: "session-2", UploadID: "upload-1", Chunk: 1, Offset: 4, DeclaredTotal: int64(len(payload)), Final: true, Payload: payload[4:],
	}, key)
	if err != nil || second.acceptBinary(frame) != nil {
		t.Fatalf("resumed chunk error = %v", err)
	}
	complete := phoneAttachmentMessage(t, secondProtocol, "complete-1", "attachment_complete", `{"uploadId":"upload-1"}`)
	event, err = second.acceptText(complete)
	if err != nil || event.State != "complete" || event.ReceivedBytes != int64(len(payload)) || event.NextChunk != 2 {
		t.Fatalf("complete event = %#v, %v", event, err)
	}
	if resolved, err := store.Resolve("phone-1", []string{"upload-1"}, now); err != nil || len(resolved) != 1 {
		t.Fatalf("completed store state = %#v, %v", resolved, err)
	}

	cancelProtocol := contract.NewSessionWithAttachments(key, nil, "session-3", "phone-1")
	cancelSession := newAttachmentSession("phone-1", cancelProtocol, store, func() time.Time { return now })
	cancelOffer := phoneAttachmentMessage(t, cancelProtocol, "offer-2", "attachment_offer", fmt.Sprintf(`{"uploadId":"upload-2","declaredTotal":%d,"sha256":"%s"}`, len(payload), digestHex))
	if _, err := cancelSession.acceptText(cancelOffer); err != nil {
		t.Fatal(err)
	}
	cancel := phoneAttachmentMessage(t, cancelProtocol, "cancel-2", "attachment_cancel", `{"uploadId":"upload-2"}`)
	event, err = cancelSession.acceptText(cancel)
	if err != nil || event.State != "cancelled" || event.SHA256 != digestHex {
		t.Fatalf("cancel event = %#v, %v", event, err)
	}
	if _, err := store.Resume("phone-1", "upload-2", now); !errors.Is(err, attachments.ErrAttachmentUnavailable) {
		t.Fatalf("cancelled store state error = %v", err)
	}
}

func TestAttachmentSessionRejectsFalseResumeClaim(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	key := []byte("0123456789abcdef0123456789abcdef")
	payload := []byte("attachment")
	digest := sha256.Sum256(payload)
	store, err := attachments.Open(filepath.Join(t.TempDir(), "attachments"), attachments.DefaultLimits(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin("phone-1", "upload-1", int64(len(payload)), fmt.Sprintf("%x", digest[:]), now); err != nil {
		t.Fatal(err)
	}
	protocol := contract.NewSessionWithAttachments(key, nil, "session-2", "phone-1")
	phoneAttachmentMessage(t, protocol, "hello-2", "hello", `{"clientInstanceId":"phone-1","supportedMajors":[1],"resume":{"mode":"warm","lastAck":1,"uploads":{"upload-1":2}}}`)
	session := newAttachmentSession("phone-1", protocol, store, func() time.Time { return now })
	if _, err := session.restoreRequested(); !errors.Is(err, contract.ErrInvalidAttachment) {
		t.Fatalf("false resume claim error = %v", err)
	}
}

func phoneAttachmentMessage(t *testing.T, session *contract.Session, id, messageType, body string) contract.Message {
	t.Helper()
	frame := []byte(fmt.Sprintf(`{"version":{"major":1,"minor":0},"messageId":"%s","sender":"phone","type":"%s","body":%s}`, id, messageType, body))
	if _, err := contract.DecodeText(frame); err != nil {
		t.Fatalf("decode %s: %v; frame=%s", messageType, err, frame)
	}
	message, err := session.AcceptText(frame)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func TestPinnedTLSServerPairsAuthenticatesAndClosesARevokedPhone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pairingService, err := pairing.NewService(ctx, pairing.NewMemoryStore(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := pairingService.TLSCertificate(testNow)
	if err != nil {
		t.Fatal(err)
	}
	messages := make(chan contract.Message, 1)
	attachmentStore, err := attachments.Open(filepath.Join(t.TempDir(), "attachments"), attachments.DefaultLimits(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var attachmentSequence atomic.Uint64
	server, err := NewServerWithAttachments(pairingService, func(ctx context.Context, sender MessageSender, message contract.Message) error {
		messages <- message
		if message.Type == "hello" {
			if err := sender.Send(ctx, contract.Message{
				Version: contract.Version{Major: 1}, MessageID: "welcome-test", Sender: "companion", Type: "welcome",
				Body: json.RawMessage(`{"sessionId":"session-1","capabilities":[],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}`),
			}); err != nil {
				return err
			}
			sequence := uint64(1)
			attachmentSequence.Store(sequence)
			return sender.Send(ctx, contract.Message{
				Version: contract.Version{Major: 1}, MessageID: "snapshot-test", Sender: "companion", Type: "snapshot", Sequence: &sequence,
				Body: json.RawMessage(`{"baseSeq":1,"computerName":"Studio Mac","projects":[],"tasks":[]}`),
			})
		}
		return nil
	}, attachmentStore, func(ctx context.Context, sender MessageSender, event AttachmentEvent) error {
		body, err := json.Marshal(map[string]any{
			"uploadId": event.UploadID, "state": event.State, "receivedBytes": event.ReceivedBytes,
			"sha256": event.SHA256, "nextChunk": event.NextChunk,
		})
		if err != nil {
			return err
		}
		sequence := attachmentSequence.Add(1)
		return sender.Send(ctx, contract.Message{
			Version: contract.Version{Major: 1}, MessageID: fmt.Sprintf("attachment-ack-%d", sequence), Sender: "companion", Type: "attachment_ack", Sequence: &sequence, Body: body,
		})
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.now = func() time.Time { return testNow }
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener, certificate) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve shutdown: %v", err)
		}
	}()

	offer, err := pairingService.BeginPairing(pairing.PairingTarget{Host: "100.64.0.10", Port: 9443, Protocol: 1}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	phoneKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&phoneKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pairRequest := pairing.PairRequest{
		Secret: offer.Secret, Host: offer.Target.Host, Port: offer.Target.Port, Protocol: offer.Target.Protocol,
		HostPublicKey: offer.HostPublicKey, DeviceID: "pixel-9", DeviceName: "Pixel 9", DevicePublicKey: publicKey,
	}
	pairRequest.Signature = signPhone(t, phoneKey, pairing.PairingProofMessage(pairRequest))
	client := pinnedClient(t, certificate)
	baseURL := "https://" + listener.Addr().String()
	pairBody := pairRequestBodyFrom(pairRequest)
	encodedPairBody, err := json.Marshal(pairBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Post(baseURL+"/v1/pair", "application/json", bytes.NewReader(encodedPairBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("pair status = %d, body = %s", response.StatusCode, body)
	}
	var paired pairResponseBody
	if err := json.NewDecoder(response.Body).Decode(&paired); err != nil || paired.DeviceID != "pixel-9" || paired.PairingGeneration == "" {
		t.Fatalf("pair response = %#v, error = %v", paired, err)
	}

	connectionA, _, err := websocket.Dial(ctx, "wss://"+listener.Addr().String()+"/v1/session?deviceId=pixel-9&sessionId=session-a", &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	connectionB, _, err := websocket.Dial(ctx, "wss://"+listener.Addr().String()+"/v1/session?deviceId=pixel-9&sessionId=session-b", &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	var challengeA sessionChallengeBody
	var challengeB sessionChallengeBody
	if err := wsjson.Read(ctx, connectionA, &challengeA); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, connectionB, &challengeB); err != nil {
		t.Fatal(err)
	}
	substitutedProof := challengeB.toProof()
	substitutedProof.Signature = signPhone(t, phoneKey, pairing.SessionProofMessage(substitutedProof))
	if err := wsjson.Write(ctx, connectionA, sessionProofBodyFrom(substitutedProof)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connectionA.Read(ctx); err == nil {
		t.Fatal("socket A accepted socket B's valid signed challenge")
	}
	connectionA.CloseNow()
	connectionB.CloseNow()

	badConnection, _, err := websocket.Dial(ctx, "wss://"+listener.Addr().String()+"/v1/session?deviceId=pixel-9&sessionId=session-bad", &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	var badChallenge sessionChallengeBody
	if err := wsjson.Read(ctx, badConnection, &badChallenge); err != nil {
		t.Fatal(err)
	}
	badProof := badChallenge.toProof()
	badProof.Signature = make([]byte, 72)
	if err := wsjson.Write(ctx, badConnection, sessionProofBodyFrom(badProof)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := badConnection.Read(ctx); err == nil {
		t.Fatal("invalid phone proof authenticated")
	}
	badConnection.CloseNow()

	uninitializedConnection, _, err := websocket.Dial(ctx, "wss://"+listener.Addr().String()+"/v1/session?deviceId=pixel-9&sessionId=session-uninitialized", &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	var uninitializedChallenge sessionChallengeBody
	if err := wsjson.Read(ctx, uninitializedConnection, &uninitializedChallenge); err != nil {
		t.Fatal(err)
	}
	uninitializedProof := uninitializedChallenge.toProof()
	uninitializedProof.Signature = signPhone(t, phoneKey, pairing.SessionProofMessage(uninitializedProof))
	if err := wsjson.Write(ctx, uninitializedConnection, sessionProofBodyFrom(uninitializedProof)); err != nil {
		t.Fatal(err)
	}
	var uninitializedAuthenticated authenticatedBody
	if err := wsjson.Read(ctx, uninitializedConnection, &uninitializedAuthenticated); err != nil {
		t.Fatal(err)
	}
	firstAction := []byte(`{"version":{"major":1,"minor":0},"messageId":"m-before-hello","sender":"phone","type":"action","body":{"actionId":"a-before-hello","kind":"interrupt_turn","taskId":"task-1"}}`)
	if err := uninitializedConnection.Write(ctx, websocket.MessageText, firstAction); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		t.Fatalf("message reached handler before hello: %#v", message)
	case <-time.After(100 * time.Millisecond):
	}
	readContext, stopUninitializedRead := context.WithTimeout(ctx, time.Second)
	if _, _, err := uninitializedConnection.Read(readContext); err == nil {
		t.Fatal("session remained open after an action before hello")
	}
	stopUninitializedRead()
	uninitializedConnection.CloseNow()

	websocketURL := "wss://" + listener.Addr().String() + "/v1/session?deviceId=pixel-9&sessionId=session-1"
	connection, _, err := websocket.Dial(ctx, websocketURL, &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	var challenge sessionChallengeBody
	if err := wsjson.Read(ctx, connection, &challenge); err != nil {
		t.Fatal(err)
	}
	proof := challenge.toProof()
	proof.Signature = signPhone(t, phoneKey, pairing.SessionProofMessage(proof))
	if err := wsjson.Write(ctx, connection, sessionProofBodyFrom(proof)); err != nil {
		t.Fatal(err)
	}
	var authenticated authenticatedBody
	if err := wsjson.Read(ctx, connection, &authenticated); err != nil || authenticated.Type != "authenticated" || authenticated.DeviceID != "pixel-9" {
		t.Fatalf("authenticated response = %#v, error = %v", authenticated, err)
	}

	hello := []byte(`{"version":{"major":1,"minor":0},"messageId":"m-hello","sender":"phone","type":"hello","body":{"clientInstanceId":"pixel-9","supportedMajors":[1],"resume":{"mode":"no_local_state"}}}`)
	if err := connection.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		if message.MessageID != "m-hello" {
			t.Fatalf("message = %#v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("valid phone frame did not reach the handler")
	}
	messageType, outbound, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		t.Fatalf("outbound frame type = %v, error = %v", messageType, err)
	}
	welcome, err := contract.DecodeText(outbound)
	if err != nil || welcome.Type != "welcome" || welcome.Sender != "companion" {
		t.Fatalf("outbound welcome = %#v, error = %v", welcome, err)
	}
	messageType, outbound, err = connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		t.Fatalf("outbound snapshot frame type = %v, error = %v", messageType, err)
	}
	snapshot, err := contract.DecodeText(outbound)
	if err != nil || snapshot.Type != "snapshot" || snapshot.Sequence == nil || *snapshot.Sequence != 1 {
		t.Fatalf("outbound snapshot = %#v, error = %v", snapshot, err)
	}
	ack := []byte(`{"version":{"major":1,"minor":0},"messageId":"ack-snapshot","sender":"phone","type":"ack","body":{"throughSeq":1}}`)
	if err := connection.Write(ctx, websocket.MessageText, ack); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		if message.Type != "ack" {
			t.Fatalf("acknowledgement message = %#v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("phone acknowledgement of the sent snapshot did not reach the handler")
	}

	attachmentPayload := []byte("from the phone")
	attachmentDigest := sha256.Sum256(attachmentPayload)
	offerFrame := fmt.Sprintf(`{"version":{"major":1,"minor":0},"messageId":"offer-live","sender":"phone","type":"attachment_offer","body":{"uploadId":"upload-live","declaredTotal":%d,"sha256":"%x"}}`, len(attachmentPayload), attachmentDigest)
	if err := connection.Write(ctx, websocket.MessageText, []byte(offerFrame)); err != nil {
		t.Fatal(err)
	}
	accepted := readAttachmentAck(t, ctx, connection)
	if accepted.State != "accepted" || accepted.NextChunk != 0 || accepted.ReceivedBytes != 0 {
		t.Fatalf("accepted ack = %#v", accepted)
	}
	attachmentKeyInput := append(append([]byte(nil), proof.HostSignature...), proof.Signature...)
	attachmentKey := sha256.Sum256(attachmentKeyInput)
	binaryFrame, err := contract.EncodeAttachmentFrame(contract.AttachmentChunk{
		SessionID: "session-1", UploadID: "upload-live", DeclaredTotal: int64(len(attachmentPayload)), Final: true, Payload: attachmentPayload,
	}, attachmentKey[:])
	if err != nil || connection.Write(ctx, websocket.MessageBinary, binaryFrame) != nil {
		t.Fatalf("binary attachment send = %v", err)
	}
	completeFrame := []byte(`{"version":{"major":1,"minor":0},"messageId":"complete-live","sender":"phone","type":"attachment_complete","body":{"uploadId":"upload-live"}}`)
	if err := connection.Write(ctx, websocket.MessageText, completeFrame); err != nil {
		t.Fatal(err)
	}
	completed := readAttachmentAck(t, ctx, connection)
	if completed.State != "complete" || completed.ReceivedBytes != int64(len(attachmentPayload)) || completed.SHA256 != fmt.Sprintf("%x", attachmentDigest) {
		t.Fatalf("completed ack = %#v", completed)
	}
	resolved, err := attachmentStore.Resolve("pixel-9", []string{"upload-live"}, testNow)
	if err != nil || len(resolved) != 1 {
		t.Fatalf("durable attachment = %#v, %v", resolved, err)
	}

	if err := pairingService.Revoke(ctx, "pixel-9"); err != nil {
		t.Fatal(err)
	}
	readContext, stopRead := context.WithTimeout(ctx, 2*time.Second)
	defer stopRead()
	if _, _, err := connection.Read(readContext); err == nil {
		t.Fatal("revoked phone socket remained open")
	}
}

func readAttachmentAck(t *testing.T, ctx context.Context, connection *websocket.Conn) AttachmentEvent {
	t.Helper()
	messageType, frame, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		t.Fatalf("attachment ack frame type = %v, error = %v", messageType, err)
	}
	message, err := contract.DecodeText(frame)
	if err != nil || message.Type != "attachment_ack" {
		t.Fatalf("attachment ack message = %#v, %v", message, err)
	}
	var event AttachmentEvent
	if err := json.Unmarshal(message.Body, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestServerRejectsPlaintextAndAnUnknownDevice(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pairingService, _ := pairing.NewService(ctx, pairing.NewMemoryStore(), rand.Reader)
	certificate, _ := pairingService.TLSCertificate(testNow)
	server, _ := NewServer(pairingService, func(context.Context, MessageSender, contract.Message) error { return nil }, nil)
	server.now = func() time.Time { return testNow }
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener, certificate) }()
	defer func() { cancel(); _ = <-done }()

	plaintextClient := &http.Client{Timeout: time.Second}
	response, plaintextErr := plaintextClient.Get("http://" + listener.Addr().String() + "/v1/pair")
	if plaintextErr == nil {
		defer response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			t.Fatalf("plaintext request succeeded with %d", response.StatusCode)
		}
	}

	client := pinnedClient(t, certificate)
	connection, response, err := websocket.Dial(ctx, "wss://"+listener.Addr().String()+"/v1/session?deviceId=missing&sessionId=session-1", &websocket.DialOptions{HTTPClient: client})
	if err == nil {
		connection.CloseNow()
		t.Fatal("unknown device received a WebSocket upgrade")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("unknown-device response = %#v, error = %v", response, err)
	}
}

func TestServerDoesNotLogUnvalidatedIdentifiers(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	pairingService, err := pairing.NewServiceWithLogger(context.Background(), pairing.NewMemoryStore(), rand.Reader, logger)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(pairingService, func(context.Context, MessageSender, contract.Message) error { return nil }, logger)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/session?deviceId=%2FUsers%2Fprivate&sessionId=session-1%0Aforged%3Dtrue", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	logs := output.String()
	for _, forbidden := range []string{"/Users/private", "forged=true"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("transport log contains unvalidated identifier %q: %s", forbidden, logs)
		}
	}
}

func TestPairingBodyReadHasADeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pairingService, err := pairing.NewService(ctx, pairing.NewMemoryStore(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := pairingService.TLSCertificate(testNow)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(pairingService, func(context.Context, MessageSender, contract.Message) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.pairReadTimeout = 50 * time.Millisecond
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener, certificate) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve shutdown: %v", err)
		}
	}()

	rawConnection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer rawConnection.Close()
	tlsConfig := pinnedClient(t, certificate).Transport.(*http.Transport).TLSClientConfig.Clone()
	connection := tls.Client(rawConnection, tlsConfig)
	if err := connection.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := io.WriteString(connection, "POST /v1/pair HTTP/1.1\r\nHost: companion\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	_, readErr := bufio.NewReader(connection).ReadString('\n')
	if netError, ok := readErr.(net.Error); ok && netError.Timeout() {
		t.Fatalf("pairing body remained open until the client deadline after %s", time.Since(started))
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("pairing body deadline took %s, want at most 500ms", elapsed)
	}
}

func TestServerCapsPreauthenticationWork(t *testing.T) {
	pairingService, err := pairing.NewService(context.Background(), pairing.NewMemoryStore(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(pairingService, func(context.Context, MessageSender, contract.Message) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.preauthSlots = make(chan struct{}, 1)
	server.preauthSlots <- struct{}{}
	for _, target := range []string{"/v1/pair", "/v1/session?deviceId=missing&sessionId=session-1"} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader("{}"))
		if strings.HasPrefix(target, "/v1/session") {
			request = httptest.NewRequest(http.MethodGet, target, nil)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want %d", target, response.Code, http.StatusServiceUnavailable)
		}
	}
}

func TestConnectionLimitedListenerCapsBeforeTLSAndUnblocksOnClose(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newConnectionLimitedListener(base, 1)
	clientOne, err := net.Dial("tcp", base.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientOne.Close()
	serverOne, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}

	clientTwo, err := net.Dial("tcp", base.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientTwo.Close()
	type acceptResult struct {
		connection net.Conn
		error      error
	}
	second := make(chan acceptResult, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		second <- acceptResult{connection: connection, error: acceptErr}
	}()
	select {
	case result := <-second:
		if result.connection != nil {
			result.connection.Close()
		}
		t.Fatalf("second connection bypassed the limit: %v", result.error)
	case <-time.After(100 * time.Millisecond):
	}
	if err := serverOne.Close(); err != nil {
		t.Fatal(err)
	}
	var serverTwo net.Conn
	select {
	case result := <-second:
		if result.error != nil {
			t.Fatal(result.error)
		}
		serverTwo = result.connection
	case <-time.After(time.Second):
		t.Fatal("second connection did not start after the first closed")
	}
	defer serverTwo.Close()

	third := make(chan error, 1)
	go func() {
		_, acceptErr := listener.Accept()
		third <- acceptErr
	}()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-third:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("blocked accept error = %v, want %v", err, net.ErrClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("closing the listener did not unblock a capacity wait")
	}
}

var testNow = time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)

func signPhone(t *testing.T, key *ecdsa.PrivateKey, message []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(message)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signature
}

func pinnedClient(t *testing.T, certificate tls.Certificate) *http.Client {
	t.Helper()
	want := certificate.Leaf.RawSubjectPublicKeyInfo
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) != 1 || !bytes.Equal(state.PeerCertificates[0].RawSubjectPublicKeyInfo, want) {
			return x509.UnknownAuthorityError{}
		}
		return nil
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}, Timeout: 3 * time.Second}
}

func pairRequestBodyFrom(request pairing.PairRequest) pairRequestBody {
	return pairRequestBody{
		Secret: request.Secret, Host: request.Host, Port: request.Port, Protocol: request.Protocol, HostPublicKey: request.HostPublicKey,
		DeviceID: request.DeviceID, DeviceName: request.DeviceName,
		DevicePublicKey: base64.RawURLEncoding.EncodeToString(request.DevicePublicKey), Signature: base64.RawURLEncoding.EncodeToString(request.Signature),
	}
}

func sessionProofBodyFrom(proof pairing.SessionProof) sessionProofBody {
	return sessionProofBody{
		DeviceID: proof.DeviceID, SessionID: proof.SessionID, PairingGeneration: proof.PairingGeneration, Protocol: proof.Protocol,
		HostPublicKey: proof.HostPublicKey, Nonce: proof.Nonce, ExpiresAt: proof.ExpiresAt.Unix(),
		HostSignature: base64.RawURLEncoding.EncodeToString(proof.HostSignature), Signature: base64.RawURLEncoding.EncodeToString(proof.Signature),
	}
}

func (body sessionChallengeBody) toProof() pairing.SessionProof {
	hostSignature, _ := base64.RawURLEncoding.DecodeString(body.HostSignature)
	return pairing.SessionProof{
		DeviceID: body.DeviceID, SessionID: body.SessionID, PairingGeneration: body.PairingGeneration, Protocol: body.Protocol,
		HostPublicKey: body.HostPublicKey, Nonce: body.Nonce, ExpiresAt: time.Unix(body.ExpiresAt, 0), HostSignature: hostSignature,
	}
}
