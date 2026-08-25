# unresolvedCheck teardown — implementation notes

**Date:** 2026-08-25. **Trigger:** user asked me to do the flagged follow-up "the unresolvedCheck
DataStore is now orphaned." **Result:** the whole mechanism removed, build green, verified on device.

## The finding that made this more than tidy-up

I called this "orphaned but harmless" in the sheet-removal report. Reading the code proved the
"harmless" part wrong. The mechanism is a persisted cross-restart block:

- `unverifiedOutcome()` (unreachable in production — you can't reach `EXECUTING` since the
  `capability_request` encode self-rejects) and, critically, `applyRestoredCheck()` were the two arming
  paths. `applyRestoredCheck()` runs at every startup from `restoreUnresolvedCheck()` and reads the
  persisted DataStore.
- `request()` refused every on-phone send while the check was armed, re-showing "Couldn't confirm that
  happened" as the Home composer message.
- The only UI that could clear it was the deleted sheet's "I checked" button (`markChecked()`).

So an upgraded pre-Phase-8 user who had a check persisted would be **soft-locked out of on-phone send,
with a message they can't dismiss**. Narrow (needs a pre-Phase-8 armed-and-never-cleared value) but real.
That made removal the right call, not just cleanup.

## What was removed

- Deleted `storage/capability/unresolved/UnresolvedCapabilityStore.kt` (interface + DataStore impl +
  reporter) and its unit test `UnresolvedCapabilityDataStoreTest.kt`; removed both now-empty `unresolved/`
  dirs (and the empty `storage/capability/` parents).
- `CapabilityInteraction.kt`: removed the `unresolvedCheck` state field, the `unresolvedStore` ctor param,
  `pendingCheck`, `restoreMutex`/`restored`, `restoreUnresolvedCheck()`, `ensureRestored()`,
  `applyRestoredCheck()`, `markChecked()`, the `request()` block-check + its `ensureRestored()` call, the
  `unverifiedOutcome()` arming lines, the `unreadableWarning` const, and the `Mutex`/`withLock`/
  `UnresolvedCapability*` imports. `publish()` is now `mutableState.value = next`. Stale doc comments in
  `dismissTerminal`, `sessionLost`, and `unverifiedOutcome` reworded.
- `LauncherSessionViewModel.kt`: removed the `unresolvedCapabilityChecks` param, its import, the
  `unresolvedStore = …` controller arg, and the `init { … restoreUnresolvedCheck() }` block.
- `LauncherApplication.kt`: removed the `unresolvedCapabilityChecks = …` ViewModel arg.
- `LocalStateOwner.kt`: removed the `capabilityUnresolvedChecks` field, its two imports, and the wiper arg.
- `LocalStateWiper.kt`: removed `WipeStep.CAPABILITY_UNRESOLVED_CHECK` (enum + `deletions` list), the
  `fromStores` param, the map entry, and the import.
- Tests: `WipeDoesNotClearTheStopListTest` pinned list "ten"→"nine" stores; `LocalStateWiperInstrumentedTest`
  dropped the capability-store setup/assertions/imports and the reset line; comment-only fixes in
  `DurableStopsTest` and `CapabilityOutcomeUnverifiedTest`.

## What was kept (deliberately)

`unverifiedOutcome()` stays — it still builds the UNVERIFIED `CapabilityOutcome` for `sessionLost()` and
`acceptActionResult()`'s `outcome_unknown` branch. Those methods and the whole capability
ROUTING/PREVIEW/EXECUTING flow are unreachable in production today (that's task 1's territory), so removing
them was out of scope for "the orphaned store." Only the persisted-block sub-mechanism came out here.

## One accepted consequence

An unpair/wipe no longer clears the on-disk `unresolved_capability_check` prefs file. That file only ever
held the fixed non-PII string "Couldn't confirm that happened" (see the `unverifiedOutcome` comment on the
fixed wording), so this is not a privacy regression. After the teardown nothing reads the file, so a value
left by a pre-Phase-8 build sits inert instead of soft-locking. Not worth a one-time delete migration.

## Build + verification

- `:app:assembleDebug :app:testDebugUnitTest :app:compileDebugAndroidTestKotlin` → BUILD SUCCESSFUL in
  19s. Unit tests ran (the edited `WipeDoesNotClearTheStopListTest` among them). Instrumented tests compile.
- APK sha256 `b375188aa2d55bb3453a56e5ba23e4d067d5ae7ffd5d294e78e9883aed1a6d8b`, installed on the Pixel 9.
- On device: LauncherActivity is the resumed + focused activity after launch; crash buffer empty. The
  modified startup path (controller, ViewModel with the init removed, `LocalStateOwner`/`LocalStateWiper`
  wiring) runs without crashing.
- **Not done:** a pixel screenshot of Home. The device auto-engaged a secure lock screen mid-session; I did
  not bypass the PIN. Destination-toggle survival rests on the code (unchanged `capabilityState.destination`
  read) plus the clean resume, not a screenshot. The instrumented wipe test compiles but was not executed on
  device (no `connectedCheck` run).

## Undo

Backup of every touched/deleted file (9 files) at
`saved-results/capability-sheet-removal/unresolved-check-teardown-backup/`. No git in this repo.
