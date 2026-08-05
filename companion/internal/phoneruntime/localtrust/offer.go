package localtrust

import (
	"encoding/json"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

const ProtocolVersion = 1

const MimeType = "application/vnd.app.codexlauncher.local-pair+json"

var (
	ErrOfferExpired = errors.New("local trust offer expired")
	ErrOfferUsed    = errors.New("local trust offer already used")
	ErrInvalidOffer = errors.New("local trust offer is invalid")
)

type OfferParams struct {
	Port      int
	ExpiresIn time.Duration
	Now       time.Time
}

type PublicOffer struct {
	ProtocolVersion int    `json:"protocolVersion"`
	OfferID         string `json:"offerId"`
	ExpiresAtUnix   int64  `json:"expiresAt"`
	Port            int    `json:"port"`
	RuntimeIdentity string `json:"runtimeIdentity"`
	TLSSPKI         string `json:"tlsSpki"`
	EphemeralPub    string `json:"ephemeralPublicKey"`
	Challenge       string `json:"challenge"`
}

type Offer struct {
	Public PublicOffer
	Secret string
	used   bool
}

func NewOffer(params OfferParams) (*Offer, error) {
	if params.Port != 9443 {
		return nil, fmt.Errorf("%w: port must be 9443", ErrInvalidOffer)
	}
	if params.ExpiresIn <= 0 {
		params.ExpiresIn = 10 * time.Minute
	}
	now := params.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	runtimeKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ephemeralKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	offerID := make([]byte, 16)
	if _, err := rand.Read(offerID); err != nil {
		return nil, err
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return nil, err
	}

	runtimePub, err := x509.MarshalPKIXPublicKey(&runtimeKey.PublicKey)
	if err != nil {
		return nil, err
	}
	ephemeralPub, err := x509.MarshalPKIXPublicKey(&ephemeralKey.PublicKey)
	if err != nil {
		return nil, err
	}
	spkiHash := sha256.Sum256(runtimePub)

	return &Offer{
		Public: PublicOffer{
			ProtocolVersion: ProtocolVersion,
			OfferID:         base64.RawURLEncoding.EncodeToString(offerID),
			ExpiresAtUnix:   now.Add(params.ExpiresIn).Unix(),
			Port:            params.Port,
			RuntimeIdentity: base64.RawURLEncoding.EncodeToString(runtimePub),
			TLSSPKI:         base64.RawURLEncoding.EncodeToString(spkiHash[:]),
			EphemeralPub:    base64.RawURLEncoding.EncodeToString(ephemeralPub),
			Challenge:       base64.RawURLEncoding.EncodeToString(challenge),
		},
		Secret: base64.RawURLEncoding.EncodeToString(secret),
	}, nil
}

func (offer *Offer) ValidateAt(now time.Time) error {
	if offer == nil {
		return ErrInvalidOffer
	}
	if offer.Public.ProtocolVersion != ProtocolVersion || offer.Public.Port != 9443 {
		return ErrInvalidOffer
	}
	if now.Unix() > offer.Public.ExpiresAtUnix {
		return ErrOfferExpired
	}
	if offer.used {
		return ErrOfferUsed
	}
	return nil
}

func (offer *Offer) Consume(now time.Time) error {
	if err := offer.ValidateAt(now); err != nil {
		return err
	}
	offer.used = true
	return nil
}


func MarshalPublicOffer(offer PublicOffer) ([]byte, error) {
	return json.Marshal(offer)
}

// ReleaseSecret verifies attestation claims, then consumes the offer and
// returns the pairing secret once. Wrong claims never mark the offer used.
func (offer *Offer) ReleaseSecret(expected AttestExpected, observed AttestObserved) (string, error) {
	if err := VerifyAttestation(expected, observed); err != nil {
		return "", err
	}
	if err := offer.Consume(time.Now().UTC()); err != nil {
		return "", err
	}
	return offer.Secret, nil
}
