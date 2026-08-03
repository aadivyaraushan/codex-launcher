# The question nobody hears

**One sentence:** the Mac writes a plain-English sentence saying why it stopped,
throws it away, and the phone tells the user their app router malfunctioned.

Status 2026-08-03: **built, judged, two judge findings fixed, green on both
machines.** Android 598 tests / 0 failures / 0 errors counted from the JUnit
XML; `go test ./... -count=1` 83 packages ok / 0 FAIL;
`release/checks/protocol/schema_test.py` validated 35 frames and rejected 40. This was the last item of real code work across both
plans that was not waiting on the owner or on the Pixel.

> **This plan was wrong on its first pass and the correction is the point.**
> It originally said the user "sees the request stop — no sentence, no reason".
> That is false, and a judge with fresh context caught it. What the user
> actually sees is worse than nothing, and the design changed as a result.

---

## What actually happens today

```
  "message Maya"
        |
        v
  +--------------+   understood it fine, needs one more word
  |  stage2      |------------------------------+
  |  Resolve     |                              |
  +--------------+                              v
        |                          Decision{ MustAsk: true,
        | resolved                            Question: "Which of Maya's
        v                                     surfaces did you mean?" }
  capability_preview                                |
                                                    v
                                    flow.Prepare -> *QuestionError
                                                    |
                                    handler.go:962 logs question_LENGTH only
                                                    |   (the sentence dies here)
                                                    v
                                    action_result { state: "cancelled" }
                                                    |
                                                    v
  PHONE: phase is still ROUTING, so this lands at CapabilityInteraction.kt:329.
  That branch special-cases "failed" and sends everything else to the else at
  :338 -- phase = FAILED, message = "The app router returned an unexpected
  result. It was not sent to Codex."
                                                    |
                                                    v
        +------------------------------------------------------+
        |  App action failed                                   |   <- the title
        |                                                      |
        |  The app router returned an unexpected result.       |
        |  It was not sent to Codex.                           |
        |                                    [ Done ]          |
        +------------------------------------------------------+
```

So a normal *"say that again a bit more specifically"* is presented to the user
as **a malfunction of the app router**, under a dialog titled **"App action
failed"**, telling them it was not sent to Codex either. Nothing failed. The
router worked. This is the same defect this codebase keeps hitting — one word
standing for two opposite truths — one level further out than anyone had looked.

The existing test `unexpectedSuccessfulRouteResultNeverFallsBackToComputer`
(`CapabilityInteractionTest.kt:108-122`) pins this branch, so the behaviour is
deliberate for genuinely unexpected results. `cancelled` is simply not one.

**A claim this plan made earlier, now withdrawn.** It said
`CapabilityEffect.UnexpectedRouteResult` was "handled nowhere in
`app/src/main`" — another finished thing with no caller. **That was wrong, and
it came from grepping for the type name rather than reading the handler.** It
is handled, by the generic `else` at `LauncherSessionViewModel.kt:518`: the
sequence is acknowledged and the user's draft is kept rather than thrown away.
No named branch, but the right behaviour. Withdrawn rather than quietly
deleted, because "grep found no mention" is the exact reasoning that produces
this plan's own favourite bug in reverse.

## There are ten of these sentences, not one

All in `stage2/resolver.go`, all already written, all already reachable.
Verified by count, twice:

| line | sentence |
|---|---|
| 198 | I'm not confident enough about what you want me to do — can you say it again? |
| 205 | I don't know which app to use for `X`. |
| 215 | The `X` class was never declared as addressed to a person or a thing, so I won't guess. |
| 246 | I don't have the app you named connected for this. |
| 264 | Which of `Maya`'s surfaces did you mean? |
| 269 | The surface I'd normally use for this isn't available right now. |
| 286, 301 | No available app can handle this right now. |
| 291, 306 | More than one app could handle this — which one did you mean? |

**Nine of the ten need no answer UI.** They explain why nothing happened; they
are not menus. Only line 264 carries choices (`dec.Candidates`), and it reads
them from a contact graph nothing ever fills, so that list is always empty in
the shipped app. Building a chooser now would build a menu with no items on it.

## What gets built

**One optional field on a message that already exists. No new message type, no
second send, no ordering to get right.**

```
  Mac                                        Phone
  ---                                        -----
  QuestionError{Question}
        |
        v
  action_result {                  ------>   phase == ROUTING, state ==
    actionId, state: "cancelled",            "cancelled", question present
    question: "Which of Maya's                     |
               surfaces did you                    v
               mean?"                        phase = QUESTION, show the
  }                                          sentence, one button
```

This was the second design. The first sent a separate unsequenced
`capability_question` message before the result, and it was worse in three ways
the judge named:

- `action_result` is **already sequenced, journaled and replayed on warm
  reconnect** (`handler.go:1270`, `ReplayAfter` at `:534`). A new unsequenced
  message opts out of all of that for free, and is simply lost if the
  connection drops between the two sends.
- Two sends means an order to depend on. The fix above only works if the
  question is processed *before* the result; with one message there is no
  window.
- It needs no `requestId`-keyed holding state on the phone.

### Wire

| field | rule | why |
|---|---|---|
| `question` | optional, `isSafeDisplay(512)`, **legal only when `state == "cancelled"`** | it is a label this app renders in its own UI, so display-stripping is right — unlike `device_action.text`, which is words one person wrote for another |

Both sides close their key sets, so both must learn it: `validation.go`'s
`onlyAllowedKeys` for `action_result` (`:618`), and `ProtocolCodec.kt:212`. The
schema too — and the derived guard added earlier today
(`contract/schema_matches_the_wire_test.go`) only checks the *type* list, not
body shape, so **this one will not be caught for free**. Write the fixture.

> **Correction:** this plan first named `envelope.schema.json` as the file to
> edit. It is not. `envelope.schema.json:44` hands the `action_result` body
> straight to `event.schema.json#/$defs/actionResult`, so `event.schema.json`
> is the only place the body shape lives, and editing the envelope would have
> been editing nothing.

### Phone

A new `CapabilityPhase.QUESTION`, and it is worth the addition rather than
reusing `FAILED`: the `FAILED` dialog is titled "App action failed"
(`CapabilitySheet.kt:122`), and reusing it would re-commit the exact defect this
plan exists to fix. The sheet gets a fourth block — the sentence, one button.

`cancelled` arriving at phase `ROUTING` **without** a question keeps the
existing else-branch behaviour. That case really is unexpected.

## Deliberately accepted

**The sentence lands in the Mac's journal.** `action_result` bodies go through
`journal.Apply` (`handler.go:1270`), so "Which of Maya's surfaces did you mean?"
is written to disk on the Mac — one of the ten sentences carries a person's
name. Today the utterance itself is not journaled, so this is new. It is
accepted because the same journal already holds the user's own task content
(`handler.go:1914`) on the same disk, so a contact name is not a new category of
data there — and because the alternative was the unsequenced message, whose
reliability cost is worse than this. **This is the phone's rule, not the Mac's:**
the probe's ban on names touching disk (`LiveReplyBoxesTest` header) is about
the phone's own ledger and is untouched by this.

## Done when

- [x] `question` accepted on `action_result` in `validation.go:618` +
      `validateOptionalQuestion` (`:945`), in `event.schema.json:78` and `:82`,
      and in `ProtocolCodec.kt:212-217` — rejected when the state is anything
      but `cancelled`, when over 512, when empty, and as an unknown key
      elsewhere
- [x] `handler.go:963` sends the sentence; nothing else about the `cancelled`
      result changes. `publishCapabilityActionResult` took the question as a
      variadic parameter, so the other six call sites are untouched
- [x] Each sentence reaches the phone in a test — **not one sample.** Eight
      distinct texts across the ten call sites (two are repeated), all asserted
      in `every sentence the mac can send arrives intact`
- [x] The phone shows the sentence and does **not** say "App action failed" —
      a new `CapabilityPhase.QUESTION` under the title "One more thing"
      (`CapabilitySheet.kt:131`). A new phase rather than reusing `FAILED`,
      because reusing it would re-commit the defect
- [x] `cancelled` with no question still takes the old else-branch — the
      control passes
- [x] A body fixture for `action_result` carrying a question. Both were
      written: a valid one in `session.jsonl`, and an invalid one (a question
      on a `confirmed` result) as `schema-drift.jsonl:39`. **The invalid one
      was mutation-tested** — deleting the `if/then` rule from
      `event.schema.json` makes the check fail with "invalid fixture was
      accepted: schema-drift.jsonl:39", and restoring it makes it pass. The
      fixture is load-bearing, not decorative
- [x] Counted from the JUnit XML and from `go test -count=1`, never from
      `BUILD SUCCESSFUL` — 596/0/0 and 83 ok/0 FAIL

## The judge round after it was built

A second judge, fresh context, was asked to decide what "good" looks like here
before seeing the change, then hold the change against it. It was not told what
to look for. It found two things.

**HIGH, and real — the question dialog could not be closed.** `dismissTerminal()`
decides what it may clear from a hand-written
`setOf(CapabilityPhase.RESULT, CapabilityPhase.FAILED)`
(`CapabilityInteraction.kt:436`), and the new phase was never added to it. The
sheet's only button and its back-press both route there
(`CapabilitySheet.kt:133`, `:136` → `LauncherActivity.kt:667`). So the user read
the question, tapped OK, and nothing happened — under a modal dialog, with
nothing else on screen to touch. Reproduced as a failing test first
(`expected:<IDLE> but was:<QUESTION>`), then fixed, with a control that
dismissing is still refused mid-flight.

**Why this slipped:** the compiler *forced* `sessionLost()`'s exhaustive `when`
to learn about QUESTION, so that one was right by construction. A `setOf` is
not checked, so the one place that needed a human to remember is the one place
that was wrong. Same enum, two lists, only one of them defended.

**Same bug elsewhere:** grepped `setOf(CapabilityPhase` / `listOf(CapabilityPhase`
across `app/src/main` — three hits total. `:442` was the bug. `:65` (`busy` =
ROUTING/PREVIEW/EXECUTING) correctly excludes QUESTION: a question is not work
in flight and the user should be able to start something new. `:400`
(`acceptResult` = IDLE/EXECUTING) correctly excludes it too: a question means
the flow stopped before executing, so no `capability_result` can follow. Both
left alone, deliberately.

**MEDIUM, also real — the schema was laxer than both decoders.** `question` had
`minLength`/`maxLength` but not the control-character-and-blank pattern every
sibling display field carries (`event.schema.json:67`). Go's `safeDisplayString`
and Kotlin's `isSafeDisplay` both reject a whitespace-only sentence; the schema
accepted it. Proved with an invalid fixture (`schema-drift.jsonl:40`, a
question of three spaces) that the check accepted before the pattern was added
and rejects after. **The judge called this "currently inert since nothing
validates fixtures against this schema file" — that part was wrong**;
`release/checks/protocol/schema_test.py` does exactly that, which is how the
fixture proved it.

After both fixes: **598 tests / 0 failures / 0 errors** on Android, 83 Go
packages ok, schema check 35 validated / **40** rejected.

## Deliberately not in scope

- **Answering.** No chooser, no answer routed back. Nine of ten sentences have
  nothing to choose between and the tenth reads an empty book.
- ~~`CapabilityEffect.UnexpectedRouteResult` having no handler.~~ Withdrawn —
  see above. It is handled at `LauncherSessionViewModel.kt:518` and does the
  right thing.
- **`action.schema.json` knows no capability action kind at all.** Found while
  writing the fixture above: adding a perfectly ordinary
  `action{kind: "capability_request"}` line to `session.jsonl` fails the schema
  check, because the action body schema lists only `start_turn`, `steer_turn`,
  `interrupt_turn`, `approval`, `question_response` and `set_project`. Every
  capability action the wire genuinely carries is missing. This is not a new
  break — it is the precise drift `schema_matches_the_wire_test.go` warns about
  in its own header, now with a reproduction. It is also *why* no capability
  fixture existed to find it. The `action` line was dropped from the fixture
  rather than widening this change; fixing it is its own piece of work.
- **The consent-not-granted flattening** (`flow/service.go:108-110`). Still out
  of scope, but **this plan described it wrongly and the real version is worth
  writing down.**

  It does *not* collapse into `invalid_action`. `capabilityFailureCode` maps
  `consent.ErrNotGranted` to `unauthorized` deliberately, with a comment saying
  why (`handler.go:1221-1225`) — that part of the code is right.

  What actually happens is one step later, on the phone. The result arrives as
  state `failed` while the phase is still `ROUTING`, and that branch does not
  read the error code at all (`CapabilityInteraction.kt:339-341`): it publishes
  **"No app action matched. Sending to computer…"** and hands the request to
  Codex. So the honest sentence — *you have not connected that app yet, and you
  can fix that* — is carried correctly all the way across the wire in the
  `unauthorized` code, and then thrown away by a branch that only looks at the
  state. Exactly the same shape as the question bug this plan fixed, one field
  over.

  Still not built here, and the reason is not wording. It is that the useful
  version of this dialog offers a *connect this app* button, and there is no
  connect flow on the phone for it to point at. Saying "you need to connect it"
  with no way to connect it is not obviously better than the fallback. That is
  an owner call, not a code one.
