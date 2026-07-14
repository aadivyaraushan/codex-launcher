package hostdoctor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/probe"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore/inspection"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service"
)

type DiscoverCodex func(string) (string, error)
type ValidateCodex func(context.Context, string) (string, error)
type RunCommand func(context.Context, string, ...string) (string, error)
type ServiceStatus func(context.Context) (hostinstall.ServiceStatus, error)
type Dial func(string, string, time.Duration) (net.Conn, error)
type InspectState func(context.Context) (inspection.State, error)
type LastError func() (string, error)

type Options struct {
	DiscoverCodex DiscoverCodex
	ValidateCodex ValidateCodex
	RunTailscale  RunCommand
	ServiceStatus ServiceStatus
	Dial          Dial
	InspectState  InspectState
	StatePath     string
	LastError     LastError
	Logger        *slog.Logger
}

type Doctor struct {
	discoverCodex DiscoverCodex
	validateCodex ValidateCodex
	runTailscale  RunCommand
	serviceStatus ServiceStatus
	dial          Dial
	inspectState  InspectState
	lastError     LastError
	logger        *slog.Logger
}

func New(options Options) *Doctor {
	discover := options.DiscoverCodex
	if discover == nil {
		discover = probe.DiscoverBinary
	}
	validate := options.ValidateCodex
	if validate == nil {
		validate = probe.ValidateVersion
	}
	runTailscale := options.RunTailscale
	if runTailscale == nil {
		runTailscale = service.CommandRunner{}.Run
	}
	dial := options.Dial
	if dial == nil {
		dial = net.DialTimeout
	}
	inspect := options.InspectState
	if inspect == nil {
		statePath := options.StatePath
		inspect = func(ctx context.Context) (inspection.State, error) { return inspection.Inspect(ctx, statePath) }
	}
	lastError := options.LastError
	if lastError == nil {
		lastError = func() (string, error) { return "none recorded", nil }
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Doctor{
		discoverCodex: discover, validateCodex: validate, runTailscale: runTailscale,
		serviceStatus: options.ServiceStatus, dial: dial, inspectState: inspect, lastError: lastError, logger: logger,
	}
}

func (doctor *Doctor) Run(ctx context.Context, config companionapp.Config) []cli.Check {
	doctor.logger.Info("[host-doctor] checks started", "input_shape", "saved_config,local_tools,user_service,state", "listen_port", config.ListenPort)
	checks := make([]cli.Check, 0, 7)
	binary, discoverErr := doctor.discoverCodex(config.CodexBinary)
	if discoverErr != nil {
		checks = append(checks, cli.Check{Name: "codex", OK: false, Detail: "Codex binary is unavailable"})
	} else if version, err := doctor.validateCodex(ctx, binary); err != nil {
		checks = append(checks, cli.Check{Name: "codex", OK: false, Detail: "Codex version check failed"})
	} else {
		checks = append(checks, cli.Check{Name: "codex", OK: true, Detail: version})
	}

	if _, err := doctor.runTailscale(ctx, "tailscale", "ip", "--assert="+config.ListenHost); err != nil {
		checks = append(checks, cli.Check{Name: "tailscale", OK: false, Detail: "configured address is not owned by connected Tailscale"})
	} else {
		checks = append(checks, cli.Check{Name: "tailscale", OK: true, Detail: "address owned: " + config.ListenHost})
	}

	serviceState := hostinstall.ServiceStatus{}
	serviceOK := false
	if doctor.serviceStatus == nil {
		checks = append(checks, cli.Check{Name: "service", OK: false, Detail: "user service status is unavailable"})
	} else if status, err := doctor.serviceStatus(ctx); err != nil {
		checks = append(checks, cli.Check{Name: "service", OK: false, Detail: "user service status could not be read"})
	} else {
		serviceState = status
		serviceOK = status.Installed && status.Running
		detail := status.Detail
		if detail == "" {
			detail = "user service is not running"
		}
		checks = append(checks, cli.Check{Name: "service", OK: serviceOK, Detail: detail})
	}

	address := net.JoinHostPort(config.ListenHost, fmt.Sprintf("%d", config.ListenPort))
	if !serviceOK {
		checks = append(checks, cli.Check{Name: "reachability", OK: false, Detail: "companion service is not running"})
	} else if connection, err := doctor.dial("tcp", address, 2*time.Second); err != nil {
		checks = append(checks, cli.Check{Name: "reachability", OK: false, Detail: "companion port is not reachable"})
	} else {
		_ = connection.Close()
		checks = append(checks, cli.Check{Name: "reachability", OK: true, Detail: "companion port accepts connections"})
	}

	state, stateErr := doctor.inspectState(ctx)
	checks = append(checks, schemaCheck(state, stateErr), identityCheck(state, stateErr))
	if lastError, err := doctor.lastError(); err != nil {
		checks = append(checks, cli.Check{Name: "last_error", OK: false, Detail: "last service error is unavailable"})
	} else {
		checks = append(checks, cli.Check{Name: "last_error", OK: true, Detail: lastError})
	}
	failed := 0
	for _, check := range checks {
		if !check.OK {
			failed++
		}
	}
	doctor.logger.Info("[host-doctor] checks completed", "output_shape", "named_safe_checks", "check_count", len(checks), "failed_count", failed, "service_running", serviceState.Running)
	return checks
}

func schemaCheck(state inspection.State, err error) cli.Check {
	if err == nil && state.SchemaCompatible {
		return cli.Check{Name: "schema", OK: true, Detail: "state schema is compatible"}
	}
	if errors.Is(err, inspection.ErrIdentityLoss) && state.SchemaCompatible {
		return cli.Check{Name: "schema", OK: true, Detail: "state schema is compatible"}
	}
	return cli.Check{Name: "schema", OK: false, Detail: "state schema is unavailable or incompatible"}
}

func identityCheck(state inspection.State, err error) cli.Check {
	if errors.Is(err, inspection.ErrIdentityLoss) {
		return cli.Check{Name: "identity", OK: false, Detail: "host identity is missing while paired devices remain"}
	}
	if err != nil {
		return cli.Check{Name: "identity", OK: false, Detail: "host identity could not be inspected"}
	}
	if !state.IdentityPresent {
		return cli.Check{Name: "identity", OK: true, Detail: "not created yet; pairing will create it"}
	}
	return cli.Check{Name: "identity", OK: true, Detail: "pinned fingerprint: " + state.IdentityFingerprint}
}
