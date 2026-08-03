# Wave 1 Sephora / Wayfair / Kayak prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Wave-3 shopping + travel prepare-and-open hand-offs
for Sephora, Wayfair (browse/open only), and Kayak (search/read; book demoted).
Append after Target/Walmart/Nike; `Wave1Specs` count **51** (was 48).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Shopping+Kayak prepare-and-open pack (Wave1Specs 48 → 51); shopping
verb **read** only; Kayak travel **read**; Play HTTP already verified 200; do
**not** use Hotels.com packages (404); do **not** add Priceline; evidence here;
no commit; restart serve; Pixel unpaired — skip device smoke.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `sephora` / `wayfair` / `kayak`; Play packages HTTP
   200 (2026-08-02); existing Target/Nike shopping + Expedia travel patterns.
2. **Outputs:** `Wave1Specs`=51; stage1 coaches sephora/wayfair shopping+read and
   kayak travel+read without cart/order/checkout/booked claims; HandOffActions
   maps display names → packages; proof log
   `shopping=target+walmart+nike+sephora+wayfair`, travel `+kayak`; flow ready
   log `shopping_adapters=5`; serve `adapter_count=51`.
3. **Algorithm:** Tests first (count 51, routes, coaching, HandOffActions, ready
   log) → RED → add Specs + coaching + packages + logs → GREEN → restart serve.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Sephora** | Wave-3 shopping; no UCP cart API. Browse/open only. |
| **Wayfair** | Same — prepare-and-open read; never claim cart/order/checkout. |
| **Kayak** | Travel search/prepare-and-open; Wave 3 book demoted → read; never claim booked. |

Do **not** use Hotels.com (`com.hotels.android` Play HTTP 404). Do **not** add
Priceline (`com.priceline.android.hybrid` Play HTTP 404) in this pack.  
Outcomes use `handoff.DraftOutcome` (never claims cart built / ordered /
checkout completed / booked).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `sephora` | Sephora | `com.sephora` | `shopping` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `wayfair` | Wayfair | `com.wayfair.wayfair` | `shopping` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `kayak` | Kayak | `com.kayak.android` | `travel` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `sephora_prepare_open_smoke`, `wayfair_prepare_open_smoke`,
`kayak_search_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **51** entries (indices 48–49 shopping after nike at 47;
  kayak at 50).
- Stage2 `ClassMap` shopping → five Specs; travel → includes kayak (dynamic).
- Stage1 coaching lines for Sephora/Wayfair (shopping/read) and Kayak
  (travel/read); never claim cart/order/checkout/booked.
- Android `HandOffActions` maps `sephora`/`Sephora`, `wayfair`/`Wayfair`,
  `kayak`/`Kayak`.
- `deeplink_proof.go` logs `shopping=target+walmart+nike+sephora+wayfair` and
  travel `+kayak`.
- Runtime ready log `shopping_adapters=5`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change. Pixel unpaired — no device smoke.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 48, want 51`
- `unknown adapter: sephora|wayfair|kayak`
- panic on `Wave1Specs()[48]`
- flow: I don't have the app you named connected
- ready log `shopping_adapters=3` (want 5)
- stage1 instructions missing `sephora` / `wayfair` / `kayak`
- HandOffActions missing Sephora/Wayfair/Kayak package asserts

**Green:**

```bash
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# → 5 packages ok; 194 PASS (go test -json Action=pass with Test set)

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# → BUILD SUCCESSFUL
```

**Serve:** rebuild `/tmp/codex-launcher-deeplink`; `serve-deeplink-proof` log shows
`adapter_count=51`, `shopping_adapters=5`,
`shopping=target+walmart+nike+sephora+wayfair`, travel `+kayak`.

## Sibling search

Searched `sephora`, `wayfair`, `kayak`, `com.sephora`, `com.wayfair.wayfair`,
`com.kayak.android`, `Hotels.com`, `priceline`, `shopping_adapters` under
companion + android — only this pack’s Specs / coaching / HandOffActions /
proof / ready logs. No Hotels.com or Priceline Specs added (explicit non-goal).

| Kind | Found |
|---|---|
| Wave1Specs shopping+kayak | Added three Specs only |
| HandOffActions | Added three package maps |
| stage1 coaching | Sephora/Wayfair/Kayak lines |
| flow ready log | shopping_adapters=5 |

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was
**not** run this pass.

## How to reuse

Re-run the verify commands above; confirm `len(Wave1Specs())==51` and serve
ready log `adapter_count=51` + `shopping=target+walmart+nike+sephora+wayfair`.
