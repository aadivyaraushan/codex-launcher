# On-phone Send → OpenClaw wiring — implementation notes

**Date:** 2026-08-25. **Scope approved by user:** full path through P3; user owns P2 attempt;
spending authorized on the flat-rate ChatGPT subscription, account `ssdear@gmail.com`. Updated as I go.

See [PLAN.md](PLAN.md) for the diagram and phase plan. This file records what was actually built,
the decisions, and the evidence.

## The recon that reshaped the plan (all verified against the live device + repo)

- **The deployed phone binary is ahead of this repo.** The running `operator-phone-runtime` is invoked
  with `-gateway-url ws://127.0.0.1:18789 -gateway-token-path … -beeper-base-url …`; the repo's
  `main.go` had none of those flags. So the OpenClaw turn-proxy path is already live on the phone, and
  the repo just needed to catch up (that became P1).
- **The phone runtime models exactly one canonical task, id `"phone-agent"`** (hardcoded constant
  `gateapproval.go:18`, set into `turnproxy.Config.TaskID` on every gateway dial). Both `chat.send`
  (start_turn) and `sessions.steer` (steer_turn) reject any other task id. (Recon subagent, Go source.)
- **`desktop_tasks` is advertised exactly when `GatewayURL` is set** (`handler.go:1909-1911`,
  `taskCapable = taskSource != nil`, non-nil only on the gateway-wired branch of `Open()`). The live
  binary has it, so on-phone `taskControlsAvailable == true` and `taskControlViewModel` is constructed.
- **On-phone there is no project and no `new_task_options`** (live WELCOME/SNAPSHOT:
  `project_count=0`, `decision=hide_option_controls` ⇒ `newTaskOptions == null`). So
  `startNewTask()` cannot run on-phone (it early-returns `Unavailable` without both). The send must go
  to the existing phone-agent task via `sendToTask()`.
- **Open item (not blocking):** repo code structurally returns exactly one task, yet the live snapshot
  logged `task_count=2` — because the deployed binary is ahead of the repo. The P0 design is robust to
  either count (it reads the id from the task list Home renders, see below), so this did not need to be
  settled to ship P0.

## P0 — Kotlin re-route (DONE, test-first)

**The one broken case.** The existing test `a home prompt starts a task even when the companion
advertises capability actions` proves the on-*computer* path (`forceCapability=false`) already falls
through the dead `capabilityController.request()` to `startNewTask` and emits `start_turn`. The only
dead-end was `forceCapability=true` (on-phone): it hit `request()` (self-rejecting since Phase 8) then
the `if (forceCapability …)` "kept local" early-return, never sending anything — and `startNewTask`
wouldn't work on-phone anyway.

**The change (surgical, one file).** In `LauncherSessionViewModel.submitHomePrompt`, the
`forceCapability=true` branch now calls a new private `submitHomePromptToPhoneAgent(prompt, draftVersion)`
and returns. That method:
- resolves the task id from `mutableState.value.snapshot?.tasks?.sortedForHome()?.firstOrNull()?.id`
  — the same list, in the same order, that Home already renders; null ⇒ log + return (send nothing,
  keep the draft),
- sends via the existing `queueTaskFollowUp(taskId, prompt)` (QUEUE ⇒ `start_turn`, reuses attachment
  prep + queue-state publish),
- clears the composer draft (`clearConfirmedDraft(draftVersion)`) only on `Accepted`/`Queued`.

I also dropped the now-unreachable `forceCapability ||` term from the fallback guard (it can no longer
reach there), so the reader isn't left tracing a dead condition.

**Decisions (poteto principles named):**
- *sendToTask, not startNewTask* — Fix Root Causes: startNewTask is provably `Unavailable` on-phone
  (no project/options).
- *taskId from `sortedForHome().firstOrNull()`, not the hardcoded `"phone-agent"`* — tracks the runtime
  and stays correct whether the snapshot has 1 or 2 tasks; matches what the user sees at the top of Home.
- *QUEUE (start_turn), not REDIRECT (steer_turn)* — Experience First: a fresh Home message must not
  hijack a possibly-running turn; steer is only offered in-UI for an actively-watched WORKING task.
- *Reuse `queueTaskFollowUp`* — Laziness Protocol: one call gets attachments, send, and queue-state.
- *Minimal blast radius* — the Activity `CapabilityOnPhone` branch, `HomeSendRouter`, and the
  on-computer path are untouched; the ViewModel's public `submitHomePrompt(forceCapability=true)`
  contract is unchanged.

**Refinement to the PLAN's "delete CapabilityInteraction".** Not done, and it should not be done
wholesale: `capabilityController` still owns the live Home AUTO/COMPUTER destination toggle
(`setDestination`, `capabilityInteraction.value.destination`) that `HomeSendRouter` reads. Only the
capability *send-flow* (`request()`/PREVIEW/EXECUTING) is dead. Deleting just those, while keeping the
destination state, is a separate cleanup left for after P0 is proven on-device.

**Tests (test-first, `LauncherSessionViewModelTest`):**
- `anOnPhoneHomePromptQueuesAStartTurnToThePhoneAgentTaskAndClearsTheDraft` — on-phone WELCOME
  (`desktop_tasks`, no options), one task; asserts a `start_turn` with `taskId=thread-1` is sent and a
  durable `confirmed/queued` result clears the draft.
- `anOnPhoneHomePromptWithNoPhoneTaskSendsNothingAndKeepsTheDraft` — empty task list; asserts no
  `start_turn` sent and draft untouched.
- RED was by construction (current code never calls `sendAction` for `forceCapability`, so the test's
  no-timeout `awaitType("action")` hangs — verified the current branch cannot emit an action). After the
  fix: whole class GREEN (`BUILD SUCCESSFUL`, both new tests in the result XML).

## P1 — Go gateway flags in the repo (DONE)

Added `-gateway-url`, `-gateway-token-path`, `-beeper-base-url` to
`companion/cmd/operator-phone-runtime/main.go` and threaded them into the `phoneruntime.Config{}` literal.
`Config` already had the three fields; `validate()` already enforces both-set-or-both-empty for
gateway URL+token and requires GatewayURL for BeeperBaseURL, so omitting the flags preserves the
prior no-gateway default. `go build` and `go vet` clean. No behavior change on the device (it already
runs a gateway-wired binary); this makes a repo rebuild reproduce the deployed artifact. `main.go` has
no unit test (matches its prior state); Config validation is covered by existing `internal/phoneruntime`
tests.

## P2 — Gateway persistence (DONE; reboot-persistence proven on device)

Live recon: `operator-runtime-watchdog` (pid 17058) is re-parented to **init (PPID=1)**, supervising
`proot → openclaw-gateway → node` plus `operator-phone-runtime`. It survives SSH disconnect — the
2026-08-12 "dies with the SSH session" caveat is outdated. The PLAN's P2 goal ("run reliably, not a
manual SSH session") is therefore met.

**Reboot persistence — the earlier "not done / needs Termux:Boot APK" note was wrong** (2026-08-25,
re-checked on device by driving the Termux app directly on-screen — no SSH needed, since the Play
build is a non-debuggable release so `run-as`/`adb` can't reach Termux's home). Corrected facts:

- The phone's Termux is the **Google Play build** (`versionName=googleplay.2026.06.21`, installer
  `com.android.vending`, signer `61fa5427`). Play-build Termux **has Termux:Boot merged into the main
  app** since v2024.10.24 — there is no separate `com.termux.boot` to install, and none could be
  sideloaded anyway (add-ons must share Termux's signature; Termux:Boot isn't on Play — the listing
  returns "Item not found"). Source: termux maintainers, corroborated on-device.
- Device proof boot support is live and enabled: `dumpsys package com.termux` shows
  `com.termux/.app.TermuxBootReceiver` registered for `BOOT_COMPLETED` and
  `android.permission.RECEIVE_BOOT_COMPLETED: granted=true`.
- The boot script **was already in place since Aug 12**: `~/.termux/boot/10-operator-runtime`
  (`/data/data/com.termux/files/home/.termux/boot/`, executable, 371 bytes) — a byte-for-byte match
  with the repo canonical `companion/internal/phoneruntime/supervisor/10-operator-runtime.sh`. It
  acquires the Termux wake-lock and `exec`s `$PREFIX/libexec/operator-runtime-watchdog`
  (`PREFIX=/data/data/com.termux/files/usr`). So reboot-persistence was configured on Aug 12; the
  earlier analysis simply didn't know Termux:Boot ships inside the Play build.
- Boot-script path correction: the repo comment said `~/.config/termux/boot/`; the real, verified path
  the merged receiver reads is `~/.termux/boot/`. The on-device script now carries the corrected
  comment (364 bytes; content otherwise identical — same `exec`).

**Reboot test — PASSED (2026-08-25 ~18:04, proven on device).** Procedure: captured a baseline of the
running stack, `adb reboot`, waited for `sys.boot_completed=1` (device sat at keyguard,
`deviceLocked=1 strongAuthRequired=0x1` — first-unlock-after-boot), the user unlocked with their PIN
(`deviceLocked=0` at 18:03:33), settled 35s, then re-ran the process check from a fresh Termux session.
The **entire supervised stack came back on its own** — boot script → watchdog → `proot … /usr/bin/
runsvdir /etc/operator/services` → `runsv phone-runtime` + `runsv openclaw-gateway` → the openclaw
gateway, `operator-phone-runtime -listen 127.0.0.1:9443 -gateway-url ws://127.0.0.1:18789`, the svlogd
loggers, and `openclaw-models` (ppid=1, detached). **All pids were new**, proving a real restart, not
survivors:

| process              | before reboot        | after reboot        |
|----------------------|----------------------|---------------------|
| runsv supervisor set | 17070 / 17071 / 17074| 5813 / 5842 / 5844  |
| operator-phone-runtime (9443 → gw 18789) | 17080 | 5904 |
| openclaw gateway     | 17078                | 5868                |
| openclaw-models      | (not captured)       | 6689 (ppid=1)       |

The `codex app-server` node child was absent post-reboot — expected: the gateway spawns it lazily on
the first turn, so the idle post-boot state is gateway + models ready. Loopback-port probing showed
nothing only because `ss`/`netstat` live inside proot, not the Termux base env; the process args carry
the real `-listen 127.0.0.1:9443` / `-gateway-url ws://127.0.0.1:18789` bindings.

Caveat worth keeping: reboot-persistence needs the **first unlock** after a power-cycle (CE storage);
until the user unlocks, the on-phone stack stays down. That is inherent to non-rooted Termux:Boot, not
a defect. A functional paid send was *not* re-run post-reboot (P3 already proved end-to-end; the reboot
test's job was "does the wired stack come back," which it did — no reason to spend more quota).

How it was done (no SSH, no APK): the Play-build Termux can't be reached by `adb`/`run-as`, so the boot
script was placed by **driving the Termux app on-screen** (adb `input`), reading each step from
screenshots. Files were staged via `adb push` to `/data/local/tmp` (world-readable) and `cp`'d in from
the Termux terminal; all temp files were removed afterward.

## P3 attempt 1 — surfaced a second, deeper blocker (storage layer)

Drove a real on-phone Home Send from the UI (device unlocked, user approved the paid run). The P0
re-route worked exactly as designed — logcat showed `home send routed decision=capabilityonphone`
then my `home prompt sent to phone agent task ... task_id=phone-agent` — but the send did **not**
reach Codex. It was blocked one layer down:

```
home send routed decision=capabilityonphone
project session closed decision=fail_pending_actions      (send triggers a warm resync)
… full reconnect, fresh snapshot applied …
action preparation completed decision=block_send_storage_unavailable
home prompt sent to phone agent task outcome=Unavailable
```

**Root cause (confirmed by device evidence, not inference).** The action journal could not persist the
`start_turn` record, so it failed closed. `StoredActionJournal.prepare` → `ActionRecordStore.save` →
`guardedWrite` → `LocalStateWriteGate.withPairedWrite`, which **requires gate mode `PAIRED`**. On this
standalone phone the gate opens as `STANDALONE`:

```
14:04:29 local-state-gate: startup write gate opened generation=1 output_shape=standalone
```

`withPairedWrite` in a non-PAIRED mode returns `Blocked` **silently** (the `capture(PAIRED)` returns
null before running the block — no reporter log, which is exactly what the capture showed: zero
`action-record` logs, only the journal's `block_send_storage_unavailable`). So on a standalone phone
**every** on-phone send was blocked at the journal, regardless of the P0 re-route. The connection-layer
`input_shape=paired_computer` / `local_pair_acked=true` is a different concept (the loopback runtime
pairing), not the write gate's Mac-pairing mode — a red herring.

The codebase already knew this shape: `ResumeCursorStore.record` carries the comment *"Unpaired
phone-runtime sessions open the gate as STANDALONE … capability actions can['t] send (seen on Pixel
dogfood)"* and falls back to `withStandaloneWrite`. `EncryptedDraftStore` does the same. **`ActionRecordStore`
was the one gated store on the send path that never got that fallback.**

**The fix (test-first, one method).** `ActionRecordStore.guardedWrite` now mirrors `ResumeCursorStore`:
try `withPairedWrite`, and on `Blocked` fall back to `withStandaloneWrite`. `WIPING` and
`STARTUP_BLOCKED` are neither PAIRED nor STANDALONE, so both writes stay blocked there — wipe-safety and
startup-safety are preserved. Tests in `ActionRecordStoreTest`:
- `savesActionRecordsWhenTheGateIsStandaloneNotPaired` — gate opened `STANDALONE`; `save` must succeed.
  RED before the fix (`AssertionError` on the `save` assertion), GREEN after.
- `blocksSavesUntilTheGateOpensAfterStartup` — gate left `STARTUP_BLOCKED`; `save` must still fail
  closed (guards the fallback against being too permissive). Green both before and after.

**Same-bug-elsewhere audit** (pattern = a gated store using `withPairedWrite` with no STANDALONE
fallback): `EncryptedDraftStore` and `ResumeCursorStore` already have the fallback (that is why the
composer draft survived the blocked send). `ActionRecordStore` fixed here. `LastConnectionStore`
(last-seen metadata) and `ProjectSelectionStore` (never invoked on-phone — `project_count=0`) share the
same latent pattern but are **not on the send path**; flagged as follow-ups, not fixed, to keep this
change scoped to the send blocker.

**Reconnect-on-send observation.** Each send triggers a warm resync (`project session closed` →
reconnect → fresh snapshot). In both traces the resync fully settled *before* `journal.prepare` ran, so
it is not what blocked the write and the post-fix send writes to a live socket. Noted for follow-up but
not a P3 blocker.

## P3 — one paid end-to-end run (DONE, proven on device)

After the storage fix, drove one real on-phone Home Send from the UI (user approved the paid run;
account re-confirmed as the flat-rate ChatGPT subscription `ssdear@gmail.com` immediately before
firing). It worked end to end:

- Journal lifecycle all green (was `block_send_storage_unavailable`):
  `action preparation … allow_send_preflight` → `action send boundary … allow_socket_write` →
  `action confirmation … result_code=accepted output_shape=durable_terminal_metadata` →
  `home prompt sent to phone agent task outcome=Accepted task_id=phone-agent`.
- The prompt appears in the **Phone agent** thread as a user message, the composer draft cleared
  (`Accepted` ⇒ `clearConfirmedDraft`), and the OpenClaw/Codex agent streamed a real, on-topic reply
  into the thread (a substantive answer distinguishing test-time training from test-time compute, with
  a reading-list plan). Screenshots: `p3-04-thread.png`, `p3-05-final.png`.

This is the full chain the plan set out to prove: on-phone Home Send → `CapabilityOnPhone` → the P0
re-route → `queueTaskFollowUp(phone-agent, start_turn)` → journal persisted → socket → **Accepted** →
OpenClaw gateway → Codex on the ChatGPT subscription → reply streamed back into the TaskScreen thread.
One Codex turn spent on `ssdear@gmail.com` (flat-rate; no per-token billing).

## Verification evidence

- P0 unit tests: `LauncherSessionViewModelTest` GREEN, both new tests present in the result XML.
- Full Android gate build (`assembleDebug` + `testDebugUnitTest` + `compileDebugAndroidTestKotlin`):
  `BUILD SUCCESSFUL`, zero failing unit-test files.
- On-device install + clean launch/reconnect: APK sha256
  `720cd24eebb033ee207704156e1fddfd5e0bec231ce557f66be8ad4458aa5c05` installed on the Pixel 9; fresh
  process reconnected through `companion socket authenticated → host task options accepted → fresh
  companion snapshot applied (project_count=0, task_count=2)`; `LauncherActivity` is the resumed
  activity; crash buffer empty. This confirms the live binary presents `task_count=2` (ahead of repo)
  and my re-route reads exactly that list. **Not done (secure PIN lock, not bypassed):** an actual UI
  Send — that on-device behavior is covered by the unit tests, and belongs to P3.
- Go: `go build` + `go vet` on `cmd/operator-phone-runtime` clean; cmd package has no tests.

## Storage fix + P3 verification evidence

- `ActionRecordStoreTest`: RED then GREEN. `savesActionRecordsWhenTheGateIsStandaloneNotPaired` failed
  before the fix (`AssertionError` on the `save`), passes after; `blocksSavesUntilTheGateOpensAfterStartup`
  green throughout. Full class green.
- Full Android gate (`assembleDebug` + `testDebugUnitTest` + `compileDebugAndroidTestKotlin`):
  `BUILD SUCCESSFUL`. New APK sha256 `3050a4645927b10753b1a7c5ae2ef45a7e11910fbdecee6d0cb72c80d09bc546`,
  installed with `-r` (state preserved), clean relaunch and reconnect.
- On-device paid send: journal `allow_send_preflight` → `allow_socket_write` →
  `result_code=accepted`, `outcome=Accepted`; agent reply streamed into the Phone agent thread. Logs in
  `p3-paid-logcat.txt`; thread screenshots `p3-04-thread.png`, `p3-05-final.png`; startup gate-mode
  evidence `p3-startup-logcat.txt` (`output_shape=standalone`).

## Follow-ups (found, not fixed — out of the send-path scope)

- `LastConnectionStore.record` and `ProjectSelectionStore` save use `withPairedWrite` with no STANDALONE
  fallback (same latent pattern as the fixed `ActionRecordStore`). Neither is on the on-phone send path
  (last-connection is last-seen metadata; project selection never fires on-phone with `project_count=0`),
  so they do not block anything today. Worth the same one-line fallback for consistency later.
- Every on-phone send triggers a warm resync (`project session closed` → reconnect). It settled before
  the write in every trace and the paid send did not resync at all, so it is cosmetic/curiosity, not a
  bug — but worth understanding before the P0b cleanup.

## Backups (no git in this repo)

`saved-results/on-phone-openclaw-wiring/p0-backup/` holds the pre-edit copies of
`LauncherSessionViewModel.kt`, `LauncherSessionViewModelTest.kt`, and `main.go`.
`saved-results/on-phone-openclaw-wiring/p3-storage-fix-backup/` holds the pre-edit copies of
`ActionRecordStore.kt` and `ActionRecordStoreTest.kt`.
