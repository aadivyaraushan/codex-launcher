//go:build !windows

package configsecurity

import (
	"errors"
	"os"
	"syscall"
)

func prepareRoot(root string) error {
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return ErrUnsafe
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || validateRoot(info) != nil {
		return ErrUnsafe
	}
	return nil
}

func publishNew(temporaryPath, path string) error {
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrExists
		}
		return ErrUnsafe
	}
	return nil
}

func syncRoot(root string) error {
	directory, err := os.Open(root)
	if err != nil {
		return ErrUnsafe
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return ErrUnsafe
	}
	return nil
}

func validateRoot(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 || !ownedByCurrentUser(info) {
		return ErrUnsafe
	}
	return nil
}

func validateFile(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 || !ownedByCurrentUser(info) {
		return ErrUnsafe
	}
	return nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
