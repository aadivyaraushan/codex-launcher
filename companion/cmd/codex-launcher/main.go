package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	codexruntime "github.com/codex-launcher/codex-launcher/companion/internal/codex/runtime"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostdoctor"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/launchd"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/systemd"
	windowservice "github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service/windows"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostmaintenance"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostsetup"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
	"github.com/codex-launcher/codex-launcher/companion/internal/servicehealth"
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
	random       io.Reader
	startCodex   func(context.Context, string) (codexOwner, error)
	relayListen  func(context.Context, relayclient.Config) (net.Listener, error)
	directListen func(string, string) (net.Listener, error)
	now          func() time.Time
	setup        cli.Setup
	installer    cli.Installer
	doctor       cli.Doctor
	health       *servicehealth.Store
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, rand.Reader))
}

func run(ctx context.Context, args []string, output, errorOutput io.Writer, random io.Reader) int {
	if len(args) > 0 && args[0] == "_maintenance" {
		return runMaintenance(ctx, args[1:], errorOutput)
	}
	dependencies := liveDependencies{
		random: random,
		startCodex: func(ctx context.Context, binary string) (codexOwner, error) {
			session, err := codexruntime.Start(ctx, codexruntime.Options{Binary: binary, ExperimentalQuestions: true})
			if err != nil {
				return nil, err
			}
			return liveCodexOwner{session: session}, nil
		},
		relayListen: func(ctx context.Context, cfg relayclient.Config) (net.Listener, error) {
			return relayclient.Listen(ctx, cfg)
		},
		directListen: net.Listen,
		now:          time.Now,
	}
	if root, err := companionapp.ConfigRoot(); err == nil {
		dependencies.health = servicehealth.New(filepath.Join(root, "health.json"), time.Now)
	}
	if usesLocalHostOperations(args) {
		setup, installer, doctor, err := defaultLocalHostOperations()
		if err != nil {
			_, _ = io.WriteString(errorOutput, "Companion installer is unavailable on this computer.\n")
			return 1
		}
		dependencies.setup = setup
		dependencies.installer = installer
		dependencies.doctor = doctor
	}
	return runWith(ctx, args, output, errorOutput, dependencies)
}

func runWith(ctx context.Context, args []string, output, errorOutput io.Writer, dependencies liveDependencies) int {
	if !needsConfiguredRuntime(args) {
		return cli.New(cli.Options{Output: output, ErrorOutput: errorOutput, Setup: dependencies.setup, Installer: dependencies.installer}).Run(ctx, args)
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
	if len(args) == 1 && args[0] == "doctor" {
		return cli.New(cli.Options{Config: &config, Output: output, ErrorOutput: errorOutput, Doctor: dependencies.doctor, Installer: dependencies.installer}).Run(ctx, args)
	}
	runtime, state, err := companionapp.OpenPersistentRuntime(ctx, config, companionapp.PersistentDependencies{Random: dependencies.random})
	if err != nil {
		_, _ = io.WriteString(errorOutput, "Companion state is unavailable.\n")
		return 1
	}
	defer state.Close()
	return cli.New(cli.Options{Runtime: runtime, Output: output, ErrorOutput: errorOutput, Doctor: dependencies.doctor, Installer: dependencies.installer}).Run(ctx, args)
}

func usesLocalHostOperations(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "setup", "install", "rollback", "uninstall", "status", "doctor":
		return true
	default:
		return false
	}
}

func defaultLocalHostOperations() (cli.Setup, cli.Installer, cli.Doctor, error) {
	root, err := companionapp.ConfigRoot()
	if err != nil {
		return nil, nil, nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, nil, err
	}
	source, err := os.Executable()
	if err != nil {
		return nil, nil, nil, err
	}
	backend, err := platformBackend(runtime.GOOS, home, root)
	if err != nil {
		return nil, nil, nil, err
	}
	installer := hostinstall.New(hostinstall.Options{
		Root: root, SourceExecutable: source, Backend: backend,
		ValidateBinary:  validateCompanionBinary,
		StartHealth:     servicehealth.New(filepath.Join(root, "health.json"), time.Now),
		DeferredMutator: hostmaintenance.New(),
		MigrateConfig: func() (func() error, error) {
			if _, err := companionapp.LoadConfig(filepath.Join(root, "config.json")); err != nil {
				return nil, err
			}
			return func() error { return nil }, nil
		},
	})
	setupValidator := hostsetup.New(hostsetup.Options{
		WriteConfig: func(config companionapp.Config) error {
			return companionapp.UpdateConfig(filepath.Join(root, "config.json"), config)
		},
	})
	setup := func(ctx context.Context, config companionapp.Config) error {
		if config.Connection.Mode == companionapp.ConnectionModeTailscale && config.Connection.Tailscale.Host == "" {
			address, discoverErr := setupValidator.DiscoverTailscaleAddress(ctx, "")
			if discoverErr != nil {
				return discoverErr
			}
			config.Connection.Tailscale.Host = address
		}
		return installer.Reconfigure(ctx, func() error { return setupValidator.Save(ctx, config) })
	}
	doctor := hostdoctor.New(hostdoctor.Options{
		ServiceStatus: installer.Status,
		StatePath:     filepath.Join(root, "state.sqlite3"),
		LastError:     servicehealth.New(filepath.Join(root, "health.json"), time.Now).LastError,
	})
	return setup, installer, doctor.Run, nil
}

func runMaintenance(ctx context.Context, args []string, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("_maintenance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	waitPID := flags.Int("wait-pid", 0, "")
	operation := flags.String("operation", "", "")
	artifact := flags.String("artifact", "", "")
	receipt := flags.String("receipt", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *waitPID <= 0 || *receipt == "" || (*operation != "replace" && *operation != "rollback" && *operation != "uninstall") || (*operation == "replace" && *artifact == "") {
		_, _ = io.WriteString(errorOutput, "Invalid deferred maintenance request.\n")
		return 2
	}
	if err := hostmaintenance.WaitForParent(*waitPID); err != nil {
		_ = hostmaintenance.WriteReceipt(*receipt, *operation, err)
		return 1
	}
	_, installer, _, setupErr := defaultLocalHostOperations()
	operationErr := setupErr
	if operationErr == nil {
		switch *operation {
		case "replace":
			operationErr = installer.Replace(ctx, *artifact)
		case "rollback":
			operationErr = installer.Rollback(ctx)
		case "uninstall":
			operationErr = installer.Uninstall(ctx)
		}
	}
	cleanupErr := hostmaintenance.CleanupSelf()
	resultErr := errors.Join(operationErr, cleanupErr)
	if err := hostmaintenance.WriteReceipt(*receipt, *operation, resultErr); err != nil {
		return 1
	}
	if resultErr != nil {
		return 1
	}
	return 0
}

func platformBackend(goos, home, root string) (hostinstall.Backend, error) {
	switch goos {
	case "darwin":
		current, err := user.Current()
		if err != nil {
			return nil, err
		}
		uid, err := strconv.Atoi(current.Uid)
		if err != nil {
			return nil, err
		}
		return launchd.New(launchd.Options{Home: home, UID: uid}), nil
	case "linux":
		return systemd.New(systemd.Options{ConfigHome: filepath.Dir(root)}), nil
	case "windows":
		return windowservice.New(windowservice.Options{MarkerPath: filepath.Join(root, "service", "windows-task-installed")}), nil
	default:
		return nil, fmt.Errorf("unsupported companion platform: %s", goos)
	}
}

func validateCompanionBinary(path string) error {
	output, err := exec.Command(path, "version").Output()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "codex-launcher ") {
		return fmt.Errorf("unexpected companion version output")
	}
	return nil
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
	attemptID := ""
	if dependencies.health != nil {
		var err error
		attemptID, err = dependencies.health.BeginAttempt()
		if err != nil {
			slog.Error("[service-health] start attempt unavailable", "error_class", "health_start_request")
			_, _ = io.WriteString(errorOutput, "Companion start health check is unavailable.\n")
			return 1
		}
	}
	updateHealth(dependencies.health, attemptID, servicehealth.StateStarting, "")
	if dependencies.random == nil || dependencies.startCodex == nil || dependencies.now == nil || (config.ConnectionMode() == companionapp.ConnectionModeRelay && dependencies.relayListen == nil) || (config.ConnectionMode() == companionapp.ConnectionModeTailscale && dependencies.directListen == nil) {
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorServiceDependencies)
		_, _ = io.WriteString(errorOutput, "Companion service dependencies are unavailable.\n")
		return 1
	}
	serviceContext, cancelService := context.WithCancel(ctx)
	owner, err := dependencies.startCodex(serviceContext, config.CodexBinary)
	if err != nil {
		cancelService()
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorCodexUnavailable)
		_, _ = io.WriteString(errorOutput, "Codex is unavailable on this computer.\n")
		return 1
	}
	if owner == nil || owner.TaskSource() == nil || owner.TaskEvents() == nil || owner.Done() == nil {
		cancelService()
		if owner != nil {
			_ = owner.Close()
		}
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorCodexUnavailable)
		_, _ = io.WriteString(errorOutput, "Codex is unavailable on this computer.\n")
		return 1
	}
	runtime, state, err := companionapp.OpenPersistentRuntime(serviceContext, config, companionapp.PersistentDependencies{
		Random: dependencies.random, TaskSource: owner.TaskSource(), TaskEvents: owner.TaskEvents(), DecisionOwner: owner.DecisionOwner(), DecisionRequests: owner.DecisionRequests(), DesktopDecisionRequests: owner.DesktopDecisionRequests(),
	})
	if err != nil {
		cancelService()
		_ = owner.Close()
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorStateUnavailable)
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
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorTLSIdentityUnavailable)
		_, _ = io.WriteString(errorOutput, "Companion TLS identity is unavailable.\n")
		return 1
	}
	var listener net.Listener
	switch config.ConnectionMode() {
	case companionapp.ConnectionModeRelay:
		relayConfig, relayErr := config.RelayClientConfig()
		if relayErr != nil {
			updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
			_, _ = io.WriteString(errorOutput, "Companion relay configuration is invalid.\n")
			return 1
		}
		// The relay listener never falls back to a direct listener: if the box
		// cannot register this Mac, the configured relay service fails closed.
		listener, err = dependencies.relayListen(serviceContext, relayConfig)
		if err != nil {
			updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
			_, _ = io.WriteString(errorOutput, "Companion could not register with the relay box.\n")
			return 1
		}
	case companionapp.ConnectionModeTailscale:
		target, targetErr := config.PairingTarget()
		if targetErr != nil {
			updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
			_, _ = io.WriteString(errorOutput, "Companion Tailscale configuration is invalid.\n")
			return 1
		}
		listener, err = dependencies.directListen("tcp", net.JoinHostPort(target.Host, strconv.Itoa(target.Port)))
		if err != nil {
			updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
			_, _ = io.WriteString(errorOutput, "Companion could not bind the configured Tailscale address.\n")
			return 1
		}
	default:
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
		_, _ = io.WriteString(errorOutput, "Companion connection mode is invalid.\n")
		return 1
	}
	defer listener.Close()
	updateHealth(dependencies.health, attemptID, servicehealth.StateRunning, "")
	if err := runtime.Mobile.Serve(serviceContext, listener, certificate); err != nil {
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorServiceStoppedUnexpectedly)
		_, _ = io.WriteString(errorOutput, "Companion service stopped unexpectedly.\n")
		return 1
	}
	if ctx.Err() == nil {
		select {
		case <-owner.Done():
			updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorServiceStoppedUnexpectedly)
			_, _ = io.WriteString(errorOutput, "Codex stopped, so the companion service was closed.\n")
			return 1
		default:
		}
	}
	updateHealth(dependencies.health, attemptID, servicehealth.StateStopped, "")
	return 0
}

func updateHealth(store *servicehealth.Store, attemptID string, state servicehealth.State, errorCode string) {
	if store == nil {
		return
	}
	if err := store.UpdateAttempt(attemptID, state, errorCode); err != nil {
		slog.Error("[service-health] update failed", "error_class", "health_write", "state", state)
	}
}
