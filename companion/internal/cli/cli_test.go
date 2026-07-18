package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostsetup"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

// testPinnedKeyFlag is a valid base64 value for the --pinned-key flag in
// tests that need one but don't care what it decodes to.
const testPinnedKeyFlag = "cGlubmVkLWtleS1ieXRlcw=="

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
	installer := &recordingInstaller{status: hostinstall.ServiceStatus{Installed: true, Running: true, Detail: "launch agent is loaded"}}
	command := New(Options{Runtime: runtime, Output: &output, ErrorOutput: &output, Logger: logger, Now: func() time.Time { return cliNow }, Installer: installer})

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
	err = json.Unmarshal(output.Bytes(), &status)
	service, _ := status["service"].(map[string]any)
	if err != nil || status["state"] != "configured" || status["pairedDevices"] != float64(1) || status["projects"] != float64(1) || service["running"] != true {
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
	offer, err := runtime.Pairing.BeginPairing(pairing.PairingTarget{Host: runtime.Config.Relay.BoxHost, Port: runtime.Config.Relay.PhonePort, Protocol: pairing.ProtocolMajor}, cliNow)
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

func TestDoctorCanInspectAValidatedConfigWithoutOpeningTheRuntime(t *testing.T) {
	config := newTestRuntime(t).Config
	called := 0
	var output bytes.Buffer
	command := New(Options{
		Config: &config, Output: &output, ErrorOutput: &output,
		Doctor: func(_ context.Context, got companionapp.Config) []Check {
			called++
			if got.ComputerName != config.ComputerName {
				t.Fatalf("doctor config = %#v", got)
			}
			return []Check{{Name: "identity", OK: false, Detail: "host identity is missing while paired devices remain"}}
		},
	})
	if code := command.Run(context.Background(), []string{"doctor"}); code != 1 || called != 1 || !strings.Contains(output.String(), `"name":"identity"`) {
		t.Fatalf("doctor exit = %d, called = %d, output = %q", code, called, output.String())
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

func TestSetupBuildsOneStrictConfigFromExplicitComputerAndFolderFlags(t *testing.T) {
	projectOne := canonicalTempDirForCLI(t)
	projectTwo := canonicalTempDirForCLI(t)
	codexBinary := filepath.Join(t.TempDir(), "codex")
	var captured companionapp.Config
	var output bytes.Buffer
	command := New(Options{
		Output: &output, ErrorOutput: &output,
		Setup: func(_ context.Context, config companionapp.Config) error { captured = config; return nil },
	})
	args := []string{
		"setup", "--computer-name", "Studio Mac",
		"--box-host", "relay.example.com", "--mac-port", "9000", "--phone-port", "8443",
		"--pinned-key", testPinnedKeyFlag, "--relay-secret", "relay-secret-value", "--codex-binary", codexBinary,
		"--project-id", "launcher", "--project-name", "Codex Launcher", "--project-path", projectOne,
		"--project-id", "notes", "--project-name", "Notes", "--project-path", projectTwo,
	}

	if exitCode := command.Run(context.Background(), args); exitCode != 0 {
		t.Fatalf("setup exit = %d, output = %q", exitCode, output.String())
	}
	if captured.Version != 1 || captured.ComputerName != "Studio Mac" || captured.CodexBinary != codexBinary {
		t.Fatalf("setup config = %#v", captured)
	}
	if captured.Relay.BoxHost != "relay.example.com" || captured.Relay.MacPort != 9000 || captured.Relay.PhonePort != 8443 ||
		captured.Relay.PinnedKey != testPinnedKeyFlag || captured.Relay.Secret != "relay-secret-value" {
		t.Fatalf("setup relay config = %#v", captured.Relay)
	}
	if len(captured.Projects) != 2 || captured.Projects[0].ID != "launcher" || captured.Projects[0].DisplayName != "Codex Launcher" || captured.Projects[0].Path != projectOne || captured.Projects[1].ID != "notes" {
		t.Fatalf("setup projects = %#v", captured.Projects)
	}
	if output.String() != "Companion setup saved. Run codex-launcher install next.\n" {
		t.Fatalf("setup output = %q", output.String())
	}
}

func TestSetupRejectsIncompleteProjectTriplesBeforeWriting(t *testing.T) {
	writes := 0
	var output bytes.Buffer
	command := New(Options{
		Output: &output, ErrorOutput: &output,
		Setup: func(context.Context, companionapp.Config) error { writes++; return nil },
	})
	args := []string{
		"setup", "--computer-name", "Studio Mac",
		"--box-host", "relay.example.com", "--pinned-key", testPinnedKeyFlag, "--relay-secret", "relay-secret-value",
		"--codex-binary", filepath.Join(t.TempDir(), "codex"), "--project-id", "main", "--project-name", "Main",
	}

	if exitCode := command.Run(context.Background(), args); exitCode != 2 || writes != 0 || !strings.HasPrefix(output.String(), "Usage:") {
		t.Fatalf("setup exit = %d, writes = %d, output = %q", exitCode, writes, output.String())
	}
}

func TestSetupReportsWhichRequiredLocalDependencyFailed(t *testing.T) {
	projectPath := canonicalTempDirForCLI(t)
	baseArgs := []string{
		"setup", "--computer-name", "Studio Mac",
		"--box-host", "relay.example.com", "--pinned-key", testPinnedKeyFlag, "--relay-secret", "relay-secret-value",
		"--codex-binary", filepath.Join(t.TempDir(), "codex"),
		"--project-id", "main", "--project-name", "Main", "--project-path", projectPath,
	}
	for _, test := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("wrapped: %w", hostsetup.ErrCodexUnavailable), "Codex is missing or broken. Check --codex-binary.\n"},
		{fmt.Errorf("wrapped: %w", hostsetup.ErrRelayUnavailable), "The relay box is unreachable, or its pinned key does not match. Check --box-host, --mac-port, and --pinned-key.\n"},
		{fmt.Errorf("wrapped: %w", hostsetup.ErrRelayRegisterRejected), "The relay box rejected the registration secret. Check --relay-secret.\n"},
		{errors.New("disk failed"), "Companion setup could not be saved.\n"},
	} {
		var output bytes.Buffer
		command := New(Options{Output: &output, ErrorOutput: &output, Setup: func(context.Context, companionapp.Config) error { return test.err }})
		if code := command.Run(context.Background(), baseArgs); code != 1 || output.String() != test.want {
			t.Fatalf("error = %v, exit = %d, output = %q", test.err, code, output.String())
		}
	}
}

func TestInstallReplaceRollbackAndUninstallRouteToTheHostInstaller(t *testing.T) {
	installer := &recordingInstaller{}
	var output bytes.Buffer
	command := New(Options{Output: &output, ErrorOutput: &output, Installer: installer})
	artifact := filepath.Join(t.TempDir(), "codex-launcher")

	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"install"}, "install"},
		{[]string{"install", "--replace", artifact}, "replace:" + artifact},
		{[]string{"rollback"}, "rollback"},
		{[]string{"uninstall"}, "uninstall"},
	} {
		output.Reset()
		if exitCode := command.Run(context.Background(), test.args); exitCode != 0 {
			t.Fatalf("args = %#v, exit = %d, output = %q", test.args, exitCode, output.String())
		}
		if got := installer.calls[len(installer.calls)-1]; got != test.want {
			t.Fatalf("args = %#v, call = %q", test.args, got)
		}
	}
}

func TestDeferredWindowsMaintenanceIsReportedAsScheduledNotCompleted(t *testing.T) {
	installer := &recordingInstaller{err: hostinstall.ErrMaintenanceScheduled}
	for _, args := range [][]string{{"install", "--replace", "replacement.exe"}, {"rollback"}, {"uninstall"}} {
		var output bytes.Buffer
		command := New(Options{Output: &output, ErrorOutput: &output, Installer: installer})
		if code := command.Run(context.Background(), args); code != 0 || !strings.Contains(output.String(), "scheduled") || strings.Contains(output.String(), "completed") {
			t.Fatalf("args = %#v, exit = %d, output = %q", args, code, output.String())
		}
	}
}

type recordingInstaller struct {
	calls  []string
	status hostinstall.ServiceStatus
	err    error
}

func (installer *recordingInstaller) Install(context.Context) error {
	installer.calls = append(installer.calls, "install")
	return installer.err
}
func (installer *recordingInstaller) Replace(_ context.Context, path string) error {
	installer.calls = append(installer.calls, "replace:"+path)
	return installer.err
}
func (installer *recordingInstaller) Rollback(context.Context) error {
	installer.calls = append(installer.calls, "rollback")
	return installer.err
}
func (installer *recordingInstaller) Uninstall(context.Context) error {
	installer.calls = append(installer.calls, "uninstall")
	return installer.err
}
func (installer *recordingInstaller) Status(context.Context) (hostinstall.ServiceStatus, error) {
	return installer.status, nil
}

func canonicalTempDirForCLI(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestRuntime(t *testing.T) *companionapp.Runtime {
	t.Helper()
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{
		Version: 1, ComputerName: "Computer", Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}},
		Relay: companionapp.RelayConfig{BoxHost: "relay.example.com", MacPort: 9000, PhonePort: 8443, PinnedKey: testPinnedKeyFlag, Secret: "relay-secret-value"},
	}
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
	port, err := strconv.Atoi(parsed.Query().Get("port"))
	if err != nil {
		t.Fatal(err)
	}
	request := pairing.PairRequest{
		Secret: parsed.Query().Get("secret"), Host: parsed.Query().Get("host"), Port: port, Protocol: pairing.ProtocolMajor,
		HostPublicKey: parsed.Query().Get("identity"), DeviceID: "pixel-9", DeviceName: "Pixel 9", DevicePublicKey: publicKey,
	}
	request.Signature = ed25519.Sign(privateKey, pairing.PairingProofMessage(request))
	if _, err := service.Pair(context.Background(), request, cliNow); err != nil {
		t.Fatal(err)
	}
}
