//go:build windows

package privatefiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateDirectoryAndFileUseCurrentUserOnlyACL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachments")
	if err := PrepareDirectory(root); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(root, "attachment-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := HardenFile(file); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(file.Name(), info); err != nil {
		t.Fatal(err)
	}
}
