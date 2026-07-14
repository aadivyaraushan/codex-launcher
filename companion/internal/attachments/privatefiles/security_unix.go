//go:build !windows

package privatefiles

import (
	"errors"
	"os"
	"syscall"
)

func PrepareDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 || !ownedByCurrentUser(info) {
		return errors.New("directory is not private")
	}
	return nil
}

func HardenFile(file *os.File) error {
	if file == nil {
		return errors.New("file is missing")
	}
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
		return errors.New("file is not private")
	}
	return nil
}

func ValidateFile(_ string, info os.FileInfo) error {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
		return errors.New("file is not private")
	}
	return nil
}

func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, okay := info.Sys().(*syscall.Stat_t)
	return okay && stat.Uid == uint32(os.Geteuid())
}
