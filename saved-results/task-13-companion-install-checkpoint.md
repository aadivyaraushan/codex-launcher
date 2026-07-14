# Task 13 companion install checkpoint

Date: 2026-07-14

## What this checkpoint proves

The companion now has tested commands for first-time setup, install, local
replacement, one-step rollback, uninstall, status, doctor, and version. Setup
checks the explicit Codex binary, asks the installed Tailscale client to assert
ownership of the selected IP, and checks that the selected address and port can
be bound before it writes config. Rerunning setup atomically updates the
approved project folders. If the service is running, it is stopped first; a
failed update or failed restart first stops any process that may have started
with the new config, then restores the prior config and service. A failure to
stop that process retains the transaction for later repair.

The per-user service implementations are:

- macOS: LaunchAgent in `~/Library/LaunchAgents`, loaded in `gui/<uid>`, with
  `RunAtLoad`, `KeepAlive`, an explicit `HOME`, and owner-only files.
- Linux: systemd user unit in the user's config directory, enabled for the
  default user target.
- Windows: current-user Task Scheduler entry with `ONLOGON` and `LIMITED`.

Windows config and service-marker files now use a protected current-user ACL.
When replace, rollback, or uninstall is invoked from the installed Windows
executable, the command schedules a helper outside the install folder, waits
for the invoking process to exit, records a fixed-code result receipt, and
schedules deletion of the helper itself.

Replacement accepts only a local regular file with matching `.sha256` and
`.provenance.json` files. It stops the service, validates the staged binary,
keeps one previous binary plus the matching config, and starts the replacement.
Failed replacement or failed rollback activation restores the last working
binary and config. An owner-only process lock rejects concurrent mutations.
Before the first service or binary change, the manager writes an owner-only
`prepared` transaction record and a separate private snapshot of the active
binary and config. Recovery copies from that snapshot rather than consuming it,
so a failed recovery can be retried. `activated` and `healthy` phases let the
next process either restore the last working version or finish the already
healthy operation without restarting it. This metadata proves artifact
consistency; it is not a publisher signature.

Install, replacement, rollback, and reconfiguration also prepare a random start
attempt ID. Success is reported only after the exact launched service writes a
matching `running` health record; stale health from a prior process is rejected.

Doctor reports the Codex version, Tailscale address ownership, service state,
TCP reachability, schema compatibility, pinned host identity fingerprint, and
the last fixed non-secret service error code. Its identity check uses SQLite in
read-only mode and does not start the normal runtime, so it cannot hide key loss
by triggering the runtime's recovery behavior.

## TDD evidence

Representative red runs captured during this task:

- CLI tests initially failed to compile with `unknown field Setup` and
  `unknown field Installer`.
- Setup tests initially failed with undefined `New`, `Options`, and dependency
  error values.
- The Tailscale-interface test showed that `EADDRNOTAVAIL` was incorrectly
  reported as a port collision before the error mapping was fixed.
- The read-only doctor integration test failed because `doctor` created
  `state.sqlite3` before inspecting it.
- Partial-install tests showed missing `stop` and `remove` calls after backend
  install/start failures.
- The LaunchAgent test failed until the log directory and explicit `HOME`
  environment were created.
- The exact-start test failed until health records carried and matched the
  prepared attempt ID.
- The persistent-config test failed until a fresh manager process could restore
  the prior config during rollback.
- The Windows partial-install test proved the scheduled task was created before
  a failing marker write; marker publication now happens first.
- The cross-process lock test held reconfiguration open and proved a concurrent
  uninstall is rejected before touching shared files.
- The interrupted-replacement retry test failed twice because recovery looked
  for consumed `.previous`/`.discarded` files. Recovery now retains a private
  transaction snapshot until it succeeds; the first forced restart failure
  leaves it intact and the second recovery passes.
- Prepared-phase tests confirm the journal and binary snapshot exist when
  replacement first calls service stop. Healthy-phase tests confirm recovery
  publishes rollback state without stopping the replacement, and finishes a
  successful rollback without restarting the service.
- The native macOS probe exposed `launchctl bootstrap` exit 5 after immediate
  unload/reload; the backend now waits for the exact launchd target to disappear.
- Launchd status tests failed until loaded/waiting jobs were distinguished from
  `state = running` or a positive active process ID.
- Reconfigure recovery tests failed with `status, stop, start, start` until the
  possibly running new-config process was explicitly stopped. The passing call
  order is `status, stop, start, stop, start`, and a forced second-stop failure
  returns the repair-required error without overwriting the active config.

Final local gate:

```text
go test ./... -race -count=1   PASS (all Go packages)
go vet ./...                   PASS
git diff --check               PASS
node release/checks/companion-install-smoke.test.mjs
                                PASS, 18 assertions
bash -n release/checks/companion-smoke.sh
                                PASS
GOOS=linux GOARCH=amd64 go build ./companion/cmd/codex-launcher
                                PASS
GOOS=windows GOARCH=amd64 go build ./companion/cmd/codex-launcher
                                PASS
```

PowerShell is not installed on this Mac, so the PowerShell parser check is not
yet verified locally. The script's static release contract passed.

## Native macOS evidence

Preflight found no existing `app.codexlauncher.companion` launchd label, config
root, or LaunchAgent file. The machine is macOS 15.5 arm64 and the working Codex
binary reports `codex-cli 0.144.1`. Tailscale is not installed.

The reproducible probe at `companion/integration/hostinstallnative/main.go`
exercised the production manager and LaunchAgent backend against a local test
service which implements only `version` and `serve`. Run with:

```bash
go run ./companion/integration/hostinstallnative
```

The real launchd calls completed:

```text
install completed
replacement completed
rollback completed
uninstall completed
native host-install smoke: install, replace, rollback, and uninstall passed
```

After uninstall, the install root, LaunchAgent plist, and loaded launchd target
were all absent.

A separate real-Codex start initialized the owned Codex app-server, then failed
at the required private ChatGPT Desktop follower while the Mac was locked:

```text
[codex-process] initialized version_checked=true
[codex-runtime] Desktop connection failed
Codex is unavailable on this computer.
```

The replacement manager restored V1 after that failed service restart. This is
positive rollback evidence, but it is not a successful real-Codex macOS smoke.
The runtime intentionally does not fall back to app-server-only mode on macOS,
because that would hide desktop-owned tasks.

## Still open

- Install and sign in to the official Tailscale clients on the Mac and Android
  test device, then repeat setup and connection recovery with a real tailnet.
- Unlock the Mac and repeat the native real-Codex initialize/read/event/deny/
  restart smoke through the verified desktop follower.
- Run native Linux and Windows lifecycle/real-Codex smokes. Current evidence is
  unit coverage plus cross-compilation only, so both remain experimental.
- Run the PowerShell parser and smoke on Windows.
- Run the deferred installed-executable replace/rollback/uninstall and Windows
  ACL tests on native Windows. They cross-compile but have not run on Windows.

No paid model call was made during this checkpoint.

## Reuse

Run the local code gate from the repository root:

```bash
go test ./... -race -count=1
go vet ./...
node release/checks/companion-install-smoke.test.mjs
bash -n release/checks/companion-smoke.sh
```

The lifecycle smoke entry points are:

```text
release/checks/companion-smoke.sh
release/checks/companion-smoke.ps1
```
