// Package relayclient is the Mac-side half of the relay box protocol
// described in planning/relay-box-build-plan.md. It dials OUT to the box's
// Mac door (the Mac is never reachable directly, so it never listens for
// the box) and hands back a plain net.Listener: every phone that shows up
// at the box becomes one Accept()-ed net.Conn, exactly as if the Mac's own
// HTTP/TLS server were listening on a normal socket.
//
// Two kinds of connection go out over the same pinned TLS (see dial.go):
//   - one long-lived "control line", registered once with a shared secret,
//     that the box uses to announce each phone as a one-time token
//     (controlline.go).
//   - one short-lived "data line" per token, redeemed and then handed to
//     the caller as the live net.Conn (Accept, below).
//
// relayclient never imports the pairing/identity packages: the inner TLS
// that actually protects the Mac<->phone conversation is the caller's
// problem, not this package's.
package relayclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

const (
	// defaultReconnectDelay is how long the control-line loop waits before
	// re-dialing after the box drops the connection (network blip, box
	// restart, box evicting us for a fresher registration, etc).
	defaultReconnectDelay = 750 * time.Millisecond
	// tokenBufferSize is how many SESSION tokens can be queued ahead of
	// Accept() calls before the control-line reader would block. A handful
	// is plenty; a real deployment redeems tokens far faster than phones
	// arrive.
	tokenBufferSize = 8
)

// errClosed is returned by Accept once the Listener has been closed (or its
// context cancelled) and no more connections will ever arrive.
var errClosed = errors.New("relayclient: listener closed")

// Config configures a Listener.
type Config struct {
	// BoxAddr is the host:port of the box's Mac door.
	BoxAddr string
	// Secret is the registration secret the box's Mac door requires.
	Secret string
	// PinnedPublicKey is the box leaf certificate's SubjectPublicKeyInfo
	// (RawSubjectPublicKeyInfo). This is the only trust anchor; there is no
	// certificate-authority fallback.
	PinnedPublicKey []byte

	// Logger receives diagnostic logging, tagged "[relayclient]". Defaults
	// to slog.Default() when nil. Never logs the secret or a token value.
	Logger *slog.Logger
	// ReconnectDelay overrides how long the control line waits before
	// re-dialing after a drop. Defaults to defaultReconnectDelay when zero.
	ReconnectDelay time.Duration
}

func (cfg Config) reconnectDelay() time.Duration {
	if cfg.ReconnectDelay > 0 {
		return cfg.ReconnectDelay
	}
	return defaultReconnectDelay
}

// Listener implements net.Listener by turning phones that show up at the
// box into net.Conn values. Construct one with Listen.
type Listener struct {
	cfg    Config
	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	tokens chan string

	closeOnce sync.Once
	closed    chan struct{}

	mu          sync.Mutex
	controlConn *tls.Conn
}

// Listen dials the box's Mac door, registers the control line, and returns
// a *Listener once that first registration's dial+handshake has succeeded
// (so a mis-pinned or unreachable box is reported here, synchronously, not
// on the first Accept). The control line is then kept alive — and
// reconnected on drop — in the background until Close or ctx is done.
func Listen(ctx context.Context, cfg Config) (*Listener, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	listenerCtx, cancel := context.WithCancel(ctx)
	l := &Listener{
		cfg:    cfg,
		logger: logger,
		ctx:    listenerCtx,
		cancel: cancel,
		tokens: make(chan string, tokenBufferSize),
		closed: make(chan struct{}),
	}

	conn, err := l.dialAndRegister(listenerCtx)
	if err != nil {
		logger.Warn("[relayclient] control line register failed")
		cancel()
		return nil, err
	}
	l.setControlConn(conn)
	logger.Info("[relayclient] control line registered")

	// Unblock a control-line read that's stuck in conn.Read when the
	// caller cancels ctx directly (without calling Close): closing the
	// underlying conn is the only way to interrupt that read.
	go func() {
		select {
		case <-listenerCtx.Done():
			l.closeControlConn()
		case <-l.closed:
		}
	}()

	go l.runControlLoop(conn)

	return l, nil
}

// dialAndRegister opens one pinned data connection to the box and sends the
// REGISTER line. The box's Mac door sends no acknowledgement for a
// successful REGISTER (see relaybox.handleRegister) — only a dial/handshake
// failure, or a write failure, is reported here. A wrong secret is
// discovered indirectly, later, when the box silently closes the
// connection and the control loop reconnects.
func (l *Listener) dialAndRegister(ctx context.Context) (*tls.Conn, error) {
	conn, err := Dial(ctx, l.cfg.BoxAddr, l.cfg.PinnedPublicKey)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte("REGISTER " + l.cfg.Secret + "\n")); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("relayclient: send register: %w", err)
	}
	return conn, nil
}

func (l *Listener) setControlConn(conn *tls.Conn) {
	l.mu.Lock()
	l.controlConn = conn
	l.mu.Unlock()
}

func (l *Listener) closeControlConn() {
	l.mu.Lock()
	conn := l.controlConn
	l.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (l *Listener) isClosed() bool {
	select {
	case <-l.closed:
		return true
	default:
		return false
	}
}

// Accept blocks until a phone's token arrives and its data line is
// successfully redeemed, then returns the live *tls.Conn (satisfying
// net.Conn) to hand to the caller's own server. A single token whose
// redeem fails (box rejected it, network error, bad reply) is logged and
// skipped — Accept moves on to the next token rather than returning an
// error, since a returned error would stop the caller's Serve loop for
// good over what is often a transient, single-phone problem. Accept only
// returns an error once the Listener is closed or its context is done.
func (l *Listener) Accept() (net.Conn, error) {
	for {
		select {
		case <-l.closed:
			return nil, errClosed
		case <-l.ctx.Done():
			return nil, l.ctx.Err()
		case token := <-l.tokens:
			conn, err := l.redeem(token)
			if err != nil {
				l.logger.Warn("[relayclient] data line redeem failed", "error", err)
				continue
			}
			l.logger.Info("[relayclient] data line redeemed")
			return conn, nil
		}
	}
}

// redeem opens a fresh pinned data line, sends REDEEM <token>, and reads
// back exactly the "OK\n" reply. The OK line is read one byte at a time on
// purpose: a buffered reader would risk pulling in bytes past the newline,
// but those bytes are the phone's own tunnel data (relayed through the box
// untouched) and must stay unread on the connection so the caller's own
// TLS layer sees them.
func (l *Listener) redeem(token string) (*tls.Conn, error) {
	conn, err := Dial(l.ctx, l.cfg.BoxAddr, l.cfg.PinnedPublicKey)
	if err != nil {
		return nil, fmt.Errorf("relayclient: dial data line: %w", err)
	}
	if _, err := conn.Write([]byte("REDEEM " + token + "\n")); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("relayclient: send redeem: %w", err)
	}
	if err := readOKLine(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// readOKLine reads exactly one newline-terminated line and requires it to
// be "OK\n", reading byte by byte so nothing past the newline is consumed.
func readOKLine(conn net.Conn) error {
	const want = "OK\n"
	buf := make([]byte, 0, len(want))
	single := make([]byte, 1)
	for {
		n, err := conn.Read(single)
		if n > 0 {
			buf = append(buf, single[0])
			if single[0] == '\n' {
				break
			}
			if len(buf) > len(want) {
				return fmt.Errorf("relayclient: redeem reply too long: %q", buf)
			}
		}
		if err != nil {
			return fmt.Errorf("relayclient: read redeem reply: %w", err)
		}
	}
	if string(buf) != want {
		return fmt.Errorf("relayclient: redeem reply not ok: %q", buf)
	}
	return nil
}

// Close is idempotent: it stops the control line and its reconnect loop,
// and makes any blocked or future Accept return errClosed.
func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
		l.cancel()
		l.closeControlConn()
		l.logger.Info("[relayclient] listener closed")
	})
	return nil
}

// relayAddr is a synthetic net.Addr for a Listener: there is no local
// socket address to report (the Mac only ever dials out), so this just
// names the box it's registered with.
type relayAddr string

func (a relayAddr) Network() string { return "relay" }
func (a relayAddr) String() string  { return string(a) }

// Addr returns a synthetic address naming the box's Mac door.
func (l *Listener) Addr() net.Addr {
	return relayAddr(l.cfg.BoxAddr)
}
