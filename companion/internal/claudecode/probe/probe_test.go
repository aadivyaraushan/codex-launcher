package probe

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseVersionAcceptsTheRealCLIOutputShape(t *testing.T) {
	// Captured from `claude --version` on 2.1.153.
	version, err := ParseVersion("2.1.153 (Claude Code)\n")
	if err != nil {
		t.Fatalf("ParseVersion() error = %v", err)
	}
	if version != (Version{Major: 2, Minor: 1, Patch: 153}) {
		t.Fatalf("ParseVersion() = %#v", version)
	}
}

func TestParseVersionRejectsOutputFromSomethingElseNamedClaude(t *testing.T) {
	tests := []string{
		"",
		"2.1.153",
		"claude 2.1.153",
		"not a version (Claude Code)",
		"v2.1.153 (Claude Code)",
		"(Claude Code)",
	}
	for _, output := range tests {
		if _, err := ParseVersion(output); !errors.Is(err, ErrVersionMalformed) {
			t.Fatalf("ParseVersion(%q) error = %v; want ErrVersionMalformed", output, err)
		}
	}
}

func TestParseVersionKeepsUnexpectedOutputOutOfUnboundedErrors(t *testing.T) {
	noisy := make([]byte, 4096)
	for index := range noisy {
		noisy[index] = 'x'
	}
	_, err := ParseVersion(string(noisy))
	if err == nil {
		t.Fatal("oversized version output was accepted")
	}
	if len(err.Error()) > 200 {
		t.Fatalf("error text is unbounded at %d bytes", len(err.Error()))
	}
}

func TestVersionOrdering(t *testing.T) {
	tests := []struct {
		left  Version
		right Version
		want  bool
	}{
		{Version{2, 0, 999}, Version{2, 1, 0}, true},
		{Version{1, 9, 9}, Version{2, 1, 0}, true},
		{Version{2, 1, 0}, Version{2, 1, 0}, false},
		{Version{2, 1, 153}, Version{2, 1, 0}, false},
		{Version{3, 0, 0}, Version{2, 1, 0}, false},
	}
	for _, test := range tests {
		if got := test.left.Before(test.right); got != test.want {
			t.Fatalf("%s.Before(%s) = %v; want %v", test.left, test.right, got, test.want)
		}
	}
}

func TestValidateVersionRejectsACLITooOldForTheControlProtocol(t *testing.T) {
	run := func(context.Context, string) ([]byte, error) { return []byte("2.0.42 (Claude Code)\n"), nil }
	_, err := validateVersion(context.Background(), "claude", run)
	if !errors.Is(err, ErrVersionTooOld) {
		t.Fatalf("validateVersion() error = %v; want ErrVersionTooOld", err)
	}
}

func TestValidateVersionAcceptsTheVerifiedCLI(t *testing.T) {
	run := func(context.Context, string) ([]byte, error) { return []byte("2.1.153 (Claude Code)\n"), nil }
	version, err := validateVersion(context.Background(), "claude", run)
	if err != nil || version != (Version{2, 1, 153}) {
		t.Fatalf("validateVersion() = %#v, %v", version, err)
	}
}

func TestValidateVersionReportsAnUnrunnableBinary(t *testing.T) {
	run := func(context.Context, string) ([]byte, error) { return nil, errors.New("exec format error") }
	_, err := validateVersion(context.Background(), "claude", run)
	if !errors.Is(err, ErrVersionUnknown) {
		t.Fatalf("validateVersion() error = %v; want ErrVersionUnknown", err)
	}
}

func TestDiscoverBinaryPrefersTheConfiguredPathOverPATH(t *testing.T) {
	directory := t.TempDir()
	explicit := filepath.Join(directory, "claude")
	if err := os.WriteFile(explicit, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookup := func(string) (string, error) { return filepath.Join(directory, "wrong-claude"), nil }
	got, err := discoverBinary(explicit, lookup, os.Stat)
	if err != nil || got != explicit {
		t.Fatalf("discoverBinary() = %q, %v; want %q", got, err, explicit)
	}
}

func TestDiscoverBinaryRejectsADirectoryOrNonExecutable(t *testing.T) {
	directory := t.TempDir()
	notExecutable := filepath.Join(directory, "claude")
	if err := os.WriteFile(notExecutable, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	lookup := func(string) (string, error) { return "", errors.New("not in PATH") }
	for _, candidate := range []string{directory, notExecutable} {
		if _, err := discoverBinary(candidate, lookup, os.Stat); !errors.Is(err, ErrBinaryNotUsable) {
			t.Fatalf("discoverBinary(%q) error = %v; want ErrBinaryNotUsable", candidate, err)
		}
	}
}

func TestDiscoverBinaryReportsAMissingCLI(t *testing.T) {
	lookup := func(string) (string, error) { return "", errors.New("not in PATH") }
	stat := func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist }
	if _, err := discoverBinary("", lookup, stat); !errors.Is(err, ErrBinaryNotFound) {
		t.Fatalf("discoverBinary() error = %v; want ErrBinaryNotFound", err)
	}
	if _, err := discoverBinary("/nowhere/claude", lookup, stat); !errors.Is(err, ErrBinaryNotFound) {
		t.Fatalf("discoverBinary(explicit missing) error = %v; want ErrBinaryNotFound", err)
	}
}

// The gate is only meaningful if it matches the CLI the owner actually has, so
// when one is installed this asserts against the real thing.
func TestInstalledCLIMatchesTheSupportedShape(t *testing.T) {
	binary, err := DiscoverBinary("")
	if err != nil {
		t.Skip("no Claude Code CLI on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	version, err := ValidateVersion(ctx, binary)
	if err != nil {
		t.Fatalf("installed CLI at %s failed validation: %v", binary, err)
	}
	t.Logf("validated Claude Code %s at %s", version, binary)
}
