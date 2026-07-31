package applenotes

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout bounds a single osascript invocation when
// OsascriptRunner.Timeout is left at zero.
const DefaultTimeout = 15 * time.Second

// argOrder lists, for each script, the Args keys in the order its "on run
// argv" template expects them. This is the one place that fixes the
// mapping from Script.Args (unordered) to argv (ordered); the templates
// themselves never see anything but positional argv values.
var argOrder = map[string][]string{
	ScriptFolderExists: {"folder"},
	ScriptCreateFolder: {"folder"},
	ScriptCreateNote:   {"folder", "title", "body"},
	ScriptReadNotes:    {"folder"},
}

// OsascriptRunner is the real ScriptRunner: it shells out to osascript on
// the owner's own Mac. It never splices user text into the script source —
// Script.Source is always a fixed template, and every piece of user text
// travels as a separate osascript command-line argument, landing in argv
// positions the template indexes into via "on run argv".
type OsascriptRunner struct {
	// Timeout bounds how long a single osascript invocation may run before
	// it is killed. Zero means DefaultTimeout.
	Timeout time.Duration
}

// Run executes one Script via osascript and returns its trimmed stdout. On
// failure it returns an error carrying osascript's stderr.
func (r OsascriptRunner) Run(ctx context.Context, s Script) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, 0, 3+len(argOrder[s.Name]))
	args = append(args, "-e", s.Source, "--")
	for _, key := range argOrder[s.Name] {
		args = append(args, s.Args[key])
	}

	cmd := exec.CommandContext(ctx, "osascript", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("osascript %s: %s", s.Name, msg)
	}

	return strings.TrimSpace(stdout.String()), nil
}
