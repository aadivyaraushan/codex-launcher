# Android Offline and Sync State

**Date:** 2026-07-13

## Purpose

Record the launcher rule that computer content is visible only after a fresh,
complete sync and disappears immediately when the connection is no longer
trusted.

## Implemented result

```text
DISCONNECTED -> CONNECTING -> SYNCING -> ONLINE
       ^              |          |          |
       +--------------+----------+----------+
                    connection loss
```

- `DISCONNECTED` always says `Computer offline` and cannot show task content or
  send a prompt.
- A valid socket moves to `SYNCING`, but content remains hidden until an atomic
  snapshot with a positive base sequence is applied.
- `ONLINE` can show computer content. Sending additionally requires an opaque
  project ID matching the companion's restricted ID shape. Raw paths and
  oversized strings cannot become a selection. If the companion reports that
  the selected folder is unavailable, the selection clears and sending stops.
- Any connection loss clears the snapshot sequence and hides computer content.
  A reconnect must sync again before becoming online.
- `INCOMPATIBLE_VERSION` and `REVOKED` do not respond to an automatic retry
  timer. Revocation also clears the selected project.

## Verification

```text
./gradlew :app:testDebugUnitTest
BUILD SUCCESSFUL; all Android JVM tests passed

./gradlew :app:connectedDebugAndroidTest
Pixel 9 Android 16 AVD: 2 tests, 0 failures
BUILD SUCCESSFUL in 23s
```

The connection state machine is still a pure tested rule set. Wiring it into
the foreground connection service and approved launcher UI remains pending.
