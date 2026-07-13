package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/probe"
)

type Options struct {
	Binary                string
	ExperimentalQuestions bool
	Logger                *slog.Logger
}

type Session struct {
	client      *appserver.Client
	command     *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	logger      *slog.Logger
	ctx         context.Context
	done        chan struct{}
	closing     atomic.Bool
	stopLogged  atomic.Bool
	closeOnce   sync.Once
	closeResult error
}

type starter struct {
	discover func(string) (string, error)
	validate func(context.Context, string) (string, error)
	command  func(context.Context, string, ...string) *exec.Cmd
	logger   *slog.Logger
}

func Start(ctx context.Context, options Options) (*Session, error) {
	return starter{
		discover: probe.DiscoverBinary,
		validate: probe.ValidateVersion,
		command:  exec.CommandContext,
		logger:   options.Logger,
	}.start(ctx, options)
}

func (start starter) start(ctx context.Context, options Options) (*Session, error) {
	if ctx == nil || start.discover == nil || start.validate == nil || start.command == nil {
		return nil, errors.New("Codex process dependency is missing")
	}
	logger := options.Logger
	if logger == nil {
		logger = start.logger
	}
	if logger == nil {
		logger = slog.Default()
	}
	binarySource := "path"
	if options.Binary != "" {
		binarySource = "explicit"
	}
	logger.Info("[codex-process] start requested", "binary_source", binarySource, "input_shape", "local_stdio,version_check")
	binary, err := start.discover(options.Binary)
	if err != nil {
		logger.Error("[codex-process] binary discovery failed", "binary_source", binarySource, "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("discover Codex binary: %w", err)
	}
	_, err = start.validate(ctx, binary)
	if err != nil {
		logger.Error("[codex-process] version check failed", "binary_source", binarySource, "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("validate Codex binary: %w", err)
	}
	command := start.command(ctx, binary, "app-server", "--stdio")
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open Codex app-server stdout: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		logger.Error("[codex-process] child start failed", "binary_source", binarySource, "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	session := &Session{
		client:  appserver.NewClient(stdout, stdin, logger, appserver.Options{ExperimentalQuestions: options.ExperimentalQuestions}),
		command: command, stdin: stdin, stdout: stdout, logger: logger,
		ctx: ctx, done: make(chan struct{}),
	}
	go session.wait()
	if err := session.client.Initialize(ctx); err != nil {
		session.closing.Store(true)
		_ = session.Close()
		logger.Error("[codex-process] initialization failed", "version_checked", true, "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("initialize Codex app-server process: %w", err)
	}
	logger.Info("[codex-process] initialized", "version_checked", true, "experimental_questions", options.ExperimentalQuestions, "output_shape", "local_app_server_client")
	return session, nil
}

func (session *Session) Client() *appserver.Client {
	if session == nil {
		return nil
	}
	return session.client
}

func (session *Session) Done() <-chan struct{} {
	if session == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return session.done
}

func (session *Session) Close() error {
	if session == nil {
		return nil
	}
	session.closeOnce.Do(func() {
		session.closing.Store(true)
		_ = session.stdin.Close()
		_ = session.stdout.Close()
		if session.command.Process != nil {
			if err := session.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				session.closeResult = fmt.Errorf("stop Codex app-server: %w", err)
			}
		}
		<-session.done
		if session.stopLogged.CompareAndSwap(false, true) {
			session.logger.Info("[codex-process] stopped", "decision", "owned_child_closed")
		}
	})
	return session.closeResult
}

func (session *Session) wait() {
	err := session.command.Wait()
	switch {
	case session.closing.Load():
	case session.ctx.Err() != nil:
		session.stopLogged.Store(true)
		session.logger.Info("[codex-process] stopped", "decision", "context_cancelled")
	default:
		errorClass := "clean_exit"
		if err != nil {
			errorClass = fmt.Sprintf("%T", err)
		}
		session.stopLogged.Store(true)
		session.logger.Error("[codex-process] child exited", "branch_reason", "unexpected_exit", "error_class", errorClass)
	}
	close(session.done)
}
