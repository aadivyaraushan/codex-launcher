# Wave 5 — the cross-adapter contract suite, and three ways to fail open

**Date:** 2026-08-03
**What for:** Wave 5 of the two-plan finish loop. The contract suite is a named deliverable
of the consumer app plan: "every adapter satisfies the same contract — declares its verbs,
previews before send, revokes completely, fails closed when its runtime is down."
It did not exist. Writing it turned up three separate places where the system failed open.
**Callers:** the loop-operator session; `saved-results/loop-operator-two-plans-status.md`;
follows `saved-results/wave4-built-but-unreachable.md`.
**Same-purpose search:** Glob `**/wave5-*.md` — this file only.

## Why a suite, rather than more adapter tests

Both real adapter bugs found this week were one adapter quietly breaking a rule that every
adapter shares, and neither adapter's own tests could see it, because each was only ever
checked against itself. Maps navigation returned a result naming an app at a `completes`
ceiling — a shape the phone drops on arrival (`ProtocolCodec.kt:187`), so that path had
never once rendered. YouTube claimed a ceiling its own search call could not reach.

The check that catches that shape is not another adapter test. It is one set of rules held
against whatever production actually registered.

`companion/internal/capability/runtime/contract_test.go`. It finds its adapters by asking
the live registry for each of the nine verbs in turn and dropping repeats, so a new adapter
falls under every rule the moment it is registered, with nobody having to remember the file
exists. This needed one small change to production code: `Inventory` gained an unexported
`reg` field (`production.go:91`), unexported because nothing outside the package should
reach past the flow to the adapters.

Measured coverage, not assumed: **77 adapters**, 77 undeclared-verb probes, 17 previews
checked, **90 outcomes** checked for ceiling honesty.

## The rules, and what each one prevents

| Rule | Prevents |
|---|---|
| Manifest validates; declares at least one verb; ceiling is real; names a smoke test | An adapter that can never be routed to but still reports itself available |
| Resolve refuses a verb the adapter never declared | A capability nobody agreed to and no manifest, review or kill list can see |
| Every preview-requiring verb yields a non-blank headline and confirm label, and the preview's fingerprint matches the plan it came from | A blank sheet that still takes a tap — consent theatre — and a preview that shows one thing while Execute checks another |
| Revoke succeeds, and succeeds again on a second call | A user tapping disconnect, seeing an error, and being told they are still connected |
| Every outcome reports a real ceiling; naming an app means hands_off; hands_off names an app | Exactly the Maps bug, caught in the manifest rather than in one code path |

**One rule was deliberately left out.** Adapters are not asked to check that a plan is
theirs, because no production path can hand them someone else's: `flow.Service` keeps the
resolved plan on the companion and a phone confirms by fingerprint, never by sending a plan
back (`flow/service.go:118,149`). Demanding fifty adapters guard a door that does not exist
would be inventing a requirement. The one real gap of that shape was a single check in a
single place — see below.

## The suite's first version was vacuous, and that is the point worth keeping

The ceiling-honesty rule **passed on its first run while asserting nothing**. Its probe
intent set no `Body`, so every `Resolve` failed, every loop iteration hit `continue`, and
the test reported green having executed zero adapters. It was measured, not guessed: a
throwaway probe printed `executes=0`.

A test that passes for free is a test that will never fail. Both loop-shaped rules now
count what they checked and fail outright if the count is zero.

## Three ways the system failed open

Found by asking, of each fix, "where else does this shape appear?" — each has a test that
was red first.

1. **`Runner.Execute` ran verbs an adapter never declared.** `Resolve` had always refused
   them (`runner.go:75`); `Execute` looked the adapter up by the plan's id and ran whatever
   the plan said. The preview check could not stop it — verbs that need no preview walk
   straight past. Red evidence: the runner returned `{Reached:completes Done:true}` for a
   verb the adapter never offered.

2. **`Runner.Preview` had the same hole.** Less dangerous — nothing irreversible happens —
   but it is the one the user sees: Operator would render a confirm sheet for a capability
   nobody agreed to, the user taps confirm, and only then does Execute refuse. Both doors
   now refuse at the earliest point.

3. **`Registry.RecordMeasuredCeiling` stored anything it was handed.** This is the worst of
   the three, because it is permanent. `Rank()` scores an unrecognised value as 0, below
   every real ceiling, so a junk measurement reads as a *fall* and becomes the adapter's
   recorded ceiling — and unlike a real fall, nothing about it is true. Nothing undoes it
   except three clean runs the adapter may never get. The red test showed an adapter whose
   effective ceiling was the empty string and which read as **proven**.

   The runner already refused these before reporting one (wave 4). But `verification.go`
   records measurements directly at three call sites (`:197`, `:245`, `:333`), all of which
   skipped that guard. The fix went into `RecordMeasuredCeiling` itself — every path that
   measures an adapter comes through there, so it is the one place that closes all of them,
   including paths nobody has written yet.

**Same-bug sweep.** Searched for direct `.Execute(ctx` / `.Preview(ctx` calls bypassing the
runner: 3 hits, all in `verification.go`, all of which resolve first, so the verb is already
checked — no change needed. Searched for every caller of `RecordMeasuredCeiling`: 3 in
`verification.go`, plus `Telemetry.Observe` from the runner — all now covered by the single
registry guard.

## Verified

`cd companion && rtk proxy go test ./...` — **0 failures, 78 packages ok** *at the point the
fixes above landed*. (Plain `go test` output is filtered by the rtk shell hook and reports
misleading counts; `rtk proxy` is what gives the real thing.)

**Correction, same day.** That green run was real but it stopped being true within the hour,
and this file said otherwise until a review caught it. After the run I added
`internal/capability/flow/consent_gate_test.go`, which calls a five-argument `flow.New` that
did not exist yet — only the test half of the cycle landed, so the `flow` package stopped
compiling and the suite went to **1 failure**. I did not re-run after adding the file, so the
number recorded here described a tree that no longer existed.

The rule this breaks is the one that caught the vacuous test earlier in the same round: a
green run you did not observe *after* the change is not evidence about the change. A recorded
test count is only about the exact tree it ran against, and writing a red test is a change
like any other.

## The lesson worth keeping

Wave 4's lesson was that a per-feature proof makes a feature demonstrable without making it
reachable. Wave 5's is the same shape one level up: a per-adapter test makes an adapter
correct against itself without making it correct against the rules every adapter shares.
Both times the test that found the problem was a test about the *system*, not about a part.
