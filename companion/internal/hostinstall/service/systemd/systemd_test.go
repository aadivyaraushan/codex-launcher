package systemd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/testkit"
)

func TestUserUnitStartsAfterLoginAndDoesNotNeedRoot(t *testing.T) {
	configHome := t.TempDir()
	runner := &testkit.RecordingRunner{}
	backend := New(Options{ConfigHome: configHome, Runner: runner})
	binary := filepath.Join(t.TempDir(), "Codex Launcher", "codex-launcher")

	if err := backend.Install(context.Background(), binary); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(configHome, "systemd", "user", UnitName)
	body, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(body)
	for _, required := range []string{"ExecStart=\"" + binary + "\" serve", "Restart=on-failure", "WantedBy=default.target"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("unit missing %q:\n%s", required, rendered)
		}
	}
	want := []testkit.Call{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		{Name: "systemctl", Args: []string{"--user", "enable", UnitName}},
	}
	if !reflect.DeepEqual(runner.Calls, want) {
		t.Fatalf("install calls = %#v", runner.Calls)
	}

	runner.Calls = nil
	if err := backend.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	runner.Output = "active\n"
	status, err := backend.Status(context.Background())
	if err != nil || !status.Installed || !status.Running {
		t.Fatalf("status = %#v, error = %v", status, err)
	}
	if err := backend.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit survived remove: %v", err)
	}
}

func TestStatusDistinguishesAbsentInactiveAndCommandFailure(t *testing.T) {
	configHome := t.TempDir()
	runner := &testkit.RecordingRunner{}
	backend := New(Options{ConfigHome: configHome, Runner: runner})
	status, err := backend.Status(context.Background())
	if err != nil || status.Installed || status.Running || len(runner.Calls) != 0 {
		t.Fatalf("absent status = %#v, calls = %#v, error = %v", status, runner.Calls, err)
	}
	if err := os.MkdirAll(filepath.Dir(backend.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.unitPath(), []byte("unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Output = "inactive\n"
	status, err = backend.Status(context.Background())
	if err != nil || !status.Installed || status.Running {
		t.Fatalf("inactive status = %#v, error = %v", status, err)
	}
	runner.Err = errors.New("user bus unavailable")
	if _, err := backend.Status(context.Background()); err == nil {
		t.Fatal("systemctl failure was hidden")
	}
}

func TestStopIsIdempotentOnlyWhenTheUnitIsAbsent(t *testing.T) {
	configHome := t.TempDir()
	runner := &testkit.RecordingRunner{}
	backend := New(Options{ConfigHome: configHome, Runner: runner})
	if err := backend.Stop(context.Background()); err != nil || len(runner.Calls) != 0 {
		t.Fatalf("absent stop error = %v, calls = %#v", err, runner.Calls)
	}
	if err := os.MkdirAll(filepath.Dir(backend.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.unitPath(), []byte("unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Err = errors.New("user bus unavailable")
	if err := backend.Stop(context.Background()); err == nil {
		t.Fatal("systemctl stop failure was hidden")
	}
}

func TestRemoveAttemptsEveryCleanupStepAndReturnsEveryFailure(t *testing.T) {
	configHome := t.TempDir()
	runner := &testkit.RecordingRunner{Err: errors.New("systemctl unavailable")}
	backend := New(Options{ConfigHome: configHome, Runner: runner})
	if err := os.MkdirAll(filepath.Dir(backend.unitPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.unitPath(), []byte("unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := backend.Remove(context.Background())
	if err == nil || !strings.Contains(err.Error(), "disable") || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("remove error = %v", err)
	}
	want := []testkit.Call{
		{Name: "systemctl", Args: []string{"--user", "disable", UnitName}},
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Calls, want) {
		t.Fatalf("remove calls = %#v", runner.Calls)
	}
}
