# Android local-state wipe V1

Date: 2026-07-13

## What this checkpoint proves

Removing the paired computer now starts a fail-closed cleanup that blocks old
and new private-state writes, records a durable wipe marker, removes every
allowed private store, and only then opens fresh pairing. The owner of the gate,
stores, keys, and wiper lives in `LauncherApplication`, so recreating
`LauncherActivity` does not create a second write gate around retained view
models.

The cleanup removes:

- selected project
- metadata-only action records
- unfinished-draft ciphertext and its separate AES key
- app-private device identity
- phone pairing key
- paired-computer record

If a deletion or startup read fails, pairing and paired-state writes stay
blocked and the launcher shows its recovery screen. A second cleanup request is
reported as already running and cannot invalidate the first cleanup.

## TDD evidence

The new tests were run before implementation and failed to compile because
`LocalWipeResult`, `WipeResult.AlreadyInProgress`, and `LauncherApplication` did
not exist. After implementation:

```text
./gradlew testDebugUnitTest lintDebug
BUILD SUCCESSFUL in 14s
170 tests, 0 failures, 0 errors, 0 skipped
```

```text
./gradlew connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.storage.wipe.LocalStateWiperInstrumentedTest,app.codexlauncher.storage.wipe.UnpairActivityTest,app.codexlauncher.launcher.home.HomeScreenTest,app.codexlauncher.LauncherActivityTest
Finished 18 tests on codex_launcher_pixel_9_api_36(AVD) - 16
BUILD SUCCESSFUL in 48s
```

The Pixel test seeds real DataStore records and Android Keystore keys, recreates
the launcher activity, taps the computer-management control and removal dialog,
then verifies the launcher returns to fresh pairing with all targeted stores
empty and only pairing-mode writes allowed.

## Android Ed25519 compatibility fix

The first real-device wipe setup exposed an Android provider problem while
validating a stored host identity: Android Keystore cannot import the external
Ed25519 host public key through `KeyFactory`. Host identity verification now
validates the exact Ed25519 SPKI prefix, extracts its 32-byte raw public key,
and uses Tink's pure-Java `Ed25519Verify`. The pinned identity format and
signature verification behavior stay unchanged.

## Re-run

Use JDK 17 and the installed API 36 SDK:

```text
export JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
cd android
./gradlew testDebugUnitTest lintDebug
./gradlew connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.storage.wipe.LocalStateWiperInstrumentedTest,app.codexlauncher.storage.wipe.UnpairActivityTest,app.codexlauncher.launcher.home.HomeScreenTest,app.codexlauncher.LauncherActivityTest
```
