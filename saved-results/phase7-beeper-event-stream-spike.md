# Phase 7 spike — Beeper DOES expose an event stream (WebSocket /v1/ws)

Date: 2026-08-12
For: Phase 7 of planning/openclaw-phone-agent-plan.md ("Spike first: does the local Beeper server expose an event stream?").

## Result: yes — no polling needed

The Beeper Client API (the same local server on `127.0.0.1:23373` our Go
client already talks to) serves a WebSocket at **`/v1/ws`**, authenticated
with the same Bearer token in the upgrade request.

Protocol, from the server's own OpenAPI spec (`GET /v1/spec`, 775 KB, 67
paths) and confirmed live:

1. Connect to `ws://127.0.0.1:23373/v1/ws` with `Authorization: Bearer <token>`.
2. Server sends `{"type":"ready","version":1,"chatIDs":[]}`.
3. Client sends `{"type":"subscriptions.set","chatIDs":["*"],"app":{"state":true}}`.
4. Server replies `subscriptions.updated`, then streams `chat.upserted`,
   `chat.deleted`, `message.upserted`, `message.deleted`, `app.state.updated`.

Delivery caveats the watcher must design around (spec's own words):
**at-most-once, no replay on reconnect, `seq` is per connection** — after a
disconnect you refetch over HTTP to reconcile drift. Initial subscription
state is empty; `subscriptions.set` replaces previous state; `["*"]` cannot
be combined with specific chat IDs.

This makes the plan's fallback question (owner open question 1: is ~15s
polling an acceptable battery cost?) moot for the normal path — polling is
only the reconcile-after-reconnect step, not the steady state.

## Evidence

- Live handshake (Beeper Desktop running on the Mac):
  `curl -i -N http://127.0.0.1:23373/v1/ws` with Bearer + Upgrade headers →
  `HTTP/1.1 101 Switching Protocols` followed immediately by the `ready`
  frame on the socket.
- Spec fetched with 200/775020 bytes; its `## WebSocket` section documents
  the flow quoted above verbatim.
- Repo state before this spike (Explore-agent read): no streaming client
  existed anywhere; new messages were only discovered when a read action
  asked for unread chats (`capability/adapters/beepermessage/adapter.go`
  resolveUnreadScan). `beeperreadback.Event` modeled a send-confirmation
  event but nothing produced it from a live channel — `message.upserted`
  over this socket is the transport it was waiting for.

## Reproduce

Beeper Desktop must be running (server rides inside it; `open -ga "Beeper
Desktop"` is enough). Token comes from `BEEPER_ACCESS_TOKEN` in the main
checkout's `.env` (path only — never commit the value):

    curl -s -H "Authorization: Bearer $BEEPER_ACCESS_TOKEN" \
      http://127.0.0.1:23373/v1/spec | python3 -m json.tool | less
    # handshake test: same URL with /v1/ws + the four WebSocket upgrade
    # headers → expect 101 + a ready frame.

Caveat carried from earlier spikes: the only verified deployment of this
server is Beeper Desktop on the paired Mac; the headless `beeper-server`
binary crashed under Termux proot (`Could not find a PHDR`, see
beeper-server-phone-linux-spike.md). The watcher should therefore take the
base URL from config, not assume localhost-on-phone.

## Mac-side slice built on this spike (judged PASS 2026-08-12)

Three units, all TDD (red first, Sonnet implementation, fresh green
confirmed by the orchestrator with `-race`):

1. `phoneruntime/beeperwatch` (commit ed4bee2): dial /v1/ws, full handshake
   per connection, message.upserted → Notify; bounded id dedupe surviving
   reconnects; isSender/isHidden/isDeleted filtered; entries-optional ids
   loaded via an injectable loader; ids-only logging.
2. `phoneruntime/agenttrigger`: `EnsureRules` materializes an owner-editable
   `<root>/agent-rules.md` on first use (drafts/summaries allowed, autonomous
   sends only to the allow list, which starts empty) and never overwrites an
   edited file; `Prompt` marks the turn as an automatic trigger, not the
   owner; `Preview` = "New message from <sender>".
3. Runtime wiring: `Config.BeeperBaseURL` (empty = off; requires a gateway),
   token via BEEPER_ACCESS_TOKEN/account.db at Open, watcher shares the turn
   proxy's cancel, delivery = `StartTriggeredTurn` on the phone-agent task —
   a new turnproxy method that sends chat.send like a normal turn but stamps
   the Home preview as plain text ("New message from Maya"), never "You: …".

Independent judge (fresh context, first-principles checklist before seeing
the code) verdict: PASS — build clean, 127 phoneruntime tests race-clean.
One gap flagged as must-fix-next, not slice-blocking: the allow list is
prose in the prompt only; `gates` policy allows sends to any already-known
recipient and bridge.go passed `AllowListed: false` hardcoded, so
"autonomous sends only to allow-listed people" had no code enforcement while
trigger turns start from attacker-controlled message text. Enforcement is
the next unit after this slice's commit.

Still open for Phase 7: live proof (test account DMs the owner's Instagram;
agent wakes and acts per rules) — needs the phone and a running gateway;
watcher Loader is nil in production wiring for now (entries-only coverage).
