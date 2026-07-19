package relaybox

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestProxyProtocolListenerExposesClientIPAndPreservesPayload(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer inner.Close()
	listener := NewProxyProtocolListener(inner, time.Second)

	accepted := make(chan net.Conn, 1)
	errors := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errors <- err
			return
		}
		accepted <- conn
	}()

	client, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("PROXY TCP4 203.0.113.9 168.220.90.156 50123 8443\r\nhello")); err != nil {
		t.Fatalf("write proxy frame: %v", err)
	}

	select {
	case err := <-errors:
		t.Fatalf("accept: %v", err)
	case conn := <-accepted:
		defer conn.Close()
		if got := conn.RemoteAddr().String(); got != "203.0.113.9:50123" {
			t.Fatalf("RemoteAddr = %q, want original client", got)
		}
		payload := make([]byte, 5)
		if _, err := io.ReadFull(conn, payload); err != nil || string(payload) != "hello" {
			t.Fatalf("payload = %q, %v", payload, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy listener did not accept")
	}
}
