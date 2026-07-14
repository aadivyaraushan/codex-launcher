package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

type Runner interface {
	Run(context.Context, string, ...string) (string, error)
}

type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, name string, arguments ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, arguments...).CombinedOutput()
	return string(output), err
}

func WriteFileAtomic(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(temporary)
		return errors.Join(writeErr, syncErr, closeErr)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
