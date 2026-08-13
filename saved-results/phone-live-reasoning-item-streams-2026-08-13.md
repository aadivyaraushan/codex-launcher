# Phone live reasoning from OpenClaw item streams

Date: 2026-08-13

What this is for: Pixel proof of PR #15 still showed no live reasoning. This records what the device actually sent, and the ingest change that follows those streams instead of waiting for `stream:"thinking"`.

## Result

Turnproxy now treats OpenClaw 2026.7.1-2 `item` and `codex_app_server.item` reasoning summaries (and assistant `thinking`/`reasoning` content parts) the same as `stream:"thinking"`: one updating `KindReasoning` row and a changing Working summary. `stream:"thinking"` still works for newer gateways. Unmatched run ids still do not attach to inbound Phone agent.

On 2026.7.1-2 the Codex projector often emits `item` as `{kind:"analysis", title:"Reasoning"}` and `codex_app_server.item` as `{phase, itemId, type:"reasoning"}` with no nested summary. Pixel UXPROBE-0813A showed that a `"Reasoning"` fallback is wrong: Chrome Working is enough until a later nested **summary** exists. Hidden CoT `content` is never read. Current contract: `saved-results/instant-working-stream-tokens.md`.

## Why PR #15 still looked empty on Pixel

Verified against the Pixel proof described in the task (2026-08-13, tip `6e2968d`) and OpenClaw 2026.7.1-2 `extensions/codex/src/app-server/event-projector.ts` (fetched this session from `v2026.7.1-2`):

- Connect advertised `caps=["thinking-events","session-scoped-events"]`. Hello-ok `accepted_caps=["chat-send-routing-contract"]` only. This build never echoed `thinking-events`.
- During a thinking-heavy Home turn, 90 `[turnproxy] received agent event` lines all had `drop_reason=not_thinking` because ingest required `Stream == "thinking"`.
- Actual streams in the Working window: `assistant` 37, `codex_app_server.item` 22, `item` 10, `codex_app_server.lifecycle` 11, `lifecycle` 10, **thinking 0**.
- The Codex projector emits `stream:"item"` (kind/title for reasoning items) and `stream:"codex_app_server.item"` (`type: item.type`). Reasoning *summaries* live on nested `item.type == "reasoning"` (or `type`/`kind` reasoning), not on `stream:"thinking"`. Hidden CoT is `content`; we never read that.
- Silent ~74s before the first chat delta is when those item events arrive. The thread stayed on the user bubble until the final reply.

Connect still sends `thinking-events` and `session-scoped-events` for newer gateways. This change does not depend on either cap. `chat-send-routing-contract` is a server feature, not a thinking subscription; there is no extra connect cap on this build that would create `stream:"thinking"`.

## Inputs → Outputs → Algorithm

1. **Inputs** — Gateway `event:"agent"` frames with Pixel stream names `item`, `codex_app_server.item`, `assistant`, or `thinking`, including nested `payload` envelopes.
2. **Outputs** — KindReasoning transcript upsert, Working summaries that change as the summary grows, info logs with `stream`, `item_type`, `data_type` (enum tokens only), `applied` / `drop_reason`. No thinking text in logs.
3. **Steps**
   1. Normalize the envelope. Keep the real stream name.
   2. If stream is `item` / `codex_app_server.item` and type/kind is reasoning, take **summary** only (string, string list, or `{text}` / `{type:summary_text,text}` objects). Never `content`.
   3. If stream is `assistant`, take content parts typed `thinking` or `reasoning`. Do not use assistant reply `data.text`.
   4. If stream is `thinking`, keep `data.text` / `data.delta`.
   5. Attach by in-flight run id. Unknown run ids drop; they are not painted onto inbound Phone agent.

## Tests run (this session)

```
go test ./companion/internal/phoneruntime/turnproxy/
go test ./companion/internal/phoneruntime/
go test ./companion/internal/app/mobilesession/
go test ./companion/internal/promptqueue/
go test ./companion/internal/codex/tasktranscript/
```

All passed.

Android unit tests were not run here: no Android SDK in this environment. The PR #15 Reasoning row mapping is unchanged.

## Reproduce / reuse

- Fixtures use Pixel stream names `item`, `codex_app_server.item`, `assistant`, including nested `payload` and `item.type=reasoning` summaries.
- Logs to grep on Pixel: `[turnproxy] received agent event` with `stream=item` or `stream=codex_app_server.item`, `item_type=reasoning`, `applied=true`.
- Lifecycle / command / assistant-reply-only events log `drop_reason=not_reasoning`.
- Never expect thinking text in those logs.

Same-bug search: `not_thinking`, `Stream != "thinking"`, `acceptsThinking` — only the turnproxy ingest filter (now `carriesReasoning`). Android Reasoning label left as-is.
