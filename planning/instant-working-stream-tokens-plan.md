# Instant Working + live thread progress

Date: 2026-08-13

## Why

Pixel probe UXPROBE-0813A (`phone-chat-26f5192ca50717ea`, Dubai) sat blank for 16.5s because `chat.send` was queued behind a prior gateway run. Working and a title-only `KindReasoning` ("Reasoning") only appeared at +68.7s. Assistant reply tokens are not chain-of-thought. A 10-minute turn with 0 chat events showed two "Reasoning" lines and dropped preamble/command as `not_reasoning`.

```
  start_turn / send accepted
       |
       v
  Working "Codex is working"   ← before waiting on chat.send ack
       |
       +-- preamble / command item --> KindActivity / KindCommand + Working
       +-- item summary text --------> KindReasoning (only if summary exists)
       +-- chat deltaText -----------> KindAgent (grows)
       |
       v
  chat final = reply
```

Chrome Working is enough for title-only `{kind:analysis,title:Reasoning}`. Do not invent CoT from hidden `content` or assistant `data.text`/`data.delta`.

## What changed

- Publish StartsTurn Working as soon as send is accepted, including while `chat.send` is still blocked.
- Home compose still returns `phone-chat-*` immediately so the phone can open the thread.
- Drop `reasoningFallbackSummary="Reasoning"`. Title-only items log `drop_reason=no_text`.
- Assistant reply-only events stay `drop_reason=not_reasoning` with empty Data after normalize.
- Preamble → KindActivity; command/commandExecution → KindCommand (`status=inProgress`).
- Chat deltas still upsert KindAgent before final.
- A later item with real summary text becomes KindReasoning and keeps updating.

Software-attest stays off. Caps are unchanged (`thinking-events` still advertised; Pixel accepted only `chat-send-routing-contract`).
