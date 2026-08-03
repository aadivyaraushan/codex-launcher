package killswitch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// The kill switch is how a capability found to be harmful gets switched off
// on phones already in people's hands, without shipping a new build. All of
// it exists — Registry.ApplyKillList is written and tested — and nothing
// calls it outside a demo command. So today the switch is a switch that is
// not wired to anything.
//
// The rule this whole package turns on is the difference between "we asked
// and nothing is killed" and "we could not ask". An empty kill list is a
// real instruction: it means every adapter is allowed back on. So anything
// that turns a failed fetch into an empty list — a 404 page, a truncated
// body, a proxy returning HTML — silently switches every killed adapter
// back on, which is the exact opposite of what a kill switch is for. That
// is the failure this package is written to make impossible.

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// probeAdapter is the smallest thing a registry will accept, so these tests
// are about the switch rather than about any real adapter.
type probeAdapter struct{ id string }

func (p probeAdapter) Describe() manifest.Manifest {
	return manifest.Manifest{
		ID:            p.id,
		Runtime:       manifest.RT2,
		Verbs:         []manifest.Verb{manifest.Write},
		Ceiling:       manifest.Completes,
		Consent:       manifest.ConsentA,
		Auth:          manifest.AuthNone,
		Cost:          manifest.CostFree,
		Capacity:      manifest.Capacity{Kind: manifest.CapacityNone},
		Platform:      manifest.PlatformBoth,
		Gates:         []manifest.Gate{manifest.GateNone},
		ProvesCeiling: p.id + "_smoke",
	}
}

func (probeAdapter) Resolve(context.Context, adapter.Intent) (adapter.Plan, error) {
	return adapter.Plan{}, nil
}
func (probeAdapter) Preview(context.Context, adapter.Plan) (adapter.Preview, error) {
	return adapter.Preview{}, nil
}
func (probeAdapter) Execute(context.Context, adapter.Plan) (adapter.Outcome, error) {
	return adapter.Outcome{Reached: manifest.Completes, Done: true}, nil
}
func (probeAdapter) Revoke(context.Context) error { return nil }

func registryWith(t *testing.T, ids ...string) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, id := range ids {
		if err := reg.Register(probeAdapter{id: id}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	return reg
}

func off(t *testing.T, reg *registry.Registry, id string) bool {
	t.Helper()
	_, disabled := reg.Disabled(id)
	return disabled
}

// stubSource hands back a canned answer, so watcher behaviour can be tested
// without any network at all.
//
// It locks every field. The watcher reads them on its own goroutine while the
// test changes them from the main one, and without the lock that is a genuine
// data race — not a theoretical one, `-race` catches it. Answering it here is
// right: the shared state is the test's, not the watcher's.
type stubSource struct {
	mu    sync.Mutex
	list  registry.KillList
	err   error
	calls int
}

func (s *stubSource) Fetch(context.Context) (registry.KillList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.list, s.err
}

func (s *stubSource) answer(list registry.KillList, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list, s.err = list, err
}

func (s *stubSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// ---- the reading rules ----------------------------------------------------

// The ordinary case, and the reason the package exists.
func TestAnAdapterNamedOnTheListIsSwitchedOff(t *testing.T) {
	reg := registryWith(t, "alpha", "beta")
	src := &stubSource{list: registry.KillList{Entries: []registry.KillEntry{
		{ID: "alpha", Reason: "charges twice"},
	}}}

	if err := New(reg, src, quietLogger()).Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if !off(t, reg, "alpha") {
		t.Error("alpha was named on the kill list and is still on")
	}
	if off(t, reg, "beta") {
		t.Error("beta was not on the list and got switched off anyway")
	}
	if reason, _ := reg.Disabled("alpha"); reason != "charges twice" {
		t.Errorf("alpha is off with reason %q; a user asking why gets no answer", reason)
	}
}

// A list that names nobody is a real instruction: it lifts every kill. This
// is what makes a failed fetch dangerous, so it has to genuinely work.
func TestAnEmptyListSwitchesEverythingBackOn(t *testing.T) {
	reg := registryWith(t, "alpha")
	src := &stubSource{list: registry.KillList{Entries: []registry.KillEntry{{ID: "alpha", Reason: "bad"}}}}
	w := New(reg, src, quietLogger())
	if err := w.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	src.list = registry.KillList{}
	if err := w.Refresh(context.Background()); err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	if off(t, reg, "alpha") {
		t.Error("the list stopped naming alpha and it is still switched off; a kill can never be lifted")
	}
}

// The rule the whole package turns on. A fetch that failed says nothing
// about what should be off, so it must change nothing. Treating it as an
// empty list would switch every killed adapter back on at the exact moment
// we have lost contact with the thing that killed them.
func TestAFailedFetchLeavesEveryKillInPlace(t *testing.T) {
	reg := registryWith(t, "alpha")
	src := &stubSource{list: registry.KillList{Entries: []registry.KillEntry{{ID: "alpha", Reason: "bad"}}}}
	w := New(reg, src, quietLogger())
	if err := w.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	src.err = errors.New("network is gone")
	if err := w.Refresh(context.Background()); err == nil {
		t.Fatal("a fetch that failed was reported as a success")
	}

	if !off(t, reg, "alpha") {
		t.Error("a failed fetch switched a killed adapter back on")
	}
}

// An id on the list this build does not have is normal: the list covers
// every version, and an older phone simply does not have every adapter on
// it. It must not be an error, or one unknown id stops the whole list from
// being applied and the kills that did match never land.
func TestAnIdThisBuildDoesNotHaveIsIgnoredNotFatal(t *testing.T) {
	reg := registryWith(t, "alpha")
	src := &stubSource{list: registry.KillList{Entries: []registry.KillEntry{
		{ID: "from-a-newer-build", Reason: "unknown here"},
		{ID: "alpha", Reason: "charges twice"},
	}}}

	if err := New(reg, src, quietLogger()).Refresh(context.Background()); err != nil {
		t.Fatalf("an unknown id made the whole refresh fail: %v", err)
	}
	if !off(t, reg, "alpha") {
		t.Error("an unknown id earlier in the list stopped a real kill from landing")
	}
}

// ---- reading the list off the network -------------------------------------

func TestTheHTTPSourceReadsARealList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"entries":[{"id":"alpha","reason":"charges twice"}]}`)
	}))
	defer server.Close()

	list, err := NewHTTPSource(server.URL, server.Client(), quietLogger()).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(list.Entries) != 1 || list.Entries[0].ID != "alpha" || list.Entries[0].Reason != "charges twice" {
		t.Fatalf("list came back as %+v", list)
	}
}

// A body that is not the list is not an empty list. A proxy sign-in page, a
// truncated response, an HTML error — every one of those parses to nothing
// under a forgiving reader, and nothing means "switch everything back on".
func TestABodyThatIsNotAListIsAnErrorNotAnEmptyList(t *testing.T) {
	for _, body := range []string{
		"<html><body>Sign in to continue</body></html>",
		`{"entries":`,
		"",
		"null",
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		}))

		_, err := NewHTTPSource(server.URL, server.Client(), quietLogger()).Fetch(context.Background())
		server.Close()

		if err == nil {
			t.Errorf("a body of %q was read as a valid kill list, which means every killed adapter comes back on", body)
		}
	}
}

// Same reasoning one layer up: an error status is not a list. A 404 or a 503
// that happens to carry a body must never be read as an instruction.
func TestAnErrorStatusIsNotAList(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusForbidden} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"entries":[]}`)
		}))

		_, err := NewHTTPSource(server.URL, server.Client(), quietLogger()).Fetch(context.Background())
		server.Close()

		if err == nil {
			t.Errorf("HTTP %d was read as a kill list saying nothing is killed", status)
		}
	}
}

// A list that says nothing is killed, delivered properly, is still a real
// instruction — this is the other side of the two tests above, and it is
// what stops an implementation passing them by rejecting everything.
func TestAProperlyDeliveredEmptyListIsAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"entries":[]}`)
	}))
	defer server.Close()

	list, err := NewHTTPSource(server.URL, server.Client(), quietLogger()).Fetch(context.Background())
	if err != nil {
		t.Fatalf("a real empty list was refused: %v", err)
	}
	if len(list.Entries) != 0 {
		t.Fatalf("expected no entries, got %+v", list.Entries)
	}
}

// ---- keeping it up to date ------------------------------------------------

// A kill switch checked once at startup only helps phones that restart. The
// watcher keeps asking, and stops when the companion does.
func TestTheWatcherKeepsCheckingUntilItIsStopped(t *testing.T) {
	reg := registryWith(t, "alpha")
	src := &stubSource{}
	w := New(reg, src, quietLogger())

	// Unbuffered on purpose. Every send parks the watcher until this test
	// reads it, so the loop runs exactly in step with the test instead of
	// spinning ahead into a buffer. That pacing is what makes the ordering in
	// the next test knowable rather than lucky.
	ctx, stop := context.WithCancel(context.Background())
	checked := make(chan struct{})
	done := make(chan struct{})
	go func() {
		w.run(ctx, func() {
			select {
			case checked <- struct{}{}:
			case <-ctx.Done():
			}
		})
		close(done)
	}()

	for i := 0; i < 3; i++ {
		<-checked
	}
	stop()
	<-done

	if src.callCount() < 3 {
		t.Fatalf("the watcher checked %d times, expected to keep going", src.callCount())
	}
}

// A single failed check must not take the watcher down with it, or one
// network blip means no kill ever lands again until the companion restarts.
func TestOneFailedCheckDoesNotStopTheWatcher(t *testing.T) {
	reg := registryWith(t, "alpha")
	src := &stubSource{err: errors.New("network is gone")}
	w := New(reg, src, quietLogger())

	ctx, stop := context.WithCancel(context.Background())
	checked := make(chan struct{})
	done := make(chan struct{})
	go func() {
		w.run(ctx, func() {
			select {
			case checked <- struct{}{}:
			case <-ctx.Done():
			}
		})
		close(done)
	}()

	// Two failed checks, then the network comes back. With an unbuffered
	// channel the watcher is parked inside tick each time this test reads,
	// so the fourth check is guaranteed to happen after the answer changed:
	// the write finishes before the third read, and the third read is what
	// releases the watcher into its fourth check.
	<-checked
	<-checked
	src.answer(registry.KillList{Entries: []registry.KillEntry{{ID: "alpha", Reason: "charges twice"}}}, nil)
	<-checked
	<-checked
	stop()
	<-done

	if !off(t, reg, "alpha") {
		t.Error("the watcher gave up after a failed check and never applied the kill that followed")
	}
}
