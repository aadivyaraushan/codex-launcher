package handshake

import "errors"

type Phase string

const (
	PhaseAwaitingOffer Phase = "awaiting_offer"
	PhaseTLSPinned     Phase = "tls_pinned"
	PhaseAttested      Phase = "attested"
	PhaseAcked         Phase = "acked"
	PhaseRejected      Phase = "rejected"
)

type Session struct {
	OfferID string
	Phase   Phase
	Reason  string
}

var ErrIllegalTransition = errors.New("localtrust handshake: illegal transition")

func New(offerID string) Session {
	return Session{OfferID: offerID, Phase: PhaseAwaitingOffer}
}

func (s Session) PinTLS() (Session, error) {
	if s.Phase != PhaseAwaitingOffer {
		return s, ErrIllegalTransition
	}
	s.Phase = PhaseTLSPinned
	return s, nil
}

func (s Session) AcceptAttestation() (Session, error) {
	if s.Phase != PhaseTLSPinned {
		return s, ErrIllegalTransition
	}
	s.Phase = PhaseAttested
	return s, nil
}

func (s Session) Ack() (Session, error) {
	if s.Phase != PhaseAttested {
		return s, ErrIllegalTransition
	}
	s.Phase = PhaseAcked
	return s, nil
}

func (s Session) Reject(reason string) Session {
	s.Phase = PhaseRejected
	s.Reason = reason
	return s
}
