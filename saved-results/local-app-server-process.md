# Local Codex App-Server Process Boundary

**Date:** 2026-07-13

## Purpose

This checkpoint fills the missing production boundary between the companion
and a local Codex app-server child. It does not publish app-server to the
network and does not make model calls.

The process startup now:

1. discovers the explicitly configured Codex binary first, otherwise `PATH`;
2. validates its `codex-cli ...` version output before creating a child;
3. starts exactly `codex app-server --stdio`;
4. keeps stderr out of the protocol stream and out of logs;
5. performs the required app-server initialization through the existing strict
   client; and
6. owns shutdown and process reaping on initialization failure, context
   cancellation, or explicit close.

The version check result is logged only as `version_checked=true`. The child-
controlled version text and configured binary path are never written to logs.

The existing adapter set now implements `ListRecent(context.Context, int)`, so
it can be passed directly as the mobile session's typed task source once the
runnable CLI composes the process and platform adapters.

## Current command evidence

The installed ChatGPT-bundled Codex help was checked before implementation:

```text
Usage: codex app-server [OPTIONS] [COMMAND]
--listen <URL> ... stdio:// (default)
--stdio Use stdio as the transport (equivalent to --listen stdio://)
```

The production code uses the explicit `--stdio` form. It does not start a
WebSocket listener or bind raw app-server to Tailscale.

## TDD evidence

The first focused run failed to compile because `Set.ListRecent`, `Options`,
`starter`, and the owned process session did not exist. After the minimum
implementation, the race-enabled focused run passed:

```text
ok companion/internal/codex/taskadapter
ok companion/internal/codex/appserver/process
```

The tests use a real child process made from the Go test binary. They verify the
exact child arguments, initialization exchange, a real `thread/list` request
through the task catalog, pre-spawn version rejection, process reaping after an
invalid initialize response, direct context cancellation, unexpected clean
exit reporting, repeatable close, Windows-safe temporary paths, and logs that
omit child-controlled version text, the configured binary path, and task
content.

Final verification:

```text
go test -race ./companion/...: PASS
go vet ./companion/...: PASS
Windows amd64 process test-binary cross-build: PASS
git diff --check: PASS
```

The independent review first found the raw-version log exposure, missing direct
cancellation coverage, a non-Windows fake home path, and silent clean child
exit. After the fixes above, its fresh race tests, Windows build, vet, and diff
check passed. The final review found no P1, P2, or P3 issues and marked the
checkpoint ready.

## Related-path search

The companion was searched for every `ValidateVersion` call, raw version log,
and app-server process launch. The older read-only probe also starts local
`app-server --stdio`, but it does not log the returned version string and owns a
short-lived probe command rather than the reusable runtime child. No sibling
raw-version log or network app-server launch was found.

## Still pending

- The runnable CLI/main must create this process, choose the hybrid Desktop plus
  app-server adapter on supported macOS/Windows builds or app-server-only on
  Linux, and pass the adapter set into `app.Dependencies.TaskSource`.
- Durable SQLite stores are still pending. The SQLite driver documentation
  lookup remains blocked on the one-time gstack browser setup permission.
- This checkpoint intentionally used a fake local child and made no live model
  call, so no paid account was touched.
