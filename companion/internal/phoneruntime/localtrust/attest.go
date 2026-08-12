package localtrust

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
)

var (
	ErrAttestWrongApp       = errors.New("attestation: wrong application id")
	ErrAttestWrongSigner    = errors.New("attestation: wrong signing digest")
	ErrAttestUntrustedRoot  = errors.New("attestation: untrusted root")
	ErrAttestWrongChallenge = errors.New("attestation: wrong challenge")
	ErrAttestRevoked        = errors.New("attestation: revoked")
	ErrAttestWeakLevel      = errors.New("attestation: weak security level")
)

type SecurityLevel string

const (
	SecuritySoftware           SecurityLevel = "software"
	SecurityTrustedEnvironment SecurityLevel = "trusted_environment"
	SecurityStrongBox          SecurityLevel = "strongbox"
)

type AttestExpected struct {
	PackageName            string
	SigningCertSHA256      string
	OfferID                string
	RuntimeIdentity        string
	EphemeralPublicKey     string
	TLSSPKI                string
	TranscriptNonce        string
	PinnedRootFingerprints map[string]struct{}
	// AllowSoftwareAttest is the AVD/emulator hatch. The zero value is the
	// production Pixel policy: software Keystore attestations are rejected.
	AllowSoftwareAttest bool
}

// AttestPolicy controls chain verification. The zero value is fail-closed:
// the leaf must chain to an embedded Google hardware attestation root.
type AttestPolicy struct {
	// AllowSoftwareAttest accepts an emulator software-CA chain when the
	// Google-root path cannot be built. Never enable on release Pixel builds.
	AllowSoftwareAttest bool
}

type AttestObserved struct {
	PackageName       string
	SigningCertSHA256 string
	Challenge         []byte
	Level             SecurityLevel
	RootFingerprint   string
	Revoked           bool
}

func ChallengeBytes(offerID, runtimeIdentity, ephemeralPub, tlsSpki, nonce string) []byte {
	material := strings.Join([]string{offerID, runtimeIdentity, ephemeralPub, tlsSpki, nonce}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return sum[:]
}

func VerifyAttestation(expected AttestExpected, observed AttestObserved) error {
	if observed.Revoked {
		return ErrAttestRevoked
	}
	if observed.PackageName != expected.PackageName {
		return ErrAttestWrongApp
	}
	if !strings.EqualFold(observed.SigningCertSHA256, expected.SigningCertSHA256) {
		return ErrAttestWrongSigner
	}
	if _, ok := expected.PinnedRootFingerprints[observed.RootFingerprint]; !ok {
		return ErrAttestUntrustedRoot
	}
	want := ChallengeBytes(
		expected.OfferID,
		expected.RuntimeIdentity,
		expected.EphemeralPublicKey,
		expected.TLSSPKI,
		expected.TranscriptNonce,
	)
	if !bytes.Equal(observed.Challenge, want) {
		return ErrAttestWrongChallenge
	}
	if observed.Level == SecuritySoftware && !expected.AllowSoftwareAttest {
		return ErrAttestWeakLevel
	}
	return nil
}
