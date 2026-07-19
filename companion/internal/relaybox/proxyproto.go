package relaybox

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const maxProxyProtocolV1Header = 108

// NewProxyProtocolListener consumes Fly's PROXY protocol v1 line before the
// connection reaches the phone door. This keeps the relayed payload unchanged
// while making RemoteAddr report the real public client for per-IP limits.
func NewProxyProtocolListener(inner net.Listener, headerTimeout time.Duration) net.Listener {
	return &proxyProtocolListener{Listener: inner, headerTimeout: headerTimeout}
}

type proxyProtocolListener struct {
	net.Listener
	headerTimeout time.Duration
}

func (listener *proxyProtocolListener) Accept() (net.Conn, error) {
	for {
		conn, err := listener.Listener.Accept()
		if err != nil {
			return nil, err
		}
		wrapped, err := readProxyProtocolV1(conn, listener.headerTimeout)
		if err == nil {
			return wrapped, nil
		}
		_ = conn.Close()
	}
}

type proxyProtocolConn struct {
	*bufferedConn
	remote net.Addr
}

func (conn *proxyProtocolConn) RemoteAddr() net.Addr { return conn.remote }

func readProxyProtocolV1(conn net.Conn, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		return nil, errors.New("relaybox: proxy protocol header timeout is required")
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	reader := bufio.NewReaderSize(conn, maxProxyProtocolV1Header)
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("relaybox: read proxy protocol header: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	if len(header) > maxProxyProtocolV1Header || !strings.HasSuffix(header, "\r\n") {
		return nil, errors.New("relaybox: invalid proxy protocol header length")
	}
	fields := strings.Fields(strings.TrimSuffix(header, "\r\n"))
	if len(fields) != 6 || fields[0] != "PROXY" || fields[1] != "TCP4" && fields[1] != "TCP6" {
		return nil, errors.New("relaybox: invalid proxy protocol header")
	}
	ip := net.ParseIP(fields[2])
	port, err := strconv.Atoi(fields[4])
	if ip == nil || err != nil || port < 1 || port > 65535 {
		return nil, errors.New("relaybox: invalid proxy protocol client address")
	}
	return &proxyProtocolConn{
		bufferedConn: &bufferedConn{Conn: conn, reader: reader},
		remote:       &net.TCPAddr{IP: ip, Port: port},
	}, nil
}
