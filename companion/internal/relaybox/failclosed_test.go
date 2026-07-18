package relaybox

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// TestPhoneDroppedWhenNoControlLine is spec test 3 (fail-closed), part 1: if
// no Mac has registered a control line, a phone that shows up at the phone
// door must be dropped *promptly* — not left hanging on a connection that
// will never be served. A box that quietly holds the phone open forever is
// the silent-failure mode the plan calls out; this proves the box fails
// loudly (closes the connection) instead.
func TestPhoneDroppedWhenNoControlLine(t *testing.T) {
	box, _, _ := newTestBox(t) // Mac door is up, but nobody registers.
	phoneAddr := startPhoneDoor(t, box)

	conn, err := net.Dial("tcp", phoneAddr)
	if err != nil {
		t.Fatalf("dial phone door: %v", err)
	}
	defer conn.Close()

	// The box should close our connection almost immediately. We give it a
	// generous 2s: if the read returns a deadline-exceeded error instead of a
	// real close, the box *hung* the phone rather than dropping it.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("box did not drop the phone when no control line was registered")
	} else if errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("box hung the phone instead of dropping it (no fail-closed): %v", err)
	}
}

// TestPhoneDroppedWhenMacNeverRedeems is spec test 3 (fail-closed), part 2: a
// control line is registered, so the box mints a token and signals it — but
// the Mac never redeems it. The box must give up after its phone-wait timeout
// and drop the phone, rather than holding the connection open indefinitely.
func TestPhoneDroppedWhenMacNeverRedeems(t *testing.T) {
	box, macAddr, pinnedKey := newTestBox(t, WithPhoneWaitTimeout(300*time.Millisecond))
	phoneAddr := startPhoneDoor(t, box)

	// Register a control line, but deliberately never act on its SESSION
	// signals (never REDEEM).
	control := dialMacDoor(t, macAddr, pinnedKey)
	defer control.Close()
	if _, err := control.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("register: %v", err)
	}
	waitForControlLine(t, box)

	conn, err := net.Dial("tcp", phoneAddr)
	if err != nil {
		t.Fatalf("dial phone door: %v", err)
	}
	defer conn.Close()

	// Confirm the box really did mint+signal a token (so we know we're testing
	// the redeem-wait path, not the no-control-line path).
	controlReader := bufio.NewReader(control)
	_ = control.SetReadDeadline(time.Now().Add(2 * time.Second))
	if line, err := controlReader.ReadString('\n'); err != nil || !strings.HasPrefix(line, "SESSION ") {
		t.Fatalf("expected a SESSION signal for the arriving phone: line=%q err=%v", line, err)
	}

	// With a 300ms wait timeout, the phone must be dropped well within 2s.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("box did not drop the phone after the redeem-wait timeout")
	} else if errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("box hung the phone past its redeem-wait timeout: %v", err)
	}
}

// startPhoneDoor runs box.ServePhoneDoor on a fresh loopback listener and
// returns its address, cleaning up on test end.
func startPhoneDoor(t *testing.T, box *Box) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen phone door: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- box.ServePhoneDoor(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return listener.Addr().String()
}
