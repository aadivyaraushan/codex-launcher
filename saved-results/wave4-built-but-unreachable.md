# Wave 4 — the "built but unreachable" audit

**Date:** 2026-08-03
**What for:** Wave 4 of the two-plan finish loop. A Stop hook rejected the claim that
everything left in both plans was an owner act. It was right: a survey of both plans
against the repo found a large amount of finished code that no user can reach.
**Callers:** the loop-operator session; `saved-results/loop-operator-two-plans-status.md`.
**Same-purpose search:** Glob `**/wave4-*.md` — this file only.

## The headline

The single biggest gap in the project was not a missing feature. It was that roughly
fifty finished adapters, the preview sheet and the whole confirm path could not be
reached by any real user.

`companion/cmd/codex-launcher/main.go:343-345` — the plain `serve` path — never assigned
`dependencies.capabilityFlow`. Every assignment to it in that file sits inside an
owner-only `serve-<name>-proof` branch. With it left nil,
`companion/internal/app/mobilesession/handler.go:861` refuses **every** capability request
a phone can send, returning "failed" before any adapter is consulted.

Verified directly, not taken from an agent's summary: a scan of `main.go` found the plain
`serve` branch at line 343 and zero `dependencies.capabilityFlow =` assignments reachable
from it.

Why it hid for so long: every adapter had its own proof command, and each of those wires
its own single-adapter flow. So every adapter was demonstrably working, one at a time,
through a path no user takes.

## The same shape, found four more times

| What | Evidence | State |
|---|---|---|
| Consent store — 331 lines, `Screen/Grant/Allow/WhyNot/Revoke/RevokeAll/Wipe`, fully unit-tested | `grep` for imports of `capability/consent` outside its own package returns **nothing** | Never consulted before an action runs |
| `Runner.Revoke` — revokes an adapter and unregisters it | Only callers are `proving/{spotify,todoist,slack}/proof.go`. No protocol message, no UI | A user has no way to revoke anything |
| Kill switch — `registry.ApplyKillList` | Only caller is the `proveadapter killswitch` one-off | Never fetched or applied in production `serve` |
| RT-4 reply stack — the probe, the rules, and the send | `grep` for `ReplyCapability`, `ReplyAdapter`, `ReplySender` outside their own packages returns **nothing** | Whole feature unreachable from the app |

## What was fixed this round

Two real bugs in the ceiling contract, both found by a fresh judge with no shared context
and then confirmed by reading the code rather than trusting the report:

1. **An unknown ceiling sailed through unclamped.** `Ceiling.Rank()`
   (`manifest.go:115-126`) returns 0 for anything it does not recognise, including the
   empty string, and `AtMost` (`manifest.go:131-136`) keeps whichever ranks lower. So an
   adapter that forgot to set `Reached` beat every real ceiling. The empty string then
   reached telemetry and was written down as that adapter's *measured* ceiling, and the
   phone silently dropped the envelope. One adapter bug both lost the answer and corrupted
   the adapter's record. Now refused outright with `ErrUnknownCeiling`, before telemetry
   sees anything.

2. **A hand-off claiming done while naming no app survived.** With a `hands_off` manifest
   and a `hands_off` result, nothing changed during the clamp, so neither arm of the
   `done` switch fired and `Done: true` stood. The phone discards exactly that shape
   (`ProtocolCodec.kt:187`, first clause). Now a `hands_off` result that names no app is
   never done.

3. **The proof harness did not obey the ceiling rule.** `cmd/proveadapter` called
   `a.Execute` directly, bypassing the runner, and `printOutcome` printed that raw ceiling
   — and those printed lines are what gets pasted into the evidence files under
   `saved-results/`. So the harness that produced our ceiling *evidence* was the one path
   that skipped the ceiling *rule*. All four call sites now route through
   `execution.Runner`, matching what `proving/{spotify,todoist}/proof.go` already did.

4. **The RT-4 send half now exists.** `ReplyAdapter.kt:14-16` had declared the send "out of
   scope", so RT-4 could decide whether a reply was allowed and then nothing carried that
   decision out. Added `DeliveryResult`, `ReplyDispatch`, `ReplySender` and the thin
   Android boundary. The rule it protects: never tell someone a message was sent when it
   was not — a stale notification, a cancelled PendingIntent, or any thrown exception all
   come back as not sent.

## What is still genuinely blocked on a person

Unchanged from the previous wave, and re-confirmed:

- The Android Linux VM terminal toggle. The Beeper AppImage cannot run under Termux
  (its loader has no PHDR under Android's C library), so the VM is the only remaining
  path for the messaging send story. No shell command flips that toggle.
- The three-runtimes gate: a macOS Automation permission click (RT-6), a Notion OAuth
  sign-in (RT-1), and — for RT-4 — one inbound message **plus** explicit permission to
  send a real message to a real person from the owner's account.
- Messenger and Signal are not installed, holding the notification-reply probe at 2 of 5.
- The legal, money, custody and distribution gates are all NOT CLEARED and every one of
  them is an email or a submission only the owner can send.

## The lesson worth keeping

A per-feature proof command makes a feature *demonstrable* without making it *reachable*.
Every adapter here passed its own proof and none of them could be used. The test that
catches this is not another adapter test — it is a test on the production build itself,
asserting that everything registered is routable and everything routable is registered.
That test now exists (`internal/capability/runtime/production_test.go`).
