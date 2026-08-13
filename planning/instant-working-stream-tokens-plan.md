# Instant Working + stream tokens

Date: 2026-08-13

## Why

Home send sat on “message sent” until the first gateway delta (often tens of seconds). The thread then showed a static Reasoning label, then a sudden final answer.

```
  chat.send ack
       |
       v
  Working "Codex is working"   ← immediately
       |
       +-- assistant/item tokens --> KindReasoning (grows)
       |
       +-- chat deltaText ---------> KindAgent (grows)
       |
       v
  chat final = reply
```

## What changed

- `StartRun` on send (StartExistingTurn, home compose, triggered).
- Chat deltas upsert KindAgent and publish a changing Working summary.
- Assistant `data.text`/`data.delta` count as reasoning while the run is in flight. Item `"Reasoning"` is a placeholder until real summary/tokens arrive, and must not wipe them.
