package relayclient_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
