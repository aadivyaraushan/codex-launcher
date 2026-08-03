# The phone used one word for "it didn't happen" and "we don't know"

**Date:** 2026-08-03
**Where:** `android/app/src/main/kotlin/app/codexlauncher/task/management/`, worktree `phase0-notification-probe`
**Plan item this came out of:** the outcome-honesty row of
`planning/consumer-app-implementation-plan.md` — telling a person the truth
about what happened to something they asked for.

## The short version

Renaming, archiving or forking a task can end three ways, and the person
holding the phone needs a different thing from each:

| what happened | what they need |
|---|---|
| it happened | nothing said |
| it did not happen | try again — nothing anywhere changed |
| we cannot tell | go look at the computer before touching it again |

`TaskActionOutcome` had one value, `Unavailable`, covering the last two. Ten
places returned it. **Four mean the request never left the phone. Six mean it
was already on its way, or already carried out, when something went wrong at
our end.**

The screen then showed all ten the same warning:

> Task action unconfirmed
> The computer did not confirm whether this change happened. Check Codex on
> your computer before trying again.

So someone whose phone was simply not connected was sent to inspect a computer
where, by construction, nothing had happened at all. And it cut both ways: the
warning is the *right* one for the six real cases, so it could not be softened
without hiding a genuine risk of doing something twice.

## Evidence

The failing test, before any code changed. It compares the two situations
directly rather than naming a value, so it fails on behaviour and not on a
missing symbol:

```
TaskActionBridgeTest > nothingSentIsNotReportedTheSameWayAsSentButNeverAnswered FAILED
    java.lang.AssertionError at TaskActionBridgeTest.kt:168
8 tests completed, 1 failed
```

The two things it built were "send returned NOT_SENT" and "sent, then the
connection closed before any answer". Both came back `Unavailable`.

## Inputs → output → steps

**Input:** a rename, archive or fork, and whatever the connection and the
computer then do.
**Output before:** `Complete`, `Forked`, `Invalid`, `NeedsReview`,
`Failed(code)` or `Unavailable` — with `Unavailable` meaning two opposite
things.
**Output now:** `Unavailable` is gone. In its place:

- **`NotSent`** — nothing left the phone; nothing anywhere changed; safe to
  try again.
- **`Unresolved`** — it was sent, or already carried out, and we cannot say
  what happened.

1. The four before-the-socket refusals return `NotSent`: a fork blocked
   because the journal could not be read, a duplicate action id, a journal
   write that failed before sending, and a send that reported `NOT_SENT`.
2. The five after-the-socket ones return `Unresolved`: the connection closing
   with the request still in flight, a missing sent-record, a journal write
   that failed *after* the answer arrived, a confirmed fork that named no new
   task, and an answer in a state we do not recognise.
3. `LauncherSessionViewModel` returning "there is no session for this task"
   also became `NotSent` — nothing was sent, by definition.
4. The screen picks one of two dialogs from a single classifier, so the three
   menu items cannot drift apart:
   - `NotSent`, `Invalid`, `Failed` → **"Nothing changed"** — *"That didn't go
     through, so nothing on your computer changed. You can try again."*
   - `Unresolved` → the existing **"Task action unconfirmed"** dialog, word for
     word as before.

## The second conflation, in the same function

`terminal.state == "failed" || terminal.state == "outcome_unknown" ->
TaskActionOutcome.Failed(error)` had the identical problem one level down: the
computer saying *"it did not happen"* and the computer saying *"I don't know"*
both became `Failed`. Now an `outcome_unknown` state, or any error the
computer itself tagged as outcome-unknown, returns `Unresolved`; a genuine
`failed` with any other code still returns `Failed(error)`.

**What gets written to the journal did not change** — only the word reported
back. The storage side already had this right (`ActionRecordState.SENT_UNKNOWN`
exists precisely for "sent, outcome unknown"), which is what made the reporting
gap easy to miss: the data was honest and the message was not.

## Why the name `Unavailable` had to go

Not tidiness. A name that truthfully covers both situations is exactly what
lets them be confused — every one of the ten sites was individually defensible
under it. Deleting it forces each site to say which one it means, and the
compiler now refuses any that does not. There is no alias and no deprecated
case; the old path is not reachable.

## Verified

Run by hand, not taken from the agent that wrote the code:

```
./gradlew :app:testDebugUnitTest --rerun-tasks   ->  BUILD SUCCESSFUL
tests=490 skipped=0 failures=0 errors=0
```

counted from `app/build/test-results/testDebugUnitTest/*.xml` rather than from
Gradle's console line, because a Gradle run that skips its tests as up-to-date
still prints BUILD SUCCESSFUL. `--rerun-tasks` forces the run; the XML proves
it happened.

Eight tests were written before the code existed, in
`app/src/test/kotlin/app/codexlauncher/task/management/TaskActionHonestyTest.kt`,
and the implementer was told not to edit them. Two are controls — a refusal
from the computer must still be `Failed`, and a confirmed action must still be
`Complete`. Without them, "answer `Unresolved` to everything" passes every
other test in the file and ships a worse product than the bug being fixed.

One instrumented test was added for the new dialog
(`TaskScreenTest.taskActionThatNeverLeftThePhoneSaysNothingChangedNotUnconfirmed`).
It compiles (`:app:compileDebugAndroidTestKotlin` succeeded) but **has not been
run** — instrumented tests need the Pixel, which was not attached for this
work.

## The same bug elsewhere — the sweep

The category is *one outcome value covering both "it did not happen" and "we
cannot tell"*. Checked every other outcome type on the phone:

| Type | Verdict |
|---|---|
| `TaskActionOutcome` | the bug → fixed here |
| `ActionSendResult` (`NOT_SENT` / `SENT_UNKNOWN`) | fine — this is the honest split already, and is where `NotSent` gets its name |
| `StateMark` | fine — has a dedicated `UNVERIFIED` for "we cannot vouch for this", separate from `FAILED` |
| `ActionRecordState` (`SENT_UNKNOWN`) | fine — storage already distinguished the two |
| `SessionFailure`, `SessionHandshake.Output` | connection-level, no outcome claim about a user's request |

Also noted while sweeping, and **not** fixed here: `CapabilityOutcome.toTaskState()`
(`capability/outcome/CapabilityOutcome.kt:112`) still has zero callers in
`app/src/main/`, so its `UNVERIFIED` mapping cannot fire. That is the separate
"built but unreachable" problem, not this one.

## How to reproduce

```bash
cd android
./gradlew :app:testDebugUnitTest --tests "*TaskAction*" --rerun-tasks
```

`--rerun-tasks` matters: without it Gradle can report success without running
anything. Do not pass `-q` — it hides failures.
