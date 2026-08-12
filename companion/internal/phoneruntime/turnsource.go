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
	// Done returns a channel that closes when the underlying gateway
	// connection drops, so the runtime can notice and redial.
	Done() <-chan struct{}
	// StartTriggeredTurn starts a turn the Beeper watcher fired on the
	// owner's behalf, stamping preview as the Home task's last message
	// instead of the trigger prompt itself.
	StartTriggeredTurn(ctx context.Context, taskID, prompt, preview string) (taskadapter.ExistingTaskResult, error)
}

// errTurnSourceNotConnected is returned by every deferredTurnSource method
// except ListRecent while no gateway connection has been made yet.
var errTurnSourceNotConnected = errors.New("phone agent gateway not connected yet")

// phoneAgentProjectLabel backfills Task.ProjectLabel when a TurnSource
// implementation leaves it empty (the phone agent's task has no project of
// its own). The mobile snapshot contract requires every task to carry a
// non-empty label (internal/mobileapi/contract/validation.go
// safeDisplayString) or the whole snapshot is rejected, so deferredTurnSource
// — the one place every TurnSource's tasks pass through on their way to the
// handler — guarantees it rather than trusting each implementation to set it.
const phoneAgentProjectLabel = "Phone agent"

// withProjectLabel returns a copy of task with a non-empty ProjectLabel.
func withProjectLabel(task taskstate.Task) taskstate.Task {
	if task.ProjectLabel == "" {
		task.ProjectLabel = phoneAgentProjectLabel
	}
	return task
}

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

// clear removes the current inner TurnSource (if any) and returns it, so the
// caller can Close it once outside the lock. After clear, connected() is
// false again until the next set().
func (deferred *deferredTurnSource) clear() TurnSource {
	deferred.mu.Lock()
	defer deferred.mu.Unlock()
	old := deferred.inner
	deferred.inner = nil
	return old
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
	tasks, err := inner.ListRecent(ctx, limit)
	if err != nil {
		return nil, err
	}
	normalized := make([]taskstate.Task, len(tasks))
	for i, task := range tasks {
		normalized[i] = withProjectLabel(task)
	}
	return normalized, nil
}

func (deferred *deferredTurnSource) CurrentTask(ctx context.Context, taskID string) (taskstate.Task, error) {
	inner := deferred.current()
	if inner == nil {
		return taskstate.Task{}, errTurnSourceNotConnected
	}
	task, err := inner.CurrentTask(ctx, taskID)
	if err != nil {
		return taskstate.Task{}, err
	}
	return withProjectLabel(task), nil
}

func (deferred *deferredTurnSource) StartExistingTurn(ctx context.Context, taskID, prompt string) (taskadapter.ExistingTaskResult, error) {
	inner := deferred.current()
	if inner == nil {
		return taskadapter.ExistingTaskResult{}, errTurnSourceNotConnected
	}
	return inner.StartExistingTurn(ctx, taskID, prompt)
}

func (deferred *deferredTurnSource) StartTriggeredTurn(ctx context.Context, taskID, prompt, preview string) (taskadapter.ExistingTaskResult, error) {
	inner := deferred.current()
	if inner == nil {
		return taskadapter.ExistingTaskResult{}, errTurnSourceNotConnected
	}
	return inner.StartTriggeredTurn(ctx, taskID, prompt, preview)
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
