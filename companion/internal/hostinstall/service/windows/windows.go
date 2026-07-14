package windows

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service"
)

const TaskName = `Codex Launcher\Companion`
const taskStateCommand = `$ErrorActionPreference='Stop'; $task=Get-ScheduledTask -TaskName 'Companion' -TaskPath '\Codex Launcher\'; [Console]::Out.Write([int]$task.State)`

type Options struct {
	Runner     service.Runner
	Logger     *slog.Logger
	MarkerPath string
}

type Backend struct {
	runner     service.Runner
	logger     *slog.Logger
	markerPath string
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
	return &Backend{runner: runner, logger: logger, markerPath: options.MarkerPath}
}

func (backend *Backend) Install(ctx context.Context, binaryPath string) error {
	if strings.Contains(binaryPath, `"`) || backend.markerPath == "" {
		return hostinstall.ErrInvalidInstallArguments
	}
	taskRun := `"` + binaryPath + `" serve`
	if err := service.WriteFileAtomic(backend.markerPath, []byte("installed\n"), 0o600); err != nil {
		return err
	}
	backend.logger.Info("[host-service] current-user task requested", "platform", "windows", "output_shape", "on_logon_limited_task")
	if _, err := backend.runner.Run(ctx, "schtasks.exe", "/Create", "/TN", TaskName, "/TR", taskRun, "/SC", "ONLOGON", "/RL", "LIMITED", "/F"); err != nil {
		return err
	}
	return nil
}

func (backend *Backend) Start(ctx context.Context) error {
	_, err := backend.runner.Run(ctx, "schtasks.exe", "/Run", "/TN", TaskName)
	return err
}

func (backend *Backend) Stop(ctx context.Context) error {
	status, err := backend.Status(ctx)
	if err != nil || !status.Installed || !status.Running {
		return err
	}
	_, err = backend.runner.Run(ctx, "schtasks.exe", "/End", "/TN", TaskName)
	return err
}

func (backend *Backend) Remove(ctx context.Context) error {
	_, markerErr := os.Lstat(backend.markerPath)
	markerExists := markerErr == nil
	_, deleteErr := backend.runner.Run(ctx, "schtasks.exe", "/Delete", "/TN", TaskName, "/F")
	if !markerExists {
		deleteErr = nil
	}
	removeErr := os.Remove(backend.markerPath)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(deleteErr, removeErr)
}

func (backend *Backend) Status(ctx context.Context) (hostinstall.ServiceStatus, error) {
	info, err := os.Lstat(backend.markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return hostinstall.ServiceStatus{Installed: false, Running: false, Detail: "scheduled task is absent"}, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return hostinstall.ServiceStatus{}, fmt.Errorf("inspect scheduled task marker: %w", err)
	}
	output, err := backend.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", taskStateCommand)
	if err != nil {
		return hostinstall.ServiceStatus{}, fmt.Errorf("read scheduled task state: %w", err)
	}
	state, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || state < 0 || state > 4 {
		return hostinstall.ServiceStatus{}, errors.New("scheduled task returned an invalid state")
	}
	return hostinstall.ServiceStatus{Installed: true, Running: state == 4, Detail: "current-user scheduled task state: " + strconv.Itoa(state)}, nil
}
