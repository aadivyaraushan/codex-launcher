//go:build windows

package configsecurity

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsOwnerOnlyConfigCanBeWrittenAndOpened(t *testing.T) {
	root := filepath.Join(t.TempDir(), "codex-launcher")
	path := filepath.Join(root, "config.json")
	if err := WriteNew(root, path, []byte("private-config"), 1024); err != nil {
		t.Fatal(err)
	}
	if err := Replace(root, path, []byte("updated-private-config"), 1024); err != nil {
		t.Fatal(err)
	}
	file, err := Open(root, path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil || string(body) != "updated-private-config" {
		t.Fatalf("body = %q, error = %v", body, err)
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("config info = %#v, error = %v", info, err)
	}
}
