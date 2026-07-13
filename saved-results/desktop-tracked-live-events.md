# Verified Desktop task-state events

**Date:** 2026-07-13

## What this is for

This checkpoint lets a successfully loaded ChatGPT Desktop task publish safe
live state changes into the companion's existing mobile event path. It does not
send prompts, replies, commands, file contents, task titles, or paths.

## Result

- A Desktop stream becomes eligible for phone events only after
  `LoadCompleteHistory` receives a fresh snapshot successfully.
- Calling `Stream` alone does not grant that eligibility. A failed owner load
  also leaves the stream ineligible.
- The six supported task states map to fixed summaries such as `Codex is
  working`, `Needs your approval`, and `Codex replied`.
- Desktop reading never waits for phone delivery. The projection keeps only the
  newest pending state for each task and at most 20 tasks.
- The Codex runtime merges Desktop and app-server events into the one stream
  already consumed by journal and phone delivery.
- Verification reads and queues the current retained Desktop state while
  holding the same lock used by later projections. A newer update therefore
  cannot be replaced by an older copied state during authorization.

## Test-first evidence

The first ownership tests failed against the earlier implementation:

```text
TestBareTrackedStreamCannotPublishMobileState:
unverified stream published MobileEvent{TaskID:"thread-1", Kind:"activity", State:"working", Summary:"Codex is working"}

TestFailedOwnerLoadCannotAuthorizeMobileEvents:
failed owner load published MobileEvent{TaskID:"thread-1", Kind:"activity", State:"working", Summary:"Codex is working"}
```

The later current-state test failed to compile against the copied-state API:

```text
cannot use stream (variable of type *StreamState) as json.RawMessage value in argument to client.markMobileStreamVerified
```

After implementation, the affected race-enabled suite passed:

```text
go test -race -count=1 ./companion/internal/codex/taskstate ./companion/internal/codex/desktopipc ./companion/internal/codex/runtime ./companion/internal/app/...

ok github.com/codex-launcher/codex-launcher/companion/internal/codex/taskstate
ok github.com/codex-launcher/codex-launcher/companion/internal/codex/desktopipc
ok github.com/codex-launcher/codex-launcher/companion/internal/codex/runtime
ok github.com/codex-launcher/codex-launcher/companion/internal/app
ok github.com/codex-launcher/codex-launcher/companion/internal/app/mobilesession
```

The independent judge ran the three main packages ten times with the race
detector, then reviewed the final current-state tightening separately. The
judge found no material issue and returned `READY`. It specifically checked the
lock order, latest-state ordering, authorization gate, content boundary, queue
bounds, source closure, and cancellation behavior.

## How to recheck

From the implementation worktree:

```bash
go test -race -count=1 ./companion/...
git diff --check
```

Search the complete event path with:

```bash
rg -n 'TaskEvents|mobileVerified|markMobileStreamVerified|mergeTaskEvents|ProjectTaskState' companion --glob '*.go'
```

## Known limit and next decision

This checkpoint covers Desktop histories the companion has already loaded and
verified. The normal recent-task catalog does not yet automatically load every
Desktop history. Before adding that behavior, choose whether the companion
should follow a small fixed number of recent Desktop tasks automatically or
only follow tasks opened from the phone. Loading every listed history without a
limit is deliberately not part of this checkpoint.
