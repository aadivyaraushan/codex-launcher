# Android Session Reconnect

**Date:** 2026-07-13

## What this checkpoint proves

The Android session owner now retries only after a network or socket loss. It
waits 1, 2, 4, 8, 16, then at most 30 seconds between attempts and reconnects
to the same paired computer with a fresh session ID. The launcher remains in
the existing disconnected state, which clears the computer snapshot and maps
to `Computer offline`, until a new snapshot is applied.

Manual disconnect cancels a pending retry. Revoked pairing and incompatible
protocol states cancel retry and forget the automatic retry target. A fresh
snapshot resets the attempt counter, so a later outage starts again at one
second.

## TDD evidence

The first focused run failed during test compilation because the production
view model did not accept the new `retryWait` behavior:

```text
LauncherSessionViewModelTest.kt:241:13 No parameter with name 'retryWait' found.
LauncherSessionViewModelTest.kt:270:13 No parameter with name 'retryWait' found.
BUILD FAILED
```

After the minimum implementation, the focused
`LauncherSessionViewModelTest` run passed all 12 tests. These tests cover the
delayed reconnect, revoked and incompatible stop states, manual cancellation,
the capped delay sequence, repeated failure attempts, and reset after a fresh
snapshot.

## Final verification

```text
Android JVM tests: 112, 0 failures, 0 errors
Android API 36 Pixel 9 AVD tests: 37, 0 failures, 0 skips
Android lint: PASS
git diff --check: PASS
```

Reproduce:

```bash
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export ANDROID_SDK_ROOT="$ANDROID_HOME"
cd android
./gradlew testDebugUnitTest lintDebug connectedDebugAndroidTest
```

## Related-path search

The repository was searched for all `SessionFailure`, `ConnectionLost`,
`PairingRevoked`, `IncompatibleVersion`, `connect`, and `disconnect` handling.
The production `CompanionSessionClient` reports failures but does not schedule
retries, leaving the launcher session view model as the one retry owner.

`ConnectionLifecycleSpike` has a separate fixed-delay reconnect loop. It is an
older Android foreground-service test spike registered only for its dedicated
instrumentation path; it does not own an authenticated paired launcher session,
so it was left unchanged.

The independent judge found no P1 or P2 issue and returned `READY`. Its one P3
test gap was then closed with a controlled test proving the attempt sequence is
`1`, `2`, then `1` after a successful fresh snapshot.

## Still pending

- A physical Pixel 9 over Tailscale has not run the reconnect path.
- The foreground service is not yet wired as the long-lived owner of this
  session view model.
- Reduced-protection confirmation and complete unpair/wipe behavior remain.
