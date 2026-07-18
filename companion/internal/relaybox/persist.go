package relaybox

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LoadOrCreateCertificate gives the box a stable TLS identity across
// restarts. Without this, every restart would mint a fresh key, and every
// Mac that had pinned the old public key (relayclient.Dial) would suddenly
// refuse to connect. The first call for a given path generates a
// certificate and writes it to disk; every call after that reads the same
// key back, so the box's identity — and the pin an operator copied into
// their Mac's config — stays valid.
func LoadOrCreateCertificate(path string, random io.Reader, now time.Time) (tls.Certificate, error) {
	if path == "" {
		return tls.Certificate{}, errors.New("relaybox: certificate path is required")
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		return decodePersistedCertificate(data)
	case errors.Is(err, os.ErrNotExist):
		return createAndPersistCertificate(path, random, now)
	default:
		return tls.Certificate{}, fmt.Errorf("relaybox: read certificate file %s: %w", path, err)
	}
}

// PinnedFingerprint returns the string an operator copies into the Mac's
// config as the box's pinned key identity: a SHA-256 hash of the box's
// public key, base64-encoded without padding so it is short enough to
// paste around. It is deterministic for a given key and does not depend on
// the certificate's serial number or validity window, only the key itself.
func PinnedFingerprint(cert tls.Certificate) string {
	if cert.Leaf == nil {
		return ""
	}
	sum := sha256.Sum256(cert.Leaf.RawSubjectPublicKeyInfo)
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

// PinnedPublicKeyBase64 returns the box's public key exactly as
// relayclient.Dial needs it to build its pin: relayclient pins connections
// by comparing the peer certificate's raw SubjectPublicKeyInfo bytes
// directly (see relayclient/dial.go's VerifyConnection callback), not a
// hash of them. So unlike PinnedFingerprint (a short hash meant for a human
// to eyeball), this is the literal bytes an operator must paste into the
// Mac's config for the pin to actually match on connect.
func PinnedPublicKeyBase64(cert tls.Certificate) string {
	if cert.Leaf == nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(cert.Leaf.RawSubjectPublicKeyInfo)
}

// createAndPersistCertificate generates a fresh box identity and writes it
// to path as PEM (private key, then certificate) before returning it, so a
// crash between generation and the next restart never loses the key that
// was already handed out as a pin.
func createAndPersistCertificate(path string, random io.Reader, now time.Time) (tls.Certificate, error) {
	cert, err := GenerateSelfSignedCertificate(random, now)
	if err != nil {
		return tls.Certificate{}, err
	}
	key, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return tls.Certificate{}, fmt.Errorf("relaybox: generated certificate key is %T, want *ecdsa.PrivateKey", cert.PrivateKey)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: marshal certificate key: %w", err)
	}

	var buffer bytes.Buffer
	if err := pem.Encode(&buffer, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: encode certificate key: %w", err)
	}
	if err := pem.Encode(&buffer, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}); err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: encode certificate: %w", err)
	}

	// 0700 on the directory and 0600 on the file: this PEM holds the box's
	// private key, so neither should be readable by anyone but the box
	// process's own user.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: create certificate directory: %w", err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: write certificate file %s: %w", path, err)
	}
	return cert, nil
}

// decodePersistedCertificate parses a certificate file written by
// createAndPersistCertificate. It explicitly parses and sets Leaf (rather
// than relying on tls.X509KeyPair's own Leaf population) to mirror how
// GenerateSelfSignedCertificate builds a certificate, and to keep this
// package's behavior independent of the GODEBUG=x509keypairleaf setting.
func decodePersistedCertificate(data []byte) (tls.Certificate, error) {
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: parse persisted certificate: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, errors.New("relaybox: persisted certificate file has no certificate block")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: parse persisted certificate leaf: %w", err)
	}
	cert.Leaf = leaf
	return cert, nil
}
