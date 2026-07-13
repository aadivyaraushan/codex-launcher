# Four-task Desktop Home following

**Date:** 2026-07-13

## What this is for

The approved Home design shows four task rows above the composer. This
checkpoint makes that limit real at both the Codex adapter and mobile snapshot
boundaries, and starts safe Desktop following for unresolved rows among those
four tasks.

## Result

- Home requests and accepts at most four tasks.
- On macOS, app-server-owned rows stay on app-server. Only unresolved catalog
  rows among the four Home tasks receive a Desktop owner check.
- A fresh Desktop snapshot changes that row to Desktop-owned only after its
  retained state maps safely, matches the requested task ID, and passes a
  separate authorization check. Loading history alone never enables events.
- A failed owner check leaves the row readable as a catalog snapshot, does not
  fail Home sync, and cannot publish Desktop events.
- Every fresh owner check first revokes older live permission and removes that
  task's pending projected state. Permission returns only after the complete
  mapped owner proof succeeds.
- Each Desktop event carries a revocable in-memory authorization token.
  Revocation cancels an event already popped but blocked on delivery. The final
  journal attempt takes the mobile-session publication lock first and then
  holds the token's read lock, so refresh uses the same lock order. Either the
  journal commit or revocation wins in a defined order; a stale event cannot
  reach the phone and refresh cannot deadlock with publication.
- Event deduplication records an update only when the snapshot generation stays
  unchanged throughout publication. A snapshot replaced during publication
  cannot make an older event suppress the next valid update.
- A permanently revoked Desktop event stops after its first rejected journal
  attempt. It is never retried as though the failure were temporary.
- Working, waiting-for-approval, waiting-for-answer, and failed tasks appear
  before replied or interrupted tasks. Ordering remains stable within each
  group.
- External catalog and Desktop-owner errors are logged only by safe error type.
  Raw remote error text is never written to catalog logs.
- Linux uses the same four-row Home limit but skips Desktop owner checks because
  its task set has no Desktop loader.

## Test-first evidence

Before implementation, the focused tests failed with these observed results:

```text
TestFailedFreshOwnerCheckRevokesPreviousMobileAuthorizationAndPendingState:
revoked stream retained queued mobile event: MobileEvent{TaskID:"thread-1", State:"working"}

TestCatalogOwnerFailuresLogOnlySafeMetadata:
logs exposed "private remote owner error"

TestSetListRecentFollowsFourHomeTasksAndOrdersAttentionBeforeRecent:
loaded tasks = []

TestTaskSnapshotRequestsAndEnforcesFourHomeTasks:
five Home tasks were accepted

TestPublishTaskEventRejectsRevokedAuthorizationBeforeJournalCommit:
undefined: ErrTaskEventAuthorizationRevoked

TestPumpTaskEventsDoesNotHoldAuthorizationWhileWaitingForPublisher:
authorization revoke deadlocked behind publisher lock

TestPumpTaskEventsNeverRetriesPermanentlyRevokedDesktopEvent:
revoked Desktop event publish attempts = 2, want 1
```

After implementation, the timing-sensitive tests passed 100 consecutive
race-enabled runs, the five affected packages passed 25 consecutive
race-enabled runs, and the complete 17-package companion suite passed:

```text
go test -race -count=100 ./companion/internal/app -run \
  'TestPumpTaskEvents(NeverRetries|DoesNotDeduplicateAcrossSnapshotReplacement|DoesNotHoldAuthorizationWhileWaitingForPublisher)'

go test -race -count=25 ./companion/internal/app \
  ./companion/internal/app/mobilesession \
  ./companion/internal/codex/desktopipc \
  ./companion/internal/codex/taskadapter \
  ./companion/internal/codex/taskstate

go test -race -count=1 ./companion/...
go vet ./companion/...
git diff --check
```

The independent final judge returned `READY` with no actionable P1, P2, or P3
findings after inspecting the final worktree and the repeated checks. It
specifically rechecked lock order, one-attempt revocation, journal safety,
snapshot-generation deduplication, mapped owner proof, four-task enforcement,
stable ordering, app-server skipping, Linux behavior, cancellation, nil
authorization, and content-free error logs.

## Main files

- `companion/internal/codex/taskadapter/adapter.go`
- `companion/internal/codex/taskadapter/catalog.go`
- `companion/internal/codex/desktopipc/client.go`
- `companion/internal/codex/desktopipc/mobile_events.go`
- `companion/internal/app/mobilesession/handler.go`
- `companion/internal/codex/taskstate/mapper.go`

## Remaining work

This checkpoint supplies safe live Home rows. It does not yet implement the
full task transcript, task controls, approval/question cards, or an older-task
browser. Those remain later Task 9 through Task 11 work.
