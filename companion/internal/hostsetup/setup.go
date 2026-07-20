package hostsetup

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/probe"
	"github.com/codex-launcher/codex-launcher/companion/internal/relayclient"
)

var (
	ErrInvalidSetup          = errors.New("companion setup values are invalid")
	ErrCodexUnavailable      = errors.New("Codex is missing or broken")
	ErrRelayUnavailable      = errors.New("the relay box is unreachable, or its pinned key does not match")
	ErrRelayRegisterRejected = errors.New("the relay box rejected the registration secret")
	ErrTailscaleUnavailable  = errors.New("Tailscale is unavailable, signed out, or has no unique owned address")
	ErrConfigWrite           = errors.New("companion setup could not be saved")
)

// defaultRegisterReadTimeout is how long Save waits, after sending
// REGISTER, before deciding the box accepted it. The box's Mac door
// protocol (see planning/relay-box-build-plan.md) sends no ack on success —
// it just keeps the connection open — and only writes/closes on a rejected
// secret. So "we read nothing for this long" is itself the success signal,
// and this timeout only needs to be long enough that a slow-but-real reject
// isn't mistaken for acceptance.
const defaultRegisterReadTimeout = 500 * time.Millisecond

type DiscoverCodex func(string) (string, error)
type ValidateCodex func(context.Context, string) (string, error)

// DialRelay opens one pinned connection to the relay box's Mac door. The
// production default adapts relayclient.Dial; tests inject a fake to
// deterministically simulate an accepted registration, a rejected one, or
// an unreachable box, without a real relay box.
type DialRelay func(ctx context.Context, addr string, pinnedPublicKey []byte) (net.Conn, error)

type WriteConfig func(companionapp.Config) error
type TailscaleIPs func(context.Context, string) (string, error)

type Options struct {
	DiscoverCodex DiscoverCodex
	ValidateCodex ValidateCodex
	DialRelay     DialRelay
	WriteConfig   WriteConfig
	TailscaleIPs  TailscaleIPs
	Logger        *slog.Logger
	// RegisterReadTimeout overrides defaultRegisterReadTimeout. Tests use
	// this to keep the registration check fast instead of waiting out the
	// production timeout on every run.
	RegisterReadTimeout time.Duration
}

type Setup struct {
	discoverCodex       DiscoverCodex
	validateCodex       ValidateCodex
	dialRelay           DialRelay
	writeConfig         WriteConfig
	tailscaleIPs        TailscaleIPs
	logger              *slog.Logger
	registerReadTimeout time.Duration
}

func New(options Options) *Setup {
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
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	registerReadTimeout := options.RegisterReadTimeout
	if registerReadTimeout <= 0 {
		registerReadTimeout = defaultRegisterReadTimeout
	}
	tailscaleIPs := options.TailscaleIPs
	if tailscaleIPs == nil {
		tailscaleIPs = func(ctx context.Context, family string) (string, error) {
			output, err := exec.CommandContext(ctx, "tailscale", "ip", family).Output()
			return string(output), err
		}
	}
	return &Setup{
		discoverCodex: discover, validateCodex: validate, dialRelay: dialRelay,
		writeConfig: options.WriteConfig, tailscaleIPs: tailscaleIPs, logger: logger, registerReadTimeout: registerReadTimeout,
	}
}

// DiscoverTailscaleAddress chooses a canonical local Tailscale literal from
// the documented `tailscale ip -4` output, falling back to `-6` only when no
// IPv4 address exists. An override must be one of those same local addresses.
func (setup *Setup) DiscoverTailscaleAddress(ctx context.Context, override string) (string, error) {
	addresses, err := setup.tailscaleAddressList(ctx, "-4")
	if err != nil {
		return "", ErrTailscaleUnavailable
	}
	if len(addresses) == 0 {
		addresses, err = setup.tailscaleAddressList(ctx, "-6")
	}
	if err != nil || len(addresses) != 1 {
		return "", ErrTailscaleUnavailable
	}
	if override != "" && override != addresses[0] {
		return "", ErrTailscaleUnavailable
	}
	return addresses[0], nil
}

func (setup *Setup) tailscaleAddressList(ctx context.Context, family string) ([]string, error) {
	output, err := setup.tailscaleIPs(ctx, family)
	if err != nil {
		return nil, err
	}
	var addresses []string
	for _, line := range strings.Split(output, "\n") {
		address := strings.TrimSpace(line)
		if address == "" {
			continue
		}
		if !companionapp.ValidTailscaleHost(address) {
			return nil, ErrTailscaleUnavailable
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

// Save validates a prospective config end to end — Codex is present and
// working, the relay box accepts our registration secret — before ever
// writing it to disk. Setup runs before the service starts, so registering
// here (unlike hostdoctor, which must never register while the service is
// live) is safe: there is no live control line yet for us to evict.
func (setup *Setup) Save(ctx context.Context, config companionapp.Config) error {
	setup.logger.Info("[host-setup] validation started",
		"input_shape", "computer,relay_box,codex_binary,projects",
		"project_count", len(config.Projects),
		"relay_box_host", config.Relay.BoxHost, "relay_mac_port", config.Relay.MacPort,
	)
	if setup.writeConfig == nil || config.Validate() != nil {
		return ErrInvalidSetup
	}
	binary, err := setup.discoverCodex(config.CodexBinary)
	if err != nil {
		setup.logger.Error("[host-setup] Codex discovery failed", "error_class", "codex_discovery")
		return fmt.Errorf("%w: %v", ErrCodexUnavailable, err)
	}
	if _, err := setup.validateCodex(ctx, binary); err != nil {
		setup.logger.Error("[host-setup] Codex version check failed", "error_class", "codex_version")
		return fmt.Errorf("%w: %v", ErrCodexUnavailable, err)
	}
	config.CodexBinary = binary
	if err := setup.registerWithRelay(ctx, config.Relay); err != nil {
		return err
	}
	if err := setup.writeConfig(config); err != nil {
		setup.logger.Error("[host-setup] config write failed", "error_class", "config_write")
		return fmt.Errorf("%w: %v", ErrConfigWrite, err)
	}
	setup.logger.Info("[host-setup] validation completed", "output_shape", "owner_only_config", "project_count", len(config.Projects))
	return nil
}

// registerWithRelay proves the box is reachable, our pin matches, and our
// secret is accepted — by actually sending REGISTER, the same way the real
// service will. The box's protocol gives no explicit "accepted" ack, so
// acceptance is inferred from a short read timing out rather than the
// connection closing; see defaultRegisterReadTimeout above.
func (setup *Setup) registerWithRelay(ctx context.Context, relay companionapp.RelayConfig) error {
	pinnedKey, err := decodePinnedKey(relay.PinnedKey)
	if err != nil {
		setup.logger.Error("[host-setup] relay pinned key is invalid", "error_class", "relay_pin")
		return fmt.Errorf("%w: %v", ErrRelayUnavailable, err)
	}
	addr := net.JoinHostPort(relay.BoxHost, portString(relay.MacPort))
	conn, err := setup.dialRelay(ctx, addr, pinnedKey)
	if err != nil {
		setup.logger.Error("[host-setup] relay box dial failed", "error_class", "relay_dial")
		return fmt.Errorf("%w: %v", ErrRelayUnavailable, err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("REGISTER " + relay.Secret + "\n")); err != nil {
		setup.logger.Error("[host-setup] relay REGISTER write failed", "error_class", "relay_register_write")
		return fmt.Errorf("%w: %v", ErrRelayUnavailable, err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(setup.registerReadTimeout)); err != nil {
		setup.logger.Error("[host-setup] relay read deadline unavailable", "error_class", "relay_deadline")
		return fmt.Errorf("%w: %v", ErrRelayUnavailable, err)
	}
	buffer := make([]byte, 1)
	_, readErr := conn.Read(buffer)
	switch {
	case errors.Is(readErr, os.ErrDeadlineExceeded):
		// No data arrived and the box kept the connection open: the box's
		// protocol accepted the secret. This is the success path.
		setup.logger.Info("[host-setup] relay registration accepted")
		return nil
	case readErr == nil, errors.Is(readErr, io.EOF):
		// The box wrote something (or closed) instead of just sitting
		// there — under the documented protocol that only happens on a
		// rejected secret.
		setup.logger.Error("[host-setup] relay registration rejected", "error_class", "relay_register_rejected")
		return ErrRelayRegisterRejected
	default:
		setup.logger.Error("[host-setup] relay registration check failed", "error_class", "relay_register_read")
		return fmt.Errorf("%w: %v", ErrRelayUnavailable, readErr)
	}
}

func decodePinnedKey(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

func portString(port int) string {
	return fmt.Sprintf("%d", port)
}
