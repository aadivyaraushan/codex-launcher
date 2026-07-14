package hostinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/operationlock"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/transaction"
)

func TestInstallReplaceRollbackAndUninstallLifecycle(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	migrations := 0
	manager := New(Options{
		Root: root, SourceExecutable: source, Backend: backend, Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		ValidateBinary: func(path string) error {
			body, err := os.ReadFile(path)
			if err != nil || (string(body) != "version-one" && string(body) != "version-two") {
				return errors.New("binary validation failed")
			}
			return nil
		},
		GOOS: "test", GOARCH: "test",
		MigrateConfig: func() (func() error, error) { migrations++; return func() error { return nil }, nil },
	})

	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("root mode = %v, error = %v", info.Mode().Perm(), err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"install:" + manager.BinaryPath(), "start"}) {
		t.Fatalf("install calls = %#v", backend.calls)
	}

	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")
	backend.calls = nil
	if err := manager.Replace(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-two")
	assertFileBody(t, manager.PreviousBinaryPath(), "version-one")
	if migrations != 1 || !reflect.DeepEqual(backend.calls, []string{"stop", "install:" + manager.BinaryPath(), "start"}) {
		t.Fatalf("replace migrations = %d, calls = %#v", migrations, backend.calls)
	}

	backend.calls = nil
	if err := manager.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if _, err := os.Stat(manager.PreviousBinaryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous binary survived one rollback: %v", err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"stop", "install:" + manager.BinaryPath(), "start"}) {
		t.Fatalf("rollback calls = %#v", backend.calls)
	}

	backend.calls = nil
	if err := manager.Uninstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private root survived uninstall: %v", err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"stop", "remove"}) {
		t.Fatalf("uninstall calls = %#v", backend.calls)
	}
}

func TestReplaceRejectsTamperingBeforeStoppingTheService(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")
	if err := os.WriteFile(artifact, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := manager.Replace(context.Background(), artifact); !errors.Is(err, ErrArtifactVerification) {
		t.Fatalf("replace error = %v", err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if len(backend.calls) != 0 {
		t.Fatalf("service changed before verification: %#v", backend.calls)
	}
}

func TestStageVerifiedArtifactPublishesOnlyTheBytesHashedAgainstMetadata(t *testing.T) {
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")
	staged := filepath.Join(t.TempDir(), "staged")
	if _, err := stageVerifiedArtifact(artifact, staged, "test", "test"); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, staged, "version-two")

	if err := os.WriteFile(artifact+".sha256", []byte(strings.Repeat("0", 64)+"  "+filepath.Base(artifact)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	badStage := filepath.Join(t.TempDir(), "bad-stage")
	if _, err := stageVerifiedArtifact(artifact, badStage, "test", "test"); !errors.Is(err, ErrArtifactVerification) {
		t.Fatalf("stage error = %v", err)
	}
	if _, err := os.Stat(badStage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mismatched bytes were staged: %v", err)
	}
}

func TestReplacementActivationNeverRemovesTheCurrentBinaryBeforePublish(t *testing.T) {
	root := t.TempDir()
	manager := New(Options{Root: root, GOOS: "test", GOARCH: "test"})
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, manager.BinaryPath(), "version-one")
	if err := manager.activateReplacement(filepath.Join(root, "missing-staged-binary")); err == nil {
		t.Fatal("activation unexpectedly passed")
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
}

func TestFailedMigrationRestoresTheOldBinaryAndRestartsIt(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	manager := New(Options{
		Root: root, SourceExecutable: source, Backend: backend,
		ValidateBinary: func(string) error { return nil },
		GOOS:           "test", GOARCH: "test",
		MigrateConfig: func() (func() error, error) { return nil, errors.New("migration failed") },
	})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")

	if err := manager.Replace(context.Background(), artifact); !errors.Is(err, ErrReplacementRolledBack) {
		t.Fatalf("replace error = %v", err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if _, err := os.Stat(manager.PreviousBinaryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed replacement left previous binary: %v", err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"stop", "stop", "install:" + manager.BinaryPath(), "start"}) {
		t.Fatalf("rollback calls = %#v", backend.calls)
	}
}

func TestFailedReplacementStartRollsBackConfigAndRestoresOldBinary(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	configRollbacks := 0
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte("version-one-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(Options{
		Root: root, SourceExecutable: source, Backend: backend,
		ValidateBinary: func(string) error { return nil },
		GOOS:           "test", GOARCH: "test",
		MigrateConfig: func() (func() error, error) {
			original, err := os.ReadFile(configPath)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(configPath, []byte("version-two-config"), 0o600); err != nil {
				return nil, err
			}
			return func() error {
				configRollbacks++
				return os.WriteFile(configPath, original, 0o600)
			}, nil
		},
	})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	backend.startErrors = []error{errors.New("new service failed"), nil}
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")

	if err := manager.Replace(context.Background(), artifact); !errors.Is(err, ErrReplacementRolledBack) {
		t.Fatalf("replace error = %v", err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if configRollbacks != 1 {
		t.Fatalf("config rollbacks = %d", configRollbacks)
	}
	assertFileBody(t, configPath, "version-one-config")
	wantCalls := []string{"stop", "install:" + manager.BinaryPath(), "start", "stop", "install:" + manager.BinaryPath(), "start"}
	if !reflect.DeepEqual(backend.calls, wantCalls) {
		t.Fatalf("replacement failure calls = %#v, want %#v", backend.calls, wantCalls)
	}
}

func TestFailedRollbackActivationRestoresTheNewerWorkingBinary(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")
	if err := manager.Replace(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	backend.startErrors = []error{errors.New("old service failed"), nil}

	if err := manager.Rollback(context.Background()); !errors.Is(err, ErrRollbackRolledBack) {
		t.Fatalf("rollback error = %v", err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-two")
	assertFileBody(t, manager.PreviousBinaryPath(), "version-one")
	wantCalls := []string{"stop", "install:" + manager.BinaryPath(), "start", "stop", "install:" + manager.BinaryPath(), "start"}
	if !reflect.DeepEqual(backend.calls, wantCalls) {
		t.Fatalf("rollback failure calls = %#v, want %#v", backend.calls, wantCalls)
	}
}

func TestSuccessfulReplacementKeepsConfigRollbackAcrossManagerProcesses(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte("version-one-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	first := New(Options{
		Root: root, SourceExecutable: source, Backend: backend,
		ValidateBinary: func(string) error { return nil }, GOOS: "test", GOARCH: "test",
		MigrateConfig: func() (func() error, error) {
			return nil, os.WriteFile(configPath, []byte("version-two-config"), 0o600)
		},
	})
	if err := first.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")
	if err := first.Replace(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, configPath, "version-two-config")
	assertFileBody(t, first.PreviousConfigPath(), "version-one-config")

	second := New(Options{Root: root, Backend: backend, ValidateBinary: func(string) error { return nil }, GOOS: "test", GOARCH: "test"})
	if err := second.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, configPath, "version-one-config")
	if _, err := os.Stat(second.PreviousConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous config survived rollback: %v", err)
	}
}

func TestInstallRejectsAnOversizedSourceWithoutPublishingIt(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "codex-launcher")
	file, err := os.OpenFile(source, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxArtifactBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})

	if err := manager.Install(context.Background()); !errors.Is(err, ErrArtifactVerification) {
		t.Fatalf("install error = %v", err)
	}
	if _, err := os.Stat(manager.BinaryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized binary was published: %v", err)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("backend calls = %#v", backend.calls)
	}
}

func TestFailedInitialInstallStopsAndRemovesEveryPartialService(t *testing.T) {
	for _, test := range []struct {
		name          string
		installErrors []error
		startErrors   []error
		wantCalls     []string
	}{
		{
			name: "backend install failed", installErrors: []error{errors.New("bootstrap failed")},
			wantCalls: []string{"install:BIN", "stop", "remove"},
		},
		{
			name: "backend start failed", startErrors: []error{errors.New("start failed")},
			wantCalls: []string{"install:BIN", "start", "stop", "remove"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
			backend := &recordingBackend{installErrors: test.installErrors, startErrors: test.startErrors}
			manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
			if err := manager.Install(context.Background()); err == nil {
				t.Fatal("install unexpectedly passed")
			}
			for index := range test.wantCalls {
				test.wantCalls[index] = strings.ReplaceAll(test.wantCalls[index], "BIN", manager.BinaryPath())
			}
			if !reflect.DeepEqual(backend.calls, test.wantCalls) {
				t.Fatalf("calls = %#v, want %#v", backend.calls, test.wantCalls)
			}
			if _, err := os.Stat(manager.BinaryPath()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed install left binary: %v", err)
			}
		})
	}
}

func TestFailedInitialInstallReportsRepairWhenStopCleanupFails(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{
		startErrors: []error{errors.New("start failed")},
		stopErrors:  []error{errors.New("stop failed")},
	}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
	if err := manager.Install(context.Background()); !errors.Is(err, ErrInstallNeedsRepair) {
		t.Fatalf("install error = %v", err)
	}
	want := []string{"install:" + manager.BinaryPath(), "start", "stop", "remove"}
	if !reflect.DeepEqual(backend.calls, want) {
		t.Fatalf("cleanup calls = %#v, want %#v", backend.calls, want)
	}
}

func TestUninstallStopsBeforeRemovingAnythingAndReturnsStopFailure(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{stopErrors: []error{errors.New("stop failed")}}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	if err := manager.Uninstall(context.Background()); err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("uninstall error = %v", err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"stop"}) {
		t.Fatalf("uninstall calls = %#v", backend.calls)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
}

func TestUninstallPreservesPrivateFilesWhenServiceRemovalFails(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{removeErrors: []error{errors.New("remove failed")}}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	if err := manager.Uninstall(context.Background()); err == nil {
		t.Fatal("uninstall unexpectedly passed")
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if !reflect.DeepEqual(backend.calls, []string{"stop", "remove"}) {
		t.Fatalf("uninstall calls = %#v", backend.calls)
	}
}

func TestInstallRequiresHealthFromTheExactPreparedStartAttempt(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	health := &recordingStartHealth{attempt: "22222222222222222222222222222222", waitErr: errors.New("matching start failed")}
	manager := New(Options{
		Root: root, SourceExecutable: source, Backend: backend, StartHealth: health,
		GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil },
	})
	if err := manager.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "matching start failed") {
		t.Fatalf("install error = %v", err)
	}
	if !reflect.DeepEqual(health.waited, []string{health.attempt}) {
		t.Fatalf("waited attempts = %#v", health.waited)
	}
	want := []string{"install:" + manager.BinaryPath(), "start", "stop", "remove"}
	if !reflect.DeepEqual(backend.calls, want) {
		t.Fatalf("calls = %#v, want %#v", backend.calls, want)
	}
}

func TestInstalledWindowsBinaryDefersMutationsBeforeTouchingLockedFiles(t *testing.T) {
	root := t.TempDir()
	backend := &recordingBackend{}
	deferred := &recordingDeferredMutator{deferOperation: true}
	manager := New(Options{Root: root, Backend: backend, DeferredMutator: deferred, GOOS: "windows", GOARCH: "amd64", ValidateBinary: func(string) error { return nil }})
	artifact := filepath.Join(t.TempDir(), "replacement.exe")
	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"replace", func() error { return manager.Replace(context.Background(), artifact) }},
		{"rollback", func() error { return manager.Rollback(context.Background()) }},
		{"uninstall", func() error { return manager.Uninstall(context.Background()) }},
	} {
		if err := operation.run(); !errors.Is(err, ErrMaintenanceScheduled) {
			t.Fatalf("%s error = %v", operation.name, err)
		}
	}
	if !reflect.DeepEqual(deferred.operations, []string{"replace:" + artifact, "rollback:", "uninstall:"}) {
		t.Fatalf("deferred operations = %#v", deferred.operations)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("locked install was touched: %#v", backend.calls)
	}
}

func TestReconfigureStopsTheRunningServiceAppliesConfigAndVerifiesRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte("old-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := manager.Reconfigure(context.Background(), func() error {
		return os.WriteFile(configPath, []byte("new-config"), 0o600)
	}); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, configPath, "new-config")
	if !reflect.DeepEqual(backend.calls, []string{"status", "stop", "start"}) {
		t.Fatalf("reconfigure calls = %#v", backend.calls)
	}
}

func TestFailedReconfigureRestartRestoresOldConfigAndService(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte("old-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{startErrors: []error{errors.New("new config failed"), nil}}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := manager.Reconfigure(context.Background(), func() error {
		return os.WriteFile(configPath, []byte("new-config"), 0o600)
	}); err == nil {
		t.Fatal("reconfigure unexpectedly passed")
	}
	assertFileBody(t, configPath, "old-config")
	if !reflect.DeepEqual(backend.calls, []string{"status", "stop", "start", "stop", "start"}) {
		t.Fatalf("reconfigure recovery calls = %#v", backend.calls)
	}
}

func TestFailedReconfigureRepairReportsFailureToStopTheNewProcess(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte("old-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	stopFailure := errors.New("new process would not stop")
	backend := &recordingBackend{
		startErrors: []error{errors.New("new config health failed")},
		stopErrors:  []error{nil, stopFailure},
	}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})

	err := manager.Reconfigure(context.Background(), func() error {
		return os.WriteFile(configPath, []byte("new-config"), 0o600)
	})
	if !errors.Is(err, ErrReconfigureNeedsRepair) || !errors.Is(err, stopFailure) {
		t.Fatalf("reconfigure error = %v", err)
	}
	assertFileBody(t, configPath, "new-config")
}

func TestManagerRejectsConcurrentMutationsBeforeTouchingSharedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte("old-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(Options{Root: root, Backend: &recordingBackend{}, GOOS: "test", GOARCH: "test"})
	entered := make(chan struct{})
	releaseApply := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- manager.Reconfigure(context.Background(), func() error {
			close(entered)
			<-releaseApply
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first mutation did not acquire the lock")
	}
	if err := manager.Uninstall(context.Background()); !errors.Is(err, operationlock.ErrBusy) {
		t.Fatalf("concurrent mutation error = %v", err)
	}
	close(releaseApply)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestRecoverRestoresTheKnownPreviousVersionAfterInterruptedReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, manager.BinaryPath(), "version-two")
	transactionBinary := filepath.Join(root, "install-transaction", "binary")
	if err := os.MkdirAll(filepath.Dir(transactionBinary), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, transactionBinary, "version-one")
	if err := os.WriteFile(filepath.Join(filepath.Dir(transactionBinary), "config.absent"), []byte("absent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := transaction.New(filepath.Join(root, "install-transaction.json"))
	if err := journal.Write(transaction.Record{Version: 1, Operation: transaction.OperationReplace, Phase: transaction.PhaseActivated}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if _, err := journal.Read(); !errors.Is(err, transaction.ErrMissing) {
		t.Fatalf("journal survived recovery: %v", err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"stop", "install:" + manager.BinaryPath(), "start"}) {
		t.Fatalf("recovery calls = %#v", backend.calls)
	}
}

func TestInterruptedReplacementRecoveryCanRetryAfterItsFirstRestartFails(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{startErrors: []error{errors.New("first recovery start failed"), nil}}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, manager.BinaryPath(), "version-two")
	transactionBinary := filepath.Join(root, "install-transaction", "binary")
	if err := os.MkdirAll(filepath.Dir(transactionBinary), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, transactionBinary, "version-one")
	if err := os.WriteFile(filepath.Join(filepath.Dir(transactionBinary), "config.absent"), []byte("absent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := transaction.New(filepath.Join(root, "install-transaction.json"))
	if err := journal.Write(transaction.Record{Version: 1, Operation: transaction.OperationReplace, Phase: transaction.PhaseActivated}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Recover(context.Background()); !errors.Is(err, ErrInstallNeedsRepair) {
		t.Fatalf("first recovery error = %v", err)
	}
	assertFileBody(t, transactionBinary, "version-one")
	if _, err := journal.Read(); err != nil {
		t.Fatalf("journal did not survive failed recovery: %v", err)
	}
	if err := manager.Recover(context.Background()); err != nil {
		t.Fatalf("second recovery failed: %v", err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if _, err := journal.Read(); !errors.Is(err, transaction.ErrMissing) {
		t.Fatalf("journal survived successful recovery: %v", err)
	}
}

func TestPreparedRollbackRecoveryKeepsTheNewerVersionAndRollbackChoice(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, manager.BinaryPath(), "version-two")
	writeExecutable(t, manager.PreviousBinaryPath(), "version-one")
	transactionBinary := filepath.Join(root, "install-transaction", "binary")
	if err := os.MkdirAll(filepath.Dir(transactionBinary), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, transactionBinary, "version-two")
	if err := os.WriteFile(filepath.Join(filepath.Dir(transactionBinary), "config.absent"), []byte("absent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := transaction.New(filepath.Join(root, "install-transaction.json"))
	if err := journal.Write(transaction.Record{Version: 1, Operation: transaction.OperationRollback, Phase: transaction.PhasePrepared}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-two")
	assertFileBody(t, manager.PreviousBinaryPath(), "version-one")
	if _, err := journal.Read(); !errors.Is(err, transaction.ErrMissing) {
		t.Fatalf("journal survived recovery: %v", err)
	}
}

func TestReplacementWritesPreparedJournalAndSnapshotBeforeStopping(t *testing.T) {
	root := t.TempDir()
	source := writeExecutable(t, filepath.Join(t.TempDir(), "codex-launcher"), "version-one")
	backend := &recordingBackend{}
	manager := New(Options{Root: root, SourceExecutable: source, Backend: backend, GOOS: "test", GOARCH: "test", ValidateBinary: func(string) error { return nil }})
	if err := manager.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	backend.stopHook = func() {
		record, err := transaction.New(filepath.Join(root, "install-transaction.json")).Read()
		if err != nil || record.Operation != transaction.OperationReplace || record.Phase != transaction.PhasePrepared {
			t.Fatalf("journal at stop = %#v, error = %v", record, err)
		}
		assertFileBody(t, filepath.Join(root, "install-transaction", "binary"), "version-one")
	}
	artifact := writeVerifiedArtifact(t, filepath.Join(t.TempDir(), "codex-launcher-v2"), "version-two")

	if err := manager.Replace(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
}

func TestHealthyReplacementRecoveryPublishesRollbackWithoutStoppingService(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, manager.BinaryPath(), "version-two")
	transactionBinary := filepath.Join(root, "install-transaction", "binary")
	if err := os.MkdirAll(filepath.Dir(transactionBinary), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, transactionBinary, "version-one")
	if err := os.WriteFile(filepath.Join(filepath.Dir(transactionBinary), "config.absent"), []byte("absent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := transaction.New(filepath.Join(root, "install-transaction.json"))
	if err := journal.Write(transaction.Record{Version: 1, Operation: transaction.OperationReplace, Phase: transaction.PhaseHealthy}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-two")
	assertFileBody(t, manager.PreviousBinaryPath(), "version-one")
	if len(backend.calls) != 0 {
		t.Fatalf("healthy service was changed during recovery: %#v", backend.calls)
	}
}

func TestHealthyRollbackRecoveryOnlyFinishesCleanup(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}
	manager := New(Options{Root: root, Backend: backend, GOOS: "test", GOARCH: "test"})
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, manager.BinaryPath(), "version-one")
	writeExecutable(t, manager.PreviousBinaryPath(), "version-one")
	transactionBinary := filepath.Join(root, "install-transaction", "binary")
	if err := os.MkdirAll(filepath.Dir(transactionBinary), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, transactionBinary, "version-two")
	if err := os.WriteFile(filepath.Join(filepath.Dir(transactionBinary), "config.absent"), []byte("absent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := transaction.New(filepath.Join(root, "install-transaction.json"))
	if err := journal.Write(transaction.Record{Version: 1, Operation: transaction.OperationRollback, Phase: transaction.PhaseHealthy}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFileBody(t, manager.BinaryPath(), "version-one")
	if _, err := os.Stat(manager.PreviousBinaryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed rollback survived recovery: %v", err)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("healthy service was changed during recovery: %#v", backend.calls)
	}
}

type recordingBackend struct {
	calls         []string
	installErrors []error
	startErrors   []error
	stopErrors    []error
	removeErrors  []error
	stopHook      func()
}

type recordingStartHealth struct {
	attempt    string
	prepareErr error
	waitErr    error
	waited     []string
}

type recordingDeferredMutator struct {
	deferOperation bool
	operations     []string
}

func (mutator *recordingDeferredMutator) DeferIfRunningInstalled(operation, artifact, _ string) (bool, error) {
	mutator.operations = append(mutator.operations, operation+":"+artifact)
	return mutator.deferOperation, nil
}

func (health *recordingStartHealth) PrepareStart() (string, error) {
	return health.attempt, health.prepareErr
}

func (health *recordingStartHealth) WaitRunning(_ context.Context, attempt string) error {
	health.waited = append(health.waited, attempt)
	return health.waitErr
}

func (backend *recordingBackend) Install(_ context.Context, binaryPath string) error {
	backend.calls = append(backend.calls, "install:"+binaryPath)
	if len(backend.installErrors) != 0 {
		err := backend.installErrors[0]
		backend.installErrors = backend.installErrors[1:]
		return err
	}
	return nil
}
func (backend *recordingBackend) Start(context.Context) error {
	backend.calls = append(backend.calls, "start")
	if len(backend.startErrors) != 0 {
		err := backend.startErrors[0]
		backend.startErrors = backend.startErrors[1:]
		return err
	}
	return nil
}
func (backend *recordingBackend) Stop(context.Context) error {
	backend.calls = append(backend.calls, "stop")
	if backend.stopHook != nil {
		backend.stopHook()
	}
	if len(backend.stopErrors) != 0 {
		err := backend.stopErrors[0]
		backend.stopErrors = backend.stopErrors[1:]
		return err
	}
	return nil
}
func (backend *recordingBackend) Remove(context.Context) error {
	backend.calls = append(backend.calls, "remove")
	if len(backend.removeErrors) != 0 {
		err := backend.removeErrors[0]
		backend.removeErrors = backend.removeErrors[1:]
		return err
	}
	return nil
}
func (backend *recordingBackend) Status(context.Context) (ServiceStatus, error) {
	backend.calls = append(backend.calls, "status")
	return ServiceStatus{Installed: true, Running: true}, nil
}

func writeExecutable(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeVerifiedArtifact(t *testing.T, path, body string) string {
	t.Helper()
	writeExecutable(t, path, body)
	digest := sha256.Sum256([]byte(body))
	checksum := hex.EncodeToString(digest[:])
	if err := os.WriteFile(path+".sha256", []byte(checksum+"  "+filepath.Base(path)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := Provenance{Artifact: filepath.Base(path), SHA256: checksum, Version: "0.2.0", GOOS: "test", GOARCH: "test", SourceCommit: "0123456789abcdef"}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".provenance.json", encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || string(body) != want {
		t.Fatalf("file %s = %q, error = %v", path, body, err)
	}
}
