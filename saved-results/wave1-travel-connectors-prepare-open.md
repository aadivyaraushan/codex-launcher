# Wave 1 travel connectors prepare-and-open (Tripadvisor / Viator / StubHub / AllTrails)

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open adapters for Wave 1 travel
connector rows that ship as hands_off search hand-offs (append after Booking.com;
`Wave1Specs` count **21**).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave-1 travel connector prepare-and-open for Tripadvisor / Viator /
StubHub / AllTrails; verify Play packages; TDD; evidence here; no commit.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **Viator** | Partner API is partner-gated — not a user OAuth completes path for consumers. |
| **AllTrails** | No public consumer API established; plan “completes” row is provisional until a user-delegated route exists. |
| **Tripadvisor** | Wave 1 connector row is hands_off until smoke proves otherwise. |
| **StubHub** | Wave 1 connector row is hands_off until smoke proves otherwise. |

Outcomes use `handoff.DraftOutcome` (never claims booked / reserved / purchased).

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `tripadvisor` | Tripadvisor | `com.tripadvisor.tripadvisor` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.tripadvisor.tripadvisor) — HTTP 200 |
| `viator` | Viator | `com.viator.mobile.android` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.viator.mobile.android) — HTTP 200; **`com.viator.mobile.consumer` 404** (corrected) |
| `stubhub` | StubHub | `com.stubhub` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.stubhub) — HTTP 200 |
| `alltrails` | AllTrails | `com.alltrails.alltrails` | `travel` | `read` | [Play](https://play.google.com/store/apps/details?id=com.alltrails.alltrails) — HTTP 200 |

Ceiling `hands_off`, consent A, auth none, RT-4 floor under plan RT-1 hand-off rows.
`ProvesCeiling`: `tripadvisor_search_prepare_open_smoke`,
`viator_search_prepare_open_smoke`, `stubhub_search_prepare_open_smoke`,
`alltrails_search_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **21** entries (was 17 after Teams + Booking.com).
- Stage2 `ClassMap` `travel` → booking + tripadvisor + viator + stubhub + alltrails.
- Stage1 coaching adds one prepare-and-open line per app (`app_named` + read;
  never claim booked).
- Android `HandOffActions` maps lowercase display names → packages above.
- `deeplink_proof.go` travel log:
  `booking+tripadvisor+viator+stubhub+alltrails`.

## Tests (red → green this session)

**One iteration cost:** ~2s Go focused packages + ~3s Android unit; shrunk by
running only the three Go packages + HandOffActionsTest (not full suite / device).

**Red (before specs / coaching / HandOffActions):**

- `Wave1Specs count = 17, want 21`
- `unknown adapter: tripadvisor|viator|stubhub|alltrails`
- flow: named app not connected for travel connectors
- panic on `Wave1Specs()[17]` for tripadvisor empty-draft case
- stage1 instructions missing `tripadvisor` / `app_named tripadvisor` etc.

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1
# → 67 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest
# → BUILD SUCCESSFUL (tests="3" failures="0")
```

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path for all four is covered by Go unit/flow tests; package launch
on device was **not** run this pass.

## Sibling sites checked

Searched: `Wave1Specs`, `want 17`, `HandOffActions`, `com.viator.mobile.consumer`,
`tripadvisor`, `viator`, `stubhub`, `alltrails`, `travel`.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- Historical saved-results mentioning `Wave1Specs`=17 are older docs; live count is **21**.
- Wrong Viator package `com.viator.mobile.consumer` deliberately **not** used.

## How to re-run

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'

# optional live (needs Pixel paired + apps installed):
# companion serve-deeplink-proof
```
