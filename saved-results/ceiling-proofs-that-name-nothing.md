# Every shipped adapter's "proof" names a test that does not exist

**Date:** 2026-08-03
**What this is for:** the record of a defect in Operator's honest-gate-reporting
mechanism, and the ratchet test that now stops it getting worse. Read this
before you trust any adapter's ceiling claim, and before you raise or lower the
pinned number in the test.

---

## The one-paragraph version

Every capability adapter declares `ProvesCeiling` — the name of the smoke test
that proves it can really do what it says. A contract test refuses any shipped
adapter that leaves it blank, with the words *"names no smoke test for its
ceiling, so the claim rests on nothing"*. But that test only checks the string
is **not empty**. Nothing checked that the name belonged to a test that exists.
Measured on 2026-08-03: of the 14 adapters this build ships, **zero** name a
proof that exists. All 14 point at a snake_case name no file in the repo
defines. So the sentence in the contract test — "the claim rests on nothing" —
was true of every adapter it was letting through.

## Inputs → outputs → how it was measured

**Input:** every adapter registered in this build, via the existing
`everyAdapter(t, nil)` helper, plus every Go test function name defined
anywhere under `companion/` (found by walking to the folder holding `go.mod`
and regexing `^func (Test[A-Za-z0-9_]*)\(` out of every `*_test.go`).

**Output:** each shipped adapter's `ProvesCeiling` value sorted into "resolves
to a real test function" or "resolves to nothing".

**Steps:** for each adapter, skip it if `Unshipped` is non-empty (same
exemption, same reason, as the manifest contract test: an adapter no build
registers makes no claim to anyone); skip it if `ProvesCeiling` is blank
(that is already someone else's failure and should not be counted twice);
otherwise look the name up in the set of real test names.

## The measurement

```
named proofs that resolve to a real test: 0
named proofs that resolve to nothing: 14
  gcalendar          -> gcalendar_events_oauth_read_write
  gdrive             -> gdrive_drive_file_oauth_read_write
  instagram          -> instagram_draft_open_smoke
  maps               -> maps_places_directions_smoke
  msteams            -> msteams_graph_work_oauth_chat_read_send
  notification_reply -> notification_reply_remote_input_accepted_smoke
  notion             -> notion_read_roundtrip_smoke
  outlook            -> outlook_graph_user_oauth_mail_read_write_send
  podcasts           -> podcasts_rss_enclosure_play_smoke
  slack              -> slack_user_oauth_channel_read_send
  spotify            -> spotify_search_play_smoke
  todoist            -> todoist_write_roundtrip_smoke
  venmo              -> venmo_draft_open_smoke
  youtube            -> youtube_search_open_smoke
```

**The field is not a bad idea that nobody understood.** Two adapters use it
exactly right: `applereminders` names `TestTheFirstWriteCreatesTheAdaptersOwnList`
and `applenotes` names `TestTheFirstWriteCreatesTheAdaptersOwnFolder`, and both
of those are real functions that run. But both are marked `Unshipped`. The only
two adapters that do it properly are the two that do not ship.

## What was changed

**1. A ratchet test, not a fix.**
`companion/internal/capability/runtime/proof_names_resolve_test.go` holds the
dangling count at a pinned 14. Adding an adapter with an invented proof name
now fails. Writing a real smoke lowers the count, and the test then tells you
to lower the pin — so the number cannot drift in either direction unnoticed.

The pin is a **debt, not a target.** The only direction it should ever move is
down.

*Why pinned rather than fixed:* writing the 14 missing smokes mostly needs
vendor accounts nobody has yet (Slack, Notion, Todoist, Spotify, Microsoft
Graph, Google OAuth). And whether a given ceiling claim should be **dropped**
instead of proven is a call for whoever owns the product, not something to
decide inside a test file.

**2. A method that lied, renamed.**
`manifest.CeilingIsProven()` returned `true` whenever the string was non-empty
— so it answered *"ceiling proven: yes"* for all 14 adapters whose proof
resolves to nothing. It had zero production callers, so there was no live bug,
only a trap for whoever called it first. Renamed to `manifest.NamesAProof()`,
which is what it actually computes, with the gap written into its comment.
Its own test was already called `...ClaimsAProvableCeiling`, so the test author
had the distinction right and only the method name missed it.

## How to reproduce

```bash
cd companion

# The measurement itself. Set the pin to 0 first if you want to see the
# full dangling list printed as a failure.
go test ./internal/capability/runtime/ -run TestEveryNamedProofResolvesToATestThatExists -v

# Whole suite, to confirm nothing else moved.
go test ./... > /tmp/gotest_out.txt 2>&1 ; echo "EXIT=$?"
grep -c "^FAIL" /tmp/gotest_out.txt   # expect 0
grep -c "^ok"   /tmp/gotest_out.txt   # expect 83
```

**Verified 2026-08-03:** red at a pin of 0, green at 14 — both directions, so
the ratchet is not passing by finding nothing. Full Go suite `EXIT=0`, 0 FAIL,
83 `ok` packages, both renamed manifest tests passing.

## Traps that cost time here

- **`rtk` silently truncated the 14-line list to 3.** The count said 14 and the
  listing showed 3. If those two disagree, believe the count and re-run writing
  to a file in `/tmp`, then grep the file.
- **zsh eats an unquoted `--include=*.go`** with "no matches found", and the
  resulting empty output looks exactly like a genuine zero. Quote it.
- **`go build` succeeding is not the suite passing.** Count `^ok` and `^FAIL`
  lines out of a saved file; do not read the word "SUCCESS" as a result.

## A second one from the same pass: a tap nobody can make

Same question — "does this name something real?" — asked of the phone's
outcome model instead of the manifest.

`CapabilityOutcome.confirmControl` (`CapabilityOutcome.kt:145`) holds the name
of the control that finishes a run stopped one tap short. It is set to
`"Confirm"` for a `ONE_TAP` ceiling at `:245`, and **read by nothing** in
`app/src/main`. `CapabilitySheet`'s RESULT dialog renders the detail line, the
message and `recoveryAction`; its buttons are Copy draft, Open *app*,
Disconnect *name*, and Done. None of them comes from `confirmControl`.

Read alone that looks like a live, user-visible bug, because
`ConnectionNotificationPolicy.kt:33` pushes *"One tap left / Open Codex
Launcher to finish it"* — sending someone to a dialog whose only button is
Done. **It is not reachable today.** The ceiling arrives from the Mac through
`Ceiling.fromWire`, and no Go adapter declares `one_tap`: the only two hits in
`companion/` are the constant itself (`manifest.go:109`) and the wire validator
(`validation.go:1107`). So it is a trap set for whoever ships the first
`one_tap` adapter, not a defect users can hit now.

Guarded by `companion/internal/capability/runtime/one_tap_needs_a_control_test.go`,
which fails the moment a shipped adapter declares `one_tap` and says in its
message what to do about it. It is a tripwire, not a rule against the feature —
whoever renders `confirmControl` deletes the test in the same change.

**Proven not vacuous:** swapping the compared constant to `HandsOff` makes it
fail naming `[venmo instagram youtube]`; restored to `OneTap` it passes. A
guard that has never been seen to fire is not evidence of anything.

*Not the same defect, checked:* the other non-blank assertions in the Kotlin
tests (`recoveryAction`, `detail`, failure messages) are about **prose shown to
the user** — there, non-blank is the correct assertion, because the string is
the substance rather than a reference to something else. `recoveryAction` is
rendered straight as `Text` in `CapabilitySheet.kt:92`.

## Where this is written down

- `planning/consumer-app-implementation-plan.md` ~line 1218, under the
  `ProvesCeiling` entry — the original claim there was *"it is a discipline
  device rather than runtime data, and it works"*, now corrected.
