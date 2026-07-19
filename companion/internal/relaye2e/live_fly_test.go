package relaye2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"net"
	"os"
	"testing"
	"time"
)

// TestLiveFlyMacDoorKeepsTheBoxPinnedTLSIdentity verifies the deployed public
// route, not Fly's private network. A TLS handler at Fly's edge would present a
// Fly certificate here and fail the exact public-key comparison.
func TestLiveFlyMacDoorKeepsTheBoxPinnedTLSIdentity(t *testing.T) {
	addr := os.Getenv("RELAYBOX_LIVE_MAC_ADDR")
	pinText := os.Getenv("RELAYBOX_LIVE_PIN")
	if addr == "" || pinText == "" {
		t.Skip("set RELAYBOX_LIVE_MAC_ADDR and RELAYBOX_LIVE_PIN to test the deployed box")
	}
	pin, err := base64.StdEncoding.DecodeString(pinText)
	if err != nil {
		t.Fatalf("decode RELAYBOX_LIVE_PIN: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var dialer net.Dialer
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("dial deployed Mac door %s: %v", addr, err)
	}
	defer raw.Close()

	conn := tls.Client(raw, &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	})
	if err := conn.HandshakeContext(ctx); err != nil {
		t.Fatalf("handshake with deployed Mac door %s: %v", addr, err)
	}
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) != 1 || !bytes.Equal(certs[0].RawSubjectPublicKeyInfo, pin) {
		t.Fatalf("deployed Mac door did not present the pinned box key (certificates = %d)", len(certs))
	}
}

// TestLiveFlyPhoneReachesCompanion runs the full production pairing and session
// protocol through the deployed Fly box. The box secret is accepted only from
// the process environment and is never logged by this test.
func TestLiveFlyPhoneReachesCompanion(t *testing.T) {
	macAddr := os.Getenv("RELAYBOX_LIVE_MAC_ADDR")
	phoneAddr := os.Getenv("RELAYBOX_LIVE_PHONE_ADDR")
	secret := os.Getenv("RELAYBOX_LIVE_SECRET")
	pinText := os.Getenv("RELAYBOX_LIVE_PIN")
	if macAddr == "" || phoneAddr == "" || secret == "" || pinText == "" {
		t.Skip("set all RELAYBOX_LIVE_* variables to test the deployed box end to end")
	}
	pin, err := base64.StdEncoding.DecodeString(pinText)
	if err != nil {
		t.Fatalf("decode RELAYBOX_LIVE_PIN: %v", err)
	}
	runPhoneReachesCompanionThroughRelayBox(t, &externalRelay{
		macDoorAddr:   macAddr,
		phoneDoorAddr: phoneAddr,
		secret:        secret,
		pinnedBoxSPKI: pin,
	})
}
