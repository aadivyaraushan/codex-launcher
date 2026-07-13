# Live Project Session Wiring

**Date:** 2026-07-13

## What this checkpoint proves

The companion now owns the authenticated mobile session handler. A cold phone
hello receives a validated welcome followed by a safe snapshot containing only
the computer display name, opaque project choices, and an empty task list. A
`set_project` action is resolved against the approved project service and gets
a sequenced `confirmed` or `failed` result.

The Android launcher now connects through its pinned session client, shows
connecting and syncing before accepting a fresh snapshot, maps the safe project
fields, opens the project selector from Home, waits for the matching action
result, and clears its launcher snapshot when the session ends. It never shows
the project path because the mobile contract does not contain one.

## TDD evidence

The session ViewModel timing test was first run against the old behavior and
failed at `LauncherSessionViewModelTest.kt:102`: after a connection failure, a
delayed stored-project load restored the launcher to online. The implementation
now advances the session generation when it accepts a failure, so every older
callback and load result is ignored. The focused rerun passed.

The Go handler/transport tests and Android bridge/ViewModel tests were also
written before their missing implementations during this checkpoint. They
cover cold welcome/snapshot output, approved and rejected project actions,
per-device acknowledgement storage, outbound WebSocket validation, matching
action results, send failure, closed sessions, live connection phases, and
stale snapshot invalidation.

## Final verification

```text
Go race tests: all companion packages PASS
Go vet: all companion packages PASS
Protocol schema plus semantic checks: validated 32 valid frames; rejected 30 invalid frames
Android JVM tests: 106, 0 failures, 0 errors, 0 skipped
Android API 36 Pixel 9 AVD tests: 37, 0 failures, 0 skips
Android lint: PASS
git diff --check: PASS
```

The running emulator reported API level `36` before the device run. Codex
Computer Use could not see the QEMU emulator window in its exposed Mac app
list, so a direct visual click-through was not possible. The real API 36 device
suite is the fallback UI and behavior evidence for this checkpoint.

Reproduce:

```bash
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export ANDROID_SDK_ROOT="$ANDROID_HOME"
export PATH="$ANDROID_HOME/platform-tools:$PATH"
go test -race ./companion/...
go vet ./companion/...
python3 release/checks/protocol/schema_test.py
(cd android && ./gradlew testDebugUnitTest lintDebug connectedDebugAndroidTest)
git diff --check
```

## Same-contract search

The repository was searched for every `SessionObserver`, `SessionConnection`,
`MessageHandler`, `MessageSender`, `MobileHandler`, and `onReady` use. All
callers now use the send-capable session interfaces, and no stale injected
`MobileHandler` dependency remains. The one project action envelope now uses
the shared Android protocol-major constant instead of a second literal.

The independent review initially found four P2 gaps. The settled code now:

- replaces the older authenticated connection for a device before accepting
  more events and rejects any later action from that replaced connection;
- sends cumulative snapshot and terminal-result acknowledgements, with a
  controlled test proving a project result is not acknowledged until its UI
  state has been applied;
- gates the selector on the live `ONLINE` phase so stale computer choices cannot
  remain visible while the Home redirect runs; and
- requires the companion's advertised `set_project` capability, otherwise
  closing the socket and showing the incompatible-version state.

The same independent judge rechecked those four fixes against the final files,
reran the focused Go race and Android tests, found no remaining P1/P2 issue,
and returned `READY`.

## Still pending

- The companion CLI still needs a real long-running serve/setup/doctor flow and
  durable SQLite-backed runtime state.
- Automatic reconnect, foreground-service ownership, reduced-protection
  confirmation, and unpair/wipe are not complete.
- Task lists, transcripts, prompt sending, approvals, and questions are later
  slices; this snapshot intentionally publishes an empty task list.
- A physical Pixel 9 over Tailscale has not yet run this live path.
