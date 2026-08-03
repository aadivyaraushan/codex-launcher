// Package alerts turns Telemetry's per-adapter rot signal into something a
// human actually sees. verification.Telemetry already folds every real
// production outcome back in and works out, per adapter, whether recent
// runs have fallen short of the ceiling that adapter claims — it sets
// Report.Alert when they have. Until this package existed, nothing ever
// read that field: the signal was computed correctly, on live traffic, and
// then dropped on the floor.
//
// The rule that earns most of this file is the one about not repeating. An
// alert that fires on every sweep for as long as the condition lasts is not
// an alert, it is a log line, and it buries the next genuinely new problem
// under the old one. So a Watcher raises an alert only when an adapter
// *enters* the alerting state, stays quiet while that state persists, and
// raises again if the adapter recovers and later rots a second time —
// because that second rot is a new fact, not a repeat of the first.
//
// The other rule, shared with internal/capability/killswitch, is the
// difference between "we checked and it's fine" and "we could not check".
// One adapter whose report cannot be read must not be recorded as
// recovered, and must not stop the sweep from checking the others.
package alerts

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/verification"
)

// Source is the narrow slice of the registry-plus-telemetry a Watcher
// needs: which adapters exist, and what real traffic has shown about each
// one. The production implementation is the live registry and the live
// execution runner's Telemetry; tests use a stub with no registry involved.
type Source interface {
	AdapterIDs() []string
	Report(adapterID string) (verification.Report, error)
}

// Alert names one adapter that has just entered the alerting state, with
// the counts that justify it.
type Alert struct {
	AdapterID    string
	Observations int
	BelowClaim   int
}

// Watcher periodically sweeps a Source and raises an Alert for every
// adapter that has just started falling short of its declared ceiling.
// Safe for concurrent use: Run sweeps on its own goroutine while something
// else may be reading the Source at the same time.
type Watcher struct {
	src    Source
	logger *slog.Logger

	mu       sync.Mutex
	alerting map[string]bool // adapter id -> currently in the alerting state
}

// New builds a Watcher backed by src. A nil logger gets slog.Default().
func New(src Source, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{src: src, logger: logger, alerting: make(map[string]bool)}
}

// Sweep checks every adapter the Source knows about and returns the alerts
// raised this pass: one for each adapter whose report says Alert is true
// and which was not already remembered as alerting. An adapter that was
// already alerting and still is stays quiet — see the package comment.
//
// One adapter whose report cannot be read is logged and skipped without
// stopping the sweep for the others, and — this is the part that matters —
// without touching its remembered state at all. A failed read is not
// evidence the adapter recovered; leaving the state exactly as it was is
// what stops "could not ask" from being silently read as "nothing is
// wrong".
//
// Every alert raised is also logged at Error level, by adapter id and
// count only, never any user content.
func (w *Watcher) Sweep() []Alert {
	var raised []Alert
	for _, id := range w.src.AdapterIDs() {
		report, err := w.src.Report(id)
		if err != nil {
			w.logger.Error("[capability-alerts] could not read adapter report, leaving its alert state unchanged",
				"adapter_id", id, "error", err.Error())
			continue
		}

		w.mu.Lock()
		wasAlerting := w.alerting[id]
		w.alerting[id] = report.Alert
		w.mu.Unlock()

		if report.Alert && !wasAlerting {
			alert := Alert{AdapterID: id, Observations: report.Observations, BelowClaim: report.BelowClaim}
			raised = append(raised, alert)
			w.logger.Error("[capability-alerts] adapter has fallen below its declared ceiling",
				"adapter_id", alert.AdapterID, "observations", alert.Observations, "below_claim", alert.BelowClaim)
		}
	}
	return raised
}

// Run sweeps on a real timer until ctx is cancelled. This is the loop
// production code starts in the background; a watcher only ever checked
// once at startup would never see an adapter that rots hours or days later.
func (w *Watcher) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	w.run(ctx, func() {
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	})
}

// run is the loop both Run and the tests drive. tick is called once per
// completed sweep and is expected to block until it is time for the next
// one — Run passes a tick that waits on a real timer, tests pass one that
// just signals a channel so a test can pace the loop by hand. run itself
// never sleeps; all the waiting lives in tick.
func (w *Watcher) run(ctx context.Context, tick func()) {
	for ctx.Err() == nil {
		w.Sweep()
		tick()
	}
}
