package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostsetup"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
)

type repeatedStrings []string

func (values *repeatedStrings) String() string { return fmt.Sprint([]string(*values)) }
func (values *repeatedStrings) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type statusOutput struct {
	Computer      string                     `json:"computer"`
	State         string                     `json:"state"`
	PairedDevices int                        `json:"pairedDevices"`
	Projects      int                        `json:"projects"`
	Service       *hostinstall.ServiceStatus `json:"service,omitempty"`
}

func (command *CLI) version() int {
	fmt.Fprintf(command.output, "codex-launcher %s\n", Version)
	return 0
}

func (command *CLI) pair(ctx context.Context) int {
	devices, err := command.runtime.Pairing.Devices(ctx)
	if err != nil {
		command.writeError("Unable to inspect paired devices.\n")
		return 1
	}
	if len(devices) != 0 {
		command.writeError("A phone is already paired. Revoke it before pairing another phone.\n")
		return 1
	}
	offer, err := command.runtime.Pairing.BeginPairing(pairing.PairingTarget{
		Host: command.runtime.Config.ListenHost, Port: command.runtime.Config.ListenPort, Protocol: pairing.ProtocolMajor,
	}, command.now())
	if err != nil {
		command.writeError("Unable to create a pairing code.\n")
		return 1
	}
	fmt.Fprintln(command.output, offer.URI)
	return 0
}

func (command *CLI) devices(ctx context.Context) int {
	devices, err := command.runtime.Pairing.Devices(ctx)
	if err != nil || command.writeJSON(devices) != nil {
		command.writeError("Unable to list paired devices.\n")
		return 1
	}
	return 0
}

func (command *CLI) status(ctx context.Context) int {
	devices, err := command.runtime.Pairing.Devices(ctx)
	if err != nil {
		command.writeError("Unable to read companion status.\n")
		return 1
	}
	status := statusOutput{Computer: command.runtime.Config.ComputerName, State: "configured", PairedDevices: len(devices), Projects: len(command.runtime.Config.Projects)}
	if command.installer != nil {
		serviceStatus, statusErr := command.installer.Status(ctx)
		if statusErr != nil {
			command.writeError("Unable to read companion service status.\n")
			return 1
		}
		status.Service = &serviceStatus
	}
	if command.writeJSON(status) != nil {
		command.writeError("Unable to write companion status.\n")
		return 1
	}
	return 0
}

func (command *CLI) revoke(ctx context.Context, deviceID string) int {
	if err := command.runtime.Pairing.Revoke(ctx, deviceID); err != nil {
		command.writeError("Unable to revoke that device.\n")
		return 1
	}
	_, _ = fmt.Fprint(command.output, "Device revoked\n")
	return 0
}

func (command *CLI) runDoctor(ctx context.Context) int {
	checks := []Check{{Name: "runtime", OK: false, Detail: "doctor checks are not wired"}}
	if command.doctor != nil {
		checks = command.doctor(ctx, *command.config)
	}
	if command.writeJSON(checks) != nil {
		command.writeError("Unable to write doctor results.\n")
		return 1
	}
	for _, check := range checks {
		if !check.OK {
			return 1
		}
	}
	return 0
}

func (command *CLI) runSetup(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var projectIDs, projectNames, projectPaths repeatedStrings
	computerName := flags.String("computer-name", "", "")
	listenHost := flags.String("listen-host", "", "")
	listenPort := flags.Int("listen-port", 9443, "")
	codexBinary := flags.String("codex-binary", "", "")
	flags.Var(&projectIDs, "project-id", "")
	flags.Var(&projectNames, "project-name", "")
	flags.Var(&projectPaths, "project-path", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *computerName == "" || *listenHost == "" || *codexBinary == "" || len(projectIDs) == 0 || len(projectIDs) != len(projectNames) || len(projectIDs) != len(projectPaths) {
		return command.usage()
	}
	configuredProjects := make([]projects.Config, len(projectIDs))
	for index := range projectIDs {
		configuredProjects[index] = projects.Config{ID: projectIDs[index], DisplayName: projectNames[index], Path: projectPaths[index]}
	}
	config := companionapp.Config{
		Version: 1, ComputerName: *computerName, ListenHost: *listenHost, ListenPort: *listenPort,
		CodexBinary: *codexBinary, Projects: configuredProjects,
	}
	if command.setup == nil {
		command.writeError("Companion setup is unavailable.\n")
		return 1
	}
	if err := config.Validate(); err != nil {
		command.writeError("Companion setup values are invalid.\n")
		return 1
	}
	if err := command.setup(ctx, config); err != nil {
		command.logger.Error("[cli] setup failed", "error_class", "setup")
		switch {
		case errors.Is(err, hostsetup.ErrCodexUnavailable):
			command.writeError("Codex is missing or broken. Check --codex-binary.\n")
		case errors.Is(err, hostsetup.ErrTailscaleUnavailable):
			command.writeError("Tailscale is missing, disconnected, or does not own that address.\n")
		case errors.Is(err, hostsetup.ErrPortUnavailable):
			command.writeError("That Tailscale address and port are already in use.\n")
		default:
			command.writeError("Companion setup could not be saved.\n")
		}
		return 1
	}
	_, _ = fmt.Fprint(command.output, "Companion setup saved. Run codex-launcher install next.\n")
	return 0
}

func (command *CLI) install(ctx context.Context, args []string) int {
	if command.installer == nil {
		command.writeError("Companion installer is unavailable.\n")
		return 1
	}
	var err error
	switch {
	case len(args) == 0:
		err = command.installer.Install(ctx)
	case len(args) == 2 && args[0] == "--replace" && args[1] != "":
		err = command.installer.Replace(ctx, args[1])
	default:
		return command.usage()
	}
	if err != nil {
		if errors.Is(err, hostinstall.ErrMaintenanceScheduled) {
			_, _ = fmt.Fprint(command.output, "Companion maintenance scheduled; it will finish after this command exits.\n")
			return 0
		}
		command.logger.Error("[cli] install operation failed", "error_class", "host_install")
		command.writeError("Companion install operation failed.\n")
		return 1
	}
	_, _ = fmt.Fprint(command.output, "Companion install operation completed.\n")
	return 0
}

func (command *CLI) rollback(ctx context.Context, args []string) int {
	if len(args) != 0 {
		return command.usage()
	}
	if command.installer == nil {
		command.writeError("Companion installer is unavailable.\n")
		return 1
	}
	if err := command.installer.Rollback(ctx); err != nil {
		if errors.Is(err, hostinstall.ErrMaintenanceScheduled) {
			_, _ = fmt.Fprint(command.output, "Companion maintenance scheduled; it will finish after this command exits.\n")
			return 0
		}
		command.logger.Error("[cli] rollback failed", "error_class", "host_install")
		command.writeError("Companion rollback failed.\n")
		return 1
	}
	_, _ = fmt.Fprint(command.output, "Companion rollback completed.\n")
	return 0
}

func (command *CLI) uninstall(ctx context.Context, args []string) int {
	if len(args) != 0 {
		return command.usage()
	}
	if command.installer == nil {
		command.writeError("Companion installer is unavailable.\n")
		return 1
	}
	if err := command.installer.Uninstall(ctx); err != nil {
		if errors.Is(err, hostinstall.ErrMaintenanceScheduled) {
			_, _ = fmt.Fprint(command.output, "Companion maintenance scheduled; it will finish after this command exits.\n")
			return 0
		}
		command.logger.Error("[cli] uninstall failed", "error_class", "host_install")
		command.writeError("Companion uninstall failed.\n")
		return 1
	}
	_, _ = fmt.Fprint(command.output, "Companion uninstalled.\n")
	return 0
}

func (command *CLI) usage() int {
	command.writeError("Usage: codex-launcher <setup FLAGS|install [--replace ARTIFACT]|rollback|uninstall|pair|devices|revoke DEVICE_ID|status|doctor|version>\n")
	return 2
}

func (command *CLI) writeJSON(value any) error {
	encoder := json.NewEncoder(command.output)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(value)
}

func (command *CLI) writeError(message string) {
	_, _ = fmt.Fprint(command.errorOutput, message)
}
