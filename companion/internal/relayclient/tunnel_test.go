package relayclient_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/relaybox"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
)

// TestEndToEndTunnelThroughBox is spec test A: a phone reaches the Mac's HTTPS
// server through the box, end to end, with the inner phone<->Mac TLS nested
// inside the outer box-pinned TLS on the Mac hop. If the whole chain — control
// line, token mint, REDEEM handoff, nested TLS handshake, request, response —
// is wired correctly, the phone gets the Mac's answer back verbatim.
func TestEndToEndTunnelThroughBox(t *testing.T) {
	network := startRelayNetwork(t)

	const wantBody = "hello-from-the-mac"
	startMacApp(t, network, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ping" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, wantBody)
	}))

	client := phoneClient(network)
	resp := phoneDo(t, func() (*http.Response, error) {
		return client.Get("https://mac.internal/v1/ping")
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != wantBody {
		t.Fatalf("body: got %q, want %q", string(body), wantBody)
	}
}

// TestBoxCannotReadTunnelBytes is spec test B: the box relays the phone<->Mac
// stream without being able to read it. The phone sends a unique marker in its
// request and the Mac sends a different unique marker in its response; both
// travel end to end and arrive intact, yet neither marker ever appears in the
// bytes the box actually handled on the phone door. What the box saw must look
// like TLS records, not plaintext.
func TestBoxCannotReadTunnelBytes(t *testing.T) {
	network := startRelayNetwork(t)

	const requestMarker = "PHONE-PLAINTEXT-MARKER-a7f3c9e1"
	const responseMarker = "MAC-PLAINTEXT-MARKER-b2d8f460"

	startMacApp(t, network, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if !bytes.Contains(got, []byte(requestMarker)) {
			t.Errorf("mac did not receive the phone's request marker; tunnel is not delivering bytes")
		}
		_, _ = io.WriteString(w, responseMarker)
	}))

	client := phoneClient(network)
	resp := phoneDo(t, func() (*http.Response, error) {
		return client.Post("https://mac.internal/v1/echo", "text/plain", bytes.NewBufferString(requestMarker))
	})
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Contains(body, []byte(responseMarker)) {
		t.Fatalf("phone did not receive the mac's response marker; tunnel is not delivering bytes")
	}

	// The exchange happened. Now prove the box could not read it.
	seen := network.phoneDoorSeen.snapshot()
	if len(seen) == 0 {
		t.Fatal("box handled no phone-door bytes; test cannot prove anything")
	}
	if bytes.Contains(seen, []byte(requestMarker)) {
		t.Error("box saw the phone's plaintext request marker: the tunnel content is NOT sealed")
	}
	if bytes.Contains(seen, []byte(responseMarker)) {
		t.Error("box saw the mac's plaintext response marker: the tunnel content is NOT sealed")
	}
	// A TLS 1.3 stream from the phone begins with a handshake record:
	// content type 0x16 (handshake) then version bytes 0x03 0x01.
	if !(seen[0] == 0x16 && len(seen) >= 3 && seen[1] == 0x03) {
		t.Errorf("box's first phone-door bytes do not look like a TLS record: % x", seen[:min(8, len(seen))])
	}
}

// TestControlLineBlipDoesNotDropActiveCall proves that the signalling line and
// an already-redeemed phone data line have independent lifetimes. Replacing the
// control registration must not interrupt a response already streaming through
// the box, and the original listener must reconnect for the next phone.
func TestControlLineBlipDoesNotDropActiveCall(t *testing.T) {
	network := startRelayNetworkWithOptions(t, relaybox.WithControlHeartbeat(20*time.Millisecond, 200*time.Millisecond))
	releaseResponse := make(chan struct{})

	const reconnectDelay = 250 * time.Millisecond
	startMacAppWithReconnectDelay(t, network, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/stream":
			_, _ = io.WriteString(w, "before-control-blip\n")
			w.(http.Flusher).Flush()
			<-releaseResponse
			_, _ = io.WriteString(w, "after-control-blip\n")
		case "/v1/after":
			_, _ = io.WriteString(w, "control-reconnected")
		default:
			http.NotFound(w, r)
		}
	}), reconnectDelay)

	client := phoneClient(network)
	resp := phoneDo(t, func() (*http.Response, error) {
		return client.Get("https://mac.internal/v1/stream")
	})
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	if err != nil || first != "before-control-blip\n" {
		t.Fatalf("read first streamed chunk: got %q, err=%v", first, err)
	}

	replacement, err := relayclient.Dial(context.Background(), network.macDoorAddr, network.boxPinnedKey)
	if err != nil {
		t.Fatalf("dial replacement control line: %v", err)
	}
	if _, err := replacement.Write([]byte("REGISTER " + testSecret + "\n")); err != nil {
		_ = replacement.Close()
		t.Fatalf("register replacement control line: %v", err)
	}
	_ = replacement.SetReadDeadline(time.Now().Add(time.Second))
	if line, err := bufio.NewReader(replacement).ReadString('\n'); err != nil || line != "PING\n" {
		_ = replacement.Close()
		t.Fatalf("replacement control line was not accepted: heartbeat=%q err=%v", line, err)
	}
	_ = replacement.Close()

	close(releaseResponse)
	second, err := reader.ReadString('\n')
	if err != nil || second != "after-control-blip\n" {
		t.Fatalf("active call was dropped with the control line: got %q, err=%v", second, err)
	}

	reconnectStarted := time.Now()
	after := phoneDo(t, func() (*http.Response, error) {
		return client.Get("https://mac.internal/v1/after")
	})
	if elapsed := time.Since(reconnectStarted); elapsed < reconnectDelay/2 {
		t.Fatalf("control line reconnected in %v, before configured backoff %v", elapsed, reconnectDelay)
	}
	defer func() { _ = after.Body.Close() }()
	body, err := io.ReadAll(after.Body)
	if err != nil || string(body) != "control-reconnected" {
		t.Fatalf("request after control reconnect: got %q, err=%v", body, err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
