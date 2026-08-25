# OpenAI router on the phone + full Beeper messaging

**Date:** 2026-08-06 · **Status:** READY TO IMPLEMENT — adversarial judge PASS (2026-08-06,
fresh-context judge; verified plan claims against the codebase). All 4 judge nits fixed
(reminder verb, seal-script name, Mac rules file address, endpoint table marked unconfirmed
until B0). Attachments closed as out of scope this round (text messaging ops only).
**Why:** The phone answered "I don't have the app you named connected for this" to an Instagram
unread ask. Two root causes, fixed together here: (1) the phone uses a dumb keyword router that
doesn't know Instagram, and (2) the Beeper adapter can only *send* — it can't read, reply, or
manage messages. Diagnosis: `saved-results/instagram-unread-route-failure-2026-08-06.md`.

## The two workstreams at a glance

```
WORKSTREAM A — smart router on the phone          WORKSTREAM B — Beeper beyond send-only
─────────────────────────────────────────         ─────────────────────────────────────────
 "what's my unread Instagram message?"             stage 2 picks the instagram adapter
        │                                                  │
        ▼                                                  ▼
 phone-runtime (Go, on the phone)                  beepermessage adapter (Go)
        │  stage-1 route request                           │  today: Send only
        ▼                                                  │  after: Read / Send / Modify / Cancel
 Android key broker (loopback :9451)                       ▼
        │  adds OpenAI key from Keystore           Beeper Desktop local API (:23373,
        ▼    (key never enters the Go process)      reached from the phone over the
 api.openai.com  →  {verb, app_class, app_named}    existing tunnel / BEEPER_DESKTOP_BASE_URL)
                                                           │
 No key in the broker?  FAIL CLOSED:                       ▼
 the phone says the router isn't set up —          Instagram / Discord / Google Messages
 it never falls back to the keyword router.        read, send, reply, edit, delete, react,
                                                   mark read/unread, archive, reminders
```

Both workstreams are needed for the motivating ask: A gets "unread Instagram" routed to
`app_class=beeper_messaging, verb=read`; B makes the adapter able to actually answer a read.

---

## Workstream A — OpenAI stage-1 router on phone-runtime

### Inputs → Outputs → Algorithm

- **Inputs:** the user's spoken/typed ask (one string); an OpenAI API key sitting sealed in the
  Android Keystore (put there by the operator, see "Dogfood provisioning" below).
- **Outputs:** a route JSON `{verb, app_class, app_named, subject, body, fields, confidence}` —
  the same shape the Mac already gets from `stage1openai.Client.Model`.
- **Algorithm:**
  1. Phone-runtime builds the same OpenAI request the Mac builds (same instructions, same JSON
     schema) but posts it to the **local broker** at `http://127.0.0.1:9451/v1/broker/openai/responses`
     with **no** Authorization header.
  2. The Android app (which owns the Keystore) receives it on its loopback server, unseals the
     key, adds `Authorization: Bearer <key>`, and forwards the request to
     `https://api.openai.com/v1/responses`. The key never leaves the Android app process.
  3. The broker streams the response body back; phone-runtime parses it exactly as the Mac does.
  4. If the broker has no key (or isn't running), the route call fails with a typed error and the
     phone **fails closed** (below). There is no fallback to the keyword router — that path is
     deleted from phone-runtime.

This copies the proven **maps broker** pattern one-for-one: Go client `androidbroker/maps/client.go`
→ Android loopback `MapsBrokerLoopback.kt` on port 9451 → key vault `MapsApiKeyVault.kt`
(AES-GCM key held in Android Keystore, sealed blob in app prefs).

### Fail-closed behavior (what the phone shows)

| Situation | Health `/v1/health` shows | What the user sees on an ask |
|---|---|---|
| Key provisioned, broker up | `router: "openai_broker"` | Normal routing |
| No key in the vault | `router: "openai_broker:no_key"` | "Operator's router isn't set up on this phone yet. Ask the operator to finish setup." |
| Broker unreachable / OpenAI error | `router: "openai_broker"` (per-ask failure) | "I couldn't reach the router just now. Try again in a moment." |

The status is probed per request (a new `GET /v1/broker/openai/status` that reports only
`{"keyed": true/false}` — never the key), so provisioning a key later works without restarting
phone-runtime. `explicit_app` never appears in phone health again.

**How those sentences actually reach the phone** (judge finding: today a stage-1 error is
collapsed to a generic `internal` failure in `mobilesession/handler.go` and the text is
dropped): the brokered client returns **typed errors** (`ErrRouterNotProvisioned`,
`ErrRouterUnreachable`), and the session handler maps those two — and only those two — onto the
same `question`/cancelled-session mechanism stage 2 already uses for its "which app?" questions
(the exact mechanism that delivered the motivating bug's message to the phone). That is a small,
named change in the handler's error mapping, with a test asserting the sentence arrives in the
session reply. All other stage-1 errors stay generic.

### Files and build order (TDD: tests first at every step)

Phase A1 — Android broker (all new code lives beside the maps broker):

1. **Generalize the credential envelope to carry a provider name** —
   `android/.../broker/maps/envelope/MapsCredentialEnvelope.kt` and
   `vault/MapsImportSession.kt` already carry `provider: "maps"` in the envelope metadata.
   Add an `openai` provider path: a second vault instance (`alias "openai-api-key-wrap-v1"`,
   pref key `openai_api_key_sealed` in the same prefs file) and accept `provider: "openai"`
   in the import session. Tests first: envelope round-trip with provider `openai`, wrong-provider
   rejection (mirror `MapsCredentialEnvelopeTest`).
2. **OpenAI broker ops** — new `broker/openai/rpc/OpenAiBrokerOps.kt`:
   `POST /v1/broker/openai/responses` (forward body to api.openai.com with bearer from vault,
   return upstream status+body) and `GET /v1/broker/openai/status` (`{"keyed": bool}`).
   Tests first (mirror `MapsBrokerOpsTest`): status without key, forward adds header, upstream
   error passthrough.
3. **Serve both providers on one always-on loopback server** — `MapsBrokerLoopback` currently
   starts only if the maps key exists (checked once at app start), which would break "provision
   later, no restart": with no keys at all there'd be no status server, and importing a key
   later would never start it. Change: the loopback server **always starts** at app start
   (`LauncherApplication.onCreate`); each request checks its provider's vault at call time.
   Dispatch `/v1/broker/maps/*` to maps ops and `/v1/broker/openai/*` to OpenAI ops; an op whose
   vault has no key answers 503 `{"error":"no_key"}`; `status` answers regardless. Same port
   9451, so the Go side needs no new port config. Test: start unkeyed → status says not keyed →
   import → status flips and ops work, no restart.

Phase A2 — Go phone-runtime:

4. **Brokered stage-1 client** — new constructor in
   `companion/internal/capability/routing/stage1/openai/` (e.g. `NewBrokered(baseURL, logger)`):
   same request building and response parsing as today, but no API key and no Authorization
   header; base URL points at the broker. A `Status(ctx)` helper calls the broker status
   endpoint. Tests first with `httptest`: request has no auth header, route JSON round-trip,
   status=no-key surfaces the typed fail-closed error.
5. **Wire it into phone-runtime** — `companion/internal/phoneruntime/runtime.go`: replace the
   `stage1explicit` block with the brokered client (`routerSource = "openai_broker"`); delete the
   explicit-rules wiring from this file (the explicit package itself stays — Mac fallback and
   tests still use it). Broker URL helper mirrors `maps_broker_url.go`
   (`ANDROID_OPENAI_BROKER_URL`, default `http://127.0.0.1:9451`). Health reports
   `openai_broker` / `openai_broker:no_key` from the status probe. Tests first: health strings,
   fail-closed error text reaching the session reply, no explicit fallback on error.
6. **Session handler error mapping** — `companion/internal/app/mobilesession/handler.go`: map
   the two typed router errors to a user-visible question/cancelled session (see fail-closed
   section above). Test first: simulated no-key broker → session reply carries the setup
   sentence, not "internal".

### Dogfood provisioning (no user UI)

Same envelope dance the maps key already uses (`scripts/maps-envelope-seal.py`,
`MapsImportSession`, exercised by `LiveMapsBrokerProofTest`):

1. On device (via adb/instrumentation): create an import offer for provider `openai` — the phone
   generates a fresh Keystore key pair and prints offer JSON (device serial, package, signing
   digest, device public key).
2. On the Mac: generalize the existing `scripts/maps-envelope-seal.py` with a
   `--provider openai` flag; it seals `OPENAI_API_KEY` from the Mac `.env` to that offer.
   The key is encrypted so only that phone's Keystore can open it.
3. On device (adb/instrumentation): import the sealed envelope → key lands in the vault →
   broker `status` flips to `keyed: true`. No restart needed.

### Router-instruction fix that rides along (the class-mismatch bug)

The OpenAI instructions in `client.go` already say Beeper sends are `app_class beeper_messaging`.
Add coaching for the new read/manage verbs (Workstream B) so "unread Instagram" routes to
`beeper_messaging` + `read` + `app_named instagram`. Also fix the Mac's **fallback** keyword
router the same way (diagnosis fix #1/#2): when Beeper is on, `messages`/`discord` must not be
advertised under class `messaging` in the `stage1explicit` rules built in
`companion/cmd/codex-launcher/production.go` (the rule list comes from the deeplink specs
there). Small, but it's the same bug in a sibling site — the sweep rule says fix it now.

---

## Workstream B — Beeper: every messaging operation for Instagram / Discord / Messages

### Inputs → Outputs → Algorithm

- **Inputs:** a stage-2-resolved intent (verb + subject + body) naming one Beeper network.
- **Outputs:** for reads — real message content shown in the session preview; for writes — a
  confirmed action (send/reply/edit/delete/react/mark-read/archive/reminder) executed through
  Beeper Desktop, with the same never-overclaim discipline the send path has today.
- **Algorithm:** adapter resolves the conversation by name (existing `SearchChats`/ambiguity
  handling), previews exactly what will be read or changed, executes one Beeper HTTP call,
  reports the outcome honestly per that operation's real completion contract (see below).

### How one route names an operation and a target message

The route JSON has only `verb, subject, body` plus a fixed set of named `fields` slots
(`stage1/openai/client.go` `namedSlots`) — one `modify` verb can't by itself distinguish
edit vs react vs mark-read vs archive vs reminder. Two additions close that:

1. **A new nullable `operation` slot in `namedSlots`** (slots are shared across all adapters and
   nullable, so this is additive and safe). Stage-1 instructions coach the model to fill it with
   one of a closed list: `reply, edit, delete, react, unreact, mark_read, mark_unread, archive,
   unarchive, pin, mute, set_reminder, clear_reminder`. The adapter validates against that list
   and asks a clarification question if it's empty or unknown for a modify/cancel verb.
2. **Message targeting is resolved by the adapter, never guessed by the model** (the schema
   already forbids the model inventing IDs). Rules, all backed by `ListMessages` on the resolved
   chat:
   - *edit / delete* → the **authenticated user's own most recent** message in that chat.
   - *reply / react* → the **most recent incoming** message in that chat.
   - If `body`/`subject` quotes text, the adapter matches it against recent messages instead.
   - The preview always shows the exact message text being changed/answered, so the confirm
     gate catches a wrong pick. No match or a stale chat → clarification question, never a
     silent best-effort.

### Beeper Desktop API inventory → adapter mapping

Inventoried from the official changelog/API reference (developers.beeper.com, current v4.2.808).
**Treat every ✚ row as unconfirmed until build step B0 re-verifies it against the live
`GET /v1/spec`** of our actual Desktop version — no adapter code before that check, since the
reference site has 404'd before (note in `beeper/client.go`).

| Beeper Desktop op | Endpoint | Operator verb | Client method (new ✚ / existing ✓) |
|---|---|---|---|
| Search chats | `GET /v1/chats/search` | (support) | ✓ `SearchChats` |
| List chats (unread filter) | `GET /v1/chats` | read | ✚ `ListChats` |
| Get one chat | `GET /v1/chats/{id}` | read | ✚ `GetChat` |
| List messages in a chat | `GET /v1/chats/{id}/messages` | read | ✚ `ListMessages` |
| Search messages | `GET /v1/messages/search` | read | ✚ `SearchMessages` |
| Get one message | `GET .../messages/{mid}` | read | ✚ (only if a flow needs it) |
| Send message | `POST /v1/chats/{id}/messages` | send | ✓ `Send` (✚ optional `replyToMessageID`) |
| Start direct chat | `POST /v1/chats/start` | send | ✓ `StartChat` |
| Edit message | `PUT .../messages/{mid}` | modify | ✚ `EditMessage` |
| Delete message | `DELETE .../messages/{mid}` | cancel | ✚ `DeleteMessage` |
| Add / remove reaction | `POST/DELETE .../reactions[/{key}]` | modify | ✚ `React` / `Unreact` |
| Mark read / unread | `POST /v1/chats/{id}/read` `/unread` | modify | ✚ `MarkRead` / `MarkUnread` |
| Archive / unarchive | `POST /v1/chats/{id}/archive` | modify | ✚ `Archive` |
| Chat state (pin, mute, title…) | `PATCH /v1/chats/{id}` | modify | ✚ `UpdateChat` (pin/mute only, v1) |
| Set / clear reminder | `POST/DELETE /v1/chats/{id}/reminders` | modify / cancel | ✚ `SetReminder` / `ClearReminder` |
| Send attachment (upload asset) | `POST /v1/assets/upload` + send | send | **out of scope this round — follow-up** (see note below table) |
| Contacts list/search | `/v1/accounts/{id}/contacts…` | (support) | ✚ only if start-chat-by-name needs it |
| Focus app / unified search / assets serve / info / oauth | various | — | **skipped: not messaging** |

Per-network truth: a chat's `capabilities` object says what its platform supports (e.g. some
networks disallow edit). The adapter checks capabilities before previewing an op and says
"Instagram doesn't support editing messages" instead of failing on execute.

**Attachments — out of scope for this round (user decision, 2026-08-06).** This round covers
text messaging operations only. Sending a photo/file needs a file on the machine running
Beeper Desktop (the Mac), and the phone has no path to put one there today — that's a separate
file-transfer piece, deferred to a follow-up. Incoming attachments in reads are described as
`[attachment: image]` etc. so reads never silently drop content.

### What a "read" looks like end to end (and its hard limits)

Reads ride the existing Plan → Preview → confirm → Execute flow unchanged (`flow/service.go`
always stores a preview; the phone shows `capability_preview`). We accept that dialog for v1 —
and make it painless: **the preview itself carries the message text**, so the user has their
answer the moment the preview appears; confirming just closes the loop. No flow or phone
protocol changes.

Scope rules for the motivating ask (subject may be empty — today's resolver would wrongly treat
an empty subject as an error):

- **No conversation named** ("my unread Instagram messages") → network-wide unread scan:
  `ListChats` filtered to this adapter's network and `unreadCount > 0`, ordered by
  `lastActivity` (Beeper's own ordering), newest first, capped at **3 chats**; for each, pull
  its latest unread messages via `ListMessages` (cap **5 messages per chat**).
- **Conversation named** → resolve it as today, list its most recent messages (cap 10).
- **Search ask** ("what did Maya say about Friday") → `SearchMessages` scoped to the network.

Wire-size limits are real (phone preview: max 8 lines × 1,024 chars; outcome detail: 2,048
chars — `ProtocolCodec.kt`). The adapter truncates: each message rendered as
`Sender: text` clipped to ~200 chars, at most 6 lines, plus a final "…and N more unread" line.
A dedicated test feeds oversized fake messages and asserts the preview still fits the codec.

### Write outcomes are per-operation, not one-size-fits-all

Only `Send` has the "accepted, pending, final visibility unknown" contract
(`{chatID, pendingMessageID}`), so only send keeps `OutcomeUnknownError`-with-pending. The
state ops (`read/unread/archive/reminders`, and per docs several others) return
`204 No Content` — success there **is** completion, and the adapter reports a plain confirmed
outcome. B0 records the actual response contract of every op from the live spec, and each
adapter op maps to confirmed vs pending accordingly, with a test per class.

### Files and build order (TDD throughout)

- **B0** — one throwaway check against the live Desktop `GET /v1/spec`: confirm every endpoint,
  field name, and per-network capability we plan to call. Cheap-iteration rule: this is the
  whole "one iteration" for API-shape questions — no adapter code until the table above is
  confirmed against our running version.
- **B1** — `companion/internal/capability/messaging/beeper/client.go`: add the ✚ methods.
  Tests first in `client_test.go` (fake HTTP server asserting method/path/body per op, read-only
  client refuses every write — extend the existing `ErrReadOnly` guard to all new writes).
  Extend `live_test.go` (env-guarded) with one read smoke against a real Desktop.
- **B2** — `companion/internal/capability/adapters/beepermessage/adapter.go`: accept verbs
  `read`, `send`, `modify`, `cancel` (manifest `Verbs` updated); dispatch on verb + the new
  `operation` field; implement the read-scope rules, message-targeting rules, truncation, and
  per-op outcome mapping defined above (an empty subject is now valid for reads —
  network-wide unread scan — so the current `ErrNoRecipient` check moves to write verbs only).
  Tests first per verb+operation, including: unread ask with empty subject returns newest
  unread messages for that network; oversized messages truncate within codec limits;
  ambiguous names still ask; unknown `operation` asks; capability-missing ops refuse with a
  plain sentence; edit/delete target own-latest, reply/react target incoming-latest.
- **B3** — stage-1 schema + instruction lines (`stage1/openai/client.go`): add `operation` to
  `namedSlots`; coach reads and manage ops to `beeper_messaging` with the closed operation
  list. Test: routing fixtures for "unread Instagram", "reply to Maya on Discord", "delete my
  last message", "archive that group chat" — each asserting verb, class, app_named, and
  operation.
- **B4** — stage-2 behavior verification (`companion/internal/capability/runtime/production.go`
  + `stage2/resolver.go`): registration under `beeper_messaging` and `ResolvedByAdapter`
  addressing already exist, and stage 2 filters candidates by manifest verb — but new tests
  must pin: a `read` route **with** `app_named=instagram` reaches that one adapter; a read
  **without** `app_named` (three adapters survive) produces a "which network?" question, not an
  error; an unsupported network name produces the named-app question. Mac and phone both get
  the expanded adapter through this one file.
- **B5** — read-only mode, both wiring sites: today `BEEPER_READONLY=1` drops Beeper entirely —
  phone (`phoneruntime/beeper_health.go` returns nil API) and Mac
  (`cmd/codex-launcher/production.go`). Change: in read-only mode both sites pass
  `client.ReadOnly()` plus a new `ProductionConfig.BeeperReadOnly bool`; registration builds
  the adapter manifest with `Verbs: [read]` only. Every new write method on the client also
  honors the existing `ErrReadOnly` guard, so writes are refused at both layers. Tests: manifest
  verbs under the flag, each write method refuses on a read-only client, read still answers.

### End-to-end acceptance (the motivating bug, re-run)

On the Pixel with key provisioned and Beeper connected: ask *"what's my most recent unread
Instagram message"* → route `beeper_messaging/read/instagram` → adapter lists unread Instagram
chats → the actual message text appears in the session reply. Also re-run the diagnosis's
repro to confirm the old wrong answer is gone.

---

## Build order across workstreams

```
A1 Android broker (envelope + ops + loopback)   B0 live /v1/spec check
        │                                              │
A2 Go brokered client + phone wiring            B1 beeper client methods
        │                                              │
        │                                       B2 adapter verbs
        │                                              │
        └────────────► B3 stage-1 instructions ◄───────┘
                              │
                       B4/B5 runtime + readonly
                              │
                    Dogfood provision on Pixel
                              │
                    End-to-end acceptance ask
```

A and B are independent until B3 (the instruction lines matter only when the OpenAI router is
what's routing on the phone). Each phase lands green on its own tests before the next starts.

## Risks & edge cases

- **Key safety:** the OpenAI key exists only (a) sealed in the Android vault and (b) in memory
  inside the Android app while adding the header. It is never written to phone-runtime files,
  env, or logs. Broker binds to 127.0.0.1 only (same as maps).
- **Loopback broker has no caller auth** (also true of maps today): any local process could spend
  the OpenAI key's quota. Accepted for dogfood; noted as a follow-up (attested caller check) —
  not expanded here.
- **Beeper Desktop is on the Mac, not the phone:** phone reads/writes go over the existing
  tunnel (`BEEPER_DESKTOP_BASE_URL`). If the tunnel is down, reads fail with "I can't reach
  Beeper right now" — health `beeper=unavailable` already reflects this.
- **API drift:** the reference site has been unreliable; B0's live-spec check is the guard.
  Version-gate anything that only exists in newer Desktop builds (`X-Beeper-Desktop-Version`
  header is available for a check if needed).
- **Per-network gaps:** capabilities differ by network (edit/react/etc.). Adapter refuses
  unsupported ops by name rather than sending and failing.
- **Model naming drift:** OpenAI might return `app_named` values we don't register ("google
  messages" vs `messages`). Stage-2's named-adapter check already turns that into a question;
  routing fixture tests in B3 pin the ids we coach.
- **Fail-closed lockout:** with no key, the phone can't route *anything* (that's the decision —
  no keyword fallback). The health string and the on-phone message make the cause obvious.
- **Read privacy:** reads pull real message content through the session. Same trust boundary as
  sends today (paired, confirmed device); no new storage — content is shown, not persisted.
- **Reads require a confirm tap (v1):** the flow always previews before executing. Accepted:
  the preview already contains the answer, so the tap costs nothing informationally. Removing
  the tap for non-destructive reads is a possible follow-up, not in scope.
- **Wrong-message targeting:** edit/delete/reply pick a message by recency rules; a race (a new
  message arriving between resolve and confirm) could shift the target. Mitigation: the plan
  pins the target **message ID** at resolve time and previews its exact text — execute acts on
  the pinned ID, never "latest at execute time".

## Success criteria

- [ ] Phone health shows `router: openai_broker`; `explicit_app` gone from phone-runtime.
- [ ] With no key: asks fail closed with the setup message; provisioning flips it live.
- [ ] Key never appears in phone-runtime process, files, or logs (grep the logs in acceptance).
- [ ] Every table row above either works on all three networks, refuses by capability with a
      plain sentence, or is explicitly deferred (attachments — out of scope this round).
- [ ] "What's my most recent unread Instagram message" (no conversation named) shows the actual
      message text in the session preview, within the phone codec limits.
- [ ] Edit/delete/reply act on a message ID pinned at resolve time, with its text shown in the
      preview.
- [ ] All new code test-first; green runs shown for A1, A2, B1, B2, B3, B4, B5.

## Open decisions for the user

None. Attachment sending was the last open decision and is closed as out of scope for this
round (2026-08-06) — text messaging operations only; attachment sending is a follow-up.
