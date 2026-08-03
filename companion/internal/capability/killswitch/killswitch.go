// Package killswitch keeps the local registry in sync with a remote list of
// adapters that must be switched off. It is how a capability found to be
// harmful gets turned off on phones already in people's hands, without
// shipping a new build.
//
// The rule that matters most here is the difference between "we asked and
// nothing is killed" and "we could not ask". An empty kill list is a real
// instruction — it means every adapter is allowed back on. So anything that
// turns a failed fetch into an empty list (a proxy sign-in page, a
// truncated body, a 404 that happens to carry a body) would silently switch
// every killed adapter back on at the exact moment contact with the thing
// that killed them was lost. That is the opposite of what a kill switch is
// for, and every rule in this file exists to make it impossible.
package killswitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/registry"
)

// Source is anything that can hand back the current kill list. The
// production implementation is HTTPSource; tests use a stub that returns a
// canned list or a canned error, with no network involved.
type Source interface {
	Fetch(context.Context) (registry.KillList, error)
}

// killListPayload is the wire shape of the kill list: {"entries":
// [{"id":..., "reason":...}, ...]}. It exists only to carry the lowercase
// JSON field names without adding json tags to registry.KillEntry, which
// lives in a different package and has no reason to know about the wire
// format.
type killListPayload struct {
	Entries []struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	} `json:"entries"`
}

// HTTPSource fetches the kill list over HTTP as plain JSON.
type HTTPSource struct {
	url    string
	client *http.Client
	logger *slog.Logger
}

// NewHTTPSource builds an HTTPSource. A nil client gets a default one with
// a timeout, so a remote server that never answers cannot hang the
// companion's startup or its background refresh loop forever.
func NewHTTPSource(url string, client *http.Client, logger *slog.Logger) *HTTPSource {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPSource{url: url, client: client, logger: logger}
}

// Fetch reads the kill list from the configured URL. It is deliberately
// strict: only a 2xx response whose body is a well-formed
// {"entries": [...]} document counts as a list. Everything else — a bad
// status code, an unreadable body, a body that parses to nothing useful —
// is an error, never treated as an empty list. See the package comment for
// why that distinction is the whole point of this file.
func (s *HTTPSource) Fetch(ctx context.Context) (registry.KillList, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return registry.KillList{}, fmt.Errorf("kill list request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return registry.KillList{}, fmt.Errorf("kill list fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not read the body as a list on a non-2xx status. A 404 or a
		// 503 can still carry a body, and that body is not an instruction.
		return registry.KillList{}, fmt.Errorf("kill list fetch: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return registry.KillList{}, fmt.Errorf("kill list read: %w", err)
	}

	// encoding/json accepts the literal "null" as a valid document and
	// leaves the target at its zero value, which looks exactly like a real
	// empty list. That would make an outage indistinguishable from "nothing
	// is killed", so reject it by hand before it ever reaches Unmarshal.
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return registry.KillList{}, fmt.Errorf("kill list fetch: body was JSON null, not a list")
	}

	var payload killListPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// Covers an empty body, truncated JSON, and HTML/text bodies (a
		// sign-in page, an error page) alike: none of them decode, so all
		// of them are errors rather than silently-empty lists.
		return registry.KillList{}, fmt.Errorf("kill list fetch: body was not a valid kill list: %w", err)
	}

	list := registry.KillList{Entries: make([]registry.KillEntry, 0, len(payload.Entries))}
	for _, e := range payload.Entries {
		list.Entries = append(list.Entries, registry.KillEntry{ID: e.ID, Reason: e.Reason})
	}
	return list, nil
}

// registryApplier is the one thing a Watcher needs from a registry: the
// ability to switch adapters on and off to match a kill list. Keeping it
// this narrow means production code can hand the watcher something that
// does not expose the underlying *registry.Registry at all (see
// runtime.Inventory.ApplyKillList), while *registry.Registry itself still
// satisfies it directly, which is what the tests in this package rely on.
type registryApplier interface {
	ApplyKillList(registry.KillList) []string
}

// Watcher periodically fetches the kill list from a Source and applies it
// to a registry.
type Watcher struct {
	reg    registryApplier
	src    Source
	logger *slog.Logger
}

// New builds a Watcher that keeps reg in sync with whatever src returns.
func New(reg registryApplier, src Source, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{reg: reg, src: src, logger: logger}
}

// Refresh does one fetch-and-apply pass. On a fetch error it returns the
// error and changes nothing in the registry — a failed fetch says nothing
// about what should be off, so it must never be read as "nothing is
// killed". On success it applies the list and logs which adapter ids
// changed state, by id and count only, never any user content.
func (w *Watcher) Refresh(ctx context.Context) error {
	list, err := w.src.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("kill list refresh: %w", err)
	}

	changed := w.reg.ApplyKillList(list)
	if len(changed) > 0 {
		w.logger.Info("[killswitch] adapter state changed", "ids", changed, "count", len(changed))
	}
	return nil
}

// Run refreshes on a real timer until ctx is cancelled. This is the loop
// production code starts in the background; a checked-once-at-startup kill
// switch only ever helps phones that happen to restart.
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
// completed check and is expected to block until it is time for the next
// one — Run passes a tick that waits on a real timer, tests pass one that
// just signals a channel so a test can pace the loop by hand. run itself
// never sleeps; all the waiting lives in tick.
func (w *Watcher) run(ctx context.Context, tick func()) {
	for ctx.Err() == nil {
		if err := w.Refresh(ctx); err != nil {
			// A single failed check must not take the loop down with it —
			// otherwise one network blip means no kill ever lands again
			// until the companion restarts.
			w.logger.Warn("[killswitch] scheduled check failed, will try again", "error", err.Error())
		}
		tick()
	}
}
