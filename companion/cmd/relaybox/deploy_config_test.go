package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFlyDeployConfigProtectsRelayInvariants checks the checked-in deployment
// boundary, where a stray Fly TLS handler would let Fly decrypt traffic that
// must stay sealed between the phone and Mac.
func TestFlyDeployConfigProtectsRelayInvariants(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate deploy config test source")
	}
	configPath := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "fly.toml"))
	encoded, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read fly.toml: %v", err)
	}
	config := string(encoded)

	requiredOnce := []string{
		`app = "codex-launcher-relay-ssdear"`,
		`primary_region = "lax"`,
		`dockerfile = "Dockerfile.relaybox"`,
		`source = "relaybox_data"`,
		`destination = "/data"`,
		`size = "shared-cpu-1x"`,
		`memory = "256mb"`,
		"\n    port = 443\n",
		"\n    port = 8443\n",
		`internal_port = 9000`,
		`internal_port = 8443`,
		`RELAYBOX_PHONE_PROXY_PROTOCOL = "true"`,
		`handlers = ["proxy_proto"]`,
	}
	for _, required := range requiredOnce {
		if strings.Count(config, required) != 1 {
			t.Errorf("fly.toml must contain %q exactly once", required)
		}
	}

	requiredTwice := []string{
		`protocol = "tcp"`,
		`auto_stop_machines = false`,
		`auto_start_machines = false`,
		`min_machines_running = 1`,
	}
	for _, required := range requiredTwice {
		if strings.Count(config, required) != 2 {
			t.Errorf("fly.toml must contain %q exactly twice", required)
		}
	}
	if strings.Count(config, `handlers = []`) != 1 {
		t.Error("fly.toml must leave exactly the Mac door as unchanged raw TCP")
	}

	for _, forbidden := range []string{
		"PLACEHOLDER",
		`handlers = ["tls"]`,
		`handlers = ["http"]`,
		"RELAYBOX_SECRET =",
	} {
		if strings.Contains(config, forbidden) {
			t.Errorf("fly.toml must not contain %q", forbidden)
		}
	}
}
