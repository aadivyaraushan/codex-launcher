package transport

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

var errInvalidWireEncoding = errors.New("mobile transport encoding is invalid")

type pairRequestBody struct {
	Secret          string `json:"secret"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Protocol        int    `json:"protocol"`
	HostPublicKey   string `json:"hostPublicKey"`
	DeviceID        string `json:"deviceId"`
	DeviceName      string `json:"deviceName"`
	DevicePublicKey string `json:"devicePublicKey"`
	Signature       string `json:"signature"`
}

type pairResponseBody struct {
	DeviceID          string `json:"deviceId"`
	PairingGeneration string `json:"pairingGeneration"`
}

type sessionChallengeBody struct {
	DeviceID          string `json:"deviceId"`
	SessionID         string `json:"sessionId"`
	PairingGeneration string `json:"pairingGeneration"`
	Protocol          int    `json:"protocol"`
	HostPublicKey     string `json:"hostPublicKey"`
	Nonce             string `json:"nonce"`
	ExpiresAt         int64  `json:"expiresAt"`
	HostSignature     string `json:"hostSignature"`
}

type sessionProofBody struct {
	DeviceID          string `json:"deviceId"`
	SessionID         string `json:"sessionId"`
	PairingGeneration string `json:"pairingGeneration"`
	Protocol          int    `json:"protocol"`
	HostPublicKey     string `json:"hostPublicKey"`
	Nonce             string `json:"nonce"`
	ExpiresAt         int64  `json:"expiresAt"`
	HostSignature     string `json:"hostSignature"`
	Signature         string `json:"signature"`
}

type authenticatedBody struct {
	Type      string `json:"type"`
	DeviceID  string `json:"deviceId"`
	SessionID string `json:"sessionId"`
}

func (body pairRequestBody) request() (pairing.PairRequest, error) {
	publicKey, err := base64.RawURLEncoding.DecodeString(body.DevicePublicKey)
	if err != nil {
		return pairing.PairRequest{}, errInvalidWireEncoding
	}
	signature, err := base64.RawURLEncoding.DecodeString(body.Signature)
	if err != nil {
		return pairing.PairRequest{}, errInvalidWireEncoding
	}
	return pairing.PairRequest{
		Secret: body.Secret, Host: body.Host, Port: body.Port, Protocol: body.Protocol, HostPublicKey: body.HostPublicKey,
		DeviceID: body.DeviceID, DeviceName: body.DeviceName, DevicePublicKey: publicKey, Signature: signature,
	}, nil
}

func sessionChallengeBodyFrom(challenge pairing.SessionChallenge) sessionChallengeBody {
	return sessionChallengeBody{
		DeviceID: challenge.DeviceID, SessionID: challenge.SessionID, PairingGeneration: challenge.PairingGeneration,
		Protocol: challenge.Protocol, HostPublicKey: challenge.HostPublicKey, Nonce: challenge.Nonce, ExpiresAt: challenge.ExpiresAt.Unix(),
		HostSignature: base64.RawURLEncoding.EncodeToString(challenge.HostSignature),
	}
}

func (body sessionProofBody) proof() (pairing.SessionProof, error) {
	hostSignature, err := base64.RawURLEncoding.DecodeString(body.HostSignature)
	if err != nil {
		return pairing.SessionProof{}, errInvalidWireEncoding
	}
	signature, err := base64.RawURLEncoding.DecodeString(body.Signature)
	if err != nil {
		return pairing.SessionProof{}, errInvalidWireEncoding
	}
	return pairing.SessionProof{
		DeviceID: body.DeviceID, SessionID: body.SessionID, PairingGeneration: body.PairingGeneration, Protocol: body.Protocol,
		HostPublicKey: body.HostPublicKey, Nonce: body.Nonce, ExpiresAt: time.Unix(body.ExpiresAt, 0),
		HostSignature: hostSignature, Signature: signature,
	}, nil
}

func proofMatchesChallenge(proof pairing.SessionProof, challenge pairing.SessionChallenge) bool {
	return proof.DeviceID == challenge.DeviceID &&
		proof.SessionID == challenge.SessionID &&
		proof.PairingGeneration == challenge.PairingGeneration &&
		proof.Protocol == challenge.Protocol &&
		proof.HostPublicKey == challenge.HostPublicKey &&
		proof.Nonce == challenge.Nonce &&
		proof.ExpiresAt.Unix() == challenge.ExpiresAt.Unix() &&
		subtle.ConstantTimeCompare(proof.HostSignature, challenge.HostSignature) == 1
}
