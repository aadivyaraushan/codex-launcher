package localtrust

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestChallengeBytesStable(t *testing.T) {
	a := ChallengeBytes("o", "r", "e", "p", "n")
	b := ChallengeBytes("o", "r", "e", "p", "n")
	if hex.EncodeToString(a) != hex.EncodeToString(b) {
		t.Fatalf("challenge not stable")
	}
	if len(a) != sha256.Size {
		t.Fatalf("want sha256 length, got %d", len(a))
	}
}

func TestRejectWrongPackageBeforeSecret(t *testing.T) {
	exp := AttestExpected{
		PackageName:            "app.codexlauncher",
		SigningCertSHA256:      "aa11",
		OfferID:                "offer-1",
		RuntimeIdentity:        "rid",
		EphemeralPublicKey:     "epk",
		TLSSPKI:                "pin",
		TranscriptNonce:        "nonce",
		PinnedRootFingerprints: map[string]struct{}{"root-avd": {}},
	}
	obs := AttestObserved{
		PackageName:       "com.evil",
		SigningCertSHA256: "aa11",
		Challenge:         ChallengeBytes("offer-1", "rid", "epk", "pin", "nonce"),
		Level:             SecurityTrustedEnvironment,
		RootFingerprint:   "root-avd",
		Revoked:           false,
	}
	if err := VerifyAttestation(exp, obs); err != ErrAttestWrongApp {
		t.Fatalf("got %v want %v", err, ErrAttestWrongApp)
	}
}

func TestPublicOfferJSONOmitsSecret(t *testing.T) {
	offer, err := NewOffer(OfferParams{Port: 9443})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalPublicOffer(offer.Public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("public offer JSON must not mention secret: %s", raw)
	}
}

func TestSecretReleasedOnlyAfterClaimsOK(t *testing.T) {
	offer, err := NewOffer(OfferParams{Port: 9443})
	if err != nil {
		t.Fatal(err)
	}
	exp := AttestExpected{
		PackageName:            "app.codexlauncher",
		SigningCertSHA256:      "aa11",
		OfferID:                offer.Public.OfferID,
		RuntimeIdentity:        offer.Public.RuntimeIdentity,
		EphemeralPublicKey:     offer.Public.EphemeralPub,
		TLSSPKI:                offer.Public.TLSSPKI,
		TranscriptNonce:        "nonce",
		PinnedRootFingerprints: map[string]struct{}{"root-avd": {}},
	}
	bad := AttestObserved{
		PackageName:       "com.evil",
		SigningCertSHA256: "aa11",
		Challenge: ChallengeBytes(
			offer.Public.OfferID, offer.Public.RuntimeIdentity, offer.Public.EphemeralPub, offer.Public.TLSSPKI, "nonce",
		),
		Level:           SecurityTrustedEnvironment,
		RootFingerprint: "root-avd",
	}
	if _, err := offer.ReleaseSecret(exp, bad); err != ErrAttestWrongApp {
		t.Fatalf("evil app should not get secret: %v", err)
	}
	good := bad
	good.PackageName = "app.codexlauncher"
	secret, err := offer.ReleaseSecret(exp, good)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || secret != offer.Secret {
		t.Fatalf("expected secret released")
	}
	if _, err := offer.ReleaseSecret(exp, good); err != ErrOfferUsed {
		t.Fatalf("second release must fail: %v", err)
	}
}
