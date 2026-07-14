package hostsetup

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

func TestSaveVerifiesCodexTailscaleAndPortBeforeWriting(t *testing.T) {
	projectPath := canonicalTempDir(t)
	config := companionapp.Config{
		Version: 1, ComputerName: "Studio Mac", ListenHost: "100.64.0.10", ListenPort: 9443, CodexBinary: "/tools/codex",
		Projects: []projects.Config{{ID: "launcher", DisplayName: "Codex Launcher", Path: projectPath}},
	}
	var calls []string
	var saved companionapp.Config
	setup := New(Options{
		DiscoverCodex: func(path string) (string, error) { calls = append(calls, "discover:"+path); return path, nil },
		ValidateCodex: func(_ context.Context, path string) (string, error) {
			calls = append(calls, "validate:"+path)
			return "codex-cli 0.144.0", nil
		},
		RunTailscale: func(_ context.Context, name string, args ...string) (string, error) {
			calls = append(calls, name+":"+args[0]+":"+args[1])
			return "100.64.0.10", nil
		},
		Listen: func(network, address string) (net.Listener, error) {
			calls = append(calls, "listen:"+network+":"+address)
			return &recordingListener{}, nil
		},
		WriteConfig: func(savedConfig companionapp.Config) error {
			calls = append(calls, "write")
			saved = savedConfig
			return nil
		},
	})

	if err := setup.Save(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	want := []string{"discover:/tools/codex", "validate:/tools/codex", "tailscale:ip:--assert=100.64.0.10", "listen:tcp:100.64.0.10:9443", "write"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if !reflect.DeepEqual(saved, config) {
		t.Fatalf("saved config = %#v", saved)
	}
}

func TestSaveFailsBeforeWritingForUnavailableDependencies(t *testing.T) {
	projectPath := canonicalTempDir(t)
	config := companionapp.Config{
		Version: 1, ComputerName: "Studio Mac", ListenHost: "100.64.0.10", ListenPort: 9443, CodexBinary: "/tools/codex",
		Projects: []projects.Config{{ID: "launcher", DisplayName: "Codex Launcher", Path: projectPath}},
	}
	tests := []struct {
		name      string
		options   Options
		wantError error
	}{
		{
			name: "missing Codex", wantError: ErrCodexUnavailable,
			options: Options{DiscoverCodex: func(string) (string, error) { return "", errors.New("missing") }},
		},
		{
			name: "broken Codex", wantError: ErrCodexUnavailable,
			options: Options{ValidateCodex: func(context.Context, string) (string, error) { return "", errors.New("broken") }},
		},
		{
			name: "missing Tailscale", wantError: ErrTailscaleUnavailable,
			options: Options{RunTailscale: func(context.Context, string, ...string) (string, error) { return "", errors.New("missing") }},
		},
		{
			name: "port collision", wantError: ErrPortUnavailable,
			options: Options{Listen: func(string, string) (net.Listener, error) { return nil, syscall.EADDRINUSE }},
		},
		{
			name: "Tailscale address not ready", wantError: ErrTailscaleUnavailable,
			options: Options{Listen: func(string, string) (net.Listener, error) { return nil, syscall.EADDRNOTAVAIL }},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writes := 0
			options := passingOptions()
			if test.options.DiscoverCodex != nil {
				options.DiscoverCodex = test.options.DiscoverCodex
			}
			if test.options.ValidateCodex != nil {
				options.ValidateCodex = test.options.ValidateCodex
			}
			if test.options.RunTailscale != nil {
				options.RunTailscale = test.options.RunTailscale
			}
			if test.options.Listen != nil {
				options.Listen = test.options.Listen
			}
			options.WriteConfig = func(companionapp.Config) error { writes++; return nil }
			if err := New(options).Save(context.Background(), config); !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
			if writes != 0 {
				t.Fatalf("config writes = %d", writes)
			}
		})
	}
}

func passingOptions() Options {
	return Options{
		DiscoverCodex: func(path string) (string, error) { return path, nil },
		ValidateCodex: func(context.Context, string) (string, error) { return "codex-cli 0.144.0", nil },
		RunTailscale:  func(context.Context, string, ...string) (string, error) { return "100.64.0.10", nil },
		Listen:        func(string, string) (net.Listener, error) { return &recordingListener{}, nil },
		WriteConfig:   func(companionapp.Config) error { return nil },
	}
}

type recordingListener struct{ closed bool }

func (*recordingListener) Accept() (net.Conn, error) { return nil, errors.New("not used") }
func (listener *recordingListener) Close() error     { listener.closed = true; return nil }
func (*recordingListener) Addr() net.Addr            { return fakeAddress("100.64.0.10:9443") }

type fakeAddress string

func (fakeAddress) Network() string        { return "tcp" }
func (address fakeAddress) String() string { return string(address) }

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
