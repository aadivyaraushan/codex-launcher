# Wave 1 Priceline / LinkedIn / eBay prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Wave-1 prepare-and-open hand-offs for Priceline
(travel read; book demoted), LinkedIn (messaging compose; Facebook/Threads peer),
and eBay (shopping read browse/open). Append after Kayak; `Wave1Specs` count
**54** (was 51).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 51 → 54; Priceline/LinkedIn/eBay; Play HTTP 200
verified 2026-08-02; do **not** add Reddit or Pinterest; evidence here; no
commit; restart serve; Pixel on Pair — skip device smoke.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `priceline` / `linkedin` / `ebay`; Play packages
   HTTP 200 (2026-08-02); existing Kayak travel + Facebook/Threads messaging +
   Sephora/Wayfair shopping patterns.
2. **Outputs:** `Wave1Specs`=54; stage1 coaches priceline travel+read, linkedin
   messaging+compose, ebay shopping+read without booked/posted/cart/bid/checkout
   claims; HandOffActions maps display names → packages; proof log travel
   `+priceline`, messaging `+linkedin`, shopping `+ebay`; flow ready log
   `shopping_adapters=6`, `travel_adapters=15`, `messaging_adapters=10`; serve
   `adapter_count=54`.
3. **Algorithm:** Tests first (count 54, routes, coaching, HandOffActions, ready
   log) → RED → add Specs + coaching + packages + logs → GREEN → restart serve.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Priceline** | Travel search/prepare-and-open; book demoted → read; never claim booked. |
| **LinkedIn** | Wave 2 personal post/comment → overnight compose only (same as Facebook/Threads). Never claim posted/commented. No OAuth/member API this pack. |
| **eBay** | Shopping browse/open read only; never claim cart built / bid placed / checkout completed. Wave 2 Browse is open; checkout is separate — no checkout verbs. |

Do **not** add Reddit (money gate / Wave 4) or Pinterest.  
Outcomes use `handoff.DraftOutcome` (never claims booked / posted / cart / bid / checkout).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `priceline` | Priceline | `com.priceline.android.negotiator` | `travel` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `linkedin` | LinkedIn | `com.linkedin.android` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02) |
| `ebay` | eBay | `com.ebay.mobile` | `shopping` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `priceline_search_prepare_open_smoke`,
`linkedin_prepare_open_smoke`, `ebay_browse_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **54** entries (priceline 51, linkedin 52, ebay 53 after
  kayak at 50).
- Stage2 `ClassMap` shopping → six Specs; travel → 15; messaging → 10 (dynamic).
- Stage1 coaching lines for Priceline (travel/read), LinkedIn (messaging/compose),
  eBay (shopping/read); never claim booked/posted/commented/cart/bid/checkout.
- Android `HandOffActions` maps `priceline`/`Priceline`, `linkedin`/`LinkedIn`,
  `ebay`/`eBay`.
- `deeplink_proof.go` logs travel `+priceline`, messaging `+linkedin`,
  shopping `+ebay`.
- Runtime ready log `shopping_adapters=6`, `travel_adapters=15`,
  `messaging_adapters=10`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change. Pixel on Pair — no device smoke.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 51, want 54`
- `unknown adapter: priceline|linkedin|ebay`
- panic on `Wave1Specs()[51]` (index out of range)
- flow: I don't have the app you named connected
- ready log `shopping_adapters=5` (want 6)
- stage1 instructions missing `priceline` / `linkedin` / `ebay`
- HandOffActions missing Priceline/LinkedIn/eBay package asserts

**Green:**

```bash
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# → 5 packages ok; 202 PASS

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# → BUILD SUCCESSFUL
```

**Serve:** rebuild `/tmp/codex-launcher-deeplink`; live `serve-deeplink-proof` log
(`/tmp/priceline-linkedin-ebay-serve.log`) shows `adapter_count=54`,
`shopping_adapters=6`, `travel_adapters=15`, `messaging_adapters=10`,
`shopping=target+walmart+nike+sephora+wayfair+ebay`, travel `+priceline`,
messaging `+linkedin`.

## Sibling search

Searched `priceline`, `linkedin`, `ebay`, `com.priceline.android.negotiator`,
`com.linkedin.android`, `com.ebay.mobile`, `reddit`, `pinterest`,
`shopping_adapters` under companion + android — only this pack’s Specs /
coaching / HandOffActions / proof / ready logs. No Reddit or Pinterest Specs
added (explicit non-goal). Prior pack’s unused hybrid package note remains
historical; this pack uses `com.priceline.android.negotiator` (Play 200).

| Kind | Found |
|---|---|
| Wave1Specs priceline/linkedin/ebay | Added three Specs only |
| HandOffActions | Added three package maps |
| stage1 coaching | Priceline/LinkedIn/eBay lines |
| flow ready log | shopping_adapters=6; travel=15; messaging=10 |

## Pixel notes

**Pixel on Pair** — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was
**not** run this pass.

## How to reuse

Re-run the verify commands above; confirm `len(Wave1Specs())==54` and serve
ready log `adapter_count=54` + shopping/travel/messaging proof strings.
