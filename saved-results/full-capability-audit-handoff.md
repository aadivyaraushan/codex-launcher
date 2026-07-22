# Codex Launcher full-capability audit handoff

**Date:** 2026-07-22
**Purpose:** Continue the physical Pixel 9 capability audit and finish the current uncommitted fixes without losing user state or overstating verification.

## Start here: what remains

Work in this existing isolated worktree:

```text
/Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/.context/worktrees/full-capability-audit
branch: codex/full-capability-audit
HEAD: 4071363e65c2c3821a53dc52f11efd468ff7988d
```

Do **not** reset, discard, or overwrite the dirty worktree. It currently contains
30 tracked file edits with 780 added and 49 removed lines, plus three new files:
the audit plan, 156-row capability inventory, and this handoff. None of these
audit changes is committed.

The next agent should complete these steps in order:

1. **Run the remaining Android device tests now, before re-pairing.** The Pixel
   is currently unlocked, connected, and showing the Codex Launcher pairing
   screen. Instrumentation already wiped the app's local pairing state, so this
   is the right time to run destructive local-state tests without wiping a new
   pairing afterward.
2. **Rerun the two suspicious UI cases individually while unlocked.** Do not
   dismiss them as suite noise until they pass alone or are fixed:
   - `UiScenarioActivityTest.onlineHomeExercisesOptionsAttachmentsDictationAndReview`
     did not find the attachment status message in the monolithic run.
   - `UiScenarioActivityTest.taskKeepsFollowUpActionsNextToTheRealKeyboard`
     measured keyboard top `1424` and action bottom `1438`. Determine whether
     this is a real 14 px overlap or a test coordinate mismatch by capturing the
     actual screen and window insets.
3. **Run all 23 instrumentation classes separately.** The previous one-process
   116-test run was invalid: local wipe/unpair tests changed shared app state,
   then the device locked and later tests reported `No compose hierarchies
   found`. Separate runner processes are the authoritative rerun.
4. **Fix any real failure test-first.** Reproduce it alone, keep the failing
   output, make the smallest underlying fix, search sibling uses, then rerun the
   focused case, its class, Android unit/lint/build checks, and the physical flow.
5. **Rebuild and reinstall the exact final APK and test APK.** Do this only after
   the last instrumentation change. Do not run local-wipe or unpair tests after
   the final pairing.
6. **Repair the stale pairing.** The phone's `paired_computer`, device identity,
   project, and last-seen files are currently zero bytes, while the companion
   still lists one old Pixel record:
   `android-2d4f4dff-d6fb-42f0-982d-e5ddcf43bdde`. Revoke only that stale record,
   generate a fresh five-minute link, and pair the Pixel through Fly. Revoking a
   device is destructive; if the continuation does not retain the user's prior
   authorization, ask once immediately before the revoke.
7. **Run the final live phone smoke pass after pairing.** Confirm online state,
   approved project, recent tasks, task opening, live follow-up delivery,
   transcript updates, intermediate activity, notification routing, All Apps,
   Appearance, Settings, and Home-role return. Mutate only disposable tasks whose
   titles start with `PHONE AUDIT`.
8. **Restore the user's exact phone state:**
   - Home draft: `calorie check. i ate `, including the trailing space.
   - Appearance: **Follow system**.
   - Default Home app and foreground screen: **Codex Launcher**.
   - `stay_on_while_plugged_in`: restore from temporary `7` to original `0`.
   - `screen_off_timeout`: leave at `1800000`.
   - Leave the app paired, online, and on Home.
9. **Finish the 156-row evidence ledger.** Replace stale `SOURCE` and `BLOCKED`
   values only where current physical, fault-injected, or automated evidence
   proves the row. Rewrite the stale blocker section, which still says the phone
   is locked and dictation/account permission is missing. Both permissions were
   already granted, and the phone is now unlocked.
10. **Update the audit plan and relay evidence.** Mark phases complete only when
    the final state above is observed. Keep the relay plan's broader completion
    claim tied to its existing local, deployed Fly, and physical-phone evidence.
11. **Run a fresh independent judge agent.** Give it the original user request,
    plan, full diff, 156-row inventory, test results, and phone evidence. Let it
    define its own completion standard first. Fix every material finding.
12. **Commit the finished work.** Verify `git diff --check`, relevant test suites,
    final device state, and `git diff origin/main...` before committing. Do not
    rename the branch or create a PR unless the user asks.

## Current authoritative state

Checked on 2026-07-22 at approximately 16:55 IST:

```text
Pixel serial: 4B230DLAQ001Z5
Model: Pixel 9
Android: 16
Lock state: unlocked
Foreground: app.codexlauncher/.LauncherActivity
Visible screen: Pair with your computer
stay_on_while_plugged_in: 7 (temporary; restore to 0)
screen_off_timeout: 1800000 (leave unchanged)
```

The phone app is unpaired because `LocalStateWiperInstrumentedTest` erased the
real app-local pairing files. The companion is healthy but its one Pixel record
is therefore stale:

```text
computer: MacBook Pro
configured: true
paired devices: 1
approved projects: 1
LaunchAgent: installed and running
doctor: 7 checks, 0 failures
Codex CLI: 0.144.1
relay: pin and registration secret match
phone door: reachable
historical last_error: listen_unavailable (doctor still marks the check OK)
```

The former autonomous goal was marked blocked after three consecutive turns in
which the phone was locked. The goal tool now reports no active goal. That is an
administrative state, not completion evidence: the user requested this handoff
so another agent can continue the unfinished checklist above.

The installed companion and a clean default build from the current source have
the same SHA-256:

```text
dcd0ba29d63374151b6cfa44dd3929f67e20c560ae574322236cd5dc2b3f8f80
```

## Commands for the remaining device suite

Never run `./gradlew connectedDebugAndroidTest` in this workspace. It uninstalls
or clears the app and has repeatedly destroyed the pairing and any restored draft.
Use assemble, install-over, and direct instrumentation instead:

```sh
cd /Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/.context/worktrees/full-capability-audit/android

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
  ./gradlew testDebugUnitTest lintDebug assembleDebug assembleRelease assembleDebugAndroidTest

adb -s 4B230DLAQ001Z5 install -r app/build/outputs/apk/debug/app-debug.apk
adb -s 4B230DLAQ001Z5 install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk

adb -s 4B230DLAQ001Z5 shell am instrument -w -r \
  -e class app.codexlauncher.debug.scenarios.UiScenarioActivityTest \
  app.codexlauncher.test/androidx.test.runner.AndroidJUnitRunner
```

Run each of these classes in its own `am instrument` command:

```text
app.codexlauncher.BootstrapInstrumentedTest
app.codexlauncher.LauncherActivityTest
app.codexlauncher.LauncherRoleTest
app.codexlauncher.accessibility.LauncherAccessibilityTest
app.codexlauncher.appearance.settings.AppearanceScreenTest
app.codexlauncher.connection.pairing.PairingScreenTest
app.codexlauncher.connection.stream.ApplicationSessionOwnershipTest
app.codexlauncher.connection.stream.CodexConnectionServiceTest
app.codexlauncher.debug.scenarios.UiScenarioActivityTest
app.codexlauncher.decision.DecisionSheetsTest
app.codexlauncher.launcher.apps.AppDrawerScreenTest
app.codexlauncher.launcher.apps.InstalledAppsRepositoryInstrumentedTest
app.codexlauncher.launcher.home.HomeScreenTest
app.codexlauncher.project.selection.ProjectSelectorTest
app.codexlauncher.storage.actions.ActionRecordStoreInstrumentedTest
app.codexlauncher.storage.connection.lastseen.LastConnectionStoreInstrumentedTest
app.codexlauncher.storage.drafts.EncryptedDraftStoreTest
app.codexlauncher.storage.secrets.PairingKeyStoreTest
app.codexlauncher.storage.wipe.LocalStateWiperInstrumentedTest
app.codexlauncher.storage.wipe.UnpairActivityTest
app.codexlauncher.task.control.PromptDictationContractTest
app.codexlauncher.task.control.TaskControlRestorationTest
app.codexlauncher.task.transcript.TaskScreenTest
```

Before each physical tap, make a fresh UI dump. Do not reuse coordinates from a
stale hierarchy:

```sh
adb -s 4B230DLAQ001Z5 shell uiautomator dump /sdcard/window.xml
adb -s 4B230DLAQ001Z5 shell cat /sdcard/window.xml
```

One service test intentionally skips its external airplane-mode case unless the
external state is provided. Record that as a named external-state skip, not an
app failure. `InstalledAppsRepositoryInstrumentedTest` previously saw SystemUI
instead of Settings only because the phone was locked. `TaskControlRestorationTest`
also failed only with no Compose hierarchy while locked; rerun both unlocked.

## Pairing and final-install sequence

The installed companion binary is here:

```text
/Users/aadivyar/Library/Application Support/codex-launcher/bin/codex-launcher
```

Use `devices` to recheck the exact stale ID, then revoke only that ID, create a
new pairing offer, and enter it directly on the phone. Never save or quote the
pairing link in the repository, handoff, logs, or final response.

```sh
companion="/Users/aadivyar/Library/Application Support/codex-launcher/bin/codex-launcher"
"$companion" devices
"$companion" revoke android-2d4f4dff-d6fb-42f0-982d-e5ddcf43bdde
"$companion" pair
```

The link is single-use and expires after five minutes. After pairing, select the
sole approved project, currently displayed as `Home folder`, and verify:

```sh
"$companion" status
"$companion" doctor
"$companion" devices
```

Expected: one new Pixel record, running service, one approved project, and all
seven doctor checks green.

If companion source changes again, rebuild and replace the installed companion
before final pairing, then prove the built and installed checksums match. Do not
launch a second live Codex app-server; the installed companion already follows
the user's live desktop tasks.

## User permissions and hard boundaries

- Approved ChatGPT login for short disposable audit tasks:
  `aadivya@fermi.ai`.
- Expected added cost for those tasks: $0 under the existing ChatGPT plan. Do
  not switch to an API key or another billed account.
- Only tasks titled `PHONE AUDIT …` may be renamed, archived, forked, stopped,
  queued, redirected, or otherwise changed. Every existing non-audit task is
  read-only.
- Audible dictation was approved. The real Google recognizer opened twice, but
  Mac `say` audio was not heard by the Pixel and Google showed `Didn't catch
  that`. Record this honestly as an acoustic-environment limitation; the app's
  dictation contract test passed.
- Fly paying account: `ssdear@gmail.com`; organization `personal`.
- The Fly deployment already exists and is healthy. Do not create, resize,
  redeploy, rotate, or delete paid Fly resources unless a new defect truly
  requires it and the user explicitly approves the exact paid/destructive step.
- The product now uses Fly only. Do not restore Tailscale code, setup flags, or
  fallback paths.
- Do not clear arbitrary phone or computer data. The required app-local wipe
  has already happened through the test.
- Preserve unrelated dirty work and do not use destructive Git commands.

## User-visible fixes currently in the dirty diff

### Android

- Home composer no longer applies duplicate IME padding; focus scrolls to the
  complete composer and keeps Attach/Send visible.
- A focused Home prompt is transient (`remember`, not `rememberSaveable`) and is
  capped at three lines while the keyboard is open.
- Follow-up/queue/redirect controls apply IME padding so the keyboard fits
  directly below the composer.
- All Apps search is transient, so reopening the drawer starts unfiltered.
- Pairing errors scroll into view while manual entry and the keyboard are open.
- Confirmed Fork results carry the new task ID through the protocol, bridge, and
  session view model; the phone opens the new fork after the fresh snapshot.
- New device tests cover these keyboard, drawer-restoration, service-cleanup,
  and fork behaviors.

### Companion

- Newly started tasks are merged provisionally into the phone snapshot while
  the shared task catalog catches up.
- Unknown live-task events trigger bounded catalog refresh/retry.
- Late activity cannot incorrectly move a terminal task back to Working unless
  the event explicitly starts a new turn.
- Older tasks outside the bounded catalog can be read directly through the app
  server for task state and transcripts.
- Empty agent-message entries render as `Agent response in progress` rather
  than disappearing.
- Existing-task controls and interrupt paths have structured, content-free
  diagnostic logs.
- A confirmed fork action includes `forkTaskId`; Go and Android protocol
  validators accept it only as a valid ID on confirmed results.

## Physical flows already verified on the Pixel

These were exercised on the current code before the destructive instrumentation
pass wiped local state:

- Manual pairing through the deployed Fly relay.
- Default Home behavior from another app.
- Opening real tasks without `Task unavailable`.
- Real-time follow-ups arriving in roughly 0–1 second.
- Live transcript updates, user/assistant messages, tool activity, and the
  placeholder for an in-progress agent response.
- Compact long task title with tap-to-expand behavior.
- Home prompt and controls visible directly above Gboard.
- Follow-up composer and controls fitting above Gboard.
- Far Home scrolling staying on Home.
- Full All Apps list, search/clear, launching 1SE, and Home return.
- Light and Dark appearance persistence across force-stop; Follow system was
  restored before the test wipe.
- Real Markdown document attach/remove and real photo attach/remove.
- Persistent generic connection notification.
- Generic reply notification while Settings was foregrounded; tapping opened
  the exact task and showed `PHONE NOTIFICATION VERIFIED`.
- Rename cancel, confirmed rename, Fork, automatic fork opening after the fix,
  archive cancel, and confirmed archive on disposable audit tasks.
- New task start, queued follow-up, and redirect of a working disposable task.

Useful captures include:

```text
/Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/.context/realtime-followup-frames/06-00-20-17.png
/Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/.context/final-apk-followup-keyboard-fixed-live.png
/Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/.context/home-far-scroll-stays-home.png
/Users/aadivyar/conductor/workspaces/codex-launcher/bozeman/.context/all-apps-unfiltered-final.png
.context/attachment-selected.png
.context/photo-attached.png
.context/appearance-light.png
.context/appearance-dark.png
.context/appearance-system-restored.png
.context/reply-notification-shade.png
.context/reply-notification-opened.png
.context/fork-auto-open-fixed-final.png
.context/redirect-accepted.png
```

## Test evidence already collected

- Android JVM: **275 tests passed**, zero failures/errors/skips after the new
  fork tests were added.
- Companion ordinary suite: **629 tests passed**, zero failures, five opt-in
  real-environment tests skipped. JSON output is in
  `.context/go-test-final.json`.
- Full current companion race suite: all **38 tested packages passed**.
- `go vet ./...`: passed.
- Protocol schema: **34 valid frames accepted, 38 invalid frames rejected**.
- Release contracts:
  - companion install smoke: 30 assertions passed;
  - repository baseline: 21 assertions passed;
  - public alpha release: 120 assertions passed.
- Live Fly read-only check on 2026-07-21:
  - account `ssdear@gmail.com`, owner `personal`;
  - one machine `d8d05eda5d3958`, started in `lax`;
  - one attached encrypted 1 GB volume;
  - raw TCP `443 -> 9000`, no handler;
  - TCP `8443 -> 8443`, PROXY protocol only;
  - `fly config validate`: green.

The broad instrumentation run is **not** green evidence. It started with a few
potentially real failures, then became invalid after shared app state changed and
the Pixel locked. Use the per-class rerun requested above.

## Privacy checks to repeat after final pairing

1. Clear logcat before the final smoke pass so old ADB diagnostic commands do
   not create false marker matches.
2. After restoring the draft, stream the app-private datastore through local
   `strings`/`rg` without putting the marker in an ADB shell command. The exact
   draft must not appear as plaintext because the draft store is encrypted.
3. Scan the captured local logcat and
   `$HOME/Library/Logs/CodexLauncher/companion.log` for known audit prompts,
   replies, task titles, and the restored draft. Expect no private content.
4. Inspect active notification text. Connection/reply notifications must remain
   generic and private.
5. Verify sensitive decision screens still use `FLAG_SECURE`; automated tests
   can prove the flag without taking a prohibited screenshot.
6. Confirm Android backup remains disabled and no task/transcript content is
   persisted locally.

Earlier checks found no `PHONE AUDIT`, `SYNC ONE`, `calorie check`, or known
follow-up text in companion logs or app-private files. One old logcat match came
from the literal marker embedded in an ADB grep command itself, which is why the
final scan must keep marker matching local.

## Documents to finish

- `planning/full-capability-pixel-audit-plan.md` — phases 5–8 are stale.
- `saved-results/codex-launcher-capability-inventory.md` — exactly 156 rows;
  many statuses and the blocker section are stale.
- `saved-results/relay-box-fly-deployment.md` — already contains the fresh Fly,
  race-suite, doctor, and checksum evidence added on 2026-07-21.
- `planning/relay-box-build-plan.md` — relay implementation and physical Fly
  proof are already recorded as complete; preserve its security and billing
  claims unless new evidence contradicts them.

The capability inventory should distinguish evidence types rather than call
everything a live physical pass:

- **Physical pass:** actual production flow driven on the Pixel.
- **Pixel scenario pass:** production screen and action exercised with injected
  safe/failure state.
- **Automated system pass:** protocol, storage, security, recovery, or host
  lifecycle behavior that is safer and stronger to verify through fault tests.
- **Named limitation:** only if a genuine external condition remains after all
  in-scope alternatives are exhausted.

## Completion checklist

- [ ] Two suspicious UI tests pass alone or are fixed with red-to-green proof.
- [ ] All 23 instrumentation classes have separate-run results while unlocked.
- [ ] Android unit, lint, debug, release, and test APK builds are green.
- [ ] Companion tests, race suite, vet, schema, and release checks remain green.
- [ ] Exact final APK is installed without clearing state afterward.
- [ ] Stale companion device is safely revoked and Pixel is freshly paired.
- [ ] Phone is online through Fly with one approved project.
- [ ] Final live task/open/sync/notification/Home/Apps/Appearance smoke passes.
- [ ] Exact draft `calorie check. i ate ` survives force-stop/reopen.
- [ ] Follow system, Codex Home, timeout `1800000`, and stay-awake `0` restored.
- [ ] Final local-storage, log, notification, backup, and secure-window checks pass.
- [ ] All 156 capability rows have current, honest evidence.
- [ ] Audit plan and saved results match the final observed state.
- [ ] Fresh independent judge reports no material gap.
- [ ] Final diff is reviewed, checked, and committed on the existing branch.

## Final-response requirement for the next agent

The workspace `AGENTS.md` requires the final response to include focused exact
code snippets copied from the final files for every non-trivial code or
configuration change. There are many behaviorally meaningful Android and Go
edits in this dirty diff. Group related snippets by file and explain briefly
what each exact block does; do not merely paraphrase the changes.
