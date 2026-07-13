package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

var cliNow = time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)

func TestVersionDoesNotRequireAConfiguredRuntime(t *testing.T) {
	var output bytes.Buffer
	command := New(Options{Output: &output})
	if exitCode := command.Run(context.Background(), []string{"version"}); exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if output.String() != "codex-launcher "+Version+"\n" {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestPairDevicesStatusAndRevokeUseOneRuntime(t *testing.T) {
	runtime := newTestRuntime(t)
	var output bytes.Buffer
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	command := New(Options{Runtime: runtime, Output: &output, ErrorOutput: &output, Logger: logger, Now: func() time.Time { return cliNow }})

	if exitCode := command.Run(context.Background(), []string{"pair"}); exitCode != 0 {
		t.Fatalf("pair exit = %d, output = %s", exitCode, output.String())
	}
	pairURI := strings.TrimSpace(output.String())
	parsed, err := url.Parse(pairURI)
	if err != nil || parsed.Scheme != "codex-launcher" || parsed.Query().Get("secret") == "" {
		t.Fatalf("pair output = %q, error = %v", pairURI, err)
	}
	if strings.Contains(logs.String(), parsed.Query().Get("secret")) {
		t.Fatalf("pairing secret leaked to logs: %s", logs.String())
	}
	pairDevice(t, runtime.Pairing, parsed)

	output.Reset()
	if exitCode := command.Run(context.Background(), []string{"devices"}); exitCode != 0 {
		t.Fatalf("devices exit = %d", exitCode)
	}
	var devices []pairing.DeviceInfo
	if err := json.Unmarshal(output.Bytes(), &devices); err != nil || len(devices) != 1 || devices[0].ID != "pixel-9" || devices[0].Name != "Pixel 9" {
		t.Fatalf("devices output = %q, parsed = %#v, error = %v", output.String(), devices, err)
	}

	output.Reset()
	if exitCode := command.Run(context.Background(), []string{"status"}); exitCode != 0 {
		t.Fatalf("status exit = %d", exitCode)
	}
	var status map[string]any
	if err := json.Unmarshal(output.Bytes(), &status); err != nil || status["state"] != "configured" || status["pairedDevices"] != float64(1) || status["projects"] != float64(1) {
		t.Fatalf("status output = %q, parsed = %#v, error = %v", output.String(), status, err)
	}

	output.Reset()
	if exitCode := command.Run(context.Background(), []string{"revoke", "pixel-9"}); exitCode != 0 || output.String() != "Device revoked\n" {
		t.Fatalf("revoke exit = %d, output = %q", exitCode, output.String())
	}
	if devices, err := runtime.Pairing.Devices(context.Background()); err != nil || len(devices) != 0 {
		t.Fatalf("devices after revoke = %#v, %v", devices, err)
	}
}

func TestPairRefusesToCreateAnotherSecretWhenADeviceIsAlreadyPaired(t *testing.T) {
	runtime := newTestRuntime(t)
	offer, err := runtime.Pairing.BeginPairing(pairing.PairingTarget{Host: runtime.Config.ListenHost, Port: runtime.Config.ListenPort, Protocol: pairing.ProtocolMajor}, cliNow)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(offer.URI)
	pairDevice(t, runtime.Pairing, parsed)
	var output bytes.Buffer
	command := New(Options{Runtime: runtime, Output: &output, ErrorOutput: &output, Now: func() time.Time { return cliNow }})

	if exitCode := command.Run(context.Background(), []string{"pair"}); exitCode != 1 || output.String() != "A phone is already paired. Revoke it before pairing another phone.\n" {
		t.Fatalf("pair exit = %d, output = %q", exitCode, output.String())
	}
}

func TestDoctorAndUsageReturnDeterministicExitCodes(t *testing.T) {
	runtime := newTestRuntime(t)
	var output bytes.Buffer
	doctor := func(context.Context, companionapp.Config) []Check {
		return []Check{{Name: "codex", OK: true, Detail: "codex-cli 0.144.0-alpha.4"}, {Name: "tailscale", OK: false, Detail: "not connected"}}
	}
	command := New(Options{Runtime: runtime, Output: &output, ErrorOutput: &output, Doctor: doctor})
	if exitCode := command.Run(context.Background(), []string{"doctor"}); exitCode != 1 {
		t.Fatalf("doctor exit = %d", exitCode)
	}
	var checks []Check
	if err := json.Unmarshal(output.Bytes(), &checks); err != nil || len(checks) != 2 || !checks[0].OK || checks[1].OK {
		t.Fatalf("doctor output = %q, checks = %#v, error = %v", output.String(), checks, err)
	}

	for _, args := range [][]string{nil, {"unknown"}, {"status", "extra"}, {"revoke"}} {
		output.Reset()
		if exitCode := command.Run(context.Background(), args); exitCode != 2 || !strings.HasPrefix(output.String(), "Usage:") {
			t.Fatalf("args = %#v, exit = %d, output = %q", args, exitCode, output.String())
		}
	}
}

func TestConfiguredCommandsFailClosedWithoutRuntime(t *testing.T) {
	for _, name := range []string{"pair", "devices", "status", "doctor"} {
		var output bytes.Buffer
		command := New(Options{Output: &output, ErrorOutput: &output})
		if exitCode := command.Run(context.Background(), []string{name}); exitCode != 1 || output.String() != "Companion setup is incomplete.\n" {
			t.Fatalf("command = %s, exit = %d, output = %q", name, exitCode, output.String())
		}
	}
	for _, args := range [][]string{{"status", "extra"}, {"pair", "extra"}, {"revoke"}} {
		var output bytes.Buffer
		command := New(Options{Output: &output, ErrorOutput: &output})
		if exitCode := command.Run(context.Background(), args); exitCode != 2 || !strings.HasPrefix(output.String(), "Usage:") {
			t.Fatalf("malformed args = %#v, exit = %d, output = %q", args, exitCode, output.String())
		}
	}
}

func newTestRuntime(t *testing.T) *companionapp.Runtime {
	t.Helper()
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	runtime, err := companionapp.NewRuntime(context.Background(), config, companionapp.Dependencies{
		PairingStore: pairing.NewMemoryStore(),
		PromptStore:  promptqueue.NewMemoryStore(),
		EventStore:   eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 32, MaxBytes: 64 * 1024}),
		Random:       rand.Reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func pairDevice(t *testing.T, service *pairing.Service, parsed *url.URL) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	port := 9443
	request := pairing.PairRequest{
		Secret: parsed.Query().Get("secret"), Host: parsed.Query().Get("host"), Port: port, Protocol: pairing.ProtocolMajor,
		HostPublicKey: parsed.Query().Get("identity"), DeviceID: "pixel-9", DeviceName: "Pixel 9", DevicePublicKey: publicKey,
	}
	request.Signature = ed25519.Sign(privateKey, pairing.PairingProofMessage(request))
	if _, err := service.Pair(context.Background(), request, cliNow); err != nil {
		t.Fatal(err)
	}
}
