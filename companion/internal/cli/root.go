package cli

import (
	"context"
	"io"
	"log/slog"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
)

const Version = "0.1.0-alpha.1"

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type Doctor func(context.Context, companionapp.Config) []Check

type Options struct {
	Runtime     *companionapp.Runtime
	Output      io.Writer
	ErrorOutput io.Writer
	Logger      *slog.Logger
	Now         func() time.Time
	Doctor      Doctor
}

type CLI struct {
	runtime     *companionapp.Runtime
	output      io.Writer
	errorOutput io.Writer
	logger      *slog.Logger
	now         func() time.Time
	doctor      Doctor
}

func New(options Options) *CLI {
	output := options.Output
	if output == nil {
		output = io.Discard
	}
	errorOutput := options.ErrorOutput
	if errorOutput == nil {
		errorOutput = output
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &CLI{runtime: options.Runtime, output: output, errorOutput: errorOutput, logger: logger, now: now, doctor: options.Doctor}
}

func (command *CLI) Run(ctx context.Context, args []string) int {
	if len(args) == 1 && args[0] == "version" {
		return command.version()
	}
	if len(args) == 0 {
		return command.usage()
	}
	if !validInvocation(args) {
		return command.usage()
	}
	name := args[0]
	command.logger.Info("[cli] command started", "command", safeCommandName(name), "input_shape", commandInputShape(name, len(args)-1))
	if command.runtime == nil {
		switch name {
		case "pair", "devices", "status", "doctor", "revoke":
			command.writeError("Companion setup is incomplete.\n")
			return 1
		}
	}
	var exitCode int
	switch name {
	case "pair":
		exitCode = command.pair(ctx)
	case "devices":
		exitCode = command.devices(ctx)
	case "status":
		exitCode = command.status(ctx)
	case "doctor":
		exitCode = command.runDoctor(ctx)
	case "revoke":
		exitCode = command.revoke(ctx, args[1])
	default:
		exitCode = command.usage()
	}
	command.logger.Info("[cli] command finished", "command", safeCommandName(name), "exit_code", exitCode)
	return exitCode
}

func validInvocation(args []string) bool {
	if len(args) == 1 {
		switch args[0] {
		case "pair", "devices", "status", "doctor", "version":
			return true
		}
	}
	return len(args) == 2 && args[0] == "revoke"
}

func safeCommandName(name string) string {
	switch name {
	case "pair", "devices", "status", "doctor", "revoke", "version":
		return name
	default:
		return "unknown"
	}
}

func commandInputShape(name string, argumentCount int) string {
	if name == "revoke" {
		return "device_id_count=" + singleCount(argumentCount)
	}
	return "argument_count=" + singleCount(argumentCount)
}

func singleCount(count int) string {
	if count == 0 {
		return "0"
	}
	return "1_or_more"
}
