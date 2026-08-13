# Phone live reasoning + new home chat

Date: 2026-08-13

## Why

Home compose always continues the one `phone-agent` OpenClaw session, and the phone never sees model thinking because turnproxy drops every non-`chat` event.

## Flow

```
  Home "What do you want done?"
           |
           v
  start_turn taskId=phone-home   ----+
                                     |  new session agent:main:phone-<id>
                                     v
                              phone-chat-<id> thread (opens)
                                     |
           follow-up in that thread  |  same session
                                     v
                              chat.send continues

  Beeper inbound
           |
           v
  StartTriggeredTurn phone-agent / agent:main:main  (unchanged)

  Gateway agent stream:"thinking"
           |
           v
  KindReasoning transcript + Working summary (updates as text grows)
```

## Inputs → Outputs → Steps

1. **Inputs** — OpenClaw `event:"agent"` `{stream:"thinking", data.text/delta}` and optional chat `message.content[{type:"thinking"}]`; Home prompt vs in-thread follow-up vs Beeper trigger.
2. **Outputs** — Reasoning rows that grow while Working; each Home send opens a fresh thread; follow-ups and inbound stay on their own sessions.
3. **Steps**
   1. Advertise `caps: ["thinking-events"]` on protocol v4 connect.
   2. Buffer per-run reasoning; upsert `KindReasoning`; publish Working with a changing safe Summary.
   3. Home send uses compose id `phone-home` → allocate `phone-chat-*` + new session.
   4. Map REASONING to a thread row so the assembler path does not drop it.
