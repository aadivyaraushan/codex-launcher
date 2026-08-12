package supervisor_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func scriptDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir := scriptDir(t)
	// companion/internal/phoneruntime/supervisor -> repo root
	return filepath.Clean(filepath.Join(dir, "..", "..", "..", ".."))
}

func TestSupervisorScriptsSyntaxAndContract(t *testing.T) {
	dir := scriptDir(t)
	boot := filepath.Join(dir, "10-operator-runtime.sh")
	watch := filepath.Join(dir, "operator-runtime-watchdog.sh")
	for _, path := range []string{boot, watch} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s must be executable", path)
		}
		out, err := exec.Command("bash", "-n", path).CombinedOutput()
		if err != nil {
			t.Fatalf("bash -n %s: %v\n%s", path, err, out)
		}
	}
	bootBody, err := os.ReadFile(boot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bootBody), "termux-wake-lock") {
		t.Fatal("boot script must acquire termux-wake-lock")
	}
	if !strings.Contains(string(bootBody), "operator-runtime-watchdog") {
		t.Fatal("boot script must exec watchdog")
	}
	watchBody, err := os.ReadFile(watch)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"flock -n",
		"no_backup/operator/runtime",
		"proot-distro login debian",
		"runsvdir /etc/operator/services",
		"backoff=1",
		"backoff=60",
	} {
		if !strings.Contains(string(watchBody), needle) {
			t.Fatalf("watchdog missing required contract fragment %q", needle)
		}
	}
}

func TestSupervisorMatchesPhoneBootCanonical(t *testing.T) {
	root := repoRoot(t)
	pairs := [][2]string{
		{
			filepath.Join(root, "companion/internal/phoneruntime/supervisor/10-operator-runtime.sh"),
			filepath.Join(root, "scripts/phone-boot/termux/boot/10-operator-runtime.sh"),
		},
		{
			filepath.Join(root, "companion/internal/phoneruntime/supervisor/operator-runtime-watchdog.sh"),
			filepath.Join(root, "scripts/phone-boot/termux/libexec/operator-runtime-watchdog.sh"),
		},
	}
	for _, pair := range pairs {
		a, err := os.ReadFile(pair[0])
		if err != nil {
			t.Fatalf("read %s: %v", pair[0], err)
		}
		b, err := os.ReadFile(pair[1])
		if err != nil {
			t.Fatalf("read %s: %v (phone-boot is canonical; copy missing)", pair[1], err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("drift between\n  %s\nand\n  %s\nKeep both identical; phone-boot is the install source.", pair[0], pair[1])
		}
	}
}

func TestPhoneBootShellContract(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts/phone-boot/test/run-tests.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("phone-boot shell contract failed: %v\n%s", err, out)
	}
}
