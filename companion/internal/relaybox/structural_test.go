package relaybox

import (
	"go/build"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBoxImportsNoPhoneOrMacIdentityMaterial is spec test F: the box must
// be buildable without ever linking the phone/Mac identity or pairing
// packages. "Can't read the content" rests on both this (no keys) and the
// passthrough rule in phonedoor.go (no terminate); this test only proves
// the first half. It walks the real import graph (via go/build), not a
// grep, so it also catches an indirect import introduced through some
// future helper package.
func TestBoxImportsNoPhoneOrMacIdentityMaterial(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine relaybox package directory")
	}
	packageDir := filepath.Dir(thisFile)
	moduleRoot, err := findModuleRoot(packageDir)
	if err != nil {
		t.Fatal(err)
	}

	forbiddenSubstrings := []string{
		"/companion/internal/pairing",
	}

	visited := make(map[string]bool)
	var walk func(dir string) error
	walk = func(dir string) error {
		if visited[dir] {
			return nil
		}
		visited[dir] = true
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			// A directory with no buildable Go files (or a stdlib/module
			// cache miss) isn't something this test can walk further; the
			// relevant first-party packages always resolve locally.
			return nil
		}
		for _, forbidden := range forbiddenSubstrings {
			if strings.Contains(pkg.ImportPath, forbidden) {
				t.Errorf("relaybox transitively imports %s (via %s)", pkg.ImportPath, dir)
			}
		}
		for _, imp := range pkg.Imports {
			if !strings.HasPrefix(imp, "github.com/codex-launcher/codex-launcher/") {
				continue // stdlib / third party: cannot be pairing/identity code
			}
			for _, forbidden := range forbiddenSubstrings {
				if strings.Contains(imp, forbidden) {
					t.Errorf("relaybox package %s imports forbidden package %s", dir, imp)
				}
			}
			importedDir := resolveImportDir(moduleRoot, imp)
			if err := walk(importedDir); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(packageDir); err != nil {
		t.Fatal(err)
	}
}

// resolveImportDir maps an import path under this module back to its
// directory on disk.
func resolveImportDir(moduleRoot, importPath string) string {
	const modulePrefix = "github.com/codex-launcher/codex-launcher/"
	relative := strings.TrimPrefix(importPath, modulePrefix)
	return filepath.Join(moduleRoot, filepath.FromSlash(relative))
}

// findModuleRoot walks upward from dir looking for go.mod.
func findModuleRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
