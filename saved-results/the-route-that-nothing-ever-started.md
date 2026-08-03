# The route that nothing ever started

**Date:** 2026-08-03
**Where:** `companion/internal/capability/adapters/notificationreply/`, `routing/stage2/resolver.go`, `runtime/production.go`, `runtime/contract_every_adapter_test.go`
**Why this exists:** Operator's Direct Reply send path was complete, wired, and had never run once. This is what was missing, what it cost to make it reachable, and a second finding — the test meant to catch exactly this kind of gap could not fail.

---

## The finding

Operator can answer a live message thread by typing into the reply box Android
puts in the notification. That whole path was built:

```
  an adapter returns DeviceWorkError   <-- nothing here
        |
  handOffToDevice           handler.go:1062
        |
  "device_action" ------->  handleDeviceAction   LauncherSessionViewModel.kt:1189
                                 |
                            DeviceReplyRequest.carryOut
                                 |
                            AndroidReplyDispatch -> RemoteInput
                                 |
  handleDeviceActionResult <--  delivered / notification_gone / refused / failed
```

Every link has a real production caller. Grep any one of them and it looks
healthy. The chain had still never run, because the thing that *starts* it —
`adapter.DeviceWorkError`, the error an adapter returns to say "the Mac cannot
do this, the phone has to" — was constructed in exactly one place in the whole
Go tree, and that place was a test fake (`mobilesession/device_work_test.go:45`).
Six references, one constructor, zero in production.

This is a distinct shape from the usual dead-code case and it is harder to see,
because the per-link check passes at every link. The question that catches it is
asked at the **start** of the chain, not the middle: *who fires the first shot,
and is that thing itself reachable?*

---

## What was built

`adapters/notificationreply` — an adapter that answers `send` and hands the
reply to the phone. `adapter.go:143` is now the first and only production
construction of `DeviceWorkError`.

Three decisions are baked in, each written into the test file's header because a
future reader will otherwise quietly reverse them:

**The verb is `send`, not `compose`.** Starting a conversation and answering one
already on screen are genuinely different jobs, and only the second can be
finished without the user. The existing hand-off adapters (instagram, whatsapp
by deeplink) open the app with drafted text and claim nothing; they keep
compose. Splitting by verb is what stops this adapter winning requests it cannot
fulfil and turning a "message someone new" into a refusal.

**The ceiling is `completes`.** A ceiling answers *how much is left for the user
to do*, and after the reply is fired there is nothing left. I first wrote that
it should be lowered to `one_tap` to express doubt, and that was wrong — the
open question about a reply is not effort, it is certainty. We know Android
accepted the text; we do not know the recipient received it. This codebase
already keeps those apart and already has the word for the second one
(`outcome_unknown`). Lowering the ceiling would have put the doubt in the field
that does not mean doubt.

**The person's name passes through exactly as typed**, including
`Dr. Ana-María O'Brien` and `मीरा`. The Mac must not resolve it. Only the phone
knows which conversations still have a live notification, and it already matches
a name against them (`ReplyHandleSource.candidatesFor`). Any tidying here —
lowercasing, trimming, mapping to a canonical id — makes the phone's match fail
on a name that would have worked.

Two things never leave the Mac: an empty reply (it would post a blank message
into a real person's chat, and the wire rejects it anyway) and a reply with no
name (the phone would match a blank against every open chat).

---

## Making it reachable cost a third addressing word

Writing the adapter is not the job; making it reachable is. Before any adapter
is picked, a class declares how it is addressed. `to_a_person` sends the request
through the contact graph to turn a name into a handle.

**The production contact graph is permanently empty.** It is created inline at
`runtime/production.go` and `Graph.Add` has no production caller anywhere — only
two test files and the offline eval harness. So any class marked `to_a_person`
answers every request with "which of Maya's surfaces did you mean?" and an empty
list of choices.

Putting reply into the existing `messaging` class would therefore have made it
unreachable on day one, with its own tests still green — the same defect this
work exists to end, rebuilt one level up.

Reusing `to_a_thing` was not right either. It means "there is no person to
resolve", and that is a lie here: there is a person, and getting them wrong
sends a real message to a real human. Reusing either word would have been this
codebase's other recurring defect, one word standing for two different truths.
So a third word, `resolved_on_the_device`: there is a person, and the device
resolves them.

That is also the better design regardless of the empty graph. The Mac has no
business choosing which of Maya's apps to answer in when the phone has already
answered that question better.

---

## Second finding: the guard against this was itself fake

`runtime/contract_every_adapter_test.go` holds shared rules every adapter must
obey (fails closed, never claims it sent something it did not). It builds a
hand-written list of every adapter and runs each rule over all of them. Its last
test existed to stop that list drifting when someone adds a new adapter package.

It compared a hand-written list of 15 names to a hand-written
`const adapterPackages = 15`. **Both sides were the expectation.** Add a
package, add nothing else, and the test stays green. Its own comment defended
this, on the grounds that reading the directory would make "a test that
discovers its own expectation" — the right principle aimed at the wrong target.
The directory is the thing under test, not the expectation.

The new adapter sailed straight past it and escaped every shared rule in the
file.

Rewritten to read `../adapters` from disk and compare in both directions. I
confirmed it fails for the right reason twice: once on the real gap —

```
these adapter packages exist but no rule in this file covers them:
notificationreply — add each to everyAdapter above, then to this list
```

— and once more after closing that gap, by creating an empty
`adapters/zztestonly/` and watching it name that instead. A guard that has only
ever been seen passing is not a guard.

---

## Evidence

- `rtk proxy go test -count=1 ./...` → **83 ok, 0 FAIL**
- `adapters/notificationreply/adapter_test.go` — 8 tests, all PASS by name
- `runtime/notification_reply_reachable_test.go` — 5 tests, all PASS by name.
  The headline one resolves a reply through the real production registry with a
  deliberately empty contact book, because that is not a simplification of
  production, it *is* production
- `TestTheHandWrittenListCoversEveryAdapterPackage` — PASS, and proven to fail
  on a planted package
- `DeviceWorkError{` in production: one hit, `adapters/notificationreply/adapter.go:143`.
  The other two are test fakes

Both spec files were written before any implementation existed, confirmed red,
and diffed afterwards against parked copies to confirm the implementer had not
edited them.

---

## Update, same day: it moved once more, to a permission nothing asked for

Same question, one link higher again. The chain now runs end to end — the
router names the class, stage 2 picks the adapter, the adapter hands the work
to the phone, the phone fires it into the notification's reply box. And on a
real phone it could not succeed once, because `DeviceNotificationAccess` is
empty until Android connects a notification listener service, Android only
connects one for an app the user has explicitly allowed, and **nothing in this
app had ever asked**. Every reply, every user, forever: `refused`.

The branch that produced that word was four lines inline in
`LauncherApplication`, where nothing could test it and nothing could tell the
person what had happened.

### The fix is on the phone, not on the wire

`refused` is the right wire word and it does not change. It means we know for
certain nothing was sent, and that is exactly true here. The wire has four
words and the Mac's decoder drops a frame carrying a fifth, so an invented
word does not degrade gracefully — it leaves the person with nothing on screen.

What was wrong is that `refused` is all the person got, and it covers three
unrelated situations with three different remedies. That is this session's
other recurring defect — one word for several truths — and the answer is the
same shape as the `handed_to_the_app` fix: keep the wire honest and narrow,
and put the explanation where the person is.

So `capability/reply/request/DeviceReplyEntry.kt` now takes the three readings
at call time and splits the two permission-shaped reasons:

```
  not in Android's enabled listeners  ->  ask, and open the settings screen
  enabled but no live listener        ->  say nothing; it comes back by itself
```

The second row is the one worth stating out loud. Android rebuilds the
listener after an app upgrade, a force-stop and low memory, and for a few
seconds either side of that the dispatch is missing on a phone where
permission is already on. Asking then sends somebody to a settings screen that
already says On, which teaches them the ask is noise.

### Why the split happens before the call, not after

`DeviceReplyRequest` returns `refused` for two further reasons of its own: an
ambiguous set of conversations it cannot choose between, and a plan that needs
the app opened by hand. Neither is a permission problem, and neither is
fixable from a settings screen. Deciding on the readings *before* that call is
what keeps them apart. Trying to decide afterwards would mean reading a
meaning back out of a word that does not carry it — which is how the defect
got here in the first place.

### Evidence

16 tests, written first and confirmed red on exactly the missing types, then
verified green by me from the JUnit XML rather than Gradle's
`BUILD SUCCESSFUL` (which prints no counts): the unit suite went **510 → 526
tests, 0 failures, 0 errors**, and all 16 pass by name. Both spec files were
diffed against parked copies afterwards and are byte-identical, so the
implementer did not edit the spec to fit the code.

The wiring was checked by hand rather than taken on report, because a thing
that is built and unreachable is the failure this whole file is about:
`LauncherApplication.kt:20` holds the ask, `:28` hands that same object to the
entry, `:50` makes the entry the only reply path, and `LauncherActivity.kt:156`
reads the same object's state and `:618` shows the dialog. One object, one
path, no branch left behind.

Checked for the same bug elsewhere: of the six permissions the manifest
declares, four are install-time, CAMERA is asked at `LauncherActivity.kt:411`
and POST_NOTIFICATIONS at `:335`. Notification-listener access was the only
one with no request site anywhere in the app.

**Still unverified: the dialog appearing on a real screen.** That needs the
Pixel, and the Pixel is locked.

---

## What this does not fix

The send has still never happened on a real phone. Android accepting text is not
a message arriving. (The word for it was `delivered` when this was written; see
the third update below for how that was fixed.) The confirm sheet, the
notification-access permission ask and the per-thread stop control are also
still open, and no reply should be fired at a real person before they exist.
(The permission ask landed later the same day — see the update below. The
confirm sheet and the stop control are still open, and that sentence still
holds for them.)

Related: `saved-results/the-unattended-path-walked-past-the-checkpoint.md`,
`planning/operator-complete-messaging-plan.md`.

---

## Update, same day: closing the gap moved it, it did not remove it

Fixing the missing first mover at the adapter meant the chain now starts from a
`stage1.Route` carrying `AppClass: "notification_reply"`. So I asked the same
question one link higher — *who builds that value, and is that thing itself
reachable?* — and the answer was nobody.

Routing is two stages. Stage 2 turns a class into an adapter; that is what the
work above fixed. Stage 1 asks the model to turn what a person said into a verb
and a class, and **`app_class` is a free-form string in the schema**
(`routing/stage1/openai/client.go:242` — `map[string]any{"type": "string"}`, no
enum). The model is therefore not choosing from a list. It writes whatever word
the instructions taught it, and of the ~90 lines of per-app coaching, not one
was about replying. WhatsApp's line even reads *"never claim the message was
sent (notification reply is a separate path)"* — naming a path it never
describes.

The consequence, in the product: **"reply to Maya: on my way" came back as
messaging / whatsapp / compose**, and the user got WhatsApp opened with a draft
to send by hand. That is precisely the hand-off the reply route exists to
replace. It happened silently, with every test in the repository green.

### Why the guard is derived on both sides

The obvious fix is a test asserting the instructions mention replying. That is
the same shape of mistake: it has to be remembered again for the next adapter,
and this repository has already lost an adapter that way (notion, dropped out of
a hand-written OAuth skip list nobody updated).

So `runtime/every_class_is_routable_test.go` writes neither side by hand:

- the classes come from `NewProduction(...)`'s own registry
- the words come from the request the client actually put on the wire, captured
  by pointing the real client at an `httptest` server and reading the
  `instructions` field out of the JSON it sent

The rule it enforces is general:

> **a class the router cannot name is an adapter nobody can reach.**

Add an adapter with a new class and forget the coaching line, and this test
names your class and tells you which file to edit. Three controls sit beside it:
the reply sentence must say where `subject` and `body` go (getting `subject`
wrong is not a missed route, it is a message to the wrong person), it must not
mention `compose` (or it swallows requests to start a conversation, which cannot
be finished without the user), and it must not claim the message is delivered or
received.

### Evidence

Red before the change, naming exactly one class:

```
--- FAIL: TestEveryClassInTheBuildIsOneTheRouterCanName
    the router was never taught to write these classes, so no utterance can
    reach their adapters: notification_reply — add a line to the instructions
    in routing/stage1/openai/client.go saying when to use each
```

Green after, with the twelfth class taught:
`ok ... /capability/routing/... /capability/runtime/...` (8 packages, 0 FAIL),
and the whole Go suite at **83 ok / 0 FAIL**.

## Update, same day: "delivered" is gone from both machines

Everything the phone observes on a reply is that
`actionIntent.send(context, 0, fillInIntent)` did not throw
(`AndroidReplyDispatch.kt:76`). That proves Android handed the text to the app
that posted the notification. It does not prove WhatsApp sent anything and it
does not prove anybody received anything — the app may be offline, logged out,
or may simply drop it.

The wire's four words are now `handed_to_the_app`, `notification_gone`,
`refused`, `failed`. Two things this deliberately is **not**:

- **not a lower ceiling.** A ceiling answers "how much is left for the person to
  do", and there is nothing left. `done` stays `true` and the ceiling stays
  `completes`.
- **not a blanket hedge.** The other three outcomes know for certain that
  nothing was sent, and controls in both specs hold them there. Somebody told
  nothing was sent can go and send it; somebody told "maybe" can do nothing.

Specs written first and confirmed red:
`app/mobilesession/honest_reply_word_test.go` (5 tests) and
`capability/reply/HonestReplyWordTest.kt` (5 tests). Verified by me after the
implementation, not taken on report: **83 ok / 0 FAIL** Go, **510 tests / 0
failures / 0 errors** Kotlin unit (counted from the JUnit XML, not from Gradle's
`BUILD SUCCESSFUL`, which prints no counts), and `grep` for
`delivered|DELIVERED` across the whole reply path on both machines returns
nothing.

## Fourth time, same day: I built a fresh one myself, then only half fixed it

Written 2026-08-03, after the permission fix above.

`ReplyGuard` — the thing that remembers "the user told me to stop replying to
this person" and "this conversation has had enough replies for now" — was
written, given 13 tests, and called by nothing. Within the hour of writing up
the exact failure it is an instance of. A rule only its own tests ever ask is
not a rule the product has.

**What is fixed.** The guard is now consulted on the real reply path and one
long-lived guard is built on the application object itself
(`LauncherApplication.kt:29`, real clock, `ReplyCap.shipped`), so it remembers
across replies instead of starting fresh each time. The cap works end to end.

**Where the check goes, and why it cannot go earlier.** A conversation is a
person *in an app*. The Mac only ever sends a person's name — it has never
known which app and by design never will, because only the phone knows which
conversations are still live. So the app is not known until
`ReplyAdapter.pick` has chosen among the live notifications for that name, and
the guard has to be asked immediately after that and before the dispatch is
touched. Asked at the door it would be keying on half a conversation, and one
person's stop would silently cover every app they are reachable in — stopping
Maya on WhatsApp would stop her on Instagram too, which is not what anybody
asked for.

**Only a reply Android actually accepted spends the allowance.** A send that
failed, or one whose notification vanished underneath it, sent nothing. Making
those spend would let a broken phone talk Operator into refusing replies it
never made.

**What is still broken, stated plainly.** `stop`, `resume` and `stopped` have
no caller anywhere in `app/src/main/`. The cap fires by itself because the
reply path walks into it. A stop needs a person to set one, and there is no
screen, no gesture and no wire message that does. **A user cannot stop a
conversation today.** That is the same defect one level up for the fourth
time, and the honest status of this item is half done — the half that is
missing is the half a user can see. It also needs storage that survives a
restart: holding the stop list in memory is fine for a cap catching a loop
within seconds, and plainly wrong for a promise made to a user.

**Evidence.** 10 tests written first and confirmed red, implementation
delegated, then verified by me from the JUnit XML: **549 tests / 0 failures /
0 errors** (up from 539), all 10 passing by name. Every spec file diffed
byte-identical against copies parked before the handoff, except the one line I
changed myself. Wiring traced by hand rather than taken on report.

**One process note worth keeping.** Making the guard a required parameter with
no default broke six call sites, not the four I had counted — two of them
(`HonestReplyWordTest.kt:64`, `DeviceReplyRequestTest.kt:193`) did not name the
parameter, so they had not tripped the error I was counting by. The
implementer hit them, refused to edit test files as instructed, and reported
the blocker instead of inventing a way around it. That is the right behaviour:
an overload without the guard would have quietly recreated the unguarded path.
I made both edits myself.

## A CI gate that could not fail, found the same day

`protocol/schema/envelope.schema.json` declares which message types the wire may
carry, and `release/checks/protocol/schema_test.py` runs it in CI. What that
check actually does is validate the fixtures in `protocol/fixtures` against the
schema — **both of them hand-written, neither of them the implementation.** Add
a message type to the Go validator and the Kotlin codec, ship it, and the
schema never learns about it, because nobody wrote a fixture for it, so there
is nothing to reject. Same shape as the adapter guard fixed earlier: two sides
written by the same person at the same moment cannot disagree.

It had drifted. Four types the wire genuinely accepts were undeclared:
`capability_preview`, `capability_result`, `device_action` and
`device_action_result` — **the entire Direct Reply path.**

**A correction on the size of it.** An earlier scout reported roughly thirteen
missing types, including `set_project`, `start_turn` and `capability_confirm`.
That was wrong, and I checked before acting on it: those are action *kinds*
carried inside the body of an `action` envelope (`validation.go:1053` onward),
not top-level message types. The real number is four.

**The fix derives both sides.** `schema_matches_the_wire_test.go` parses this
package's own `validation.go` with Go's standard `go/ast`, pulls the case
labels out of `validateBody`'s switch on `message.Type`, and compares them
against the enum in the checked-in schema file — in both directions, so a
phantom type in the schema fails too. There is no list written by hand in that
test. A third test holds that the reader still finds real types, because if
`validation.go` were restructured so the switch could not be found, the other
two would compare an empty list against an empty list and pass while guarding
nothing.

**A second list in the same file that can drift the same way.** The schema's
first `allOf` rule names which types require `seq`, and its `else` branch
actively forbids `seq` on everything else. `capability_result` requires one, so
declaring it without also adding it to that list would have made the schema
wrong in a new way — it would reject every real `capability_result` frame. That
rule is not yet covered by a derived guard; it is the obvious next one.

**Evidence, and a guard I made sure I had seen fail.** The four types are now
declared, the type enum is 20, and `capability_result` was added to the seq
rule. Go package green uncached (`-count=1`). The existing CI check still runs
clean: 34 frames validated, 38 invalid frames rejected, no fixture regressions.
Because a block that has only ever been seen passing proves nothing, I put
seven frames through the new blocks by hand and every one behaved: a good
`device_action_result` validates; `outcome: "delivered"` is rejected (the dead
word, still dead); the same frame sent by `companion` instead of `phone` is
rejected; an extra body key is rejected; `capability_result` **without** `seq`
is rejected and **with** `seq` validates, which is the seq rule biting in both
directions; and a `capability_result` carrying `queued` as its ceiling is
rejected.

**One mistake of mine worth recording, because the implementer caught it.** I
briefed the ceiling enum as `queued, sent, confirmed, outcome_unknown, failed,
cancelled`. That is the `action_result` *state* enum — a different field
entirely. The real ceilings are `completes`, `one_tap`, `hands_off`
(`validation.go:1095`). The implementer read the function instead of trusting
the brief and used the code's values. Had it trusted me, the schema would have
been confidently wrong about the exact field this plan's ceiling fix was about.

**What is still not covered.** There are no fixtures for the four new types, so
the CI fixture check exercises none of them — the derived guard catches a type
going undeclared, which was the actual defect, but body-shape drift inside
those four types is still uncovered.

## Closing the loop: the stop is now something a person can actually do

Same day, straight after. The gap recorded above — the guard could stop a
conversation and nobody could set one — is closed, in two steps.

**Step one: somewhere to set it.** Two candidate places were checked first and
neither could name a conversation, which is the whole difficulty. A stop names
a person *in an app*. The confirm sheet holds only the strings the Mac
pre-rendered (`CapabilityInteraction.kt:39-47`): no person at all, and
`adapterId` is a display label like "gmail", not an Android package. And
`device_action` reaches `carryOutDeviceReply` with no sheet whatsoever
(`LauncherSessionViewModel.kt:1189-1197`), by design. The one moment both facts
exist together is inside `carryOut` after `ReplyAdapter.pick` has chosen — which
is exactly where the guard already sits. So the guard publishes what it just
recorded and a row offers the stop from that.

Deliberately **not** a dialog. `NotificationAccessDialog` is modal because it is
rare and blocking; this appears after every successful reply, and a modal box
each time would be intolerable. It matches the app's existing quiet banner.
The wording never says "sent" or "delivered", because the phone only knows
Android accepted the text: *"Handed a reply to WhatsApp for Maya."* with
*"Stop replying to Maya"* and *"OK"*.

The offer appears only when a reply **actually went out**. A refusal, a failure,
or a notification that vanished sent nothing, and offering to stop a
conversation Operator never spoke in would tell the user something untrue about
what their phone just did.

**Step two: making it mean something.** A stop that a restart forgets is not a
promise. An upgrade, a force-stop or Android reclaiming memory wiped every
stop, and Operator would quietly start replying again in a conversation
somebody had deliberately shut, with nothing telling them. And `resume` had no
caller, so from the user's side a stop was permanent. Both fixed: `DurableStops`
restores at startup off the main thread, the offer row writes through it, and a
"Stopped conversations" section in the existing settings screen lists them with
*"Turn replies back on"*.

**Three design points worth keeping.**

*Persistence sits outside `ReplyGuard`.* Where a decision is kept between runs
is a question about this app — which storage, which thread, what happens when
the file is unreadable — and none of it belongs in the rule. It also avoids
adding a required constructor argument to a class with nine call sites, which
is how the previous round cost six mechanical edits.

*Restore only ever adds, never clears.* Android runs it again when it brings the
app back to the front, and clearing first would undo a stop the user made in
between.

*An unreadable stop list reports `UNKNOWN`, not "no stops".* Same distinction
between *didn't happen* and *don't know* this codebase has already been bitten
by. A stop also holds for the current run even when the write fails — losing it
at the next restart is bad, silently not stopping right now is worse.

**Storage.** Preferences DataStore shaped after `ProjectSelectionStore`, each
conversation stored as JSON so a person's name containing a comma, pipe or
newline round-trips safely — the escaping does the separating, rather than a
delimiter chosen in the hope no name contains it.

**Evidence.** 549 → 561 → **575 tests / 0 failures / 0 errors**, counted from
the JUnit XML each time, never from `BUILD SUCCESSFUL`. Every spec diffed
byte-identical against copies parked before each handoff. Reachability checked
by grep rather than taken on report: `restore` at `LauncherApplication.kt:36`,
`stop` at `LauncherActivity.kt:644`, `resume` at `:606`, the list at `:377`.

**One decision left with the owner, flagged rather than settled quietly.** The
stop list is not cleared by `LocalStateWiper` when the user removes the paired
computer. Keeping it is what I would choose: if a wipe cleared stops, an unpair
and re-pair would silently resume replying to someone the user had shut off,
which is the more harmful of the two failures — and the unpair dialog
enumerates what it removes rather than promising to remove everything, so
nothing on screen becomes untrue. The counter-argument is real: the list holds
**other people's names**, kept on the phone, and a "remove my data" action can
reasonably be expected to clear them.

## A guarantee that holds, but not for the reason its comment gives (2026-08-03)

`DeviceReplyRequest.kt:64-73` says a reply only ever happens after somebody
confirmed a sheet on this phone, and the whole reply path spends that sentence
as its stand-in for consent — it is why `attended = true` is passed. I traced
it instead of trusting it, and the conclusion is worth keeping because it is
neither "the comment is right" nor "the comment is wrong".

It holds. The one place a `device_action` is built (`handler.go:1062`) has
exactly one caller in the tree, inside the `capability_confirm` branch
(`handler.go:1001`). `Service.Prepare` previews unconditionally
(`service.go:120`) — no adapter and no verb can skip it. On the phone the only
route to `respond(true)` is a button, with no timer, no remembered decision and
no "always allow"; dismissing the sheet answers **no**.

But the mechanism is **sequencing plus correlation, not attendance**. What
`Confirm` actually checks is that the incoming fingerprint matches the preview
the Mac itself issued, then deletes it so it cannot be replayed
(`service.go:152`). That is "this confirm names the preview I showed for this
request, and I have not consumed it yet". A Mac cannot verify a human tapped
anything, and this one does not try. The human part is enforced entirely on the
phone, by there being one path and it being a button.

Two consequences worth writing down rather than rediscovering:

- `RefusalReason.UNATTENDED` is a branch nothing can currently reach, because
  its only call site hardcodes `attended = true`. That is correct today and it
  is an invariant, not a check.
- The consent store is a **no-op for this adapter**. `consent.Requires` returns
  true only for class B; reply is class A. So the preview→confirm handshake is
  not one gate among several — it is the entire consent mechanism for a reply.

**And a real defect on the same surface.** The sheet that *is* the consent step
printed the adapter's programmer id at the user, one capital letter deep:
"Notification_reply · send", "Maps_saved_places · open", "Gcalendar · create".
Fixed with a small `adapterLabel` — a written-down list of product names,
falling back to tidying the id — so a new adapter degrades to something plain
instead of something wrong. Reply's label is "This phone", because the machine
showing the sheet genuinely does not know which app the reply will land in and
must not guess. Same bug found at three more sites by grepping
`replaceFirstChar`; all fixed. Unit suite 575 → **585 / 0 failures / 0 errors**,
counted from the JUnit XML.

## The contact book's obvious producer is barred by this repo's own rule (2026-08-03)

The remaining open item says the notification probe already sees every sender's
name and app, so it should feed the Mac's contact graph. It should not, at
least not that way. `LiveReplyBoxesTest`'s header draws the line explicitly: a
conversation title is kept in memory only, for as long as the PendingIntent it
is paired with, and **never** reaches `NotificationSighting`, the on-disk
ledger, or a log line. Streaming those names to the Mac crosses that line by
design.

Two structural mismatches on top of it: the graph keys on adapter id plus a
handle (a number, thread id or address), and a sighting gives an Android
package plus a display name, which are neither; and the production graph is
built and discarded in the same expression (`production.go:320`), so nothing
holds a reference for a producer to add to.

The shape that would respect the rule is **ask, don't stream** — the Mac asks
the phone about the one person it was just told to message, the way the reply
route already asks `candidatesFor`. Names travel only for someone the user just
named, and nothing is stored on the Mac. It makes a synchronous resolve into a
round-trip, which is a design change and an owner call, so it was not started.

---

## The question nobody heard — found, built, green (2026-08-03)

**What was wrong.** The Mac's router writes a plain-English sentence when it
understood a request perfectly well and needs one more word — "Which of Maya's
surfaces did you mean?", "I don't have the app you named connected for this",
eight more. `handler.go:962` logged the sentence's *length* and threw the
sentence away, then sent `action_result{state: "cancelled"}`. On the phone that
arrived while the phase was still ROUTING, fell through the `failed` special
case at `CapabilityInteraction.kt:329`, and put up a dialog titled **"App
action failed"** saying "The app router returned an unexpected result. It was
not sent to Codex."

Nothing failed. The router worked. This is the same defect this codebase keeps
producing — one word standing for two opposite truths — one level further out
than anyone had looked.

**What was built.** One optional `question` field on `action_result`, legal
only when the state is `cancelled`, display-stripped at 512 characters, plus a
new `CapabilityPhase.QUESTION` on the phone shown under "One more thing".
Files: `validation.go:618`/`:945`, `handler.go:963`/`:1249`,
`event.schema.json:78`/`:82`, `ProtocolCodec.kt:212`,
`CapabilityInteraction.kt:342`, `CapabilitySheet.kt:131`.

**Verified.** Android 598 tests / 0 failures / 0 errors, counted from the JUnit
XML rather than from `BUILD SUCCESSFUL`. `go test ./... -count=1`: 83 packages
ok, 0 FAIL. `release/checks/protocol/schema_test.py`: 35 frames validated, 40
rejected.

### Three things worth keeping from how this went

**A judge with fresh context overturned the plan's premise and its design.**
The first draft said the user "sees the request stop — no sentence, no reason".
False, and the truth was worse: they were told their router malfunctioned. The
same judge killed the first design — a separate unsequenced `capability_question`
message — by pointing out that `action_result` is *already* sequenced, journaled
and replayed on warm reconnect (`handler.go:1270`, `ReplayAfter` at `:534`). A
new unsequenced message opts out of all three and is simply lost if the
connection drops between two sends. One optional field on an existing message
removed the ordering window by construction. The judge paid for itself twice.

**The plan named the wrong schema file, and the derived guard could not have
caught it.** It said `envelope.schema.json`. But `envelope.schema.json:44` hands
the `action_result` body straight to `event.schema.json#/$defs/actionResult` —
the envelope only lists type *names*. `schema_matches_the_wire_test.go` compares
type lists too, not body shapes, so an edit to the wrong file would have passed
every check while changing nothing.

**The fixture was mutation-tested, so it is known to be load-bearing.** An
invalid frame — a question on a `confirmed` result — went into
`schema-drift.jsonl:39`. Deleting the `if/then` rule from `event.schema.json`
makes the check fail with `invalid fixture was accepted: schema-drift.jsonl:39`;
restoring it makes it pass. A fixture nobody has ever seen fail is the same
category of thing as a guard whose two sides were written by one person in one
sitting.

### Found on the way, not fixed

**`action.schema.json` does not know a single capability action kind.** Writing
the valid fixture meant adding an ordinary
`action{kind: "capability_request"}` line — and it failed the schema check. The
action body schema lists only `start_turn`, `steer_turn`, `interrupt_turn`,
`approval`, `question_response` and `set_project`. Every capability action the
wire really carries is absent. This is exactly the drift
`schema_matches_the_wire_test.go` warns about in its own header, now with a
reproduction — and it is also *why* nothing caught it: the schema check only
validates fixtures, and no capability fixture existed. The `action` line was
dropped rather than widening the change. Reproduce it by re-adding:

```
{"version":{"major":1,"minor":0},"messageId":"m-013","sender":"phone","type":"action","body":{"actionId":"action-capability-1","kind":"capability_request","utterance":"message Maya"}}
```

to `protocol/fixtures/session.jsonl` and running
`python3 release/checks/protocol/schema_test.py`.

**A claim withdrawn, because getting this wrong is the same mistake.** While
writing this up I recorded that `CapabilityEffect.UnexpectedRouteResult` was
constructed and "handled nowhere in `app/src/main`" — one more finished thing
with no caller. It was wrong, and it came from grepping for the type name
instead of reading the handler. It *is* handled, by the generic `else` at
`LauncherSessionViewModel.kt:518`: the sequence is acknowledged and the user's
draft is kept rather than thrown away. No branch mentions it by name, which is
why grep missed it, and which is exactly why "grep found nothing" is not
evidence of nothing.

**Checked while I was there, and fine.** The new QUESTION path returns
`CapabilityEffect.None`, so it takes the early return at
`LauncherSessionViewModel.kt:497` and does not acknowledge the frame
immediately. That is not a leak: `ack` carries `throughSeq` and the Mac applies
it with `journal.Acknowledge(deviceID, throughSeq)` (`handler.go:355`), so it
is cumulative and the next higher ack covers it. The early return also leaves
`pendingHomePrompt` in place, which is the behaviour you want here — the Mac
asked for one more word, so the person's draft should still be there when they
give it.


### A judge found two real things after it was already green

A second judge, fresh context, was asked to work out what a correct version of
this would have to get right *before* it saw the change. It was given no hints.

**HIGH — the question dialog could not be closed.** `dismissTerminal()`
(`CapabilityInteraction.kt:436`) picks what it may clear from a hand-written
`setOf(RESULT, FAILED)`, and the new phase was never added. Both the dialog's
one button and its back-press route there. The user read the question, tapped
OK, and nothing happened, under a modal dialog with nothing else to touch.

**The reason it slipped is the interesting part.** The same enum is consulted in
two ways in the same file. `sessionLost()` uses an exhaustive `when`, so the
compiler *made* it learn about the new phase. `dismissTerminal()` uses a
`setOf`, which nothing checks. The one place that needed a person to remember is
the one place that was wrong. Reproduced as a failing test
(`expected:<IDLE> but was:<QUESTION>`) before the fix, with a control that
dismissing is still refused while a request is in flight.

Checked for the same bug elsewhere: `setOf(CapabilityPhase` /
`listOf(CapabilityPhase` across `app/src/main` gives three hits. One was the
bug. `busy` (`:65`) rightly excludes the new phase — a question is not work in
flight, and the person should be able to start something else. `acceptResult`
(`:400`) rightly excludes it too — a question means the flow stopped before
anything executed, so no result can follow. Both left alone on purpose.

**MEDIUM — the schema was laxer than either decoder.** `question` had length
bounds but not the control-character-and-blank pattern every sibling display
field carries (`event.schema.json:67`). Both real implementations reject a
whitespace-only sentence; the schema accepted it. Proved with an invalid
fixture of three spaces (`schema-drift.jsonl:40`) that the check accepted
before and rejects after.

**One correction to the judge.** It called the schema gap "currently inert since
nothing in the repo validates fixtures against this JSON Schema file directly."
That is wrong — `release/checks/protocol/schema_test.py` does exactly that, and
running it is how the gap was proved. A judge being confidently wrong about one
supporting detail while right about the finding is worth recording: the finding
still had to be verified, not accepted.


---

## The schema that could not fail — 2026-08-03

**What this was.** Writing the fixture for the question feature above meant
adding one ordinary line to `protocol/fixtures/session.jsonl`: an
`action` frame with `kind: "capability_request"` — the message the phone sends
every single time somebody asks their computer to do something in an app. The
schema check rejected it. Not because the frame was wrong; the Go validator and
the Kotlin decoder both accept it. Because `protocol/schema/action.schema.json`
had never heard of that kind.

**The measurement, derived from both sides rather than remembered.**
The validator's own `switch kind` accepts thirteen action kinds. The schema
named six.

```
  missing (7): capability_request, capability_confirm, capability_disconnect,
               archive_task, fork_task, rename_task, dismiss_unknown_control
```

Every capability action — the entire "ask my computer to do a thing in an app"
path — plus every task-management action except the three oldest.

**Why nothing caught it.** The only thing that ever reads these schema files is
`release/checks/protocol/schema_test.py`, and all it does is validate the
hand-written examples in `protocol/fixtures/`. A kind nobody wrote an example
for is a kind the schema is never asked about. Both sides of that check are
written by the same person in the same sitting. That is the third time this
repository has produced that exact shape of dead guard.

The sibling guard `schema_matches_the_wire_test.go` was green the whole time. It
compares message *type* names and says nothing about what is inside a body.

**What was built.** The derived test first, the schema second — adding seven
branches by hand and stopping would leave the same hole open for the eighth.
`schema_matches_the_action_kinds_test.go` pulls the kinds out of `validation.go`
with `go/ast` and out of `action.schema.json` off disk, and compares them both
directions. There is no list of kinds anywhere in that file.

**The judge earned its cost twice on this one.**

*It refuted a claim in the plan before anything was built.* The plan said the
new test could "reuse the same AST walk" as the sibling guard. It cannot. The
existing extractor matches a switch whose tag is a selector named `Type`
(`message.Type`); `validateAction` writes `switch kind {`, a bare identifier.
Reusing it would have found nothing, reported nothing missing, and **passed
while guarding zero** — the same dead-guard bug the test exists to prevent,
rebuilt inside the fix for it. The new matcher keys on the enclosing function
name instead, and calls `t.Fatal` if it ever finds no kinds at all.

*It found a live bug in a kind the plan's own table called "known".* The
schema's `start_turn` branch required `taskId` + `text`. The validator accepts a
*second* shape for the same kind — a brand-new task with `projectId`, `modelId`,
`reasoningId`, `permissionModeId` and no `taskId` (`validation.go:1028-1031`,
identically in `ProtocolCodec.kt:265-269`). No fixture had ever carried it.

**The thing worth carrying forward: enum parity is not body parity.** After all
this, the schema lists all thirteen kinds and the derived guard keeps that true
forever. It is still perfectly possible for a branch's *required fields* to
drift from the validator's, and nothing here catches that. The `start_turn`
new-task shape is the proof, and it was found only because a person read both
files side by side. Do not read this change as making the schema trustworthy.

**Found on the way, fixed.** `attachmentIds` is capped at 16 items by
`validateOptionalIDs` (`validation.go:1296`). The pre-existing
`start_turn`/`steer_turn` branch had no `maxItems` at all. A 17-item list would
have passed the published contract and been refused by both real decoders. One
word, now on both branches. The implementing agent reported this honestly as an
asymmetry it had been told not to touch, which is the behaviour to want from a
subagent.

**Checked that the test was not satisfied by cheating.** After the agent made
the derived test pass, the thirteen kinds were re-derived from `validation.go`
and compared to the thirteen measured before any of the work started. Identical
set. The validator had not been quietly widened to meet the test.

**Verified green, counted rather than trusted.**

| check | result |
|---|---|
| Android, from the JUnit XML | 598 tests / 0 failures / 0 errors |
| `go test ./... -count=1` | 83 packages ok / 0 FAIL |
| `release/checks/protocol/schema_test.py` | validated 43 frames, rejected 41 |

The schema check went 35 → 43 validated (eight new frames, one per new shape)
and 40 → 41 rejected. Android matters here and is easy to forget: the Kotlin
contract test decodes every line of `session.jsonl` and requires every line of
`schema-drift.jsonl` to be refused, so new fixtures are a third implementation's
problem too. The implementing agent ran Go and Python only; Android was run
afterwards and passed.

**Mutation-tested, so the new rejection is load-bearing.** Flipping
`capability_request`'s `additionalProperties` to `true` made the check fail with
`invalid fixture was accepted: schema-drift.jsonl:41`. Restoring it printed
`validated 43 protocol frames and rejected 41 invalid frames`.

**Left out on purpose.** `device_action` body shapes drifted too — the guard's
own header says so — and that is the same work again, separately. Making the
Kotlin decoder a third derived side is a different language and a different
parse; the two-way Go-to-schema guard closes the hole that actually shipped.

Plan: `planning/the-schema-that-cannot-fail-plan.md`.


---

## A flaky Go test, seen once, mechanism identified — 2026-08-03

Worth writing down because it will look like a regression the next time it
happens and it is not one.

**What was seen.** A full `go test ./... -count=1` reported
`TestDesktopExistingTaskUnknownWritesStayUnknownAndNeverReplay`
(`internal/app/mobilesession/handler_test.go:1041`) as FAIL, in a run where the
only source change was in a different package. Earlier the same session the
same command reported 83 ok / 0 FAIL, twice.

**What was checked.** Run alone: passed 5 times out of 5. Run as
`-count=8` on just that package: passed, 12.2s. It did **not** reproduce
outside the full-suite run.

**The mechanism, read rather than guessed.** The test waits for messages with
`awaitSentMessage`, whose entire wait is:

```go
case <-time.After(time.Second):
    t.Fatal("timed out waiting for mobile message")
```

`live_events_test.go:239-248`. A one-second wall-clock deadline. During
`go test ./...` the machine is compiling and running dozens of packages at
once, and a goroutine that would normally deliver in microseconds can miss a
1s deadline. Nothing about the deadline scales with load.

**Stated at the right strength:** the timeout is definitely the only wait in
that test, and the failure only appeared under full-suite load. That the load
starved *this* deadline is the obvious explanation but was not directly
observed — the failure was not reproduced.

**Not fixed, on purpose.** Raising the bound is a one-word change and would
probably work, but a deadline is also the only thing standing between this
suite and a real hang, and picking the new number blind is how a timeout stops
being a test. If it recurs, the fix worth making is to wait on a condition
rather than a clock.

**What to do when it fails again:** re-run that one test alone before
believing it. If it passes alone, it is this.


---

## The two types the contract said nothing about — 2026-08-03

**What this was.** `protocol/schema/envelope.schema.json` is the published wire
contract. It lists twenty message types in an enum, then constrains each one in
a separate branch. Two types — `task_read` and `task_page` — were in the enum
and in no branch. The only thing the contract asked of them was that a `body`
key exist and be an object. Any keys, any values, and **either side allowed to
send it**: the published document said a `task_page` may come from the phone,
which the Mac refuses.

**How it was found.** By auditing every body shape after the previous change,
which had closed with the sentence "enum parity is not body parity, and nothing
here will catch it". This is what that sentence meant, measured.

**The part worth keeping: the guard corrected its author.** The first count was
done by hand with a throwaway script and said *four* types were unconstrained.
It was wrong. `attachment_cancel` and `attachment_complete` do have a branch
(`envelope.schema.json:133`) — it names its two types with an `enum` instead of
a `const`, and the script only read `const`. The derived test, written before
any fix and reading both forms, went red naming exactly `task_page, task_read`
and nothing else.

A hand measurement and a derived one disagreed, and the derived one was right.
That is the whole argument for writing the guard before the fix, and here it
was made against the person writing it. The plan was renamed from
"the four types" to "the two types" rather than quietly edited.

**A second self-inflicted near-miss, caught by a judge.** The first version of
the guard counted a type as covered if *any* branch mentioned it. One branch
(`:26-29`) names five types purely to say whether they carry a `seq` — it
constrains nothing about the body. Under that version, a future type could be
listed in the seq branch, get no body rules at all, and satisfy the guard: the
exact dead-guard bug the file exists to catch, rebuilt inside the check for it.
Fixed by only counting a branch whose `then` defines `body`. Re-ran: still
exactly two, so the tightening did not lose the other eighteen.

**This is a documentation fix, not a bug fix, and that matters.** A judge
checked the Kotlin decoder against the Go validator for both types and they
agree key-for-key — `ProtocolCodec.kt:161-167` and `validTaskPage` at `:402`,
including entries-never-null, the unique-id set, and the
error-implies-empty-entries-and-no-cursor rule. Both runtimes already refuse
everything the schema was accepting. Nothing users touch was broken. What was
broken is the document anyone writing a third implementation would read.
Recorded plainly rather than sold as a save.

**Two questions answered from the source rather than assumed:**

- `limit` is required on `task_read`. `uintInRange` calls `uintValueOK`, which
  returns `(0, false)` when the value is absent (`validation.go:1323`). Kotlin
  reaches the same answer differently: it coerces the absent value to 0 and
  rejects it as outside 1..64 (`ProtocolCodec.kt:165`).
- A file-change entry requires only `path` and `kind`. `onlyAllowedKeys`
  (`:1417`) forbids unknown keys and never requires anything — the requirement
  comes from `boundedString` and `safeDisplayString` failing on absent
  values (`:893`), while `diff` has an explicit nil guard.

**The check the agent could not have known to run.** The implementing agent ran
Go and Python and reported success honestly on that basis. But
`protocol/fixtures/session.jsonl` is read line by line by the Kotlin test and
every line must decode (`ProtocolContractTest.kt:86`), and every line of
`invalid/schema-drift.jsonl` must be **refused** by Kotlin (`:419`). A fixture
change is a three-implementation change. Android was run afterwards: 598 tests
/ 0 failures / 0 errors. **This is the standing lesson for delegating fixture
work — the brief should name Android, and this one did not.**

**Verified, counted rather than trusted:**

| check | before | after |
|---|---|---|
| `release/checks/protocol/schema_test.py` | 43 validated / 41 rejected | **45 / 45** |
| `go test ./... -count=1` | 83 ok / 0 FAIL | 83 ok / 0 FAIL |
| Android, from the JUnit XML | 598 / 0 / 0 | 598 / 0 / 0 |

**Anti-cheat, done structurally rather than by asking.** The agent was
forbidden to touch `validation.go`. Rather than take its word, the files
modified during its run were listed by modification time: exactly the four
allowed files. The twenty message types derived from `validation.go` afterwards
are identical to the twenty measured before. (A `git diff` would have been
useless here — the branch has a whole session of uncommitted work in it, so the
diff shows fifty files no matter who touched what.)

**All four new invalid fixtures mutation-tested**, each schema rule broken and
restored: removing `task_read`'s sender const, flipping its
`additionalProperties` to true, removing `"type":"array"` from
`task_page.entries`, and removing the error/entries sub-rule. Each produced
`invalid fixture was accepted: schema-drift.jsonl:NN` for its own line, and
each restore came back to 45/45.

**Known and left alone.** Go's `boundedString` counts Unicode runes; JSON
Schema `maxLength` counts code points. They differ only for astral-plane
characters. Not worked around, written down.

Plan: `planning/the-two-types-with-no-body-plan.md`.

---

## The lock screen was failing the tests — 2026-08-03

**What it looked like:** the connected suite failed a different one or two of
its 128 tests every run, always `ComposeTimeoutException … after 5000 ms` in
`UiScenarioActivityTest.awaitHierarchy`. Run that class on its own and it was
23 of 23, three times over. Every symptom said "flaky test on a busy phone".

**It was not.** I raised the wait from five seconds to twenty, on that theory,
and re-ran the full suite. It failed **ten** tests instead of two, at twenty
seconds each. A wait that fails at 20s when 5s was already generous is not
waiting for a busy device.

**What it actually was.** `dumpsys window` right after that run showed
`showing=true` — the phone had locked part way through. It was unlocked when
the run started. The lock screen sits on top of `UiScenarioActivity`, so
Compose never attaches a hierarchy, so every remaining test in that class times
out. That is why the failures cascade, why they start at a different test each
run (whenever the lock happens to land), and why a bigger timeout cannot help.

**Proved it rather than assumed it.** Same build, same class, phone deliberately
left locked:

```
LOCKED, before the fix:  23 tests / 22 failures
LOCKED, after the fix:   23 tests /  0 failures
```

Counted from `app/build/outputs/androidTest-results/**/*.xml`, not from
Gradle's `BUILD SUCCESSFUL`. `showing=true` was re-checked immediately before
the green run, so the fix is what changed and not the phone's mood.

**The fix** is two attributes on the debug-only activity in
`android/app/src/debug/AndroidManifest.xml` — `android:showWhenLocked="true"`
and `android:turnScreenOn="true"` — and the wait went back to five seconds. The
activity is shell-only (`android.permission.DUMP`), re-checks the debuggable
flag before drawing, and shows nothing but local synthetic sample data, so
letting it draw over the lock screen exposes nothing real.

**Checked for the same bug elsewhere — and the first answer was wrong.** I
first concluded nothing else was affected, because in the run where the phone
locked part way through, only `UiScenarioActivityTest` failed. That was an
accident of timing, not immunity: it happened to be the class running when the
lock landed. Running the full suite with the phone locked *from the start*
failed `LauncherActivityTest` too — all five of its tests, same Compose
timeout — even though it launches through `ActivityScenario` rather than
`am start`. The lock screen covers any activity.

So `showWhenLocked` fixes exactly one class, the largest one, and no more. The
rest of the suite genuinely needs a phone somebody has unlocked. This lock is
secure (`dumpsys trust`: `deviceLocked=1`, `trustState=UNTRUSTED`, and
`wm dismiss-keyguard` does not clear it), so no software here can open it, and
turning the lock off would mean disabling one of the phone's own defences to
make a test suite go green — not a trade worth making.

**So the second half of the fix is to say so.** `release/checks/android-device-ready.mjs`
reads the keyguard's own line out of `dumpsys window` and stops
`bootstrap-smoke.sh` before the connected step with "The phone's screen is
locked. Unlock it by hand, then run this again." Verified against the real
locked Pixel: it printed that and exited 1. An unreadable dump reports
*unknown* and lets the run continue rather than pretending the phone is ready —
a check that cannot see the phone should not become a new way to block on it.
Six tests, run before the code existed and confirmed red with
`ERR_MODULE_NOT_FOUND`, green afterwards at `# pass 6 / # fail 0`.

**The lesson worth keeping:** raising a timeout is the cheapest thing to try
and the easiest way to bury a real cause. The twenty-second run is what
disproved the load theory, so it earned its keep — but only because the number
was checked afterwards instead of being accepted because the suite went
quieter.

### The proof, on a locked phone — same day

The fix was then confirmed the hard way rather than the convenient way: the
full connected suite was run with the keyguard up (checked immediately before
the run — `showing=true` under `KeyguardServiceDelegate`), which is the
condition that used to produce a wall of mystery timeouts. Counted out of
`app/build/outputs/androidTest-results/**/*.xml`, per class:

    24 tests   0 failed   UiScenarioActivityTest      <-- showWhenLocked
    20 tests  20 failed   HomeScreenTest
    15 tests  15 failed   TaskScreenTest
     7 tests   7 failed   LauncherActivityTest
     … every other Compose class, all of its tests
     7 tests   0 failed   CodexConnectionServiceTest  <-- draws nothing
     8 tests   0 failed   EncryptedDraftStoreTest     <-- draws nothing

129 tests, 67 failures, 0 errors. That split is the whole diagnosis in one
run: every class that draws a screen fails, every class that does not passes,
and the single class carrying `showWhenLocked` is untouched. Nothing here is
"flaky" — the failures are perfectly predictable once you know what to look
at. The unit suite was 598 / 0 / 0 in the same run, which is the control: none
of this has anything to do with the code being wrong.

24 rather than 23 because four reply-consent screens were added to the debug
scenario catalogue the same day. That is the cheap way to close a "never seen
on a real screen" note: `UiScenarioActivityTest.everyFixedScenarioRendersItsExpectedRoot`
walks `ScenarioCatalog.all` and renders every entry on the phone, so adding a
catalogue entry buys the real-screen check for free instead of costing a new
bespoke test.

**And the debug-only flag cannot reach anybody.** `showWhenLocked` is granted
to one debug-only, shell-permission activity, so the thing worth proving is
that it is absent from a real release build — not argued from the source tree,
which is where this kind of claim usually stops. Built `assembleDebug
assembleRelease`, then read the shipped binary with `apkanalyzer`:
`release/checks/android-debug-apk-isolation.test.mjs` passed its 7 assertions
(the release manifest and DEX contain no `UiScenarioActivity`, no
`ScenarioCatalog`, no `app.codexlauncher.debug.scenarios` at all), and a
direct grep of the printed 190-line release manifest for
`showWhenLocked|turnScreenOn|UiScenarioActivity` returned nothing.

One honest note on that last check: the first attempt printed a clean `0` for
an entirely uninteresting reason — `ANDROID_HOME` was unset in that shell, so
`apkanalyzer` never ran and `grep -c` was counting an empty stream. A zero from
a command that did not run looks exactly like a zero from a command that did.
The number above is from the re-run that resolved the analyzer explicitly and
printed the manifest's line count alongside the hit count, so an empty input
could not pass as a clean result.
