package phoneruntime

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/localtrust"
)

func testEmulatorChainDER(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Android Keystore Software Attestation"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestReleasePendingViaAttestationRejectsEmulatorChainByDefault(t *testing.T) {
	rt, err := Open(context.Background(), Config{
		Root:          t.TempDir(),
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()
	if _, err := rt.CreateLocalPairOffer(); err != nil {
		t.Fatalf("offer: %v", err)
	}
	_, err = rt.ReleasePendingViaAttestation(localPairAttestRequest{
		Nonce:          "nonce",
		ChainDERBase64: []string{base64.StdEncoding.EncodeToString(testEmulatorChainDER(t))},
	})
	if !errors.Is(err, localtrust.ErrAttestChain) {
		t.Fatalf("production local-pair must reject emulator chain, got %v", err)
	}
}

func TestReleasePendingViaAttestationHatchPassesEmulatorChainVerify(t *testing.T) {
	rt, err := Open(context.Background(), Config{
		Root:                t.TempDir(),
		DisplayName:         "Operator phone",
		ListenAddress:       "127.0.0.1:0",
		AllowSoftwareAttest: true,
	}, Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()
	if _, err := rt.CreateLocalPairOffer(); err != nil {
		t.Fatalf("offer: %v", err)
	}
	_, err = rt.ReleasePendingViaAttestation(localPairAttestRequest{
		Nonce:          "nonce",
		ChainDERBase64: []string{base64.StdEncoding.EncodeToString(testEmulatorChainDER(t))},
	})
	if errors.Is(err, localtrust.ErrAttestChain) {
		t.Fatalf("AVD hatch should get past Google-root chain verify, got %v", err)
	}
	if err == nil {
		t.Fatal("self-signed leaf without an attestation extension must still fail parse, not succeed pairing")
	}
}
