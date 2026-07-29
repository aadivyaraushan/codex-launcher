// Package process supervises one Claude Code CLI child.
//
// A CLI child serves exactly one conversation, which is the structural
// difference from the Codex app-server: there is no single long-lived server to
// multiplex tasks over. The companion therefore spawns a child per turn and
// resumes the session from disk for the next one, which keeps the number of
// live children bounded by the number of turns actually in flight.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/codex-launcher/codex-launcher/companion/internal/claudecode/streamjson"
)

var (
	ErrInvalidOptions = errors.New("Claude Code session options are invalid")
	ErrStartFailed    = errors.New("Claude Code session could not be started")
)

// PermissionMode values the CLI accepts. The companion only uses these three:
// they are what the phone's three permission choices map onto.
const (
	PermissionModeDefault = "default"
	PermissionModePlan    = "plan"
	PermissionModeBypass  = "bypassPermissions"
)

type Options struct {
	// Binary is the validated CLI path from the probe package.
	Binary string
	// SessionID starts a new conversation under an ID the companion chose.
	// Exactly one of SessionID and Resume must be set.
	SessionID string
	// Resume continues an existing conversation from its on-disk transcript.
	Resume string
	// ProjectPath is the approved project the CLI runs inside. It becomes the
	// child's working directory, which is what bounds the agent's file access,
	// so it must be an absolute path that the caller has already checked
	// against the owner's approved projects.
	ProjectPath    string
	Model          string
	Effort         string
	PermissionMode string
	Logger         *slog.Logger
}

func (options Options) validate() error {
	switch {
	case strings.TrimSpace(options.Binary) == "":
		return fmt.Errorf("%w: no CLI binary", ErrInvalidOptions)
	case options.SessionID == "" && options.Resume == "":
		return fmt.Errorf("%w: neither a new session ID nor a session to resume", ErrInvalidOptions)
	case options.SessionID != "" && options.Resume != "":
		return fmt.Errorf("%w: a session cannot be both new and resumed", ErrInvalidOptions)
	case !filepath.IsAbs(options.ProjectPath):
		return fmt.Errorf("%w: project path must be absolute", ErrInvalidOptions)
	case !validPermissionMode(options.PermissionMode):
		return fmt.Errorf("%w: unsupported permission mode %q", ErrInvalidOptions, options.PermissionMode)
	}
	for _, value := range []string{options.SessionID, options.Resume, options.Model, options.Effort, options.ProjectPath} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%w: option contains a line break", ErrInvalidOptions)
		}
	}
	return nil
}

func validPermissionMode(mode string) bool {
	switch mode {
	case PermissionModeDefault, PermissionModePlan, PermissionModeBypass:
		return true
	default:
		return false
	}
}

// CommandArgs builds the CLI invocation. It is exported so the wiring above can
// be asserted without spawning anything.
func CommandArgs(options Options) []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		// Routes every tool approval to the companion as a can_use_tool control
		// request, which is what becomes an approval sheet on the phone. Without
		// it the CLI would decide on its own and the owner would never be asked.
		"--permission-prompt-tool", "stdio",
	}
	if options.SessionID != "" {
		args = append(args, "--session-id", options.SessionID)
	} else {
		args = append(args, "--resume", options.Resume)
	}
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.Effort != "" {
		args = append(args, "--effort", options.Effort)
	}
	args = append(args, "--permission-mode", options.PermissionMode)
	return args
}

// Session is one supervised CLI child and the stream client bound to it.
type Session struct {
	client  *streamjson.Client
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	logger  *slog.Logger

	done        chan struct{}
	closing     atomic.Bool
	stopLogged  atomic.Bool
	closeOnce   sync.Once
	closeResult error
}

type starter struct {
	command func(context.Context, string, ...string) *exec.Cmd
}

// Start spawns the CLI and completes the control-protocol handshake.
func Start(ctx context.Context, options Options) (*Session, error) {
	return starter{command: exec.CommandContext}.start(ctx, options)
}

func (start starter) start(ctx context.Context, options Options) (*Session, error) {
	if ctx == nil || start.command == nil {
		return nil, fmt.Errorf("%w: missing dependency", ErrInvalidOptions)
	}
	if err := options.validate(); err != nil {
		return nil, err
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	mode := "resume"
	if options.SessionID != "" {
		mode = "new"
	}
	logger.Info("[claude-process] start requested",
		"session_mode", mode, "permission_mode", options.PermissionMode,
		"model_selected", options.Model != "", "effort_selected", options.Effort != "",
		"input_shape", "local_stdio,stream_json")

	command := start.command(ctx, options.Binary, CommandArgs(options)...)
	command.Dir = options.ProjectPath
	// The CLI's stderr carries diagnostics that could quote file contents, and
	// nothing reads it, so it is dropped rather than pooled in memory.
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: open stdin: %v", ErrStartFailed, err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("%w: open stdout: %v", ErrStartFailed, err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		logger.Error("[claude-process] child start failed", "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("%w: %v", ErrStartFailed, err)
	}

	session := &Session{
		client:  streamjson.NewClient(stdout, stdin, logger),
		command: command, stdin: stdin, stdout: stdout, logger: logger,
		done: make(chan struct{}),
	}
	go session.wait()
	session.client.Start()
	if err := session.client.Initialize(ctx); err != nil {
		session.closing.Store(true)
		_ = session.Close()
		logger.Error("[claude-process] handshake failed", "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("%w: %v", ErrStartFailed, err)
	}
	logger.Info("[claude-process] ready", "session_mode", mode, "output_shape", "stream_json_client")
	return session, nil
}

func (session *Session) Client() *streamjson.Client {
	if session == nil {
		return nil
	}
	return session.client
}

// Done closes when the child exits.
func (session *Session) Done() <-chan struct{} {
	if session == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return session.done
}

// Close stops the child and waits for it to be reaped. It never leaves an
// orphaned CLI process behind, which matters because each one holds the
// owner's Claude credentials open.
func (session *Session) Close() error {
	if session == nil {
		return nil
	}
	session.closeOnce.Do(func() {
		session.closing.Store(true)
		_ = session.client.Close()
		_ = session.stdin.Close()
		_ = session.stdout.Close()
		if session.command.Process != nil {
			if err := session.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				session.closeResult = fmt.Errorf("stop Claude Code session: %w", err)
			}
		}
		<-session.done
		if session.stopLogged.CompareAndSwap(false, true) {
			session.logger.Info("[claude-process] stopped", "decision", "owned_child_closed")
		}
	})
	return session.closeResult
}

func (session *Session) wait() {
	err := session.command.Wait()
	switch {
	case session.closing.Load():
	default:
		errorClass := "clean_exit"
		if err != nil {
			errorClass = fmt.Sprintf("%T", err)
		}
		session.stopLogged.Store(true)
		session.logger.Info("[claude-process] child exited", "branch_reason", "turn_finished_or_exited", "error_class", errorClass)
	}
	close(session.done)
}
