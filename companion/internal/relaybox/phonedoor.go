package relaybox

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"time"
)

// ServePhoneDoor accepts raw TCP connections on listener (the phone door)
// until ctx is cancelled or the listener fails. The box never terminates
// TLS here: every byte it reads from a phone connection is copied straight
// to the matching Mac data line, and vice versa (relay, below). This is the
// passthrough rule that keeps the box unable to read phone<->Mac content.
func (box *Box) ServePhoneDoor(ctx context.Context, listener net.Listener) error {
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-stopped:
		}
	}()
	defer close(stopped)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go box.handlePhoneConn(ctx, conn)
	}
}

func (box *Box) handlePhoneConn(ctx context.Context, conn net.Conn) {
	box.mu.Lock()
	control := box.control
	box.mu.Unlock()
	if control == nil {
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "no_control_line")
		_ = conn.Close()
		return
	}

	client := conn.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(client); err == nil {
		client = host
	}
	if !box.phoneLimiter.allow(client, box.now()) {
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "rate_limited", "client_ip", client)
		_ = conn.Close()
		return
	}
	select {
	case box.pendingPhones <- struct{}{}:
		defer func() { <-box.pendingPhones }()
	default:
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "pending_cap_reached", "client_ip", client)
		_ = conn.Close()
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(box.phonePrefaceTimeout))
	preface := make([]byte, 2)
	if _, err := io.ReadFull(conn, preface); err != nil {
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "preface_timeout", "client_ip", client)
		_ = conn.Close()
		return
	}
	if preface[0] != 0x16 || preface[1] != 0x03 {
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "invalid_tls_preface", "client_ip", client)
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	conn = &bufferedConn{Conn: conn, reader: bufio.NewReader(io.MultiReader(bytes.NewReader(preface), conn))}

	token, pending, err := box.tokens.mint(box.now(), box.tokenTTL)
	if err != nil {
		box.logger.Error("[relaybox] phone connection dropped", "branch_reason", "token_mint_failed")
		_ = conn.Close()
		return
	}
	if err := control.sendSession(token); err != nil {
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "control_line_signal_failed")
		box.tokens.dropIfCurrent(token, pending)
		_ = conn.Close()
		return
	}

	timer := time.NewTimer(box.phoneWaitTimeout)
	defer timer.Stop()
	select {
	case dataConn := <-pending.connCh:
		box.logger.Info("[relaybox] phone glued to data line")
		relay(conn, dataConn)
	case <-timer.C:
		box.logger.Warn("[relaybox] phone connection dropped", "branch_reason", "redeem_wait_timeout")
		box.tokens.dropIfCurrent(token, pending)
		_ = conn.Close()
	case <-ctx.Done():
		box.tokens.dropIfCurrent(token, pending)
		_ = conn.Close()
	}
}

// relay copies raw bytes both directions between the phone connection and
// the Mac data line until either side closes, then closes both. This is the
// box's entire job on the content path: it never looks at what it is
// copying.
func relay(phone, data net.Conn) {
	defer func() { _ = phone.Close() }()
	defer func() { _ = data.Close() }()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(data, phone)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(phone, data)
		done <- struct{}{}
	}()
	<-done
}
