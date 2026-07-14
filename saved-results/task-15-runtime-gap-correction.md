# Task 15 Android runtime gap correction

Date: 2026-07-14

## What this records

The first full Android behavior inventory found two production gaps before the
hands-on emulator audit:

1. Home displayed a dictation button, but `LauncherActivity` did not connect it
   to Android speech recognition.
2. Offline Home supported a last-successful-connection label, but production
   did not record or supply one.

This checkpoint records the tested correction. It does not claim that the full
Task 15 emulator audit is complete.

## Observable result

- Home dictation now starts the Android speech recognizer only while the exact
  draft is editable. The draft ViewModel records the launch version and applies
  returned text in one guarded update. Typing, reset, or a pairing-owner change
  while recognition is open rejects the stale result without changing text or
  claiming success.
- The session records a timestamp only when a fresh authenticated snapshot
  moves the connection into `ONLINE`. Refresh snapshots do not rewrite it. The
  observer captures the originating pairing generation, so a stale observer
  cannot assign a timestamp to a later pairing.
- The timestamp is stored per pairing, survives store recreation, and is part
  of the recoverable private-state wipe.

## TDD evidence

The first focused run failed before implementation with missing
`PromptDictationUpdate`, `homeDictationUpdate`, and `LastConnectionStore` code.
The independent review then found two deeper cases. Tests added for those cases
failed with missing `shouldRecordSuccessfulConnection`, `beginDictation`,
`applyDictation`, and `homeDictationMessage`, plus a failing runtime contract.

After implementation, these checks passed:

```text
./android/gradlew -p android :app:testDebugUnitTest :app:lintDebug
BUILD SUCCESSFUL
246 unit tests, 0 failures, 0 errors, 0 skipped

release/checks/*.test.mjs
android runtime contract: 10 assertions passed
companion install smoke contract: 18 assertions passed
design contract: 19 assertions passed
public alpha release contract: 120 assertions passed
repository baseline: 21 assertions passed
toolchain pins: 13 assertions passed

connectedDebugAndroidTest on codex_launcher_pixel_9_api_36 Android 16
19 targeted tests, 0 skipped, 0 failed
```

The targeted device run included `HomeScreenTest`,
`LastConnectionStoreInstrumentedTest`, and
`LocalStateWiperInstrumentedTest`.

## Reuse

Run the local checks from the repository root:

```bash
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./android/gradlew -p android :app:testDebugUnitTest :app:lintDebug --no-daemon --console=plain
for test in release/checks/*.test.mjs; do node "$test"; done
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./android/gradlew -p android :app:connectedDebugAndroidTest '-Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.storage.connection.lastseen.LastConnectionStoreInstrumentedTest,app.codexlauncher.storage.wipe.LocalStateWiperInstrumentedTest,app.codexlauncher.launcher.home.HomeScreenTest' --no-daemon --console=plain
```

## Still open

Task 15 still needs the debug-only scenario surface, hands-on ADB interaction
across the full UI behavior checklist, actual launcher/system lifecycle checks,
the final full test run, and a fresh whole-product judge.
