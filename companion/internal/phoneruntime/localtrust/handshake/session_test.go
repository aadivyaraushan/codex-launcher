package handshake_test

import (
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/localtrust/handshake"
)

func TestHappyPathOfferToAck(t *testing.T) {
	s := handshake.New("offer-1")
	var err error
	s, err = s.PinTLS()
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.AcceptAttestation()
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.Ack()
	if err != nil {
		t.Fatal(err)
	}
	if s.Phase != handshake.PhaseAcked {
		t.Fatalf("%s", s.Phase)
	}
}

func TestForgedServerCannotSkipTLS(t *testing.T) {
	s := handshake.New("offer-1")
	if _, err := s.AcceptAttestation(); err == nil {
		t.Fatal("expected illegal transition")
	}
}

func TestRejectEndsSession(t *testing.T) {
	s := handshake.New("offer-1").Reject("forged_spki")
	if s.Phase != handshake.PhaseRejected || s.Reason != "forged_spki" {
		t.Fatalf("%+v", s)
	}
}
