# Codex runtime composition

**Date:** 2026-07-13

**Purpose:** Record the verified owner that combines the local Codex app-server
process with ChatGPT Desktop on supported platforms and shuts every owned
resource down truthfully.

## Result

- macOS starts the owned local `codex app-server --stdio` process, connects the
  version-pinned same-user Desktop socket, and exposes one task adapter set that
  can route both Desktop-owned and app-server-owned tasks.
- Linux starts only the owned app-server and rejects Desktop task routing with
  `ErrDesktopUnavailable`.
- Windows fails with `ErrWindowsDesktopUnavailable` before starting a child.
  This is intentional and truthful until the verified Windows named-pipe
  connector is implemented.
- Startup failures close every owner that was already created. An explicit
  close stops Desktop before app-server, preserves both close errors with
  `errors.Join`, and returns the same result on repeated calls.
- Context cancellation, unexpected app-server exit, and unexpected Desktop
  exit close the whole session. Planned close and cancellation are not logged
  as unexpected owner failures.
- Logs record the platform, adapter choice, safe branch reason, and error type.
  The Windows fail-closed decision is logged before return. Logs do not record
  the configured Codex binary path.

## Test-first evidence

The first runtime test run failed to compile because `startWith`, `Options`,
`dependencies`, and the lifecycle interfaces did not exist. After the first
implementation, two added behavior tests failed before their fixes:

```text
TestStartRejectsAnAlreadyCancelledContextBeforeStartingAProcess:
error = <nil>, started = true

TestPlannedCloseIsNeverReportedAsUnexpectedAppExit:
branch_reason=app_server_stopped
```

A simultaneous cancellation/app-exit test then failed on attempt 1 because the
select chose the app exit. The final watcher checks `ctx.Err()` before choosing
its log branch. The Desktop-exit test initially timed out after one second, and
the connector test failed to compile because `SessionConnector.Done` did not
exist. The final code exposes that stop signal and watches it.

The Windows decision-log test also failed with an empty log before startup
logging was moved ahead of the safe platform rejection.

## Final verification

```text
go test -race ./companion/internal/codex/runtime -count=100
ok .../companion/internal/codex/runtime

go test -race ./companion/internal/codex/runtime ./companion/internal/codex/desktopipc \
  -run 'TestDesktopExitClosesTheOwnedAppServer|TestSessionConnectorDoneTracksTheConnectedClient' -count=20
ok .../companion/internal/codex/runtime
ok .../companion/internal/codex/desktopipc

go test -race ./companion/...
PASS: every companion package

go vet ./companion/...
PASS

GOOS=windows GOARCH=amd64 go test -exec=true ./companion/...
PASS: every companion test package compiled for Windows

git diff --check
PASS
```

## Independent judge

The independent judge defined a lifecycle, routing, platform, privacy,
concurrency, and test-quality rubric before grading the files. It reran the full
race suite, 100 focused lifecycle repetitions, `go vet`, Windows and Linux
runtime test-binary builds, and the diff check. It reported no P1, P2, or P3
findings and marked this checkpoint `READY`.

## Same-kind search

Searched the companion for `process.Start`, `NewDarwinSessionConnector`, every
`Close() error`, and `closeOwners`. The owned Codex child and Desktop connector
are now composed only here. The other close methods are transport wrappers or
test doubles and do not own this two-part session, so they do not need the same
change.

## Remaining work

- Compose this Codex session with durable pairing, prompt, and event stores in
  the runnable CLI/main process.
- Add the verified Windows named-pipe Desktop connector before claiming Windows
  runtime support.
- Add the mobile transcript and action paths above the adapter set.
