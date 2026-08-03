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
	random              io.Reader
	startCodex          func(context.Context, string) (codexOwner, error)
	startTodoistProof   func(context.Context, io.Writer) (mobilesession.CapabilityFlow, io.Closer, error)
	startSpotifyProof   func(context.Context, io.Writer) (mobilesession.CapabilityFlow, io.Closer, error)
	startSlackProof     func(context.Context, io.Writer) (mobilesession.CapabilityFlow, io.Closer, error)
	startGoogleProof    func(context.Context, io.Writer) (mobilesession.CapabilityFlow, io.Closer, error)
	startMicrosoftProof func(context.Context, io.Writer) (mobilesession.CapabilityFlow, io.Closer, error)
	startMSTeamsProof   func(context.Context, io.Writer) (mobilesession.CapabilityFlow, io.Closer, error)
	startInstagramProof func(context.Context, io.Writer) (mobilesession.CapabilityFlow, error)
	startPodcastsProof  func(context.Context, io.Writer) (mobilesession.CapabilityFlow, error)
	startMapsProof      func(context.Context, io.Writer) (mobilesession.CapabilityFlow, error)
	startYouTubeProof   func(context.Context, io.Writer) (mobilesession.CapabilityFlow, error)
	startDeepLinkProof  func(context.Context, io.Writer) (mobilesession.CapabilityFlow, error)
	startProductionFlow func(context.Context, io.Writer) (mobilesession.CapabilityFlow, error)
	capabilityFlow      mobilesession.CapabilityFlow
	relayListen         func(context.Context, relayclient.Config) (net.Listener, error)
	now                 func() time.Time
	setup               cli.Setup
	installer           cli.Installer
	doctor              cli.Doctor
	health              *servicehealth.Store
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
		random:              random,
		startTodoistProof:   startTodoistProof,
		startSpotifyProof:   startSpotifyProof,
		startSlackProof:     startSlackProof,
		startGoogleProof:    startGoogleProof,
		startMicrosoftProof: startMicrosoftProof,
		startMSTeamsProof:   startMSTeamsProof,
		startInstagramProof: startInstagramProof,
		startPodcastsProof:  startPodcastsProof,
		startMapsProof:      startMapsProof,
		startYouTubeProof:   startYouTubeProof,
		startDeepLinkProof:  startDeepLinkProof,
		startProductionFlow: startProductionCapabilityFlow,
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
		now: time.Now,
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
	if len(args) == 1 && args[0] == "serve-todoist-proof" {
		if dependencies.startTodoistProof == nil {
			_, _ = io.WriteString(errorOutput, "Todoist proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, connection, startErr := dependencies.startTodoistProof(ctx, output)
		if startErr != nil || capabilityFlow == nil || connection == nil {
			slog.Error("[todoist-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil, "missing_connection", connection == nil)
			_, _ = io.WriteString(errorOutput, "Todoist proof sign-in could not start.\n")
			return 1
		}
		defer func() {
			if closeErr := connection.Close(); closeErr != nil {
				slog.Error("[todoist-proof-serve] token cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
		}()
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-spotify-proof" {
		if dependencies.startSpotifyProof == nil {
			_, _ = io.WriteString(errorOutput, "Spotify proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, connection, startErr := dependencies.startSpotifyProof(ctx, output)
		if startErr != nil || capabilityFlow == nil || connection == nil {
			slog.Error("[spotify-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil, "missing_connection", connection == nil)
			_, _ = io.WriteString(errorOutput, "Spotify proof sign-in could not start.\n")
			return 1
		}
		defer func() {
			if closeErr := connection.Close(); closeErr != nil {
				slog.Error("[spotify-proof-serve] token cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
		}()
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-slack-proof" {
		if dependencies.startSlackProof == nil {
			_, _ = io.WriteString(errorOutput, "Slack proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, connection, startErr := dependencies.startSlackProof(ctx, output)
		if startErr != nil || capabilityFlow == nil || connection == nil {
			slog.Error("[slack-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil, "missing_connection", connection == nil)
			_, _ = io.WriteString(errorOutput, "Slack proof sign-in could not start.\n")
			return 1
		}
		defer func() {
			if closeErr := connection.Close(); closeErr != nil {
				slog.Error("[slack-proof-serve] token cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
		}()
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-google-proof" {
		if dependencies.startGoogleProof == nil {
			_, _ = io.WriteString(errorOutput, "Google proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, connection, startErr := dependencies.startGoogleProof(ctx, output)
		if startErr != nil || capabilityFlow == nil || connection == nil {
			slog.Error("[google-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil, "missing_connection", connection == nil)
			_, _ = io.WriteString(errorOutput, "Google proof sign-in could not start.\n")
			return 1
		}
		defer func() {
			if closeErr := connection.Close(); closeErr != nil {
				slog.Error("[google-proof-serve] token cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
		}()
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-microsoft-proof" {
		if dependencies.startMicrosoftProof == nil {
			_, _ = io.WriteString(errorOutput, "Microsoft proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, connection, startErr := dependencies.startMicrosoftProof(ctx, output)
		if startErr != nil || capabilityFlow == nil || connection == nil {
			slog.Error("[microsoft-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil, "missing_connection", connection == nil)
			_, _ = io.WriteString(errorOutput, "Microsoft proof sign-in could not start.\n")
			return 1
		}
		defer func() {
			if closeErr := connection.Close(); closeErr != nil {
				slog.Error("[microsoft-proof-serve] token cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
		}()
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-msteams-proof" {
		if dependencies.startMSTeamsProof == nil {
			_, _ = io.WriteString(errorOutput, "Microsoft Teams proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, connection, startErr := dependencies.startMSTeamsProof(ctx, output)
		if startErr != nil || capabilityFlow == nil || connection == nil {
			slog.Error("[msteams-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil, "missing_connection", connection == nil)
			_, _ = io.WriteString(errorOutput, "Microsoft Teams proof sign-in could not start.\n")
			return 1
		}
		defer func() {
			if closeErr := connection.Close(); closeErr != nil {
				slog.Error("[msteams-proof-serve] token cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
		}()
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-instagram-proof" {
		if dependencies.startInstagramProof == nil {
			_, _ = io.WriteString(errorOutput, "Instagram proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, startErr := dependencies.startInstagramProof(ctx, output)
		if startErr != nil || capabilityFlow == nil {
			slog.Error("[instagram-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil)
			_, _ = io.WriteString(errorOutput, "Instagram proof serve could not start.\n")
			return 1
		}
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-podcasts-proof" {
		if dependencies.startPodcastsProof == nil {
			_, _ = io.WriteString(errorOutput, "Podcasts proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, startErr := dependencies.startPodcastsProof(ctx, output)
		if startErr != nil || capabilityFlow == nil {
			slog.Error("[podcasts-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil)
			_, _ = io.WriteString(errorOutput, "Podcasts proof serve could not start.\n")
			return 1
		}
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-maps-proof" {
		if dependencies.startMapsProof == nil {
			_, _ = io.WriteString(errorOutput, "Maps proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, startErr := dependencies.startMapsProof(ctx, output)
		if startErr != nil || capabilityFlow == nil {
			slog.Error("[maps-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil)
			_, _ = io.WriteString(errorOutput, "Maps proof serve could not start.\n")
			return 1
		}
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-youtube-proof" {
		if dependencies.startYouTubeProof == nil {
			_, _ = io.WriteString(errorOutput, "YouTube proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, startErr := dependencies.startYouTubeProof(ctx, output)
		if startErr != nil || capabilityFlow == nil {
			slog.Error("[youtube-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil)
			_, _ = io.WriteString(errorOutput, "YouTube proof serve could not start.\n")
			return 1
		}
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve-deeplink-proof" {
		if dependencies.startDeepLinkProof == nil {
			_, _ = io.WriteString(errorOutput, "Deep-link proof dependencies are unavailable.\n")
			return 1
		}
		capabilityFlow, startErr := dependencies.startDeepLinkProof(ctx, output)
		if startErr != nil || capabilityFlow == nil {
			slog.Error("[deeplink-proof-serve] startup failed", "error_class", fmt.Sprintf("%T", startErr), "missing_flow", capabilityFlow == nil)
			_, _ = io.WriteString(errorOutput, "Deep-link proof serve could not start.\n")
			return 1
		}
		dependencies.capabilityFlow = capabilityFlow
		return serve(ctx, config, errorOutput, dependencies)
	}
	if len(args) == 1 && args[0] == "serve" {
		// Plain `serve` is what every real phone actually talks to — the
		// `serve-<name>-proof` commands above only ever ran for the owner,
		// one adapter at a time, by hand. Without this, dependencies.capabilityFlow
		// stayed nil here forever and handler.go refused every capability
		// request a phone could send, no matter how many adapters were
		// finished. A missing router (no OPENAI_API_KEY) is not fatal to the
		// rest of the service — Codex sessions, pairing, and everything else
		// must keep working — so this only warns and leaves capabilityFlow
		// nil; it does not stop serve from starting.
		if dependencies.startProductionFlow != nil {
			capabilityFlow, buildErr := dependencies.startProductionFlow(ctx, output)
			if buildErr != nil || capabilityFlow == nil {
				slog.Warn("[production-serve] capability flow unavailable; capability requests will be refused",
					"error_class", fmt.Sprintf("%T", buildErr))
			} else {
				dependencies.capabilityFlow = capabilityFlow
			}
		}
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
		case "pair", "devices", "status", "doctor", "serve", "serve-todoist-proof", "serve-spotify-proof", "serve-slack-proof", "serve-google-proof", "serve-microsoft-proof", "serve-msteams-proof", "serve-instagram-proof", "serve-podcasts-proof", "serve-maps-proof", "serve-youtube-proof", "serve-deeplink-proof":
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
	if dependencies.random == nil || dependencies.startCodex == nil || dependencies.relayListen == nil || dependencies.now == nil {
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
		CapabilityFlow: dependencies.capabilityFlow,
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
	relayConfig, err := config.RelayClientConfig()
	if err != nil {
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
		_, _ = io.WriteString(errorOutput, "Companion relay configuration is invalid.\n")
		return 1
	}
	// The relay listener never falls back to a plain net.Listen: if the box
	// won't register us (bad secret, pin mismatch, box unreachable), the
	// service must fail closed rather than come up unreachable-but-alive.
	listener, err := dependencies.relayListen(serviceContext, relayConfig)
	if err != nil {
		updateHealth(dependencies.health, attemptID, servicehealth.StateFailed, servicehealth.ErrorListenUnavailable)
		_, _ = io.WriteString(errorOutput, "Companion could not register with the relay box.\n")
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
