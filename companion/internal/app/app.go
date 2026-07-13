package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore"
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

type PersistentDependencies struct {
	Random     io.Reader
	Logger     *slog.Logger
	TaskSource mobilesession.TaskSource
	TaskEvents <-chan taskstate.MobileEvent
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
	promptQueue := promptqueue.New(dependencies.PromptStore, logger)
	mobileHandler, err := mobilesession.NewWithTaskSourceAndQueue(ctx, config.ComputerName, projectService, journal, dependencies.TaskSource, promptQueue, logger, time.Now)
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
		Queue:    promptQueue,
		Journal:  journal,
		Mobile:   mobileServer,
	}
	if dependencies.TaskEvents != nil {
		go pumpTaskEvents(ctx, dependencies.TaskEvents, mobileHandler, logger)
	}
	logger.Info("[app] runtime ready", "input_shape", "pairing,projects,queue,journal,mobile_transport", "project_count", len(config.Projects), "listen_port", config.ListenPort)
	return runtime, nil
}

func OpenPersistentRuntime(ctx context.Context, config Config, dependencies PersistentDependencies) (*Runtime, io.Closer, error) {
	root, err := ConfigRoot()
	if err != nil {
		return nil, nil, err
	}
	return openPersistentRuntimeAt(ctx, config, dependencies, filepath.Join(root, "state.sqlite3"))
}

func openPersistentRuntimeAt(ctx context.Context, config Config, dependencies PersistentDependencies, statePath string) (*Runtime, io.Closer, error) {
	logger := dependencies.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("[app-storage] opening durable state", "input_shape", "single_sqlite_file", "state_path_source", "config_root")
	store, err := durablestore.Open(ctx, statePath, eventjournal.Limits{MaxEvents: 2048, MaxBytes: 8 * 1024 * 1024})
	if err != nil {
		logger.Error("[app-storage] durable state open failed", "error_class", "sqlite_initialization")
		return nil, nil, err
	}
	runtime, err := NewRuntime(ctx, config, Dependencies{
		PairingStore: store, PromptStore: store, EventStore: store, Random: dependencies.Random, Logger: logger,
		TaskSource: dependencies.TaskSource, TaskEvents: dependencies.TaskEvents,
	})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	logger.Info("[app-storage] durable state ready", "output_shape", "pairing,prompt,event_stores")
	return runtime, store, nil
}
