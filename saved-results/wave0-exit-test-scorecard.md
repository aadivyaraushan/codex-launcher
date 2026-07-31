# Wave 0 exit-test scorecard — every clause, met or not, and who is blocking

**Date:** 2026-07-31
**Source of the bar:** `planning/consumer-app-implementation-plan.md`, lines 1064–1110. That
block is the plan's own literal definition of "Wave 0 is done". This file walks it clause by
clause so nobody has to reconstruct the answer from the plan text plus a dozen evidence files.

**Headline: Wave 0 is NOT done.** The engineering is done and green. Of fourteen clauses, **nine
are met, four are unmet, and one is partly met** — and every one of the five gaps is waiting on
a person, not on code. Three wait on the owner (a click at the Mac, a sign-in, inbound messages
to the phone, and an account named in writing), one waits on a lawyer, one waits on Apple.
Nothing on this list is blocked on more programming.

Two of the fourteen rows — 2 and 14 — are the same underlying Play-policy finding, which the
exit test happens to state twice. So the nine met clauses cover eight distinct pieces of work.

---

## The board

```
  CLAUSE                                         STATE      BLOCKED ON
  ------------------------------------------------------------------------
  1  DESIGN.md amended                           MET        --
  2  Play content-access policy answered         MET        --
  3  Notification-reply probe FINISHED (5 apps)  UNMET 2/5  owner of the phone
  4  RT-1 audit, yes/no/BD per row               MET        --
  5  3 runtimes end to end, deferrals named      MET        --
  6  Eval set green, incl. must-ask cases        MET        --
  7  No iOS claimed; manifests say android       MET today  -- (fixed 2026-07-31)
  8  An adapter switched off remotely            MET        --
  9  All 3 proving adapters driven by hand       UNMET      owner (3 separate acts)
  10 GATE money   account named + OK recorded    UNMET      owner, in writing
  11 GATE legal   class B review booked, dated   UNMET      owner must book it
  12 GATE custody model written + rotated once   MET        --
  13 GATE Apple   read-through + question asked  PART       owner must send the question
  14 GATE Play    filed, or written no-need      MET        --
  ------------------------------------------------------------------------
  MET 9   PARTIAL 1   UNMET 4        none of the gaps is a coding gap
```

**Two clauses read differently from the gate board, on purpose.** `wave0-gate-status.md` tracks
whether a gate is *cleared for a real user*; this file tracks whether the *exit-test clause* is
satisfied. They differ in two places, and the difference is real rather than a bookkeeping slip:

- **Custody**: the board says NOT CLEARED, because no non-owner's token may be held yet. The exit
  clause asks only that the model be written and the rotation path exercised once. Both are done,
  so the clause is met and the gate stays shut.
- **Play**: the board says NOT CLEARED (nothing filed). The exit clause explicitly allows a second
  branch — a written finding that no declaration is needed, resting on a named policy clause — and
  the read-through took that branch. Clause met; the gate stays open until Play actually reviews
  the app, which cannot happen before submission.

---

## Clause by clause

### 1. "DESIGN.md amended (states + notification purpose)" — MET

`DESIGN.md` carries the three new state marks (One tap left, Handed off, Unverified) with
their shapes and tones, and the notification-access purpose statement. Evidence: the marks
appear in `DESIGN.md` and are mirrored exactly in
`android/app/src/main/kotlin/app/codexlauncher/capability/outcome/CapabilityOutcome.kt`,
where a test asserts no two marks share a shape.

### 2. "the Play content-access policy answered" — MET

`saved-results/wave0-play-notification-access-policy.md`. Finding: Notification Listener is
**not** on Play's Permissions Declaration Form list, in either the current sensitive-permissions
page or its announced April 2026 update — so no declaration and no demo video are required, so
far as the public help pages document. The finding rests on named clauses, and two of its
quotes were re-fetched from the live Google pages on 2026-07-31 and matched word for word.

The file is honest about what it is: a negative finding (absence of a rule), not confirmation
that no hidden rule exists.

### 3. "the Android notification-reply probe FINISHED, with a per-app yes/no for Messages, WhatsApp, Instagram, MESSENGER and Signal written down" — **UNMET**

**2 of 5 answered**, up from 1 while this scorecard was being written.
`saved-results/wave0-notification-reply-probe.md`, measured on the real Pixel 9 (serial
`4B230DLAQ001Z5`, Android 16, SDK 36).

WhatsApp: **CAN_REPLY**, free-form RemoteInput key `direct_reply_input`.
Instagram: **CAN_REPLY**, free-form RemoteInput key `DirectNotificationConstants.DirectReply`,
15 sightings, `canned_only=0`. Instagram answered itself — real DMs arrived on the device, the
listener swept the shade unprompted, and the ledger flipped. Nothing was staged and no code
changed. That is the probe working as designed, and it is the reason it stays installed.

The plan is explicit that Messenger is on this list *because* its status was being assumed from
Instagram rather than measured, and "an assumption in the same table as five measurements reads
as a measurement". So the four unanswered rows cannot be filled in by reasoning — that is the
exact mistake the clause exists to stop.

**What each of the four needs, and it is the owner's to do:**

| App | What is missing | Installed? (checked 2026-07-31) |
|---|---|---|
| Messages | one inbound SMS to the Pixel, from any phone | yes — it has just never posted |
| Messenger | one message, after installing | **no** — neither `com.facebook.orca` nor `com.facebook.mlite` |
| Signal | one message, after installing | **no** — `org.thoughtcrime.securesms` absent |

Installing apps and receiving messages on someone's personal accounts is theirs to do, not
something to be done on their behalf. Inbound SMS also cannot be faked:
`android.provider.Telephony.SMS_RECEIVED` is a protected broadcast and the system refuses
`adb shell am broadcast`.

Messages is the cheapest of the three — the app is already there, so it needs one text message
and nothing else.

### 4. "the RT-1 audit written down with a yes/no/BD per row" — MET

`saved-results/rt1-reachability-audit.md`, ~18 connector rows, each with a verdict.

### 5. "THREE runtimes proven end to end — RT-1, RT-4, RT-6 — against one contract and one router, with RT-2 moved to Wave 1, RT-3 to Wave 2 and RT-5 to Wave 4, deliberately, under one stated rule, and named in each place" — MET

One contract (`companion/internal/capability/manifest`), one registry, one two-stage router,
and adapters for all three runtimes built against them. The three deferrals are stated in the
plan under one rule — *prove a runtime in the wave that first ships it* — with the cost of each
written out and named where each lands (Wave 1 opens with RT-2's proof as a stop-the-line
checkpoint; Wave 2 cannot start without RT-3; RT-5 waits for its own billed account).

"Proven end to end" here means the code path exists, is tested, and runs. Whether the three
adapters have been *driven by hand on real hardware* is clause 9, and it is a separate answer.

### 6. "eval set green INCLUDING the ambiguity cases that must ask" — MET

`go test ./companion/internal/capability/routing/...` → **44 passed in 4 packages**, including
the cases where the right answer is to ask the user rather than guess.

### 7. "NO iOS answer is required here and none is claimed — every adapter's manifest says `platform: android` until a real iPhone says otherwise" — MET as of today, and it was not met an hour ago

**This clause was quietly failing and writing this scorecard is what found it.** Both shipping
adapters declared `platform: both`:

- `applenotes/notes.go` said both, with a defensible-sounding comment: the companion runs on the
  Mac and the phone only talks to it, so nothing is Android-specific.
- `notion/notion.go` said both, on the same logic: it is a cloud call made by the companion.

Both readings are technically true and both are the wrong call. **No iPhone has run any part of
this.** There is no iOS client at all, and both iOS probes are deferred. `both` promises an iOS
user a route nobody has walked — and the plan's rule is flat rather than reasoned for exactly
that reason.

Fixed 2026-07-31: both manifests now say `android`, each adapter's test asserts it with the
reasoning written down, and `proveadapter`'s kill-switch drill was corrected too — it was
querying the registry with `PlatformBoth` as if it were a wildcard, which it is not (a caller
runs on one platform). Re-ran the drill by hand afterwards; it still lists `apple-notes` as the
surviving write adapter.

Grepped for `PlatformBoth` / `PlatformIOS` across the whole companion tree: 13 hits, 3 in
shipping code (all three fixed above), the rest in test fixtures and the manifest package's own
definition, which are correct as they stand.

### 8. "an adapter you have actually switched off remotely" — MET

`go run ./companion/cmd/proveadapter killswitch`. Verdict line:

```
VERDICT: kill switch proven: notion was switched off remotely and unreachable
through the registry, then restored
```

Not a unit test — the real registry, the real kill list, refused through the real `Get`, and
restored afterwards.

### 9. "ALL THREE proving adapters driven by hand on real hardware with evidence recorded" — **UNMET, all three**

`saved-results/wave0-proving-adapters-hands-on.md`. Every one of the three is blocked, and each
on a different act only the owner can perform:

| Adapter | Blocked on |
|---|---|
| RT-6 Apple Notes | macOS Automation permission — one click, by the owner, at the Mac's keyboard. macOS gates one app controlling another per (controller, target) pair; nobody can approve that on the owner's behalf. |
| RT-1 Notion | the owner signing in to their own Notion workspace. The hosted MCP is OAuth-only and refuses bearer tokens, so there is no credential to supply — it has to be a person at a browser. |
| RT-4 reply on the Pixel | the owner's say-so to actually fire a reply. This one narrowed today: the free-form reply box is now *proven present* on WhatsApp and Instagram, so the missing piece is no longer evidence. Firing one sends a real message to a real person from the owner's account, which is theirs to authorise, not something to infer. |

What *is* proven without the live runs: each adapter's manifest, ceiling claims, verb set, error
paths and refusal behaviour are covered by tests, and the RT-4 adapter answers `NOT_MEASURED`
rather than guessing — it does not claim a ceiling it has not reached. That is the right
behaviour under the block, but it is not the clause.

### 10. GATE — MONEY: "the account named in writing — literal id, and personal or work — and the owner's OK recorded against it. Nothing metered runs before this." — **UNMET**

Two sentences, and only the second one holds.

- **The account has not been named in writing and no OK is recorded against it.**
  `saved-results/wave0-gate-status.md` says `money — NOT CLEARED — owner, in writing`. This is
  the gate the plan calls out as day-one work rather than exit-day work, and it is not done.
- **Nothing metered has run.** No billed model call was made in Wave 0: the routing eval harness
  runs offline against fixtures, and the custody rotation drill uses a local master key rather
  than a billed cloud key service. The `MasterKey` interface exists so a cloud key service can be
  swapped in later — that swap is one interface implementation, and it stays behind this gate.

So the rule the gate protects has not been broken, but the gate itself is open. Wave 1's router
work calls a model, so this is the first thing that has to close, not the last.

### 11. GATE — LEGAL: "the class B review on the calendar with a date" — **UNMET**

`saved-results/wave0-gate-status.md`: *"not cleared, not yet asked"*. The plan asks for the
review to be **booked**, not held — booking has a lead time nobody here controls, and it blocks
the Wave 1 iMessage row and the whole of Wave 3. Booking a lawyer is the owner's to do.

### 12. GATE — CUSTODY: "the token-store security model written down and its rotation path exercised once, not just described" — MET, both halves

- Written: `saved-results/custody-gate-token-store-security-model.md`.
- Exercised: `go run ./companion/cmd/proveadapter rotate`. Verdict line:

```
VERDICT: rotation proven: 3 rows moved from key-1 to key-2 and verified in
63.667µs; the retired key was refused; a rotation that could not finish left
the store untouched on key-2
```

The drill runs the real path — store rows under key-1, create key-2, rotate, prove the retired
key can no longer read anything, read every row back under the new key, then run the failure
case and show the store untouched. 18 tests pin the store's shape, including that no method
can list all tokens at once (that shape is what a breach uses) and that a token carries no
handles.

### 13. GATE — APP STORE (Apple): "the guideline read-through done and Apple's answer requested on the ambiguous parts. 'Asked, awaiting reply' clears this; 'we think it's probably fine' does not." — **PARTIAL**

- Read-through: **done.** `saved-results/wave0-apple-review-guideline-readthrough.md`, sourced
  live from Apple's guidelines on 2026-07-31, cross-checked across two independent fetches, with
  its own honest note that sub-lettering under 5.1.1 was inconsistent between them. Four of its
  quotes were re-fetched and matched verbatim.
- Question to Apple: **not sent.** `wave0-gate-status.md`: *"not cleared, not yet asked"*.

The clause is written to reject exactly the state this is in. The read-through's two open
questions are ready to send — whether Login with Apple (4.8) is triggered by Operator's own
sign-up flow, and whether "opens the app with a draft loaded" reads as genuine utility (4.2)
rather than a thin wrapper (4.1/4.3). Sending a message to Apple on the owner's behalf needs
the owner's say-so, so the draft is where it stops.

### 14. GATE — PLAY POLICY: "Declaration filed, video submitted, Google's answer recorded — or, if the read-through says no declaration is needed, that finding written down with the policy clause it rests on." — MET, via the second branch

The read-through took the second branch and named its clauses. See clause 2.

One thing the file flags that the gate does not, and it is worth carrying forward: Google's own
Play Protect guidance names notification-reply hijacking as a known fraud pattern. Operator's
mechanism looks like it from the outside even though it never reads message content. That is a
review-flag risk, not a policy violation, and the file recommends recording a short demo video
anyway — as a precaution, explicitly not as a documented requirement.

---

## What has to happen for Wave 0 to close

In order of lead time, longest first — the first three can be started today and then waited on.

1. **Book the class B legal review** (clause 11). Longest lead time, blocks Wave 1's iMessage
   row and all of Wave 3. Book it, do not hold it.
2. **Send Apple the two open questions** (clause 13). "Asked, awaiting reply" is enough to
   clear the gate, so the clock starts the moment it is sent.
3. **Name the model-provider account in writing and record your OK against it** (clause 10) —
   literal account id, and whether it is personal or work. Takes a minute, but Wave 1's router
   work is the first thing that calls a model, so nothing metered can start until it is done.
4. **Approve macOS Automation for the companion** (clause 9, RT-6). One click at the Mac.
5. **Sign in to Notion in a browser** (clause 9, RT-1). One OAuth screen.
6. **Send the Pixel one SMS; install Messenger and Signal from your own Play account and get one
   message on each** (clauses 3 and 9-RT-4). Three rows left, and the SMS one needs no install.
   Instagram closed itself while this was being written, so leaving the probe running is a real
   strategy rather than a stall.
7. Re-run the three hands-on drills and update
   `saved-results/wave0-proving-adapters-hands-on.md` and
   `saved-results/wave0-notification-reply-probe.md` with the results. Instructions for redoing
   each are already in those files.

## How to reproduce the green half

```bash
go build ./... && go vet ./companion/... && go test ./companion/...
```
→ 814 passed in 58 packages, vet clean (2026-07-31).

```bash
cd android && ./gradlew :app:testDebugUnitTest :app:assembleDebug
```
→ 347 tests, 0 failures, 0 errors; assembleDebug SUCCESSFUL (2026-07-31).

```bash
go run ./companion/cmd/proveadapter killswitch
go run ./companion/cmd/proveadapter rotate
```
→ the two verdict lines quoted above.
