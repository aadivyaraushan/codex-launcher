// Package probe finds the Claude Code CLI on this computer and refuses to use
// one that is too old to speak the protocol the companion depends on.
//
// The companion drives Claude Code over stdio with
// `--input-format stream-json --output-format stream-json`, and it routes tool
// approvals to the phone through `--permission-prompt-tool stdio`, which makes
// the CLI emit `can_use_tool` control requests. None of that is a published
// stability contract, so the version gate here is deliberately strict: a CLI
// that cannot be confirmed compatible is rejected at startup rather than
// discovered to be incompatible halfway through somebody's task.
package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// MinimumVersion is the oldest Claude Code CLI the companion will drive. The
// integration was verified against 2.1.153; 2.1.0 is the floor because that is
// the series whose control protocol shape we have checked.
var MinimumVersion = Version{Major: 2, Minor: 1, Patch: 0}

var (
	ErrBinaryNotFound   = errors.New("Claude Code CLI was not found on this computer")
	ErrBinaryNotUsable  = errors.New("Claude Code CLI is not an executable file")
	ErrVersionUnknown   = errors.New("Claude Code CLI version could not be read")
	ErrVersionTooOld    = errors.New("Claude Code CLI is too old for this companion")
	ErrVersionMalformed = errors.New("Claude Code CLI version output was not understood")
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func (version Version) String() string {
	return fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
}

// Before reports whether version sorts strictly before other.
func (version Version) Before(other Version) bool {
	if version.Major != other.Major {
		return version.Major < other.Major
	}
	if version.Minor != other.Minor {
		return version.Minor < other.Minor
	}
	return version.Patch < other.Patch
}

// DiscoverBinary resolves the CLI path, preferring an explicit configured path
// over PATH. The native installer and the npm global install put `claude` in
// different places, so an explicit path is the supported escape hatch when the
// service's PATH does not match the owner's login shell.
func DiscoverBinary(explicit string) (string, error) {
	return discoverBinary(explicit, exec.LookPath, os.Stat)
}

func discoverBinary(explicit string, lookup func(string) (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	binary := explicit
	if binary == "" {
		found, err := lookup("claude")
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrBinaryNotFound, err)
		}
		binary = found
	}
	info, err := stat(binary)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBinaryNotFound, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%w: %s", ErrBinaryNotUsable, binary)
	}
	return binary, nil
}

// versionPattern matches the CLI's `--version` line, which reads like
// "2.1.153 (Claude Code)".
var versionPattern = regexp.MustCompile(`^(\d{1,6})\.(\d{1,6})\.(\d{1,6})\b`)

// ParseVersion reads the version out of `claude --version` output. The trailing
// product name is required: matching a bare version number would happily accept
// any program on PATH named claude.
func ParseVersion(output string) (Version, error) {
	line := strings.TrimSpace(output)
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	if !strings.Contains(line, "(Claude Code)") {
		return Version{}, fmt.Errorf("%w: %q", ErrVersionMalformed, bounded(line))
	}
	match := versionPattern.FindStringSubmatch(line)
	if match == nil {
		return Version{}, fmt.Errorf("%w: %q", ErrVersionMalformed, bounded(line))
	}
	major, majorErr := strconv.Atoi(match[1])
	minor, minorErr := strconv.Atoi(match[2])
	patch, patchErr := strconv.Atoi(match[3])
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return Version{}, fmt.Errorf("%w: %q", ErrVersionMalformed, bounded(line))
	}
	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// ValidateVersion runs the CLI's version command and enforces MinimumVersion.
func ValidateVersion(ctx context.Context, binary string) (Version, error) {
	return validateVersion(ctx, binary, func(ctx context.Context, binary string) ([]byte, error) {
		return exec.CommandContext(ctx, binary, "--version").Output()
	})
}

func validateVersion(ctx context.Context, binary string, run func(context.Context, string) ([]byte, error)) (Version, error) {
	output, err := run(ctx, binary)
	if err != nil {
		return Version{}, fmt.Errorf("%w: %v", ErrVersionUnknown, err)
	}
	version, err := ParseVersion(string(output))
	if err != nil {
		return Version{}, err
	}
	if version.Before(MinimumVersion) {
		return Version{}, fmt.Errorf("%w: found %s, need %s or newer", ErrVersionTooOld, version, MinimumVersion)
	}
	return version, nil
}

// Resolve discovers and version-checks in one step, which is what every caller
// outside tests actually wants.
func Resolve(ctx context.Context, explicit string) (string, Version, error) {
	binary, err := DiscoverBinary(explicit)
	if err != nil {
		return "", Version{}, err
	}
	version, err := ValidateVersion(ctx, binary)
	if err != nil {
		return "", Version{}, err
	}
	return binary, version, nil
}

// bounded keeps unexpected CLI output out of logs at unbounded length.
func bounded(value string) string {
	const maximum = 120
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}
