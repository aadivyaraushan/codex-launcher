package relaybox

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDataLineTokenSafety is spec test D: a forged token is rejected, a
// valid token can only be redeemed once (reuse is rejected), and an expired
// token is rejected even though it was never redeemed.
func TestDataLineTokenSafety(t *testing.T) {
	fakeNow := time.Unix(1_800_000_000, 0)
	clock := &fakeClock{now: fakeNow}
	box, addr, pinnedKey := newTestBox(t, WithClock(clock.Now), WithTokenTTL(2*time.Second), WithPhoneWaitTimeout(3*time.Second))

	control := dialMacDoor(t, addr, pinnedKey)
	defer control.Close()
	if _, err := control.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("register: %v", err)
	}
	waitForControlLine(t, box)

	t.Run("forged token is rejected", func(t *testing.T) {
		attacker := dialMacDoor(t, addr, pinnedKey)
		defer attacker.Close()
		if _, err := attacker.Write([]byte("REDEEM not-a-real-token\n")); err != nil {
			t.Fatalf("redeem write: %v", err)
		}
		assertConnectionRejected(t, attacker)
	})

	t.Run("valid token redeems once, reuse is rejected", func(t *testing.T) {
		token := triggerSessionToken(t, box, control)

		redeemer := dialMacDoor(t, addr, pinnedKey)
		defer redeemer.Close()
		if _, err := redeemer.Write([]byte("REDEEM " + token + "\n")); err != nil {
			t.Fatalf("redeem write: %v", err)
		}
		reader := bufio.NewReader(redeemer)
		redeemer.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimRight(line, "\r\n") != "OK" {
			t.Fatalf("first redeem = %q, %v; want OK", line, err)
		}

		replay := dialMacDoor(t, addr, pinnedKey)
		defer replay.Close()
		if _, err := replay.Write([]byte("REDEEM " + token + "\n")); err != nil {
			t.Fatalf("replay redeem write: %v", err)
		}
		assertConnectionRejected(t, replay)
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		token := triggerSessionToken(t, box, control)
		clock.advance(3 * time.Second) // past the 2s TTL

		late := dialMacDoor(t, addr, pinnedKey)
		defer late.Close()
		if _, err := late.Write([]byte("REDEEM " + token + "\n")); err != nil {
			t.Fatalf("late redeem write: %v", err)
		}
		assertConnectionRejected(t, late)
	})
}

// triggerSessionToken makes a throwaway phone connection to box and reads
// the resulting "SESSION <token>" signal off control, returning the token.
func triggerSessionToken(t *testing.T, box *Box, control net.Conn) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen phone door: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	// Cancelling on return (via defer) would race the phone connection's
	// handlePhoneConn goroutine: ctx.Done() firing before the redeemer gets
	// to use the token would drop it out from under the test. Only cancel
	// when the whole test is done.
	t.Cleanup(cancel)
	go func() { _ = box.ServePhoneDoor(ctx, listener) }()
	phone, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial phone door: %v", err)
	}
	t.Cleanup(func() { _ = phone.Close() })

	reader := bufio.NewReader(control)
	control.SetReadDeadline(time.Now().Add(3 * time.Second))
	line := readLine(t, reader)
	control.SetReadDeadline(time.Time{})
	token, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "SESSION ")
	if !ok {
		t.Fatalf("expected a SESSION line, got %q", line)
	}
	return token
}

func assertConnectionRejected(t *testing.T, conn net.Conn) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 1)
	if _, err := conn.Read(buffer); err == nil {
		t.Fatal("connection was not rejected")
	}
}

// fakeClock lets the expiry test move time forward instantly instead of
// sleeping past a real TTL — the whole test runs in milliseconds. It is
// read from the box's goroutines and written from the test goroutine, so
// it needs its own lock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
