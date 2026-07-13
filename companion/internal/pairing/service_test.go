package pairing

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)

func TestPairingOfferIsBoundExpiredAndSingleUse(t *testing.T) {
	service := newTestService(t)
	offer, err := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base64.RawURLEncoding.DecodeString(offer.Secret)
	if err != nil || len(secret) != 16 || !offer.ExpiresAt.Equal(testNow.Add(5*time.Minute)) {
		t.Fatalf("offer = %#v, secret bytes = %d, error = %v", offer, len(secret), err)
	}
	parsed, err := url.Parse(offer.URI)
	if err != nil || parsed.Scheme != "codex-launcher" || parsed.Host != "pair" || parsed.Query().Get("host") != "mac.tailnet.ts.net" || parsed.Query().Get("port") != "9443" || parsed.Query().Get("v") != "1" || parsed.Query().Get("identity") != offer.HostPublicKey || parsed.Query().Get("secret") != offer.Secret {
		t.Fatalf("pairing URI = %q, parsed = %#v, error = %v", offer.URI, parsed, err)
	}
	request := signedPairRequest(t, offer, "pixel-9")
	if _, err := service.Pair(context.Background(), request, testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Pair(context.Background(), request, testNow.Add(2*time.Minute)); !errors.Is(err, ErrPairingCodeUsed) {
		t.Fatalf("reused code error = %v", err)
	}
	expired, err := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Pair(context.Background(), signedPairRequest(t, expired, "pixel-10"), testNow.Add(6*time.Minute)); !errors.Is(err, ErrPairingCodeExpired) {
		t.Fatalf("expired code error = %v", err)
	}
}

func TestDeviceListNeverExposesStoredPublicKeys(t *testing.T) {
	service, _, _ := pairedService(t)
	devices, err := service.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := DeviceInfo{ID: "pixel-9", Name: "Pixel 9", PairedAt: testNow}
	if len(devices) != 1 || devices[0] != want {
		t.Fatalf("devices = %#v, want %#v", devices, want)
	}
}

func TestPairingRejectsWrongBindingKeyAndInterruptedCommit(t *testing.T) {
	store := NewMemoryStore()
	service, err := NewService(context.Background(), store, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	offer, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	request := signedPairRequest(t, offer, "pixel-9")
	wrongHost := request
	wrongHost.Host = "attacker.tailnet.ts.net"
	if _, err := service.Pair(context.Background(), wrongHost, testNow); !errors.Is(err, ErrPairingBinding) {
		t.Fatalf("wrong host error = %v", err)
	}
	wrongSignature := request
	wrongSignature.Signature = make([]byte, ed25519.SignatureSize)
	if _, err := service.Pair(context.Background(), wrongSignature, testNow); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("wrong signature error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Pair(cancelled, request, testNow); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled pair error = %v", err)
	}
	if _, err := store.Device(context.Background(), "pixel-9"); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("half-paired device lookup = %v", err)
	}
	if _, err := service.Pair(context.Background(), request, testNow); err != nil {
		t.Fatalf("retry after interrupted commit = %v", err)
	}
}

func TestPairingRejectsGuessedIdentityVersionAndKeySubstitution(t *testing.T) {
	service := newTestService(t)
	offer, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	valid := signedPairRequest(t, offer, "pixel-9")
	tests := []struct {
		name string
		edit func(*PairRequest)
		want error
	}{
		{name: "guessed secret", edit: func(request *PairRequest) { request.Secret = base64.RawURLEncoding.EncodeToString(make([]byte, 16)) }, want: ErrPairingCodeExpired},
		{name: "wrong identity", edit: func(request *PairRequest) { request.HostPublicKey = "substituted" }, want: ErrPairingBinding},
		{name: "wrong version", edit: func(request *PairRequest) { request.Protocol = 2 }, want: ErrPairingBinding},
		{name: "wrong port", edit: func(request *PairRequest) { request.Port++ }, want: ErrPairingBinding},
		{name: "substituted device key", edit: func(request *PairRequest) { request.DevicePublicKey, _, _ = ed25519.GenerateKey(rand.Reader) }, want: ErrInvalidProof},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.edit(&request)
			if _, err := service.Pair(context.Background(), request, testNow); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPairingLogsNeverContainSecretNameOrKey(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	service, err := NewServiceWithLogger(context.Background(), NewMemoryStore(), rand.Reader, logger)
	if err != nil {
		t.Fatal(err)
	}
	offer, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	request := signedPairRequest(t, offer, "pixel-9")
	if _, err := service.Pair(context.Background(), request, testNow); err != nil {
		t.Fatal(err)
	}
	logs := output.String()
	for _, forbidden := range []string{offer.Secret, request.DeviceName, base64.RawURLEncoding.EncodeToString(request.DevicePublicKey)} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("pairing log contains protected value %q: %s", forbidden, logs)
		}
	}
	if !strings.Contains(logs, "[pairing] device paired") || !strings.Contains(logs, "device_id=pixel-9") {
		t.Fatalf("pairing log lacks safe context: %s", logs)
	}
}

func TestSessionAuthenticationRejectsSubstitutionStaleAndReplay(t *testing.T) {
	service := newTestService(t)
	offer, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	request, privateKey := signedPairRequestWithKey(t, offer, "pixel-9")
	if _, err := service.Pair(context.Background(), request, testNow); err != nil {
		t.Fatal(err)
	}
	challenge, err := service.BeginSession("pixel-9", "session-1", testNow)
	if err != nil {
		t.Fatal(err)
	}
	auth := proofFromChallenge(challenge)
	auth.Signature = ed25519.Sign(privateKey, SessionProofMessage(auth))
	session, err := service.Authenticate(context.Background(), auth, testNow.Add(time.Second))
	if err != nil || session.DeviceID() != "pixel-9" {
		t.Fatalf("authenticate = %#v, %v", session, err)
	}
	if _, err := service.Authenticate(context.Background(), auth, testNow.Add(2*time.Second)); !errors.Is(err, ErrReplay) {
		t.Fatalf("replay error = %v", err)
	}
	stale, _ := service.BeginSession("pixel-9", "session-2", testNow)
	staleProof := proofFromChallenge(stale)
	staleProof.Signature = ed25519.Sign(privateKey, SessionProofMessage(staleProof))
	if _, err := service.Authenticate(context.Background(), staleProof, testNow.Add(2*time.Minute)); !errors.Is(err, ErrChallengeExpired) {
		t.Fatalf("stale challenge error = %v", err)
	}
	wrongSession, _ := service.BeginSession("pixel-9", "session-3", testNow)
	wrongProof := proofFromChallenge(wrongSession)
	wrongProof.SessionID = "substituted"
	wrongProof.Signature = ed25519.Sign(privateKey, SessionProofMessage(wrongProof))
	if _, err := service.Authenticate(context.Background(), wrongProof, testNow); !errors.Is(err, ErrChallengeBinding) {
		t.Fatalf("substituted session error = %v", err)
	}
}

func TestStatelessChallengesCannotBeExhaustedBeforeAuthentication(t *testing.T) {
	service, privateKey, _ := pairedService(t)
	var last SessionChallenge
	for index := 0; index < 100; index++ {
		challenge, err := service.BeginSession("pixel-9", "session-"+strconv.Itoa(index), testNow)
		if err != nil {
			t.Fatal(err)
		}
		last = challenge
	}
	if len(service.sessionReplays) != 0 {
		t.Fatalf("unauthenticated challenges allocated replay state: %d", len(service.sessionReplays))
	}
	proof := proofFromChallenge(last)
	proof.Signature = ed25519.Sign(privateKey, SessionProofMessage(proof))
	if _, err := service.Authenticate(context.Background(), proof, testNow); err != nil {
		t.Fatalf("real phone blocked after challenge flood: %v", err)
	}
}

func TestAuthenticatedSessionsAreCappedAndAClosedSlotCanBeReused(t *testing.T) {
	service, privateKey, offer := pairedService(t)
	sessions := make([]*Session, 0, MaxActiveSessionsPerDevice)
	for index := 0; index < MaxActiveSessionsPerDevice; index++ {
		sessions = append(sessions, authenticatedSession(t, service, privateKey, offer, "active-"+strconv.Itoa(index)))
	}
	if err := authenticateWithKey(service, offer, "over-capacity", privateKey); !errors.Is(err, ErrSessionCapacity) {
		t.Fatalf("session above active cap error = %v", err)
	}

	sessions[0].Close()
	if err := authenticateWithKey(service, offer, "replacement", privateKey); err != nil {
		t.Fatalf("replacement session after close = %v", err)
	}
}

func TestChallengeIssuedBeforeRevocationCannotAuthenticateAfterward(t *testing.T) {
	service, privateKey, _ := pairedService(t)
	challenge, err := service.BeginSession("pixel-9", "pre-revoke", testNow)
	if err != nil {
		t.Fatal(err)
	}
	proof := proofFromChallenge(challenge)
	proof.Signature = ed25519.Sign(privateKey, SessionProofMessage(proof))
	if err := service.Revoke(context.Background(), "pixel-9"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), proof, testNow); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("post-revoke challenge error = %v", err)
	}
}

func TestChallengeIssuedBeforeRevocationCannotAuthenticateAfterRepair(t *testing.T) {
	service, privateKey, _ := pairedService(t)
	challenge, err := service.BeginSession("pixel-9", "pre-revoke", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(context.Background(), "pixel-9"); err != nil {
		t.Fatal(err)
	}

	offer, err := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	request := PairRequest{Secret: offer.Secret, Host: offer.Target.Host, Port: offer.Target.Port, Protocol: offer.Target.Protocol, HostPublicKey: offer.HostPublicKey, DeviceID: "pixel-9", DeviceName: "Pixel 9", DevicePublicKey: publicKey}
	request.Signature = ed25519.Sign(privateKey, PairingProofMessage(request))
	if _, err := service.Pair(context.Background(), request, testNow); err != nil {
		t.Fatal(err)
	}

	proof := proofFromChallenge(challenge)
	proof.Signature = ed25519.Sign(privateKey, SessionProofMessage(proof))
	if _, err := service.Authenticate(context.Background(), proof, testNow); !errors.Is(err, ErrChallengeBinding) {
		t.Fatalf("pre-revoke proof after re-pair error = %v", err)
	}
}

func TestRevocationClosesSessionsAndRejectsFutureChallenges(t *testing.T) {
	service, privateKey, offer := pairedService(t)
	session := authenticatedSession(t, service, privateKey, offer, "session-1")
	if err := service.Revoke(context.Background(), "pixel-9"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("revocation did not close the active session")
	}
	if _, err := service.BeginSession("pixel-9", "session-2", testNow); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("revoked challenge error = %v", err)
	}
}

func TestKeyRotationAcceptsOverlapThenRemovesOldKey(t *testing.T) {
	service, oldPrivate, offer := pairedService(t)
	session := authenticatedSession(t, service, oldPrivate, offer, "session-1")
	newPublic, newPrivate, _ := ed25519.GenerateKey(rand.Reader)
	rotation := RotationProof{DeviceID: "pixel-9", NewPublicKey: newPublic}
	rotation.Signature = ed25519.Sign(oldPrivate, RotationProofMessage(rotation))
	if err := service.BeginKeyRotation(context.Background(), session, rotation); err != nil {
		t.Fatal(err)
	}
	if err := authenticateWithKey(service, offer, "overlap-old", oldPrivate); err != nil {
		t.Fatalf("old overlap key = %v", err)
	}
	if err := authenticateWithKey(service, offer, "overlap-new", newPrivate); err != nil {
		t.Fatalf("new overlap key = %v", err)
	}
	confirmation := RotationConfirmation{DeviceID: "pixel-9", NewPublicKey: newPublic}
	confirmation.Signature = ed25519.Sign(newPrivate, RotationConfirmationMessage(confirmation))
	if err := service.ConfirmKeyRotation(context.Background(), session, confirmation); err != nil {
		t.Fatal(err)
	}
	if err := authenticateWithKey(service, offer, "old-rejected", oldPrivate); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("retired old key error = %v", err)
	}
	if err := authenticateWithKey(service, offer, "new-kept", newPrivate); err != nil {
		t.Fatalf("confirmed new key = %v", err)
	}
}

func TestTLSCertificateRenewsUnderStableIdentityPin(t *testing.T) {
	service := newTestService(t)
	first, err := service.TLSCertificate(testNow)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.TLSCertificate(testNow.Add(24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	firstLeaf, err := x509.ParseCertificate(first.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	secondLeaf, err := x509.ParseCertificate(second.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(firstLeaf.RawSubjectPublicKeyInfo) != string(secondLeaf.RawSubjectPublicKeyInfo) || firstLeaf.SerialNumber.Cmp(secondLeaf.SerialNumber) == 0 || !firstLeaf.NotAfter.After(testNow) {
		t.Fatalf("certificate identity/renewal mismatch")
	}
}

func TestConcurrentPairingCodesCannotEnrollTwoPhones(t *testing.T) {
	store := &pairingBarrierStore{MemoryStore: NewMemoryStore(), bothEntered: make(chan struct{})}
	service, err := NewService(context.Background(), store, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	second, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	requests := []PairRequest{signedPairRequest(t, first, "pixel-9"), signedPairRequest(t, second, "pixel-10")}
	results := make(chan error, 2)
	for _, request := range requests {
		go func(request PairRequest) {
			_, pairErr := service.Pair(context.Background(), request, testNow)
			results <- pairErr
		}(request)
	}
	errorsSeen := []error{<-results, <-results}
	successes, alreadyPaired := 0, 0
	for _, pairErr := range errorsSeen {
		if pairErr == nil {
			successes++
		} else if errors.Is(pairErr, ErrAlreadyPaired) {
			alreadyPaired++
		}
	}
	if successes != 1 || alreadyPaired != 1 {
		t.Fatalf("concurrent pair results = %#v", errorsSeen)
	}
}

func TestMissingHostIdentityInvalidatesRestoredDeviceRecords(t *testing.T) {
	store := NewMemoryStore()
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := store.SaveDevice(context.Background(), DeviceRecord{ID: "pixel-9", Name: "Pixel 9", CurrentPublicKey: publicKey, PairedAt: testNow}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(context.Background(), store, rand.Reader); err != nil {
		t.Fatal(err)
	}
	if devices, err := store.Devices(context.Background()); err != nil || len(devices) != 0 {
		t.Fatalf("devices after identity recovery = %#v, %v", devices, err)
	}
}

type pairingBarrierStore struct {
	*MemoryStore
	mu          sync.Mutex
	entered     int
	bothEntered chan struct{}
	closeOnce   sync.Once
}

func (store *pairingBarrierStore) Devices(ctx context.Context) ([]DeviceRecord, error) {
	store.mu.Lock()
	store.entered++
	if store.entered == 2 {
		store.closeOnce.Do(func() { close(store.bothEntered) })
	}
	store.mu.Unlock()
	select {
	case <-store.bothEntered:
	case <-time.After(50 * time.Millisecond):
	}
	return store.MemoryStore.Devices(ctx)
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(context.Background(), NewMemoryStore(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func signedPairRequest(t *testing.T, offer PairingOffer, deviceID string) PairRequest {
	t.Helper()
	request, _ := signedPairRequestWithKey(t, offer, deviceID)
	return request
}

func signedPairRequestWithKey(t *testing.T, offer PairingOffer, deviceID string) (PairRequest, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	request := PairRequest{Secret: offer.Secret, Host: offer.Target.Host, Port: offer.Target.Port, Protocol: offer.Target.Protocol, HostPublicKey: offer.HostPublicKey, DeviceID: deviceID, DeviceName: "Pixel 9", DevicePublicKey: publicKey}
	request.Signature = ed25519.Sign(privateKey, PairingProofMessage(request))
	return request, privateKey
}

func pairedService(t *testing.T) (*Service, ed25519.PrivateKey, PairingOffer) {
	t.Helper()
	service := newTestService(t)
	offer, _ := service.BeginPairing(PairingTarget{Host: "mac.tailnet.ts.net", Port: 9443, Protocol: 1}, testNow)
	request, privateKey := signedPairRequestWithKey(t, offer, "pixel-9")
	if _, err := service.Pair(context.Background(), request, testNow); err != nil {
		t.Fatal(err)
	}
	return service, privateKey, offer
}

func authenticatedSession(t *testing.T, service *Service, key ed25519.PrivateKey, offer PairingOffer, sessionID string) *Session {
	t.Helper()
	challenge, err := service.BeginSession("pixel-9", sessionID, testNow)
	if err != nil {
		t.Fatal(err)
	}
	proof := proofFromChallenge(challenge)
	proof.Signature = ed25519.Sign(key, SessionProofMessage(proof))
	session, err := service.Authenticate(context.Background(), proof, testNow)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func authenticateWithKey(service *Service, offer PairingOffer, sessionID string, key ed25519.PrivateKey) error {
	challenge, err := service.BeginSession("pixel-9", sessionID, testNow)
	if err != nil {
		return err
	}
	proof := proofFromChallenge(challenge)
	proof.Signature = ed25519.Sign(key, SessionProofMessage(proof))
	_, err = service.Authenticate(context.Background(), proof, testNow)
	return err
}

func proofFromChallenge(challenge SessionChallenge) SessionProof {
	return SessionProof{DeviceID: challenge.DeviceID, SessionID: challenge.SessionID, PairingGeneration: challenge.PairingGeneration, Protocol: challenge.Protocol, HostPublicKey: challenge.HostPublicKey, Nonce: challenge.Nonce, ExpiresAt: challenge.ExpiresAt, HostSignature: append([]byte(nil), challenge.HostSignature...)}
}
