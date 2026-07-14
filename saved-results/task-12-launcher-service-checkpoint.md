# Task 12 Launcher and Connection Service Checkpoint

**Date:** 2026-07-14
**Purpose:** Record the production app drawer, Appearance screen, foreground
connection service, Android 16 VM evidence, and the release tests that still
need a real Tailscale pairing.

## Result

The production Android app now has one application-owned authenticated Codex
session. `CodexConnectionService` observes that session instead of opening a
second socket, promotes itself immediately as a `connectedDevice` foreground
service, watches Android's default network, and requests an immediate reconnect
only when the session is actually disconnected. Android activity recreation
does not replace the session or draft owners, disconnect the healthy session,
or restart the foreground service. Temporary pairing-storage loading now leaves
the process connection alone; only confirmed unpairing or failed recovery tears
it down.

Generic notifications cover reply, approval, question, failure, and computer
offline changes. They contain no task title, prompt, path, command, task ID, or
other computer content. A denied permission or blocked channel suppresses the
update without stopping the connection. Deleted channels are recreated the next
time the visible launcher starts the service.

A rejected foreground-service start is shown as
`Background connection unavailable`, and an app that disappears between drawer
loading and launch now produces a visible error beside the app list.

The old unauthenticated `ConnectionLifecycleSpike` and its duplicate WebSocket
owner were removed.

## TDD evidence

The first unit run failed because the production notification types did not
exist:

```text
Unresolved reference 'ConnectionNotificationPolicy'
Unresolved reference 'ConnectionNotice'
Unresolved reference 'StreamState'
BUILD FAILED
```

The first service compile failed because start rejection handling did not
exist:

```text
Unresolved reference 'startForTest'
BUILD FAILED
```

The healthy-session network test then found an actual duplicate-connection
bug:

```text
LauncherSessionViewModelTest > networkAvailableDoesNotReplaceAHealthyOnlineSession FAILED
1 test completed, 1 failed
```

The fix ignores default-network callbacks while a session is already
connecting, syncing, or online, and reconnects immediately only from the
disconnected state.

The process-recovery tests also began red because `ConnectionBootstrapper` did
not exist, then passed after the saved-pairing bootstrap was implemented.

The independent judge then found that activity recreation treated temporary
pairing loading as unpaired. The regression tests first failed to compile on
the missing lifecycle command and service create/destroy counters:

```text
Unresolved reference 'PairingConnectionCommand'
Unresolved reference 'createCount'
Unresolved reference 'destroyCount'
BUILD FAILED
```

After the fix, the paired recreation test observed no additional service create
or destroy. The same red run also required visible start-rejection and app-launch
failure messages. A stronger suite then found an existing test that left Android
Settings in front of the following Compose test; that ordering failed once, the
Settings test was changed to wait until Back actually returned, and the affected
16-test sequence passed.

## Final automated verification

Environment:

```text
AVD: codex_launcher_pixel_9_api_36
Hardware profile: Pixel 9
Android: 16 / API 36
ABI: arm64-v8a
```

Final command:

```bash
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
cd android
./gradlew :app:testDebugUnitTest :app:lintDebug :app:connectedDebugAndroidTest
```

Observed result:

```text
JVM tests: 239
Android VM tests passed: 84
Skipped Android VM tests: 1 (the separately driven network cycle)
Failed Android VM tests: 0
Lint: PASS
BUILD SUCCESSFUL in 2m 6s
```

The VM tests cover app search and launch, hidden/disabled app filtering,
Android Settings routing, Appearance controls, activity recreation, service
promotion and explicit stop, connected-device manifest type, channel recovery,
permission suppression, generic task notifications, start rejection, app-owned
session identity, paired activity recreation without a service restart,
screen-off, confirmed deep Doze, in-process stream observation in Doze and
after wake, and default-network observation. The forced-idle test does not send
a WebSocket frame, so network delivery during deep Doze remains open. The
external network test is marked skipped unless
its required outside controller is explicitly enabled, so an ordinary run no
longer records that gate as a false pass.

## Real VM interaction evidence

The app was installed into the Pixel 9 Android 16 VM and operated through the
Android accessibility tree, real taps, text input, screenshots, activity state,
and package state:

1. Selected Codex Launcher as Android's HOME role and preferred HOME activity.
2. Cold-started it with the Home key.
3. Opened the app drawer, typed `tm`, and observed only `TMoble` remain.
4. Launched Android Settings through both the fixed escape route and the
   installed-app row.
5. Opened Launcher settings, selected Dark, and observed the checked state and
   rendered dark screen.
6. Force-stopped and reopened the launcher during the UI pass.
7. Rebooted the emulator twice. The HOME role persisted after the corrected
   preferred-activity setup, and the Home key reopened Codex Launcher after
   Android completed boot.
8. Triggered a real Android background foreground-service start rejection. The
   VM returned `ForegroundServiceStartNotAllowedException`.
9. Re-ran the externally controlled network-cycle test after the evidence fixes
   while the production service and test stream were live. Android changed
   `mDefaultNetwork` from network `107` to `null`, the service logged
   `default network was lost`, Android created network `108`,
   and the service logged `default network became available` before the focused
   test passed (`BUILD SUCCESSFUL in 28s`).

Screenshots:

- `outputs/task-12-vm-evidence/pairing.png`
- `outputs/task-12-vm-evidence/app-search.png`
- `outputs/task-12-vm-evidence/appearance-dark.png`
- `outputs/task-12-vm-evidence/reboot-home.png`

## Related-path audit

The repository was searched for the old spike, connection retries, session
owners, notification text, app launching, and Appearance controls.

- The production manifest declares only
  `.connection.stream.CodexConnectionService` as the connection service.
- `LauncherSessionViewModel` remains the only authenticated reconnect owner.
- `LauncherStreamClient` only maps safe phase and opaque task-state data into
  the service.
- `InstalledAppsRepository` continues to use `LauncherApps`; it does not add a
  broad package-query permission. Its only launch caller now checks the Boolean
  result and shows failure instead of discarding it.
- Appearance still contains only theme choices, two previews, and the approved
  Android settings note.

## Still open before release completion

These are not claimed as verified:

- Tailscale is not installed on this Mac, so a real Tailscale stop, restart,
  update, and VPN-loss cycle has not run.
- A paired production session has not yet been force-stopped and restored after
  full process death on the VM. The app-owned bootstrap and its fail-closed
  branches are tested, but that is indirect evidence.
- The forced deep-Doze test proves the foreground service and its in-process
  observer remain alive, but it does not prove that a server-to-phone WebSocket
  frame crosses the network during deep idle. That requires a real paired
  companion or a production-socket harness.
- The real Android notification permission dialog was not denied while a paired
  session was active. Revoking the permission from inside instrumentation kills
  the instrumentation process on Android 16, so the controlled permission
  branch is the automated evidence for now.
- Computer Use could not inspect the macOS emulator window because the Mac was
  locked. The recorded Android accessibility/tap/screenshot path is the current
  fallback, not a substitute for the final Computer Use pass required by the
  overall goal.
- A physical Pixel 9 has not run this checkpoint.

## Reuse

After Tailscale is installed and the Mac is unlocked, pair the VM with the real
companion, repeat the network and force-stop cases with the authenticated
session, then retry Computer Use and append the evidence here.
