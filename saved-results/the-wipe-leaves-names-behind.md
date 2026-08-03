# The wipe leaves people's names behind

**Date:** 2026-08-03
**Status:** **Open — needs an owner decision. Nothing about the behaviour was changed.**
Current behaviour is now pinned by tests so the decision cannot be made by accident.
**What this is for:** "Wipe local state" clears ten stores and leaves an
eleventh, and that eleventh is the only one holding real people's names.

---

## What is true today

`LocalStateWiper` deletes exactly ten things (`storage/wipe/LocalStateWiper.kt:31-43`):
project selection, action records, last connection, resume cursor, draft
ciphertext, draft key, device identity, pairing key, pairing record, capability
unresolved-check.

The reply **stop list** is not among them. Nothing under `storage/wipe/`
mentions it — grepped for `ReplyStopStore`, `stoppedThreads` and `reply_stops`:
zero hits.

## Why that matters more than "do stops survive a wipe"

The stop list is persisted by `storage/reply/stops/ReplyStopStore.kt`. For each
stopped conversation it writes an Android package name **and a person's name, in
plain text**, as JSON, into a preferences file called `reply_stops`
(`ReplyStopStore.kt:56-61`). It is restored at process start
(`LauncherApplication.kt:36`), so it survives restarts as intended.

So the question is not only "should a wipe forget who you stopped". It is that a
wipe today leaves **names of real people** on the device.

That sits badly next to a rule this codebase already wrote down for itself. From
`LiveReplyBoxesTest`'s header: a conversation title is *who*, not *what*, it is
needed to reply to the right person, and so it "is kept in memory only, for
exactly as long as the PendingIntent it is paired with, and **it never reaches
`NotificationSighting`, the on-disk ledger, or a log line**."

`ReplyStopStore` is a place where a name does reach disk. Nothing records that
anyone weighed the exception.

## The two answers are not actually opposed

The apparent trade — keep the stops, or keep the names off disk — dissolves on
inspection.

**The stored name is never displayed.** The only read of `ThreadKey.person`
anywhere in `android/app/src/main` is the line in `ReplyStopStore` that writes
it out. Verified by grep: one hit, the write. The guard matches on whole
`ThreadKey` equality and keys its maps by an internal normalised value
(`ReplyGuard.kt:24-30`).

A one-way hash of package-plus-person would therefore keep every stop working
across a restart and put no names on disk at all.

**Why that was not done here.** It changes an on-disk format, which brings a
migration question with a dangerous answer available: dropping existing entries
would silently resume conversations the user had explicitly stopped. Choosing
how to migrate is the owner's call, and getting it wrong fails in the direction
that actually harms someone.

## Deliberate or accidental

Accidental, as far as anything records. No comment in `LocalStateWiper.kt`, no
test, and neither plan mentions it. `ReplyGuard.kt:100-104` does explain that
the **rate-cap history** is in-memory only on purpose — so the file shows the
author thinking about persistence — but says nothing about the stop list versus
a wipe. Every other store the wiper touches has a wipe entry; this one was never
added.

## Which way it errs today

For **reply suppression**, safe: a wipe runs in-process without restarting
(`LauncherActivity.kt:279-291` navigates to `PAIRING`, no process kill), so both
the in-memory set and the on-disk copy survive and a stopped conversation stays
stopped. A wipe cannot make Operator start replying to someone again.

For **data removal**, unsafe: the names stay.

## The two ways to close it

- **(a) Add a wipe step.** Simple, and it makes "wipe" mean what it says. Costs
  the user their stop list, which is the unsafe direction for suppression.
- **(b) Store a hash instead of the name.** Keeps stops through a wipe *and*
  puts no names on disk. Needs a migration decision for existing files.

(b) is better on the merits and is the one with a real question attached.

## The tests that hold it

`android/app/src/test/kotlin/app/codexlauncher/storage/wipe/WipeDoesNotClearTheStopListTest.kt`
— 3 tests, green, pinning the current ten-item list and the absence of any
reply-stop step. Proven able to fail: adding a `REPLY_STOPS` member to the real
`WipeStep` enum turned 2 of the 3 red; the production file was then restored
byte-identical and the suite re-run.

Verified from the JUnit XML, not from Gradle's wording: **616 → 619 tests / 0
failures / 0 errors**, 82 XML files.

## How to re-check

```bash
cd android/app/src/main/kotlin/app/codexlauncher
sed -n '31,43p' storage/wipe/LocalStateWiper.kt          # the ten deletions
sed -n '56,61p' storage/reply/stops/ReplyStopStore.kt    # the name, in plain text
grep -rn "\.person" .                                     # one hit: the write
grep -rni "reply_stops\|ReplyStopStore" storage/wipe/     # expect none
```
