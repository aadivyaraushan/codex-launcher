# Making the contact graph real

**Status:** design, not built — and **blocked on one owner decision**, see below.
Written 2026-08-03.

> **The short version, if you read nothing else.** Everything downstream waits
> on feedstock, and there are **two** owner decisions in front of it, not one.
>
> 1. **The big one.** No Slack, Teams, Google, Outlook, Spotify or Todoist
>    adapter is registered at all in the plain `serve` build. Not conditionally
>    — always. `production.go:279-292` skips all seven, and the code explains
>    why (`oauthReason`, `production.go:96`): they need an interactive sign-in
>    that only exists in the owner-only proof commands, and there is no stored,
>    refreshable token to load instead. So companion-side feedstock is not
>    blocked on a scope; it is blocked on **persisting an OAuth token across
>    restarts**, which is unbuilt and is a data-at-rest security decision.
> 2. **The smaller one.** If that is solved, Slack DMs then also need two more
>    scopes than the three the code requests today.
>
> The phone-side alternative needs neither, but needs a privacy call instead.
> **Nothing below can be built until one of those paths is chosen.**
**Background:** `saved-results/contact-graph-never-consulted.md` — the audit that
found six gaps. Gap 1 (never consulted) is fixed and verified. This plan covers
gaps 2, 3, 5 and 6.

## The one thing to get right about the order

The obvious next piece is the disambiguation question — gap 5 and 6, "which
Maya did you mean?" and the answer coming back. **It must not be built first.**

```
   graph is empty  -->  Resolve returns MustAsk with ZERO candidates
                                |
                                v
                   a question sheet with an empty list
```

Building the ask before the graph has anything in it produces a chooser with
nothing to choose from: a complete, tested, unreachable subsystem. That is this
codebase's most frequent defect and it would be the tenth instance, entered
into knowingly. Feedstock first.

```
   gap 3            gap 2            gap 5+6
   a source   -->   Add() gets  -->  the ask, and
   of entries       called           the answer
   ----------       ----------       ----------
   BUILD FIRST      trivial once     only useful
                    3 exists         after 2
```

## Gap 3 — where entries come from

The plan's rule is two sources and nothing else: *threads inside adapters the
user has already connected, plus the phone's address book.* No scraping, no
enrichment.

**Source B below is neither of those, and this plan does not pretend otherwise.**
Notification contents are a third category: on-device observation. They are not
a thread inside a connected adapter — nobody connected anything — and they are
not the address book, which needs `READ_CONTACTS` and is absent from the app.
So choosing B is not "applying the rule", it is **asking the owner to widen the
rule to admit a third source.** That is stated here rather than buried, because
the argument *"the data already arrives on the phone for the reply feature, so
using it costs nothing"* is precisely the reasoning a two-source rule exists to
stop. Data arriving for one purpose is not consent for a second one.

Neither is wired. What exists today:

| Adapter | What it lists | Person's name available? |
|---|---|---|
| Slack | channels only — `ListChannels` requests `types=public_channel,private_channel` | **no.** DMs (`im`) are not requested |
| Teams | chats by `topic` | **no.** participants are never fetched |
| Instagram | nothing — hands off to the OS | no |

So there is no source, and adding one means asking each adapter for something
it does not currently fetch.

**This is inside the plan's rule, not outside it.** A Slack DM is "a thread
inside an adapter the user has already connected". Requesting `types=im` and
reading the other participant's display name is source one, exactly as
written — not new collection, not enrichment.

### Two candidate sources, and which to do first

```
  A. COMPANION SIDE                     B. PHONE SIDE
  ------------------                    -------------
  Slack/Teams list DMs, read the        LiveReplyBoxes already holds
  other participant's name              (person, package, conversationKey)
       |                                for every messaging notification
       v                                     |
  Graph.Add on the companion                 v
                                        ...but the graph is on the Mac,
  + no new data leaves the user's       and there is no wire message for
    machine                             this. Sending it means shipping
  + one adapter proves the whole        every notification sender's name
    chain end to end                    from phone to Mac.
  - needs an OAuth scope check
                                        - a real privacy decision, and
                                          the plan says the graph lives
                                          on BOTH sides, so the honest
                                          answer may be a second graph
                                          on the phone rather than a sync
```

**Neither is unblocked, and A is the longer road — this is a reversal.**

An earlier version of this plan said "do A first: it proves the whole chain
end to end with one adapter." That was written believing Slack was reachable
from the production router. It is not. `production.go:279-292` unconditionally
skips Slack — along with Teams, Spotify, Todoist, Calendar, Drive and Outlook —
from the plain `serve` build, and the only code that touches `oauth/slack` is
the owner-only `serve-slack-proof` command, which runs its own separate flow and
never sees the resolver the product uses.

So building DM-listing into the Slack adapter today would produce a finished,
tested feature **that the production router can never call** — the exact defect
this plan opens by warning against, one layer up from where it was looking.

```
   A needs, in order:            B needs:
   -------------------           --------
   1. persist an OAuth token     1. a wire message carrying
      across restarts               (person, package, key)
      (unbuilt; data-at-rest        phone -> Mac
      security decision)         2. a privacy call: is sending
   2. widen Slack scopes 3 -> 5     every notification sender's
   3. list DMs, call Add            name off the phone acceptable?
                                 3. call Add on the Mac's graph
   the graph is already on the       (already there)
   Mac, so step 3 is small —
   steps 1 and 2 are not
```

**A's first step is the CUSTODY GATE work** — storing refresh tokens at rest —
which is already an open item elsewhere in the project and is much larger than
this plan. **B's blocker is a single product decision** the owner can answer in
a sentence.

That makes B the shorter path on current evidence. It is still not the obvious
choice: B is also the decision about *where the contact graph lives*, and the
plan says it lives on the phone and the companion both. Two copies of the most
sensitive table in the product, kept in step, is a materially different design
from one copy with a sync, and neither is written down yet. That question should
be answered deliberately — but it is now the cheaper question to answer, not the
one to defer.

**Open for the owner, and the reason to pause on B:** the plan says the contact
graph "lives on the phone and the companion". Two graphs that both hold the
most sensitive table in the product, kept in step, is a materially different
design from one graph with a sync. Neither is written down yet.

### What A needs

0. **First, and largest: Slack has to be registered in the production build at
   all.** It is not today, and no scope or DM code changes that.
   `production.go:279-292` skips it unconditionally, for the reason written at
   `production.go:86-96`: plain `serve` starts unattended on a machine nobody is
   watching, so there is no moment to run a browser sign-in, and no persisted
   refresh token to load instead. The comment is explicit that persisting one is
   *"future work, not something to fake here."* Until that is built, steps 1-3
   below produce code with no production caller.

1. Slack: request `types=im` alongside the channel types, and read the
   counterpart's display name. This needs `im:read` and `users:read`, which are
   not the same grant as `channels:read`.

   **The code does say what it asks for today.** `ScopesForVerbs`
   (`capability/oauth/slack/flow.go:103-124`) requests `channels:read` and
   `groups:read`, plus `chat:write` when the connection includes `send`. That
   is the whole request — channels and groups, no DMs and no user directory.

   So this is not an unknown, it is a widening: **three scopes today, five to
   read DMs.** `im:read` to list them, `users:read` to turn the counterpart's
   user id into a name.

   **Owner decision, second in line behind step 0:** is widening the Slack
   request from three scopes to five acceptable? It changes what the user is
   asked to agree to at connect time, and it re-prompts everyone already
   connected. That is a product call, not an engineering one — but it is a
   specific, answerable question, not a mystery about the current state.

   *(Two corrections, both 2026-08-03. First: an earlier version said no scope
   string existed anywhere in the Slack path — wrong, it was written from
   `adapters/slack/` and the proof command and missed the `oauth/slack/`
   package, which is where every adapter's scopes live. Second: that same
   version called the scope widening "the blocker" and recommended doing A
   first. Also wrong, and the more serious of the two — Slack is not registered
   in the production build at all, so the scope question is not even reachable
   yet. Both errors came from checking one directory and generalising from it.)*
2. One place that turns a listed DM into a `contacts.Entry`
   (`Person`, `AdapterID`, `Handle`, `LastSeen`, `Source: Observed`) and calls
   `Graph.Add`.
3. The graph instance must be the *same* one the resolver holds
   (`production.go:299` — `stage2.New(reg, contacts.NewGraph(time.Now), …)`),
   which today is constructed inline, in the argument list, and handed to
   nothing else. Nobody can call `Add` on it because nobody can reach it. It
   has to be lifted to a named value and kept, before step 2 is possible.

   On "the CUSTODY GATE work" named in step 0: that is
   `internal/capability/custody/custody.go` — a built and tested encrypted token
   vault whose only caller is the owner-only `cmd/proveadapter/rotate.go`, and
   which **does not persist to disk at all.** So it is itself an instance of the
   defect this plan is about, and step 0 needs it given a real caller and real
   storage, not merely switched on.

### What B needs

1. A wire message carrying `(person, package, conversationKey)` from phone to
   Mac, and a call to `Add` on the Mac's graph.
2. **A decided `LastSeen`.** The graph's resolution rules turn on recency —
   `graph.go` uses 14-day and 90-day thresholds — so an entry with a wrong
   `LastSeen` resolves wrongly and silently. A phone-sourced entry must use
   **the notification's own post time**, not the moment the Mac received it. If
   a batch arrives after the phone was offline for a week, stamping it "now"
   would promote a week-old conversation over a fresher one.
3. The privacy call in decision B above, which is also the request to widen the
   two-source rule.

   One correction to how B is described: `LiveReplyBoxes` does **not** hold an
   entry for every messaging notification. It records one only where a usable
   reply action was found on the notification, so it is a subset — apps that
   post messages without an inline reply action contribute nothing. B's coverage
   is therefore narrower than "everyone who messages you", and how much narrower
   is not knowable from the code.

## Gap 2 — calling Add

Trivial once 3 exists, and it is the wiring, so it gets its own verification
rather than being assumed: a test that asserts the production graph is
non-empty after a connected adapter has listed its threads. Not a unit test of
`Add` — `Add` is already tested and already correct. The test that matters is
the one that fails if nobody calls it.

## Before gaps 5 and 6: nothing downstream uses the answer yet

This plan spends its first half checking that a proposed producer would really
be called. The same question has to be asked in the other direction, and the
answer is uncomfortable: **once the graph resolves "Maya" to a handle, no
adapter registered in the production build reads that handle.**

Verified rather than assumed. Every non-test read of `Handle` in
`internal/capability/`:

| Where | What it does |
|---|---|
| `adapter/adapter.go:54` | builds the fingerprint string |
| `adapters/podcasts/adapter.go:222` | a log line, and podcasts is not person-addressed |
| `verification/verification.go:234,237` | inside `Tier2Runner.ConnectLoop`; neither `NewTier1` nor `NewTier2` has a production caller |
| `contacts/graph.go`, `stage2/resolver.go`, `flow/service.go` | the routing plumbing itself |

Two classes are `ToAPerson` (`production.go:50-51`): `messaging` and `money`.
Everything registered under them is a hand-off:

- `instagram` — `grep -c Handle` → **0**
- Venmo, Cash App, Zelle via `deeplinkadapter.Wave1Specs()` — `grep -c Handle`
  → **0**

Both are `Compose`-verb, `HandsOff`-ceiling adapters. They open the app; the
person picks the recipient inside it. They read `Subject` as a display hint and
never touch the resolved handle.

**What that means for the value of 5 and 6.** A working chooser would stop the
flow from blocking with a question. It would not let anything send to a
resolved person, because there is no adapter that can. The whole return on the
most expensive part of this plan is *fewer interruptions*, not *new
capability* — until a non-hand-off, person-addressed adapter exists. That is a
real benefit and worth having, but it is a much smaller one than "Operator can
message Maya", and the decision to build a chooser should be made knowing which
of the two it buys.

This is the mirror image of the defect diagnosed at the top of this plan, and
it was missed on the first three passes for the same reason it always is:
checking the producer feels like checking the whole chain.

## Gaps 5 and 6 — the ask, and the answer

Only worth building after 2, and only with the section above in mind.

**The phone side is entirely new work.** Unlike the Mac side, there is no prior
art to extend: no chooser or disambiguation screen exists anywhere in
`android/app/src/main/`, `capability_answer` appears in neither codebase, and
the one place the phone already faces this choice — `ReplyAdapter.pick`
(`ReplyAdapter.kt:149-152`) — **declines** rather than disambiguates, returning
null the moment there is more than one conversation. So the chooser sheet, the
wire message, and the decoder entry are all greenfield, and none of the phone's
existing state machines can be reused.

Design, so it is decided rather than improvised:

```
  MAC                                    PHONE
  ---                                    -----
  Prepare -> QuestionError               (as of 2026-08-03 this reports
    carrying CANDIDATES                   "cancelled" with no error, not
    (today it carries only a string)      "failed" - handler.go:921-946.
       |                                  Honest, but the question text
       v                                  still never reaches the phone,
  PENDING QUESTION LEDGER                 so the user sees the request
                                          stop without being asked.)
    hold requestId -> {owner, subject,
    utterance, candidates}. Without
    this there is nothing to re-run
    Prepare from when the answer
    arrives, and no way to turn a
    choiceId back into an Entry.
       |
       v
  capability_question  ------------->     
    { requestId, question,                     |
      choices: [ {choiceId,                    v
                  person, app,             chooser sheet, one row
                  lastSeen} ] }            per choice
       |                                       |
       |                                       v
       <-------------------------------  capability_answer
                                           { requestId, choiceId }
       |
       v
  Graph.Answer(subject, chosen)  -- records a pin, so next time
       |                            the same subject resolves silently
       v
  re-run Prepare -> capability_preview, as normal
```

**Eight decisions, made:**

1. **The phone never sends a handle back.** The Mac hands out an opaque
   `choiceId` and takes only that. A handle arriving from the phone would be a
   value the Mac then acts on without having resolved it itself, which defeats
   the point of stage 2 owning the turn from name to handle.
2. **The chooser shows person, app, and how recently — not the raw handle.** The
   app and the recency are what actually tell two Mayas apart. Fall back to a
   masked handle tail only when two choices are identical on all three.
3. **A question that is never answered becomes `cancelled`,** on the same TTL
   the pending-preview map already uses. Not `failed` (nothing broke) and not
   `outcome_unknown` (we know exactly what happened: nobody chose).
   **A phone that disconnects while a question is outstanding clears it
   immediately, rather than waiting out the TTL.** `devicework.Ledger` already
   draws this distinction — `Expired()` sweeps on time, `DeviceGone(deviceID)`
   sweeps on disconnect — and the reason applies here with more force. A
   pending question is not just a timer; it is a promise to accept an answer.
   Holding it open for a phone that has gone means the next thing to reconnect
   could answer a question asked of a session that no longer exists. Clearing on
   disconnect and making the user ask again is the cheap, honest outcome.
4. **`QuestionError` has to carry the candidates.** It currently holds a bare
   string, and `Decision.Candidates` is dropped on the floor when the error is
   built (`flow/service.go:27-31,102`).
5. **`capability_question` is a new outbound message, not a new outcome word.**
   It parallels `capability_preview`, which already carries a body the phone
   renders. The four outcome words stay four.
6. **The pending question is held in a ledger modelled on the one that already
   exists for this.** `Prepare` returns as soon as it must ask
   (`flow/service.go:100-103`) and keeps nothing, so today there is no place a
   later answer could be applied — no subject to re-resolve, and no way to turn
   a `choiceId` back into a `contacts.Entry`. `devicework.Ledger`
   (`capability/devicework/ledger.go`) is already exactly this shape for the
   neighbouring problem of "the phone owes us an answer": `Wait` **returns
   false if that request is already outstanding**, `Settle(requestID)` lets only
   the first answer win, `Expired()` sweeps the ones nobody answered, and
   `DeviceGone` drops a disconnected phone's. Reuse that shape rather than
   writing a bare map — a bare map would let a double-tapped chooser record two
   pins and send the message twice.
7. **A name that matches nobody is a different message from a name that matches
   several.** `Graph.Resolve` returns `MustAsk` with an **empty** candidate list
   when nothing matches (`contacts/graph.go:220-222`), and the resolver's
   current wording — *"Which of Maya's surfaces did you mean?"* — is a question
   with no answers attached. That is the empty-chooser shape this plan opens by
   warning against, and it does **not** go away once feedstock exists: it
   recurs permanently for every name the graph has never seen. So zero
   candidates must say *"I don't have anyone called Maya"* and take no answer,
   while two or more candidates open the chooser. One branch, decided at the
   point the question is built, not in the phone's rendering.
8. **`capability_question` is sent live, never through the journal** — the same
   rule `device_action` already follows, for the reason written at
   the head of `handOffToDevice` in `mobilesession/handler.go`: a request the
   phone missed while
   offline has to expire rather than fire late into a conversation that has
   moved on. A chooser is worse than a plain action here, not better: answering
   *"Maya at work"* an hour after the fact pins a routing preference from a
   question the user no longer remembers being asked.

## What is NOT in this plan

- **The wipe path.** `Graph.Wipe` and `consent.Store.Wipe` both have zero
  callers, so the plan's "wiped by the existing local-state-wipe path" is not
  true on the companion side. It is a small piece of work but it belongs with
  the decision about where the graph lives (B above) — wiping one graph while a
  second copy survives on the other device would be worse than not offering it.
- **Persistence.** Neither the graph nor the consent store touches disk. Adding
  a file of who-talks-to-whom is a data-at-rest decision with real weight and
  is not something to do as a side effect of wiring feedstock.
