package launchd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/testkit"
)

func TestLaunchAgentUsesTheCurrentUserDomainAndEscapesPaths(t *testing.T) {
	home := t.TempDir()
	runner := &testkit.RecordingRunner{}
	waitedTarget := ""
	backend := New(Options{Home: home, UID: 501, Runner: runner, WaitUnloaded: func(_ context.Context, target string) error {
		waitedTarget = target
		return nil
	}})
	binary := filepath.Join(home, "Codex & Launcher", "codex-launcher")

	if err := backend.Install(context.Background(), binary); err != nil {
		t.Fatal(err)
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
	body, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(body)
	for _, required := range []string{"<string>" + Label + "</string>", "<string>" + strings.ReplaceAll(binary, "&", "&amp;") + "</string>", "<string>serve</string>", "<key>RunAtLoad</key>", "<key>KeepAlive</key>", "<key>EnvironmentVariables</key>", "<key>HOME</key>", "<string>" + home + "</string>"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("plist missing %q:\n%s", required, rendered)
		}
	}
	if info, err := os.Stat(plistPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("plist mode = %v, error = %v", info.Mode().Perm(), err)
	}
	logDirectory := filepath.Join(home, "Library", "Logs", "CodexLauncher")
	if info, err := os.Stat(logDirectory); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("log directory = %#v, error = %v", info, err)
	}
	want := []testkit.Call{{Name: "launchctl", Args: []string{"bootstrap", "gui/501", plistPath}}}
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
	if waitedTarget != "gui/501/"+Label {
		t.Fatalf("waited target = %q", waitedTarget)
	}
	runner.Output = "state = running\n"
	status, err := backend.Status(context.Background())
	if err != nil || !status.Installed || !status.Running {
		t.Fatalf("status = %#v, error = %v", status, err)
	}
	if err := backend.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Fatalf("plist survived remove: %v", err)
	}
}

func TestStatusDistinguishesLoadedWaitingFromRunning(t *testing.T) {
	home := t.TempDir()
	backend := New(Options{Home: home, UID: 501, Runner: &testkit.RecordingRunner{Output: "state = waiting\n"}})
	if err := os.MkdirAll(filepath.Dir(backend.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.plistPath(), []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := backend.Status(context.Background())
	if err != nil || !status.Installed || status.Running {
		t.Fatalf("waiting status = %#v, error = %v", status, err)
	}
}

func TestStatusTreatsAnActiveXPCProxyPIDAsRunning(t *testing.T) {
	home := t.TempDir()
	backend := New(Options{Home: home, UID: 501, Runner: &testkit.RecordingRunner{Output: "state = xpcproxy\npid = 4242\n"}})
	if err := os.MkdirAll(filepath.Dir(backend.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.plistPath(), []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := backend.Status(context.Background())
	if err != nil || !status.Running {
		t.Fatalf("xpcproxy status = %#v, error = %v", status, err)
	}
}

func TestUnloadWaitDoesNotTreatAnUnknownLaunchctlFailureAsAbsent(t *testing.T) {
	backend := New(Options{Home: t.TempDir(), UID: 501, Runner: &testkit.RecordingRunner{Err: errors.New("permission denied")}})
	if err := backend.waitUntilUnloaded(context.Background(), backend.target()); err == nil {
		t.Fatal("unknown launchctl failure was treated as an unloaded service")
	}
}

func TestStopIsIdempotentForTheExactLaunchctlMissingExitCode(t *testing.T) {
	home := t.TempDir()
	missingErr := exec.Command("sh", "-c", "exit 113").Run()
	backend := New(Options{Home: home, UID: 501, Runner: &testkit.RecordingRunner{Err: missingErr}})
	if err := os.MkdirAll(filepath.Dir(backend.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.plistPath(), []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatalf("missing service stop error = %v", err)
	}
}

func TestStatusAndStopDoNotHideLaunchctlFailures(t *testing.T) {
	home := t.TempDir()
	runner := &testkit.RecordingRunner{}
	backend := New(Options{Home: home, UID: 501, Runner: runner})
	status, err := backend.Status(context.Background())
	if err != nil || status.Installed || status.Running || len(runner.Calls) != 0 {
		t.Fatalf("absent status = %#v, calls = %#v, error = %v", status, runner.Calls, err)
	}
	if err := os.MkdirAll(filepath.Dir(backend.plistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend.plistPath(), []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Err = errors.New("launchd domain unavailable")
	if _, err := backend.Status(context.Background()); err == nil {
		t.Fatal("launchctl status failure was hidden")
	}
	if err := backend.Stop(context.Background()); err == nil {
		t.Fatal("launchctl stop failure was hidden")
	}
}
