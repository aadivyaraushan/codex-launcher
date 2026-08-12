# Phone-agent Home Send (Phase 5 UI pong)

Date: 2026-08-12

## Why

On a linked Pixel, Home shows the Phone agent row and a typed prompt, but
Send stays off. After Phase 8, Home Send only starts a *new* Mac task, which
needs project + model options the phone-runtime does not have. The phone
already has one persistent task (`phone-agent`) and an existing-turn path
(`StartExistingTurn` → gateway `chat.send`). Wire Home Send to that path.

## Flow

```
  typed prompt on Home
           |
           v
  +------------------+     Mac online with project
  | HomeSendRouter   |----------------------------> startNewTask (unchanged)
  +------------------+
           |
           | phone-runtime ready + phone-agent row present
           v
  submitHomePrompt (no model selection)
           |
           v
  start_turn { taskId: phone-agent }   // existing turn, not projectId
           |
           v
  turnproxy chat.send  session agent:main:main
```

Opening the row: phone-runtime advertises `task_transcripts` and answers
`ReadTranscript` from the last known user/agent lines (not full history).

## Inputs → Outputs → Steps

1. **Inputs** — Home prompt text; welcome caps; snapshot tasks; whether a
   Mac project is the active path.
2. **Outputs** — Send enabled without model chips when `phone-agent` is the
   route; tapping Send starts that task’s existing turn; tapping the row
   opens a thread if transcripts are advertised.
3. **Steps**
   1. Enable Send when routing to `phone-agent` (no `new_task_options`).
   2. Route that Send to `sendToTask` / `StartExistingTurn`, not `startNewTask`.
   3. Implement `ReadTranscript` on turnproxy + deferred source so welcome
      advertises `task_transcripts`.
   4. Leave Mac `new_task_options` → `startNewTask` unchanged.

## Out of scope

Beeper empty-subject; fake Mac catalogs; restoring Auto/Phone chips.
