# Phone live reasoning + new home chat

Date: 2026-08-13

What this is for: Operator phone-agent UX on `worktree-phase2-tool-bridge`. Live reasoning while the model thinks, and Home compose opening a new conversation instead of appending to `phone-agent`.

## Result

Live thinking is ingested from OpenClaw `event:"agent"` `stream:"thinking"` (and chat-message thinking content). Each Home “What do you want done?” send allocates a new `phone-chat-*` task / `agent:main:phone-*` session. Beeper inbound stays on `phone-agent` / `agent:main:main`.

`OPERATOR_ALLOW_SOFTWARE_ATTEST` was not changed (still off unless the AVD env/flag is set).

## UX before / after

| Surface | Before | After |
| --- | --- | --- |
| Thinking | Ignored. Phone saw Working on first chat delta, then the reply. | Reasoning row + Working summary update as thinking text grows. |
| Home compose | `start_turn` on `phone-agent` continued the one OpenClaw session. | `start_turn` on virtual `phone-home` opens `phone-chat-<id>` and a new session. |
| Follow-up in an open thread | Same session (unchanged). | Same session (unchanged). |
| Beeper / inbound | `phone-agent` / `agent:main:main`. | Unchanged. |

## How to verify on Pixel

1. Do **not** set `OPERATOR_ALLOW_SOFTWARE_ATTEST` on a physical Pixel.
2. Thinking: send a thinking-heavy prompt in a phone chat. While the status is Working, Reasoning rows should grow (safe display text only; newlines collapsed). Then the reply lands.
3. Home: type a new prompt on Operator Home “What do you want done?”. The UI should open a **new** thread, not append to Phone agent. A second Home send should open another new thread. Follow-up inside an open thread should stay on that thread.
4. Inbound / Beeper: a triggered message should still land on the stable Phone agent row.

## Tests run (this session)

```
go test ./companion/internal/phoneruntime/turnproxy/
go test ./companion/internal/promptqueue/
go test ./companion/internal/app/mobilesession/
go test ./companion/internal/phoneruntime/
```

All four passed.

Android unit tests were not run here: Gradle could not find an Android SDK (`ANDROID_HOME` / `local.properties` missing).

## Reproduce / reuse

- Compose id: `phone-home` (never listed as a home row).
- New chats: `phone-chat-<hex>` with gateway session `agent:main:phone-<hex>`.
- Inbound: `phone-agent` + `agent:main:main`.
- Connect handshake sends `caps: ["thinking-events"]` on protocol v4.
- User `chat.send` (Home and in-thread) sets `thinking: "high"`. Triggered Beeper turns do not.
- `action_result.forkTaskId` is the new chat id so the phone can open it.
