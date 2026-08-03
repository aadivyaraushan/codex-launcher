# Pixel Home Send no-op fix — judge note

**Date:** 2026-08-02  
**Purpose:** Independent LLM-as-judge review of the Home Send silent no-op fix (`newTaskOptions == null` → Send disabled).  
**Worktree:** `phase0-notification-probe`

## Gate facts

1. **Callers:** None in code — human/heartbeat artifact under `saved-results/`.
2. **Peer:** `saved-results/wave1-pixel-send-noop-diagnosis.md` (diagnosis only; no prior judge note — Glob found only that file).
3. **Data I/O:** none (static judgment markdown).
4. **User instruction (verbatim excerpt):** Write a short judge note to `saved-results/wave1-pixel-send-noop-fix-judge.md` (date 2026-08-02, title, standard, verdict, findings).

## Standard (first principles)

A strong fix for this bug must:

1. Make the silent no-op impossible from the user-facing control: when selection cannot be formed (`newTaskOptions` null), Send must not be tappable.
2. Keep the happy path: with project + options + non-blank editable draft, Send enables and invokes `onSend` with a real selection.
3. Align the UI enable gate with what `LauncherActivity` actually requires to submit (at least `selection != null` for this failure mode).
4. Prove the disabled case with a focused UI test that sets `newTaskOptions = null` and asserts Send is not enabled; prove the enable path still works with options present.
5. Leave diagnosis accurate enough that overnight smokes know to wait for options / re-pair rather than treating adapters as broken.

## Verdict

**Pass-with-warnings**

## Findings

- **Fix matches the root cause.** `HomeScreen` Send `enabled` now requires `selection != null` alongside existing gates (`HomeScreen.kt:412–418`). With `newTaskOptions` null, `selection` is null (`HomeScreen.kt:250–251`), so Send stays off — the mash-and-no-companion path from the diagnosis is closed at the control.
- **Activity contract unchanged and still consistent for this bug.** `LauncherActivity` still only submits when `selection != null && version != null` (`LauncherActivity.kt:436–441`). The UI now refuses the null-selection case before the click; that is the right layer for the reported smoke failure.
- **TDD surface is present and pointed.** `sendStaysDisabledUntilNewTaskOptionsArrive` builds Home with project + ready draft + `newTaskOptions = null` and asserts Send not enabled (`HomeScreenTest.kt:150–161`). `selectedProjectEnablesTheComposerAction` passes `taskOptions()` and asserts enable + click delivers the prompt (`HomeScreenTest.kt:165–183`).
- **Diagnosis artifact is coherent.** `saved-results/wave1-pixel-send-noop-diagnosis.md` correctly maps Home → Activity no-op and records the enablement fix + claimed instrumented green on Pixel `4B230DLAQ001Z5`. This judge session did not re-run those instrumented tests.

## Gaps / remaining risks

- **Sibling silent drop still exists for draft `version == null`:** Activity still no-ops without UI feedback if Send is enabled but `draftComposerState.version` is null (`LauncherActivity.kt:437–441`). UI gates on `canEdit`, not version; if those can diverge, the same “looks sent / text stays” class of bug remains for a different cause.
- **Disabled test is static only:** it does not cover options arriving later and flipping Send on, nor does it assert `onSend` was never called (relies on Compose `enabled = false` not firing click).
- **No user-facing reason** when Send is disabled for missing options (automation/docs must infer “wait for model/permission controls”).

## One-line recommendation for next heartbeat

Treat Send-as-disabled-until-options as the smoke wait signal; optionally harden Activity’s `version == null` path the same way (disable or surface) if draft READY can ever lack a version.
