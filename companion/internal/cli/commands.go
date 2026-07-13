package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
)

type statusOutput struct {
	Computer      string `json:"computer"`
	State         string `json:"state"`
	PairedDevices int    `json:"pairedDevices"`
	Projects      int    `json:"projects"`
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
		checks = command.doctor(ctx, command.runtime.Config)
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

func (command *CLI) usage() int {
	command.writeError("Usage: codex-launcher <pair|devices|revoke DEVICE_ID|status|doctor|version>\n")
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
