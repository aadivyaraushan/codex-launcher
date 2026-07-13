package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
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
	TaskSource   mobilesession.TaskSource
	TaskEvents   <-chan taskstate.MobileEvent
}

type Runtime struct {
	Config   Config
	Pairing  *pairing.Service
	Projects *projects.Service
	Queue    *promptqueue.Queue
	Journal  *eventjournal.Journal
	Mobile   *transport.Server
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
	journal := eventjournal.New(dependencies.EventStore, logger)
	mobileHandler, err := mobilesession.NewWithTaskSource(ctx, config.ComputerName, projectService, journal, dependencies.TaskSource, logger, time.Now)
	if err != nil {
		logger.Error("[app] mobile session startup failed", "error_class", "mobile_session_initialization")
		return nil, err
	}
	mobileServer, err := transport.NewServer(pairingService, mobileHandler.Handle, logger)
	if err != nil {
		logger.Error("[app] mobile transport startup failed", "error_class", "mobile_transport_initialization")
		return nil, err
	}
	runtime := &Runtime{
		Config:   config,
		Pairing:  pairingService,
		Projects: projectService,
		Queue:    promptqueue.New(dependencies.PromptStore, logger),
		Journal:  journal,
		Mobile:   mobileServer,
	}
	if dependencies.TaskEvents != nil {
		go pumpTaskEvents(ctx, dependencies.TaskEvents, mobileHandler, logger)
	}
	logger.Info("[app] runtime ready", "input_shape", "pairing,projects,queue,journal,mobile_transport", "project_count", len(config.Projects), "listen_port", config.ListenPort)
	return runtime, nil
}
