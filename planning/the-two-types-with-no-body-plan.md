# The two types with no body

**One sentence:** two of the twenty message types the wire carries are named in
the schema's type list and then never given a body shape at all, so a frame of
one of those types passes the published contract with any body and any sender.

Status 2026-08-03: **built and green.** Both types have a body branch, the
derived guard passes in both directions, and every new fixture is exercised by
all three implementations. `go test ./... -count=1` 83 packages ok / 0 FAIL;
Android 598 tests / 0 failures / 0 errors from the JUnit XML; the schema check
went from "validated 43 / rejected 41" to **"validated 45 / rejected 45"**.

> **This plan said "four" and the number was wrong.** The correction is worth
> keeping, because of what caught it. The first count came from a throwaway
> script that read each branch's `if` condition as a `const` only. One branch
> names its two types with an `enum` instead
> (`envelope.schema.json:133` — `attachment_cancel`, `attachment_complete`), so
> that script did not see it and reported those two as unconstrained. They are
> not; their branch is at `:134` and it is correct.
>
> **The derived test found this before anything was built.** It reads both
> `const` and `enum`, and its first red run named `task_page, task_read` and
> nothing else. A hand measurement and a derived one disagreed, and the derived
> one was right — which is the entire argument for writing the guard before the
> fix, made against the person writing it.

---

## How it was found

The change before this one made the *action kinds* agree between the validator
and `action.schema.json`, and closed with a sentence that turned out to be a
prediction:

> **Enum parity is not body parity.** It will still be possible for a branch's
> required fields to drift from the validator's, and nothing here will catch it.

Auditing every body shape in `envelope.schema.json` to see how bad that was
found something worse than drift. Two types have no branch to drift.

## The measurement

`envelope.schema.json` is a top-level object with a `type` enum listing twenty
message types, plus an `allOf` of per-type branches shaped
`if type == X then { sender: ..., body: {...} }`. Twenty types in the enum,
**eighteen covered by a branch, two not**:

```
  type enum (20)                        allOf branches (16)
  --------------                        -------------------
  hello  welcome  snapshot  event       hello  welcome  snapshot  event
  decision_read  decision_page          decision_read  decision_page
  capability_preview                    capability_preview
  capability_result                     capability_result
  device_action                         device_action
  device_action_result                  device_action_result
  action  action_result                 action  action_result
  ack  error                            ack  error
  attachment_offer                      attachment_offer
  attachment_ack                        attachment_ack

  attachment_cancel                     attachment_cancel   } one branch,
  attachment_complete                   attachment_complete }  named by enum
                                                               (:133), correct

  task_read              ------------>  (nothing)
  task_page              ------------>  (nothing)
```

`additionalProperties: false` and `required: [version, messageId, sender, type,
body]` sit at the *top* level, so a frame of one of these four types must have a
`body` key — and `body` is typed `object` at `:22`, so junk-but-an-object gets
through. **The `sender` pin lives inside each branch's `then`, so these two
accept the wrong sender too**: the published contract says a `task_page` may be
sent by the phone. The Mac's validator says it may not (`validation.go:562`).

Worth saying plainly: this is not the schema being *wrong* about these two. It
is the schema having no opinion at all, which is the failure mode that reads as
success.

## What the validator actually requires

Read out of `validation.go`, not remembered:

| type | sender | keys | rules |
|---|---|---|---|
| `task_read` (`:555`) | phone | `onlyAllowedKeys(requestId, taskId, limit, beforeEntryId)` | ids valid; `limit` an integer in 1..64 (`MaxTranscriptPageEntries`, `messages.go:22`); `beforeEntryId` optional but a valid id when present |
| `task_page` (`:561`) | companion | `onlyAllowedKeys(requestId, taskId, entries, earlierCursor, truncated, error)` | see below |
`task_read` is four lines. `task_page` is the deep one
(`validateTaskPage`, `:849`):

- `truncated` must be present and a boolean (`boolValue(...) == nil` fails)
- `entries` must be an array, never `null`, at most 64
- every entry's `id` and `turnId` a valid id, and **`id` unique across the page**
- if `error` is present: `entries` must be empty **and** `earlierCursor` absent
- each entry is one of three shapes (`validateTranscriptEntry`, `:876`):

```
  kind: user | agent | reasoning | plan | activity
        exact { id, turnId, kind, text }, text bounded

  kind: command
        allowed { id, turnId, kind, status, command, output }
        status in inProgress|completed|failed|declined
        output optional

  kind: file_change
        exact { id, turnId, kind, status, changes }
        each change: allowed { path, kind, diff }
                     path <= 4096, kind a safe display string <= 64,
                     diff optional
```

There is no `transcriptEntry` in `event.schema.json`'s `$defs` today
(`id, projectId, project, task, error, snapshot, event, actionResult,
decisionPage, decisionRequest, decisionQuestion`), so this shape has to be
written once and referenced, not inlined twice.

## Why nothing caught it

Same shape as the last two, one layer down:

```
  validateBody's type switch
        |
        |  compared by schema_matches_the_wire_test.go  -> PASSES
        v
  envelope.schema.json  "type": { "enum": [ ...20 names... ] }
        |
        |  and then nobody compares this to
        v
  envelope.schema.json  "allOf": [ ...16 branches... ]
```

The existing guard reads the **enum** and is satisfied. A type can be in the
enum and have no branch, and the guard has no way to notice, because it never
looks at the branch list. The fixtures cannot notice either: an unconstrained
body accepts every fixture anyone writes for it, so the check goes green the
harder you try.

`protocol/fixtures/` has no `task_read` or `task_page` line today — neither
valid nor invalid, confirmed by grep. (`attachment_cancel` and
`attachment_complete` do have fixtures, `session.jsonl:8-9`, and they are
checked against the real branch at `:133`.)

## What gets built

**The derived guard first, the branches second** — same order as last time, and
for the same reason: adding two branches by hand leaves the hole open for the
third. It also already earned its keep by correcting the count above.

1. **A third derived test**, in
   `companion/internal/mobileapi/contract/schema_matches_the_wire_test.go`'s
   package. It reads the message types out of `validateBody`'s switch (the
   existing helper already does exactly this and *can* be reused here — unlike
   last time, this is the same switch, on `message.Type`, a selector named
   `Type`) and reads the **type each `allOf` branch names**, by `const` *and*
   by `enum`, off disk, then compares both directions. **It must fail first,
   naming exactly those two.** If it ever finds zero branches, `t.Fatal` rather
   than pass. Done: `schema_declares_a_body_for_every_type_test.go`, red with
   `task_page, task_read`.

2. **Two branches**, each mirroring the table above — including the `sender`
   pin, which is half of what is missing.

3. **One new `$defs/transcriptEntry`** in `event.schema.json`, three sub-shapes,
   referenced by `task_page`'s `entries`. Written once.

4. **A fixture per type** in `session.jsonl`, and enough invalid fixtures in
   `schema-drift.jsonl` to prove each new branch is load-bearing rather than
   decorative — at minimum: a `task_read` sent by the companion, a `task_page`
   whose `entries` is `null`, a `task_page` with a duplicate entry id if the
   schema can express it, and an `attachment_cancel` with an extra key.

## How bad this actually is, stated honestly

**Nothing is broken on either machine.** A judge with fresh context checked the
Kotlin decoder against the Go validator for both types and they agree
key-for-key — `ProtocolCodec.kt:162-167` for `task_read`, and `:168` with
`validTaskPage`/`validTranscriptEntry` at `:402-418` for `task_page`, including
the entries-never-null rule, the unique-id set, the
error-implies-empty-entries-and-no-cursor rule, and all three transcript-entry
shapes. Both runtimes already refuse the garbage frames this schema accepts.

So this is a fix to the **published contract document**, not to shipped
behaviour. It matters because that document is what anyone writing a third
implementation would read, and for these two types it currently says "anything
goes". It should not be sold as a bug fix.

## What this still does not fix

**Uniqueness and cross-field rules the schema cannot easily say.** `entries`
having unique `id`s across a page is expressible only as `uniqueItems` on whole
objects, which is not the same rule. The `error`-implies-empty-`entries` rule is
expressible with `if`/`then` and should be written. Where the schema genuinely
cannot express a validator rule, **say so in a comment in the fixture file
rather than quietly leaving it out** — an unstated gap is how all three of these
started.

**This is the last of the enum-versus-branch family that has been measured.**
After it, `envelope.schema.json` has a branch per type and both directions are
derived. Body *contents* drifting from the validator inside a branch remains
uncaught, and no amount of this closes that.

## Done when

- [x] The derived test fails first, naming exactly `task_page, task_read`
- [x] All twenty types have a branch; the test passes both directions with no
      list of type names anywhere in it
- [x] Checked that the validator was not quietly narrowed to make it pass: the
      twenty types it derives are identical to the twenty measured before, and
      `validation.go` was not modified during the implementing agent's run at
      all (checked by modification time, not by trusting the report)
- [x] All four new invalid fixtures mutation-tested, each restored after:
      removing `task_read`'s sender const, flipping its `additionalProperties`
      to true, removing `"type":"array"` from `task_page.entries`, and removing
      the error/entries sub-rule — each produced
      `invalid fixture was accepted: schema-drift.jsonl:NN` for its own line
- [x] Nothing previously valid is now rejected — validated went 43 → 45
- [x] Counted, not trusted: Android from the JUnit XML, `go test ./...
      -count=1`, and the schema check's own two numbers, all re-run by hand
      after the agent reported. **Android is not optional** — `ProtocolContractTest.kt` decodes every `session.jsonl` line
      with the Kotlin codec and requires every `schema-drift.jsonl` line to be
      refused by it, so new fixtures are a third implementation's problem


## What was actually built

Two branches in `envelope.schema.json` — `task_read` (sender `phone`, exact keys
`requestId`/`taskId`/`limit` plus optional `beforeEntryId`, `limit` an integer
1..64) and `task_page` (sender `companion`, required
`requestId`/`taskId`/`entries`/`truncated`, with an inner `if error then
entries.maxItems: 0 and not required earlierCursor`). One new
`$defs/transcriptEntry` in `event.schema.json`, a `oneOf` of the three entry
shapes, referenced rather than inlined. `task_page`'s `error` reuses the
existing `$defs/error`.

Two settled questions, each answered from the helper rather than assumed:

- **`limit` is required on `task_read`.** `uintInRange` calls `uintValueOK`,
  which returns `(0, false)` for an absent value (`validation.go:1323`), so a
  missing `limit` always fails. Kotlin agrees for a different reason — it
  coerces the absent value to 0 and checks `!in 1..64` (`ProtocolCodec.kt:165`).
- **A `changes` entry requires only `path` and `kind`.** `onlyAllowedKeys`
  (`:1417`) forbids unknown keys and never requires anything; the real
  requirement comes from `boundedString(change["path"], 4096)` and
  `safeDisplayString(change["kind"], 64)`, which both fail on absent, while
  `diff` is guarded by an explicit nil check (`:893`).

## Verified on three implementations, not one

This is the part worth keeping. `session.jsonl` is read line by line and every
line must decode in Kotlin (`ProtocolContractTest.kt:86`), and every line of
`schema-drift.jsonl` must be **refused** by Kotlin (`:419`). So the six new
fixture lines are checked by the JSON Schema (Python), by the Kotlin decoder,
and — for the branch list — by the derived Go guard.

The implementing agent ran Go and Python only and reported success honestly on
that basis. Android was run afterwards and passed at 598/0/0. A fixture change
that had looked like a schema-only edit is in fact a change three
implementations have to agree with, and only one of the three was covered by
the report.

## Known and left alone

`boundedString` counts Unicode runes in Go; JSON Schema `maxLength` counts
code points. The two differ only for astral-plane characters (some emoji), and
only by a factor that would let a string a few characters over the Go limit
pass the schema. Not worked around — written down here so the next person does
not rediscover it as a bug.
