package cli

import (
	"context"
	"io"
	"log/slog"
	"time"

	companionapp "github.com/codex-launcher/codex-launcher/companion/internal/app"
	"github.com/codex-launcher/codex-launcher/companion/internal/hostinstall"
)

const Version = "0.1.0-alpha.1"

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type Doctor func(context.Context, companionapp.Config) []Check
type Setup func(context.Context, companionapp.Config) error

type Installer interface {
	Install(context.Context) error
	Replace(context.Context, string) error
	Rollback(context.Context) error
	Uninstall(context.Context) error
	Status(context.Context) (hostinstall.ServiceStatus, error)
}

type Options struct {
	Runtime     *companionapp.Runtime
	Config      *companionapp.Config
	Output      io.Writer
	ErrorOutput io.Writer
	Logger      *slog.Logger
	Now         func() time.Time
	Doctor      Doctor
	Setup       Setup
	Installer   Installer
}

type CLI struct {
	runtime     *companionapp.Runtime
	config      *companionapp.Config
	output      io.Writer
	errorOutput io.Writer
	logger      *slog.Logger
	now         func() time.Time
	doctor      Doctor
	setup       Setup
	installer   Installer
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
	config := options.Config
	if options.Runtime != nil {
		runtimeConfig := options.Runtime.Config
		config = &runtimeConfig
	}
	return &CLI{
		runtime: options.Runtime, config: config, output: output, errorOutput: errorOutput, logger: logger, now: now,
		doctor: options.Doctor, setup: options.Setup, installer: options.Installer,
	}
}

func (command *CLI) Run(ctx context.Context, args []string) int {
	if len(args) == 1 && args[0] == "version" {
		return command.version()
	}
	if len(args) == 0 {
		return command.usage()
	}
	name := args[0]
	if !knownCommand(name) || !validArgumentShape(args) {
		return command.usage()
	}
	command.logger.Info("[cli] command started", "command", safeCommandName(name), "input_shape", commandInputShape(name, len(args)-1))
	if command.runtime == nil {
		switch name {
		case "pair", "devices", "status", "revoke":
			command.writeError("Companion setup is incomplete.\n")
			return 1
		case "doctor":
			if command.config == nil {
				command.writeError("Companion setup is incomplete.\n")
				return 1
			}
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
		if len(args) != 2 {
			exitCode = command.usage()
		} else {
			exitCode = command.revoke(ctx, args[1])
		}
	case "setup":
		exitCode = command.runSetup(ctx, args[1:])
	case "install":
		exitCode = command.install(ctx, args[1:])
	case "rollback":
		exitCode = command.rollback(ctx, args[1:])
	case "uninstall":
		exitCode = command.uninstall(ctx, args[1:])
	default:
		exitCode = command.usage()
	}
	command.logger.Info("[cli] command finished", "command", safeCommandName(name), "exit_code", exitCode)
	return exitCode
}

func knownCommand(name string) bool {
	switch name {
	case "pair", "devices", "status", "doctor", "revoke", "version", "setup", "install", "rollback", "uninstall":
		return true
	default:
		return false
	}
}

func validArgumentShape(args []string) bool {
	switch args[0] {
	case "pair", "devices", "status", "doctor", "rollback", "uninstall":
		return len(args) == 1
	case "revoke":
		return len(args) == 2
	case "install":
		return len(args) == 1 || len(args) == 3
	case "setup":
		return len(args) > 1
	default:
		return false
	}
}

func safeCommandName(name string) string {
	switch name {
	case "pair", "devices", "status", "doctor", "revoke", "version", "setup", "install", "rollback", "uninstall":
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
