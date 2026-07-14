package hostdoctor

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore/inspection"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

func TestRunReportsVersionsReachabilityServiceTailnetIdentitySchemaAndLastError(t *testing.T) {
	config := doctorConfig(t)
	var calls []string
	doctor := New(Options{
		DiscoverCodex: func(path string) (string, error) { calls = append(calls, "discover:"+path); return path, nil },
		ValidateCodex: func(context.Context, string) (string, error) { return "codex-cli 0.144.0", nil },
		RunTailscale: func(_ context.Context, name string, args ...string) (string, error) {
			calls = append(calls, name+":"+args[0]+":"+args[1])
			return config.ListenHost, nil
		},
		ServiceStatus: func(context.Context) (hostinstall.ServiceStatus, error) {
			return hostinstall.ServiceStatus{Installed: true, Running: true, Detail: "launch agent is loaded"}, nil
		},
		Dial: func(network, address string, timeout time.Duration) (net.Conn, error) {
			calls = append(calls, "dial:"+network+":"+address)
			return &doctorConnection{}, nil
		},
		InspectState: func(context.Context) (inspection.State, error) {
			return inspection.State{SchemaCompatible: true, IdentityPresent: true, IdentityFingerprint: "pinned-fingerprint", PairedDevices: 1}, nil
		},
		LastError: func() (string, error) { return "none", nil },
	})

	checks := doctor.Run(context.Background(), config)
	want := []cli.Check{
		{Name: "codex", OK: true, Detail: "codex-cli 0.144.0"},
		{Name: "tailscale", OK: true, Detail: "address owned: 100.64.0.10"},
		{Name: "service", OK: true, Detail: "launch agent is loaded"},
		{Name: "reachability", OK: true, Detail: "companion port accepts connections"},
		{Name: "schema", OK: true, Detail: "state schema is compatible"},
		{Name: "identity", OK: true, Detail: "pinned fingerprint: pinned-fingerprint"},
		{Name: "last_error", OK: true, Detail: "none"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("checks = %#v, want %#v", checks, want)
	}
	wantCalls := []string{"discover:/tools/codex", "tailscale:ip:--assert=100.64.0.10", "dial:tcp:100.64.0.10:9443"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestRunFailsClosedForBrokenDependenciesAndIdentityLoss(t *testing.T) {
	config := doctorConfig(t)
	doctor := New(Options{
		DiscoverCodex: func(string) (string, error) { return "", errors.New("missing") },
		RunTailscale:  func(context.Context, string, ...string) (string, error) { return "", errors.New("offline") },
		ServiceStatus: func(context.Context) (hostinstall.ServiceStatus, error) {
			return hostinstall.ServiceStatus{}, errors.New("unknown")
		},
		Dial: func(string, string, time.Duration) (net.Conn, error) { return nil, errors.New("refused") },
		InspectState: func(context.Context) (inspection.State, error) {
			return inspection.State{SchemaCompatible: true, PairedDevices: 1}, inspection.ErrIdentityLoss
		},
		LastError: func() (string, error) { return "codex_unavailable", nil },
	})

	checks := doctor.Run(context.Background(), config)
	if len(checks) != 7 {
		t.Fatalf("checks = %#v", checks)
	}
	for _, index := range []int{0, 1, 2, 3, 5} {
		check := checks[index]
		if check.OK {
			t.Fatalf("check unexpectedly passed: %#v", check)
		}
	}
	if !checks[4].OK {
		t.Fatalf("schema check = %#v", checks[4])
	}
	if !checks[6].OK || checks[6].Detail != "codex_unavailable" {
		t.Fatalf("last error check = %#v", checks[6])
	}
}

func doctorConfig(t *testing.T) companionapp.Config {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return companionapp.Config{
		Version: 1, ComputerName: "Studio Mac", ListenHost: "100.64.0.10", ListenPort: 9443, CodexBinary: "/tools/codex",
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: path}},
	}
}

type doctorConnection struct{ net.Conn }

func (*doctorConnection) Close() error { return nil }
