# Operator on-device dictation

Date: 2026-08-24

## What this is for

Replace Android's Google/system speech activity with English-only, on-device dictation that writes live text into Operator's prompt and follow-up fields. Recording continues until the user taps Stop, then the transcript is editable. The model downloads once with visible progress and is then reusable offline.

## Result

- Operator now uses Moonshine Voice 0.1.5 with the Small Streaming English model. No cloud transcription fallback remains.
- First use on the connected Pixel 9 visibly moved from `Downloading voice model… 1%` to `62%`, then to `Listening… Tap Stop when finished.`
- Live microphone transcription changed the visible prompt while recording. The Stop control was still present after continued speech and pauses; tapping it returned the field to an enabled, editable state.
- The model cache under Operator's private `no_backup/moonshine-models` directory occupied 139,123 KiB, about 136 MiB.
- With Wi-Fi and mobile data disabled, a full Operator process restart loaded the disk-cached model and reached Listening in 559 ms (`1787616148.495` request to `1787616149.054` listening). Both networks were restored afterward.
- The first installed implementation reached Listening in 601 ms and returned from explicit Stop to an editable draft in 303 ms. Independent review then found that Moonshine's `stop()` alone left its audio-capture thread alive. The final implementation now calls `stop()`, `close()`, and creates a fresh transcriber; controller tests prove release/reload, failure cleanup/retry, and navigation cleanup. A final Pixel microphone-indicator rerun could not start because the secure lock screen returned before handoff.
- The test transcript was removed afterward. The final prompt field was empty and editable, Wi-Fi was enabled, and mobile data was restored to `1`.
- The final debug APK was 112,325,557 bytes (about 107 MiB). The prior debug APK observed before this change was about 47 MiB.

The live test proved the full local data path, but it did not produce a controlled word-error score: nearby ambient speech was the input. Accuracy should be judged with the user's normal speaking voice before changing model size.

## Test evidence

- TDD red: the first unit test compile failed because `LiveDictationDraft` did not exist; the first Android screen tests failed because the new dictation state and callbacks did not exist.
- Unit, lint, and packaging: `testDebugUnitTest lintDebug assembleDebug assembleDebugAndroidTest` completed with `BUILD SUCCESSFUL in 40s`. The unit XML files contain 758 tests with zero reported failures.
- Pixel screen tests: direct instrumentation returned `OK (2 tests)` for the Home download/Stop flow and Task live-text/Stop flow. Direct instrumentation was used so app data was not cleared.
- Independent review initially failed the build after inspecting the pinned Moonshine bytecode and finding that `MicTranscriber.stop()` did not release `AudioRecord`. A TDD fix changed the engine contract to release the microphone with `stop()` plus `close()`, reload the disk cache on the next session, and clean up on runtime errors and navigation. The judge's second review returned PASS and its four focused controller tests passed.
- Release checks: repository baseline, toolchain pins, Android runtime contract, and Android debug surface all passed, 4/4.
- Source scan: active source and release checks contain no old `PromptDictationContract`, `PromptDictationResult`, `beginDictation`, or `applyDictation` path. The only active `RecognizerIntent` text is a release assertion forbidding its return; older saved evidence remains historical.
- `git diff --check` passed.

## Remaining physical check

The final APK with the microphone-release fix is installed on Pixel serial `4B230DLAQ001Z5`. The phone securely locked before the last direct instrumentation and microphone-app-operation check. Unlock the Pixel, start and stop one short dictation, and confirm Android's microphone indicator turns off; no code or model download should be needed.

## Reproduce

From the worktree root:

```sh
cd android
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./gradlew --no-daemon testDebugUnitTest lintDebug assembleDebug assembleDebugAndroidTest
cd ..
node --test release/checks/repository-baseline.test.mjs release/checks/toolchain-pins.test.mjs release/checks/android-runtime-contract.test.mjs release/checks/android-debug-surface.test.mjs
```

For the two focused Pixel checks, first confirm the intended serial, install both APKs with `adb -s <serial> install -r`, then run the two named classes directly with `adb shell am instrument`. Do not use the connected Gradle test task because this project preserves pairing and drafts during device verification.
