package alerts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/verification"
)

// Telemetry already folds every real production outcome back in and works out,
// per adapter, whether the last few runs fell short of what that adapter claims
// it can do. It sets Report.Alert when they have.
//
// Nothing anywhere reads it. The signal that an adapter has quietly rotted is
// computed correctly, on live traffic, and then dropped on the floor. This
// package is the thing that finally looks.
//
// The rule that earns most of this file is the one about not repeating. An
// alert that fires on every sweep for as long as the condition lasts is not an
// alert, it is a log line, and it buries the next genuinely new one. So an
// alert fires when an adapter *enters* the alerting state, and stays quiet
// afterwards — but it must fire again if that adapter recovers and then rots a
// second time, because that is a new fact.

// stubSource stands in for the registry plus its telemetry. It locks its
// fields: Run drives sweeps on its own goroutine while a test changes the
// answers from the main one, which is a real data race, not a theoretical one.
type stubSource struct {
	mu      sync.Mutex
	ids     []string
	reports map[string]verification.Report
	errs    map[string]error
	sweeps  int
}

func newStubSource(ids ...string) *stubSource {
	return &stubSource{
		ids:     ids,
		reports: make(map[string]verification.Report),
		errs:    make(map[string]error),
	}
}

func (s *stubSource) AdapterIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweeps++
	return append([]string(nil), s.ids...)
}

func (s *stubSource) Report(adapterID string) (verification.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.errs[adapterID]; err != nil {
		return verification.Report{}, err
	}
	return s.reports[adapterID], nil
}

// answer sets what the next sweep will see for one adapter.
func (s *stubSource) answer(adapterID string, alert bool, observations, belowClaim int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[adapterID] = verification.Report{
		AdapterID:    adapterID,
		Observations: observations,
		BelowClaim:   belowClaim,
		Alert:        alert,
	}
}

func (s *stubSource) fail(adapterID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs[adapterID] = err
}

func (s *stubSource) sweepCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sweeps
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAnAdapterThatIsFineRaisesNothing(t *testing.T) {
	src := newStubSource("todoist")
	src.answer("todoist", false, 12, 0)

	raised := New(src, quietLogger()).Sweep()

	if len(raised) != 0 {
		t.Fatalf("a healthy adapter must raise nothing, got %d alerts: %+v", len(raised), raised)
	}
}

func TestAnAdapterThatFallsShortRaisesOneAlertNamingIt(t *testing.T) {
	src := newStubSource("todoist", "notion")
	src.answer("todoist", true, 9, 4)
	src.answer("notion", false, 3, 0)

	raised := New(src, quietLogger()).Sweep()

	if len(raised) != 1 {
		t.Fatalf("want exactly one alert, got %d: %+v", len(raised), raised)
	}
	if raised[0].AdapterID != "todoist" {
		t.Fatalf("want the alert to name todoist, got %q", raised[0].AdapterID)
	}
	if raised[0].Observations != 9 || raised[0].BelowClaim != 4 {
		t.Fatalf("an alert must carry the counts that justify it, got observations=%d belowClaim=%d",
			raised[0].Observations, raised[0].BelowClaim)
	}
}

// The one that keeps the signal worth having. Something that shouts every
// fifteen minutes for a week is something people mute.
func TestAnAdapterAlreadyAlertingDoesNotRaiseAgain(t *testing.T) {
	src := newStubSource("todoist")
	src.answer("todoist", true, 9, 4)
	watcher := New(src, quietLogger())

	first := watcher.Sweep()
	second := watcher.Sweep()
	third := watcher.Sweep()

	if len(first) != 1 {
		t.Fatalf("want one alert on the first sweep, got %d", len(first))
	}
	if len(second) != 0 || len(third) != 0 {
		t.Fatalf("a condition that has not changed must stay quiet, got %d then %d", len(second), len(third))
	}
}

// Recovering and rotting again is a new fact, so it gets a new alert. Without
// this, an adapter that fails every other day is reported once, ever.
func TestAnAdapterThatRecoversAndRotsAgainRaisesASecondAlert(t *testing.T) {
	src := newStubSource("todoist")
	watcher := New(src, quietLogger())

	src.answer("todoist", true, 9, 4)
	if got := watcher.Sweep(); len(got) != 1 {
		t.Fatalf("want one alert while it is failing, got %d", len(got))
	}

	src.answer("todoist", false, 12, 4)
	if got := watcher.Sweep(); len(got) != 0 {
		t.Fatalf("recovery is not an alert, got %d", len(got))
	}

	src.answer("todoist", true, 15, 7)
	if got := watcher.Sweep(); len(got) != 1 {
		t.Fatalf("rotting a second time must raise again, got %d", len(got))
	}
}

// One unreadable adapter must not cost us the sweep. Otherwise a single bad id
// means no alert about any other adapter ever lands again.
func TestOneAdapterThatCannotBeReadDoesNotStopTheOthers(t *testing.T) {
	src := newStubSource("broken", "todoist")
	src.fail("broken", errors.New("no such adapter"))
	src.answer("todoist", true, 9, 4)

	raised := New(src, quietLogger()).Sweep()

	if len(raised) != 1 || raised[0].AdapterID != "todoist" {
		t.Fatalf("want the healthy-path alert to survive a broken sibling, got %+v", raised)
	}
}

// An adapter that cannot be read is not an adapter that is fine. Silence here
// would be the same mistake the kill switch exists to avoid: turning "could
// not ask" into "nothing is wrong".
func TestAnAdapterThatCannotBeReadIsNotTreatedAsHealthy(t *testing.T) {
	src := newStubSource("todoist")
	src.answer("todoist", true, 9, 4)
	watcher := New(src, quietLogger())

	if got := watcher.Sweep(); len(got) != 1 {
		t.Fatalf("want one alert first, got %d", len(got))
	}

	// It stops being readable. That must not be recorded as a recovery.
	src.fail("todoist", errors.New("registry closed"))
	watcher.Sweep()

	// Now it is readable again and still failing. Because the unreadable
	// sweep was not a recovery, this is still the same unresolved condition
	// and must stay quiet rather than looking like a fresh problem.
	src.fail("todoist", nil)
	src.answer("todoist", true, 20, 9)
	if got := watcher.Sweep(); len(got) != 0 {
		t.Fatalf("an unreadable sweep must not reset the alert state, got %d alerts", len(got))
	}
}

func TestEveryRegisteredAdapterIsChecked(t *testing.T) {
	src := newStubSource("todoist", "notion", "spotify")
	src.answer("todoist", true, 9, 4)
	src.answer("notion", true, 5, 5)
	src.answer("spotify", true, 7, 3)

	raised := New(src, quietLogger()).Sweep()

	if len(raised) != 3 {
		t.Fatalf("want every adapter checked, got %d alerts: %+v", len(raised), raised)
	}
}

// Run is the loop production starts in the background. It is driven here by a
// tick the test controls, so the test paces it exactly and nothing sleeps.
func TestTheLoopKeepsSweepingUntilItIsStopped(t *testing.T) {
	src := newStubSource("todoist")
	src.answer("todoist", false, 1, 0)
	watcher := New(src, quietLogger())

	ctx, stop := context.WithCancel(context.Background())
	swept := make(chan struct{})
	done := make(chan struct{})
	go func() {
		watcher.run(ctx, func() {
			select {
			case swept <- struct{}{}:
			case <-ctx.Done():
			}
		})
		close(done)
	}()

	for i := 0; i < 3; i++ {
		<-swept
	}
	stop()
	<-done

	if src.sweepCount() < 3 {
		t.Fatalf("want at least 3 sweeps, got %d", src.sweepCount())
	}
}
