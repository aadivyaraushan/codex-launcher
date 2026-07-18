package relaybox

import (
	"bufio"
	"strings"
	"testing"
	"time"
)

// TestRegistrationRejectsWrongOrEmptySecret is spec test E (part 1): a
// registrant that does not present the box's exact secret must not become
// the control line. We prove that indirectly: register the real secret
// first, then try to hijack the slot with a wrong (and separately, empty)
// secret, and confirm the real control line still receives session
// signals afterward.
func TestRegistrationRejectsWrongOrEmptySecret(t *testing.T) {
	box, addr, pinnedKey := newTestBox(t, WithPhoneWaitTimeout(2*time.Second))

	good := dialMacDoor(t, addr, pinnedKey)
	defer good.Close()
	if _, err := good.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("register: %v", err)
	}
	waitForControlLine(t, box)

	for _, badSecret := range []string{"wrong-secret", ""} {
		attacker := dialMacDoor(t, addr, pinnedKey)
		if _, err := attacker.Write([]byte("REGISTER " + badSecret + "\n")); err != nil {
			t.Fatalf("attacker register write: %v", err)
		}
		buffer := make([]byte, 1)
		attacker.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := attacker.Read(buffer); err == nil {
			t.Fatalf("attacker with secret %q was not rejected", badSecret)
		}
		attacker.Close()
	}

	// The legitimate control line must still be the one in charge: a phone
	// arriving now should get a SESSION signal on `good`, not silence.
	signalPhoneArrival(t, box)

	reader := bufio.NewReader(good)
	good.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "SESSION ") {
		t.Fatalf("legitimate control line did not receive a session signal: line=%q err=%v", line, err)
	}
}

// TestCorrectSecretReregistrationEvictsPriorControlLine is spec test E
// (part 2): a second registrant presenting the *correct* secret takes the
// single control-line slot immediately. The old control line stops
// receiving SESSION signals; the new one receives them.
func TestCorrectSecretReregistrationEvictsPriorControlLine(t *testing.T) {
	box, addr, pinnedKey := newTestBox(t, WithPhoneWaitTimeout(2*time.Second))

	first := dialMacDoor(t, addr, pinnedKey)
	defer first.Close()
	if _, err := first.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Give the box a moment to install the first control line before the
	// second registrant shows up, so the eviction is unambiguous.
	time.Sleep(50 * time.Millisecond)

	second := dialMacDoor(t, addr, pinnedKey)
	defer second.Close()
	if _, err := second.Write([]byte("REGISTER correct-secret-value\n")); err != nil {
		t.Fatalf("second register: %v", err)
	}

	// The first connection must be closed by the box (evicted).
	first.SetReadDeadline(time.Now().Add(3 * time.Second))
	buffer := make([]byte, 1)
	if _, err := first.Read(buffer); err == nil {
		t.Fatal("evicted control line was not closed")
	}

	// A phone arriving now must be signalled on the second (current)
	// control line.
	signalPhoneArrival(t, box)

	reader := bufio.NewReader(second)
	second.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "SESSION ") {
		t.Fatalf("new control line did not receive a session signal: line=%q err=%v", line, err)
	}
}

func waitForControlLine(t *testing.T, box *Box) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		box.mu.Lock()
		ready := box.control != nil
		box.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("control line was never registered")
}
