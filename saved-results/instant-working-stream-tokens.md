# Instant Working + streamed reasoning/reply tokens

Date: 2026-08-13
For: Operator phone-agent Home send felt idle for tens of seconds, then dumped a final answer. Reasoning rows showed the title `Reasoning` with no growing text.

## Inputs → outputs → steps

1. **Inputs** — `chat.send` ack; chat `state=delta` `deltaText`; agent `stream=assistant|thinking|item` with `data.text` / `data.delta` or item summaries.
2. **Outputs** — StartsTurn Working `"Codex is working"` on send; KindReasoning text that grows past the title; KindAgent live draft that grows on chat deltas; final still becomes the reply. Logs: rune counts / stream / item_type, never thinking text.
3. **Steps**
   1. On `chat.send` accepted, `StartRun` publishes StartsTurn Working and marks the run in-flight.
   2. Chat deltas upsert KindAgent and publish Working whose Summary is the accumulated safe reply (so eventpump does not drop equals).
   3. Assistant `data.text`/`data.delta` (and thinking/item summaries) update KindReasoning. A later `"Reasoning"` title does not overwrite live tokens.

## Root causes (verified in `9501ffa`)

**A) Time-to-first-signal** — `sendChat` in `companion/internal/phoneruntime/turnproxy/source.go` only appended the user line after ack. First Working/StartsTurn came from `TurnMapper.Apply` on the first chat delta, or `ApplyAgent` when reasoning text changed.

**B) No streamed tokens** — Chat `deltaText` stayed in `run.text` until `state=final` appended KindAgent. Item-type reasoning with type/title only stored the fallback `"Reasoning"`. `reasoningFromEnvelope` for `stream=assistant` ignored `data.text`/`data.delta` unless a content part was typed thinking/reasoning.

## Sibling search

- `appendTranscript(KindAgent` — only this turnproxy path; now `upsertTranscript`.
- Other send paths: `StartTriggeredTurn` shares `sendChat` (gets Working). `RedirectExistingTurn` is a steer of an already-working turn; left unchanged.
- Assistant ingest — only `turnproxy/mapper.go`. Desktop Codex `taskstate/live.go` already emits Working on `turn/started`.

## How to re-run

```
go test ./companion/internal/phoneruntime/turnproxy/ -count=1
```

Covered: send → Working before any chat delta; chat deltas update KindAgent before final; assistant/item deltas update KindReasoning past the title; logs have rune counts and no thinking text.

Software-attest was not enabled or touched.

PR: https://github.com/aadivyaraushan/codex-launcher/pull/21 (into `worktree-phase2-tool-bridge`). Independent judge signed off: instant Working, KindAgent/KindReasoning streaming, placeholder guard, CoT-safe logs, and proving tests hold.
