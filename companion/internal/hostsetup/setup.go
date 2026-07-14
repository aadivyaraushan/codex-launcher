package hostsetup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"syscall"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/probe"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall/service"
)

var (
	ErrInvalidSetup         = errors.New("companion setup values are invalid")
	ErrCodexUnavailable     = errors.New("Codex is missing or broken")
	ErrTailscaleUnavailable = errors.New("Tailscale is missing, disconnected, or does not own the selected address")
	ErrPortUnavailable      = errors.New("the selected Tailscale port is already in use")
	ErrConfigWrite          = errors.New("companion setup could not be saved")
)

type DiscoverCodex func(string) (string, error)
type ValidateCodex func(context.Context, string) (string, error)
type RunCommand func(context.Context, string, ...string) (string, error)
type Listen func(string, string) (net.Listener, error)
type WriteConfig func(companionapp.Config) error

type Options struct {
	DiscoverCodex DiscoverCodex
	ValidateCodex ValidateCodex
	RunTailscale  RunCommand
	Listen        Listen
	WriteConfig   WriteConfig
	Logger        *slog.Logger
}

type Setup struct {
	discoverCodex DiscoverCodex
	validateCodex ValidateCodex
	runTailscale  RunCommand
	listen        Listen
	writeConfig   WriteConfig
	logger        *slog.Logger
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
	runTailscale := options.RunTailscale
	if runTailscale == nil {
		runTailscale = service.CommandRunner{}.Run
	}
	listen := options.Listen
	if listen == nil {
		listen = net.Listen
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Setup{
		discoverCodex: discover, validateCodex: validate, runTailscale: runTailscale,
		listen: listen, writeConfig: options.WriteConfig, logger: logger,
	}
}

func (setup *Setup) Save(ctx context.Context, config companionapp.Config) error {
	setup.logger.Info("[host-setup] validation started", "input_shape", "computer,tailscale_address,port,codex_binary,projects", "project_count", len(config.Projects), "listen_port", config.ListenPort)
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
	if _, err := setup.runTailscale(ctx, "tailscale", "ip", "--assert="+config.ListenHost); err != nil {
		setup.logger.Error("[host-setup] Tailscale address check failed", "error_class", "tailscale_address")
		return fmt.Errorf("%w: %v", ErrTailscaleUnavailable, err)
	}
	listener, err := setup.listen("tcp", net.JoinHostPort(config.ListenHost, fmt.Sprintf("%d", config.ListenPort)))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			setup.logger.Error("[host-setup] port availability check failed", "error_class", "listen_port", "listen_port", config.ListenPort)
			return fmt.Errorf("%w: %v", ErrPortUnavailable, err)
		}
		setup.logger.Error("[host-setup] Tailscale interface readiness check failed", "error_class", "tailscale_bind")
		return fmt.Errorf("%w: %v", ErrTailscaleUnavailable, err)
	}
	if err := listener.Close(); err != nil {
		setup.logger.Error("[host-setup] temporary listener close failed", "error_class", "listen_close")
		return fmt.Errorf("%w: %v", ErrPortUnavailable, err)
	}
	if err := setup.writeConfig(config); err != nil {
		setup.logger.Error("[host-setup] config write failed", "error_class", "config_write")
		return fmt.Errorf("%w: %v", ErrConfigWrite, err)
	}
	setup.logger.Info("[host-setup] validation completed", "output_shape", "owner_only_config", "project_count", len(config.Projects))
	return nil
}
