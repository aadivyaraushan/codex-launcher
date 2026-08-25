package phoneruntime_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime"
)

// The OpenClaw plugin (Phase 3) connects to the bridge over TLS and pins the
// runtime's certificate instead of disabling verification. That only works if
// the runtime exports the certificate it is actually serving to a file the
// plugin can read.
func TestAgentBridgeCertExportedMatchesServedCertificate(t *testing.T) {
	root := t.TempDir()
	rt, err := phoneruntime.Open(context.Background(), phoneruntime.Config{
		Root:          root,
		DisplayName:   "Operator phone",
		ListenAddress: "127.0.0.1:0",
	}, phoneruntime.Dependencies{Random: rand.Reader})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rt.Close()

	raw, err := os.ReadFile(rt.AgentBridgeCertPath())
	if err != nil {
		t.Fatalf("cert not exported on disk: %v", err)
	}
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("exported file is not a PEM certificate (block=%v)", block)
	}
	if len(rest) != 0 {
		t.Fatalf("exported file has %d trailing bytes after the certificate", len(rest))
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		t.Fatalf("exported certificate does not parse: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = rt.Serve(ctx) }()

	var addr string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if addr = rt.BoundAddress(); addr != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("runtime never bound")
	}

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	served := conn.ConnectionState().PeerCertificates
	if len(served) == 0 {
		t.Fatal("server presented no certificate")
	}
	if !bytes.Equal(served[0].Raw, block.Bytes) {
		t.Fatal("exported certificate differs from the certificate the runtime serves")
	}
}
