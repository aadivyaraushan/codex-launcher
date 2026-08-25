# Capability threads: chat UI, reply-to-AI, open-app-page tool

2026-08-07 · owner decisions locked in this session: threads replace the alert dialogs
for ALL capability tasks; the thread composer talks to the AI (it is NOT a direct
message-send box); threads persist like desktop tasks; open-app-page ships
best-effort for all connected apps with plain-app-launch fallback.

## The shape of the change

```
TODAY                                   TARGET
-----                                   ------
Home query                              Home query
  |                                       |
  v                                       v
capability_request ---> phone runtime   capability_request (UNCHANGED envelope)
  |                        |              |  app navigates to thread page NOW,
  v                        v              |  taskId := actionId it just generated
AlertDialog preview   route+adapter       v                    |
  | confirm                             phone runtime creates THREAD TASK
  v                                     with id = that same actionId (sqlite),
AlertDialog result                      routes utterance -> adapter
(gone on dismiss,                         |
 nothing persists)                        v
                                        Thread page (existing TaskScreen shell)
                                          | user line   <- transcript entries
                                          | message rows (sender + text + links)
                                          | agent lines  <- via task_page/events
                                          | confirm sheet over thread (preview)
                                          | question sheet over thread (QUESTION)
                                          v
                                        composer at bottom -> start_turn
                                          existingTask {taskId} -> routed as next
                                          capability in the SAME thread
```

**The taskId linkage (load-bearing, no wire change):** the client already generates
the `actionId` for every `capability_request`, and the runtime echoes it back as
`requestId` in preview/result (`handler.go:947-952`, `flow/service.go:82`). The
thread task's id IS that actionId. The app navigates to the thread page at submit
time using the id it generated — it never needs to learn the taskId from the wire,
and `capability_preview`/`capability_result` stay byte-identical. A follow-up turn
gets a fresh actionId; the runtime maps it to the same thread because it arrived
via `start_turn` existingTask {taskId}.

Key discovery making this cheap: the phone runtime already embeds the full
`mobilesession.Handler` with snapshot/task_read/task_page/live-event support, but
passes a **nil TaskSource** (`companion/internal/app/mobilesession/handler.go:183-184`,
`taskCapable=false`). Implementing a phone-side TaskSource backed by a thread store
lights up the existing Android task list, TaskScreen transcript, and composer with
almost no new protocol.

## Feature map (what the user asked -> what we build)

| Ask | Build |
|---|---|
| Messages in rows like a texting app, sender + formatted links | New wire transcript entry kind `message` {sender, text, timestamp}; Android renderer row (sender header, selectable text, clickable links) |
| Page with a reply box, "kinda like a chat" | Existing TaskScreen + TaskControls composer, reused; composer sends follow-up instruction to the AI for that thread |
| Every task opens a conversation thread, like desktop | Phone runtime creates a task per capability request; app navigates to TASK destination instead of showing dialogs; dialogs deleted |
| Works for all apps, not just Instagram | The thread flow wraps the capability pipeline itself (router -> adapter -> preview -> result), so every adapter inherits it |
| New tool: "open my unread DMs page on Instagram" | `open_page` verb: deep-link registry (measured links) + new device_action kind `open_page` fired package-pinned by the Android app; fallback = plain app launch |

## Protocol changes (fixture-bound: Go validation + Kotlin codec + protocol/fixtures/, one coordinated phase)

The contract is exact-keys on BOTH sides (`contract/validation.go:574-590` +
`ProtocolCodec.kt:175,184`), so each change lands in all three places at once:

1. **New transcript entry kind `message`**: `{id, turnId, kind:"message", sender, text, sentAt}`
   — joins "user"/"agent"/"reasoning"/"plan"/"activity"/"command"/"file_change"
   (`validation.go:876-890`). Carries one row of a read result.
2. **New device_action kind `open_page`** — the envelope shape does NOT change.
   `device_action` stays `exactKeys(requestId, kind, handle, text)`
   (`validation.go:598`); the new kind reuses the slots the way
   `notification_reply`/`youtube_play` already do (`handler.go:1248,1254-1255`):
   `handle` = app slug from the registry, `text` = the registry URL. The URL comes
   from the Go registry only, never from user text; Kotlin re-validates handle+URL
   against its own allowlist and fires a package-pinned ACTION_VIEW (pattern:
   `YouTubePlaybackAction.kt:60-63`).
3. **No change** to capability_request/preview/result/confirm envelopes in v1. The
   preview/confirm still flows on the existing envelopes; the app renders it as a
   sheet over the thread page (matches the approval-sheet vision in
   `outputs/codex-launcher-visual-directions.html`). Thread membership needs no wire
   field — see the taskId linkage above.
4. **QUESTION phase** (router asks e.g. "which conversation?"): arrives as
   `action_result` state=cancelled + question (`handler.go:958-978`,
   `CapabilityInteraction.kt:369`) — a path with no preview and today no way to
   answer. Because the app navigates to the thread page at submit time (before any
   runtime reply), the question renders as a question-sheet over the thread page
   (the mockup's question-sheet), and the runtime appends it to the thread as an
   `agent` entry so it persists. Answering = the sheet submits a follow-up turn
   with the answer text into the same thread. No envelope change.

## Phases (each ends green before the next starts)

**P1 — Protocol: `message` entry + `open_page` device action.**
Go validation + Kotlin ProtocolCodec + fixtures (`protocol/fixtures/*.jsonl`,
schema-drift negatives). Lever: extend the wire-contract round-trip tests on both
sides. Done = Go `contract` tests + Kotlin `ProtocolContractTest` green.

**P2 — Phone runtime: thread store + TaskSource.**
Sqlite-backed store of threads (id, title, created, entries). Wire it as the
handler's TaskSource/TaskTranscriptSource/NewTaskSource/ExistingTaskSource.
Capability flow appends entries: user utterance -> `user`; read results -> `message`
rows (beeper.Message already has sender/text/timestamp — structure survives instead
of being flattened to "sender: text · …"); other results -> `agent` text; failures
and router questions -> honest `agent` text. **Follow-up fork:** the phone runtime's
TaskSource implements `ExistingTaskSource.StartExistingTurn` (`handler.go:73`) by
bridging into the capability flow — it mints a fresh actionId, calls the same
`flow.Prepare` the capability_request path uses, and appends the resulting entries
to the named thread; it never touches a coding-agent turn engine (none exists on the
phone). Done = Go unit tests incl. real-body EncodeText levers (the
adapter-fields-only-break-on-the-wire lesson), threads survive runtime restart.

**P3 — Android: thread page replaces dialogs.**
Home query -> new-task action -> navigate to TASK. `message` entries render as
texting-app rows (sender header, body, clickable links, theme tokens from
QuietInstrumentTokens). Preview/confirm = bottom sheet over the thread; QUESTION =
question sheet over the thread (protocol point 4); EXECUTING = activity row;
RESULT lands as transcript entries. Delete `CapabilitySheet` dialog
mounting (fix-in-place rule: old path removed). This also retires the wrong
"Replied" header for reads (`CapabilityOutcome.of` StateMark). Done = unit +
androidTest green (extend CapabilitySheetTest -> thread tests, TaskScreenTest).

**P4 — open_page tool.**
Go: extend the existing `deeplink` adapter (read it first; do not build a sibling)
with a page registry per app — measured links only: `instagram://direct-inbox`
(inbox, screen-verified), `instagram://user?username=<u>` (profile), `spotify:home`,
`spotify:playlist:<id>`, `whatsapp://send`, `slack://open`, `todoist://today`,
`notion://www.notion.so`, `ms-outlook://emails`. Unknown page or unresolvable link
-> plain app launch (existing handedOffTo path). Discord/Gmail/Google Chat get app
launch only (no working schemes — measured). Kotlin: `open_page` handler with
per-app allowlist + package pin. Done = router sends "open my unread DMs on
Instagram" -> open_page(instagram, direct-inbox); on-device: inbox actually opens.

**Confirm-sheet state is deliberately ephemeral.** A pending, unconfirmed preview
lives only in the client state machine; killing the app mid-confirm drops it (the
runtime's preview expires server-side). On reopen the thread shows entries up to
the last result and the composer works; the user re-asks if they still want the
action. Persisting pending confirms into the transcript is a v2 decision for the
owner, not silently added here.

**P5 — Carry-over structural fixes (small, independent).**
(a) Validate capability_result body BEFORE journaling (handler.go ~:1045) so an
invalid result can never brick warm-resume again. (b) The 5 same-class empty-detail
adapters (applereminders:280, applenotes:283, gcalendar:165, gdrive:150,
outlook:188). Done = red-then-green tests per fix.

**P6 — On-device acceptance.**
Redeploy runtime + install app on the Pixel (adb currently disconnected — needs the
cable/wireless re-attached). Script: unread-Instagram read -> thread with rows ->
follow-up "reply to <name> saying X" -> preview sheet -> confirm -> sent via Beeper;
"open my unread DMs page on Instagram" -> inbox opens; restart app -> thread still
listed; restart app WHILE a confirm sheet is pending -> thread reopens cleanly, no
sheet, no crash, next follow-up works. Evidence: screenshots + runtime log lines,
appended to `saved-results/ux-misroute-decision-trail.tsv`.

## Risks the judge should attack

- Desktop-task semantics bent onto capability threads: model/reasoning pickers,
  project scoping, stop/interrupt buttons in TaskScreen may not fit a phone thread.
  Mitigation: TaskSource controls what snapshot advertises; hide computer-only
  affordances when the task's origin is the phone runtime.
- Preview-as-sheet vs preview-as-entry: sheet matches the mockup and reuses the
  state machine; if the owner wants confirms inline in the transcript later, that is
  a v2 change on top, not a rewrite.
- Live updates: does task_page refresh mid-conversation need the event stream
  (`live_events`), and does the phone runtime emit task events today? P2 must verify
  and, if silent, emit task-updated events on entry append.
  - RESOLVED (P2 verification, 2026-08-07): no server push needed. The Android app
    already polls task_read on an interval for any open task page
    (LauncherSessionViewModel.kt:873-884, startTranscriptRefresh), so an open thread
    picks up follow-up rows by itself. PublishTaskEvent cannot serve thread tasks
    (they are outside the handler's journaled snapshot); do not wire it.

## Judge findings carried into P3 (from the P2 review)

- Follow-up preview delivery: PublishCapabilityPreview pushes an unsolicited
  capability_preview keyed by a SERVER-minted turn id, but the Kotlin
  CapabilityInteraction.acceptPreview only accepts a preview whose requestId matches
  the phone-generated id of an in-flight request (CapabilityInteraction.kt:249-253).
  P3 MUST teach the phone to accept previews for turn ids it learned from
  start_turn/thread context, or the whole follow-up confirm path is unreachable.
- A follow-up preview generated while the phone is disconnected is dropped (no
  active connections, no retry). P3/P6 decide: acceptable (user re-asks) or the
  store re-emits on reconnect. Note it in the acceptance script either way.
- Concurrency: two simultaneous turns on one thread interleave through the inner
  flow with no ordering guarantee. Accepted for v1 — same behavior as two
  simultaneous capability requests today; sqlite serializes the writes.
- Undocumented deep links can rot: open_page is explicitly best-effort with app
  launch fallback; registry is one table, cheap to re-measure.
- Follow-up routing has no thread context in v1 (each utterance routed fresh).
  "Reply to her" with no name may misroute; acceptable v1, noted to owner.

## Not in scope

Direct-send composer (owner explicitly rejected), Instagram Stories/App-ID work,
any Meta developer registration, https app-links (chooser problem), changing the
Beeper send route.
