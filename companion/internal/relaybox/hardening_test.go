package relaybox

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

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
