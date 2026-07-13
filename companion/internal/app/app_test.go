package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

var appNow = time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC)

func TestLoadConfigAcceptsStrictOwnerOnlyConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL validation intentionally fails closed until the documented ACL implementation is added")
	}
	projectPath := canonicalTempDir(t)
	path := writeConfig(t, map[string]any{
		"version":      1,
		"computerName": "Aadi's Mac",
		"listenHost":   "100.64.0.10",
		"listenPort":   9443,
		"codexBinary":  absoluteCodexPath(),
		"projects":     []map[string]string{{"id": "launcher", "displayName": "Codex Launcher", "path": projectPath}},
	})

	config, err := loadConfig(path, filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != 1 || config.ComputerName != "Aadi's Mac" || config.ListenHost != "100.64.0.10" || config.ListenPort != 9443 || len(config.Projects) != 1 {
		t.Fatalf("config = %#v", config)
	}
}

func TestWriteConfigPublishesOwnerOnlyFileOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL validation intentionally fails closed until the documented ACL implementation is added")
	}
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	config := Config{Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443, CodexBinary: absoluteCodexPath(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	if err := writeConfigAt(path, root, config); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil || rootInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config root info = %#v, error = %v", rootInfo, err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("config file info = %#v, error = %v", fileInfo, err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var shape struct {
		Projects []map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(written, &shape); err != nil || len(shape.Projects) != 1 {
		t.Fatalf("written config shape = %#v, error = %v", shape, err)
	}
	for _, key := range []string{"id", "displayName", "path"} {
		if _, ok := shape.Projects[0][key]; !ok {
			t.Fatalf("written project is missing %q: %s", key, written)
		}
	}
	for _, key := range []string{"ID", "DisplayName", "Path"} {
		if _, ok := shape.Projects[0][key]; ok {
			t.Fatalf("written project contains non-contract key %q: %s", key, written)
		}
	}
	loaded, err := loadConfig(path, root)
	if err != nil || loaded.ComputerName != config.ComputerName {
		t.Fatalf("loaded config = %#v, error = %v", loaded, err)
	}

	replacement := config
	replacement.ComputerName = "Replacement"
	if err := writeConfigAt(path, root, replacement); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("replacement error = %v", err)
	}
	loaded, err = loadConfig(path, root)
	if err != nil || loaded.ComputerName != "Computer" {
		t.Fatalf("config changed after refused replacement = %#v, %v", loaded, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".config-*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %#v", matches)
	}
}

func TestWriteConfigValidatesBeforeCreatingFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL validation intentionally fails closed until the documented ACL implementation is added")
	}
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	config := Config{Version: 1, ComputerName: "Computer", ListenHost: "192.168.1.10", ListenPort: 9443}
	if err := writeConfigAt(path, root, config); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid config write error = %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config root created for invalid input: %v", err)
	}
}

func TestLoadConfigRejectsUnknownOversizedSymlinkAndOpenPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL validation intentionally fails closed until the documented ACL implementation is added")
	}
	valid := map[string]any{
		"version": 1, "computerName": "Computer", "listenHost": "100.64.0.10", "listenPort": 9443,
		"projects": []map[string]string{{"id": "main", "displayName": "Main", "path": canonicalTempDir(t)}},
	}

	unknown := cloneMap(valid)
	unknown["surprise"] = true
	unknownPath := writeConfig(t, unknown)
	if _, err := loadConfig(unknownPath, filepath.Dir(unknownPath)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unknown-field error = %v", err)
	}

	oversized := filepath.Join(secureTempDir(t), "oversized.json")
	if err := os.WriteFile(oversized, make([]byte, MaxConfigBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(oversized, filepath.Dir(oversized)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("oversized error = %v", err)
	}

	realPath := writeConfig(t, valid)
	linkPath := filepath.Join(secureTempDir(t), "config-link.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(linkPath, filepath.Dir(linkPath)); !errors.Is(err, ErrUnsafeConfigFile) {
		t.Fatalf("symlink error = %v", err)
	}

	if runtime.GOOS != "windows" {
		openPath := writeConfig(t, valid)
		if err := os.Chmod(openPath, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(openPath, filepath.Dir(openPath)); !errors.Is(err, ErrUnsafeConfigFile) {
			t.Fatalf("open-permission error = %v", err)
		}
	}

	insecureRoot := t.TempDir()
	if err := os.Chmod(insecureRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	insecurePath := filepath.Join(insecureRoot, "config.json")
	encoded, _ := json.Marshal(valid)
	if err := os.WriteFile(insecurePath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(insecurePath, insecureRoot); !errors.Is(err, ErrUnsafeConfigFile) {
		t.Fatalf("open-directory error = %v", err)
	}
}

func TestLoadConfigRejectsDuplicateAndCaseVariantKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL validation intentionally fails closed until the documented ACL implementation is added")
	}
	projectPath := canonicalTempDir(t)
	for name, encoded := range map[string]string{
		"duplicate":        `{"version":1,"computerName":"Computer","listenHost":"100.64.0.10","listenHost":"192.168.1.10","listenPort":9443,"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
		"case variant":     `{"version":1,"computerName":"Computer","listenHost":"100.64.0.10","ListenHost":"192.168.1.10","listenPort":9443,"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
		"unicode fold":     `{"version":1,"computerName":"Computer","listenHost":"100.64.0.10","liſtenHost":"100.64.0.11","listenPort":9443,"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
		"nested duplicate": `{"version":1,"computerName":"Computer","listenHost":"100.64.0.10","listenPort":9443,"projects":[{"id":"main","id":"other","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(secureTempDir(t), "config.json")
			if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadConfig(path, filepath.Dir(path)); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("duplicate-key error = %v", err)
			}
		})
	}
}

func TestConfigRejectsUnsafeNetworkAndProjectShapes(t *testing.T) {
	base := Config{Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	for name, mutate := range map[string]func(*Config){
		"wildcard IPv4":          func(config *Config) { config.ListenHost = "0.0.0.0" },
		"wildcard IPv6":          func(config *Config) { config.ListenHost = "::" },
		"private LAN":            func(config *Config) { config.ListenHost = "192.168.1.10" },
		"public IP":              func(config *Config) { config.ListenHost = "8.8.8.8" },
		"unverified DNS":         func(config *Config) { config.ListenHost = "computer.example.com" },
		"missing computer":       func(config *Config) { config.ComputerName = "" },
		"invalid port":           func(config *Config) { config.ListenPort = 0 },
		"path-shaped project ID": func(config *Config) { config.Projects[0].ID = "../main" },
	} {
		t.Run(name, func(t *testing.T) {
			config := base
			config.Projects = append([]projects.Config(nil), base.Projects...)
			mutate(&config)
			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestConfigAcceptsOnlyTailscaleAddressRanges(t *testing.T) {
	base := Config{Version: 1, ComputerName: "Computer", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	for _, host := range []string{"100.64.0.1", "100.127.255.254", "fd7a:115c:a1e0::1"} {
		config := base
		config.ListenHost = host
		if err := config.Validate(); err != nil {
			t.Fatalf("Tailscale address %q rejected: %v", host, err)
		}
	}
	for _, host := range []string{"100.63.255.255", "100.128.0.1", "fd7a:115c:a1e1::1"} {
		config := base
		config.ListenHost = host
		if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("non-Tailscale address %q error = %v", host, err)
		}
	}
}

func TestRuntimeWiresSharedStoresWithoutChangingHostIdentityOrConfirmedResult(t *testing.T) {
	config := Config{Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	pairingStore := pairing.NewMemoryStore()
	promptStore := promptqueue.NewMemoryStore()
	eventStore := eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 32, MaxBytes: 64 * 1024})
	dependencies := Dependencies{
		PairingStore: pairingStore, PromptStore: promptStore, EventStore: eventStore, Random: rand.Reader,
	}

	first, err := NewRuntime(context.Background(), config, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	firstOffer, err := first.Pairing.BeginPairing(pairing.PairingTarget{Host: config.ListenHost, Port: config.ListenPort, Protocol: pairing.ProtocolMajor}, appNow)
	if err != nil {
		t.Fatal(err)
	}
	entry := promptqueue.Entry{ActionID: "action-1", ThreadID: "thread-1", ProjectID: "main", Prompt: "hello", CreatedAt: appNow}
	if err := first.Queue.Enqueue(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	want := promptqueue.Result{Code: "accepted", ThreadID: "thread-1", TurnID: "turn-1"}
	if _, err := first.Queue.DispatchNext(context.Background(), "thread-1", func(context.Context, promptqueue.Entry) (promptqueue.Result, error) { return want, nil }, nil, appNow); err != nil {
		t.Fatal(err)
	}

	second, err := NewRuntime(context.Background(), config, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	secondOffer, err := second.Pairing.BeginPairing(pairing.PairingTarget{Host: config.ListenHost, Port: config.ListenPort, Protocol: pairing.ProtocolMajor}, appNow)
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.Queue.Result(context.Background(), "action-1")
	if err != nil || got != want || firstOffer.HostPublicKey != secondOffer.HostPublicKey {
		t.Fatalf("restarted runtime result = %#v, error = %v, identity stable = %v", got, err, firstOffer.HostPublicKey == secondOffer.HostPublicKey)
	}
	if resolved, err := second.Projects.Resolve("main"); err != nil || resolved == "" {
		t.Fatalf("project resolve = %q, %v", resolved, err)
	}
	if second.Mobile == nil {
		t.Fatal("restarted runtime did not wire the mobile TLS transport")
	}
}

func TestPersistentRuntimeReopensTheSameProductionStores(t *testing.T) {
	config := Config{Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	statePath := filepath.Join(t.TempDir(), "state.sqlite3")
	first, firstStore, err := openPersistentRuntimeAt(context.Background(), config, PersistentDependencies{Random: rand.Reader}, statePath)
	if err != nil {
		t.Fatal(err)
	}
	entry := promptqueue.Entry{ActionID: "action-persistent", ThreadID: "thread-1", ProjectID: "main", Prompt: "hello", CreatedAt: appNow}
	if err := first.Queue.Enqueue(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatal(err)
	}
	second, secondStore, err := openPersistentRuntimeAt(context.Background(), config, PersistentDependencies{Random: rand.Reader}, statePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondStore.Close() })
	if _, err := second.Queue.DispatchNext(context.Background(), "thread-1", nil, nil, appNow); !errors.Is(err, promptqueue.ErrOutcomeUnknown) {
		t.Fatalf("reopened queued action error = %v", err)
	}
}

func TestRuntimeRejectsMissingStoresAndRandomSource(t *testing.T) {
	config := Config{Version: 1, ComputerName: "Computer", ListenHost: "100.64.0.10", ListenPort: 9443, Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	valid := Dependencies{
		PairingStore: pairing.NewMemoryStore(), PromptStore: promptqueue.NewMemoryStore(),
		EventStore: eventjournal.NewMemoryStore(eventjournal.Limits{MaxEvents: 4, MaxBytes: 1024}), Random: rand.Reader,
	}
	for name, mutate := range map[string]func(*Dependencies){
		"pairing store": func(dependencies *Dependencies) { dependencies.PairingStore = nil },
		"prompt store":  func(dependencies *Dependencies) { dependencies.PromptStore = nil },
		"event store":   func(dependencies *Dependencies) { dependencies.EventStore = nil },
		"random source": func(dependencies *Dependencies) { dependencies.Random = nil },
	} {
		t.Run(name, func(t *testing.T) {
			dependencies := valid
			mutate(&dependencies)
			if _, err := NewRuntime(context.Background(), config, dependencies); !errors.Is(err, ErrMissingDependency) {
				t.Fatalf("runtime error = %v", err)
			}
		})
	}
}

func writeConfig(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(secureTempDir(t), "config.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func secureTempDir(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func absoluteCodexPath() string {
	if runtime.GOOS == "windows" {
		return `C:\Program Files\Codex\codex.exe`
	}
	return "/Applications/ChatGPT.app/Contents/Resources/codex"
}

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
