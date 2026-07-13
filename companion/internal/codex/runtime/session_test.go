package runtime

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver/process"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

func TestDarwinSessionCombinesDesktopAndAppServerAdapters(t *testing.T) {
	events := &eventLog{}
	app := newFakeAppSession(events)
	desktop := &fakeDesktopConnector{events: events, client: &desktopipc.Client{}}

	session, err := startWith(context.Background(), Options{Binary: "/private/codex"}, dependencies{
		goos: "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) {
			events.add("app.start")
			return app, nil
		},
		newDesktop: func(string, *slog.Logger) desktopConnector {
			events.add("desktop.new")
			return desktop
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.Tasks().For(taskstate.Task{Source: taskstate.SourceDesktop}); err != nil {
		t.Fatalf("desktop adapter = %v", err)
	}
	if _, err := session.Tasks().For(taskstate.Task{Source: taskstate.SourceAppServer}); err != nil {
		t.Fatalf("app-server adapter = %v", err)
	}
	if got := events.snapshot(); !reflect.DeepEqual(got, []string{"app.start", "desktop.new", "desktop.connect"}) {
		t.Fatalf("startup events = %#v", got)
	}
}

func TestLinuxSessionUsesOnlyTheOwnedAppServer(t *testing.T) {
	app := newFakeAppSession(nil)
	desktopCreated := false
	session, err := startWith(context.Background(), Options{}, dependencies{
		goos:           "linux",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
		newDesktop: func(string, *slog.Logger) desktopConnector {
			desktopCreated = true
			return &fakeDesktopConnector{}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if desktopCreated {
		t.Fatal("Linux created a Desktop connector")
	}
	if _, err := session.Tasks().For(taskstate.Task{Source: taskstate.SourceAppServer}); err != nil {
		t.Fatalf("app-server adapter = %v", err)
	}
	if _, err := session.Tasks().For(taskstate.Task{Source: taskstate.SourceDesktop}); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("desktop adapter error = %v", err)
	}
}

func TestWindowsFailsClosedUntilItsVerifiedDesktopConnectorExists(t *testing.T) {
	started := false
	var logs lockedBuffer
	session, err := startWith(context.Background(), Options{Logger: slog.New(slog.NewTextHandler(&logs, nil))}, dependencies{
		goos: "windows",
		startAppServer: func(context.Context, process.Options) (appSession, error) {
			started = true
			return newFakeAppSession(nil), nil
		},
	})
	if session != nil || !errors.Is(err, ErrWindowsDesktopUnavailable) {
		t.Fatalf("session = %#v, error = %v", session, err)
	}
	if started {
		t.Fatal("Windows started a child before its Desktop connector was available")
	}
	if !strings.Contains(logs.String(), "branch_reason=windows_desktop_unavailable") {
		t.Fatalf("Windows decision log = %s", logs.String())
	}
}

func TestDesktopConnectionFailureCleansUpEveryStartedOwner(t *testing.T) {
	app := newFakeAppSession(nil)
	desktop := &fakeDesktopConnector{connectErr: errors.New("desktop unavailable")}
	session, err := startWith(context.Background(), Options{}, dependencies{
		goos:           "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
		newDesktop:     func(string, *slog.Logger) desktopConnector { return desktop },
	})
	if session != nil || err == nil {
		t.Fatalf("session = %#v, error = %v", session, err)
	}
	if app.closeCount() != 1 || desktop.closeCount != 1 {
		t.Fatalf("cleanup counts: app = %d, desktop = %d", app.closeCount(), desktop.closeCount)
	}
}

func TestCloseStopsDesktopBeforeAppServerAndReturnsEveryErrorOnce(t *testing.T) {
	events := &eventLog{}
	appErr := errors.New("app close failed")
	desktopErr := errors.New("desktop close failed")
	app := newFakeAppSession(events)
	app.closeErr = appErr
	desktop := &fakeDesktopConnector{events: events, client: &desktopipc.Client{}, closeErr: desktopErr}
	session, err := startWith(context.Background(), Options{}, dependencies{
		goos:           "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
		newDesktop:     func(string, *slog.Logger) desktopConnector { return desktop },
	})
	if err != nil {
		t.Fatal(err)
	}

	first := session.Close()
	second := session.Close()
	if !errors.Is(first, desktopErr) || !errors.Is(first, appErr) || !errors.Is(second, desktopErr) || !errors.Is(second, appErr) {
		t.Fatalf("close errors: first = %v, second = %v", first, second)
	}
	if app.closeCount() != 1 || desktop.closeCount != 1 {
		t.Fatalf("close counts: app = %d, desktop = %d", app.closeCount(), desktop.closeCount)
	}
	if got := events.snapshot(); !reflect.DeepEqual(got, []string{"desktop.connect", "desktop.close", "app.close"}) {
		t.Fatalf("lifecycle events = %#v", got)
	}
}

func TestContextCancellationClosesTheWholeSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	app := newFakeAppSession(nil)
	desktop := &fakeDesktopConnector{client: &desktopipc.Client{}}
	session, err := startWith(ctx, Options{}, dependencies{
		goos:           "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
		newDesktop:     func(string, *slog.Logger) desktopConnector { return desktop },
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	waitClosed(t, session.Done())
	if app.closeCount() != 1 || desktop.closeCount != 1 {
		t.Fatalf("close counts: app = %d, desktop = %d", app.closeCount(), desktop.closeCount)
	}
}

func TestContextCancellationIsNeverReportedAsUnexpectedAppExit(t *testing.T) {
	for attempt := 0; attempt < 50; attempt++ {
		var logs lockedBuffer
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		app := newFakeAppSession(nil)
		app.stop()
		session := &Session{
			app: app, logger: slog.New(slog.NewTextHandler(&logs, nil)),
			done: make(chan struct{}), watchDone: make(chan struct{}),
		}
		session.watch(ctx)
		if strings.Contains(logs.String(), "branch_reason=app_server_stopped") {
			t.Fatalf("attempt %d logged cancellation as app-server failure: %s", attempt, logs.String())
		}
	}
}

func TestOwnedAppServerExitClosesTheDesktopSession(t *testing.T) {
	app := newFakeAppSession(nil)
	desktop := &fakeDesktopConnector{client: &desktopipc.Client{}}
	session, err := startWith(context.Background(), Options{}, dependencies{
		goos:           "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
		newDesktop:     func(string, *slog.Logger) desktopConnector { return desktop },
	})
	if err != nil {
		t.Fatal(err)
	}
	app.stop()
	waitClosed(t, session.Done())
	if desktop.closeCount != 1 {
		t.Fatalf("desktop close count = %d", desktop.closeCount)
	}
}

func TestDesktopExitClosesTheOwnedAppServer(t *testing.T) {
	app := newFakeAppSession(nil)
	desktop := &fakeDesktopConnector{client: &desktopipc.Client{}, done: make(chan struct{})}
	session, err := startWith(context.Background(), Options{}, dependencies{
		goos:           "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
		newDesktop:     func(string, *slog.Logger) desktopConnector { return desktop },
	})
	if err != nil {
		t.Fatal(err)
	}
	close(desktop.done)
	waitClosed(t, session.Done())
	if app.closeCount() != 1 || desktop.closeCount != 1 {
		t.Fatalf("close counts: app = %d, desktop = %d", app.closeCount(), desktop.closeCount)
	}
}

func TestStartRejectsInvalidDependenciesBeforeStartingAProcess(t *testing.T) {
	started := false
	_, err := startWith(nil, Options{}, dependencies{
		goos: "darwin",
		startAppServer: func(context.Context, process.Options) (appSession, error) {
			started = true
			return newFakeAppSession(nil), nil
		},
	})
	if err == nil || started {
		t.Fatalf("error = %v, started = %v", err, started)
	}
}

func TestStartRejectsAnAlreadyCancelledContextBeforeStartingAProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := false
	_, err := startWith(ctx, Options{}, dependencies{
		goos: "linux",
		startAppServer: func(context.Context, process.Options) (appSession, error) {
			started = true
			return newFakeAppSession(nil), nil
		},
	})
	if !errors.Is(err, context.Canceled) || started {
		t.Fatalf("error = %v, started = %v", err, started)
	}
}

func TestLifecycleLogsDoNotExposeTheConfiguredBinaryPath(t *testing.T) {
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	app := newFakeAppSession(nil)
	session, err := startWith(context.Background(), Options{Binary: "/Users/private/secret/codex", Logger: logger}, dependencies{
		goos:           "linux",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "/Users/private/secret/codex") {
		t.Fatalf("logs exposed configured path: %s", logs.String())
	}
	if strings.Contains(logs.String(), "branch_reason=app_server_stopped") {
		t.Fatalf("planned close was logged as an app-server failure: %s", logs.String())
	}
	for _, expected := range []string{"[codex-runtime] start requested", "platform=linux", "[codex-runtime] ready", "[codex-runtime] stopped"} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs missing %q: %s", expected, logs.String())
		}
	}
}

func TestPlannedCloseIsNeverReportedAsUnexpectedAppExit(t *testing.T) {
	var logs lockedBuffer
	app := newFakeAppSession(nil)
	app.closeEntered = make(chan struct{})
	app.closeRelease = make(chan struct{})
	session, err := startWith(context.Background(), Options{Logger: slog.New(slog.NewTextHandler(&logs, nil))}, dependencies{
		goos:           "linux",
		startAppServer: func(context.Context, process.Options) (appSession, error) { return app, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	waitClosed(t, app.closeEntered)
	waitClosed(t, session.watchDone)
	close(app.closeRelease)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "branch_reason=app_server_stopped") {
		t.Fatalf("planned close was logged as an app-server failure: %s", logs.String())
	}
}

func TestStartUsesTheCurrentPlatform(t *testing.T) {
	if currentPlatform() != runtime.GOOS {
		t.Fatalf("current platform = %q, want %q", currentPlatform(), runtime.GOOS)
	}
}

type fakeAppSession struct {
	events       *eventLog
	done         chan struct{}
	closeErr     error
	closeEntered chan struct{}
	closeRelease chan struct{}
	once         sync.Once
	mu           sync.Mutex
	closes       int
}

func newFakeAppSession(events *eventLog) *fakeAppSession {
	return &fakeAppSession{events: events, done: make(chan struct{})}
}

func (*fakeAppSession) Client() *appserver.Client     { return &appserver.Client{} }
func (session *fakeAppSession) Done() <-chan struct{} { return session.done }
func (session *fakeAppSession) Close() error {
	session.mu.Lock()
	session.closes++
	session.mu.Unlock()
	if session.events != nil {
		session.events.add("app.close")
	}
	session.stop()
	if session.closeEntered != nil {
		close(session.closeEntered)
	}
	if session.closeRelease != nil {
		<-session.closeRelease
	}
	return session.closeErr
}
func (session *fakeAppSession) stop() { session.once.Do(func() { close(session.done) }) }
func (session *fakeAppSession) closeCount() int {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.closes
}

type fakeDesktopConnector struct {
	events     *eventLog
	client     *desktopipc.Client
	done       chan struct{}
	connectErr error
	closeErr   error
	closeCount int
}

func (connector *fakeDesktopConnector) Connect(context.Context) (*desktopipc.Client, error) {
	if connector.events != nil {
		connector.events.add("desktop.connect")
	}
	return connector.client, connector.connectErr
}
func (connector *fakeDesktopConnector) Done() <-chan struct{} { return connector.done }
func (connector *fakeDesktopConnector) Close() error {
	connector.closeCount++
	if connector.events != nil {
		connector.events.add("desktop.close")
	}
	return connector.closeErr
}

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (log *eventLog) add(event string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.events = append(log.events, event)
}
func (log *eventLog) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.events...)
}

type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.b.Write(data)
}
func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.b.String()
}

func waitClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session did not stop")
	}
}
