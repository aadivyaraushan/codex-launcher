# Android Launcher Shell Checkpoint

**Date:** 2026-07-13

## Purpose

Record the first Android 16 launcher behavior and the pure state/design rules
that the later Compose screens must consume.

## Result

- The installed app declares separate Android `MAIN + HOME + DEFAULT` and
  ordinary `MAIN + LAUNCHER` entry points.
- The Home policy keeps the computer fixed, maps only an opaque selected
  project ID to its display name, and enables sending only after an online
  snapshot with a positive base sequence and a currently offered project.
- Every disconnected, connecting, syncing, incompatible, and revoked state
  removes task content and the snapshot cursor from the rendered Home state.
- All apps and Android Settings escape-route flags remain present in every
  Home state. These are policy flags only; the actual buttons are still part
  of the pending Compose UI.
- The approved Deep Charcoal and Warm Paper palettes, type minimums, spacing,
  touch sizes, and task-state wording are now pure Kotlin tokens. JVM tests
  verify both normal-text colors meet a 4.5:1 contrast floor.

## Test-first evidence

- `LauncherRoleTest.homeIntentResolvesToLauncherActivity` failed on the Pixel
  9 AVD before the Home intent filter was added.
- `HomeUiStateTest` first failed to compile before the Home policy existed.
- `QuietInstrumentTokensTest` first failed to compile before the tokens
  existed.

## Verification

```text
Pixel AVD: codex_launcher_pixel_9_api_36 / Android 16 / API 36
JVM tests: 37 tests, 0 failures, 0 errors, 0 skipped
Device tests: 5 tests, 0 failures, 0 errors, 0 skipped
Android lint: PASS
Repository bootstrap smoke: PASS
git diff --check: PASS
```

Manual Android role interaction on the running AVD:

```text
Home query: 3 activities, including app.codexlauncher/.LauncherActivity
cmd role add-role-holder android.app.role.HOME app.codexlauncher: exit 0
Press Home: topResumedActivity = app.codexlauncher/.LauncherActivity
Press Back: topResumedActivity remains app.codexlauncher/.LauncherActivity
```

The independent checkpoint judge returned `READY` with no P1 or P2 findings.

Reproduce from `android/` with the Pixel AVD running:

```bash
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=$(/usr/libexec/java_home -v 17) \
./gradlew :app:testDebugUnitTest :app:connectedDebugAndroidTest
```

## Still pending

- Compose dependencies, theme adapter, bundled fonts, persistent appearance
  preference, insets, large-font/reduced-motion UI checks, and the actual seven
  screens.
- Real All apps and Android Settings buttons; the present checkpoint proves
  only policy and platform availability.
- Pairing Keystore, pinned transport, foreground connection service, and the
  full computer-use-style Pixel interaction matrix.
- Compose/compiler and DataStore API work remains paused until the required
  official-doc browser path is available.
