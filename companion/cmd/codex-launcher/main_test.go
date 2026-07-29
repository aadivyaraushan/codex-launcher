package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
	"github.com/codex-launcher/codex-launcher/companion/internal/servicehealth"
)

// testPinnedKeyFlag is a base64-encoded stand-in for a box's public key,
// only used so config.Validate()'s pinned-key decode step passes in tests
// that never actually dial a relay box.
const testPinnedKeyFlag = "cGlubmVkLWtleS1ieXRlcw=="

// testRelayConfig returns a RelayConfig that passes Validate for tests that
// load a saved config directly rather than going through setup.
func testRelayConfig() companionapp.RelayConfig {
	return companionapp.RelayConfig{
		BoxHost: "relay.example.com", MacPort: 9000, PhonePort: 8443,
		PinnedKey: testPinnedKeyFlag, Secret: "relay-secret-value",
	}
}

func TestFirstTimeSetupAndInstallDoNotRequireAnExistingConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setupCalls := 0
	installer := &mainRecordingInstaller{}
	dependencies := liveDependencies{
		setup: func(_ context.Context, config companionapp.Config) error {
			setupCalls++
			if config.ComputerName != "Studio Mac" || config.Projects[0].Path != projectPath {
				t.Fatalf("setup config = %#v", config)
			}
			return nil
		},
		installer: installer,
	}
	setupArgs := []string{
		"setup", "--computer-name", "Studio Mac", "--box-host", "relay.example.com", "--pinned-key", testPinnedKeyFlag, "--relay-secret", "relay-secret-value",
		"--codex-binary", filepath.Join(t.TempDir(), "codex"),
		"--project-id", "main", "--project-name", "Main", "--project-path", projectPath,
	}
	if code := runWith(context.Background(), setupArgs, io.Discard, io.Discard, dependencies); code != 0 || setupCalls != 1 {
		t.Fatalf("setup exit = %d, calls = %d", code, setupCalls)
	}
	if code := runWith(context.Background(), []string{"install"}, io.Discard, io.Discard, dependencies); code != 0 || !reflect.DeepEqual(installer.calls, []string{"install"}) {
		t.Fatalf("install exit = %d, calls = %#v", code, installer.calls)
	}
}

func TestTaskCommandsStillRequireSavedConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var errorOutput bytes.Buffer
	if code := runWith(context.Background(), []string{"status"}, io.Discard, &errorOutput, liveDependencies{}); code != 1 || errorOutput.String() != "Companion setup is incomplete.\n" {
		t.Fatalf("status exit = %d, stderr = %q", code, errorOutput.String())
	}
}

func TestConfiguredDoctorAndStatusUseInjectedHostChecks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Test computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	installer := &mainRecordingInstaller{status: hostinstall.ServiceStatus{Installed: true, Running: true}}
	dependencies := liveDependencies{
		random: rand.Reader, installer: installer,
		doctor: func(context.Context, companionapp.Config) []cli.Check {
			return []cli.Check{{Name: "injected", OK: true, Detail: "checked"}}
		},
	}
	var output bytes.Buffer
	if code := runWith(context.Background(), []string{"doctor"}, &output, io.Discard, dependencies); code != 0 || !strings.Contains(output.String(), `"name":"injected"`) {
		t.Fatalf("doctor exit = %d, output = %q", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(root, "state.sqlite3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("doctor created or changed runtime state before inspection: %v", err)
	}
	output.Reset()
	if code := runWith(context.Background(), []string{"status"}, &output, io.Discard, dependencies); code != 0 || !strings.Contains(output.String(), `"running":true`) {
		t.Fatalf("status exit = %d, output = %q", code, output.String())
	}
}

func TestPlatformBackendSelectsEachDocumentedCurrentUserService(t *testing.T) {
	home := t.TempDir()
	configHome := t.TempDir()
	for _, test := range []struct {
		goos string
		want string
	}{
		{"darwin", "*launchd.Backend"},
		{"linux", "*systemd.Backend"},
		{"windows", "*windows.Backend"},
	} {
		backend, err := platformBackend(test.goos, home, configHome)
		if err != nil || reflect.TypeOf(backend).String() != test.want {
			t.Fatalf("platform = %s, backend = %T, error = %v", test.goos, backend, err)
		}
	}
	if _, err := platformBackend("plan9", home, configHome); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
}

type mainRecordingInstaller struct {
	calls  []string
	status hostinstall.ServiceStatus
}

func (installer *mainRecordingInstaller) Install(context.Context) error {
	installer.calls = append(installer.calls, "install")
	return nil
}
func (installer *mainRecordingInstaller) Replace(_ context.Context, path string) error {
	installer.calls = append(installer.calls, "replace:"+path)
	return nil
}
func (installer *mainRecordingInstaller) Rollback(context.Context) error {
	installer.calls = append(installer.calls, "rollback")
	return nil
}
func (installer *mainRecordingInstaller) Uninstall(context.Context) error {
	installer.calls = append(installer.calls, "uninstall")
	return nil
}
func (installer *mainRecordingInstaller) Status(context.Context) (hostinstall.ServiceStatus, error) {
	return installer.status, nil
}

var _ cli.Installer = (*mainRecordingInstaller)(nil)

func TestConfiguredCLIUsesPersistentRuntimeAcrossProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{
		Version: 1, ComputerName: "Test computer", Relay: testRelayConfig(),
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}},
	}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}

	var firstOutput, errorOutput bytes.Buffer
	if code := run(context.Background(), []string{"pair"}, &firstOutput, &errorOutput, rand.Reader); code != 0 {
		t.Fatalf("first pair exit = %d, stderr = %s", code, errorOutput.String())
	}
	firstURI, err := url.Parse(strings.TrimSpace(firstOutput.String()))
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := firstURI.Query().Get("identity")
	if firstIdentity == "" {
		t.Fatalf("first pair URI = %q", firstOutput.String())
	}

	var secondOutput bytes.Buffer
	if code := run(context.Background(), []string{"pair"}, &secondOutput, &errorOutput, rand.Reader); code != 0 {
		t.Fatalf("second pair exit = %d, stderr = %s", code, errorOutput.String())
	}
	secondURI, err := url.Parse(strings.TrimSpace(secondOutput.String()))
	if err != nil || secondURI.Query().Get("identity") != firstIdentity {
		t.Fatalf("second identity stable = %v, error = %v", secondURI.Query().Get("identity") == firstIdentity, err)
	}
	if _, err := os.Stat(filepath.Join(root, "state.sqlite3")); err != nil {
		t.Fatalf("persistent state database missing: %v", err)
	}
}

func TestServeUsesPersistentRuntimeAndTheOwnedCodexTaskSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{
		Version: 1, ComputerName: "Test computer", Relay: testRelayConfig(),
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}},
	}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner := newFakeCodexOwner()
	health := servicehealth.New(filepath.Join(root, "health.json"), func() time.Time { return time.Date(2026, 7, 14, 4, 0, 0, 0, time.UTC) })
	var errorOutput bytes.Buffer
	code := runWith(ctx, []string{"serve"}, io.Discard, &errorOutput, liveDependencies{
		random:     rand.Reader,
		startCodex: func(context.Context, string) (codexOwner, error) { return owner, nil },
		relayListen: func(context.Context, relayclient.Config) (net.Listener, error) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
			return listener, err
		},
		now:    func() time.Time { return time.Date(2026, 7, 14, 4, 0, 0, 0, time.UTC) },
		health: health,
	})
	if code != 0 || !owner.closed {
		t.Fatalf("serve exit = %d, owner closed = %v, stderr = %s", code, owner.closed, errorOutput.String())
	}
	if _, err := os.Stat(filepath.Join(root, "state.sqlite3")); err != nil {
		t.Fatalf("serve state database missing: %v", err)
	}
	if record, err := health.Read(); err != nil || record.AttemptID == "" || record.State != servicehealth.StateStopped || record.LastError != "" {
		t.Fatalf("health after clean stop = %#v, error = %v", record, err)
	}
}

func TestServeRecordsOnlyASafeCodeWhenCodexCannotStart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Test computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	health := servicehealth.New(filepath.Join(root, "health.json"), time.Now)
	code := runWith(context.Background(), []string{"serve"}, io.Discard, io.Discard, liveDependencies{
		random: rand.Reader, startCodex: func(context.Context, string) (codexOwner, error) { return nil, errors.New("secret raw process error") },
		relayListen: func(context.Context, relayclient.Config) (net.Listener, error) {
			return net.Listen("tcp", "127.0.0.1:0")
		},
		now: time.Now, health: health,
	})
	if code != 1 {
		t.Fatalf("serve exit = %d", code)
	}
	record, err := health.Read()
	if err != nil || record.State != servicehealth.StateFailed || record.LastError != servicehealth.ErrorCodexUnavailable {
		t.Fatalf("health = %#v, error = %v", record, err)
	}
	encoded, err := os.ReadFile(filepath.Join(root, "health.json"))
	if err != nil || strings.Contains(string(encoded), "secret raw process error") {
		t.Fatalf("unsafe health record = %q, error = %v", encoded, err)
	}
}

func TestOwnedCodexExitStopsTheMobileService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Test computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	owner := newFakeCodexOwner()
	var errorOutput bytes.Buffer
	code := runWith(context.Background(), []string{"serve"}, io.Discard, &errorOutput, liveDependencies{
		random:     rand.Reader,
		startCodex: func(context.Context, string) (codexOwner, error) { return owner, nil },
		relayListen: func(context.Context, relayclient.Config) (net.Listener, error) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			go func() {
				time.Sleep(20 * time.Millisecond)
				close(owner.done)
			}()
			return listener, err
		},
		now: time.Now,
	})
	if code != 1 || !strings.Contains(errorOutput.String(), "Codex stopped") || !owner.closed {
		t.Fatalf("serve exit = %d, owner closed = %v, stderr = %s", code, owner.closed, errorOutput.String())
	}
}

func TestPairCommandOfferIsAcceptedByTheRunningService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := companionapp.ConfigRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := companionapp.Config{Version: 1, ComputerName: "Test computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: projectPath}}}
	if err := companionapp.WriteConfig(filepath.Join(root, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := newFakeCodexOwner()
	addresses := make(chan string, 1)
	serveDone := make(chan int, 1)
	go func() {
		serveDone <- runWith(ctx, []string{"serve"}, io.Discard, io.Discard, liveDependencies{
			random:     rand.Reader,
			startCodex: func(context.Context, string) (codexOwner, error) { return owner, nil },
			relayListen: func(context.Context, relayclient.Config) (net.Listener, error) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err == nil {
					addresses <- listener.Addr().String()
				}
				return listener, err
			},
			now: time.Now,
		})
	}()
	address := <-addresses
	var pairOutput bytes.Buffer
	if code := run(context.Background(), []string{"pair"}, &pairOutput, io.Discard, rand.Reader); code != 0 {
		t.Fatalf("pair exit = %d", code)
	}
	pairURI, err := url.Parse(strings.TrimSpace(pairOutput.String()))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(pairURI.Query().Get("port"))
	request := pairing.PairRequest{
		Secret: pairURI.Query().Get("secret"), Host: pairURI.Query().Get("host"), Port: port,
		Protocol: 1, HostPublicKey: pairURI.Query().Get("identity"), DeviceID: "pixel-9", DeviceName: "Pixel 9", DevicePublicKey: publicKey,
	}
	request.Signature = ed25519.Sign(privateKey, pairing.PairingProofMessage(request))
	body, err := json.Marshal(map[string]any{
		"secret": request.Secret, "host": request.Host, "port": request.Port, "protocol": request.Protocol,
		"hostPublicKey": request.HostPublicKey, "deviceId": request.DeviceID, "deviceName": request.DeviceName,
		"devicePublicKey": base64.RawURLEncoding.EncodeToString(request.DevicePublicKey), "signature": base64.RawURLEncoding.EncodeToString(request.Signature),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}}, Timeout: 3 * time.Second}
	response, err := client.Post("https://"+address+"/v1/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("pair response status = %d", response.StatusCode)
	}
	cancel()
	if code := <-serveDone; code != 0 {
		t.Fatalf("serve exit = %d", code)
	}
}

type fakeCodexOwner struct {
	events chan taskstate.MobileEvent
	done   chan struct{}
	closed bool
}

func newFakeCodexOwner() *fakeCodexOwner {
	return &fakeCodexOwner{events: make(chan taskstate.MobileEvent), done: make(chan struct{})}
}

func (*fakeCodexOwner) TaskSource() companionTaskSource                         { return emptyTaskSource{} }
func (owner *fakeCodexOwner) TaskEvents() <-chan taskstate.MobileEvent          { return owner.events }
func (*fakeCodexOwner) DecisionOwner() *decisions.AppServerOwner                { return nil }
func (*fakeCodexOwner) DecisionRequests() <-chan appserver.ServerRequest        { return nil }
func (*fakeCodexOwner) DesktopDecisionRequests() <-chan appserver.ServerRequest { return nil }
func (owner *fakeCodexOwner) Done() <-chan struct{}                             { return owner.done }
func (owner *fakeCodexOwner) Close() error {
	if !owner.closed {
		owner.closed = true
		close(owner.events)
	}
	return nil
}

type emptyTaskSource struct{}

func (emptyTaskSource) ListRecent(context.Context, int) ([]taskstate.Task, error) { return nil, nil }
