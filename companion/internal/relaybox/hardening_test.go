package relaybox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPhoneDoorCapsPendingConnectionsAndRequiresPromptClientBytes(t *testing.T) {
	box, macAddr, pinnedKey := newTestBox(t,
		WithMaxPendingPhones(1),
		WithPhonePrefaceTimeout(80*time.Millisecond),
		WithPhoneRateLimits(100, 100, time.Minute),
	)
	control := dialMacDoor(t, macAddr, pinnedKey)
	defer control.Close()
	if _, err := control.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("register control line: %v", err)
	}
	reader := bufio.NewReader(control)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen phone door: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = box.ServePhoneDoor(ctx, listener) }()

	idle, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial idle phone: %v", err)
	}
	defer idle.Close()
	_ = control.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, err := reader.ReadString('\n'); err == nil {
		t.Fatal("an idle phone consumed a Mac slot before sending any TLS bytes")
	}

	nonTLS, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial non-TLS phone: %v", err)
	}
	defer nonTLS.Close()
	_, _ = nonTLS.Write([]byte("GE"))
	_ = control.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if line, err := reader.ReadString('\n'); err == nil {
		t.Fatalf("non-TLS preface consumed a Mac slot: %q", line)
	}

	first, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial first phone: %v", err)
	}
	defer first.Close()
	if _, err := first.Write([]byte{0x16, 0x03}); err != nil {
		t.Fatalf("write first phone preface: %v", err)
	}
	_ = control.SetReadDeadline(time.Now().Add(time.Second))
	if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, "SESSION ") {
		t.Fatalf("first phone SESSION = %q, %v", line, err)
	}

	second, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial second phone: %v", err)
	}
	defer second.Close()
	_, _ = second.Write([]byte{0x16, 0x03})
	_ = control.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if line, err := reader.ReadString('\n'); err == nil {
		t.Fatalf("pending cap allowed a second SESSION: %q", line)
	}
}

func TestPhoneDoorRateLimitsOneClientWithoutBlockingAnother(t *testing.T) {
	limiter := newConnectionLimiter(1, 2, time.Minute)
	now := time.Unix(100, 0)
	if !limiter.allow("203.0.113.10", now) {
		t.Fatal("first client connection was rejected")
	}
	if limiter.allow("203.0.113.10", now) {
		t.Fatal("same client exceeded its per-IP limit")
	}
	if !limiter.allow("198.51.100.7", now) {
		t.Fatal("a different client was blocked before the global limit")
	}
	if limiter.allow("192.0.2.8", now) {
		t.Fatal("global connection limit was not enforced")
	}
	if !limiter.allow("203.0.113.10", now.Add(time.Minute)) {
		t.Fatal("rate limit did not reset after its window")
	}
}

func TestControlHeartbeatClosesAStaleRegistration(t *testing.T) {
	box, macAddr, pinnedKey := newTestBox(t, WithControlHeartbeat(20*time.Millisecond, 70*time.Millisecond))
	control := dialMacDoor(t, macAddr, pinnedKey)
	defer control.Close()
	if _, err := control.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("register control line: %v", err)
	}
	waitForControlLine(t, box)

	reader := bufio.NewReader(control)
	_ = control.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if line, err := reader.ReadString('\n'); err != nil || line != "PING\n" {
		t.Fatalf("heartbeat = %q, %v; want PING", line, err)
	}
	for {
		if _, err := reader.ReadString('\n'); err != nil {
			break
		}
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		box.mu.Lock()
		registered := box.control != nil
		box.mu.Unlock()
		if !registered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale control line remained registered after heartbeat timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNewRejectsWeakSecret proves the box refuses a registration secret too
// short to resist guessing. With the Tailscale network gate gone, this secret
// is the only thing standing between "my Mac" and anyone else claiming the
// control line on a public box, so a one- or five-character secret must not be
// allowed to become that gate.
func TestNewRejectsWeakSecret(t *testing.T) {
	cert, err := GenerateSelfSignedCertificate(rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}

	if _, err := New("short", cert); !errors.Is(err, ErrWeakSecret) {
		t.Fatalf("New with a 5-char secret: err = %v, want ErrWeakSecret", err)
	}

	atMinimum := strings.Repeat("a", MinSecretLength)
	if _, err := New(atMinimum, cert); err != nil {
		t.Fatalf("New with a %d-char secret should be accepted, got %v", MinSecretLength, err)
	}
}

// TestMacDoorRejectsOverlongHeaderQuickly proves the box cannot be made to
// buffer without bound on its public Mac door. A client that streams a header
// line with no newline must be dropped as soon as the header exceeds the byte
// cap — not held open until the 5-second header timeout — so an unbounded read
// can never grow memory per connection. The timing assertion is what
// distinguishes a real byte cap (drops in well under a second) from merely
// waiting out the header deadline.
func TestMacDoorRejectsOverlongHeaderQuickly(t *testing.T) {
	_, addr, pinnedKey := newTestBox(t)
	conn := dialMacDoor(t, addr, pinnedKey)
	defer func() { _ = conn.Close() }()

	flood := bytes.Repeat([]byte("A"), maxMacDoorHeaderBytes+4096)
	if _, err := conn.Write(flood); err != nil {
		// A partial write is fine: the box may have already closed on us once
		// the cap tripped. What matters is the close happens fast, checked next.
		t.Logf("write to mac door returned %v (box may have closed early)", err)
	}

	start := time.Now()
	// Deadline below the box's 5s header timeout: if the box only closes when
	// that timeout fires, this read returns a timeout at ~4s and the elapsed
	// check below fails, which is exactly the unbounded-read regression we
	// want to catch.
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("mac door returned data for an overlong header; it should have closed the connection")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("mac door took %v to reject an overlong header; the byte cap is not enforced", elapsed)
	}
}
