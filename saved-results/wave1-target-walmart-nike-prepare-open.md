# Wave 1 Target / Walmart / Nike prepare-and-open (shopping)

**Date:** 2026-08-02  
**Purpose:** Record overnight Wave-3 shopping prepare-and-open hand-offs for
Target, Walmart, and Nike (browse/open only; no UCP cart API). Append after
Expedia; `Wave1Specs` count **48** (was 45).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Shopping prepare-and-open pack (Wave1Specs 45 → 48); verb **read**
only; new AppClass `shopping`; TDD; Play HTTP already verified 200; do **not**
add Sephora/Wayfair; evidence here; no commit; restart serve.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `target` / `walmart` / `nike`; Play packages HTTP 200
   (2026-08-02); existing deeplink prepare-and-open pattern (finance/Expedia peers).
2. **Outputs:** `Wave1Specs`=48; stage1 coaches `shopping`+read without cart/
   order/checkout claims; HandOffActions maps display names → packages; proof
   log `shopping=target+walmart+nike`; flow ready log `shopping_adapters=3`;
   serve `adapter_count=48`.
3. **Algorithm:** Tests first (count 48, route, coaching, HandOffActions, ready
   log) → RED → add Specs + coaching + packages + logs → GREEN → restart serve.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Target** | Wave-3 shopping; no UCP cart API in overnight pack. Browse/open only. |
| **Walmart** | Same — prepare-and-open read; never claim cart/order/checkout. |
| **Nike** | Same — prepare-and-open read; never claim cart/order/checkout. |

Do **not** add Sephora / Wayfair in this pack.  
Outcomes use `handoff.DraftOutcome` (never claims cart built / ordered /
checkout completed).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `target` | Target | `com.target.ui` | `shopping` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `walmart` | Walmart | `com.walmart.android` | `shopping` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `nike` | Nike | `com.nike.omega` | `shopping` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `target_prepare_open_smoke`, `walmart_prepare_open_smoke`,
`nike_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **48** entries (indices 45–47 after expedia at 44).
- Stage2 `ClassMap` `shopping` → target+walmart+nike (built dynamically from Specs).
- Stage1 stable class list includes `shopping`; coaching lines for Target/
  Walmart/Nike with verb read; never claim cart built / ordered / checkout completed.
- Android `HandOffActions` maps `target`/`Target`, `walmart`/`Walmart`, `nike`/`Nike`.
- `deeplink_proof.go` logs `shopping=target+walmart+nike`.
- Runtime ready log adds `shopping_adapters` (same pattern as finance/notes).

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel unpaired —
no device smoke.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 45, want 48`
- `unknown adapter: target|walmart|nike`
- panic on `Wave1Specs()[45]`
- flow: I don't know which app to use for "shopping"
- ready log missing `shopping_adapters`
- stage1 instructions missing `shopping` / `target` / `walmart` / `nike`
- HandOffActions missing Target/Walmart/Nike package asserts

**Green:**

```bash
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# → 5 packages ok; 99 PASS (-v)

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# → BUILD SUCCESSFUL
```

**Serve:** rebuild `/tmp/codex-launcher-deeplink`; `serve-deeplink-proof` log shows
`adapter_count=48`, `shopping_adapters=3`, `shopping=target+walmart+nike`.

## Sibling search

Searched `target`, `walmart`, `nike`, `shopping`, `com.target.ui`,
`com.walmart.android`, `com.nike.omega`, `shopping_adapters` under companion +
android — only this pack’s Specs / coaching / HandOffActions / proof / ready
logs. No Sephora/Wayfair Specs added (explicit non-goal).

| Kind | Found |
|---|---|
| Wave1Specs shopping | Added three Specs only |
| HandOffActions | Added three package maps |
| stage1 stable classes | Extended list with `shopping` |
| flow ready log | Added `shopping_adapters` |

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was
**not** run this pass.

## How to reuse

Re-run the verify commands above; confirm `len(Wave1Specs())==48` and serve
ready log `adapter_count=48` + `shopping=target+walmart+nike`.
