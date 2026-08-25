# Operator becomes a persistent phone agent (OpenClaw brain)

**Date:** 2026-08-11
**Status:** Judge-reviewed PASS (2026-08-11), including same-day amendments:
chat-first UI (phase 6), real-user on-phone verification gate, on-phone
credentials, README format (phase 10)
**Supersedes (on acceptance):** the predetermined-routing architecture in
[finish-consumer-and-messaging-plan.md](finish-consumer-and-messaging-plan.md) and
[consumer-app-implementation-plan.md](consumer-app-implementation-plan.md). Their
adapter, credential-broker, and phone-runtime work is kept; their
utterance→route→confirm flow is what this plan deletes.

## The change in one picture

```text
TODAY                                     AFTER
-----                                     -----
You type one request                      A persistent agent runs on the phone
      |                                         |
      v                                         v
stage1: cloud model guesses               OpenClaw Gateway (Node.js, in the
  which "route" it is                     same Termux/Debian Linux the
      |                                   phone-runtime already lives in)
      v                                       |  decides for itself what to do,
stage2: table maps route                      |  when, and with which tool
  to one adapter + handle                     v
      |                                   Operator tool bridge (Go, loopback)
      v                                       |
one adapter runs once,                        v
then everything stops                     the SAME adapters as today:
                                          Beeper (Instagram/Discord/SMS),
                                          Calendar, Drive, Slack, Outlook,
                                          Spotify, Notion, Maps, YouTube
```

## How the pieces sit on the phone

```text
+----------------------------- Pixel (Android) -----------------------------+
|                                                                           |
|  Operator launcher app (Kotlin)             Termux -> proot Debian        |
|  - home screen, task transcript UI          +---------------------------+ |
|  - credential broker (tokens stay           | OpenClaw Gateway (Node)   | |
|    in Android, short-lived only)            |  - agent loop, memory,    | |
|  - notification listener                    |    heartbeat, cron        | |
|         |                                   |  - model: your ChatGPT/   | |
|         | loopback TLS :9443                |    Codex OAuth (flat rate)| |
|         v                                   +------------+--------------+ |
|  operator-phone-runtime (Go)  <--- ws/http loopback ---->|                |
|  - tool bridge: adapters exposed as agent tools          |                |
|  - turn proxy: launcher <-> Gateway                      |                |
|  - hard policy gates (approval for risky sends)          |                |
|  - Beeper server :23373 (already here)                   |                |
+---------------------------------------------------------------------------+
```

Nothing leaves the phone except the model API calls and the service APIs the
adapters already call.

## Context

- Owner decision (this session, 2026-08-11): the product is a persistent agent
  that lives on the phone and does things, not a launcher that executes one
  predetermined function per request. Decisions made in the question round:
  brain = OpenClaw itself; no screen control — existing adapters are the hands;
  same repo, becomes the product; model = existing ChatGPT/Codex auth;
  autonomy = act on its own under a user-editable standing rule-set;
  distribution = open source with a paste-one-prompt install, not a polished
  consumer product.
- Why OpenClaw and not our own loop: it is a proven, actively developed
  open-source persistent agent (gateway + heartbeat + cron + memory + plugin
  tools + phone nodes), it officially supports OpenAI Codex OAuth under a
  ChatGPT subscription ([provider docs](https://docs.openclaw.ai/providers/openai)),
  and running it on Android in Termux/proot is a documented community path.
  Porting a proven implementation beats writing a new agent loop.
- Why this is cheap here: everything hard is already built and live-proven on
  the Pixel — `operator-phone-runtime` (Go, arm64) runs under Debian/runit with
  Beeper connected (`saved-results/openai-beeper-pixel-dogfood-2026-08-06.md`),
  every integration is a Go adapter with a machine-readable manifest
  (`companion/internal/capability/manifest/manifest.go`), and credentials
  already flow as short-lived tokens over loopback
  (`android/.../runtime/broker/CredentialBroker.kt`).

## Scope

**In:**

1. OpenClaw Gateway installed and persistent inside the existing Termux/Debian
   runtime, authenticated with the owner's ChatGPT/Codex OAuth.
2. A tool bridge: every live adapter (Beeper nets, Calendar, Drive, Slack,
   Outlook, Spotify, Notion, Maps, YouTube) callable by the agent as a tool.
3. Standing-rules autonomy: agent acts alone by default; a small set of hard
   gates (below) still stops at the launcher's existing approval sheet.
4. Launcher = the agent's UI, chat-first (owner decision 2026-08-11): the
   whole interface collapses to task threads you open and talk in. The agent
   speaks first when something happens; every ask it has (approval, question)
   arrives as a message in the thread with the preview inline, answered by a
   tapped suggested reply or by typing. No sheets, no alerts. On-computer
   (Codex) threads and on-phone (agent) threads look identical in one list;
   the only place the distinction exists is two new-session buttons: "on
   phone" and "on computer".
5. Incoming-message triggers: a new message (via Beeper or the notification
   listener) can wake the agent without the owner asking anything.
6. Deletion of the predetermined-function pipeline (stage1, stage2, flow,
   class table, `capability_request`/`capability_confirm`).
7. Open-source packaging: honest README, one-paste install prompt, sanitize
   pass.

**Explicitly out:**

- Screen control / accessibility automation (owner decision).
- iOS, Play Store submission, payments, legal review.
- The Mac companion + relay-box Codex path. It stays working; whether it
  later becomes an OpenClaw tool ("run code on my computer") is a separate
  decision. Nothing in this plan may break its transport or companion code.
  The one thing that changes is how its threads look: phase 6 renders them
  in the same chat surface as everything else.
- New service integrations (WhatsApp, Teams, Todoist stay deferred/skipped
  exactly as they are wired today).
- Multi-user/product hardening. Open-source alpha honesty instead: the README
  says plainly this is a shaky, powerful, self-hosted experiment.

## Constraints

- **Money:** the brain runs on the owner's existing ChatGPT subscription via
  Codex OAuth — flat rate, no new per-token billing. Gate: at install (phase 1)
  the OAuth must be shown to resolve to the intended ChatGPT account and be
  recorded in `saved-results/` before any agent traffic. Subscription usage
  caps apply (Plus tier has a weekly Codex quota).
- **Credentials stay in Android.** The Termux side never holds refresh tokens
  or API keys (Maps broker pattern stays). The bridge requests short-lived
  tokens per call through the existing `credential_request` flow.
- **Loopback only — and loopback is not enough for the Gateway.** Gateway,
  bridge, and Beeper server bind 127.0.0.1 inside the phone. But OpenClaw's
  known "ClawJacked" attack class steals a gateway token from a malicious
  webpage in the device's own browser, which connects from localhost —
  loopback binding does not stop it, and this phone runs a browser daily.
  Required, verified in phase 1: pin an OpenClaw version patched against the
  Jan–Feb 2026 gateway advisories, set a non-wildcard `allowedOrigins`,
  confirm the Control UI's device-auth check is active, and demonstrate from
  the phone's browser that a cross-origin page cannot open a Gateway
  connection.
- **The launcher UI and protocol stay.** The generic turn/event family
  (`start_turn`, `steer_turn`, `interrupt_turn`, approvals, task states) is
  already shaped for streaming an agent and is reused, not replaced.
- **Existing test matrix stays green** at every phase: `go test ./... -race`,
  `go vet`, Android unit + lint, release checks.
- **Fix in place:** when the agent path is proven, the routing pipeline is
  deleted in the same wave — no flag keeping both paths alive.
- **Real-user verification on the connected phone (owner decision
  2026-08-11).** The owner's Pixel stays connected (adb) for the whole build.
  A phase is not done until its proof has been run on the phone exactly as a
  real user would run it: start from the home screen, tap and type the actual
  steps a new user would take, watch what actually renders and happens.
  **Who drives it:** the implementing session, over adb, using only the
  surface a finger and keyboard use — screen-level taps, swipes, and typed
  text on the rendered UI, reading the result from screenshots. What's
  banned is bypassing the UI: launching activities via adb intents, test
  hooks, or curl to endpoints standing in for a screen the user would see.
  The owner is pulled in only for steps a human must do (a fingerprint, an
  OAuth consent screen), and asked to do exactly that step and nothing more.
  **Evidence:** each walkthrough produces a timestamped screenshot per step
  (screen recording for the end-to-end proofs), stored under `saved-results/`
  alongside the step log — so the phase 11 judge can check the pass without
  trusting the report. When a step fails or behaves unlike what a real user
  would accept (crash, hang, confusing state, wrong output), the cause is
  found and fixed and the walkthrough is rerun from the start of that
  phase's steps, looping until it passes clean. Long soaks are the one
  exception to full reruns: a phase with a long-duration proof names a cheap
  gating check that must pass clean first; the soak then runs once, and only
  a failure of the soak itself repeats it. Unit/integration tests are
  necessary but never sufficient.
- **Reuse the credentials already on the phone.** Any OAuth or sign-in this
  plan needs (ChatGPT/Codex OAuth, Google services, Beeper, anything else)
  uses the accounts already signed in on the owner's Pixel — no new accounts,
  no credentials imported from elsewhere. This governs accounts the agent
  *signs into*; the separate "test account" some proofs message (phases 3
  and 7) is a conversation partner on the other end, not signed into the
  Pixel, and is unaffected. The money gate still applies: the resolved
  account is shown to the owner and recorded before first use.

## Alternatives considered

| Choice | Alternative | Why rejected |
|---|---|---|
| OpenClaw as brain | Own loop on Claude Agent SDK; Kotlin-native loop | Owner decision; buys loop/memory/triggers/plugins for free; Kotlin loop dies to Android background limits |
| Native OpenClaw plugin calling a small loopback HTTP bridge | Full MCP server in Go | Both ends are ours; MCP adds a protocol layer with no second consumer today. Plugin tools are OpenClaw's documented first-class path. Revisit if a second MCP client appears |
| Go runtime proxies launcher↔Gateway | Launcher speaks OpenClaw WebSocket directly | Proxy reuses proven pairing/TLS/UI untouched; a Kotlin WS client + new UI is a bigger diff for the same outcome |
| Same repo pivots | New repo / fork of OpenClaw | Owner decision; adapters + launcher + runtime already live here and stay load-bearing |

## The standing rule-set (autonomy model)

Two layers, deliberately different strengths:

1. **Soft rules the agent reads** — a user-editable rules file in the OpenClaw
   workspace (its standard agent-instructions mechanism). Ships with a default
   the owner edits in any text editor: reply tone, who it may talk to, what it
   should do on heartbeat, quiet hours.
2. **Hard gates the agent cannot cross** — enforced in the Go bridge, not in
   the model's head. Initial set (owner can extend):
   - First-ever message to a recipient the agent has never messaged before →
     stops for the owner's OK.
   - Any `Revoke`/disconnect of a service → stops for the owner's OK.
   - Anything the adapter manifest marks irreversible AND outside the rules
     file's allow-list → stops for the owner's OK.
   - **Cross-adapter exfiltration:** an outbound send whose turn also read
     data from a different adapter (Drive, Calendar, Notion, ...) → stops for
     the owner's OK, even to an allow-listed recipient. This is the standard
     prompt-injection path for this tool set: an inbound message tells the
     agent to fetch private data and send it somewhere "known". The bridge
     tracks which adapters a turn has read from and gates the send.
   Everything else executes autonomously and shows up in the transcript and
   action journal after the fact.

   A hard gate is enforced in the Go bridge; how it *looks* is a chat
   message. The agent's ask renders in the thread with the exact outbound
   preview inline (phase 6). Until phase 6 lands, the existing approval
   sheet renders it.

   **Release rule (exact-token only):** a hard gate releases on exactly one
   thing — the structured approval action carrying the gate's id, sent by
   tapping the approve suggested reply, compared literally in the Go bridge.
   Typed text NEVER releases a gate: "yeah go for it", "sure", "k" all route
   to the agent as steering, and if the agent then still wants to act it
   must re-present the same gate. Deny is likewise a structured action and
   is always offered. The model's interpretation of language sits outside
   the gate, permanently.

## Phases

Each phase is independently shippable, ends green, and names its proof.
Every proof runs on the owner's connected Pixel as a real-user walkthrough
per the Constraints bullet above — driven like a real user, fixed and rerun
until it passes. Details below the list.

1. **Spike: OpenClaw alive in the phone's Debian** (ops, no repo code)
2. **Tool bridge, server side** (Go: `/v1/agent-tools/list` + `/call`)
3. **OpenClaw plugin `operator-tools`** (Node: manifests → registered tools)
4. **First autonomous proof + hard gates** (Go policy + live Instagram send)
5. **Turn proxy: launcher becomes the agent's chat** (Go WS client → events)
6. **Chat-first UI collapse** (Kotlin: sheets die, everything is a message)
7. **Triggers: incoming messages wake the agent** (Beeper poll/notification)
8. **Delete the predetermined-function pipeline** (the pivot lands)
9. **Boot persistence + battery survival** (runit + termux-boot + doc)
10. **Open-source packaging + install prompt** (README, script, sanitize)
11. **End-to-end judged proof on the Pixel**

### Phase 1 — Spike: OpenClaw alive in the phone's Debian

- In the existing proot Debian on the Pixel: install Node ≥ 22, install
  OpenClaw, run onboarding with **OpenAI Codex OAuth** — signing in with the
  ChatGPT account already on the phone (per the credentials-on-phone
  constraint), completing the browser leg of the flow on the phone itself.
- **Owner gate (money rule):** before the first agent turn, show which ChatGPT
  account the OAuth resolved to; owner approves; record account + date in
  `saved-results/openclaw-phone-brain-setup.md`.
- Gateway bound loopback-only; verify no port reachable from the LAN.
- Gateway hardening per the Constraints bullet: patched version pinned,
  non-wildcard `allowedOrigins`, device-auth confirmed, and a live check that
  a cross-origin page in the phone's browser cannot reach the Gateway.
- Proof: gateway healthy after phone reboot of the Termux session; one chat
  turn answered via its local WebChat; heartbeat fires.
- No repo code changes. Failure here (RAM, proot socket issues, OAuth flow
  needing a browser) stops the plan before any code is written.

### Phase 2 — Tool bridge, server side

- `companion/internal/phoneruntime/` gains two loopback endpoints on :9443:
  `GET /v1/agent-tools/list` (walk `registry.Registry`, translate each
  adapter's `manifest.Manifest` — verbs, params, ceiling, consent, cost — into
  a plain JSON tool descriptor) and `POST /v1/agent-tools/call`
  (Resolve → Preview → Execute via the existing `execution.Runner`, credential
  broker round-trip included, result + telemetry back as JSON).
- Auth: bearer token minted at pairing, stored only in the Debian runtime's
  OpenClaw config; loopback-only enforced like every other endpoint.
- Data shapes first: `ToolDescriptor`, `ToolCallRequest`, `ToolCallResult`
  (one Go file, mirrored in the plugin).
- Tests (written first, red before code): unit tests with a fake adapter;
  integration test proving a call round-trips through runner + a stub broker.

### Phase 3 — OpenClaw plugin `operator-tools`

- New top-level `agentbridge/openclaw-plugin/` (TypeScript/Node, OpenClaw
  plugin layout: manifest + runtime module registering one tool per
  descriptor from `/list`, invoking `/call`).
- Tool names/descriptions come from the manifests so the agent sees honest
  ceilings ("sends a Beeper message; delivery confirmed only after readback").
- Tests: plugin unit tests against a mocked bridge; on-device
  `openclaw tools list` shows the adapter tools.
- Proof: in OpenClaw chat, "message <test account> on Instagram saying hi"
  executes through Beeper with no Operator routing involved.

### Phase 4 — First autonomous proof + hard gates

- Go-side policy in the bridge (`agent-tools/call` path): the four hard gates
  above, including per-turn read-set tracking for the cross-adapter
  exfiltration gate; a gated call returns `approval_required` + fires the
  existing approval action to the launcher; approval releases exactly that
  call once (reuse `consent`/approval machinery). The Kotlin-side
  `ReplyGuard`/`DurableStops` stay untouched — they guard the launcher's own
  notification-reply feature, separate from this bridge.
- Rules-file template shipped to the OpenClaw workspace with commented
  defaults.
- Tests first: policy unit tests (new recipient blocked, known recipient
  passes, approval releases once, denial is durable, read-Drive-then-send
  blocked without approval).
- Proof: agent autonomously replies to a known test contact; agent attempting
  a first-contact message stops at the launcher sheet.

### Phase 5 — Turn proxy: launcher becomes the agent's chat

- `phoneruntime` gains an OpenClaw Gateway client (WebSocket, loopback):
  `start_turn`/`steer_turn`/`interrupt_turn` forward to the Gateway;
  Gateway stream maps to existing `event` kinds (activity/reply/failure) and
  task states. The launcher renders it with zero Kotlin changes expected;
  if a mapping gap appears, extend the mapper, not the schema.
- Tests: mapper unit tests (golden transcripts); integration test with a fake
  gateway.
- Proof: typing in the launcher composer talks to the persistent agent; the
  transcript streams; interrupt works.

### Phase 6 — Chat-first UI collapse

Approved direction: the mockup shown and confirmed 2026-08-11 (three screens:
home as task threads, agent-initiated thread, approval as a chat message).
Move it into `outputs/agent-chat-interface-mockup.html` as the source
artifact.

- **DESIGN.md first.** The repo's own rule is document-before-code: amend the
  Core States section (approval sheet, question sheet, capability sheet, and
  "one tap left" as a separate surface are retired; their protocol events
  render as agent messages in the thread with the preview inline; the
  six-mark state system itself is kept), and add the decisions-log rows.
- **Home = thread list.** Each task row shows the last message ("Agent: …" /
  "You: …") plus the existing state dot. No other chrome.
- **No computer/phone distinction in the list.** Codex-on-computer threads
  and phone-agent threads render identically. The distinction lives in
  exactly one place: two new-session buttons, "on phone" and "on computer".
  The computer flow keeps its project/folder pick inside that flow; the
  fixed project selector leaves Home.
- **Asks are messages, but not weaker ones.** Approval and question events
  map to agent messages with inline preview cards and tappable suggested
  replies. Hard-gate approvals follow the release rule above: only the
  tapped structured approve/deny actions resolve them; typed text is always
  steering. Questions (non-gate) may accept typed answers.
- **The ask cannot be scrolled past.** A pending ask pins above the composer
  until resolved, and opening a thread in `Needs your answer` lands on the
  ask, not the latest message. Home keeps DESIGN.md's rule that a
  waiting-for-user task outranks the clock.
- **The six state marks survive.** What retires is the sheet *surface*, not
  the mark system. The one-tap-left half-circle renders on the inline
  preview card and on the thread row exactly as DESIGN.md specifies, and
  its mandated visual separation from `Handed off` carries over unchanged.
- **The preview card carries the sheet's mandatory fields.** Requested
  access, affected paths or `None`, the exact redacted command or outbound
  content, permission duration, and (for computer tasks) computer + project
  — per DESIGN.md's Approval section. Disconnect, timeout, ambiguity, or
  simultaneous pending asks fail closed, same as today.
- Tool activity renders as the existing compact mono lines in-thread.
- Tests: Kotlin unit tests mapping approval/question/activity events to
  message renderings; existing instrumentation tests updated for the removed
  sheets.
- Proof on the Pixel: a hard-gated send arrives as a chat message; typing
  "go ahead" does NOT release it (it routes as steering and the agent
  re-presents the gate) while tapping Approve does; a computer task and a
  phone-agent task sit in the same list, indistinguishable except by their
  content.

### Phase 7 — Triggers: incoming messages wake the agent

- Spike first (one iteration cheap): does the local Beeper server expose an
  event stream? If yes, a small watcher in `phoneruntime` forwards new
  inbound messages to the Gateway as events; if no, poll on the existing
  client. The Android notification listener stays as fallback trigger for
  non-Beeper apps.
- Rules file governs what the agent may do unprompted (default: drafts and
  summaries yes, autonomous sends only to allow-listed people).
- Proof: test account DMs the owner's Instagram; agent wakes, drafts or
  replies per rules, transcript shows the run; owner phone untouched.

### Phase 8 — Delete the predetermined-function pipeline

- Delete `capability/routing/stage1`, `routing/stage2`, `capability/flow`,
  the `classAddressing` table + `classMapFor()` in
  `capability/runtime/production.go`, the `capability_request`/
  `capability_confirm` branches in `protocol/schema/action.schema.json` and
  `app/mobilesession/handler.go`, and the Kotlin capability-sheet entry path
  that sent `capability_request`.
- Keep: registry, adapters, runner, manifests, consent/approvals, journal,
  credential broker — they are the tool surface now.
- Migrate-and-delete in one wave; grep for orphaned references
  (`capability_request`, `stage1`, `classAddressing`) across Go, Kotlin,
  schema, docs, release checks; update the protocol contract tests on both
  sides (Go `contract_test.go`, Kotlin `ProtocolContractTest.kt`).
- Proof: full matrix green; the launcher's only brain is the agent.

### Phase 9 — Boot persistence + battery survival

- runit service for the Gateway next to the existing phone-runtime service;
  Termux:Boot starts Termux + services on device boot.
- Termux's own survival is a named deliverable: Termux is a separate app/UID
  from the launcher, so the launcher's foreground service does nothing for
  it. Battery-optimization exemption granted to Termux itself, plus
  `termux-wake-lock` held by the boot script; both steps written into the
  install doc.
- Proof: reboot the Pixel, touch nothing, agent answers a message and fires
  its heartbeat within N minutes. That short reboot check is the gating
  check and loops until clean; the 24h-idle soak then runs once, repeated
  only if the soak itself fails (per the long-soak exception in
  Constraints).

### Phase 10 — Open-source packaging + install prompt

- README rewritten around what it now is: a persistent agent that lives in
  your phone, open-source, self-hosted, honest about flakiness and about what
  the notification listener and messaging access mean.
- **README format (owner decision 2026-08-11), modeled on
  [hindsight](https://github.com/aadivyaraushan/hindsight) and
  [pgGraph](https://github.com/Evokoa/pgGraph):** problem-to-solution order —
  banner + badges; a hook that motivates the problem through a concrete,
  relatable scenario (not abstract positioning); "how it works" as a diagram
  plus numbered steps; aggressively minimal install (the one-paste prompt
  front and center); a safety/permissions table stating plainly what the
  agent can touch and what stops it (the hard gates); FAQ, contributing,
  license. Conversational but precise; honest about limits over hype.
- `scripts/install-phone-agent.md`: the paste-into-any-LLM prompt (and a
  plain shell script it wraps) that takes a stock Pixel from Termux install →
  Debian → Node → OpenClaw → plugin → APK pairing.
- Sanitize pass before push (no tokens, no owner accounts, no device serials
  in committed files).
- Proof: the install doc replayed on a clean proot (fresh `proot-distro
  install debian` alongside the live one) reaches a healthy gateway.

### Final deliverable (owner decision 2026-08-11)

What comes back to the owner at the end is **the GitHub repo with
everything in it**: all code, the rewritten README, the install prompt,
docs, and `saved-results/` evidence, committed and pushed to the existing
remote (`github.com/aadivyaraushan/codex-launcher`) after the phase 10
sanitize pass and the phase 11 judged proof. The hand-back is that repo
URL, in a state where a stranger landing on it can read the README and
install the agent on their own phone.

### Phase 11 — End-to-end judged proof

- A fresh judge agent derives, from first principles, what a persistent
  phone agent must demonstrably do (unprompted action under rules, gated
  action stopping at approval, reboot survival, no credential in Termux,
  loopback-only) and then attacks the evidence from phases 1–10.
- **The plan's acceptance gate (owner decision 2026-08-11):** everything this
  plan claims — every Scope item and every phase proof — is demonstrated
  working on the phone from a real-user perspective, in one continuous
  session: pick up the phone, use it like a person who just installed this,
  and every claimed behavior happens. Any step that doesn't survive that
  walkthrough is a failed gate, gets fixed, and the walkthrough reruns.
- Findings fixed or explicitly accepted by the owner; verdict recorded in
  `saved-results/`.

## Verification (project level)

- `go test ./... -race -count=1`, `go vet ./...` — every phase.
- `./android/gradlew -p android testDebugUnitTest lintDebug` — phases touching
  Kotlin or the protocol.
- `node release/checks/public-alpha-release.test.mjs` — phases 8 and 10.
- Plugin: `npm test` in `agentbridge/openclaw-plugin/` + `openclaw tools list`
  on device.
- Live proofs run on the owner's Pixel (accepted substrate per the 2026-08-06
  binding delta), which stays connected throughout. Every proof is a
  real-user walkthrough per the Constraints bullet: the implementing session
  drives screen-level input over adb (taps, swipes, typed text on the
  rendered UI, screenshots to see), from the home screen, fixed and rerun
  until it works. Where a step needs a human hand (an OAuth consent screen,
  a fingerprint), the owner is asked to do exactly that step and nothing
  more. Walkthrough scripts, per-step timestamped screenshots, screen
  recordings for end-to-end proofs, outcomes, and fix logs land in
  `saved-results/`.

## Implementation guidance (for the implementing session)

- TDD order per repo rules: restate observable done → failing tests shown red
  → minimum code → green shown. Bug fixes reproduce first.
- Worktree isolation for every code phase; trivial doc edits in place.
- pstack: run the **how** skill over `phoneruntime` and `capability/runtime`
  before phase 2; **architect** before phases 2 and 5 (types cross function
  boundaries); **deslop** before each commit; **show-me-your-work** decision
  trail for the multi-day run; **babysit** after each PR.
- Delegation: implementation code to Sonnet subagents in worktrees; wide
  reads to Haiku; orchestrator writes tests and confirms red/green.
- Logging: bridge and proxy log inputs/branches/outputs per repo logging rule,
  tagged `[agentbridge]`, no message bodies or tokens at info level.
- Every fix greps for its siblings before closing.

## Open questions (owner, non-blocking until their phase)

1. Phase 7: if Beeper exposes no event stream, is polling every ~15s an
   acceptable battery cost, or should notification-listener triggers carry it
   alone until Beeper adds one?
2. Phase 10: publish the OpenClaw plugin to ClawHub too, or repo-only at first?
3. After phase 8: does the Mac/relay Codex path stay in the README as a
   first-class feature or move to an appendix as legacy?
