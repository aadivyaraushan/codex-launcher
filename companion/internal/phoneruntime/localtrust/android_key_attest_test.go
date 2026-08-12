package localtrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"
)

func mustSelfSignedCA(t *testing.T, cn string) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
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
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key, der
}

func mustLeafIssuedBy(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func emulatorLikeChain(t *testing.T) [][]byte {
	t.Helper()
	ca, caKey, caDER := mustSelfSignedCA(t, "Android Keystore Software Attestation")
	leafDER := mustLeafIssuedBy(t, ca, caKey, "attest-key")
	return [][]byte{leafDER, caDER}
}

func TestEmulatorChainRejectedByDefault(t *testing.T) {
	roots, _, err := LoadGoogleAttestationRoots()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = VerifyAndroidKeyAttestationChain(emulatorLikeChain(t), roots, AttestPolicy{})
	if !errors.Is(err, ErrAttestChain) {
		t.Fatalf("production path must reject emulator chain, got %v", err)
	}
}

func TestEmulatorChainAcceptedWithSoftwarePolicy(t *testing.T) {
	roots, _, err := LoadGoogleAttestationRoots()
	if err != nil {
		t.Fatal(err)
	}
	leaf, fp, err := VerifyAndroidKeyAttestationChain(emulatorLikeChain(t), roots, AttestPolicy{AllowSoftwareAttest: true})
	if err != nil {
		t.Fatalf("AVD hatch should accept emulator software chain: %v", err)
	}
	if leaf == nil {
		t.Fatal("expected leaf")
	}
	if fp == "" {
		t.Fatal("expected emulator root fingerprint")
	}
}

func TestEmulatorHatchStillRejectsBrokenChain(t *testing.T) {
	roots, _, err := LoadGoogleAttestationRoots()
	if err != nil {
		t.Fatal(err)
	}
	_, caKeyA, derA := mustSelfSignedCA(t, "ca-a")
	_, _, derB := mustSelfSignedCA(t, "ca-b")
	// Leaf from A presented with unrelated B as the "root".
	caA, err := x509.ParseCertificate(derA)
	if err != nil {
		t.Fatal(err)
	}
	leafDER := mustLeafIssuedBy(t, caA, caKeyA, "leaf")
	_, _, err = VerifyAndroidKeyAttestationChain([][]byte{leafDER, derB}, roots, AttestPolicy{AllowSoftwareAttest: true})
	if !errors.Is(err, ErrAttestChain) {
		t.Fatalf("hatch must not accept an unchained pair of certs, got %v", err)
	}
}

func TestEmptyChainRejectedEvenWithHatch(t *testing.T) {
	roots, _, err := LoadGoogleAttestationRoots()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = VerifyAndroidKeyAttestationChain(nil, roots, AttestPolicy{AllowSoftwareAttest: true})
	if !errors.Is(err, ErrAttestChain) {
		t.Fatalf("empty chain must stay invalid, got %v", err)
	}
}

func TestGoogleRootsRemainRequiredWithoutHatch(t *testing.T) {
	_, fps, err := LoadGoogleAttestationRoots()
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) == 0 {
		t.Fatal("embedded Google attestation roots must stay present")
	}
}

func TestFindContextTagBytesBounded(t *testing.T) {
	// Self-referential-looking SEQUENCE must not overflow.
	der := []byte{0x30, 0x04, 0x30, 0x02, 0x05, 0x00}
	_, _ = findContextTagBytes(der, 709)
}
