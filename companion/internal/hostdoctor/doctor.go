package hostdoctor

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/probe"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore/inspection"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
)

type DiscoverCodex func(string) (string, error)
type ValidateCodex func(context.Context, string) (string, error)
type ServiceStatus func(context.Context) (hostinstall.ServiceStatus, error)
type Dial func(string, string, time.Duration) (net.Conn, error)

// DialRelay opens one pinned connection to the relay box's Mac door. Doctor
// sends CHECK (never REGISTER), which verifies the saved secret without taking
// the live service's single control slot.
type DialRelay func(ctx context.Context, addr string, pinnedPublicKey []byte) (net.Conn, error)

type InspectState func(context.Context) (inspection.State, error)
type LastError func() (string, error)

type Options struct {
	DiscoverCodex DiscoverCodex
	ValidateCodex ValidateCodex
	DialRelay     DialRelay
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
	dialRelay     DialRelay
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
	dialRelay := options.DialRelay
	if dialRelay == nil {
		dialRelay = func(ctx context.Context, addr string, pinnedPublicKey []byte) (net.Conn, error) {
			return relayclient.Dial(ctx, addr, pinnedPublicKey)
		}
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
		discoverCodex: discover, validateCodex: validate, dialRelay: dialRelay,
		serviceStatus: options.ServiceStatus, dial: dial, inspectState: inspect, lastError: lastError, logger: logger,
	}
}

func (doctor *Doctor) Run(ctx context.Context, config companionapp.Config) []cli.Check {
	doctor.logger.Info("[host-doctor] checks started", "input_shape", "saved_config,local_tools,user_service,state",
		"relay_box_host", config.Relay.BoxHost, "relay_mac_port", config.Relay.MacPort)
	checks := make([]cli.Check, 0, 7)
	binary, discoverErr := doctor.discoverCodex(config.CodexBinary)
	if discoverErr != nil {
		checks = append(checks, cli.Check{Name: "codex", OK: false, Detail: "Codex binary is unavailable"})
	} else if version, err := doctor.validateCodex(ctx, binary); err != nil {
		checks = append(checks, cli.Check{Name: "codex", OK: false, Detail: "Codex version check failed"})
	} else {
		checks = append(checks, cli.Check{Name: "codex", OK: true, Detail: version})
	}

	checks = append(checks, doctor.relayBoxCheck(ctx, config.Relay))

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

	// The reachability check dials the box's PHONE door, not the Mac door:
	// it is asking "could a phone reach us through the box right now",
	// which only means anything once the service itself is running.
	address := net.JoinHostPort(config.Relay.BoxHost, fmt.Sprintf("%d", config.Relay.PhonePort))
	if !serviceOK {
		checks = append(checks, cli.Check{Name: "reachability", OK: false, Detail: "companion service is not running"})
	} else if connection, err := doctor.dial("tcp", address, 2*time.Second); err != nil {
		checks = append(checks, cli.Check{Name: "reachability", OK: false, Detail: "relay box phone door is not reachable"})
	} else {
		_ = connection.Close()
		checks = append(checks, cli.Check{Name: "reachability", OK: true, Detail: "relay box phone door accepts connections"})
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

// relayBoxCheck proves the relay is reachable and both saved credentials match.
// CHECK is deliberately distinct from REGISTER, so doctor cannot evict the
// service's live control line while testing the secret.
func (doctor *Doctor) relayBoxCheck(ctx context.Context, relay companionapp.RelayConfig) cli.Check {
	pinnedKey, err := base64.StdEncoding.DecodeString(relay.PinnedKey)
	if err != nil || len(pinnedKey) == 0 {
		return cli.Check{Name: "relay-box", OK: false, Detail: "configured pinned key is invalid"}
	}
	addr := net.JoinHostPort(relay.BoxHost, fmt.Sprintf("%d", relay.MacPort))
	connection, err := doctor.dialRelay(ctx, addr, pinnedKey)
	if err != nil {
		return cli.Check{Name: "relay-box", OK: false, Detail: relayDialFailureDetail(err)}
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetReadDeadline(deadline)
	} else {
		_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	}
	if _, err := connection.Write([]byte("CHECK " + relay.Secret + "\n")); err != nil {
		return cli.Check{Name: "relay-box", OK: false, Detail: "box reachable, but registration secret check failed"}
	}
	reply := make([]byte, len("OK\n"))
	if _, err := io.ReadFull(connection, reply); err != nil || string(reply) != "OK\n" {
		return cli.Check{Name: "relay-box", OK: false, Detail: "box reachable, but registration secret does not match"}
	}
	return cli.Check{Name: "relay-box", OK: true, Detail: "box reachable, pinned key and registration secret match"}
}

// relayDialFailureDetail distinguishes "never reached the box" from
// "reached it but the handshake failed" (almost always a pin mismatch)
// using relayclient.Dial's two distinct wrapped-error prefixes. This is a
// best-effort label for the operator, not a typed classification —
// relayclient does not export a sentinel for pin mismatch specifically.
func relayDialFailureDetail(err error) string {
	switch {
	case strings.Contains(err.Error(), "pinned handshake"):
		return "box reachable, but pinned key does not match"
	case strings.Contains(err.Error(), "dial box"):
		return "relay box is unreachable"
	default:
		return "relay box check failed"
	}
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
