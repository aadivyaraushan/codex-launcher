# Adapters declared billing and approval checkpoints that nothing enforced

**Date:** 2026-08-03
**Where:** `companion/` (Go), worktree `phase0-notification-probe`
**Plan item this came out of:** `planning/consumer-app-implementation-plan.md:1110` —
*"authorization gate + class H hand-off contract, with tests proving an
unofficial, unauthorized or under-scoped verb cannot execute"*

## The short version

Every adapter is **required** to declare its gates — the extra checkpoints a
request must clear before it runs (`none`, `approval`, `billing`). Manifest
validation rejects an adapter that leaves the list empty, so all fifteen
adapter files declare one.

Outside that validator, **nothing in the product ever read the field.** Not the
runner, not the flow, not the handler. An adapter that declared a checkpoint was
executed exactly like one that declared none.

The sharp end is money. `adapters/maps` declares `Cost: CostPerCall` and
`Gates: [GateBilling]` — every call is billed to the owner's Google Cloud
account — and the runner called it with no billing check at all.

**A validator is not an enforcer.** `"unknown gate %q"` proves the word is
spelled correctly and nothing more. That distinction is the whole finding.

## Evidence

Every non-test read of `.Gates` in the repository, before the fix:

```
internal/capability/manifest/manifest.go:437    if len(m.Gates) == 0 {
internal/capability/manifest/manifest.go:440    for _, g := range m.Gates {
```

Both are inside `manifest.go`'s own validation of itself. There were no others.

Reproduced as a failing test before any code changed
(`internal/capability/execution/gate_test.go`). The failure line is the bug:

```
--- FAIL: TestABillingGatedAdapterNeverReachesItsOwnExecute
    the adapter was called and billed before the gate was checked:
    calls=[resolve execute]
```

## The part that made the first fix inadequate

The obvious fix — check the gate in `Runner.Execute` — was written, and it was
not enough. **The maps adapter spends the money during `Resolve`, not during
`Execute`.** Its own runtime test says so out loud:

```go
if api.calls != 1 {
    t.Fatalf("api calls=%d, want 1 at resolve", api.calls)
}
```

So a gate on `Execute` alone let the paid API call happen and then refused to
use the answer: the charge *and* no result — worse than either doing it or not.

This surfaced only because enforcing the gate broke three existing tests, which
is what pointed at where the cost is really paid. The lesson generalises:
**put the checkpoint at the door where the cost is actually incurred**, which
is not always the door named "execute".

The fix therefore refuses at all three doors — `Resolve`, `Preview`, `Execute`.

## Inputs → output → steps

**Input:** a plan naming an adapter whose manifest declares a gate.
**Output before:** the adapter ran, and was billed.
**Output now:** `ErrGateNotCleared`, naming the adapter and the gate, with the
adapter never called.

1. `Runner.Resolve` / `Preview` / `Execute` look the adapter up in the registry.
2. Each checks the verb is offered (unchanged).
3. Each then scans the whole `Gates` list. Any entry that is not `GateNone`
   refuses. The whole list is scanned because a `GateNone` sitting next to a
   real gate is not permission — the strictest entry decides.
4. Only then is the adapter's own method called.

## Why refuse, rather than build a way to clear a gate

Nothing in this product can clear one: there is no approval flow and no billing
consent anywhere. Inventing one is a product and money decision, not an
engineering one. Refusing is the conservative half — **refusing to run is
recoverable; a charge on someone's account is not.**

## What this costs, stated plainly

**The maps proof flow is now blocked.** `NewMaps` prepares and confirms through
the runner, so it refuses until a billing checkpoint exists. Three tests that
asserted the old behaviour were rewritten to assert the refusal, and the two
flow-level ones now also assert the fake API recorded **zero** calls.

This is a real capability regression and the owner should know about it. It is
the right trade: the alternative is billing a cloud account on every request
with no checkpoint. Maps is not registered in the production build
(`production.go` never constructs it), so no shipped path changes.

## Verified

Re-run by hand, independently of the agents that wrote the code:

- `go build ./...` — ok
- `go vet ./...` — clean
- `go test -count=1 ./...` — **82 packages ok, 0 FAIL**

`rtk proxy` is required for all of these. A bare `go test` is silently rewritten
by a shell hook into a cached path that reported `ok` for a package that had six
failing tests.

Nine tests were written before the code existed, and the implementer was told
not to edit them. One is a control (`TestAnUngatedAdapterStillExecutes`) —
without it, "refuse everything" would pass every other test and break the whole
product.

## The same bug elsewhere — the sweep

The category is *a manifest field every adapter must declare that nothing
reads*. Grepped every other field for non-test readers outside the manifest
package:

| Field | Enforced? |
|---|---|
| `Consent` | **yes** — `consent/consent.go` |
| `Verbs`, `Ceiling`, `Platform` | **yes** — runner and resolver |
| `Gates` | no → **fixed here** |
| `Cost` | **no readers anywhere** |
| `Capacity` (rate limits) | **no readers anywhere** |
| `Region` (data residency) | **no readers anywhere** |
| `ProvesCeiling` | **no readers anywhere** |
| `Auth` | printed by a proof command; never acted on |

Four fields remain declared-and-ignored. They are **not** fixed here, because
each needs a decision that is not an engineering one:

- `Cost` — unenforced means nothing budgets or warns before a paid call. What
  is the budget? Nobody has said.
- `Capacity` — unenforced means no rate limiting. YouTube's own client
  documents a real daily quota (`SearchQuotaCostPerCall = 100`), so this is not
  theoretical.
- `Region` — every adapter currently declares `global`, so there is no live
  consequence today, but nothing would catch it if one did not.
- `ProvesCeiling` — names a proof that should exist for a claimed ceiling.
  Nothing checks the named proof exists, so an adapter can claim a ceiling it
  never proved. **Measured 2026-08-03** (see below).

### ProvesCeiling, measured

88 distinct proof names are declared across the adapters. Checking each against
`saved-results/` (which is git-tracked, 69 files):

- **65** appear verbatim in a proof file.
- **23** do not appear anywhere in the repo except the compiled binary.

The 23 are not all unproven, though, and that is the actual finding. Spot
checks show `doordash_prepare_open_smoke` and `uber_estimates_prepare_open_smoke`
have a real proof — `saved-results/wave1-rides-food-handoff-pack.md` — that
simply never quotes the token. Meanwhile `applenotes` uses a completely
different convention: a Go test function name
(`TestTheFirstWriteCreatesTheAdaptersOwnFolder`).

So `ProvesCeiling` is currently unenforceable, not merely unenforced. **Three
conventions are in use at once** — a Go test name, a token quoted in a proof
file, and a token nothing quotes — so no single check can tell "proved" from
"claimed". Deciding the one convention is a small engineering call, but it is
still a call, and the wrong way to close it would be to paste the missing
tokens into proof files to make a check pass. That would turn an unverified
claim into a fabricated one.

The 23 untraceable names, for whoever makes that call:

```
applemusic_prepare_open_smoke      messages_prepare_open_smoke
audible_prepare_open_smoke         msteams_graph_work_oauth_chat_read_send
cashapp_draft_open_smoke           outlook_graph_user_oauth_mail_read_write_send
chipotle_draft_open_smoke          resy_prepare_open_smoke
discord_prepare_open_smoke         slack_user_oauth_channel_read_send
doordash_prepare_open_smoke        spotify_prepare_open_smoke
gcalendar_events_oauth_read_write  spotify_search_play_smoke
gdrive_drive_file_oauth_read_write starbucks_draft_open_smoke
maps_places_directions_smoke       todoist_write_roundtrip_smoke
uber_estimates_prepare_open_smoke  ubereats_prepare_open_smoke
venmo_draft_open_smoke             youtube_search_open_smoke
zelle_draft_open_smoke
```

Most of these are in the production build today.

How to re-measure:

```bash
cd <repo root>
grep -rhoE 'ProvesCeiling: *"[a-z0-9_]*"' companion/internal/capability/adapters/ \
  | sed 's/.*"\(.*\)"/\1/' | sort -u > /tmp/declared.txt
grep -rhoE '[a-z0-9]+_[a-z0-9_]*(smoke|read_write)' saved-results/ | sort -u > /tmp/recorded.txt
comm -23 /tmp/declared.txt /tmp/recorded.txt
```

## How to reproduce

```bash
cd companion
rtk proxy go test -count=1 -run 'Gate|Gated|Ungated' ./internal/capability/execution/
```

`-count=1` is required; Go caches test results and a cached pass here means
nothing. `rtk proxy` is required for the reason given above.
