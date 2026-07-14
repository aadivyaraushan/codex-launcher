package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/launchd"
)

func main() {
	if runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "native host-install smoke currently supports macOS only")
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "native host-install smoke failed:", err)
		os.Exit(1)
	}
	fmt.Println("native host-install smoke: install, replace, rollback, and uninstall passed")
}

func run() error {
	ctx := context.Background()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(current.Uid)
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "codex-launcher-native-install-")
	if err != nil {
		return err
	}
	backend := launchd.New(launchd.Options{Home: home, UID: uid})
	preflight, err := backend.Status(ctx)
	if err != nil || preflight.Installed || preflight.Running {
		_ = os.RemoveAll(root)
		return fmt.Errorf("existing companion launch agent prevents isolated smoke: status=%#v: %w", preflight, err)
	}
	manager := hostinstall.New(hostinstall.Options{
		Root: root, SourceExecutable: filepath.Join(root, "source-v1"), Backend: backend,
		ValidateBinary: validateBinary, GOOS: "darwin", GOARCH: runtime.GOARCH,
	})
	defer func() {
		_ = backend.Stop(ctx)
		_ = backend.Remove(ctx)
		_ = os.RemoveAll(root)
	}()
	if err := writeService(managerSource(manager), "smoke-v1"); err != nil {
		return err
	}
	if err := manager.Install(ctx); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if err := requireRunning(ctx, backend, "install"); err != nil {
		return err
	}
	replacement := filepath.Join(root, "source-v2")
	if err := writeService(replacement, "smoke-v2"); err != nil {
		return err
	}
	if err := writeMetadata(replacement, "smoke-v2"); err != nil {
		return err
	}
	if err := manager.Replace(ctx, replacement); err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	if err := requireRunning(ctx, backend, "replace"); err != nil {
		return err
	}
	if err := manager.Rollback(ctx); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	if err := requireRunning(ctx, backend, "rollback"); err != nil {
		return err
	}
	if err := manager.Uninstall(ctx); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	status, err := backend.Status(ctx)
	if err != nil || status.Installed || status.Running {
		return fmt.Errorf("uninstall status = %#v: %w", status, err)
	}
	return nil
}

func managerSource(manager *hostinstall.Manager) string {
	return filepath.Join(filepath.Dir(filepath.Dir(manager.BinaryPath())), "source-v1")
}

func writeService(path, version string) error {
	body := "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo 'codex-launcher " + version + "'; exit 0; fi\nif [ \"${1:-}\" = serve ]; then trap 'exit 0' TERM INT; while :; do sleep 1; done; fi\nexit 2\n"
	return os.WriteFile(path, []byte(body), 0o700)
}

func validateBinary(path string) error {
	output, err := exec.Command(path, "version").CombinedOutput()
	if err != nil || len(output) == 0 {
		return errors.New("dummy service version check failed")
	}
	return nil
}

func requireRunning(ctx context.Context, backend *launchd.Backend, operation string) error {
	status, err := backend.Status(ctx)
	if err != nil || !status.Installed || !status.Running {
		details, _ := exec.Command("launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())+"/"+launchd.Label).CombinedOutput()
		return fmt.Errorf("%s status = %#v: %v\n%s", operation, status, err, details)
	}
	return nil
}

func writeMetadata(path, version string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	checksum := hex.EncodeToString(digest[:])
	if err := os.WriteFile(path+".sha256", []byte(checksum+"  "+filepath.Base(path)+"\n"), 0o600); err != nil {
		return err
	}
	provenance := hostinstall.Provenance{Artifact: filepath.Base(path), SHA256: checksum, Version: version, GOOS: "darwin", GOARCH: runtime.GOARCH, SourceCommit: "0123456789abcdef"}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".provenance.json", append(encoded, '\n'), 0o600)
}
