package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/relaybox"
)

// syncBuffer is a mutex-protected buffer so the test goroutine can read the
// banner while run's goroutine is still writing to it. A plain bytes.Buffer
// is not safe for that.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestRunRejectsEmptySecret is spec test M1 (part 1): the box must never
// come up without a registration secret, and it must fail before it ever
// touches the listeners it was given (proven here by passing nil listeners
// — a run that tried to Accept on them would panic, not error cleanly).
func TestRunRejectsEmptySecret(t *testing.T) {
	cfg := Config{
		Secret:          "",
		CertPath:        filepath.Join(t.TempDir(), "box.pem"),
		MacListenAddr:   "127.0.0.1:0",
		PhoneListenAddr: "127.0.0.1:0",
	}
	err := run(context.Background(), cfg, nil, nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error for an empty secret, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "secret") {
		t.Fatalf("expected the error to mention the missing secret, got %q", err)
	}
}

// TestRunRejectsEmptyCertPath is spec test M1 (part 2): same guard for a
// missing certificate path, since the box has nowhere to persist (or find)
// its identity without one.
func TestRunRejectsEmptyCertPath(t *testing.T) {
	cfg := Config{
		Secret:          "correct-secret",
		CertPath:        "",
		MacListenAddr:   "127.0.0.1:0",
		PhoneListenAddr: "127.0.0.1:0",
	}
	err := run(context.Background(), cfg, nil, nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error for an empty certificate path, got nil")
	}
}

// TestRunServesBothDoorsAndPrintsPinnedBanner is spec test M2+M3: a smoke
// test that run, given two real loopback listeners, (a) prints a startup
// banner containing the pinned public-key fingerprint and never the secret,
// (b) accepts a pinned TLS handshake plus REGISTER on the Mac door without
// closing the connection, (c) accepts a plain TCP connection on the phone
// door, and (d) shuts down cleanly (no error) when ctx is cancelled.
func TestRunServesBothDoorsAndPrintsPinnedBanner(t *testing.T) {
	const secret = "smoke-test-secret"
	certPath := filepath.Join(t.TempDir(), "box.pem")

	macListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen mac door: %v", err)
	}
	phoneListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen phone door: %v", err)
	}

	cfg := Config{
		Secret:          secret,
		CertPath:        certPath,
		MacListenAddr:   macListener.Addr().String(),
		PhoneListenAddr: phoneListener.Addr().String(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := &syncBuffer{}
	runDone := make(chan error, 1)
	go func() { runDone <- run(ctx, cfg, macListener, phoneListener, out) }()

	// run() writes its certificate file (and the banner that quotes its
	// fingerprint) before it starts serving. Poll briefly for the file to
	// exist instead of a fixed sleep, so the test is fast on a healthy run
	// and still correct if this machine happens to be slow.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, statErr := os.Stat(certPath); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for run to write the certificate file")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Load the same on-disk certificate (LoadOrCreateCertificate loads
	// rather than regenerates when the file already exists) to learn the
	// pinned public key run is actually serving with.
	cert, err := relaybox.LoadOrCreateCertificate(certPath, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("load certificate written by run: %v", err)
	}
	fingerprint := relaybox.PinnedFingerprint(cert)

	banner := waitForSubstring(t, out, fingerprint, 3*time.Second)
	if strings.Contains(banner, secret) {
		t.Fatalf("banner must never contain the secret, got: %s", banner)
	}
	if !strings.Contains(banner, "PINNED KEY") {
		t.Fatalf("expected banner to label the pinned key, got: %s", banner)
	}

	// (b) Mac door: pinned TLS handshake, then REGISTER. A connection that
	// is accepted (not rejected) stays open with no data flowing, so a
	// short read must time out — not fail with EOF/reset.
	macConn := dialPinnedTLS(t, macListener.Addr().String(), cert.Leaf.RawSubjectPublicKeyInfo)
	defer macConn.Close()
	if _, err := macConn.Write([]byte("REGISTER " + secret + "\n")); err != nil {
		t.Fatalf("write REGISTER: %v", err)
	}
	macConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buffer := make([]byte, 1)
	_, readErr := macConn.Read(buffer)
	var netErr net.Error
	if !(errors.As(readErr, &netErr) && netErr.Timeout()) {
		t.Fatalf("expected the accepted control line to stay open (read timeout), got err=%v", readErr)
	}

	// (c) Phone door: plain TCP connect must succeed.
	phoneConn, err := net.DialTimeout("tcp", phoneListener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial phone door: %v", err)
	}
	_ = phoneConn.Close()

	// (d) Cancelling ctx must shut both doors down cleanly.
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("expected a clean shutdown, got err=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for run to shut down after ctx cancellation")
	}
}

func waitForSubstring(t *testing.T, out *syncBuffer, substr string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		text := out.String()
		if strings.Contains(text, substr) {
			return text
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for output to contain %q; got: %s", substr, text)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// dialPinnedTLS opens a TLS connection to the box's Mac door, pinning the
// exact public key the way relayclient.Dial does, without importing
// relayclient (this binary has no business depending on the Mac's trust
// package).
func dialPinnedTLS(t *testing.T, addr string, pinnedKey []byte) *tls.Conn {
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
