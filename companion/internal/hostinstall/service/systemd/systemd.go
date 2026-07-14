package systemd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service"
)

const UnitName = "codex-launcher.service"

type Options struct {
	ConfigHome string
	Runner     service.Runner
	Logger     *slog.Logger
}

type Backend struct {
	configHome string
	runner     service.Runner
	logger     *slog.Logger
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
	return &Backend{configHome: options.ConfigHome, runner: runner, logger: logger}
}

func (backend *Backend) Install(ctx context.Context, binaryPath string) error {
	body := []byte(fmt.Sprintf(`[Unit]
Description=Codex Launcher companion
After=network-online.target

[Service]
Type=simple
ExecStart="%s" serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, escapeExecPath(binaryPath)))
	if err := service.WriteFileAtomic(backend.unitPath(), body, 0o600); err != nil {
		return err
	}
	backend.logger.Info("[host-service] user unit written", "platform", "linux", "output_shape", "systemd_user_unit")
	if _, err := backend.runner.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	_, err := backend.runner.Run(ctx, "systemctl", "--user", "enable", UnitName)
	return err
}

func (backend *Backend) Start(ctx context.Context) error {
	_, err := backend.runner.Run(ctx, "systemctl", "--user", "start", UnitName)
	return err
}

func (backend *Backend) Stop(ctx context.Context) error {
	exists, err := backend.unitExists()
	if err != nil || !exists {
		return err
	}
	_, err = backend.runner.Run(ctx, "systemctl", "--user", "stop", UnitName)
	return err
}

func (backend *Backend) Remove(ctx context.Context) error {
	_, disableErr := backend.runner.Run(ctx, "systemctl", "--user", "disable", UnitName)
	removeErr := os.Remove(backend.unitPath())
	if os.IsNotExist(removeErr) {
		removeErr = nil
	}
	_, reloadErr := backend.runner.Run(ctx, "systemctl", "--user", "daemon-reload")
	return errors.Join(
		wrapCleanupError("disable systemd user service", disableErr),
		wrapCleanupError("remove systemd user unit", removeErr),
		wrapCleanupError("reload systemd user manager", reloadErr),
	)
}

func (backend *Backend) Status(ctx context.Context) (hostinstall.ServiceStatus, error) {
	exists, err := backend.unitExists()
	if err != nil {
		return hostinstall.ServiceStatus{}, err
	}
	if !exists {
		return hostinstall.ServiceStatus{Installed: false, Running: false, Detail: "systemd user service is absent"}, nil
	}
	output, err := backend.runner.Run(ctx, "systemctl", "--user", "show", UnitName, "--property=ActiveState", "--value")
	if err != nil {
		return hostinstall.ServiceStatus{}, fmt.Errorf("read systemd user service state: %w", err)
	}
	state := strings.TrimSpace(output)
	return hostinstall.ServiceStatus{Installed: true, Running: state == "active", Detail: "systemd user service state: " + state}, nil
}

func (backend *Backend) unitExists() (bool, error) {
	info, err := os.Stat(backend.unitPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect systemd user unit: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("systemd user unit is not a regular file")
	}
	return true, nil
}

func wrapCleanupError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func (backend *Backend) unitPath() string {
	return filepath.Join(backend.configHome, "systemd", "user", UnitName)
}

func escapeExecPath(path string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`)
	return replacer.Replace(path)
}
