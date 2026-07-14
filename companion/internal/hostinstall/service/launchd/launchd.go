package launchd

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service"
)

const Label = "app.codexlauncher.companion"

type Options struct {
	Home         string
	UID          int
	Runner       service.Runner
	Logger       *slog.Logger
	WaitUnloaded func(context.Context, string) error
}

type Backend struct {
	home         string
	uid          int
	runner       service.Runner
	logger       *slog.Logger
	waitUnloaded func(context.Context, string) error
}

func New(options Options) *Backend {
	runner := options.Runner
	if runner == nil {
		runner = service.CommandRunner{}
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	backend := &Backend{home: options.Home, uid: options.UID, runner: runner, logger: logger, waitUnloaded: options.WaitUnloaded}
	if backend.waitUnloaded == nil {
		backend.waitUnloaded = backend.waitUntilUnloaded
	}
	return backend
}

func (backend *Backend) Install(ctx context.Context, binaryPath string) error {
	plistPath := backend.plistPath()
	logDirectory := filepath.Join(backend.home, "Library", "Logs", "CodexLauncher")
	if err := os.MkdirAll(logDirectory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(logDirectory, 0o700); err != nil {
		return err
	}
	body, err := launchAgent(binaryPath, filepath.Join(logDirectory, "companion.log"), backend.home)
	if err != nil {
		return err
	}
	if err := service.WriteFileAtomic(plistPath, body, 0o600); err != nil {
		return err
	}
	backend.logger.Info("[host-service] launch agent written", "platform", "darwin", "output_shape", "user_launch_agent")
	_, err = backend.runner.Run(ctx, "launchctl", "bootstrap", backend.domain(), plistPath)
	return err
}

func (backend *Backend) Start(ctx context.Context) error {
	_, err := backend.runner.Run(ctx, "launchctl", "kickstart", "-k", backend.target())
	return err
}

func (backend *Backend) Stop(ctx context.Context) error {
	exists, err := backend.plistExists()
	if err != nil || !exists {
		return err
	}
	_, err = backend.runner.Run(ctx, "launchctl", "bootout", backend.target())
	if err != nil {
		if launchctlServiceMissing(err) {
			return nil
		}
		return err
	}
	return backend.waitUnloaded(ctx, backend.target())
}

func (backend *Backend) Remove(context.Context) error {
	err := os.Remove(backend.plistPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (backend *Backend) Status(ctx context.Context) (hostinstall.ServiceStatus, error) {
	exists, err := backend.plistExists()
	if err != nil {
		return hostinstall.ServiceStatus{}, err
	}
	if !exists {
		return hostinstall.ServiceStatus{Installed: false, Running: false, Detail: "launch agent is absent"}, nil
	}
	output, err := backend.runner.Run(ctx, "launchctl", "print", backend.target())
	if err != nil {
		return hostinstall.ServiceStatus{}, fmt.Errorf("read launch agent state: %w", err)
	}
	running := launchAgentRunning(output)
	detail := "launch agent is loaded but not running"
	if running {
		detail = "launch agent is running"
	}
	return hostinstall.ServiceStatus{Installed: true, Running: running, Detail: detail}, nil
}

func (backend *Backend) plistExists() (bool, error) {
	info, err := os.Stat(backend.plistPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect launch agent: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("launch agent is not a regular file")
	}
	return true, nil
}

func (backend *Backend) plistPath() string {
	return filepath.Join(backend.home, "Library", "LaunchAgents", Label+".plist")
}

func (backend *Backend) domain() string { return fmt.Sprintf("gui/%d", backend.uid) }
func (backend *Backend) target() string { return backend.domain() + "/" + Label }

func (backend *Backend) waitUntilUnloaded(ctx context.Context, target string) error {
	waitContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := backend.runner.Run(waitContext, "launchctl", "print", target); err != nil {
			if launchctlServiceMissing(err) {
				return nil
			}
			return fmt.Errorf("verify launch agent unloaded: %w", err)
		}
		select {
		case <-waitContext.Done():
			return fmt.Errorf("wait for launch agent to unload: %w", waitContext.Err())
		case <-ticker.C:
		}
	}
}

func launchAgentRunning(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "state = running" {
			return true
		}
		if strings.HasPrefix(trimmed, "pid = ") {
			pid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "pid = ")))
			if err == nil && pid > 0 {
				return true
			}
		}
	}
	return false
}

func launchctlServiceMissing(err error) bool {
	var exitError *exec.ExitError
	return errors.As(err, &exitError) && exitError.ExitCode() == 113
}

func launchAgent(binaryPath, logPath, home string) ([]byte, error) {
	var binary bytes.Buffer
	if err := xml.EscapeText(&binary, []byte(binaryPath)); err != nil {
		return nil, err
	}
	var log bytes.Buffer
	if err := xml.EscapeText(&log, []byte(logPath)); err != nil {
		return nil, err
	}
	var escapedHome bytes.Buffer
	if err := xml.EscapeText(&escapedHome, []byte(home)); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, Label, binary.String(), escapedHome.String(), log.String(), log.String())), nil
}
