# Wave 1 airlines + Citymapper prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open hand-offs for United,
Delta, Southwest, American Airlines, and Citymapper (travel/read only). Append
after Netflix/Facebook; `Wave1Specs` count **38** (was 33).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Implement Wave 1 prepare-and-open for airlines + Citymapper;
TDD; verify Play packages; evidence here; no commit; no secrets.
**Evidence schema:** markdown status artifact (title, date, purpose, callers,
package table, red→green commands, Pixel notes, sibling search). No secrets.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **United / Delta / Southwest / American Airlines** | No partner booking/check-in API in Wave 1. `read` = flight status or manage-booking browse. Never claim booked, checked-in, or boarded. |
| **Citymapper** | Transit directions browse only. Never claim booked / checked-in / boarded. |

Outcomes use `handoff.DraftOutcome` (never claims booked / checked-in / boarded).

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `united` | United | `com.united.mobile.android` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.united.mobile.android) — HTTP **200** |
| `delta` | Delta | `com.delta.mobile.android` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.delta.mobile.android) — HTTP **200** |
| `southwest` | Southwest | `com.southwestairlines.mobile` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.southwestairlines.mobile) — HTTP **200** |
| `american` | American Airlines | `com.aa.android` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.aa.android) — HTTP **200** |
| `citymapper` | Citymapper | `com.citymapper.app.release` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.citymapper.app.release) — HTTP **200** |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `united_prepare_open_smoke`, `delta_prepare_open_smoke`,
`southwest_prepare_open_smoke`, `american_prepare_open_smoke`,
`citymapper_prepare_open_smoke`.

**Southwest package correction:** Suggested `com.southwestair.mobile` returned
HTTP **404**. Variants tried: `com.southwest.mobile` 404, `com.southwestair.android`
404, `com.southwestairlines.mobile` **200** — used that.

## Wiring

- `Wave1Specs()` now has **38** entries (was 33 after Netflix/Facebook).
- Stage2 `ClassMap` `travel` → … + united/delta/southwest/american/citymapper
  (auto from Spec AppClass; no new class key).
- Stage1 coaching: travel/read for all five; refuse booked / checked-in / boarded.
- Android `HandOffActions` maps display names including lowercase
  `american airlines` → `com.aa.android`.
- `deeplink_proof.go` travel log includes
  `…+united+delta+southwest+american+citymapper`.

## Tests (red → green this session)

**One iteration cost:** ~3s Go focused packages + ~1s Android unit; shrunk by
running only the three Go packages + HandOffActionsTest (not full suite /
device).

**Red (before specs / coaching / HandOffActions):**

- `Wave1Specs count = 33, want 38`
- `unknown adapter: united|delta|southwest|american|citymapper`
- flow: I don't have the app you named connected for this (five subtests)
- panic on `Wave1Specs()[33]` for airline empty-draft case
- stage1 instructions missing `united` (and peers)
- HandOffActionsTest AssertionError at United package assert (line 73)

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1
# → 120 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest
# → BUILD SUCCESSFUL
```

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was
**not** run this pass. No browser automation (per plan). No secrets added.

## Sibling sites checked

Searched: `Wave1Specs`, `want 33`, `HandOffActions`, `united`, `delta`,
`southwest`, `american`, `citymapper`, `com.southwestair.mobile`,
`com.southwestairlines.mobile`.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- Southwest id corrected from 404 suggestion to `com.southwestairlines.mobile`.
- `flow.go` class ready log unchanged (no new AppClass — still `travel`).
- Historical saved-results mentioning `Wave1Specs`=33 are older docs; live
  count is **38**.

## How to re-run

```bash
for pkg in com.united.mobile.android com.delta.mobile.android \
  com.southwestairlines.mobile com.aa.android com.citymapper.app.release; do
  curl -sI -o /dev/null -w "$pkg %{http_code}\n" \
    "https://play.google.com/store/apps/details?id=$pkg"
done

go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'

# optional live (needs Pixel paired + apps installed):
# companion serve-deeplink-proof
```
