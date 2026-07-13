package app

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

var ErrMissingDependency = errors.New("companion runtime dependency is missing")

type Dependencies struct {
	PairingStore pairing.Store
	PromptStore  promptqueue.Store
	EventStore   eventjournal.Store
	Random       io.Reader
	Logger       *slog.Logger
}

type Runtime struct {
	Config   Config
	Pairing  *pairing.Service
	Projects *projects.Service
	Queue    *promptqueue.Queue
	Journal  *eventjournal.Journal
}

func NewRuntime(ctx context.Context, config Config, dependencies Dependencies) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if dependencies.PairingStore == nil || dependencies.PromptStore == nil || dependencies.EventStore == nil || dependencies.Random == nil {
		return nil, ErrMissingDependency
	}
	logger := dependencies.Logger
	if logger == nil {
		logger = slog.Default()
	}
	pairingService, err := pairing.NewServiceWithLogger(ctx, dependencies.PairingStore, dependencies.Random, logger)
	if err != nil {
		logger.Error("[app] pairing startup failed", "error_class", "pairing_initialization")
		return nil, err
	}
	projectService, err := projects.NewWithLogger(config.Projects, logger)
	if err != nil {
		logger.Error("[app] project startup failed", "error_class", "project_initialization")
		return nil, ErrInvalidConfig
	}
	runtime := &Runtime{
		Config:   config,
		Pairing:  pairingService,
		Projects: projectService,
		Queue:    promptqueue.New(dependencies.PromptStore, logger),
		Journal:  eventjournal.New(dependencies.EventStore, logger),
	}
	logger.Info("[app] runtime ready", "input_shape", "pairing,projects,queue,journal", "project_count", len(config.Projects), "listen_port", config.ListenPort)
	return runtime, nil
}
