package relaybox

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestLoadOrCreateCertificateCreatesFileWhenMissing is spec test P1: the
// first call on a path that does not exist yet must generate a fresh
// certificate, write it to disk (creating any missing parent directories),
// and return a usable certificate with its Leaf populated.
func TestLoadOrCreateCertificateCreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	// Nest the path under a directory that does not exist yet, so this test
	// also proves LoadOrCreateCertificate creates missing parent directories.
	path := filepath.Join(dir, "nested", "box.pem")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("precondition: expected %s to not exist, stat err=%v", path, err)
	}

	cert, err := LoadOrCreateCertificate(path, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("LoadOrCreateCertificate: %v", err)
	}
	if cert.Leaf == nil {
		t.Fatal("expected Leaf to be populated on a freshly created certificate")
	}
	if len(cert.Leaf.RawSubjectPublicKeyInfo) == 0 {
		t.Fatal("expected a non-empty public key in the freshly created certificate")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected certificate file to be created at %s: %v", path, err)
	}
}

// TestLoadOrCreateCertificatePersistsAcrossCalls is spec test P2: a second
// call against the same path must load the same key back rather than
// generating a new one, so the box's identity (and the Mac's pin) survives a
// restart.
func TestLoadOrCreateCertificatePersistsAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "box.pem")

	first, err := LoadOrCreateCertificate(path, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("first LoadOrCreateCertificate: %v", err)
	}
	second, err := LoadOrCreateCertificate(path, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("second LoadOrCreateCertificate: %v", err)
	}

	firstFingerprint := PinnedFingerprint(first)
	secondFingerprint := PinnedFingerprint(second)
	if firstFingerprint == "" {
		t.Fatal("expected a non-empty fingerprint")
	}
	if firstFingerprint != secondFingerprint {
		t.Fatalf("expected the same identity across loads, got %q then %q", firstFingerprint, secondFingerprint)
	}
}

// TestPinnedFingerprintStableAndDistinct is spec test P3: PinnedFingerprint
// must be deterministic for a given key (so an operator can copy it once
// into the Mac's config) and must differ between two independently
// generated boxes (so it is actually identifying something).
func TestPinnedFingerprintStableAndDistinct(t *testing.T) {
	pathA := filepath.Join(t.TempDir(), "a.pem")
	pathB := filepath.Join(t.TempDir(), "b.pem")

	certA, err := LoadOrCreateCertificate(pathA, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("LoadOrCreateCertificate A: %v", err)
	}
	certB, err := LoadOrCreateCertificate(pathB, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("LoadOrCreateCertificate B: %v", err)
	}

	// Stability: fingerprinting the same loaded certificate twice must give
	// the same string both times.
	if PinnedFingerprint(certA) != PinnedFingerprint(certA) {
		t.Fatal("expected PinnedFingerprint to be deterministic for the same certificate")
	}

	// Reload A from disk and confirm the fingerprint survives the round trip.
	reloadedA, err := LoadOrCreateCertificate(pathA, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("reload LoadOrCreateCertificate A: %v", err)
	}
	if PinnedFingerprint(certA) != PinnedFingerprint(reloadedA) {
		t.Fatal("expected fingerprint to survive a reload from disk")
	}

	// Distinctness: two independently generated boxes must not collide.
	if PinnedFingerprint(certA) == PinnedFingerprint(certB) {
		t.Fatal("expected two independently generated certificates to have different fingerprints")
	}
}

// TestLoadOrCreateCertificateFilePermissions is spec test P4: the persisted
// file holds the box's private key, so it must not be group- or
// world-readable.
// TestPinnedPublicKeyBase64RoundTripsToRawSubjectPublicKeyInfo is the pin-
// format invariant the Mac side depends on: relayclient.Dial pins on the
// certificate's raw SubjectPublicKeyInfo bytes directly (not a hash of
// them), so whatever PinnedPublicKeyBase64 prints must decode back to
// exactly those bytes, or a pin an operator pastes into the Mac's config
// would never match.
func TestPinnedPublicKeyBase64RoundTripsToRawSubjectPublicKeyInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "box.pem")
	cert, err := LoadOrCreateCertificate(path, rand.Reader, time.Now())
	if err != nil {
		t.Fatalf("LoadOrCreateCertificate: %v", err)
	}

	encoded := PinnedPublicKeyBase64(cert)
	if encoded == "" {
		t.Fatal("expected a non-empty pinned public key")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("PinnedPublicKeyBase64 did not produce valid standard base64: %v", err)
	}
	if !bytes.Equal(decoded, cert.Leaf.RawSubjectPublicKeyInfo) {
		t.Fatalf("decoded pinned key does not match the certificate's raw SubjectPublicKeyInfo")
	}
}

// TestPinnedPublicKeyBase64EmptyWithoutLeaf mirrors PinnedFingerprint's
// existing nil-Leaf behavior: no leaf means no key to report, not a panic.
func TestPinnedPublicKeyBase64EmptyWithoutLeaf(t *testing.T) {
	if got := PinnedPublicKeyBase64(tls.Certificate{}); got != "" {
		t.Fatalf("PinnedPublicKeyBase64 with nil Leaf = %q, want empty", got)
	}
}

func TestLoadOrCreateCertificateFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permission bits are not meaningful on windows")
	}
	path := filepath.Join(t.TempDir(), "box.pem")
	if _, err := LoadOrCreateCertificate(path, rand.Reader, time.Now()); err != nil {
		t.Fatalf("LoadOrCreateCertificate: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat certificate file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected certificate file to be 0600, got %o", perm)
	}
}
