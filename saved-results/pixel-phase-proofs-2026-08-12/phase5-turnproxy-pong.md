# Phase 5 — turn proxy pong (Pixel UI)

- Serial: `4B230DLAQ001Z5`
- Timestamp (UTC): captured overnight 2026-08-12
- Verdict: **FAIL / blocked**

## What we tried

1. Opened `app.codexlauncher/.LauncherActivity` with local-pair acked, runtime `taskCapable=true`, gateway up.
2. Composer already held `Reply with exactly: pong` (also retried with retyped prompt).
3. **Send prompt** control stayed `enabled=false` for the entire session.

## Why Send stays disabled

From `HomeScreen.kt`, Send requires:

- `state.canSend` (standalone ready) — **true**
- `composerState.text.isNotBlank()` — **true**
- `selection != null` — **false**

`selection` comes from `sessionUiState.newTaskOptions`. On connect the Android log shows:

`host task options accepted decision=hide_option_controls model_count=0 permission_mode_count=0`

Phone-runtime turnproxy (`companion/internal/phoneruntime/turnproxy`) implements `TaskSource` + `ExistingTaskSource` only — **not** `NewTaskOptionsSource` and **not** `TaskTranscriptSource`. So:

- Home cannot start a new task (no model/permission catalog → no selection → Send disabled).
- Tapping the Phone agent row cannot open TaskScreen (`openTask` requires `transcriptCapable`).

Health still reports `taskCapable:true` because the gateway websocket is connected; that flag alone does not unlock the composer.

## Artifacts

- `phase5-before-send-*.png` / `phase5-after-reply-*.png` — composer with pong text, Send disabled, status Replied
- `phase5-send-disabled-*.png` + xml — latest disabled Send hierarchy
- `phase5-send-blocked-logs-redacted.txt` — redacted Android logs
- `phase5-stuck-working-*.png` — earlier Working state reflected from a **CLI** `openclaw agent` run into the phone-agent row (not a UI composer send)

## Next unblock

Wire turnproxy (or a thin catalog adapter) so welcome advertises `new_task_options` with a safe default model/permission for the phone-agent lane, **or** route home Send on standalone to `start_turn` on the fixed `phone-agent` task without requiring new-task options; also advertise/implement `task_transcripts` so the Phone agent row can open.
