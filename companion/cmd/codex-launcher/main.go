package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	codexruntime "github.com/codex-launcher/codex-launcher/companion/internal/codex/runtime"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

type companionTaskSource = mobilesession.TaskSource

type codexOwner interface {
	TaskSource() companionTaskSource
	TaskEvents() <-chan taskstate.MobileEvent
	DecisionOwner() *decisions.AppServerOwner
	DecisionRequests() <-chan appserver.ServerRequest
	DesktopDecisionRequests() <-chan appserver.ServerRequest
	Done() <-chan struct{}
	Close() error
}

type liveCodexOwner struct{ session *codexruntime.Session }

func (owner liveCodexOwner) TaskSource() companionTaskSource { return owner.session.Tasks() }
func (owner liveCodexOwner) TaskEvents() <-chan taskstate.MobileEvent {
	return owner.session.TaskEvents()
}
func (owner liveCodexOwner) DecisionOwner() *decisions.AppServerOwner {
	return owner.session.DecisionOwner()
}
func (owner liveCodexOwner) DecisionRequests() <-chan appserver.ServerRequest {
	return owner.session.DecisionRequests()
}
func (owner liveCodexOwner) DesktopDecisionRequests() <-chan appserver.ServerRequest {
	return owner.session.DesktopDecisionRequests()
}
func (owner liveCodexOwner) Done() <-chan struct{} { return owner.session.Done() }
func (owner liveCodexOwner) Close() error          { return owner.session.Close() }

type liveDependencies struct {
	random     io.Reader
	startCodex func(context.Context, string) (codexOwner, error)
	listen     func(string, string) (net.Listener, error)
	now        func() time.Time
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, rand.Reader))
}

func run(ctx context.Context, args []string, output, errorOutput io.Writer, random io.Reader) int {
	return runWith(ctx, args, output, errorOutput, liveDependencies{
		random: random,
		startCodex: func(ctx context.Context, binary string) (codexOwner, error) {
			session, err := codexruntime.Start(ctx, codexruntime.Options{Binary: binary, ExperimentalQuestions: true})
			if err != nil {
				return nil, err
			}
			return liveCodexOwner{session: session}, nil
		},
		listen: net.Listen,
		now:    time.Now,
	})
}

func runWith(ctx context.Context, args []string, output, errorOutput io.Writer, dependencies liveDependencies) int {
	if !needsConfiguredRuntime(args) {
		return cli.New(cli.Options{Output: output, ErrorOutput: errorOutput}).Run(ctx, args)
	}
	root, err := companionapp.ConfigRoot()
	if err != nil {
		_, _ = io.WriteString(errorOutput, "Companion setup is incomplete.\n")
		return 1
	}
	config, err := companionapp.LoadConfig(filepath.Join(root, "config.json"))
	if err != nil {
		_, _ = io.WriteString(errorOutput, "Companion setup is incomplete.\n")
		return 1
	}
	if len(args) == 1 && args[0] == "serve" {
		return serve(ctx, config, errorOutput, dependencies)
	}
	runtime, state, err := companionapp.OpenPersistentRuntime(ctx, config, companionapp.PersistentDependencies{Random: dependencies.random})
	if err != nil {
		_, _ = io.WriteString(errorOutput, "Companion state is unavailable.\n")
		return 1
	}
	defer state.Close()
	return cli.New(cli.Options{Runtime: runtime, Output: output, ErrorOutput: errorOutput}).Run(ctx, args)
}

func needsConfiguredRuntime(args []string) bool {
	if len(args) == 1 {
		switch args[0] {
		case "pair", "devices", "status", "doctor", "serve":
			return true
		}
	}
	return len(args) == 2 && args[0] == "revoke"
}

func serve(ctx context.Context, config companionapp.Config, errorOutput io.Writer, dependencies liveDependencies) int {
	if dependencies.random == nil || dependencies.startCodex == nil || dependencies.listen == nil || dependencies.now == nil {
		_, _ = io.WriteString(errorOutput, "Companion service dependencies are unavailable.\n")
		return 1
	}
	serviceContext, cancelService := context.WithCancel(ctx)
	owner, err := dependencies.startCodex(serviceContext, config.CodexBinary)
	if err != nil {
		cancelService()
		_, _ = io.WriteString(errorOutput, "Codex is unavailable on this computer.\n")
		return 1
	}
	if owner == nil || owner.TaskSource() == nil || owner.TaskEvents() == nil || owner.Done() == nil {
		cancelService()
		if owner != nil {
			_ = owner.Close()
		}
		_, _ = io.WriteString(errorOutput, "Codex is unavailable on this computer.\n")
		return 1
	}
	runtime, state, err := companionapp.OpenPersistentRuntime(serviceContext, config, companionapp.PersistentDependencies{
		Random: dependencies.random, TaskSource: owner.TaskSource(), TaskEvents: owner.TaskEvents(), DecisionOwner: owner.DecisionOwner(), DecisionRequests: owner.DecisionRequests(), DesktopDecisionRequests: owner.DesktopDecisionRequests(),
	})
	if err != nil {
		cancelService()
		_ = owner.Close()
		_, _ = io.WriteString(errorOutput, "Companion state is unavailable.\n")
		return 1
	}
	defer state.Close()
	defer owner.Close()
	defer cancelService()
	go func() {
		select {
		case <-owner.Done():
			cancelService()
		case <-serviceContext.Done():
		}
	}()
	certificate, err := runtime.Pairing.TLSCertificate(dependencies.now())
	if err != nil {
		_, _ = io.WriteString(errorOutput, "Companion TLS identity is unavailable.\n")
		return 1
	}
	listener, err := dependencies.listen("tcp", net.JoinHostPort(config.ListenHost, fmt.Sprintf("%d", config.ListenPort)))
	if err != nil {
		_, _ = io.WriteString(errorOutput, "Companion could not listen on the configured Tailscale address.\n")
		return 1
	}
	defer listener.Close()
	if err := runtime.Mobile.Serve(serviceContext, listener, certificate); err != nil {
		_, _ = io.WriteString(errorOutput, "Companion service stopped unexpectedly.\n")
		return 1
	}
	if ctx.Err() == nil {
		select {
		case <-owner.Done():
			_, _ = io.WriteString(errorOutput, "Codex stopped, so the companion service was closed.\n")
			return 1
		default:
		}
	}
	return 0
}
