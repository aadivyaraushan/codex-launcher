# Wave 2 judge verdict — 2026-08-03 push

**Verdict: PASS WITH FIXES.** The work is unusually honest and well-evidenced —
every COMPLETE claim I checked traces to a real on-device observation, no
code anywhere asserts a completion verb it hasn't earned, and the Beeper
failure call is sound. The one real defect is a documentation-propagation
gap: the top-of-file Pixel 9 status table and the messaging plan were
correctly demoted, but several older narrative sections deeper in
`consumer-app-implementation-plan.md` still assert unconditional COMPLETE for
rows the same file now marks demoted/unverified elsewhere — exactly the kind
of drift the plan's own discipline is meant to prevent.

## Standard, set before reading the output

From `consumer-app-implementation-plan.md`'s "Pixel 9 self-verification" and
"ceiling is a measured fact" sections, and `operator-complete-messaging-plan.md`:
- A COMPLETE claim requires someone watching the action happen on the Pixel
  (screenshot, `dumpsys`, or equivalent) — not a passing unit test, not a
  200 response, not a manifest declaration.
- Anything not driven on-device ships `unverified` or HAND-OFF, and the UI/manifest
  must say so — a stale COMPLETE anywhere (table, manifest, code comment, UI
  copy) after a demotion is the specific failure mode the plan calls out.
- Hand-off copy must never use a completion verb (paid/booked/ordered/posted/sent).
- The Beeper spike must distinguish "disproven" from "untested" honestly, and
  demotions must cascade to every place that named the old ceiling.
- Test claims (red→green counts) must be reproducible by rerunning the suite,
  and the assertions must be non-tautological.

I verified all of this by rerunning tests myself and grepping code, not by
trusting the evidence files' prose.

## Findings, ranked

### HIGH — stale unconditional "COMPLETE" claims survive in the plan body after the 2026-08-03 demotion

`planning/consumer-app-implementation-plan.md:421`: **"Personal Instagram DMs
are COMPLETE via Beeper Server (not a logged-in Operator browser)."** No
qualifier, no pointer to the spike-fail exit. This directly contradicts the
same file's own line 26 ("DEMOTED 2026-08-03 → all three HAND-OFF") and
`operator-complete-messaging-plan.md`'s explicit "SPIKE FAILED... messaging
COMPLETE is BLOCKED." The line immediately above it (line 396, "Instagram DM
send **depends on** the messaging COMPLETE fork") is appropriately
conditional — line 421 is not, so the section is internally inconsistent on
its own terms, not just against the top table.

`planning/consumer-app-implementation-plan.md:1234` ("Media Spotify Web API
COMPLETE (search + play)") and `:1276` ("YouTube COMPLETE search+open") are
older Wave-1-planning ASCII blocks with no "after smoke" / "unverified"
qualifier, contradicting the file's own line 27 ("Spotify... unverified
2026-08-03") and line 28 ("YouTube search — DEMOTED 2026-08-03... 403
PERMISSION_DENIED"). Other mentions in the same document (lines 1225, 1274,
2135) correctly carry "after smoke" or "COMPLETE only after..." — so the
convention exists, it just wasn't applied everywhere the demotion touches.

**Why this matters:** the plan explicitly frames "the ceiling is a measured
fact, not a manifest claim" as its hardest-won lesson, and a reader six
months from now skimming the Wave 1/Wave 2 narrative sections (very plausible
— those are the sections that describe what to build) will see unconditional
COMPLETE and have no reason to check the top table for the correction.

**Smallest fix:** add the same "after smoke; else HAND-OFF" / "unverified
pending owner OAuth" qualifier used elsewhere in the doc to lines 421, 1234,
and 1276 (three short edits, no restructuring needed).

**Not a HIGH in the product itself:** no code anywhere makes these claims —
`grep -rl beeper` across all `.go`/`.kt` files returns zero hits, so there is
no Beeper client, no wired messaging adapter, and nothing in the actual app
that a user could see contradicting the demotion. This is a paper-trail risk,
not a shipped-behavior risk.

### MEDIUM — YouTube manifest's single `Ceiling: Completes` doesn't reflect the verb-level demotion

`companion/internal/capability/adapters/youtube/adapter.go:53-61` declares
one manifest with `Verbs: {Read, Play}` and `Ceiling: Completes`, even though
the adapter's own evidence file (`wave1-youtube-maps-complete.md:11`) proved
`Read` (search) returns a live `403 PERMISSION_DENIED` and only `Play`
(open) is actually COMPLETE. Runtime behavior is honest — `Resolve()`
propagates the API error rather than fabricating an outcome — so no false
"Reached: Completes" can currently be produced. But the static manifest
would mislead anyone who reads `Describe()` before making a call, and the
plan's own contract has no way to express a per-verb ceiling, which this
adapter's own doc comment concedes. Low present risk because the adapter is
not wired into `cmd/codex-launcher/main.go` (confirmed by diff — zero
`spotify`/`youtube`/`maps` references there), so nothing user-facing depends
on it yet. **Fix:** split Read out at `Unverified`/`HandsOff` ceiling, or add
a comment on `Describe()` flagging the known live restriction, before this
package is ever wired into the router.

### LOW — none worth a separate line; the rest of the propagation checked out

- Messaging plan (`operator-complete-messaging-plan.md`) demotions are
  complete and consistent: scope matrix, spike-fail exit, "Done when"
  checklist, and Judge section all agree the three nets are HAND-OFF.
- No Discord bot client exists anywhere (`grep -rli "discord.*bot"` → only
  the deeplink adapter's comment stating the opposite), matching the
  owner-action-pack's claim.
- Android `CapabilityOutcome.kt` quotes in `wave1-handoff-pixel-evidence.md`
  (`claimsSuccess`, "Done here means 'staged', not 'sent'") verified verbatim
  against the actual file — accurate, not paraphrased into something rosier.

## Test-claim verification (reran myself, not trusted from prose)

- `go test ./internal/capability/adapters/spotify/... ./internal/capability/oauth/spotify/...` → 15 + 4 = 19 passed, matches the claim in `wave1-spotify-complete.md`. Spot-checked `TestExecutePlayDemotesToHandsOffOnNoActiveDevice` — asserts the Detail never contains "playback started"/"is playing" and must contain "cannot" and the track name; not tautological.
- `go test ./internal/capability/adapters/youtube/... ./internal/capability/adapters/maps/...` → 21 passed, matches `wave1-youtube-maps-complete.md`.
- `go test ./internal/capability/handoff/... ./internal/capability/adapters/deeplink/... ./internal/capability/runtime/deeplink/...` → all green, matches the 196-test claim in `wave1-handoff-pixel-evidence.md`.
- `go test ./...` (whole `companion/` tree) → 1229 passed, 87 packages, no failures.

## Copy-rule check (grepped myself)

`grep -rniE '"[^"]*(paid|booked|ordered|posted|sent|reserved|purchased)[^"]*"'` across `adapters/spotify`, `adapters/youtube`, `adapters/maps`, `capability/handoff`, and `adapters/deeplink/adapter.go` (excluding tests): **zero hits.** No completion-verb claim exists in any hand-off or new-adapter user-facing string.

## Beeper failure call

Sound, and the honest-labeling is the strongest part of this push. The spike
correctly separates "Termux disproven" (a specific, reproducible failure —
`Could not find a PHDR`, SIGABRT, Android's bionic loader cannot run the
AppImage's ELF, confirmed by `uname -a` and the missing `ld-linux-aarch64.so.1`)
from "Android 15 Linux VM untested" (blocked by a Developer Options toggle
`adb`/shell cannot flip, not by a technical dead end). It explicitly declines
to try `proot-distro` and states why (known Electron/proot sandbox
incompatibility, low odds, would blow the time budget) rather than silently
stopping. It also declined to fire a real send once blocked by the harness's
own auto-mode classifier, rather than routing around it — the right call
given the plan's own irreversible-action rule. No sign of giving up early;
the stopping point matches a real technical wall, not a convenience wall.

## What was done well (say so plainly)

- Every COMPLETE/HAND-OFF/unverified verdict I checked cites a specific
  on-device observation (screenshot + `dumpsys` state, or a named API error),
  not just a test pass — meeting the plan's bar to the letter.
- Scope cuts (not wiring youtube/maps/spotify into `main.go`, skipping
  consent/confirm UX after the Beeper spike failed) are disclosed plainly as
  decisions, not hidden as silent completions.
- The messaging plan's own "Done when" checklist is written in the demoted
  state with unchecked boxes and explanations — no attempt to make paused
  work look finished.
