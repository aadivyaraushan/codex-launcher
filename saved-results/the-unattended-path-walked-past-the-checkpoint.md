# The check that stops a charge was only on the path a person watches

**Date:** 2026-08-03
**Where:** `companion/internal/capability/{manifest,execution,verification}/`, worktree `phase0-notification-probe`
**Plan item this came out of:** the "declared but unenforced" family in
`planning/consumer-app-implementation-plan.md`. Same family as
[`every-failure-was-called-an-invalid-request.md`](every-failure-was-called-an-invalid-request.md).

## The short version

A **gate** is an adapter's way of saying "this needs a checkpoint before it
runs" — someone's approval, or a billing account that has agreed to be charged.
Nothing in this product can clear one today, so every door has to refuse.

`execution.Runner` did refuse, at all three of its doors. But `execution.Runner`
is only the path a person is waiting on. Three other entry points hold the
adapter registry directly and call `Resolve` and `Execute` themselves:

- `verification.Tier1Runner.Run` — the nightly probe
- `verification.Tier2Runner.ConnectLoop` — sends for real, on the user's account
- `verification.Tier2Runner.Heartbeat` — runs at every wake

None of them checked a gate. **These are the unattended paths** — a nightly job
and a wake heartbeat, with nobody watching. That is the worse half to have
missed, not the safer one: a charge on a user-facing request gets noticed when
it happens; a charge on a nightly job repeats every night until someone reads a
bill.

It was latent — `NewTier1` and `NewTier2` have no production callers yet, which
is exactly why it was worth closing now. Wiring the scheduler is the moment the
hole becomes real, and by then whoever does the wiring has no reason to look
here.

## Why the fix was to move the rule, not copy it

`execution` imports `verification` (for Telemetry), so `verification` cannot
import `execution` back. That import loop is *why* the check was skipped rather
than reused — the easy fix was blocked, so it got dropped.

Copying `checkGate` into `verification` would have left two copies of one rule
to drift apart, which is this codebase's most repeated bug. So the rule moved to
the one place both packages already import — the manifest itself:

```go
func (m Manifest) CheckGates() error {
    for _, g := range m.Gates {
        if g != GateNone {
            return fmt.Errorf("%w: %s requires %s", ErrGateNotCleared, m.ID, g)
        }
    }
    return nil
}
```

`execution.checkGate` and `execution.ErrGateNotCleared` were **deleted**, not
wrapped or aliased. Five test files were repointed at the new names. Verified by
reading the diff myself: those five changed nothing but the symbol and its
import — no assertion moved.

## Inputs → output → steps

**Input:** an adapter about to be used, and which door it came through.
**Output:** either the adapter runs, or the caller gets `ErrGateNotCleared` and
the adapter is **never touched at all** — not called and then discarded.

1. Every door calls `a.Describe().CheckGates()` before touching the adapter.
2. Any entry that is not `GateNone` refuses. A list of nothing but `GateNone`
   passes — a permissive entry is not permission, but it is not a gate either.
3. `handler.go`'s `capabilityFailureCode` maps the error to **`unauthorized`**
   for the phone: the request was fine, something the user could clear is
   missing. Unchanged by this work, and checked.

## The two things that were nearly got wrong

**The check must be at the door where the cost is paid.** Gating only at
`Execute` was useless for the one adapter that declares a billing gate: maps
bills during `Resolve` (its own test asserts "api calls=1, want 1 at resolve"),
so a late check means the bill lands *and* the runner then refuses to use the
answer. All three doors check.

**A gate refusal must not disable the adapter.** Disabling is what a *failed*
heartbeat means — the session died, so fail the next task fast. A gate is not a
failure; the adapter is fine and untouched. In `Heartbeat` the check sits after
the registry lookup but before `Resolve`, and returns the error directly instead
of routing through `reg.Disable`, so a gated adapter stays registered and
enabled. There is a test that only exists to hold that line.

## Controls

Three of the seven tests exist purely to stop the obvious over-correction —
without them, "refuse everything" passes every other test and the nightly
verification silently never runs again:

- an adapter with no gate must still run through **all three** doors, and be
  observed to have actually executed (`executes == 1`), not merely to have
  returned no error;
- a `Gates` list of nothing but `GateNone` must pass;
- the user-facing runner and the unattended runner must refuse with the **same**
  error value, or code that handles one silently misses the other.

The test adapter counts `Resolve` calls as well as `Execute` calls, so a test
can prove the adapter was never touched rather than merely that it did not
finish.

## Evidence

Baseline before the change: 82 packages ok, with `internal/capability/verification`
failing to build (the spec was red on `CheckGates undefined`,
`manifest.ErrGateNotCleared` undefined).

After, run by me rather than taken from the agent that wrote the code:

```
rtk proxy go build ./...          -> clean
rtk proxy go vet ./...            -> clean
rtk proxy go test -count=1 ./...  -> 82 packages ok, 0 FAIL
```

All seven spec tests named individually with `-v` → PASS. `gate_test.go` itself
byte-identical to the spec as written — I read it back to confirm, because a
test file that quietly changes is how a green run stops meaning anything.

## How to reproduce

```bash
cd companion
rtk proxy go test -count=1 -v -run "GateNobodyCanClear|GateRefusalDoesNotDisable|NoGateStillRunsEverywhere|GateNoneIsNotAGate|BothRunnersRefuseWithTheSameError" ./internal/capability/verification/
```

`rtk proxy` is required and so is `-count=1`. A bare `go test` is rewritten by a
shell hook into a cached path and has reported `ok` for a package with six
failing tests.

## What this does not fix

`Cost`, `Capacity` (rate limits) and `Region` (data residency) are still
declared by every adapter and read by nothing. `Gates` was the one with a bill
attached, so it went first. The others each need an owner decision about what
enforcement should even mean, not more code.
