# Cost, Capacity and Region are decoration

**Date:** 2026-08-03
**Status:** Measured. No code changed — setting the values is an owner decision,
but this write-up narrows what that decision is actually about.
**What this is for:** three fields on every capability manifest look like
protections and enforce nothing. The plan lists them as "needs owner input on
the values". The more useful fact is that no value would change any behaviour.

---

## The claim these fields make

Every adapter's manifest declares a `Cost`, a `Capacity` and a `Region`
(`companion/internal/capability/manifest/manifest.go`). All 18 adapter
construction sites fill them in. Reading the manifest, they look like a
per-adapter budget, rate limit and geographic restriction.

## What they actually do

| Field | Read by | Enforced | Validated |
|---|---|---|---|
| `Cost` | `Validate()` only (`manifest.go:479-480`) | **No** | it is one of three known strings |
| `Capacity` | `Validate()` only (`manifest.go:490-491`) | **No** | `Kind` only — `Limit` is never checked |
| `Region` | **nothing** | **No** | **not at all** — no branch in `Validate()` |

Verified by grep over `companion/`, excluding `_test.go`:

- `.Region` — **zero** non-test reads. Every occurrence is a struct-literal
  write (`Region: []string{"global"}`, 18 sites).
- `Capacity.Admits(connected int)` exists (`manifest.go:321`) and has **no
  callers at all** outside its own definition. Nothing tracks how many
  connections are open, so nothing could call it.
- `Cost` — no gate, billing check or runner branch inspects it. An adapter
  declaring a metered cost runs exactly like a free one.

`execution/runner.go` gates on `CheckGates()` (lines 94, 114, 164) — the
separate `Gates` field. That is the only gating that happens.

Also worth knowing: `Capacity.Limit` is bounds-checked inside `ParseCapacity`
(`manifest.go:266-268`), but all 18 adapters are built as Go struct literals and
set `Limit` directly, skipping that parse. So the one check that exists is on a
path nothing takes.

## Why this is the same defect twice over

This is the pattern already found in this repo with `ProvesCeiling` and the
accessor once called `CeilingIsProven`: **the shape is checked and the substance
is not.** `Validate()` confirms `Cost` is a legal word. It does not confirm —
and nothing anywhere confirms — that declaring an expensive cost has any
consequence.

Every adapter currently declares `Region: []string{"global"}`, so a region
restriction has never been exercised even by accident.

## What this means for the owner decision

The plan records these as blocked on owner input: a budget number, a rate limit,
a region list. That framing overstates what a value would buy. Supplying all
three today changes nothing at runtime, because there is no reader to change.

So the decision is really two decisions, and the first is not about values:

1. **Should these be enforced at all, or removed?** A declared-and-ignored
   restriction is worse than no field, because it reads like a guarantee. If
   they are meant to be real, they need a consulting site — most naturally
   beside `CheckGates()` in `execution/runner.go`, which is the existing place a
   manifest can stop a run.
2. **Only then, what values?** — the budget, rate and region answers.

## It is not hypothetical — two adapters declare a cap today

An earlier draft of this file said the gap costs nothing "while every adapter
says `global` and `free`". That was wrong, and checking the declared values
rather than assuming them is what showed it:

- **`spotify`** — `Capacity{Kind: CapacityCapped, Limit: 25}`, `Gates: [GateNone]`
  (`spotify/adapter.go:54,59`)
- **`youtube`** — `Capacity{Kind: CapacityCapped, Limit: 100}`, `Gates: [GateNone]`
  (`youtube/adapter.go:60,61`)

Both declare a real limit and no gate, so nothing will ever apply either one.

**`maps` is deliberately not in that list**, and the difference is the point. It
declares `Cost: CostPerCall` *alongside* `Gates: [GateBilling]`
(`maps/adapter.go:73`), and gates are genuinely enforced by `CheckGates()`. Its
cost declaration is backed by something that runs. So the defect is not
"declares a restriction" — it is "declares a restriction with nothing behind
it".

`Region` is still uniform: all shipped adapters say `global`, so that one is a
trap for the future rather than a live gap.

## The tests that hold it

`companion/internal/capability/runtime/declared_limits_nothing_keeps_test.go` —
a ratchet pinning the two unenforced caps so the number cannot grow, and a
tripwire that fires the day any adapter declares a narrower region than
`global`. Both proven able to fail: giving `spotify` a real gate fired the
ratchet (in the "pin is now too high" direction), and setting `youtube`'s region
to `us` fired the tripwire. Both adapter files were then restored and the full
suite re-run: **0 FAIL, 83 `ok` packages**.

## How to re-check

```bash
cd companion
grep -rn "\.Region" . --include="*.go" | grep -v _test.go   # expect: none
grep -rn "Admits(" . --include="*.go" | grep -v _test.go    # expect: the definition only
sed -n '463,497p' internal/capability/manifest/manifest.go  # Validate(), all of it
grep -n "CheckGates" internal/capability/execution/runner.go
```
