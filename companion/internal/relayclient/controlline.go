package relayclient

import (
	"bufio"
	"crypto/tls"
	"strings"
	"time"
)

// runControlLoop owns the control line for the lifetime of the Listener: it
// reads "SESSION <token>\n" lines and delivers each token to Accept, and
// reconnects (re-dial + re-REGISTER) whenever the line drops, until the
// Listener is closed or its context is done. conn is the already-registered
// connection Listen just opened.
func (l *Listener) runControlLoop(conn *tls.Conn) {
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			if l.isClosed() || l.ctx.Err() != nil {
				return
			}
			l.logger.Warn("[relayclient] control line dropped, reconnecting")
			next, ok := l.reconnectControlLine()
			if !ok {
				return
			}
			conn = next
			reader = bufio.NewReader(conn)
			continue
		}

		line = strings.TrimRight(line, "\r\n")
		token, ok := strings.CutPrefix(line, "SESSION ")
		if !ok || token == "" {
			l.logger.Warn("[relayclient] control line: unexpected line")
			continue
		}

		l.logger.Info("[relayclient] token received")
		select {
		case l.tokens <- token:
		case <-l.closed:
			return
		case <-l.ctx.Done():
			return
		}
	}
}

// reconnectControlLine retries dial+REGISTER, waiting cfg.reconnectDelay
// between attempts, until it succeeds or the Listener is closed / its
// context is done (in which case ok is false and the caller should stop).
func (l *Listener) reconnectControlLine() (conn *tls.Conn, ok bool) {
	delay := l.cfg.reconnectDelay()
	for {
		select {
		case <-l.closed:
			return nil, false
		case <-l.ctx.Done():
			return nil, false
		case <-time.After(delay):
		}

		next, err := l.dialAndRegister(l.ctx)
		if err != nil {
			l.logger.Warn("[relayclient] reconnect attempt failed")
			continue
		}
		l.setControlConn(next)
		l.logger.Info("[relayclient] control line reconnected")
		return next, true
	}
}
