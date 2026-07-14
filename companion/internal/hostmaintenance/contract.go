package hostmaintenance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var ErrUnsupported = errors.New("deferred companion maintenance is unsupported on this platform")

type Scheduler struct{}

type Receipt struct {
	Version   int       `json:"version"`
	Operation string    `json:"operation"`
	OK        bool      `json:"ok"`
	ErrorCode string    `json:"errorCode,omitempty"`
	Finished  time.Time `json:"finishedAt"`
}

func New() *Scheduler { return &Scheduler{} }

func (*Scheduler) DeferIfRunningInstalled(operation, artifact, installedPath string) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	currentPath, err := os.Executable()
	if err != nil {
		return false, err
	}
	current, err := os.Stat(currentPath)
	if err != nil {
		return false, err
	}
	installed, err := os.Stat(installedPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(current, installed) {
		return false, nil
	}
	return true, scheduleMaintenance(currentPath, operation, artifact)
}

func ReceiptPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "codex-launcher", "maintenance", "last-result.json"), nil
}

func WriteReceipt(path, operation string, operationErr error) error {
	if path == "" || (operation != "replace" && operation != "rollback" && operation != "uninstall") {
		return errors.New("invalid maintenance receipt")
	}
	receipt := Receipt{Version: 1, Operation: operation, OK: operationErr == nil, Finished: time.Now().UTC()}
	if operationErr != nil {
		receipt.ErrorCode = "operation_failed"
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".receipt-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
