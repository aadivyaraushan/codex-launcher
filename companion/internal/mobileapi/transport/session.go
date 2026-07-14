package transport

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/attachments"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/contract"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

var errInvalidWireEncoding = errors.New("mobile transport encoding is invalid")

type AttachmentEvent struct {
	UploadID      string `json:"uploadId"`
	State         string `json:"state"`
	ReceivedBytes int64  `json:"receivedBytes"`
	SHA256        string `json:"sha256"`
	NextChunk     uint32 `json:"nextChunk"`
}

type attachmentSession struct {
	deviceID string
	protocol *contract.Session
	store    *attachments.Store
	now      func() time.Time
}

func newAttachmentSession(deviceID string, protocol *contract.Session, store *attachments.Store, now func() time.Time) *attachmentSession {
	if protocol == nil || store == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &attachmentSession{deviceID: deviceID, protocol: protocol, store: store, now: now}
}

func (session *attachmentSession) acceptText(message contract.Message) (*AttachmentEvent, error) {
	if session == nil {
		return nil, contract.ErrInvalidAttachment
	}
	switch message.Type {
	case "attachment_offer":
		var offer contract.AttachmentOffer
		if err := json.Unmarshal(message.Body, &offer); err != nil {
			return nil, contract.ErrInvalidAttachment
		}
		progress, err := session.store.Begin(session.deviceID, offer.UploadID, offer.DeclaredTotal, offer.SHA256, session.now())
		if err != nil {
			return nil, err
		}
		if progress.ReceivedBytes != 0 {
			if err := session.restore(offer.UploadID, progress.NextChunk); err != nil {
				return nil, err
			}
		}
		return &AttachmentEvent{UploadID: offer.UploadID, State: "accepted", ReceivedBytes: progress.ReceivedBytes, SHA256: offer.SHA256, NextChunk: progress.NextChunk}, nil
	case "attachment_cancel":
		uploadID, err := attachmentMessageID(message)
		if err != nil {
			return nil, err
		}
		retained, err := session.store.Retained(session.deviceID, uploadID, session.now())
		if err != nil {
			return nil, err
		}
		if err := session.store.Cancel(session.deviceID, uploadID); err != nil {
			return nil, err
		}
		return &AttachmentEvent{UploadID: uploadID, State: "cancelled", ReceivedBytes: int64(len(retained.Received)), SHA256: retained.SHA256, NextChunk: retained.NextChunk}, nil
	case "attachment_complete":
		uploadID, err := attachmentMessageID(message)
		if err != nil {
			return nil, err
		}
		completed, err := session.store.Complete(session.deviceID, uploadID, session.now())
		if err != nil {
			return nil, err
		}
		retained, err := session.store.Retained(session.deviceID, uploadID, session.now())
		if err != nil {
			return nil, err
		}
		return &AttachmentEvent{UploadID: uploadID, State: "complete", ReceivedBytes: retained.DeclaredTotal, SHA256: completed.SHA256, NextChunk: retained.NextChunk}, nil
	default:
		return nil, nil
	}
}

func (session *attachmentSession) acceptBinary(frame []byte) error {
	if session == nil {
		return contract.ErrInvalidAttachment
	}
	chunk, err := session.protocol.AcceptAttachmentFrameChunk(frame)
	if err != nil {
		return err
	}
	_, err = session.store.Append(session.deviceID, chunk.UploadID, chunk.Chunk, chunk.Offset, chunk.Final, chunk.Payload, session.now())
	return err
}

func (session *attachmentSession) restoreRequested() ([]AttachmentEvent, error) {
	if session == nil {
		return nil, contract.ErrInvalidAttachment
	}
	requested := session.protocol.RequestedUploads()
	events := make([]AttachmentEvent, 0, len(requested))
	for uploadID, claimedChunk := range requested {
		if err := session.restore(uploadID, claimedChunk); err != nil {
			return nil, err
		}
		retained, err := session.store.Retained(session.deviceID, uploadID, session.now())
		if err != nil {
			return nil, err
		}
		received := int64(len(retained.Received))
		if retained.Complete {
			received = retained.DeclaredTotal
		}
		events = append(events, AttachmentEvent{UploadID: uploadID, State: "accepted", ReceivedBytes: received, SHA256: retained.SHA256, NextChunk: retained.NextChunk})
	}
	return events, nil
}

func (session *attachmentSession) restore(uploadID string, claimedChunk uint32) error {
	retained, err := session.store.Retained(session.deviceID, uploadID, session.now())
	if err != nil || retained.NextChunk != claimedChunk {
		return contract.ErrInvalidAttachment
	}
	session.protocol.CancelAttachment(uploadID)
	if retained.Complete {
		return session.protocol.RestoreCompletedAttachment(contract.AttachmentAck{
			UploadID: uploadID, ReceivedBytes: retained.DeclaredTotal, SHA256: retained.SHA256,
		})
	}
	return session.protocol.RestoreAttachment(contract.RetainedAttachment{
		Offer:    contract.AttachmentOffer{UploadID: uploadID, DeclaredTotal: retained.DeclaredTotal, SHA256: retained.SHA256},
		Received: retained.Received, NextChunk: retained.NextChunk, ExpiresAt: retained.ExpiresAt,
	})
}

func attachmentMessageID(message contract.Message) (string, error) {
	var body struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.Unmarshal(message.Body, &body); err != nil || body.UploadID == "" {
		return "", contract.ErrInvalidAttachment
	}
	return body.UploadID, nil
}

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
