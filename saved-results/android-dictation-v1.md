# Android follow-up dictation V1

**Date:** 2026-07-14  
**Purpose:** Record the implemented Android speech-to-composer boundary and its privacy checks.

## Result

- The Voice button starts Android's installed `ACTION_RECOGNIZE_SPEECH` activity with the free-form language model and the visible prompt `Speak your follow-up`.
- The first non-blank returned phrase is trimmed and appended to existing typed text without changing the existing indentation or trailing whitespace. It remains editable and is not sent until the user presses the normal send, queue, or redirect button.
- Cancel, empty/error, missing-activity, and security-rejection paths keep the existing text and show distinct messages.
- The app does not request `RECORD_AUDIO`; the system speech activity owns audio capture. Only its returned text reaches the composer.
- Follow-up drafts live only in the retained `LauncherSessionViewModel`. A saved-state restoration test proves the default composer text is empty after Android saves/restores Compose state, while a unit test proves the retained ViewModel restores the same task's draft during normal in-app navigation and after a fresh companion snapshot. Explicit unpair and ViewModel destruction clear that memory; a transient connection loss does not.
- Logs record only the result kind, request source, error type/shape, task ID, text length, and the `memory_only` storage decision. They never log recognized or typed text.

## Official API evidence

The Android reference checked on 2026-07-14 says `ACTION_RECOGNIZE_SPEECH` starts an activity that prompts for speech, `EXTRA_LANGUAGE_MODEL` selects the preferred model, and `EXTRA_RESULTS` contains an `ArrayList<String>` of results. Android's Activity Result guide recommends the AndroidX Activity Result API and says its callback must be registered on every recreation.

- <https://developer.android.com/reference/android/speech/RecognizerIntent>
- <https://developer.android.com/training/basics/intents/result>

## TDD evidence

The first test compile failed because `PromptDictationContract`, `PromptDictationResult`, `mergePromptDictation`, and the TaskScreen dictation callback did not exist. After implementation, the focused contract and UI tests passed. A later Pixel saved-state test failed because the first implementation used `rememberSaveable` and restored `Keep across recreation`; this exposed plaintext prompt storage. After moving the draft to retained ViewModel memory and using plain `remember` only as a test/default fallback, the restoration test reported empty editable text and passed. A separate test then failed to compile until an explicit unpair memory-clear operation was added. Independent review found two more failures: snapshot rebuild dropped the visible draft, and append logic trimmed existing formatting. Both new regression tests failed before their fixes and passed afterward. The final independent rerun returned `READY`.

## Verification

```text
./gradlew :app:testDebugUnitTest :app:lintDebug :app:connectedDebugAndroidTest
Starting 70 tests on codex_launcher_pixel_9_api_36(AVD) - 16
Finished 70 tests on codex_launcher_pixel_9_api_36(AVD) - 16
0 skipped, 0 failed
BUILD SUCCESSFUL in 1m 58s
```

The focused dictation set ran 13 Pixel interactions covering the contract, append/cancel/unavailable/error UI states, normal task controls, and saved-state privacy before the full suite.

## Reproduce

```bash
cd /Users/aadivyar/Documents/Codex/2026-07-12/uf-u-implementation/android
JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home \
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
./gradlew :app:testDebugUnitTest :app:lintDebug :app:connectedDebugAndroidTest
```

## Remaining Task 10 work

- Android photo/document selection.
- Authenticated attachment upload, disk-backed companion storage, quota enforcement, hash checking, cleanup, restart recovery, and Codex handoff.
