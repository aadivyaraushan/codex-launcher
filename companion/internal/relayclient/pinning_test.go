package relayclient_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/relaybox"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
)

// TestDialAcceptsPinnedBoxKey is the positive half of spec test C: dialing the
// real box with its real public key pinned succeeds.
func TestDialAcceptsPinnedBoxKey(t *testing.T) {
	network := startRelayNetwork(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := relayclient.Dial(ctx, network.macDoorAddr, network.boxPinnedKey)
	if err != nil {
		t.Fatalf("dial with correct pin should succeed, got: %v", err)
	}
	_ = conn.Close()
}

// TestDialRejectsWrongBoxKey is spec test C proper: pinning is the ONLY trust
// anchor and there is no certificate-authority fallback. A box presenting any
// key other than the pinned one is rejected at the handshake, even though the
// cert is a perfectly valid self-signed certificate a CA-based check might be
// coaxed into a different failure on. This is what stops a machine-in-the-
// middle that swapped the box.
func TestDialRejectsWrongBoxKey(t *testing.T) {
	network := startRelayNetwork(t)

	// A different, unrelated self-signed key: stands in for an impostor box.
	impostorCert, err := relaybox.GenerateSelfSignedCertificate(rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("generate impostor certificate: %v", err)
	}
	wrongPin := impostorCert.Leaf.RawSubjectPublicKeyInfo

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := relayclient.Dial(ctx, network.macDoorAddr, wrongPin)
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial with wrong pin must fail, but it succeeded: pinning is not enforced")
	}
}
