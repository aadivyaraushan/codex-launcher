package relaybox

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	// defaultTokenTTL is how long a minted data-line token stays valid if the
	// Mac never redeems it. Kept short: an orphaned token is meant to expire
	// in a few seconds, not linger.
	defaultTokenTTL = 5 * time.Second
	// defaultPhoneWaitTimeout is how long the box holds a phone connection
	// open, waiting for the Mac to redeem the token it was given, before
	// giving up and dropping the phone connection cheaply.
	defaultPhoneWaitTimeout = 5 * time.Second
	// macDoorHeaderTimeout bounds how long the box waits for the first line
	// (REGISTER/REDEEM) on a new Mac-door connection before giving up.
	macDoorHeaderTimeout = 5 * time.Second
	// maxMacDoorHeaderBytes caps how many bytes the box will buffer while
	// reading a Mac-door header line (REGISTER/REDEEM), so a client
	// streaming a header with no newline on the PUBLIC Mac door cannot grow
	// memory without bound.
	maxMacDoorHeaderBytes = 4096

	// MinSecretLength is the minimum length a registration secret must
	// have. With the Tailscale network gate gone, the relay secret is the
	// sole access gate, so a trivially short secret must be refused.
	//
	// This must stay in sync with app/config.go's minRelaySecretLength:
	// the two packages can't share a const without a new dependency
	// between them, so the value is duplicated deliberately.
	MinSecretLength = 16
)

var (
	// ErrMissingSecret is returned by New when no registration secret is given.
	ErrMissingSecret = errors.New("relaybox: registration secret is required")
	// ErrWeakSecret is returned by New when a registration secret is given
	// but is shorter than MinSecretLength.
	ErrWeakSecret = errors.New("relaybox: registration secret is too short")
	// ErrMissingCertificate is returned by New when no Mac-door certificate is given.
	ErrMissingCertificate = errors.New("relaybox: mac-door certificate is required")
)

// Box is the meeting-point relay: it holds one Mac's registration, mints
// short-lived tokens for phones that show up, and copies raw bytes between
// the two once the Mac claims a token. It never parses or decrypts the
// phone<->Mac stream, and it holds none of their keys.
type Box struct {
	secret           []byte
	cert             tls.Certificate
	now              func() time.Time
	tokenTTL         time.Duration
	phoneWaitTimeout time.Duration
	logger           *slog.Logger

	tokens *tokenStore

	mu      sync.Mutex
	control *controlLine
}

// Option configures optional Box behavior. Tests use these to shrink
// timeouts so a single iteration of the token-expiry / wait-timeout tests
// takes milliseconds instead of seconds.
type Option func(*Box)

// WithClock overrides the clock used for token expiry bookkeeping. Defaults
// to time.Now. Connection-level network deadlines always use the real clock
// regardless of this setting, since net.Conn deadlines are wall-clock based.
func WithClock(now func() time.Time) Option {
	return func(b *Box) {
		if now != nil {
			b.now = now
		}
	}
}

// WithTokenTTL overrides how long a minted data-line token stays valid.
func WithTokenTTL(ttl time.Duration) Option {
	return func(b *Box) {
		if ttl > 0 {
			b.tokenTTL = ttl
		}
	}
}

// WithPhoneWaitTimeout overrides how long a phone connection waits for the
// Mac to redeem its token before the box drops it.
func WithPhoneWaitTimeout(timeout time.Duration) Option {
	return func(b *Box) {
		if timeout > 0 {
			b.phoneWaitTimeout = timeout
		}
	}
}

// WithLogger overrides the box's logger. Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(b *Box) {
		if logger != nil {
			b.logger = logger
		}
	}
}

// New creates a Box. secret is the registration secret the Mac must present
// on the Mac door to become (or take back) the control line; cert is the
// box's own TLS identity for the Mac door (see GenerateSelfSignedCertificate).
func New(secret string, cert tls.Certificate, opts ...Option) (*Box, error) {
	if secret == "" {
		return nil, ErrMissingSecret
	}
	if len(secret) < MinSecretLength {
		return nil, ErrWeakSecret
	}
	if len(cert.Certificate) == 0 || cert.PrivateKey == nil {
		return nil, ErrMissingCertificate
	}
	box := &Box{
		secret:           []byte(secret),
		cert:             cert,
		now:              time.Now,
		tokenTTL:         defaultTokenTTL,
		phoneWaitTimeout: defaultPhoneWaitTimeout,
		logger:           slog.Default(),
		tokens:           newTokenStore(),
	}
	for _, opt := range opts {
		opt(box)
	}
	return box, nil
}
