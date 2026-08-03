# Pixel Auto→Open smoke — send no-op diagnosis + fix

**Date:** 2026-08-02  
**Purpose:** Explain why overnight Discord/Uber adb Auto→Open smokes left text in the Prompt field and never hit companion capability logs — and record the UI fix.

## Gate facts

1. **Callers:** None in code — human/overnight artifact under `saved-results/`.
2. **Peer:** overnight batch status; Discord/Uber smoke evidence files.
3. **Data I/O:** none (static diagnosis + Home Send enablement).
4. **User:** heartbeat continue consumer plan — diagnose Pixel smoke failures.

## Code path (verified)

1. `HomeScreen` send calls `onSend(composerState.text, selection)` where `selection = newTaskOptions?.normalize(...)` (`HomeScreen.kt`).
2. `LauncherActivity` only submits when both are present:

```kotlin
onSend = { prompt, selection ->
    val version = draftComposerState.version
    if (selection != null && version != null) {
        scope.launch { sessionViewModel.submitHomePrompt(prompt, selection, version) }
    }
}
```

(`LauncherActivity.kt` ~436–441)

3. If `newTaskOptions` is null (options not yet loaded / pairing incomplete), `selection` is null → **silent no-op**: draft text is not cleared, companion never receives the prompt.

## Fix (shipped this heartbeat)

**Inputs:** Home composer with prompt text; `newTaskOptions` may be null; draft `version` may be null.  
**Outputs:** Send button disabled until `selection != null` **and** `composerState.version != null` (plus existing canSend/canEdit/non-blank/review/busy gates).  
**Algorithm:**
1. Require `selection != null` and `composerState.version != null` in Send `enabled` (`HomeScreen.kt`) — matches `LauncherActivity` submit guards.
2. Instrument tests: `sendStaysDisabledUntilNewTaskOptionsArrive`, `sendStaysDisabledWhenDraftVersionIsMissing`; enable-path uses `readyDraft` with a `DraftVersion`.
3. Existing enable-path tests pass `taskOptions()` so Send can enable.

**Verify (this session):**
```text
# red then green for version gate
./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=\
app.codexlauncher.launcher.home.HomeScreenTest#sendStaysDisabledWhenDraftVersionIsMissing
→ FAILED (assert not enabled) before product change; BUILD SUCCESSFUL after

./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=\
app.codexlauncher.launcher.home.HomeScreenTest#sendStaysDisabledWhenDraftVersionIsMissing,\
app.codexlauncher.launcher.home.HomeScreenTest#sendStaysDisabledUntilNewTaskOptionsArrive,\
app.codexlauncher.launcher.home.HomeScreenTest#selectedProjectEnablesTheComposerAction
→ BUILD SUCCESSFUL in 20s (Pixel 4B230DLAQ001Z5)

./gradlew :app:installDebug → BUILD SUCCESSFUL
```

Judge: Pass-with-warnings → version gate closed this heartbeat — `wave1-pixel-send-noop-fix-judge.md`.

## Conclusion

Not a Wave-1 adapter bug. Send used to look tappable while doing nothing; it now stays disabled until options **and** draft version are present. Smoke automation should still wait for options / re-pair if they never arrive. Owner Approves remain the main overnight wall for Group B live proofs.

**Pixel note (heartbeat):** device currently shows unpaired Pair screen — Auto→Open smokes blocked until owner re-pairs.

## How to reuse

Before adb Send: wait until Send is enabled (options + draft ready). Re-pair if options never arrive. Debug APK with this fix is installed on `4B230DLAQ001Z5`.
