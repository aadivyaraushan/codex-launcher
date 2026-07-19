package relayclient

import (
	"bytes"
	"testing"
)

func TestControlCommandAnswersHeartbeatWithoutCreatingASession(t *testing.T) {
	var reply bytes.Buffer
	token, session, err := handleControlCommand(&reply, "PING")
	if err != nil {
		t.Fatalf("handle PING: %v", err)
	}
	if session || token != "" {
		t.Fatalf("PING produced a session token %q", token)
	}
	if got := reply.String(); got != "PONG\n" {
		t.Fatalf("heartbeat reply = %q, want PONG", got)
	}
}
