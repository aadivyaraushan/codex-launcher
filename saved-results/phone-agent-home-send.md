# Phone-agent Home Send (Phase 5 UI pong)

Date: 2026-08-12  
Branch: `cursor/phone-agent-home-send-141c`  
Base: `worktree-phase2-tool-bridge` (includes PR #9 SafeDisplay/SafeLastMessage; this change does not redo that sanitize work)

## What this is for

Pixel 9 (serial 4B230DLAQ001Z5) could see the Phone agent row and type
`Say only: pong`, but Home Send stayed disabled. Gateway/CLI turns updated
the preview; the UI could not start a turn. This records the smallest
client + turnproxy wiring so Home Send uses the existing phone-agent turn
instead of Mac `startNewTask`.

## Result

Home Send, with phone-runtime linked and the `phone-agent` row present,
starts `start_turn { taskId: phone-agent }` (existing turn → gateway
`chat.send`). Mac `new_task_options` → `startNewTask` is unchanged.
turnproxy now implements `ReadTranscript`, so welcome can advertise
`task_transcripts` and tapping the row can open a thread.

## Why

After Phase 8, Home required `new_task_options` selection and always called
`startNewTask`. Phone-runtime implements `TaskSource` + `ExistingTaskSource`
only: welcome has `desktop_tasks`, not `new_task_options` or
`task_transcripts`. The persistent task is `phone-agent`.

## How to reuse / prove

Go (from repo root):

```
go test -count=1 ./companion/internal/phoneruntime/turnproxy/ ./companion/internal/phoneruntime/
```

Verified 2026-08-12: both packages `ok`.

Android (from `android/`, needs SDK):

```
./gradlew :app:testDebugUnitTest --tests app.codexlauncher.launcher.home.HomeSendRouterTest --tests app.codexlauncher.launcher.home.HomeUiStateTest --tests app.codexlauncher.connection.runtime.LauncherSessionViewModelTest
```

Verified 2026-08-12: `BUILD SUCCESSFUL`.

On device: Operator Home, phone-runtime linked, Phone agent row visible,
type `Say only: pong`, tap Send. Expect Home `lastMessage` to update.
Tapping the row should open a thread if the runtime is this tip.

## Judge

Independent judge (2026-08-12): **PASS-WITH-WARNINGS**. No must-fix on the
Home → existing-turn path. Warnings: no on-device pong proof in this run;
Activity wiring is covered by router + ViewModel tests, not an Activity
test; `task_transcripts` on welcome is proven by the TaskTranscriptSource
type assertion, not a full phone-runtime hello frame.

- `selection != null` / `Choose model options before sending`: only the Mac
  `StartComputerTask` path still requires selection.
- `submitHomePrompt`: now branches to existing-turn when `phone-agent` is in
  the snapshot and selection is null.
- Other `TaskSource`s: Mac catalog already has `ReadTranscript`; only
  turnproxy was missing it.
