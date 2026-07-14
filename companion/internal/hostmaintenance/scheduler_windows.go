//go:build windows

package hostmaintenance

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/windows"
)

const maxHelperBytes int64 = 512 * 1024 * 1024

func scheduleMaintenance(currentPath, operation, artifact string) error {
	receiptPath, err := ReceiptPath()
	if err != nil {
		return err
	}
	directory := filepath.Dir(receiptPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	stale, _ := filepath.Glob(filepath.Join(directory, "maintenance-*.exe"))
	for _, path := range stale {
		_ = os.Remove(path)
	}
	helper, err := os.CreateTemp(directory, "maintenance-*.exe")
	if err != nil {
		return err
	}
	helperPath := helper.Name()
	input, err := os.Open(currentPath)
	if err != nil {
		_ = helper.Close()
		_ = os.Remove(helperPath)
		return err
	}
	written, copyErr := io.Copy(helper, io.LimitReader(input, maxHelperBytes+1))
	inputErr := input.Close()
	syncErr := helper.Sync()
	closeErr := helper.Close()
	if copyErr != nil || inputErr != nil || syncErr != nil || closeErr != nil || written <= 0 || written > maxHelperBytes {
		_ = os.Remove(helperPath)
		return errors.Join(copyErr, inputErr, syncErr, closeErr)
	}
	_ = os.Remove(receiptPath)
	arguments := []string{"_maintenance", "--wait-pid", strconv.Itoa(os.Getpid()), "--operation", operation, "--receipt", receiptPath}
	if artifact != "" {
		arguments = append(arguments, "--artifact", artifact)
	}
	command := exec.Command(helperPath, arguments...)
	if err := command.Start(); err != nil {
		_ = os.Remove(helperPath)
		return err
	}
	return command.Process.Release()
}

func WaitForParent(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 {
		return errors.New("unexpected parent wait result")
	}
	return nil
}

func CleanupSelf() error {
	currentPath, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command("cmd.exe", "/D", "/C", `ping 127.0.0.1 -n 3 > nul & del /f /q "%CODEX_LAUNCHER_DELETE_PATH%"`)
	command.Env = append(os.Environ(), "CODEX_LAUNCHER_DELETE_PATH="+currentPath)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
