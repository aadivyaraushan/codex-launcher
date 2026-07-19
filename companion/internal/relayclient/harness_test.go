package relayclient_test

// This file builds a full, in-process relay network for the tunnel tests:
//
//	test-phone  --raw TCP-->  [ box phone door ]      (inner TLS passes through)
//	                                 |  relay (raw bytes)
//	     Mac app  <--pinned TLS--  [ box mac door ]  <--relayclient control+data
//
// The "Mac app" is a tiny HTTPS server wrapped in the Mac's own pairing TLS
// (the INNER layer). The relayclient package supplies the net.Listener it runs
// on. The test-phone dials the box's phone door with raw TCP and speaks the
// inner TLS itself, pinning the Mac's key — exactly like the real Android app.
//
// Nothing here imports the real pairing/identity packages; a throwaway
// self-signed cert stands in for the Mac's pairing identity, which is all the
// inner TLS layer needs to be a faithful end-to-end.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/relaybox"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
)

const testSecret = "correct-registration-secret"

// relayNetwork is a running box (both doors) plus the recorded view of every
// byte the box handled on the phone door — the exact bytes the box could
// inspect if it tried.
type relayNetwork struct {
	phoneDoorAddr string
	macDoorAddr   string
	boxPinnedKey  []byte // box leaf cert SPKI, what the Mac pins
	macCert       tls.Certificate
	macPinnedKey  []byte // Mac leaf cert SPKI, what the phone pins
	phoneDoorSeen *recordedBytes
}

// startRelayNetwork spins up a Box with a phone door and a Mac door on
// loopback, cancelling everything on test cleanup.
func startRelayNetwork(t *testing.T) *relayNetwork {
	return startRelayNetworkWithOptions(t)
}

func startRelayNetworkWithOptions(t *testing.T, options ...relaybox.Option) *relayNetwork {
	t.Helper()

	boxCert, err := relaybox.GenerateSelfSignedCertificate(rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("generate box certificate: %v", err)
	}
	macCert, err := relaybox.GenerateSelfSignedCertificate(rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("generate mac certificate: %v", err)
	}

	box, err := relaybox.New(testSecret, boxCert, options...)
	if err != nil {
		t.Fatalf("new box: %v", err)
	}

	macListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen mac door: %v", err)
	}
	rawPhoneListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen phone door: %v", err)
	}
	seen := &recordedBytes{}
	phoneListener := &recordingListener{inner: rawPhoneListener, seen: seen}

	ctx, cancel := context.WithCancel(context.Background())
	macDone := make(chan error, 1)
	phoneDone := make(chan error, 1)
	go func() { macDone <- box.ServeMacDoor(ctx, macListener) }()
	go func() { phoneDone <- box.ServePhoneDoor(ctx, phoneListener) }()
	t.Cleanup(func() {
		cancel()
		<-macDone
		<-phoneDone
	})

	return &relayNetwork{
		phoneDoorAddr: rawPhoneListener.Addr().String(),
		macDoorAddr:   macListener.Addr().String(),
		boxPinnedKey:  boxCert.Leaf.RawSubjectPublicKeyInfo,
		macCert:       macCert,
		macPinnedKey:  macCert.Leaf.RawSubjectPublicKeyInfo,
		phoneDoorSeen: seen,
	}
}

// startMacApp registers the Mac with the box via relayclient and runs a tiny
// HTTPS server (the inner TLS layer, using the Mac's pairing cert) on the
// listener relayclient provides. handler decides the response.
func startMacApp(t *testing.T, network *relayNetwork, handler http.Handler) {
	startMacAppWithReconnectDelay(t, network, handler, 0)
}

func startMacAppWithReconnectDelay(t *testing.T, network *relayNetwork, handler http.Handler, reconnectDelay time.Duration) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	listener, err := relayclient.Listen(ctx, relayclient.Config{
		BoxAddr:         network.macDoorAddr,
		Secret:          testSecret,
		PinnedPublicKey: network.boxPinnedKey,
		ReconnectDelay:  reconnectDelay,
	})
	if err != nil {
		t.Fatalf("relayclient listen/register: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	innerListener := tls.NewListener(listener, &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{network.macCert},
		NextProtos:   []string{"http/1.1"},
	})
	server := &http.Server{Handler: handler}
	serveDone := make(chan struct{})
	go func() {
		_ = server.Serve(innerListener)
		close(serveDone)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		<-serveDone
	})
}

// phoneClient returns an http.Client that reaches the Mac app the way the real
// phone does: raw TCP to the box's phone door, then inner TLS pinning the Mac's
// key. The URL host is ignored — every request goes to the phone door.
func phoneClient(network *relayNetwork) *http.Client {
	dialPhoneDoor := func(ctx context.Context, _ /*network*/, _ /*addr*/ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", network.phoneDoorAddr)
	}
	transport := &http.Transport{
		DialContext:       dialPhoneDoor,
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
			NextProtos:         []string{"http/1.1"},
			VerifyConnection:   pinChecker(network.macPinnedKey),
		},
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

// phoneDo issues a phone request, retrying briefly on a connection-level
// failure. Right after the Mac registers there is a small window where the box
// has accepted the control-line connection but has not yet read its REGISTER
// line, so a phone that races in is (correctly) dropped with no_control_line.
// A real phone simply reconnects; retrying here keeps the tunnel tests
// deterministic under heavy parallel load without weakening any assertion — a
// dropped attempt returns a transport error, never a wrong status or body.
func phoneDo(t *testing.T, send func() (*http.Response, error)) *http.Response {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := send()
		if err == nil {
			return resp
		}
		if time.Now().After(deadline) {
			t.Fatalf("phone request through box failed after retries: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// pinChecker returns a VerifyConnection callback that accepts exactly the leaf
// whose SubjectPublicKeyInfo matches want, with no certificate-authority
// fallback — the same shape both the Mac (pinning the box) and the phone
// (pinning the Mac) use.
func pinChecker(want []byte) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) != 1 {
			return errors.New("expected exactly one peer certificate")
		}
		if !bytes.Equal(state.PeerCertificates[0].RawSubjectPublicKeyInfo, want) {
			return errors.New("peer certificate does not match pin")
		}
		return nil
	}
}

// recordedBytes accumulates every byte the box read from and wrote to the phone
// door, under a mutex so the test can read it after the exchange.
type recordedBytes struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *recordedBytes) append(p []byte) {
	r.mu.Lock()
	r.buf.Write(p)
	r.mu.Unlock()
}

func (r *recordedBytes) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.buf.Bytes()...)
}

// recordingListener wraps the phone-door listener so every accepted connection
// records the bytes the box handles on it.
type recordingListener struct {
	inner net.Listener
	seen  *recordedBytes
}

func (l *recordingListener) Accept() (net.Conn, error) {
	conn, err := l.inner.Accept()
	if err != nil {
		return nil, err
	}
	return &recordingConn{Conn: conn, seen: l.seen}, nil
}

func (l *recordingListener) Close() error   { return l.inner.Close() }
func (l *recordingListener) Addr() net.Addr { return l.inner.Addr() }

// recordingConn records both directions: Read is what the phone sent the box,
// Write is what the box sent the phone. Both are on the wire the box terminates
// nothing on, so both must stay ciphertext.
type recordingConn struct {
	net.Conn
	seen *recordedBytes
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.seen.append(p[:n])
	}
	return n, err
}

func (c *recordingConn) Write(p []byte) (int, error) {
	if len(p) > 0 {
		c.seen.append(p)
	}
	return c.Conn.Write(p)
}
