package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/attachments"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
	"github.com/codex-launcher/codex-launcher/companion/internal/durablestore"
	"github.com/codex-launcher/codex-launcher/companion/internal/eventjournal"
	"github.com/codex-launcher/codex-launcher/companion/internal/mobileapi/transport"
	"github.com/codex-launcher/codex-launcher/companion/internal/pairing"
	"github.com/codex-launcher/codex-launcher/companion/internal/projects"
	"github.com/codex-launcher/codex-launcher/companion/internal/promptqueue"
)

var ErrMissingDependency = errors.New("companion runtime dependency is missing")

type Dependencies struct {
	PairingStore            pairing.Store
	PromptStore             promptqueue.Store
	EventStore              eventjournal.Store
	Random                  io.Reader
	Logger                  *slog.Logger
	TaskSource              mobilesession.TaskSource
	TaskEvents              <-chan taskstate.MobileEvent
	AttachmentStore         *attachments.Store
	DecisionOwner           *decisions.AppServerOwner
	DecisionRequests        <-chan appserver.ServerRequest
	DesktopDecisionRequests <-chan appserver.ServerRequest
}

type PersistentDependencies struct {
	Random                  io.Reader
	Logger                  *slog.Logger
	TaskSource              mobilesession.TaskSource
	TaskEvents              <-chan taskstate.MobileEvent
	DecisionOwner           *decisions.AppServerOwner
	DecisionRequests        <-chan appserver.ServerRequest
	DesktopDecisionRequests <-chan appserver.ServerRequest
}

type Runtime struct {
	Config      Config
	Pairing     *pairing.Service
	Projects    *projects.Service
	Queue       *promptqueue.Queue
	Journal     *eventjournal.Journal
	Mobile      *transport.Server
	Attachments *attachments.Store
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
	mobileHandler, err := mobilesession.NewWithTaskSourceQueueAndAttachments(ctx, config.ComputerName, projectService, journal, dependencies.TaskSource, promptQueue, dependencies.AttachmentStore, logger, time.Now)
	if err != nil {
		logger.Error("[app] mobile session startup failed", "error_class", "mobile_session_initialization")
		return nil, err
	}
	decisionTasks, decisionTasksReady := dependencies.TaskSource.(decisionTaskReader)
	decisionReady := dependencies.DecisionOwner != nil && dependencies.DecisionRequests != nil && decisionTasksReady
	decisionPartial := dependencies.DecisionOwner != nil || dependencies.DecisionRequests != nil
	if decisionPartial && !decisionReady {
		logger.Error("[app] decision startup failed", "error_class", "decision_dependency")
		return nil, ErrMissingDependency
	}
	if decisionReady {
		router := decisions.NewRouter(dependencies.DecisionOwner, logger)
		mobileHandler.EnableDecisions(router)
		go pumpDecisionRequests(ctx, dependencies.DecisionRequests, dependencies.DecisionOwner, router, decisionTasks, mobileHandler, config.ComputerName, logger, time.Now, false)
		if dependencies.DesktopDecisionRequests != nil {
			go pumpDecisionRequests(ctx, dependencies.DesktopDecisionRequests, dependencies.DecisionOwner, router, decisionTasks, mobileHandler, config.ComputerName, logger, time.Now, true)
		}
	}
	var mobileServer *transport.Server
	if dependencies.AttachmentStore != nil {
		mobileServer, err = transport.NewServerWithAttachments(pairingService, mobileHandler.Handle, dependencies.AttachmentStore, mobileHandler.PublishAttachmentAck, logger)
	} else {
		mobileServer, err = transport.NewServer(pairingService, mobileHandler.Handle, logger)
	}
	if err != nil {
		logger.Error("[app] mobile transport startup failed", "error_class", "mobile_transport_initialization")
		return nil, err
	}
	runtime := &Runtime{
		Config:      config,
		Pairing:     pairingService,
		Projects:    projectService,
		Queue:       promptQueue,
		Journal:     journal,
		Mobile:      mobileServer,
		Attachments: dependencies.AttachmentStore,
	}
	if dependencies.TaskEvents != nil {
		go pumpTaskEvents(ctx, dependencies.TaskEvents, mobileHandler, logger)
	}
	logger.Info("[app] runtime ready", "input_shape", "pairing,projects,queue,journal,mobile_transport", "project_count", len(config.Projects), "connection_mode", config.ConnectionMode())
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
	attachmentStore, err := attachments.Open(filepath.Join(filepath.Dir(statePath), "attachments"), attachments.DefaultLimits(), logger)
	if err != nil {
		_ = store.Close()
		logger.Error("[app-storage] attachment state open failed", "error_class", "attachment_initialization")
		return nil, nil, err
	}
	runtime, err := NewRuntime(ctx, config, Dependencies{
		PairingStore: store, PromptStore: store, EventStore: store, Random: dependencies.Random, Logger: logger,
		TaskSource: dependencies.TaskSource, TaskEvents: dependencies.TaskEvents, AttachmentStore: attachmentStore,
		DecisionOwner: dependencies.DecisionOwner, DecisionRequests: dependencies.DecisionRequests, DesktopDecisionRequests: dependencies.DesktopDecisionRequests,
	})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	logger.Info("[app-storage] durable state ready", "output_shape", "pairing,prompt,event_stores")
	return runtime, store, nil
}
