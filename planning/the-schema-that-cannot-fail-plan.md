# The schema that cannot fail

**One sentence:** more than half the action kinds the wire genuinely carries are
missing from the checked-in schema, and no check can notice, because the only
thing that reads the schema is a set of hand-written examples that never
included one.

Status 2026-08-03: **built and green.** All thirteen kinds declared, the
derived guard passes in both directions, `go test ./... -count=1` 83 packages
ok / 0 FAIL, Android 598 tests / 0 failures / 0 errors counted from the JUnit
XML, and the schema check went from "validated 35 / rejected 40" to
**"validated 43 / rejected 41"** — eight new frames exercised, one new
rejection proved load-bearing.

Android is not incidental here: `ProtocolContractTest.kt` decodes every line of
`session.jsonl` with the Kotlin codec and requires every line of
`schema-drift.jsonl` to be refused by it, so a new fixture is a third
implementation's problem too. The implementing agent ran Go and Python only;
Android was run afterwards and passed.

---

## How it was found

Writing a fixture for the question feature meant adding an ordinary line to
`protocol/fixtures/session.jsonl`:

```json
{"type":"action","sender":"phone","body":{"actionId":"action-capability-1","kind":"capability_request","utterance":"message Maya"}}
```

`release/checks/protocol/schema_test.py` rejected it. Not because the frame is
wrong — the Go validator and the Kotlin decoder both accept it and the app
sends it every time somebody asks for an app action — but because
`action.schema.json` has never heard of it.

## The measurement

Derived from `validation.go`'s own `switch kind` and from
`action.schema.json`'s `oneOf`, not from anybody's memory:

```
  validator accepts (13)        schema knows (6)         missing (7)
  ----------------------        ----------------         -----------
  start_turn                    start_turn
  steer_turn                    steer_turn
  interrupt_turn                interrupt_turn
  approval                      approval
  question_response             question_response
  set_project                   set_project
  capability_request       ---------------------->  capability_request
  capability_confirm       ---------------------->  capability_confirm
  capability_disconnect    ---------------------->  capability_disconnect
  archive_task             ---------------------->  archive_task
  fork_task                ---------------------->  fork_task
  rename_task              ---------------------->  rename_task
  dismiss_unknown_control  ---------------------->  dismiss_unknown_control
```

Every capability action is missing. So is every task-management action except
the three oldest.

## Why nothing caught it

```
   validation.go  ProtocolCodec.kt        <- the two implementations
        |               |
        |               |                    (nothing compares these
        |               |                     to the schema)
        v               v
   action.schema.json  <----- read only by ----- protocol/fixtures/*.jsonl
                                                  ^
                                                  |
                                        hand-written, and nobody
                                        ever wrote one for these seven
```

The schema is only ever exercised through fixtures. A kind with no fixture is a
kind the schema is never asked about. **Both sides of the check are written by
the same person in the same sitting** — the identical defect already found and
fixed twice in this repo (`runtime/contract_every_adapter_test.go`, and the
message-*type* half of `schema_matches_the_wire_test.go`).

`schema_matches_the_wire_test.go` derives and compares **message type names**,
and it passes. It says nothing about body shapes, so it is green today with
seven kinds missing.

## What gets built

**The derived guard first, the schema second.** Adding the seven branches by
hand and stopping would leave the same hole for the eighth.

1. **A derived test.** A second comparison in `schema_matches_the_wire_test.go`:
   pull the `switch kind` inside `validateAction` out of `validation.go` with
   `go/ast`, read the `kind` `const`/`enum` values out of `action.schema.json`,
   report the difference. Neither side hand-written. **It must fail first,
   naming all seven.**

   **This is not a reuse of the existing walk, and saying so was wrong.** The
   existing extractor matches `sw.Tag.(*ast.SelectorExpr)` with `Sel.Name ==
   "Type"` (`schema_matches_the_wire_test.go:71-73`) — it is built for
   `message.Type`. `validateAction` switches on a bare local, `switch kind {`
   (`validation.go:1009`), which is an `ast.Ident`, not a selector. That
   matcher finds nothing here. The new one keys on the enclosing function name
   instead. Multi-literal cases (`case "archive_task", "fork_task":`,
   `validation.go:1074`) are fine — the existing per-clause `BasicLit` loop
   already handles them and does so today for
   `"attachment_cancel", "attachment_complete"` (`:638`).
2. **The seven schema branches**, each mirroring what the validator actually
   requires — key sets from `exactKeys`/`onlyAllowedKeys`, id patterns from
   `validID`, lengths from `boundedString`.
3. **An eighth branch, for a kind the schema already "knows".** The schema's
   `start_turn`/`steer_turn` branch (`action.schema.json:6-17`) requires
   `taskId` + `text`. The validator accepts a *second* shape for `start_turn` —
   a new task with `projectId`, `modelId`, `reasoningId`, `permissionModeId`
   and no `taskId` at all (`validation.go:1028-1031`, and identically in
   `ProtocolCodec.kt:265-269`). No fixture has ever carried it, so nothing
   noticed. It is the same bug inside a kind this plan's own table calls
   "known", which is worth saying out loud: **the derived guard fixes enum
   parity and does nothing for body parity.** Do not let this change be read as
   making the schema trustworthy.
4. **A fixture per kind** in `session.jsonl`, so the Python check exercises each
   new branch rather than trusting it — including one for the new-task shape.

## What this does not fix, stated plainly

**Enum parity is not body parity.** After this, the schema will list all
thirteen kinds and the derived guard will keep that true forever. It will still
be possible for a branch's *required fields* to drift from the validator's, and
nothing here will catch it — the `start_turn` new-task shape above is a live
example, found only because a human read both. The only reason to add fixtures
in step 4 is that a fixture is the one thing that exercises a branch's shape.

## Done when

- [x] The derived test failed first, naming exactly the seven:
      `archive_task, capability_confirm, capability_disconnect,
      capability_request, dismiss_unknown_control, fork_task, rename_task`
- [x] `action.schema.json` carries all thirteen kinds in twelve branches, plus
      the `start_turn` new-task shape
- [x] The derived test passes in both directions, with no list anywhere in it.
      Checked that the validator was not quietly widened to make it pass: the
      thirteen kinds it derives are identical to the thirteen measured before
      any of this was written
- [x] Eight valid fixtures, one per new shape. One invalid fixture
      (`schema-drift.jsonl:41`) **mutation-tested**: flipping that branch's
      `additionalProperties` to `true` makes the check fail with "invalid
      fixture was accepted", and flipping it back makes it pass
- [x] Nothing that shipped before is now rejected — "validated N" went 35 → 43
      and no previously-valid fixture failed
- [x] `go test ./... -count=1`: 83 ok / 0 FAIL. Schema check: validated 43 /
      rejected 41

## Found while building, fixed

`attachmentIds` is capped at 16 by `validateOptionalIDs` (`validation.go:1296`)
and the existing `start_turn`/`steer_turn` branch had no `maxItems` at all
(`action.schema.json:15`). A 17-item list would have passed the published
contract and been refused by both real decoders. One word, now on both
branches.

## Deliberately not in scope

- **`device_action` bodies.** The guard's header says those drifted too. Same
  shape of work, separate change.
- **Making the Kotlin decoder a third derived side.** It is a different
  language and a different parse; the two-way Go↔schema guard closes the hole
  that actually shipped.
