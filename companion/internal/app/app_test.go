package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

// testRelayConfig returns a RelayConfig that passes Validate, for tests that
// only care about some other field. PinnedKey is base64 of an arbitrary
// non-empty byte slice — Validate only checks it decodes, not that it's a
// real key.
func testRelayConfig() RelayConfig {
	return RelayConfig{
		BoxHost: "relay.example.com", MacPort: 9000, PhonePort: 8443,
		PinnedKey: base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")), Secret: "relay-secret-value",
	}
}

var appNow = time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC)

// TestValidateRejectsWeakRelaySecret proves Config.Validate refuses a
// registration secret that is too short or carries control characters. The
// relay secret is now the only application-level access gate (the Tailscale
// network gate is gone), so a trivially guessable or malformed secret must be
// rejected at the config boundary, before it is ever persisted or dialed.
func TestValidateRejectsWeakRelaySecret(t *testing.T) {
	weak := []struct {
		name   string
		secret string
	}{
		{"empty", ""},
		{"too short", "short"},
		{"whitespace padded but short", "   short   "},
		{"embedded newline", strings.Repeat("a", 20) + "\n"},
		{"embedded control char", strings.Repeat("a", 20) + "\x01"},
	}
	for _, tc := range weak {
		relay := testRelayConfig()
		relay.Secret = tc.secret
		config := Config{Version: 1, ComputerName: "Computer", Relay: relay, CodexBinary: absoluteCodexPath(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
		if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: Validate accepted secret %q (err = %v), want ErrInvalidConfig", tc.name, tc.secret, err)
		}
	}

	// A clean secret at the minimum length is accepted (the good path testRelayConfig relies on).
	if err := (Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), CodexBinary: absoluteCodexPath(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}).Validate(); err != nil {
		t.Fatalf("Validate rejected a well-formed config: %v", err)
	}
}

func TestLoadConfigAcceptsStrictOwnerOnlyConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-mode assertions are POSIX-specific; Windows ACL behavior has its own native tests")
	}
	projectPath := canonicalTempDir(t)
	path := writeConfig(t, map[string]any{
		"version":      1,
		"computerName": "Aadi's Mac",
		"relay": map[string]any{
			"boxHost": "relay.example.com", "macPort": 9000, "phonePort": 8443,
			"pinnedKey": base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")), "secret": "relay-secret-value",
		},
		"codexBinary": absoluteCodexPath(),
		"projects":    []map[string]string{{"id": "launcher", "displayName": "Codex Launcher", "path": projectPath}},
	})

	config, err := loadConfig(path, filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != 2 || config.ComputerName != "Aadi's Mac" || config.Connection.Mode != ConnectionModeRelay || config.Relay.BoxHost != "relay.example.com" || config.Relay.MacPort != 9000 || config.Relay.PhonePort != 8443 || len(config.Projects) != 1 {
		t.Fatalf("config = %#v", config)
	}
}

func TestLoadConfigNormalizesRecognizedLegacyConfigsWithoutRewritingThem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-mode assertions are POSIX-specific")
	}
	projectPath := canonicalTempDir(t)
	for name, encoded := range map[string]string{
		"relay":     `{"version":1,"computerName":"Computer","codexBinary":` + quoted(absoluteCodexPath()) + `,"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}],"relay":{"boxHost":"relay.example.com","macPort":9000,"phonePort":8443,"pinnedKey":"` + base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")) + `","secret":"relay-secret-value"}}`,
		"tailscale": `{"version":1,"computerName":"Computer","codexBinary":` + quoted(absoluteCodexPath()) + `,"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}],"listenHost":"100.64.0.10","listenPort":9443}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(secureTempDir(t), "config.json")
			if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
				t.Fatal(err)
			}
			config, source, err := loadConfigWithSource(path, filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if source == ConfigSourceV2 || config.Version != 2 {
				t.Fatalf("source = %q, config = %#v", source, config)
			}
			if name == "relay" && config.Connection.Mode != ConnectionModeRelay {
				t.Fatalf("relay legacy config mode = %q", config.Connection.Mode)
			}
			if name == "tailscale" && (config.Connection.Mode != ConnectionModeTailscale || config.Connection.Tailscale.Host != "100.64.0.10") {
				t.Fatalf("tailscale legacy config = %#v", config.Connection)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != encoded {
				t.Fatalf("legacy config changed: got %q, error = %v", got, err)
			}
		})
	}
}

func TestWriteConfigAlwaysPublishesVersion2ConnectionChoice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-mode assertions are POSIX-specific")
	}
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	legacy := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	if err := writeConfigAt(path, root, legacy); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"version": 2`) || !strings.Contains(string(encoded), `"connection"`) || strings.Contains(string(encoded), `"listenHost"`) {
		t.Fatalf("new config did not use the version 2 connection shape: %s", encoded)
	}
}

func TestConfigRejectsMixedAndInvalidTailscaleConnectionShapes(t *testing.T) {
	base := Config{Version: 2, ComputerName: "Computer", Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}, Connection: ConnectionConfig{Mode: ConnectionModeTailscale, Tailscale: TailscaleConfig{Host: "100.64.0.10", Port: 9443}}}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid tailscale config rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"outside tailscale range": func(config *Config) { config.Connection.Tailscale.Host = "100.63.255.255" },
		"noncanonical host":       func(config *Config) { config.Connection.Tailscale.Host = "100.064.0.10" },
		"relay mixed in":          func(config *Config) { config.Connection.Relay = testRelayConfig() },
		"missing mode":            func(config *Config) { config.Connection.Mode = "" },
	} {
		t.Run(name, func(t *testing.T) {
			config := base
			mutate(&config)
			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestWriteConfigPublishesOwnerOnlyFileOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-mode assertions are POSIX-specific; Windows ACL behavior has its own native tests")
	}
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	config := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), CodexBinary: absoluteCodexPath(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
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
		t.Skip("permission-mode assertions are POSIX-specific; Windows ACL behavior has its own native tests")
	}
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	config := Config{Version: 1, ComputerName: ""}
	if err := writeConfigAt(path, root, config); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid config write error = %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config root created for invalid input: %v", err)
	}
}

func TestUpdateConfigAtomicallyChangesTheApprovedProjectFolders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs in the Windows native configsecurity test")
	}
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	firstPath := canonicalTempDir(t)
	secondPath := canonicalTempDir(t)
	first := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), CodexBinary: absoluteCodexPath(), Projects: []projects.Config{{ID: "first", DisplayName: "First", Path: firstPath}}}
	if err := writeConfigAt(path, root, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Projects = []projects.Config{{ID: "second", DisplayName: "Second", Path: secondPath}}
	if err := updateConfigAt(path, root, second); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path, root)
	if err != nil || !reflect.DeepEqual(loaded.Projects, second.Projects) {
		t.Fatalf("updated config = %#v, error = %v", loaded, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".config-*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %#v", matches)
	}
}

func TestLoadConfigRejectsUnknownOversizedSymlinkAndOpenPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-mode assertions are POSIX-specific; Windows ACL behavior has its own native tests")
	}
	valid := map[string]any{
		"version": 1, "computerName": "Computer",
		"relay": map[string]any{
			"boxHost": "relay.example.com", "macPort": 9000, "phonePort": 8443,
			"pinnedKey": base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")), "secret": "relay-secret-value",
		},
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
		t.Skip("permission-mode assertions are POSIX-specific; Windows ACL behavior has its own native tests")
	}
	projectPath := canonicalTempDir(t)
	relay := `"macPort":9000,"phonePort":8443,"pinnedKey":"` + base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")) + `","secret":"relay-secret-value"`
	for name, encoded := range map[string]string{
		"duplicate":        `{"version":1,"computerName":"Computer","relay":{"boxHost":"a.example.com","boxHost":"b.example.com",` + relay + `},"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
		"case variant":     `{"version":1,"computerName":"Computer","relay":{"boxHost":"a.example.com","BoxHost":"b.example.com",` + relay + `},"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
		"unicode fold":     `{"version":1,"computerName":"Computer","relay":{"boxHoſt":"a.example.com","boxHost":"b.example.com",` + relay + `},"projects":[{"id":"main","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
		"nested duplicate": `{"version":1,"computerName":"Computer","relay":{"boxHost":"a.example.com",` + relay + `},"projects":[{"id":"main","id":"other","displayName":"Main","path":` + quoted(projectPath) + `}]}`,
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
	base := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	for name, mutate := range map[string]func(*Config){
		"missing computer":             func(config *Config) { config.ComputerName = "" },
		"path-shaped project ID":       func(config *Config) { config.Projects[0].ID = "../main" },
		"empty box host":               func(config *Config) { config.Relay.BoxHost = "" },
		"box host with a space":        func(config *Config) { config.Relay.BoxHost = "relay example.com" },
		"box host too long":            func(config *Config) { config.Relay.BoxHost = strings.Repeat("a", 254) },
		"invalid mac port":             func(config *Config) { config.Relay.MacPort = 0 },
		"invalid phone port":           func(config *Config) { config.Relay.PhonePort = 70000 },
		"mac and phone port collide":   func(config *Config) { config.Relay.PhonePort = config.Relay.MacPort },
		"non-base64 pinned key":        func(config *Config) { config.Relay.PinnedKey = "not valid base64!!" },
		"empty pinned key":             func(config *Config) { config.Relay.PinnedKey = "" },
		"empty relay secret":           func(config *Config) { config.Relay.Secret = "" },
		"whitespace-only relay secret": func(config *Config) { config.Relay.Secret = "   " },
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

// TestConfigAcceptsPlausibleRelayHostsNotJustTailscaleRanges is the relay
// era's replacement for the old Tailscale-only address gate: BoxHost is a
// public relay box's hostname or IP now, so any non-empty, control-char-free
// host under the length cap must validate — not just addresses in the
// Tailscale CGNAT ranges.
func TestConfigAcceptsPlausibleRelayHostsNotJustTailscaleRanges(t *testing.T) {
	base := Config{Version: 1, ComputerName: "Computer", Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	for _, host := range []string{"relay-box.fly.dev", "203.0.113.10", "100.64.0.1", "my-relay-box", "fd7a:115c:a1e0::1"} {
		config := base
		config.Relay = testRelayConfig()
		config.Relay.BoxHost = host
		if err := config.Validate(); err != nil {
			t.Fatalf("relay host %q rejected: %v", host, err)
		}
	}
}

func TestRuntimeWiresSharedStoresWithoutChangingHostIdentityOrConfirmedResult(t *testing.T) {
	config := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
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
	firstOffer, err := first.Pairing.BeginPairing(pairing.PairingTarget{Host: config.Relay.BoxHost, Port: config.Relay.PhonePort, Protocol: pairing.ProtocolMajor}, appNow)
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
	secondOffer, err := second.Pairing.BeginPairing(pairing.PairingTarget{Host: config.Relay.BoxHost, Port: config.Relay.PhonePort, Protocol: pairing.ProtocolMajor}, appNow)
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
	config := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
	statePath := filepath.Join(t.TempDir(), "state.sqlite3")
	first, firstStore, err := openPersistentRuntimeAt(context.Background(), config, PersistentDependencies{Random: rand.Reader}, statePath)
	if err != nil {
		t.Fatal(err)
	}
	entry := promptqueue.Entry{ActionID: "action-persistent", ThreadID: "thread-1", ProjectID: "main", Prompt: "hello", CreatedAt: appNow}
	if err := first.Queue.Enqueue(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	payload := []byte("partial attachment")
	digest := sha256.Sum256(payload)
	attachmentNow := time.Unix(1_800_000_000, 0)
	if first.Attachments == nil {
		t.Fatal("persistent runtime did not wire attachment storage")
	}
	if _, err := first.Attachments.Begin("phone-1", "upload-1", int64(len(payload)), fmt.Sprintf("%x", digest[:]), attachmentNow); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Attachments.Append("phone-1", "upload-1", 0, 0, false, payload[:4], attachmentNow); err != nil {
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
	retained, err := second.Attachments.Retained("phone-1", "upload-1", attachmentNow)
	if err != nil || retained.NextChunk != 1 || string(retained.Received) != string(payload[:4]) {
		t.Fatalf("reopened attachment = %#v, %v", retained, err)
	}
}

func TestRuntimeRejectsMissingStoresAndRandomSource(t *testing.T) {
	config := Config{Version: 1, ComputerName: "Computer", Relay: testRelayConfig(), Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: canonicalTempDir(t)}}}
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
