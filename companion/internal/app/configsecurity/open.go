package configsecurity

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var (
	ErrUnsafe   = errors.New("configuration file or directory is unsafe")
	ErrTooLarge = errors.New("configuration file is too large")
	ErrExists   = errors.New("configuration file already exists")
)

func Open(root, path string, maxBytes int64) (*os.File, error) {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanRoot) || !filepath.IsAbs(cleanPath) || filepath.Dir(cleanPath) != cleanRoot {
		return nil, ErrUnsafe
	}
	rootInfo, err := os.Lstat(cleanRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || validateRoot(rootInfo) != nil {
		return nil, ErrUnsafe
	}
	pathInfo, err := os.Lstat(cleanPath)
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafe
	}
	if pathInfo.Size() > maxBytes {
		return nil, ErrTooLarge
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, ErrUnsafe
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) || validateFile(openedInfo) != nil {
		file.Close()
		return nil, ErrUnsafe
	}
	if openedInfo.Size() > maxBytes {
		file.Close()
		return nil, ErrTooLarge
	}
	return file, nil
}

func WriteNew(root, path string, encoded []byte, maxBytes int64) error {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanRoot) || !filepath.IsAbs(cleanPath) || filepath.Dir(cleanPath) != cleanRoot {
		return ErrUnsafe
	}
	if int64(len(encoded)) > maxBytes {
		return ErrTooLarge
	}
	if err := prepareRoot(cleanRoot); err != nil {
		return err
	}
	if _, err := os.Lstat(cleanPath); err == nil {
		return ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafe
	}
	temporary, err := os.CreateTemp(cleanRoot, ".config-*.tmp")
	if err != nil {
		return ErrUnsafe
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, bytes.NewReader(encoded)); err != nil {
		temporary.Close()
		return ErrUnsafe
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return ErrUnsafe
	}
	if err := temporary.Close(); err != nil {
		return ErrUnsafe
	}
	if err := publishNew(temporaryPath, cleanPath); err != nil {
		return err
	}
	return syncRoot(cleanRoot)
}
