# RT-4: making the notification-reply stack reachable

**Date:** 2026-08-03 (rewritten after judge review)
**Status:** design, not yet built. Gap 1 is in build; Gap 2 is not.
**Why now:** the whole RT-4 stack is written, tested and unreachable. The
consumer-app plan wants "runtimes proven end to end — RT-1, RT-4, RT-6" (plan
line 1160). RT-4 cannot be proven because nothing can trigger it.

> **First draft was wrong and was thrown out.** It had the companion block
> inside `flow.Confirm` waiting for the phone to answer. A judge with fresh
> context found that this can never work, and I confirmed it in the code: the
> companion reads one frame at a time and handles it on the same goroutine
> (`internal/mobileapi/transport/server.go:398`), holding `publishMu` for the
> whole of the action (`handler.go:673`). The phone's answer can only be read
> by the *next* turn of that loop, which cannot start until the wait returns.
> So the wait would time out 100% of the time. The section "How the answer
> actually gets back" below is the replacement.

---

## What "unreachable" means here, precisely

Every one of these was checked by grep over `android/app/src`, 2026-08-03:

| Piece | State |
| --- | --- |
| `ReplyAdapter` (the rules) | written, 9 unit tests, **no caller in `main/`** |
| `ReplySender` (the guard) | written, 6 unit tests, **no caller in `main/`** |
| `AndroidReplyDispatch` (the send) | written, **never constructed anywhere, including tests** |
| `LiveReplyActions` (the store) | written, tested, held by the service |
| `NotificationProbeService` | runs; `liveReplyActions` is a public `val` at line 51 that **nothing outside the class reads** |
| the service itself | referenced only by `AndroidManifest.xml:36` — no class names it |
| the wire | **no message about notifications, sightings or replies exists** in either direction |

So there are two separate gaps, and they are worth fixing in that order:

1. **Inside the phone**: nothing can get at the running listener service.
2. **Across the wire**: nothing can ask the phone to reply.

---

## Gap 1 — inside the phone

```
  BEFORE                                   AFTER

  NotificationProbeService                 NotificationProbeService
   +-- liveReplyActions  (public,           +-- onListenerConnected:
   |     read by nobody)                    |     builds AndroidReplyDispatch,
   +-- sightings (kept internal)            |     installs it
                                            +-- onListenerDisconnected:
   [ nothing else in the app ]              |     releases it
                                            +-- onDestroy:
                                            |     releases it too
                                            v
                                         DeviceNotificationAccess
                                          (one process-wide holder)
                                            ^
                                            |  reads
                                            +-- anything in the app process
                                                that needs a reply box
```

A `NotificationListenerService` is constructed by Android, not by the app, so
there is no constructor to inject into and no useful binder. The ordinary
answer is a process-wide holder the service fills in on `onListenerConnected`
and empties when it goes away.

**Two rules make it safe.**

*Empty means no reply box, never "not checked yet."* A holder keeping a service
Android has already torn down hands out permission slips for notifications that
no longer exist — the exact failure `DeliveryResult.NOTIFICATION_GONE` exists
to report honestly.

*Release is identity-checked.* Android may build the replacement service before
destroying the old one. If the outgoing instance's teardown clears the holder
unconditionally, it wipes the **new** instance's registration, and from then on
every reply on a perfectly healthy phone answers "that notification is gone"
forever, with nothing in the logs to say why. So `release(x)` clears only when
`x` is the same object that is currently held.

`onDestroy` is its own override, not folded into `onListenerDisconnected`.
Android does not promise to deliver the disconnect before teardown, and a
teardown with no preceding disconnect would leave the holder pointing at a dead
service.

**Tests:** `DeviceNotificationAccessTest` (7 tests, on a laptop — no Robolectric
in this build) covers empty-at-rest, install, release, release-when-nothing,
reconnect-replaces, stale-release-is-ignored, and concurrent read-while-install.
The service wiring itself needs one instrumented test on the phone, in the style
of `LocalStateWiperInstrumentedTest.kt`.

**Deliverable:** `AndroidReplyDispatch` gets constructed for the first time, and
`ReplySender` gets its first caller.

---

## Gap 2 — across the wire

Routing needs the language model, which lives on the Mac. Execution needs the
notification, which lives on the phone. So a reply is the first capability whose
decision and whose action are on different machines.

### Who decides which conversation

**The phone does.** The first draft had the companion send a
`conversationKey` — but that key is Android's own per-notification `sbn.key`
(`NotificationProbeService.kt:182-196`), created and thrown away entirely on the
phone. It has never crossed the wire, and the companion's routing only ever
resolves a contact handle from the contact graph
(`internal/capability/routing/stage2/resolver.go:172`). The companion cannot
name a conversation it has never heard of.

The alternative — a new phone→companion message that syncs the live notification
ledger — means shipping a running list of who has messaged you to the Mac just
to pick a target. That is a lot of new surface, and it is exactly what
`ReplyAdapter.pick` already does on the phone.

```
  companion sends:  { contact handle, text }        <- names a person
  phone resolves:   ReplyAdapter.pick(handle)       <- names a conversation
                    -> one match      : send
                    -> none, or two   : decline, do not guess
```

`pick` already declines rather than guessing between two conversations, for the
same reason the contact graph does. That behaviour is the answer here, not a gap.

### How the answer actually gets back

Not by waiting. `capability_confirm` returns straight away with a *pending*
acknowledgement, and the real answer arrives later on its own.

```
  PHONE                                    COMPANION (Mac)

  "reply to Maya: on my way"
        |
        +-- action: capability_request ------->  stage1 -> stage2
                                                      |
                                              notification_reply adapter
                                              Resolve + Preview run here.
                                              Execute does NOT run here.
        <-- capability_preview ----------------------+
        |
   user confirms
        |
        +-- action: capability_confirm ------->  flow.Confirm
        |                                             |
        |                                    records "waiting on device"
        |                                             |
        <-- action_result: accepted ------------------+   (returns at once;
        |                                                  read loop freed)
        <-- device_action ---------------------------- +
        |    {requestId, kind:"notification_reply",
        |     handle, text}
        v
   ReplyAdapter.pick -> ReplySender -> AndroidReplyDispatch
        |
        +-- device_action_result ------------->  new case in Handle()
             {requestId, outcome:                      |
              delivered | notification_gone            | completes the
              | failed | refused}                      | waiting record
                                                       v
        <-- capability_result ------------------ queueDelivery (async path)
             {ceiling, done, detail}
```

The one thing that makes this work: `device_action_result` is handled as an
ordinary inbound frame, and the `capability_result` it produces goes out through
the existing asynchronous `queueDelivery` / `deliverBroadcasts` path
(`handler.go:1713-1751`) — the only path that legitimately runs off the read
loop. Nothing ever blocks the loop that has to read the phone's answer.

### The state nobody owns yet

`flow.Service.Confirm` deletes its pending-preview entry before calling
`Execute` (`service.go:156`). Once execution needs a phone round trip, something
new has to remember "request X is waiting on device Y". It needs a name, its own
lock, and three cleanup rules:

- the phone disconnects while waiting → the request ends as `outcome_unknown`;
- a second result for the same request → ignored, not applied twice;
- nothing arrives before a hard timeout → `outcome_unknown`.

`outcome_unknown` — never `done: true`, and never `done: false` either, because
a reply that timed out may well have been sent.

**It got a name: `internal/capability/devicework`, holding a `Ledger`.** Its
whole job is answering "is this request still outstanding, and who owes us an
answer". It holds no adapters, sends nothing, and knows no wording — so the
three rules above can be tested without a socket, a phone, or a real second
passing.

```
   Wait(requestId, deviceId, kind) -> bool    false if already waiting
   Settle(requestId)  -> (Record, bool)       true for the FIRST caller only
   DeviceGone(deviceId) -> []Record           that phone's waits, nobody else's
   Expired()            -> []Record           past deadline, each returned once
```

Two decisions inside that are easy to get backwards:

- **The clock is passed in, and nothing in here starts a goroutine.** Sweeps are
  driven by the caller. A package that owns its own ticker cannot be tested at a
  chosen moment, so the timeout rule would end up either untested or tested by
  actually sleeping.
- **`Settle` is a claim, not a lookup.** The read loop, the delivery goroutine
  and the timeout sweep can all reach one request at once. Exactly one may win.
  The race that matters is a late answer arriving *after* the timeout already
  told the user we could not find out: accepting it would emit a second
  `capability_result` and overwrite an honest "we don't know" with a claim.

### The two new frames, and what they deliberately leave out

| | `device_action` | `device_action_result` |
|---|---|---|
| direction | Mac → phone | phone → Mac |
| body | `{requestId, kind, handle, text}` | `{requestId, outcome}` |
| sequenced | no | no |

- **No conversation id.** Covered above: the Mac has never seen Android's
  notification key and cannot name a conversation. It names a person.
- **No wording in the result.** One word for what happened, nothing else. The
  sentence the user reads is built on the Mac beside every other capability's
  sentence, so the phrasing for "we don't know" cannot drift between the two
  machines — which is the failure this whole round has been fixing.
- **Neither is sequenced.** A `device_action` asks for something to happen
  *now*. One the phone missed while offline must expire, not fire late into a
  conversation that has moved on; the ledger's timeout ends it, not a redelivery.
- **Four outcome words: `delivered | notification_gone | failed | refused`.**
  `notification_gone` is separate from `failed` on purpose — nobody caused it
  and nobody can retry into it, because the conversation moved on.

**Open, with a stated default.** `ReplyPlan` has a third variant, `HandOff`,
and the four words above have no place for it. Default taken: report it as
`refused`, which is literally true (we did not send it) and keeps the result
body at two keys — a `handed_off` word would need a fifth field for the app
label, because `capability_result` rejects `ceiling: "hands_off"` with an empty
`handedOffTo`. To settle when item 8 is built: read what actually makes
`ReplyAdapter.plan()` return `HandOff`, and check whether that can happen at all
on a path that started from a notification with a working reply box.

### Saying "we could not find out" — second attempt

> **The first attempt was also rejected.** It proposed one optional field,
> `certain`, on `capability_result`, defaulting to true. A judge with fresh
> context took it apart and was right on every count. What follows is what
> survived.

**Why the `certain` bolt-on failed.** The proposal assumed a new wire field
would reach the user as "unknown". It would not. `acceptResult`
(`CapabilityInteraction.kt:249-256`) builds the outcome from `ceiling`, `done`,
`detail` and `app` only, and `CapabilityOutcome.of`
(`CapabilityOutcome.kt:148-165`) has an unconditional `if (!done)` branch that
sets `mark = FAILED`, `claimsFailure = true`, `recoveryAction = "Try again"`. So
`{certain: false, done: false}` would have rendered as a red **Failed / Try
again** — the exact false claim the whole feature exists to prevent — and
`ConnectionNotificationPolicy.kt:32` would have pushed "Codex needs attention"
on top of it.

**And here is the thing worth stopping for.** `StateMark.UNVERIFIED` already
exists (`CapabilityOutcome.kt:90`) and means precisely "we do not know". `.of()`
has never produced it, and `toTaskState()` (`CapabilityOutcome.kt:107-119`)
carries a comment asserting it never will. **The third state was built years-deep
into the phone's own model and left unreachable** — the same failure this whole
plan is about, one layer further in.

So the wire is not where this starts.

```
  WRONG ORDER (rejected)          RIGHT ORDER
  add wire field                  1. make UNVERIFIED reachable on the phone
       |                             (CapabilityOutcome.of gains `certain`;
       v                              certain=false -> UNVERIFIED, never FAILED)
  hope the screen                 2. fix every reader that branches on `done`
  does the right thing               alone (list below)
       |                          3. only then add the wire field
       v
  screen says "Failed"
```

**Step 2's readers, all of which today infer failure from `done` alone:**

| Where | What it decides |
| --- | --- |
| `CapabilityOutcome.kt:148` | `of()` has no `certain` parameter at all |
| `CapabilityOutcome.kt:112` | `FAILED -> TaskState.FAILED` |
| `CapabilityInteraction.kt:250` | reads `done`, feeds `of()` |
| `LauncherSessionViewModel.kt:528` | whether the user's draft is thrown away |
| `ConnectionNotificationPolicy.kt:32` | whether to push "needs attention" |

**Two corrections to this plan's own earlier reasoning.**

*The reason given for rejecting `action_result` was false.* A successful
`capability_confirm` sends **zero** `action_result`s today — it goes straight to
`capability_result` (`handler.go:912-941`), and `action_result` appears only on
the failure path. There was no "already spent" ack to be blocked by; that ack was
something this plan itself invented. The real reason to reject `action_result` is
different and better: it has no `detail`, `ceiling` or `handedOffTo` fields
(`validation.go:591-601`, `ProtocolCodec.kt:190-199`), so it structurally cannot
carry the sentence a person needs to read.

*The reason given for rejecting a tri-state `done` was not a reason.* "Every
consumer must be revisited" is a cost, not a defect — and as the table above
shows, this plan pays that identical cost anyway. Between a `certain` bolt-on and
a genuine third state, cost no longer decides it. Pick on honesty: the phone's
model already has three states, so the wire matching it is the smaller lie.

### The conflict is settled, and by something already shipped

> **Third correction to this plan.** It was about to invent a `certain` boolean.
> This project already answers "we could not find out", end to end, in
> production, and has for a while — just not for capabilities.

Grep for `outcome_unknown`. It is a **peer terminal state** on `action_result`,
sitting beside `confirmed`, `failed` and `cancelled`
(`ProtocolCodec.kt:583`), with its own transitions (`:756-757`), its own
`TaskQueueState` (`TaskSummary.kt:36`), its own Go side
(`promptqueue/queue.go:86`, `handler.go:716`), and a screen that renders it
(`TaskControls.kt:79`). Four independent readers on the phone branch on it.

And look at what that screen actually does, because it is the whole rulebook:

```
  TaskControls.kt:65-90 — the shipped "we don't know" behaviour

  1. say it plainly ....... "Queued follow-up outcome unknown."
  2. name where to check .. "Check Codex on your computer before sending another."
  3. BLOCK the retry ...... followUpsBlocked = queueState == OUTCOME_UNKNOWN
  4. let them close it .... [ I checked Codex ]  -> clears the state
```

Never "try again" — *check*, then *acknowledge*. Item 3 landed on step 1 and
step 2 for capabilities without knowing this existed; **steps 3 and 4 are still
missing on the capability path**, and that is now the honest next piece of work.

So the three open questions all resolve the same way — by following the
precedent rather than inventing beside it:

| Question | Answer, from `outcome_unknown` |
| --- | --- |
| Boolean `certain`, or a third state? | **A third state.** The house pattern is a peer terminal state. A flag would give capabilities a second, incompatible vocabulary for the thing actions already say. |
| May an uncertain result name the app? | **Yes, and the conflict was imaginary.** Naming an app while uncertain is *recovery guidance* ("Check Signal"), not a hand-off *claim*. `CapabilityOutcome.of` already keeps these apart: it uses `app` for the recovery sentence and leaves `handedOffToApp` null. `handedOffTo` on the wire keeps meaning "control definitely went here" and stays gated on `done`. Nothing needs to change. |
| Is the wire change additive? | **No.** `capability_result` validates with `body.keys != setOf(...)` — an *exact* key set (`ProtocolCodec.kt:184`). Any new key is rejected by an unpatched peer, so phone and companion must change together. This is a version-gated change, not a soft one. |

**When it does reach Go:** if a boolean survives anywhere, it is `Certain *bool`,
never a bare `bool` — `handler.go:369` already uses the pointer form for exactly
this reason: a bare `bool` with `omitempty` silently drops `false`, and `false`
is the one value here that must never vanish.

## Order of work

| # | Item | Size | Risk | State |
| --- | --- | --- | --- | --- |
| 1 | Gap 1 — holder + first construction of `AndroidReplyDispatch` | small | low, phone-only | **done** |
| 2 | Settle the hand-off conflict: may an uncertain result name the app it went to? | design | — | **done** — answered by `outcome_unknown`; see the table above. Yes, and no contract change is needed |
| 3 | Make `StateMark.UNVERIFIED` reachable — `CapabilityOutcome.of` gains `certain`, with a test proving `certain=false` can never produce `FAILED` or `claimsFailure` | small | low, phone-only, no wire change | **done** — 405 tests, 0 failures |
| 3b | Match the rest of the shipped `outcome_unknown` behaviour on the capability path: `sessionLost()`, **block the retry**, **"I checked"** | small | low, phone-only | **done** — 415 tests, 0 failures |
| 4 | Only clear the user's draft on `claimsSuccess`, not on `!claimsFailure` — the old rule deleted it after a one-tap that had sent nothing and after a hand-off we could not see | one line | low | **done** — 419 tests, 0 failures |
| 4b | **The pending check must survive a reconnect and a destination switch.** Found by a fresh-context judge, then confirmed in the code. Two CRITICALs and one HIGH, all one root cause | small | **high — this is the bug 3b exists to prevent** | **done** — 423 tests, 0 failures |
| 4c | ~~Wire capability outcomes into `TaskSummary` so `toTaskState()` has a caller~~ → **one rule joining the two names for "we lost track of it"**: `TaskSummary.effectiveState()`, read by the notification policy and the home row. The item as written was aimed at the wrong function — see below | medium | low | **done** — 439 tests, 0 failures |
| 5 | ~~The third terminal state on the wire~~ → **stop flattening the companion's ambiguous failures into "failed"**. No contract change after all — see below | medium | low, **no wire change** | **done** — all 9 adapters wired, full Go suite green |
| 5b | `flow.Service.Confirm` threw away a known-good outcome when telemetry bookkeeping errored (`runner.go:205-207`) | one line | low | **done** — fixed in the runner; the bookkeeping failure is logged, the real outcome is returned |
| 6 | `device_action` / `device_action_result` on both codecs, carrying a contact handle — not a conversation key | medium | medium, contract change | **done** — 9 tests each side, same fixtures, 448 tests / 0 failures |
| 7 | The "waiting on device" record + the new `Handle()` case, with its three cleanup tests | medium | medium | **done** — ledger 12 tests + wiring 10 tests, race-clean; 82 packages ok, 0 FAIL, `go vet` clean |
| 8 | Phone side: `device_action` → `ReplyAdapter.pick` → `ReplySender` | ~~small~~ **medium** | low | **done** — 481 tests / 0 failures. The chain is reachable end to end: `NotificationProbeService.kt:255` is the first and only place `ReplyHandle` is constructed, and `LauncherApplication.kt:37` passes the real reply path into the session |
| 9 | The proof on the Pixel | — | **owner-blocked**: needs a real inbound message and explicit permission to send to a real person | blocked |

The order changed after the second judge review. Items 3 and 4 are now **ahead**
of every wire change, because a wire field the screen ignores is worse than no
field at all — it looks like the honesty problem is solved while the screen still
says "Failed". Both are phone-only and change no contract, so they are safe to
build before item 2 is settled; item 5 onward is not.

Item 9 cannot be done without the owner. Items 1–8 make it a single action on the
day the owner is available, which is the point of doing them now.

---

## Item 4b — the warning that erased itself

A judge with fresh context, given no hint about what to look for, read item 3b's
work and found that the retry block — the part with teeth — could be cleared
without anyone checking anything.

```
  connection dies
        |
        v
  fail()  --> sessionLost()      sets the pending check, blocks the retry
        |
        +--> scheduleRetry()     ~1 second later
                  |
                  v
           startConnection(force = true)
                  |
                  v
           closeCurrent(invalidate = true)
                  |
                  v
           capabilityController.clear()   <-- rebuilds the whole state
                  |
                  v
           pending check gone, retry unblocked, no one checked anything
```

So the banner appears for about a second and vanishes on its own. The next
prompt goes out. That is a duplicate message to a real person, produced by the
feature built to prevent duplicate messages to real people.

Three ways in, one cause:

| Where | Why it wipes the check | Severity |
| --- | --- | --- |
| `CapabilityInteraction.clear()` (`:365`) | reached by automatic reconnect, one second after the drop | CRITICAL |
| `setDestination()` (`:87`) | guards only on `busy`, and `RESULT` is not busy; the toggle sits on the home screen and is always tappable | CRITICAL |
| `respond()`'s NOT_SENT rollback (`:173`) | `.copy()` off the *current* state, so a send that fails after a newer answer landed forces `phase` back to `PREVIEW` while an "outcome unknown" banner is still up — and claims "Nothing was changed", which is the one thing we just said we could not know | HIGH |

The cause is the same in all three: `unresolvedCheck` lives **inside**
`CapabilityInteractionState`, and several methods replace that state wholesale.
The fix is to move it out of the wholesale replacement so **only `markChecked()`
can clear it** — which closes all three doors and any future one, instead of
patching three call sites.

Spec: four tests appended to `CapabilitySessionLostTest.kt`. The third one is
reproducible without threads, because `sendAction` is called *outside* the lock
— a test's send lambda can drop the session and then report NOT_SENT, forcing
exactly the interleaving.

## Item 5, rescoped — the wire was never the problem

The plan spent three rounds designing a third terminal state for
`capability_result`. It turns out the wire already carries one, and the
companion already knows how to send it:

```
  errorCodes (ProtocolCodec.kt:591) ......... includes "outcome_unknown"
  action_result state set (:198) ............ includes "outcome_unknown"
  companion already emits that exact shape ... handler.go:1400-1401
      result["state"] = "outcome_unknown"
      result["error"] = {"code": "outcome_unknown", "retryable": false}
```

**No contract change. Nothing to negotiate between phone and companion.**

The real problem is one line on the companion:

```
  handler.go:921-924
      outcome, err := handler.capabilityFlow.Confirm(...)
      if err != nil {
          ... publishCapabilityActionResult(ctx, sender, actionID, "failed")   <-- every error
      }
```

Every error becomes `"failed"` with a hardcoded `invalid_action`. That is
correct for most of them — no credentials, verb not offered, consent refused,
a non-2xx answer from a server that plainly heard us. It is wrong for the ones
where the request demonstrably left the machine and only the *reply* went
missing, because the runner flattens those too (`runner.go:143-146` treats every
adapter error identically).

Real paths where the side effect may already have happened:

| Kind | Where |
| --- | --- |
| HTTP round-trip lost after the request was sent | every network adapter's `do`/`invoke` — slack `client.go:171`, todoist `:131`, notion `session.go:85`, outlook `:168`, gcalendar `:139`, gdrive `:130`, msteams `:133` |
| `osascript` killed by a deadline mid-run | applereminders `reminders.go:47` (write path `:247-256`), applenotes `notes.go` (`:251`) |

The AppleScript case is the clearest: the Apple Event that creates the reminder
can reach Reminders.app before the process is killed, and Go has no way to find
out.

The precedent to copy is already in the tree and is strict in the right way —
**request demonstrably transmitted, response demonstrably lost, method
demonstrably mutating → unknown, not failed**
(`internal/codex/appserver/client.go:445-489`, gated on `mutatingMethod` at
`:1134-1141`). `promptqueue` goes further and persists `StateSentUnknown`
*before* calling the sender (`queue.go:126-134`), so the "we don't yet know"
survives a crash.

So the work is: a marker error on the capability path mirroring
`appserver.OutcomeUnknownError`, adapters classifying their in-flight losses as
ambiguous, and `handler.go` routing it to the `outcome_unknown` it already
knows how to send. Told to "try again" today, a user re-posts a Slack message
that may already be there.

## Item 4c — `toTaskState()` has no caller

Item 3 added `TaskState.UNVERIFIED`, mapped it in `HomeUiState`, and gave it a
push notice in `ConnectionNotificationPolicy` ("Couldn't confirm that
happened"). Then a grep of `app/src/main` for `toTaskState` returned its own
definition (`CapabilityOutcome.kt:108`) and one mention inside a comment.
**Nothing calls it.**

So none of that can fire. If the phone is in someone's pocket when the
connection drops mid-send, they get no notification at all — they find out when
they next open the app, if they open it.

This is the built-but-unreachable pattern again, and it is worth naming how it
happened here: the compiler *helped* it happen. Adding an enum value broke three
exhaustive `when`s, each was decided carefully, and all three green ticks said
the work was done — while the function holding them together had no caller.
Exhaustiveness proves every branch is handled, never that the thing is reached.

Scoped as its own item rather than folded into 4b: connecting capability
outcomes to the task list is a real piece of work, not a one-line fix, and 4b is
a live bug that should not wait behind it.

### How it was actually fixed, and why not the way the item was written

Written as "wire capability outcomes into `TaskSummary`". Checked before
building, and that framing was wrong. Three greps settled it:

- `toTaskState` in `app/src/` — only test callers.
- Anything assigning `TaskState.UNVERIFIED` in `app/src/main` — nothing.
- `"unverified"` anywhere in `companion/internal` — nothing. The companion
  never sends that task state, and `TaskState.fromWire` is the only production
  source. So the state was unreachable from both ends.

But the phone *does* already know. It marks such a task
`TaskQueueState.OUTCOME_UNKNOWN` in five places
(`LauncherSessionViewModel.kt:559, 568, 593, 628, 1114`), and
`TaskControls.kt:65` already blocks follow-ups on it. Two names for one fact,
never introduced to each other:

```
  what the phone knows                what was built to show it
  ────────────────────                ────────────────────────
  TaskQueueState                      TaskState.UNVERIFIED
    .OUTCOME_UNKNOWN                    │
      │                                 ├─ ConnectionNotificationPolicy:37
      ├─ TaskControls:65 ✓ used         │    "Couldn't confirm that happened"
      │                                 └─ HomeUiState:58  "Unverified"
      └────────── no path ─────────────X   (both unreachable)
```

So the fix is not a new bridge from capability outcomes. It is one rule —
a task whose `queueState` is `OUTCOME_UNKNOWN` reports `TaskState.UNVERIFIED` —
used at the two places that read a task for display:

1. `StreamClient.kt:27`, which feeds the notification policy. This is the
   pocket case: the push notice can now fire.
2. `HomeUiState.toHomeTask()`, so the home row carries `StateMark.UNVERIFIED`
   instead of going on saying "Working".

Point 2 reverses a deliberate decision recorded at `HomeUiState.kt:32-34`
("the sheet's CapabilityOutcome already carries it"). That reasoning holds for
the capability sheet, which is open in front of you. It does not hold for a row
in a list you are scrolling past: there is no sheet, and nothing else on the row
says anything is wrong. An off-device `statusSummary` must not be able to
suppress the mark either — free text from the other machine should not get a say
in whether "we lost track of this" is shown.

`toTaskState()` still has no caller after this, because both routes go through
`queueState`, not through a `CapabilityOutcome`. **Open decision:** delete it,
or leave it. Deleting is the honest answer to built-but-unreachable and is what
"least code" says. Not done here because `CapabilityOutcomeUnverifiedTest.kt`
records real intent about the home list through it, and that intent is now
satisfied by a different route. Worth 10 minutes with fresh eyes, not a 4am
deletion.

---

## Item 7's wiring — how a capability says "not here"

The ledger from the section above is a pure bookkeeper, and for a while nothing
called it. Three questions had to be answered before anything could.

**How does a capability say its work must finish on the phone?** `Confirm`
returns `(Outcome, error)` and is fully synchronous. The house pattern for "this
ending is not one of the ordinary ones" is already a distinguished error type —
`OutcomeUnknownError`. So a second one, next to it:
`DeviceWorkError{AdapterID, Kind, Handle, Text}`. It is not an unknown outcome;
it means everything is decided and only the acting is somewhere else. Its
`Error()` names the adapter and the kind and nothing more — `Handle` and `Text`
are things a real person wrote, and error strings reach logs.

**Which of the two names does a bad ending get reported under?** Both, it turns
out, and this is why `Record` grew an `ActionID`:

```
          a real answer                  we could not find out
                |                                 |
        capability_result                   action_result
        keyed on requestId                keyed on actionId
                |                                 |
                +------- one Record holds both ---+
```

All three bad endings hand the caller only a `Record`. A record carrying one
name could not be reported at all.

**What closes the three cleanup rules, given there is no disconnect callback?**
There is no hook in `Handler` for a phone going away — `disconnectRecipients`
is internal to delivery failure. But `Handle`'s `hello` branch already notices a
replaced session. That is enough, and it is not a workaround: `device_action` is
deliberately not sequenced and never replayed, so a phone on a fresh session has
no memory of the ask and will never answer it.

```
  hello from a device with work outstanding  ->  outcome_unknown  (it never got the ask)
  SweepDeviceWork past the deadline          ->  outcome_unknown  (it may well have sent)
  a second device_action_result              ->  ignored, so neither of the above is overwritten
```

`SweepDeviceWork` is exported and driven by the caller — no goroutine, no ticker
inside the handler, because a background sweeper makes the exact deadline a test
observes unpredictable. It is also called opportunistically at the top of
`Handle`, so a live session notices its own timeouts without an external ticker.

### The sentence that was a guess

The first draft mapped the wire's `refused` to *"More than one conversation
matched, so nothing was sent."* Reading `ReplyAdapter.plan()` killed it.
`refused` covers two different phone-side endings — the phone could not tell
which conversation was meant, **or** the notification came back without a usable
reply box (`ReplyAdapter.kt:122`) and the phone handed off to the app. The Mac
cannot know which. Naming one as fact is the exact dishonesty this whole round
exists to remove. It now reads *"Nothing was sent — this one has to be replied
to in its own app."*, which is true either way.

This also settles the open question recorded earlier about `ReplyPlan.HandOff`:
it **is** reachable on the confirm path, and it maps to `refused` — not because
a hand-off is a refusal, but because the wire has four words and no room for a
fifth without a body field for the app label, which the exact-key-set check
rejects on both machines.

---

## Item 8 was never small

It was written as "phone side: `device_action` → `ReplyAdapter.pick` →
`ReplySender`", one line, sized small. Three things were wrong with that.

**`ReplyHandle(` is constructed nowhere in the app.** Only the data class
declaration exists in main source. `plan()` and `pick()` are fully tested rules
running on input that nothing produces — the built-but-unreachable pattern
again, this time at the *input* side rather than the output side. Every previous
sighting of it was a finished thing with no caller; this is a finished thing
with no feedstock, which reads exactly the same from a test report.

**`ReplySender` throws away the distinction the wire needs.** `ReplySender.kt:25`
computes `dispatch.deliver(...) == DeliveryResult.DELIVERED` and keeps only the
boolean. `DeliveryResult` already splits `DELIVERED` / `NOTIFICATION_GONE` /
`FAILED` — the same three the wire wants — and "the notification vanished under
us" and "the send failed" are different things to tell somebody. Fixed in place:
`send` now returns the plan, the raw `DeliveryResult`, and the outcome.

**Nothing on the phone knows who a conversation is with.** This is the real
one. The Mac names a *person*; `NotificationSighting` deliberately holds no
names at all — a body *length* and never a body, counts and never content. So
the join the Mac assumes simply does not exist.

The line drawn, rather than quietly widening what the probe stores: a
conversation title is *who*, not *what*, and it is needed to reply to the right
person. It is kept **in memory only**, for exactly as long as the PendingIntent
it is paired with, and it never reaches `NotificationSighting`, the on-disk
ledger, or a log line. It dies with the process and with the listener
disconnecting, same as the reply boxes already do — `LiveReplyActions` is
generic and already does that storage, eviction and locking, so the new index
uses it rather than becoming a second store beside it.

Matching is generous and deciding is strict, on purpose:

```
  "maya"  ->  every box whose person matches, including two different Mayas
                                 |
                         ReplyAdapter.pick
                                 |
              declines if they span more than one conversation
```

Whole words only — matching any substring would reply to Maya when asked about
May, and to Dan when asked about Danielle. A blank handle matches nobody rather
than everybody. Putting the ambiguity check anywhere but `pick` would mean two
places that both have to be right about never guessing between people.

---

## Findings recorded but not built

From the fresh-context judge on the `outcome_unknown` round, plus what turned up
while fixing them. Kept here so they are not rediscovered from scratch.

| # | Where | What | Severity |
|---|---|---|---|
| A | `CapabilityInteraction.kt:265-273` | Every non-cancel result state, including `outcome_unknown`, became "App action failed. It was not sent to Codex." | **CRITICAL — fixed**, see below |
| B | 7 HTTP adapters | Ambiguity decided by `method != http.MethodGet` alone, so a DNS failure or refused connection — where nothing was sent — was reported as "we don't know" | MEDIUM — **fixed**, one shared rule |
| C | `capability_disconnect` in `handler.go` | No `errors.As` check for `OutcomeUnknownError`, unlike `capability_confirm`. Was inert; **fixing B made it live** | MEDIUM — **fixed**, see below |
| D | `CapabilityInteraction` state | A bare in-memory `MutableStateFlow`. Android killing the process while an outcome is unresolved would drop the warning silently. Unverified guess, not confirmed | LOW — open, unverified |
| E | `TaskActionsMenu.kt:155-163` | The "Task action unconfirmed — the computer did not confirm whether this change happened" dialog is shown for **every** non-Complete outcome, including a definite failure. The mirror image of A: hedging about something we do know | LOW — open |
| F | `TaskActionBridge.kt:90` | Only `TaskAction.Fork` gets the honest unresolved treatment; rename and archive turn `outcome_unknown` into `TaskActionOutcome.Failed`. Softened by E — the dialog they land on happens to be honestly worded | LOW — **read properly and downgraded; do not build.** See below |

**E, read properly — real, but it cannot be fixed by sorting the error codes,
because one of them means three different things.**

`TaskActionsMenu.kt:144` is `failureVisible = outcome != TaskActionOutcome.Complete`.
Six outcomes exist (`Complete`, `Forked`, `Invalid`, `Unavailable`,
`NeedsReview`, `Failed`) and five of them land on one dialog reading *"The
computer did not confirm whether this change happened. Check Codex on your
computer before trying again."*

That is the mirror of finding A. A was false confidence; this is false doubt —
and false doubt has its own cost, because it sends someone to go and check
something that certainly did not happen, and it adds friction to the one case
where trying again is completely safe.

The obvious fix is to sort the outcomes into "definitely not done" and "we do
not know". It does not survive contact with the code:

- `Invalid` is clean. The phone rejected the title or the id before anything
  was sent. Nothing happened, certainly.
- **`Unavailable` is not one situation, it is three**, returned from four
  places in `TaskActionBridge.perform`: the send never left the phone (line 82),
  the journal could not record the attempt (line 73), *and* — line 99 — a real
  answer came back and could not be stored. The first two definitely did not
  happen. The third one definitely did, and we lost the record of it. Sorting a
  value that means both is not possible; it has to be split at the source first.
- `Failed(code)` covers eleven error codes and which of them are ambiguous
  depends on what the Mac emits for each. `OUTCOME_UNKNOWN` is ambiguous by
  definition; `INVALID_ACTION` reads like a rejection at the door; several
  others I would be guessing about. **I have not read what the companion emits
  for each code, so I am not classifying them.**

So the honest next step is not a wording change. It is: split `Unavailable`
into "never sent" and "answered but not recorded", then sort. Recorded here
rather than half-done, because a dialog that is confidently wrong about three
more situations is not an improvement on one that is vaguely wrong about five.

**F, read properly — the fork-only rule is right, and this is why.** The
finding was recorded as a gap: three actions exist (`Rename`, `Archive`,
`Fork`), and only one of them is spared being called "Failed" when the reply
was lost. Reading it against the harm rather than against the symmetry flips
the answer.

The whole reason a lost reply must not be reported as "failed" is that it
invites a retry, and a retry of something that already happened does damage.
So the question is what each retry actually does:

```
  rename, twice, to the same title   ->  the same title.       no damage
  archive, twice                     ->  still archived.       no damage
  fork, twice                        ->  two tasks.            damage
```

Fork is the only one of the three whose second attempt creates something new,
and it is the only one that got the treatment. That is not an oversight; it is
the rule applied correctly and then, unhelpfully, given a fork-shaped name.

Building the "fix" would cost real work for no safety gain: the unresolved
concept is threaded through the UI as fork-specific in at least six places
(`unconfirmedForkTaskIds`, `TaskScreen.unresolvedFork`,
`TaskActionsMenu.unresolvedFork`, `LauncherActivity.kt:529`,
`LauncherSessionViewModel.kt:387/646/657`), so generalising it means touching
all of them.

What is left is a smaller, real problem, and it is **E**, not F: a rename that
in fact succeeded is shown as "Failed" for a moment. The user is told something
false, but nothing is at stake and the next snapshot corrects it. Worth fixing
where the wording is decided, which is E's line — not by rebuilding this.

**A, in full.** The companion was taught to send `state: "outcome_unknown"`
(item 5) and the phone was never checked. `acceptActionResult` branched only on
`pendingDecision == "cancel" && resultState == "cancelled"`; everything else fell
into one path that set `phase = FAILED` and the message "App action failed. It
was not sent to Codex." — rendered verbatim by `CapabilitySheet.kt:112-118`.

A flat, confident, false claim in the exact situation the feature exists to
avoid, and it set no pending check, so the user could immediately re-send and
the person on the other end would get the message twice. Every sibling consumer
handled `outcome_unknown` explicitly; this one did not.

Fixed by giving it the same ending `sessionLost()` produces, through a shared
helper so the two cannot drift: `StateMark.UNVERIFIED`, neither claim flag,
`pendingCheck` armed, and a new `CapabilityEffect.OutcomeUnknown`.

**B, in full.** Seven adapters each copied the same rule: anything that is not a
GET is ambiguous. Half right. `http.Client.Do` also fails when the request never
left the machine — the hostname did not resolve, the connection was refused, the
TLS handshake was rejected. Reporting those as "we don't know" sends someone to
check an app where nothing happened, and blocks their next prompt for no reason.

Replaced with one function, `adapter.ClassifyHTTPFailure`, and seven callers.
Where the line sits: an error is ambiguous only once the connection is up and
the request is on the wire. Anything failing before that point is a certain
failure. An unrecognised error leans to "unknown", because the harm is lopsided
— a needless "go and check" is annoying, a duplicate message to a real person
cannot be taken back.

**C, in full — a finding that stopped being inert because of the fix above.**
C was filed LOW on the grounds that nothing could produce the error it failed to
check for. Routing the two Google revoke calls (`gcalendar/client.go:145`,
`gdrive/client.go:135`) through `ClassifyHTTPFailure` made it produce one, and
nobody would have noticed: the fix and the finding were in different files, and
every test stayed green.

The failure it allowed is a loop with no exit:

```
   revoke sent ──► Google revokes the token
                        │
                   reply lost
                        │
                        ▼
        phone told "failed"  ── user taps disconnect again
                        ▲                    │
                        │                    ▼
                        └──── 400 ◄── Google: already revoked
```

Round two onwards is a plain 4xx, so "failed" looks right and never changes.
The user ends up certain an app still has their calendar when the access is
already gone — the screen and the world disagree forever, and nothing in the
loop can correct it.

Fixed by mirroring the confirm branch exactly (`handler.go`, disconnect case).
Note the retry rule differs here — retrying a revoke harms nobody, unlike
re-sending a message — but *the report of what happened must still be true*, and
`"confirmed"` is not the answer either: this is the one action whose entire
point is cutting off access.

**And the row nobody could see (item 4c, second half).** Wiring
`queueState → TaskState.UNVERIFIED` made the mark reachable, but the words under
the mark still came from the raw state, so a task we had lost track of printed
"Working" in our own voice next to a question-mark badge. `unverifiedLabel`
("Unverified") had been written for exactly this and had never once been shown.
Both now come from the effective state. A status line that arrived from
off-device still wins the words, the same as for every other state — that rule
is pinned by `HomeTaskMarkTest.aStatusLineFromTheComputerCannotEraseTheMark` and
was left alone.

---

## What is deliberately not here

- iOS. Deferred with the rest of iOS.
- Replying to an app with no reply box. `ReplyAdapter` already refuses and
  hands off; that is the correct answer, not a gap.
- Choosing between two conversations. `ReplyAdapter.pick` already declines
  rather than guessing, for the same reason the contact graph does.

## Known small gap, accepted

Between a process restart (holder empty) and `onListenerConnected` finishing its
re-sweep (`NotificationProbeService.kt:60-71`), a `device_action` arriving in
that window reports `NOTIFICATION_GONE` for a notification that is in fact still
on screen. It fails in the safe direction — it under-claims — so it is accepted
rather than fixed.
