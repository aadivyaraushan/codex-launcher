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
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

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
	server, err := NewServer(pairingService, func(ctx context.Context, sender MessageSender, message contract.Message) error {
		messages <- message
		if message.Type == "hello" {
			return sender.Send(ctx, contract.Message{
				Version: contract.Version{Major: 1}, MessageID: "welcome-test", Sender: "companion", Type: "welcome",
				Body: json.RawMessage(`{"sessionId":"session-1","capabilities":[],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900}}`),
			})
		}
		return nil
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

	if err := pairingService.Revoke(ctx, "pixel-9"); err != nil {
		t.Fatal(err)
	}
	readContext, stopRead := context.WithTimeout(ctx, 2*time.Second)
	defer stopRead()
	if _, _, err := connection.Read(readContext); err == nil {
		t.Fatal("revoked phone socket remained open")
	}
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
