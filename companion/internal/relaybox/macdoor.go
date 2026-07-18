package relaybox

import (
	"bufio"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"net"
	"strings"
	"sync"
	"time"
)

// controlLine is the Mac's one signalling connection to the box. sendSession
// tells the Mac "a phone is here, here is its token." Only one control line
// is live at a time; a fresh correct-secret REGISTER evicts whatever was
// here before.
type controlLine struct {
	conn    net.Conn
	writeMu sync.Mutex
	closed  bool
}

func (line *controlLine) sendSession(token string) error {
	line.writeMu.Lock()
	defer line.writeMu.Unlock()
	if line.closed {
		return net.ErrClosed
	}
	_, err := line.conn.Write([]byte("SESSION " + token + "\n"))
	return err
}

func (line *controlLine) close() {
	line.writeMu.Lock()
	line.closed = true
	line.writeMu.Unlock()
	_ = line.conn.Close()
}

// ServeMacDoor terminates the box's own TLS on listener (the Mac door) and
// handles REGISTER / REDEEM connections until ctx is cancelled or the
// listener fails. The box's own certificate is what the Mac pins
// (relayclient.Dial); this is the only door the box program terminates TLS
// on.
func (box *Box) ServeMacDoor(ctx context.Context, listener net.Listener) error {
	tlsListener := tls.NewListener(listener, &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{box.cert},
	})
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = tlsListener.Close()
		case <-stopped:
		}
	}()
	defer close(stopped)
	for {
		conn, err := tlsListener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go box.handleMacConn(conn)
	}
}

func (box *Box) handleMacConn(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(macDoorHeaderTimeout))
	reader := bufio.NewReaderSize(conn, maxMacDoorHeaderBytes)
	lineBytes, err := reader.ReadSlice('\n')
	if err != nil {
		box.logger.Warn("[relaybox] mac-door connection rejected", "branch_reason", "header_read_failed")
		_ = conn.Close()
		return
	}
	// Copy: ReadSlice's returned slice aliases the reader's internal buffer
	// and is only valid until the next read from reader.
	line := string(lineBytes)
	_ = conn.SetReadDeadline(time.Time{})
	line = strings.TrimRight(line, "\r\n")
	switch {
	case strings.HasPrefix(line, "REGISTER "):
		box.handleRegister(conn, strings.TrimPrefix(line, "REGISTER "))
	case strings.HasPrefix(line, "REDEEM "):
		box.handleRedeem(conn, reader, strings.TrimPrefix(line, "REDEEM "))
	default:
		box.logger.Warn("[relaybox] mac-door connection rejected", "branch_reason", "unknown_framing")
		_ = conn.Close()
	}
}

func (box *Box) handleRegister(conn net.Conn, providedSecret string) {
	if subtle.ConstantTimeCompare([]byte(providedSecret), box.secret) != 1 {
		box.logger.Warn("[relaybox] registration rejected", "branch_reason", "invalid_secret")
		_ = conn.Close()
		return
	}
	line := &controlLine{conn: conn}
	box.mu.Lock()
	previous := box.control
	box.control = line
	box.mu.Unlock()
	if previous != nil {
		box.logger.Info("[relaybox] control line evicted", "branch_reason", "new_registration")
		previous.close()
	}
	box.logger.Info("[relaybox] control line registered")
	box.watchControlLine(line)
}

// watchControlLine blocks for the lifetime of a control line, so its
// eviction (or disconnect) can clear box.control. It ignores any bytes read
// (there is no heartbeat protocol in this slice) and only cares about the
// connection closing.
func (box *Box) watchControlLine(line *controlLine) {
	discard := make([]byte, 1)
	for {
		if _, err := line.conn.Read(discard); err != nil {
			break
		}
	}
	box.mu.Lock()
	if box.control == line {
		box.control = nil
	}
	box.mu.Unlock()
	box.logger.Info("[relaybox] control line disconnected")
}

func (box *Box) handleRedeem(conn net.Conn, reader *bufio.Reader, token string) {
	pending, ok := box.tokens.redeem(token, box.now())
	if !ok {
		box.logger.Warn("[relaybox] data-line redeem rejected", "branch_reason", "unknown_or_expired_token")
		_ = conn.Close()
		return
	}
	if _, err := conn.Write([]byte("OK\n")); err != nil {
		box.logger.Warn("[relaybox] data-line redeem failed", "branch_reason", "ok_write_failed")
		_ = conn.Close()
		return
	}
	dataConn := net.Conn(&bufferedConn{Conn: conn, reader: reader})
	select {
	case pending.connCh <- dataConn:
		box.logger.Info("[relaybox] data line redeemed")
	default:
		// Should not happen: a token is only ever redeemed once, and its
		// channel is created with room for exactly one connection.
		box.logger.Error("[relaybox] data-line handoff dropped", "branch_reason", "unexpected_full_channel")
		_ = conn.Close()
	}
}
