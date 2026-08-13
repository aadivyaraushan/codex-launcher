# Phone live reasoning attach (PR13 follow-up)

Date: 2026-08-13

What this is for: Operator on Pixel still showed no live reasoning after PR #13. Production logs only had `[turnproxy] applied chat event` delta/final. This records why, and what the attach fix does.

## Result

Thinking events from OpenClaw 2026.7.1-2 now attach to the in-flight phone run even when `sessionKey` is missing, nested, or is `agent:main:main` while the chat is `agent:main:phone-*`. Android maps `REASONING` to a labeled `ThreadMessage.Reasoning` row, not unlabeled Activity.

## Why PR13 never applied thinking on device

Verified against OpenClaw 2026.7.1-2 `src/infra/agent-events.ts` and `src/gateway/server-chat.ts` (fetched this session):

- Agent `sessionKey` is optional. Gateway may broadcast `event:"agent"` `stream:"thinking"` with no sessionKey, or with a canonical key that is not the home-chat key.
- PR13 looked up `bySession[payload.sessionKey]` first and returned with no log when the key was empty or pointed at inbound `agent:main:main`.
- `ApplyAgent` also required an exact sessionKey match, so even a correct conversation would drop mismatched keys.
- Nested envelopes (`payload: { stream, data, runId }`, top-level `type:"thinking"` + `text`) did not fill `AgentEventPayload.Stream`.
- Drop paths were silent, so Pixel logs could not tell “never arrived” from “arrived and dropped”.
- A sessionKey fallback to `agent:main:main` when runId did not match would have painted home thinking onto the inbound Phone agent row. Unmatched run ids now drop with `unknown_run`.
- `applied=true` is only logged when a Working event is actually emitted. Empty or duplicate thinking logs `applied=false` with `drop_reason=no_text|unchanged|finished_run`.

Caps: 2026.7.1-2 protocol hello-ok does not require `thinking-events` (broadcasts non-tool agent streams to Control UI-visible clients). Newer gateways do. Connect now sends `thinking-events` and `session-scoped-events`, and logs sent vs accepted caps.

## Inputs → Outputs → Algorithm

1. **Inputs** — Gateway `event:"agent"` frames (canonical, nested `payload`, `type:"thinking"`, `data.stream`, missing sessionKey, or sessionKey `agent:main:main`) plus the in-flight `chat.send` run id.
2. **Outputs** — One updating `KindReasoning` transcript row, Working summaries that change as thinking grows, info logs with event/stream/runId/sessionKey-present/applied-or-drop-reason and no thinking text, Android “Reasoning” label.
3. **Steps**
   1. Normalize the envelope into `stream` / `runId` / `data.text|delta`.
   2. Prefer the conversation that already owns that run id (`pendingRunID`, active turn, mapper, recent transcript). Only then fall back to sessionKey.
   3. Stamp the conversation’s own sessionKey and `ApplyAgent`.
   4. Log apply vs drop without thinking text.

## Tests run (this session)

```
go test ./companion/internal/phoneruntime/turnproxy/
go test ./companion/internal/phoneruntime/
go test ./companion/internal/app/mobilesession/
go test ./companion/internal/promptqueue/
```

All passed.

Android unit tests were not run here: no Android SDK in this environment.

## Reproduce / reuse

- Home chat: `phone-chat-<hex>` / `agent:main:phone-<hex>`. Thinking with `sessionKey: agent:main:main` or no sessionKey still attaches if `runId` matches the `chat.send` turn.
- Logs to grep on Pixel: `[turnproxy] received agent event` with `applied=true|false` and `drop_reason=`.
- Never expect thinking text in those logs.
