//go:build windows

package platformtests

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

func TestWindowsJunctionCannotBecomeAConfiguredProject(t *testing.T) {
	container := t.TempDir()
	target := t.TempDir()
	junction := filepath.Join(container, "junction")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %v: %s", err, output)
	}
	if _, err := projects.New([]projects.Config{{ID: "project-main", DisplayName: "Main", Path: junction}}); !errors.Is(err, projects.ErrUnsafeProjectPath) {
		t.Fatalf("junction configuration error = %v", err)
	}
}

func TestWindowsNetworkShareCannotBecomeAConfiguredProject(t *testing.T) {
	if _, err := projects.New([]projects.Config{{ID: "project-main", DisplayName: "Main", Path: `\\server\share\project`}}); !errors.Is(err, projects.ErrUnsafeProjectPath) {
		t.Fatalf("network share configuration error = %v", err)
	}
}
