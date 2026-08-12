package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseRuntimeCLIWiresGatewayFlags(t *testing.T) {
	cli, err := parseRuntimeCLI([]string{
		"-root", "/var/lib/operator-phone",
		"-gateway-url", "ws://127.0.0.1:18789",
		"-gateway-token-path", "/var/lib/operator-phone/gateway-token",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cli.GatewayURL != "ws://127.0.0.1:18789" {
		t.Fatalf("GatewayURL = %q", cli.GatewayURL)
	}
	if cli.GatewayTokenPath != "/var/lib/operator-phone/gateway-token" {
		t.Fatalf("GatewayTokenPath = %q", cli.GatewayTokenPath)
	}
	if cli.AllowSoftwareAttest {
		t.Fatal("software attest must stay off by default")
	}
	if cli.BeeperBaseURL != "" {
		t.Fatalf("BeeperBaseURL = %q, want empty when the flag is omitted", cli.BeeperBaseURL)
	}
}

func TestParseRuntimeCLIWiresBeeperBaseURL(t *testing.T) {
	cli, err := parseRuntimeCLI([]string{
		"-root", "/var/lib/operator-phone",
		"-gateway-url", "ws://127.0.0.1:18789",
		"-gateway-token-path", "/var/lib/operator-phone/gateway-token",
		"-beeper-base-url", "http://127.0.0.1:23373",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cli.BeeperBaseURL != "http://127.0.0.1:23373" {
		t.Fatalf("BeeperBaseURL = %q, want the loopback Client API URL", cli.BeeperBaseURL)
	}
	if cli.AllowSoftwareAttest {
		t.Fatal("software attest must stay off when wiring Beeper")
	}
}

func TestParseRuntimeCLIAllowSoftwareFromFlag(t *testing.T) {
	cli, err := parseRuntimeCLI([]string{
		"-root", "/tmp/root",
		"-allow-software-attest",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.AllowSoftwareAttest {
		t.Fatal("expected -allow-software-attest to enable the AVD hatch")
	}
}

func TestParseRuntimeCLIAllowSoftwareFromEnv(t *testing.T) {
	t.Setenv("OPERATOR_ALLOW_SOFTWARE_ATTEST", "1")
	cli, err := parseRuntimeCLI([]string{"-root", "/tmp/root"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.AllowSoftwareAttest {
		t.Fatal("expected OPERATOR_ALLOW_SOFTWARE_ATTEST=1 to enable the AVD hatch")
	}
}

func TestParseRuntimeCLIEnvDoesNotDisableExplicitFlag(t *testing.T) {
	t.Setenv("OPERATOR_ALLOW_SOFTWARE_ATTEST", "0")
	cli, err := parseRuntimeCLI([]string{
		"-root", "/tmp/root",
		"-allow-software-attest",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.AllowSoftwareAttest {
		t.Fatal("explicit CLI flag must win over a disabled env value")
	}
}

func TestHelpListsGatewayAndAttestFlags(t *testing.T) {
	var buf bytes.Buffer
	_, err := parseRuntimeCLI([]string{"-h"}, &buf)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("got %v, want flag.ErrHelp", err)
	}
	out := buf.String()
	for _, needle := range []string{
		"-gateway-url",
		"-gateway-token-path",
		"-allow-software-attest",
		"-beeper-base-url",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("help missing %s:\n%s", needle, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "bearer token value") {
		t.Fatalf("help must not talk about token values:\n%s", out)
	}
	if strings.Contains(out, "-beeper-token") {
		t.Fatalf("Beeper token must not be a CLI flag (Config has no token path; token comes from env or account.db):\n%s", out)
	}
}
