package pairing

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ProtocolMajor = 1

const (
	MaxActiveSessionsPerDevice = 4
	MaxSessionReplaysPerDevice = 32
	MaxSessionReplaysGlobal    = 64
)

var (
	ErrAlreadyPaired      = errors.New("a phone is already paired")
	ErrChallengeBinding   = errors.New("session challenge binding does not match")
	ErrChallengeExpired   = errors.New("session challenge expired")
	ErrInvalidProof       = errors.New("signature proof is invalid")
	ErrPairingBinding     = errors.New("pairing code binding does not match")
	ErrPairingCodeExpired = errors.New("pairing code expired")
	ErrPairingCodeUsed    = errors.New("pairing code was already used")
	ErrReplay             = errors.New("session proof was already used")
	ErrSessionClosed      = errors.New("authenticated session is closed")
	ErrSessionCapacity    = errors.New("authenticated session capacity reached")
)

type PairingTarget struct {
	Host     string
	Port     int
	Protocol int
}

type PairingOffer struct {
	Secret        string
	URI           string
	HostPublicKey string
	Target        PairingTarget
	ExpiresAt     time.Time
}

type PairRequest struct {
	Secret          string
	Host            string
	Port            int
	Protocol        int
	HostPublicKey   string
	DeviceID        string
	DeviceName      string
	DevicePublicKey ed25519.PublicKey
	Signature       []byte
}

type SessionChallenge struct {
	DeviceID          string
	SessionID         string
	PairingGeneration string
	Protocol          int
	HostPublicKey     string
	Nonce             string
	ExpiresAt         time.Time
	HostSignature     []byte
}

type SessionProof struct {
	DeviceID          string
	SessionID         string
	PairingGeneration string
	Protocol          int
	HostPublicKey     string
	Nonce             string
	ExpiresAt         time.Time
	HostSignature     []byte
	Signature         []byte
}

type RotationProof struct {
	DeviceID     string
	NewPublicKey ed25519.PublicKey
	Signature    []byte
}

type RotationConfirmation struct {
	DeviceID     string
	NewPublicKey ed25519.PublicKey
	Signature    []byte
}

type pendingPairing struct {
	target    PairingTarget
	expiresAt time.Time
	inUse     bool
}

type sessionReplay struct {
	deviceID  string
	expiresAt time.Time
}

type Service struct {
	store       Store
	random      io.Reader
	logger      *slog.Logger
	identity    ed25519.PrivateKey
	publicKey   ed25519.PublicKey
	fingerprint string

	mu             sync.Mutex
	deviceMu       sync.RWMutex
	pairings       map[[32]byte]pendingPairing
	consumed       map[[32]byte]time.Time
	sessionReplays map[[32]byte]sessionReplay
	sessions       map[string]map[*Session]struct{}
}

type Session struct {
	deviceID  string
	sessionID string
	done      chan struct{}
	onClose   func(*Session)
	once      sync.Once
}

func NewService(ctx context.Context, store Store, random io.Reader) (*Service, error) {
	return NewServiceWithLogger(ctx, store, random, slog.Default())
}

func NewServiceWithLogger(ctx context.Context, store Store, random io.Reader, logger *slog.Logger) (*Service, error) {
	if store == nil || random == nil {
		return nil, errors.New("pairing store and secure random source are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	identity, err := store.HostIdentity(ctx)
	if errors.Is(err, ErrIdentityNotFound) {
		devices, deviceErr := store.Devices(ctx)
		if deviceErr != nil {
			return nil, fmt.Errorf("inspect devices without host identity: %w", deviceErr)
		}
		if len(devices) != 0 {
			if clearErr := store.ClearDevices(ctx); clearErr != nil {
				return nil, fmt.Errorf("invalidate devices after host identity loss: %w", clearErr)
			}
			logger.Warn("[pairing] invalidated paired devices", "branch_reason", "host_identity_missing", "device_count", len(devices))
		}
		_, identity, err = ed25519.GenerateKey(random)
		if err == nil {
			err = store.SaveHostIdentity(ctx, identity)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("load host identity: %w", err)
	}
	if len(identity) != ed25519.PrivateKeySize {
		return nil, errors.New("stored host identity is invalid")
	}
	publicKey := append(ed25519.PublicKey(nil), identity.Public().(ed25519.PublicKey)...)
	fingerprint := base64.RawURLEncoding.EncodeToString(publicKey)
	return &Service{store: store, random: random, logger: logger, identity: append(ed25519.PrivateKey(nil), identity...), publicKey: publicKey, fingerprint: fingerprint,
		pairings: make(map[[32]byte]pendingPairing), consumed: make(map[[32]byte]time.Time), sessionReplays: make(map[[32]byte]sessionReplay), sessions: make(map[string]map[*Session]struct{})}, nil
}

func (service *Service) BeginPairing(target PairingTarget, now time.Time) (PairingOffer, error) {
	if !validTarget(target) {
		return PairingOffer{}, ErrPairingBinding
	}
	secret := make([]byte, 16)
	if _, err := io.ReadFull(service.random, secret); err != nil {
		return PairingOffer{}, fmt.Errorf("generate pairing code: %w", err)
	}
	encodedSecret := base64.RawURLEncoding.EncodeToString(secret)
	expiresAt := now.Add(5 * time.Minute)
	hash := sha256.Sum256(secret)
	service.mu.Lock()
	service.expireLocked(now)
	service.pairings[hash] = pendingPairing{target: target, expiresAt: expiresAt}
	service.mu.Unlock()
	query := url.Values{}
	query.Set("host", target.Host)
	query.Set("port", strconv.Itoa(target.Port))
	query.Set("v", strconv.Itoa(target.Protocol))
	query.Set("identity", service.fingerprint)
	query.Set("secret", encodedSecret)
	pairingURL := (&url.URL{Scheme: "codex-launcher", Host: "pair", RawQuery: query.Encode()}).String()
	service.logger.Info("[pairing] offer created", "host", target.Host, "port", target.Port, "protocol", target.Protocol, "expires_at", expiresAt)
	return PairingOffer{Secret: encodedSecret, URI: pairingURL, HostPublicKey: service.fingerprint, Target: target, ExpiresAt: expiresAt}, nil
}

func (service *Service) Pair(ctx context.Context, request PairRequest, now time.Time) (DeviceRecord, error) {
	secret, err := base64.RawURLEncoding.DecodeString(request.Secret)
	if err != nil || len(secret) != 16 {
		return DeviceRecord{}, ErrPairingBinding
	}
	hash := sha256.Sum256(secret)
	service.mu.Lock()
	service.expireLocked(now)
	if _, used := service.consumed[hash]; used {
		service.mu.Unlock()
		return DeviceRecord{}, ErrPairingCodeUsed
	}
	pending, ok := service.pairings[hash]
	if !ok {
		service.mu.Unlock()
		return DeviceRecord{}, ErrPairingCodeExpired
	}
	if !now.Before(pending.expiresAt) {
		delete(service.pairings, hash)
		service.mu.Unlock()
		return DeviceRecord{}, ErrPairingCodeExpired
	}
	if pending.inUse {
		service.mu.Unlock()
		return DeviceRecord{}, ErrPairingCodeUsed
	}
	if request.Host != pending.target.Host || request.Port != pending.target.Port || request.Protocol != pending.target.Protocol || request.HostPublicKey != service.fingerprint {
		service.mu.Unlock()
		return DeviceRecord{}, ErrPairingBinding
	}
	pending.inUse = true
	service.pairings[hash] = pending
	service.mu.Unlock()
	release := func() {
		service.mu.Lock()
		if current, exists := service.pairings[hash]; exists {
			current.inUse = false
			service.pairings[hash] = current
		}
		service.mu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		release()
		return DeviceRecord{}, err
	}
	if !validID(request.DeviceID) || !validName(request.DeviceName) || len(request.DevicePublicKey) != ed25519.PublicKeySize || len(request.Signature) != ed25519.SignatureSize || !ed25519.Verify(request.DevicePublicKey, PairingProofMessage(request), request.Signature) {
		release()
		return DeviceRecord{}, ErrInvalidProof
	}
	service.deviceMu.Lock()
	defer service.deviceMu.Unlock()
	devices, err := service.store.Devices(ctx)
	if err != nil {
		release()
		return DeviceRecord{}, fmt.Errorf("check paired devices: %w", err)
	}
	if len(devices) != 0 {
		release()
		return DeviceRecord{}, ErrAlreadyPaired
	}
	generationBytes := make([]byte, 16)
	if _, err := io.ReadFull(service.random, generationBytes); err != nil {
		release()
		return DeviceRecord{}, fmt.Errorf("generate pairing generation: %w", err)
	}
	record := DeviceRecord{ID: request.DeviceID, Name: request.DeviceName, PairingGeneration: base64.RawURLEncoding.EncodeToString(generationBytes), CurrentPublicKey: append(ed25519.PublicKey(nil), request.DevicePublicKey...), PairedAt: now}
	if err := service.store.SaveDevice(ctx, record); err != nil {
		release()
		return DeviceRecord{}, fmt.Errorf("commit paired device: %w", err)
	}
	service.mu.Lock()
	delete(service.pairings, hash)
	service.consumed[hash] = pending.expiresAt.Add(5 * time.Minute)
	service.mu.Unlock()
	service.logger.Info("[pairing] device paired", "device_id", request.DeviceID)
	return cloneDevice(record), nil
}

func (service *Service) BeginSession(deviceID, sessionID string, now time.Time) (SessionChallenge, error) {
	if !validID(deviceID) || !validID(sessionID) {
		return SessionChallenge{}, ErrChallengeBinding
	}
	service.deviceMu.RLock()
	device, err := service.store.Device(context.Background(), deviceID)
	service.deviceMu.RUnlock()
	if err != nil {
		return SessionChallenge{}, err
	}
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(service.random, nonce); err != nil {
		return SessionChallenge{}, fmt.Errorf("generate session challenge: %w", err)
	}
	challenge := SessionChallenge{DeviceID: deviceID, SessionID: sessionID, PairingGeneration: device.PairingGeneration, Protocol: ProtocolMajor, HostPublicKey: service.fingerprint, Nonce: base64.RawURLEncoding.EncodeToString(nonce), ExpiresAt: now.Add(time.Minute)}
	challenge.HostSignature = ed25519.Sign(service.identity, SessionChallengeMessage(challenge))
	service.logger.Debug("[pairing] session challenge created", "device_id", deviceID, "session_id", sessionID)
	return challenge, nil
}

func (service *Service) Authenticate(ctx context.Context, proof SessionProof, now time.Time) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	challenge := SessionChallenge{DeviceID: proof.DeviceID, SessionID: proof.SessionID, PairingGeneration: proof.PairingGeneration, Protocol: proof.Protocol, HostPublicKey: proof.HostPublicKey, Nonce: proof.Nonce, ExpiresAt: proof.ExpiresAt, HostSignature: proof.HostSignature}
	if !now.Before(proof.ExpiresAt) {
		return nil, ErrChallengeExpired
	}
	if !validID(proof.DeviceID) || !validID(proof.SessionID) || !validID(proof.PairingGeneration) || proof.Protocol != ProtocolMajor || proof.HostPublicKey != service.fingerprint || len(proof.Nonce) > 128 || len(proof.HostSignature) != ed25519.SignatureSize || !ed25519.Verify(service.publicKey, SessionChallengeMessage(challenge), proof.HostSignature) {
		return nil, ErrChallengeBinding
	}
	service.deviceMu.RLock()
	defer service.deviceMu.RUnlock()
	device, err := service.store.Device(ctx, proof.DeviceID)
	if err != nil {
		return nil, err
	}
	if device.PairingGeneration != proof.PairingGeneration {
		return nil, ErrChallengeBinding
	}
	valid := len(proof.Signature) == ed25519.SignatureSize && ed25519.Verify(device.CurrentPublicKey, SessionProofMessage(proof), proof.Signature)
	if !valid && len(device.PendingPublicKey) == ed25519.PublicKeySize {
		valid = ed25519.Verify(device.PendingPublicKey, SessionProofMessage(proof), proof.Signature)
	}
	if !valid {
		return nil, ErrInvalidProof
	}
	replayKey := sha256.Sum256(proof.HostSignature)
	service.mu.Lock()
	service.expireLocked(now)
	if _, replayed := service.sessionReplays[replayKey]; replayed {
		service.mu.Unlock()
		return nil, ErrReplay
	}
	replaysForDevice := 0
	for _, replay := range service.sessionReplays {
		if replay.deviceID == proof.DeviceID {
			replaysForDevice++
		}
	}
	if len(service.sessions[proof.DeviceID]) >= MaxActiveSessionsPerDevice || len(service.sessionReplays) >= MaxSessionReplaysGlobal || replaysForDevice >= MaxSessionReplaysPerDevice {
		service.mu.Unlock()
		service.logger.Warn("[pairing] authenticated session rejected", "device_id", proof.DeviceID, "branch_reason", "session_capacity")
		return nil, ErrSessionCapacity
	}
	session := &Session{deviceID: proof.DeviceID, sessionID: proof.SessionID, done: make(chan struct{})}
	session.onClose = service.removeSession
	service.sessionReplays[replayKey] = sessionReplay{deviceID: proof.DeviceID, expiresAt: proof.ExpiresAt.Add(time.Minute)}
	if service.sessions[proof.DeviceID] == nil {
		service.sessions[proof.DeviceID] = make(map[*Session]struct{})
	}
	service.sessions[proof.DeviceID][session] = struct{}{}
	service.mu.Unlock()
	service.logger.Info("[pairing] session authenticated", "device_id", proof.DeviceID, "session_id", proof.SessionID)
	return session, nil
}

func (service *Service) Devices(ctx context.Context) ([]DeviceInfo, error) {
	service.deviceMu.RLock()
	defer service.deviceMu.RUnlock()
	records, err := service.store.Devices(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]DeviceInfo, 0, len(records))
	for _, record := range records {
		devices = append(devices, DeviceInfo{ID: record.ID, Name: record.Name, PairedAt: record.PairedAt})
	}
	sort.Slice(devices, func(left, right int) bool { return devices[left].ID < devices[right].ID })
	service.logger.Debug("[pairing] devices listed", "output_count", len(devices))
	return devices, nil
}

func (service *Service) Revoke(ctx context.Context, deviceID string) error {
	service.deviceMu.Lock()
	defer service.deviceMu.Unlock()
	if err := service.store.DeleteDevice(ctx, deviceID); err != nil {
		return err
	}
	service.mu.Lock()
	active := make([]*Session, 0, len(service.sessions[deviceID]))
	for session := range service.sessions[deviceID] {
		active = append(active, session)
	}
	delete(service.sessions, deviceID)
	service.mu.Unlock()
	for _, session := range active {
		session.Close()
	}
	service.logger.Info("[pairing] device revoked", "device_id", deviceID, "closed_session_count", len(active))
	return nil
}

func (service *Service) BeginKeyRotation(ctx context.Context, session *Session, proof RotationProof) error {
	if !validSession(session, proof.DeviceID) || len(proof.NewPublicKey) != ed25519.PublicKeySize || len(proof.Signature) != ed25519.SignatureSize {
		return ErrSessionClosed
	}
	service.deviceMu.Lock()
	defer service.deviceMu.Unlock()
	device, err := service.store.Device(ctx, proof.DeviceID)
	if err != nil {
		return err
	}
	if !ed25519.Verify(device.CurrentPublicKey, RotationProofMessage(proof), proof.Signature) {
		return ErrInvalidProof
	}
	device.PendingPublicKey = append(ed25519.PublicKey(nil), proof.NewPublicKey...)
	if err := service.store.SaveDevice(ctx, device); err != nil {
		return fmt.Errorf("store pending device key: %w", err)
	}
	service.logger.Info("[pairing] device key rotation started", "device_id", proof.DeviceID)
	return nil
}

func (service *Service) ConfirmKeyRotation(ctx context.Context, session *Session, confirmation RotationConfirmation) error {
	if !validSession(session, confirmation.DeviceID) || len(confirmation.NewPublicKey) != ed25519.PublicKeySize || len(confirmation.Signature) != ed25519.SignatureSize {
		return ErrSessionClosed
	}
	service.deviceMu.Lock()
	defer service.deviceMu.Unlock()
	device, err := service.store.Device(ctx, confirmation.DeviceID)
	if err != nil {
		return err
	}
	if len(device.PendingPublicKey) != ed25519.PublicKeySize || !equalBytes(device.PendingPublicKey, confirmation.NewPublicKey) || !ed25519.Verify(device.PendingPublicKey, RotationConfirmationMessage(confirmation), confirmation.Signature) {
		return ErrInvalidProof
	}
	device.CurrentPublicKey = append(ed25519.PublicKey(nil), device.PendingPublicKey...)
	device.PendingPublicKey = nil
	if err := service.store.SaveDevice(ctx, device); err != nil {
		return fmt.Errorf("confirm device key rotation: %w", err)
	}
	service.logger.Info("[pairing] device key rotation confirmed", "device_id", confirmation.DeviceID)
	return nil
}

func (service *Service) TLSCertificate(now time.Time) (tls.Certificate, error) {
	serialBytes := make([]byte, 16)
	if _, err := io.ReadFull(service.random, serialBytes); err != nil {
		return tls.Certificate{}, fmt.Errorf("generate TLS serial: %w", err)
	}
	serial := new(big.Int).SetBytes(serialBytes)
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Codex Launcher Companion"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(30 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(service.random, template, template, service.publicKey, service.identity)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create companion TLS certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse companion TLS certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: service.identity, Leaf: leaf}, nil
}

func (session *Session) DeviceID() string      { return session.deviceID }
func (session *Session) SessionID() string     { return session.sessionID }
func (session *Session) Done() <-chan struct{} { return session.done }
func (session *Session) Close() {
	if session == nil {
		return
	}
	session.once.Do(func() {
		close(session.done)
		if session.onClose != nil {
			session.onClose(session)
		}
	})
}

func (service *Service) removeSession(session *Session) {
	service.mu.Lock()
	defer service.mu.Unlock()
	delete(service.sessions[session.deviceID], session)
	if len(service.sessions[session.deviceID]) == 0 {
		delete(service.sessions, session.deviceID)
	}
}

func PairingProofMessage(request PairRequest) []byte {
	return signedMessage("pair-v1", request.Secret, request.Host, strconv.Itoa(request.Port), strconv.Itoa(request.Protocol), request.HostPublicKey, request.DeviceID, request.DeviceName, base64.RawURLEncoding.EncodeToString(request.DevicePublicKey))
}

func SessionProofMessage(proof SessionProof) []byte {
	return signedMessage("session-v1", proof.DeviceID, proof.SessionID, proof.PairingGeneration, strconv.Itoa(proof.Protocol), proof.HostPublicKey, proof.Nonce, strconv.FormatInt(proof.ExpiresAt.Unix(), 10), base64.RawURLEncoding.EncodeToString(proof.HostSignature))
}

func SessionChallengeMessage(challenge SessionChallenge) []byte {
	return signedMessage("session-challenge-v1", challenge.DeviceID, challenge.SessionID, challenge.PairingGeneration, strconv.Itoa(challenge.Protocol), challenge.HostPublicKey, challenge.Nonce, strconv.FormatInt(challenge.ExpiresAt.Unix(), 10))
}

func RotationProofMessage(proof RotationProof) []byte {
	return signedMessage("rotate-v1", proof.DeviceID, base64.RawURLEncoding.EncodeToString(proof.NewPublicKey))
}

func RotationConfirmationMessage(confirmation RotationConfirmation) []byte {
	return signedMessage("rotate-confirm-v1", confirmation.DeviceID, base64.RawURLEncoding.EncodeToString(confirmation.NewPublicKey))
}

func signedMessage(values ...string) []byte {
	result := make([]byte, 0, 256)
	for _, value := range values {
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(value)))
		result = append(result, length...)
		result = append(result, value...)
	}
	return result
}

func validTarget(target PairingTarget) bool {
	if target.Protocol != ProtocolMajor || target.Port < 1 || target.Port > 65535 || len(target.Host) == 0 || len(target.Host) > 253 || strings.ContainsAny(target.Host, " /\\") {
		return false
	}
	return net.ParseIP(target.Host) != nil || strings.Trim(target.Host, ".-") != ""
}

func validID(value string) bool   { return strings.TrimSpace(value) != "" && len(value) <= 128 }
func validName(value string) bool { return strings.TrimSpace(value) != "" && len(value) <= 128 }
func validSession(session *Session, deviceID string) bool {
	if session == nil || session.deviceID != deviceID {
		return false
	}
	select {
	case <-session.done:
		return false
	default:
		return true
	}
}
func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}
func (service *Service) expireLocked(now time.Time) {
	for hash, pairing := range service.pairings {
		if !now.Before(pairing.expiresAt) && !pairing.inUse {
			delete(service.pairings, hash)
		}
	}
	for hash, expiresAt := range service.consumed {
		if !now.Before(expiresAt) {
			delete(service.consumed, hash)
		}
	}
	for replayKey, replay := range service.sessionReplays {
		if !now.Before(replay.expiresAt) {
			delete(service.sessionReplays, replayKey)
		}
	}
}
