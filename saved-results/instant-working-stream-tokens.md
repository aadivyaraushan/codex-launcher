# Instant Working before chat.send ack (Pixel UXPROBE-0813A)

Date: 2026-08-13
For: Operator phone-agent Home send felt idle, then dumped a final answer. Live Pixel timestamps contradicted treating assistant replies as chain-of-thought and using a `"Reasoning"` fallback.

## Probe (verified from the task, not re-run here)

UXPROBE-0813A `phone-chat-26f5192ca50717ea` (Dubai):

- Android `start_turn` 11:12:44.550
- `chat.send` accepted 11:13:01 (+16.5s) — blocked behind a prior gateway run
- first agent event lifecycle +39s
- first Working + KindReasoning +68.7s (title-only `item_type=reasoning`)
- assistant stream +69.5s `drop_reason=not_reasoning`; Data.Text/Delta empty after normalize
- first chat delta +69.5s `emitted=0` (run already started); reply only on final +70.4s
- `stream=thinking`: 0. Caps still only `chat-send-routing-contract`

Longer turn: 10 min, 0 chat events, UI = user bubble + two lines both `"Reasoning"`. 119 reasoning items `drop_reason=unchanged` (same fallback). preamble/command dropped `not_reasoning`.

## Inputs → outputs → steps

1. **Inputs** — Home/inbound send (`start_turn`); gateway `chat.send` that may sit behind another run; agent item/preamble/command events; chat `deltaText`; optional item **summary** text.
2. **Outputs** — StartsTurn Working `"Codex is working"` on the new task before `chat.send` ack; live KindActivity/KindCommand during the think/tool window; KindAgent that grows on chat deltas; KindReasoning only when a summary actually exists. Logs: enums / rune counts, never thinking text.
3. **Steps**
   1. On send accepted (before waiting on `chat.send`), stamp the user line, `StartRun`, publish Working, return `phone-chat-*` so the phone can open the thread.
   2. Wait for `chat.send` in the background. On ack, alias the server `runId` onto the idempotency key.
   3. Title-only `{kind:analysis,title:Reasoning}` / `type=reasoning` with no summary: do not upsert KindReasoning (`drop_reason=no_text`).
   4. Assistant reply tokens: leave Data empty after normalize (`drop_reason=not_reasoning`). Do not treat them as CoT.
   5. Preamble → KindActivity; command/commandExecution → KindCommand with `status=inProgress`.
   6. Chat deltas upsert KindAgent and publish a changing Working summary. Final is still the reply.
   7. If a later item has summary text, upsert KindReasoning with that text and keep updating. Never read hidden `content`.

## Root causes (Pixel vs original hypothesis)

**A) Time-to-first-signal** — Working after `chat.send` ack is too late. Pixel waited 16.5s on a serialized gateway run, and the phone never learned the new `phone-chat-*` id until `StartExistingTurn` returned.

**B) Title-only Reasoning** — Fallback `"Reasoning"` produced duplicate Reasoning rows and 119 `unchanged` drops. Chrome Working is enough until a real summary exists.

**C) Assistant tokens are not CoT** — Pixel dropped `stream=assistant` as `not_reasoning` with empty Data after normalize. Do not ingest reply `data.text`/`data.delta` as reasoning.

**D) No live progress in the long think window** — preamble/command were dropped `not_reasoning`, so a 10-minute tool turn showed only the user bubble plus `"Reasoning"`.

## Sibling search

- `reasoningFallbackSummary` — none left (removed).
- Title `"Reasoning"` in turnproxy — only used to *detect* title-only analysis items and to refuse using that title as preamble text (`mapper.go`).
- Assistant `data.text` as CoT — only this mapper; now empty after normalize unless content type is thinking/reasoning.
- Desktop Codex `tasktranscript` `"Reasoning activity"` is a different app-server mapper; left unchanged.
- `StartTriggeredTurn` shares `sendChat` (gets Working before ack). `RedirectExistingTurn` is a steer of an already-working turn; left unchanged.
- Software-attest — not enabled or touched.

## How to re-run

```
go test ./companion/internal/phoneruntime/turnproxy/ -count=1
```

Covered: Working visible before `chat.send` ack (home compose held); title-only items do not create KindReasoning (`drop_reason=no_text`); assistant reply tokens `not_reasoning` with empty Data; preamble/command map to KindActivity/KindCommand; item summary still becomes KindReasoning; chat deltas still update KindAgent before final.

PR: https://github.com/aadivyaraushan/codex-launcher/pull/21 (into `worktree-phase2-tool-bridge`).
