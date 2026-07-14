package windows

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/testkit"
)

func TestCurrentUserTaskRunsAtLogonWithLimitedPrivileges(t *testing.T) {
	runner := &testkit.RecordingRunner{}
	marker := filepath.Join(t.TempDir(), "service", "windows-task-installed")
	backend := New(Options{Runner: runner, MarkerPath: marker})
	binary := `C:\Users\Aadi User\AppData\Local\Codex Launcher\codex-launcher.exe`

	if err := backend.Install(context.Background(), binary); err != nil {
		t.Fatal(err)
	}
	wantCreate := testkit.Call{
		Name: "schtasks.exe",
		Args: []string{"/Create", "/TN", TaskName, "/TR", `"C:\Users\Aadi User\AppData\Local\Codex Launcher\codex-launcher.exe" serve`, "/SC", "ONLOGON", "/RL", "LIMITED", "/F"},
	}
	if !reflect.DeepEqual(runner.Calls, []testkit.Call{wantCreate}) {
		t.Fatalf("install calls = %#v", runner.Calls)
	}
	info, err := os.Stat(marker)
	if err != nil {
		t.Fatalf("marker missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode = %v", info.Mode().Perm())
	}

	runner.Calls = nil
	if err := backend.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runner.Output = "4"
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	runner.Output = "3"
	status, err := backend.Status(context.Background())
	if err != nil || !status.Installed || status.Running {
		t.Fatalf("status = %#v, error = %v", status, err)
	}
	if err := backend.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantLifecycle := []testkit.Call{
		{Name: "schtasks.exe", Args: []string{"/Run", "/TN", TaskName}},
		{Name: "powershell.exe", Args: []string{"-NoProfile", "-NonInteractive", "-Command", taskStateCommand}},
		{Name: "schtasks.exe", Args: []string{"/End", "/TN", TaskName}},
		{Name: "powershell.exe", Args: []string{"-NoProfile", "-NonInteractive", "-Command", taskStateCommand}},
		{Name: "schtasks.exe", Args: []string{"/Delete", "/TN", TaskName, "/F"}},
	}
	if !reflect.DeepEqual(runner.Calls, wantLifecycle) {
		t.Fatalf("lifecycle calls = %#v", runner.Calls)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker survived remove: %v", err)
	}
}

func TestStatusUsesMarkerAndNumericTaskStateWithoutHidingCommandErrors(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "service", "windows-task-installed")
	runner := &testkit.RecordingRunner{}
	backend := New(Options{Runner: runner, MarkerPath: marker})
	status, err := backend.Status(context.Background())
	if err != nil || status.Installed || status.Running || len(runner.Calls) != 0 {
		t.Fatalf("absent status = %#v, calls = %#v, error = %v", status, runner.Calls, err)
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("installed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Output = "3"
	status, err = backend.Status(context.Background())
	if err != nil || !status.Installed || status.Running {
		t.Fatalf("ready status = %#v, error = %v", status, err)
	}
	runner.Err = errors.New("Task Scheduler unavailable")
	if _, err := backend.Status(context.Background()); err == nil {
		t.Fatal("Task Scheduler error was hidden")
	}
}

func TestInstallNeverCreatesTheTaskWhenItsOwnershipMarkerCannotBeWritten(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &testkit.RecordingRunner{}
	backend := New(Options{Runner: runner, MarkerPath: filepath.Join(parentFile, "marker")})
	if err := backend.Install(context.Background(), `C:\codex-launcher.exe`); err == nil {
		t.Fatal("install unexpectedly passed")
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("scheduled task was created without a marker: %#v", runner.Calls)
	}
}
