# Saying "we could not find out" — ceiling-rot alerts and the unverified outcome

**Date:** 2026-08-03
**Worktree:** `.claude/worktrees/phase0-notification-probe`
**What this is for:** the round that followed
[kill-switch-and-disconnect-round.md](kill-switch-and-disconnect-round.md).
Two pieces of Operator that were fully built and had no caller, plus the
discovery that the project already had the design we were about to reinvent.

---

## 1. Ceiling-rot alerts — switching on a signal that was already being computed

`verification.Telemetry` folds every real production capability outcome back in
and works out whether an adapter has quietly fallen short of what its manifest
claims. It sets an `Alert` flag on the report. **Nothing had ever read that
flag.** The computation ran on live traffic and the answer went nowhere.

Making it reachable needed three small things and no new logic:

| Piece | File |
| --- | --- |
| a way to list what to check | `registry.Registry.AdapterIDs()` |
| a way to reach the *live* Telemetry, not a fresh empty one | `execution.Runner.Telemetry()` |
| the sweep itself, with a no-repeat rule | `internal/capability/verification/alerts/` |

The new package copies the shape of `killswitch.go` deliberately — narrow
`Source` interface, `New`, `Sweep`, `Run(ctx, every)`, private
`run(ctx, tick func())` — so watcher-style code in this repo stays one pattern.

**The two rules with teeth**, both written as tests before the code:

- **Something already alerting does not alert again.** A thing that shouts every
  fifteen minutes for a week gets muted, and then the one that matters is
  missed too.
- **An adapter that cannot be read is not treated as recovered.** This is the
  same mistake the kill switch exists to prevent: a failed read must never look
  like good news.

**Wired at** `cmd/codex-launcher/production.go:89`, right beside
`startKillSwitch`, sweeping every 5 minutes. It is in-memory only — no network
call, no third-party account, no cost — which is why it can sweep far more often
than the 15-minute kill-list check.

**Verified by hand, not from a summary:**

```
inv.reg = reg
runner := execution.New(reg)
inv.tel = runner.Telemetry()          # runtime/production.go:258-261
return flow.New(..., runner, ...), inv, nil
```

`runner` is the same one handed to `flow.New`, so `Inventory.Report` reads the
Telemetry that production traffic actually feeds. `Inventory{}` is constructed
nowhere but inside `NewProduction`, and every other occurrence is a zero value
returned beside a non-nil error, so there is no path with a nil `tel`.

```
ok  .../internal/capability/verification         1.961s
ok  .../internal/capability/verification/alerts  2.243s   (-race)
```

---

## 2. `StateMark.UNVERIFIED` — a state the phone's own model could not produce

`StateMark.UNVERIFIED` (`CapabilityOutcome.kt:90`) means "we do not know". It had
existed for a long time, and **`CapabilityOutcome.of` had no way to ask for it**.
`toTaskState()` carried a comment asserting it never would.

This is the same failure as the two above, one layer further in: not an unwired
subsystem, but a *UI state that existed in the model and was unreachable*.

`of()` now takes `certain: Boolean = true`. When false it returns UNVERIFIED with
**both** claim flags false, checked ahead of `done` and `ceiling` — not knowing
outranks any claim either would otherwise make. The default keeps every existing
caller producing exactly what it always did.

**The rule that matters most:** the recovery must never say "try again".

> Retrying a send that may already have gone sends it twice, and the person on
> the other end gets the message twice with no idea why.

It says `"Check Signal to see if it sent"` instead.

Adding `TaskState.UNVERIFIED` broke three exhaustive `when`s, which is the point
— each was decided explicitly rather than silenced with a catch-all:

| Where | Decision |
| --- | --- |
| `CapabilityOutcome.kt:107` | its own state, so the home list and the sheet say the same thing |
| `HomeUiState.kt:29` | no badge — the sheet's outcome already carries the mark |
| `ConnectionNotificationPolicy.kt:32` | *does* notify: "Couldn't confirm that happened" |

```
405 tests, 0 failures     (was 397; +8 new)
```

Counted from `app/build/test-results/testDebugUnitTest/TEST-*.xml`, off a
`--rerun-tasks` run, because a Gradle run that finishes in under a second ran
nothing.

---

## 3. The finding that reshaped the plan

While reading `ProtocolCodec.kt` to design the wire change, this turned up on the
`action_result` validation line:

```
!validOptionalError(body["error"], state in setOf("failed", "outcome_unknown"))
```

**Operator already ships a complete "we could not find out" design, end to end.**
Not for capabilities — for actions. `outcome_unknown` is a peer terminal state
beside `confirmed`/`failed`/`cancelled` (`ProtocolCodec.kt:583`) with its own
transitions (`:756-757`), its own `TaskQueueState` (`TaskSummary.kt:36`), its own
Go side (`promptqueue/queue.go:86`, `handler.go:716`), and a screen that renders
it. Four independent phone readers branch on it.

And that screen (`TaskControls.kt:65-90`) is the whole rulebook:

```
  1. say it plainly ....... "Queued follow-up outcome unknown."
  2. name where to check .. "Check Codex on your computer before sending another."
  3. BLOCK the retry ...... followUpsBlocked = queueState == OUTCOME_UNKNOWN
  4. let them close it .... [ I checked Codex ]  -> clears the state
```

The RT-4 plan had twice proposed a `certain` boolean on `capability_result`
instead. That would have given capabilities a second, incompatible vocabulary for
something actions already say correctly — and it covered only steps 1 and 2.

**Three open questions closed at once:**

- *Boolean or third state?* Third state. Follow the precedent.
- *May an uncertain result name the app it went to?* Yes — the conflict was
  imaginary. Naming an app while uncertain is **recovery guidance**
  ("Check Signal"), not a hand-off **claim**. `CapabilityOutcome.of` already
  keeps these apart: it uses `app` for the sentence and leaves `handedOffToApp`
  null. `handedOffTo` on the wire keeps meaning "control definitely went here"
  and stays gated on `done`. Nothing needs to change.
- *Is the wire change additive?* **No.** `capability_result` validates with
  `body.keys != setOf(...)` — an exact key set (`ProtocolCodec.kt:184`). An
  unpatched peer rejects any new key outright, so phone and companion must
  change together.

---

## 4. Two bugs this uncovered, both now fixed

`LauncherSessionViewModel.fail()` calls `capabilityController.clear()`
(**line 1337**). So: the user confirms "reply to Sarah on Signal", the phone
sends `capability_confirm`, the connection dies — and the sheet simply vanishes.
The user is never told anything. The reply may well have been sent.

That is the one case where the phone knows **on its own** that it does not know,
with no wire field involved. `fail()` now calls `sessionLost()` instead —
`closeCurrent()` deliberately still calls `clear()`, because a shutdown the user
asked for is not a lost session. The capability path now has steps 3 and 4 of
the rulebook above: a banner that outlives the dismissed sheet, an "I checked"
button, and `request()` refusing to send until it is pressed. Dismissing the
sheet does **not** count as having checked — closing a panel is not the same as
going and looking, which is why the shipped follow-up version gives its button
its own line too.

### And the draft it was quietly deleting — `LauncherSessionViewModel.kt:530`

```kotlin
if (pending != null && !outcome.claimsFailure) { clearConfirmedDraft(...) }
```

Only one of the three endings claims success:

| ceiling | done | claimsSuccess | claimsFailure | what really happened |
| --- | --- | --- | --- | --- |
| `completes` | true | **true** | false | Operator did it |
| `one_tap` | true | false | false | staged — **nothing sent yet** |
| `hands_off` | true | false | false | control left; we cannot see |
| any | false | false | **true** | it broke |

(`CapabilityOutcome.kt:213`, `:241`, `:264`.)

So "not a failure" deleted the draft after a one-tap result where **nothing had
been sent** and a button was still waiting to be pressed, and after a hand-off
where we cannot see whether it landed — exactly when someone is most likely to
want their words back. Now `outcome.claimsSuccess`: throw someone's words away
only when we know the thing they wanted actually happened.

The red run isolated it perfectly — the two guard tests (`completes` clears, a
failure keeps) passed from the start, and only the two new rules failed.

```
419 tests, 0 failures
```

Grepped `claimsFailure` / `claimsSuccess` across `android/app/src/main` and
`companion/`: one decision site, now fixed; every other hit is the definition
itself or a log line. Nothing equivalent exists on the Go side.

---

## 5. What the judge found — the warning that erased itself

A fresh-context Opus judge worked out what a good answer to this problem would
look like *before* reading any of the code, and was deliberately not told what I
suspected. It found that the retry block — the part of the rulebook with teeth —
could be cleared without anyone checking anything.

I confirmed all three in the code myself rather than taking the report:

| Where | What happens | Confirmed at |
| --- | --- | --- |
| automatic reconnect | `fail()` sets the check, then `scheduleRetry()` → `startConnection(force=true)` → `closeCurrent()` → `clear()` rebuilds the state and the check is gone about a second later | `LauncherSessionViewModel.kt:200-203`, `:1373`; `CapabilityInteraction.kt:365` |
| the destination toggle | guards only on `busy`, and `RESULT` is not busy; the control sits on the home screen and is always tappable | `CapabilityInteraction.kt:87-97` |
| a failed send | the NOT_SENT rollback `.copy()`s off the *current* state, so it can force `phase` back to `PREVIEW` under a live "outcome unknown" banner while claiming "Nothing was changed" | `CapabilityInteraction.kt:160-181` |

The first is the sharpest, because it is automatic. The banner appears for
roughly a second, disappears with nobody having looked at anything, the block
lifts, and the next prompt goes out — a duplicate message to a real person,
produced by the feature built to stop duplicate messages to real people.

**One cause, one fix.** `unresolvedCheck` lives *inside*
`CapabilityInteractionState`, and several methods replace that state wholesale.
Moving it out of the wholesale replacement means **only `markChecked()` can
clear it**, which shuts all three doors and any future one. Patching three call
sites would have left the fourth.

The third is testable without threads: `sendAction` is called *outside* the
lock, so a test's send lambda can drop the session and then report NOT_SENT,
forcing exactly that interleaving on purpose.

**Fixed and verified by hand.** `unresolvedCheck` now lives in a private
`pendingCheck` field, and one helper is the only thing in the file that writes
state:

```kotlin
private fun publish(next: CapabilityInteractionState) {
    mutableState.value = next.copy(unresolvedCheck = pendingCheck)   // :94
}
```

Seventeen direct assignments became `publish(...)` calls. Grepping
`mutableState.value = ` in that file now returns exactly one hit, line 94.
`pendingCheck` is set in one place (`sessionLost`, `:365`) and cleared in one
place (`markChecked`, `:387`). `respond()` gained the staleness guard at `:190`:
`if (confirmationActionId != action.first) return false`.

```
423 tests, 0 failures, 0 errors
```

Counted from the XML myself off a `--rerun-tasks` run, not taken from the
implementer's report. The four spec tests were confirmed failing first
(`BUILD FAILED in 6s`) — and one of them originally passed *before* the fix
existed, because asserting only the end state matched the buggy behaviour too.
It was rewritten to assert the refusal in the middle. A test that is green
before the code exists is not a test.

### And a fourth: `toTaskState()` has no caller

Section 2 above added `TaskState.UNVERIFIED`, mapped it in `HomeUiState`, and
gave it the push notice "Couldn't confirm that happened". A grep of
`app/src/main` for `toTaskState` returns its own definition
(`CapabilityOutcome.kt:108`) and one mention inside a comment. Nothing calls it,
so none of that can fire. Phone in a pocket when the connection drops mid-send
means no notification at all.

Worth naming how this got past three careful decisions: **the compiler helped it
happen.** Adding an enum value broke three exhaustive `when`s, each branch was
decided explicitly rather than silenced, and all three went green — while the
function tying them together had no caller. Exhaustiveness proves every branch
is *handled*. It never proves the thing is *reached*. That is the fifth sighting
of this pattern in this project, and the first where the safety net produced the
false confidence.

---

## 6. The wire change the plan spent three rounds designing was never needed

The plan's item 5 was "add a third terminal state to `capability_result`" — a
contract change, phone and companion together, because the body is validated
against an exact key set. Before building it, one question: *does the companion
actually have an uncertainty it cannot express?*

It has plenty. But the wire already has the word for it:

| | |
| --- | --- |
| `outcome_unknown` is a valid `action_result` state | `ProtocolCodec.kt:198` |
| `outcome_unknown` is a valid error code | `ProtocolCodec.kt:591` |
| the companion already sends that exact shape | `handler.go:1408-1409` |

**Zero contract change.** The defect was one line:

```go
outcome, err := handler.capabilityFlow.Confirm(...)
if err != nil {
    ... publishCapabilityActionResult(ctx, sender, actionID, "failed")   // every error
}
```

Every error became `"failed"` with a hardcoded `invalid_action`, because
`runner.go:143-146` flattens every adapter error into one shape. That is right
for most of them — no credentials, verb not offered, consent refused, a server
that plainly heard us and said no. It is wrong for the ones where the request
left the machine and only the *reply* went missing.

The bar, copied from the precedent already in the tree
(`appserver/client.go:445-489`, gated on `mutatingMethod` at `:1134-1141`):
**request demonstrably transmitted, response demonstrably lost, operation
demonstrably mutating.** All three, or it is a failure.

Nine adapters can lose a reply that way — every HTTP-backed one, plus both
osascript-backed Apple ones, where the Apple Event can reach Reminders.app
before the process is killed by a deadline. The classification is deliberately
narrow, e.g. `slack/client.go:177`:

```go
if method != http.MethodGet {   // a read that fails changed nothing
    return &adapter.OutcomeUnknownError{...}
}
```

A non-2xx status is never unknown — the server heard us and answered.

**The trap this round nearly repeated.** Steps 1 and 2 (the marker type, the
handler routing) would have passed every test and shipped as dead code. Step 3,
wiring the adapters, is what makes it reachable, and it is the step that looks
optional. The implementer was told so explicitly and asked for a per-adapter
reachability statement. I then grepped rather than believing it:

```
$ grep -rln "OutcomeUnknownError" internal/capability/adapters/ | grep -v _test
applenotes/runner.go  applereminders/runner.go  gcalendar/client.go
gdrive/client.go      msteams/client.go         notion/session.go
outlook/client.go     slack/client.go           todoist/client.go        (9/9)
```

### The other bug in the same file

`runner.go:205-207` returned an error when `tel.Observe` failed. Observe is pure
bookkeeping — it reads the registry and records a measured ceiling, touching
nothing outside. But the caller discarded the good outcome
(`flow/service.go:174-177`), and the phone was told the action failed. **The
message was already sent.** Told it failed, someone sends it again.

Now the failure is logged and the real outcome is returned. A failure to write
down what happened is never a failure of the thing itself. The red run said it
in one line: `a send that went through was reported as an error: unknown
adapter: sms`.

---

## How to reproduce

```
cd <repo>/.claude/worktrees/phase0-notification-probe

cd companion
rtk proxy go test ./internal/capability/verification/... -count=1 -race

cd ../android
rtk proxy ./gradlew testDebugUnitTest --rerun-tasks
find . -name "TEST-*.xml" -path "*testDebugUnitTest*" -exec cat {} + \
  | grep -o 'tests="[0-9]*"' | awk -F'"' '{t+=$2} END {print t}'
```

`rtk` filters command output, so anything whose real output matters needs
`rtk proxy`. `./gradlew ... -q` prints nothing on a *failing* Kotlin build.

**No money was spent and no external service was contacted in this round.** The
alert sweep is in-memory only. The tier-1 nightly verification runner remains
deliberately unwired: it would drive probes against roughly 40 third-party test
accounts, which needs a named account and the owner's explicit approval.
