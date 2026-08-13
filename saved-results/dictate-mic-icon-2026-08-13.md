# Home Dictate mic icon

**Date:** 2026-08-13  
**For:** Replace the home Dictate lightning glyph (`⌁`) with a real microphone icon without changing Deepgram tap-to-talk.  
**PR:** https://github.com/aadivyaraushan/codex-launcher/pull/17  
**Branch:** `cursor/dictate-mic-icon-6f34` into `worktree-phase2-tool-bridge`

## Result

Home Dictate and task follow-up Dictate draw `Icons.Filled.Mic` when idle, and `"…"` while recording or uploading. Home reads `homeDictationTap.state.recording/uploading` from `LauncherActivity`. Labels stay `Dictate prompt` / `Dictate follow-up`. Deepgram key handling, `RECORD_AUDIO`, and composer merge are unchanged.

Docs checked: Android [Icons.Filled](https://developer.android.com/reference/kotlin/androidx/compose/material/icons/Icons.Filled) (core set has no Mic), [Compose Material 3 1.4.0 notes](https://developer.android.com/jetpack/androidx/releases/compose-material3) (add `material-icons-extended` yourself; Mic is not in core), and the [Bottom app bar sample](https://developer.android.com/develop/ui/compose/components/app-bars) which uses `Icon(Icons.Filled.Mic, ...)`.

## What changed

- Shared `PromptDictationMicIcon(busy)`: mic when idle, `"…"` when busy.
- `HomeScreen` takes `dictationRecording` / `dictationUploading`; button stays enabled while recording (second tap stops) and disables while uploading.
- `LauncherActivity` passes `homeDictationTap.state.recording/uploading` into Home. `onDictate` still calls the same Deepgram tap.
- Tests: idle has no `⌁` / `"Mic"` / `"…"`; recording shows `"…"`; uploading shows `"…"` and disables the home button.

## Verify

From `android/` with `ANDROID_HOME` set:

```bash
./gradlew :app:compileDebugKotlin :app:compileDebugAndroidTestKotlin
./gradlew :app:testDebugUnitTest --tests app.codexlauncher.task.control.PromptDictationTest \
  --tests app.codexlauncher.task.control.dictation.PromptDictationEngineTest \
  --tests app.codexlauncher.task.control.dictation.DeepgramListenClientTest \
  --tests app.codexlauncher.task.control.dictation.DeepgramApiKeySourceTest \
  --tests app.codexlauncher.task.composer.DraftComposerViewModelTest
```

This cloud run (2026-08-13), after wiring live recording/uploading:

- `compileDebugKotlin` and `compileDebugAndroidTestKotlin`: BUILD SUCCESSFUL
- Dictation-related unit tests: **36 tests / 0 failures** (`PromptDictationTest` 7, `PromptDictationEngineTest` 10, `DeepgramListenClientTest` 5, `DeepgramApiKeySourceTest` 4, `DraftComposerViewModelTest` 10)
- Full `:app:testDebugUnitTest` (earlier on this branch): **708 tests / 1 failure**, `TaskControlViewModelTest.accepted start_turn with forkTaskId opens the new thread` expected `thread-1` got `phone-home`. That class was not edited here.
- Instrumented `HomeScreenTest` / `TaskScreenTest` were not run: no Android device/emulator in this VM.

## Independent judge

First review: **pass-with-nits** — follow-up had lost `"…"` while busy. That nit is now fixed: home and follow-up share `PromptDictationMicIcon`, and Home receives live `recording`/`uploading` from `homeDictationTap`.

## Sibling search

Searched Kotlin for `⌁`, `Text("Mic")`, `Dictate prompt`, `Icons.Filled.Mic`. App glyphs were only Home `⌁` and follow-up `"Mic"`/`"…"`. Left `outputs/codex-launcher-visual-directions.html` mock (still `⌁`) because it is not the Android control; `design-contract.test.mjs` only asserts `aria-label="Dictate prompt"`.
