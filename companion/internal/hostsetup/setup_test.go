package hostsetup

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

// testRegisterReadTimeout keeps the registration check's "did the box stay
// quiet" wait short in tests, instead of the production default.
const testRegisterReadTimeout = 20 * time.Millisecond

func TestSaveVerifiesCodexAndRelayRegistrationBeforeWriting(t *testing.T) {
	projectPath := canonicalTempDir(t)
	config := testRelayConfig(projectPath)
	var calls []string
	var saved companionapp.Config
	// received gets the REGISTER line the fake box actually read, off of a
	// channel rather than a shared variable, so there is nothing for the
	// background "box" goroutine and the test goroutine to race on.
	received := make(chan string, 1)
	setup := New(Options{
		DiscoverCodex: func(path string) (string, error) { calls = append(calls, "discover:"+path); return path, nil },
		ValidateCodex: func(_ context.Context, path string) (string, error) {
			calls = append(calls, "validate:"+path)
			return "codex-cli 0.144.0", nil
		},
		DialRelay: func(_ context.Context, addr string, pinnedPublicKey []byte) (net.Conn, error) {
			calls = append(calls, "dial:"+addr)
			return acceptingRelayConn(t, received), nil
		},
		WriteConfig: func(savedConfig companionapp.Config) error {
			calls = append(calls, "write")
			saved = savedConfig
			return nil
		},
		RegisterReadTimeout: testRegisterReadTimeout,
	})

	if err := setup.Save(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	want := []string{"discover:/tools/codex", "validate:/tools/codex", "dial:relay.example.com:9000", "write"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if !reflect.DeepEqual(saved, config) {
		t.Fatalf("saved config = %#v", saved)
	}
	select {
	case line := <-received:
		if line != "REGISTER relay-secret-value\n" {
			t.Fatalf("relay box received %q, want the REGISTER line with the configured secret", line)
		}
	default:
		t.Fatal("expected the fake relay box to have received a REGISTER line")
	}
}

func TestSaveFailsBeforeWritingForUnavailableDependencies(t *testing.T) {
	projectPath := canonicalTempDir(t)
	config := testRelayConfig(projectPath)
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
			name: "relay box unreachable", wantError: ErrRelayUnavailable,
			options: Options{DialRelay: func(context.Context, string, []byte) (net.Conn, error) {
				return nil, errors.New("connection refused")
			}},
		},
		{
			name: "relay rejects the secret", wantError: ErrRelayRegisterRejected,
			options: Options{DialRelay: func(_ context.Context, _ string, _ []byte) (net.Conn, error) {
				return rejectingRelayConn(t), nil
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writes := 0
			options := passingOptions(t)
			if test.options.DiscoverCodex != nil {
				options.DiscoverCodex = test.options.DiscoverCodex
			}
			if test.options.ValidateCodex != nil {
				options.ValidateCodex = test.options.ValidateCodex
			}
			if test.options.DialRelay != nil {
				options.DialRelay = test.options.DialRelay
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

func TestDiscoverTailscaleAddressUsesOneOwnedLiteralOrAnExactOverride(t *testing.T) {
	addresses := func(_ context.Context, family string) (string, error) {
		switch family {
		case "-4":
			return "100.64.0.10\n", nil
		case "-6":
			return "fd7a:115c:a1e0::10\n", nil
		default:
			return "", errors.New("unexpected family")
		}
	}
	setup := New(Options{TailscaleIPs: addresses})
	for _, override := range []string{"", "100.64.0.10"} {
		got, err := setup.DiscoverTailscaleAddress(context.Background(), override)
		if err != nil || got != "100.64.0.10" {
			t.Fatalf("override = %q, address = %q, error = %v", override, got, err)
		}
	}
	if _, err := setup.DiscoverTailscaleAddress(context.Background(), "100.64.0.11"); !errors.Is(err, ErrTailscaleUnavailable) {
		t.Fatalf("unowned override error = %v", err)
	}
}

func TestDiscoverTailscaleAddressFailsForMissingOrAmbiguousOutput(t *testing.T) {
	for name, output := range map[string]string{"missing": "", "multiple": "100.64.0.10\n100.64.0.11\n", "malformed": "not-an-ip\n"} {
		t.Run(name, func(t *testing.T) {
			setup := New(Options{TailscaleIPs: func(context.Context, string) (string, error) { return output, nil }})
			if _, err := setup.DiscoverTailscaleAddress(context.Background(), ""); !errors.Is(err, ErrTailscaleUnavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func passingOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		DiscoverCodex:       func(path string) (string, error) { return path, nil },
		ValidateCodex:       func(context.Context, string) (string, error) { return "codex-cli 0.144.0", nil },
		DialRelay:           func(context.Context, string, []byte) (net.Conn, error) { return acceptingRelayConn(t, nil), nil },
		WriteConfig:         func(companionapp.Config) error { return nil },
		RegisterReadTimeout: testRegisterReadTimeout,
	}
}

// testRelayConfig returns a Config that passes Validate, wired for the
// fake relay connections below (BoxHost/MacPort feed the dial address the
// tests assert on).
func testRelayConfig(projectPath string) companionapp.Config {
	return companionapp.Config{
		Version: 1, ComputerName: "Studio Mac", CodexBinary: "/tools/codex",
		Projects: []projects.Config{{ID: "launcher", DisplayName: "Codex Launcher", Path: projectPath}},
		Relay: companionapp.RelayConfig{
			BoxHost: "relay.example.com", MacPort: 9000, PhonePort: 8443,
			PinnedKey: base64.StdEncoding.EncodeToString([]byte("pinned-key-bytes")), Secret: "relay-secret-value",
		},
	}
}

// acceptingRelayConn simulates the box's Mac door accepting a registration:
// it reads the REGISTER line (so the caller's Write does not block) and
// then goes quiet — never writing, never closing — exactly like the real
// box's documented no-ack-on-success behavior. If received is non-nil, the
// exact line the fake box read is sent on it (buffered, so this never
// blocks the goroutine), letting a test verify what was sent without
// sharing mutable state across goroutines.
func acceptingRelayConn(t *testing.T, received chan string) net.Conn {
	t.Helper()
	client, box := net.Pipe()
	t.Cleanup(func() { _ = box.Close() })
	go func() {
		reader := bufio.NewReader(box)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if received != nil {
			received <- line
		}
		// Deliberately do nothing else: stay open and silent, like a real
		// accepted registration.
	}()
	return client
}

// rejectingRelayConn simulates the box's Mac door rejecting a registration:
// it reads the REGISTER line and then closes, which is exactly what the
// documented protocol does on a bad secret.
func rejectingRelayConn(t *testing.T) net.Conn {
	t.Helper()
	client, box := net.Pipe()
	go func() {
		reader := bufio.NewReader(box)
		_, _ = reader.ReadString('\n')
		_ = box.Close()
	}()
	return client
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
