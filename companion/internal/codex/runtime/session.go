package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	goruntime "runtime"
	"sync"
	"sync/atomic"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver/process"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

const desktopClientType = "codex-launcher-companion"
const taskEventQueueSize = 64

var (
	ErrDesktopUnavailable        = taskadapter.ErrDesktopUnavailable
	ErrInvalidLifecycle          = errors.New("Codex runtime lifecycle dependency is missing")
	ErrUnsupportedPlatform       = errors.New("Codex runtime platform is unsupported")
	ErrWindowsDesktopUnavailable = errors.New("verified Windows Desktop integration is not available yet")
)

type Options struct {
	Binary                string
	ExperimentalQuestions bool
	Logger                *slog.Logger
}

type Session struct {
	tasks                   taskadapter.Set
	app                     appSession
	desktop                 desktopConnector
	logger                  *slog.Logger
	taskEvents              chan taskstate.MobileEvent
	decisionOwner           *decisions.AppServerOwner
	decisionRequests        <-chan appserver.ServerRequest
	desktopDecisionRequests <-chan appserver.ServerRequest
	eventCancel             context.CancelFunc

	closeOnce   sync.Once
	closeResult error
	done        chan struct{}
	watchDone   chan struct{}
	closing     atomic.Bool
}

type appSession interface {
	Client() *appserver.Client
	Done() <-chan struct{}
	Close() error
}

type desktopConnector interface {
	Connect(context.Context) (*desktopipc.Client, error)
	Done() <-chan struct{}
	Close() error
}

type dependencies struct {
	goos           string
	startAppServer func(context.Context, process.Options) (appSession, error)
	newDesktop     func(string, *slog.Logger) desktopConnector
}

func Start(ctx context.Context, options Options) (*Session, error) {
	return startWith(ctx, options, dependencies{
		goos: currentPlatform(),
		startAppServer: func(ctx context.Context, options process.Options) (appSession, error) {
			return process.Start(ctx, options)
		},
		newDesktop: func(clientType string, logger *slog.Logger) desktopConnector {
			return desktopipc.NewDarwinSessionConnector(clientType, logger)
		},
	})
}

func currentPlatform() string {
	return goruntime.GOOS
}

func startWith(ctx context.Context, options Options, deps dependencies) (*Session, error) {
	if ctx == nil || deps.goos == "" || deps.startAppServer == nil {
		return nil, ErrInvalidLifecycle
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	binarySource := "path"
	if options.Binary != "" {
		binarySource = "explicit"
	}
	logger.Info("[codex-runtime] start requested", "platform", deps.goos, "binary_source", binarySource, "input_shape", "owned_app_server,optional_verified_desktop")
	if err := ctx.Err(); err != nil {
		logger.Info("[codex-runtime] start stopped", "platform", deps.goos, "branch_reason", "context_cancelled")
		return nil, err
	}
	if deps.goos == "windows" {
		logger.Warn("[codex-runtime] start rejected", "platform", deps.goos, "branch_reason", "windows_desktop_unavailable")
		return nil, ErrWindowsDesktopUnavailable
	}
	if deps.goos != "darwin" && deps.goos != "linux" {
		logger.Error("[codex-runtime] start rejected", "platform", deps.goos, "branch_reason", "unsupported_platform")
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, deps.goos)
	}
	if deps.goos == "darwin" && deps.newDesktop == nil {
		logger.Error("[codex-runtime] start rejected", "platform", deps.goos, "branch_reason", "desktop_factory_missing")
		return nil, ErrInvalidLifecycle
	}

	app, err := deps.startAppServer(ctx, process.Options{
		Binary: options.Binary, ExperimentalQuestions: options.ExperimentalQuestions, Logger: logger,
	})
	if err != nil {
		logger.Error("[codex-runtime] app-server start failed", "platform", deps.goos, "error_class", fmt.Sprintf("%T", err))
		return nil, fmt.Errorf("start owned Codex app-server: %w", err)
	}
	if app == nil || app.Client() == nil || app.Done() == nil {
		cleanupErr := closeOwners(nil, app)
		return nil, errors.Join(ErrInvalidLifecycle, cleanupErr)
	}

	var desktop desktopConnector
	var desktopClient *desktopipc.Client
	var tasks taskadapter.Set
	adapterMode := "app_server"
	if deps.goos == "darwin" {
		desktop = deps.newDesktop(desktopClientType, logger)
		if desktop == nil {
			cleanupErr := closeOwners(nil, app)
			return nil, errors.Join(ErrInvalidLifecycle, cleanupErr)
		}
		var connectErr error
		desktopClient, connectErr = desktop.Connect(ctx)
		if connectErr != nil {
			logger.Warn("[codex-runtime] Desktop connection unavailable; continuing with owned app-server", "platform", deps.goos, "error_class", fmt.Sprintf("%T", connectErr), "error", connectErr, "decision", "app_server_fallback")
			if closeErr := desktop.Close(); closeErr != nil {
				logger.Warn("[codex-runtime] unavailable Desktop connector cleanup failed", "error_class", fmt.Sprintf("%T", closeErr))
			}
			desktop = nil
			desktopClient = nil
			tasks, err = taskadapter.NewAppServerOnly(app.Client())
		} else {
			tasks, err = taskadapter.New(desktopClient, app.Client())
			adapterMode = "desktop_and_app_server"
		}
	} else {
		tasks, err = taskadapter.NewAppServerOnly(app.Client())
	}
	if err != nil {
		logger.Error("[codex-runtime] adapter setup failed", "platform", deps.goos, "error_class", fmt.Sprintf("%T", err))
		cleanupErr := closeOwners(desktop, app)
		return nil, errors.Join(fmt.Errorf("build Codex task adapters: %w", err), cleanupErr)
	}

	eventContext, eventCancel := context.WithCancel(ctx)
	decisionOwner := decisions.NewAppServerOwner(app.Client())
	var desktopDecisionRequests <-chan appserver.ServerRequest
	if desktopClient != nil {
		decisionOwner.AttachDesktop(desktopClient)
		desktopDecisionRequests = desktopClient.DecisionRequests()
	}
	session := &Session{
		tasks: tasks, app: app, desktop: desktop, logger: logger,
		taskEvents: make(chan taskstate.MobileEvent, taskEventQueueSize), decisionOwner: decisionOwner, decisionRequests: app.Client().Requests(), desktopDecisionRequests: desktopDecisionRequests, eventCancel: eventCancel,
		done: make(chan struct{}), watchDone: make(chan struct{}),
	}
	logger.Info("[codex-runtime] ready", "platform", deps.goos, "adapter_mode", adapterMode, "output_shape", "owned_task_session")
	appEvents := make(chan taskstate.MobileEvent, taskEventQueueSize)
	var desktopEvents <-chan taskstate.MobileEvent
	if desktopClient != nil {
		desktopEvents = desktopClient.TaskEvents()
	}
	go mergeTaskEvents(eventContext, session.taskEvents, logger, appEvents, desktopEvents)
	go func() {
		if err := projectAppServerEvents(eventContext, app.Client().Events(), appEvents, logger); err != nil {
			logger.Error("[codex-runtime] live event projection failed", "error_class", fmt.Sprintf("%T", err), "decision", "close_owned_runtime")
			_ = session.Close()
		}
	}()
	go session.watch(ctx)
	return session, nil
}

func (session *Session) DecisionOwner() *decisions.AppServerOwner {
	if session == nil {
		return nil
	}
	return session.decisionOwner
}

func (session *Session) DecisionRequests() <-chan appserver.ServerRequest {
	if session == nil || session.decisionRequests == nil {
		closed := make(chan appserver.ServerRequest)
		close(closed)
		return closed
	}
	return session.decisionRequests
}

func (session *Session) DesktopDecisionRequests() <-chan appserver.ServerRequest {
	if session == nil || session.desktopDecisionRequests == nil {
		return nil
	}
	return session.desktopDecisionRequests
}

func (session *Session) Tasks() taskadapter.Set {
	if session == nil {
		return taskadapter.Set{}
	}
	return session.tasks
}

func (session *Session) TaskEvents() <-chan taskstate.MobileEvent {
	if session == nil || session.taskEvents == nil {
		closed := make(chan taskstate.MobileEvent)
		close(closed)
		return closed
	}
	return session.taskEvents
}

func (session *Session) Done() <-chan struct{} {
	if session == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return session.done
}

func (session *Session) Close() error {
	if session == nil {
		return nil
	}
	session.closeOnce.Do(func() {
		session.closing.Store(true)
		if session.eventCancel != nil {
			session.eventCancel()
		}
		session.closeResult = closeOwners(session.desktop, session.app)
		if session.closeResult != nil {
			session.logger.Error("[codex-runtime] stop failed", "error_class", fmt.Sprintf("%T", session.closeResult))
		} else {
			session.logger.Info("[codex-runtime] stopped", "decision", "all_owned_sessions_closed")
		}
		close(session.done)
	})
	return session.closeResult
}

func (session *Session) watch(ctx context.Context) {
	defer close(session.watchDone)
	branchReason := ""
	var desktopDone <-chan struct{}
	if session.desktop != nil {
		desktopDone = session.desktop.Done()
	}
	select {
	case <-ctx.Done():
		branchReason = "context_cancelled"
	case <-session.app.Done():
		branchReason = "app_server_stopped"
	case <-desktopDone:
		branchReason = "desktop_stopped"
	case <-session.done:
		return
	}
	if session.closing.Load() {
		return
	}
	if ctx.Err() != nil {
		branchReason = "context_cancelled"
	}
	if branchReason == "context_cancelled" {
		session.logger.Info("[codex-runtime] shutdown requested", "branch_reason", branchReason)
	} else {
		session.logger.Error("[codex-runtime] shutdown requested", "branch_reason", branchReason)
	}
	_ = session.Close()
}

func closeOwners(desktop desktopConnector, app appSession) error {
	var desktopErr error
	if desktop != nil {
		desktopErr = desktop.Close()
	}
	var appErr error
	if app != nil {
		appErr = app.Close()
	}
	return errors.Join(desktopErr, appErr)
}
