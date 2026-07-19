package hostdoctor

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestRunReportsCodexRelayBoxReachabilityServiceIdentitySchemaAndLastError(t *testing.T) {
	config := doctorConfig(t)
	var calls []string
	doctor := New(Options{
		DiscoverCodex: func(path string) (string, error) { calls = append(calls, "discover:"+path); return path, nil },
		ValidateCodex: func(context.Context, string) (string, error) { return "codex-cli 0.144.0", nil },
		DialRelay: func(_ context.Context, addr string, pinnedPublicKey []byte) (net.Conn, error) {
			calls = append(calls, "dialRelay:"+addr)
			return newDoctorConnection("OK\n"), nil
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
		{Name: "relay-box", OK: true, Detail: "box reachable, pinned key and registration secret match"},
		{Name: "service", OK: true, Detail: "launch agent is loaded"},
		{Name: "reachability", OK: true, Detail: "relay box phone door accepts connections"},
		{Name: "schema", OK: true, Detail: "state schema is compatible"},
		{Name: "identity", OK: true, Detail: "pinned fingerprint: pinned-fingerprint"},
		{Name: "last_error", OK: true, Detail: "none"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("checks = %#v, want %#v", checks, want)
	}
	wantCalls := []string{"discover:/tools/codex", "dialRelay:relay.example.com:9000", "dial:tcp:relay.example.com:8443"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

// CHECK validates the saved registration secret without taking the live
// service's single REGISTER slot.
func TestRelayBoxCheckValidatesSecretWithoutRegistering(t *testing.T) {
	config := doctorConfig(t)
	connection := newDoctorConnection("OK\n")
	doctor := New(Options{
		DiscoverCodex: func(path string) (string, error) { return path, nil },
		ValidateCodex: func(context.Context, string) (string, error) { return "codex-cli 0.144.0", nil },
		DialRelay: func(context.Context, string, []byte) (net.Conn, error) {
			return connection, nil
		},
		ServiceStatus: func(context.Context) (hostinstall.ServiceStatus, error) {
			return hostinstall.ServiceStatus{Installed: true, Running: true}, nil
		},
		Dial: func(string, string, time.Duration) (net.Conn, error) { return &doctorConnection{}, nil },
		InspectState: func(context.Context) (inspection.State, error) {
			return inspection.State{SchemaCompatible: true}, nil
		},
		LastError: func() (string, error) { return "none", nil },
	})

	doctor.Run(context.Background(), config)
	if got := connection.written.String(); got != "CHECK relay-secret-value\n" {
		t.Fatalf("relay-box check wrote %q, want a non-registering CHECK", got)
	}
	if !connection.closed {
		t.Fatal("relay-box check must close the connection it opened")
	}
}

func TestRelayBoxCheckFailsWhenRegistrationSecretDoesNotMatch(t *testing.T) {
	config := doctorConfig(t)
	doctor := New(Options{
		DialRelay: func(context.Context, string, []byte) (net.Conn, error) {
			return newDoctorConnection("ERR\n"), nil
		},
	})

	check := doctor.relayBoxCheck(context.Background(), config.Relay)
	if check.OK || check.Detail != "box reachable, but registration secret does not match" {
		t.Fatalf("relay check = %#v, want a failed secret-mismatch check", check)
	}
}

func TestRunFailsClosedForBrokenDependenciesAndIdentityLoss(t *testing.T) {
	config := doctorConfig(t)
	doctor := New(Options{
		DiscoverCodex: func(string) (string, error) { return "", errors.New("missing") },
		DialRelay:     func(context.Context, string, []byte) (net.Conn, error) { return nil, errors.New("connection refused") },
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
		Version: 1, ComputerName: "Studio Mac", CodexBinary: "/tools/codex",
		Projects: []projects.Config{{ID: "main", DisplayName: "Main", Path: path}},
		Relay: companionapp.RelayConfig{
			BoxHost: "relay.example.com", MacPort: 9000, PhonePort: 8443,
			PinnedKey: base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")), Secret: "relay-secret-value",
		},
	}
}

type doctorConnection struct {
	net.Conn
	readBuffer *bytes.Reader
	written    bytes.Buffer
	closed     bool
}

func newDoctorConnection(response string) *doctorConnection {
	return &doctorConnection{readBuffer: bytes.NewReader([]byte(response))}
}

func (connection *doctorConnection) Write(data []byte) (int, error) {
	return connection.written.Write(data)
}
func (connection *doctorConnection) Read(data []byte) (int, error) {
	return connection.readBuffer.Read(data)
}
func (connection *doctorConnection) SetReadDeadline(time.Time) error { return nil }
func (connection *doctorConnection) Close() error                    { connection.closed = true; return nil }
