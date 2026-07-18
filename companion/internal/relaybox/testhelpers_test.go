package relaybox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"
)

// dialMacDoor opens a pinned TLS connection to a Box's Mac door, exactly the
// way relayclient does, without depending on that package (relaybox must
// stay free of anything above it in the trust chain, so pulling relayclient
// in here would be backwards).
func dialMacDoor(t *testing.T, addr string, pinnedKey []byte) *tls.Conn {
	t.Helper()
	config := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) != 1 || !bytes.Equal(state.PeerCertificates[0].RawSubjectPublicKeyInfo, pinnedKey) {
				return errors.New("box certificate does not match pin")
			}
			return nil
		},
	}
	rawConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial mac door: %v", err)
	}
	conn := tls.Client(rawConn, config)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		t.Fatalf("mac door handshake: %v", err)
	}
	return conn
}

func readLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	return line
}

// newTestBox builds a Box plus a running Mac door listener for use by the
// registration/token tests, which drive the wire protocol directly instead
// of going through relayclient.
func newTestBox(t *testing.T, opts ...Option) (box *Box, macDoorAddr string, pinnedKey []byte) {
	t.Helper()
	cert, err := GenerateSelfSignedCertificate(rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("generate box certificate: %v", err)
	}
	box, err = New("correct-secret-value", cert, opts...)
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen mac door: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- box.ServeMacDoor(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("mac door shutdown: %v", err)
		}
	})
	return box, listener.Addr().String(), cert.Leaf.RawSubjectPublicKeyInfo
}

// signalPhoneArrival starts a throwaway phone-door listener for box, dials
// it once (as a phone would), and lets the resulting connection sit until
// the test's control-line reader has had a chance to observe the SESSION
// signal it triggers. It intentionally does not complete a handshake — the
// registration tests only care whether the control line was signalled.
func signalPhoneArrival(t *testing.T, box *Box) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen phone door: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = box.ServePhoneDoor(ctx, listener) }()
	t.Cleanup(func() { _ = listener.Close() })
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial phone door: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}
