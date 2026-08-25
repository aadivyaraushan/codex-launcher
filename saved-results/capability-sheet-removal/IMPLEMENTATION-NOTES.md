# Capability sheet removal — implementation notes

**Date:** 2026-08-25. **Decision:** option A from `DECISION.md` (user chose "just remove the capability
sheet and dialogs"). **Result:** done, built green, verified on device (Pixel 9, `4B230DLAQ001Z5`).

## What was removed

The `CapabilitySheet` composable (an AlertDialog-based UI for the capability preview / running /
result / failed / question phases plus the unresolved-check banner) and only the wiring that fed that
UI:

- `capability/interaction/CapabilitySheet.kt` — deleted.
- `LauncherActivity.kt` — removed the `CapabilitySheet(...)` mount block and its two now-orphaned
  imports (`CapabilitySheet`, `HandOffActions`).
- `connection/runtime/LauncherSessionViewModel.kt` — removed the six sheet-only wrapper methods
  (`respondToCapability`, `answerCapabilityQuestion`, `disconnectCapability`, `retryCapabilityAction`,
  `dismissCapabilityResult`, `markCapabilityChecked`), which had no caller once the mount was gone.
- Debug scenarios: removed the six capability scenarios (`CAPABILITY_CONFIRM`, `_RUNNING`,
  `_RESULT_UNKNOWN`, `_FAILED`, `_QUESTION`, `_UNRESOLVED_CHECK`) from `UiScenarioActivity.kt`
  (the `CapabilitySheetLaterPhaseScenario` function and the `CAPABILITY_CONFIRM` branch of
  `ReplyConsentScenario`) and `ScenarioCatalog.kt`. The three `REPLY_*` consent scenarios stay.
- Tests: deleted `CapabilitySheetTest.kt`; removed the two capability tests from
  `UiScenarioActivityTest.kt`; removed the six capability wireNames from `ScenarioCatalogTest.kt`.
- Cleaned five stale doc comments that named the deleted `CapabilitySheet` as a live sibling
  (`CapabilityInteraction.kt`, `HomeScreen.kt`, `LauncherDialogs.kt`, `TaskActionsMenu.kt`,
  `TaskControls.kt`).

## What was deliberately kept (out of "just the sheet" scope)

`capabilityState` is not sheet-only. `LauncherActivity` still reads `capabilityState.destination`
(the Home on-phone/on-computer toggle), `.busy`, and `.message` (the Home "Codex services
unavailable" feedback). So the whole upstream stays: `CapabilityInteraction` controller,
`LauncherSessionViewModel`'s controller plumbing (`request`, `acceptPreview`, `acceptActionResult`,
`acceptResult`, `sessionLost`, `setDestination`, `restoreUnresolvedCheck`), `AdapterLabel`
(used by the controller), `HandOffActions` (its `draftFromPreviewLines` is used by the controller),
`CapabilityOutcome`, the `ProtocolCodec` `CAPABILITY_*` enum members (already inert, documented), and
`UnresolvedCapabilityStore` + its wipe wiring.

## Known follow-ups (not done here, flagged for a decision)

- **On-phone send is a live dead-end.** In on-phone mode the default Home "Send" still routes to
  `CapabilityInteraction.request()`, whose `capability_request` envelope self-validates through the
  codec that rejects it, so it always falls back to "Codex services unavailable." Removing the sheet
  did not touch this. The real fix is wiring on-phone sends to the OpenClaw agent-tool path — a
  separate piece needing backend work and a money/scope decision.
- **Orphaned unresolved-check store.** Nothing live arms `unresolvedCheck` now, and the only renderer
  (the sheet banner) is gone, so a value persisted by a pre-Phase-8 build would sit unused (no banner,
  no way to clear except a full wipe). Deleting `UnresolvedCapabilityStore` + its wipe step is a
  clean follow-up but touches `LocalStateOwner`/`LocalStateWiper`, beyond "just the sheet."

## Build + verification

- Build gate green: `:app:assembleDebug :app:testDebugUnitTest` BUILD SUCCESSFUL (unit tests ran).
  Also compiled instrumented tests: `:app:compileDebugAndroidTestKotlin` green.
- APK sha256 `17cd9f95f0d098693772039205103634f0b8259b20f823985008d621756fb17a`, installed on the Pixel 9.
- On device: app launches, Home renders, no crash (`topResumedActivity=LauncherActivity`, empty crash
  buffer). The on-phone/on-computer toggle is present and active (proves `capabilityState.destination`
  survived). The removed `capability_confirm` scenario is now rejected
  (`scenario_known=false, decision=finish_activity`); the surviving `reply_stop_offer` scenario still
  renders ("Handed a reply to WhatsApp for Maya").

## Undo

Pre-removal backup of every touched/deleted file (85 files) is at
`saved-results/capability-sheet-removal/pre-removal-backup/main-and-debug-and-tests.tar`. This repo is
not under git, so that tar is the only restore path.
