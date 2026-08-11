package phoneruntime

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskadapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate"
)

// TurnSource is what a connected OpenClaw Gateway session looks like to the
// runtime: it can list and read the phone agent's one task, drive turns on
// it, and it owns a connection that eventually needs closing.
// turnproxy.Source is the real implementation; tests substitute a stub.
type TurnSource interface {
	mobilesession.TaskSource
	mobilesession.ExistingTaskSource
	io.Closer
}

// errTurnSourceNotConnected is returned by every deferredTurnSource method
// except ListRecent while no gateway connection has been made yet.
var errTurnSourceNotConnected = errors.New("phone agent gateway not connected yet")

// deferredTurnSource is the TurnSource handed to the mobile session handler
// at Open, before the gateway websocket exists. The handler is built once
// and does not tolerate being rebuilt later, but the connect-retry loop
// needs time (and possibly several attempts) to reach the gateway — this
// lets the handler start immediately against a stand-in that starts
// forwarding once set() is called from the retry loop.
type deferredTurnSource struct {
	mu    sync.Mutex
	inner TurnSource
}

func newDeferredTurnSource() *deferredTurnSource {
	return &deferredTurnSource{}
}

// set installs the real, connected TurnSource. Called at most once, from
// the connect-retry goroutine, the moment it succeeds.
func (deferred *deferredTurnSource) set(source TurnSource) {
	deferred.mu.Lock()
	defer deferred.mu.Unlock()
	deferred.inner = source
}

func (deferred *deferredTurnSource) connected() bool {
	deferred.mu.Lock()
	defer deferred.mu.Unlock()
	return deferred.inner != nil
}

func (deferred *deferredTurnSource) current() TurnSource {
	deferred.mu.Lock()
	defer deferred.mu.Unlock()
	return deferred.inner
}

// ListRecent returns an empty list rather than an error while unconnected,
// so the handler's startup snapshot still builds instead of failing Open.
func (deferred *deferredTurnSource) ListRecent(ctx context.Context, limit int) ([]taskstate.Task, error) {
	inner := deferred.current()
	if inner == nil {
		return []taskstate.Task{}, nil
	}
	return inner.ListRecent(ctx, limit)
}

func (deferred *deferredTurnSource) CurrentTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	inner := deferred.current()
	if inner == nil {
		return taskstate.Task{}, errTurnSourceNotConnected
	}
	return inner.CurrentTask(ctx, taskID)
}

func (deferred *deferredTurnSource) StartExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	inner := deferred.current()
	if inner == nil {
		return taskadapter.ExistingTaskResult{}, errTurnSourceNotConnected
	}
	return inner.StartExistingTurn(ctx, taskID, prompt)
}

func (deferred *deferredTurnSource) RedirectExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	inner := deferred.current()
	if inner == nil {
		return taskadapter.ExistingTaskResult{}, errTurnSourceNotConnected
	}
	return inner.RedirectExistingTurn(ctx, taskID, prompt)
}

func (deferred *deferredTurnSource) InterruptExistingTurn(ctx context.Context, taskID string) (taskadapter.ExistingTaskResult, error) {
	inner := deferred.current()
	if inner == nil {
		return taskadapter.ExistingTaskResult{}, errTurnSourceNotConnected
	}
	return inner.InterruptExistingTurn(ctx, taskID)
}

// Close closes the inner source if one was ever set, and is safe to call
// more than once — Runtime.Close and a second, defensive call both work.
func (deferred *deferredTurnSource) Close() error {
	deferred.mu.Lock()
	inner := deferred.inner
	deferred.inner = nil
	deferred.mu.Unlock()
	if inner == nil {
		return nil
	}
	return inner.Close()
}
