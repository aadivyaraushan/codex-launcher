# Wave 1 Airbnb / OpenTable / Grubhub prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open hand-offs for Airbnb
(travel/read), OpenTable (food/read), and Grubhub (food read|order). Append
after YouTube; `Wave1Specs` count **42** (was 39).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Airbnb/OpenTable/Grubhub prepare-and-open (Wave1Specs 39 → 42);
TDD; Play HTTP already verified 200; never claim booked/ordered; evidence
here; no commit.

## Why hands_off / verb demotion

| App | Why hand-off (not completes) |
|---|---|
| **Airbnb** | Wave 3 lists `book`; Wave 1 overnight rule demotes → `read` (search/prepare-and-open, same as Booking.com). Never claim booked. |
| **OpenTable** | Wave 3 lists `book`; demoted → `read` (same as Resy). Never claim reservation booked. |
| **Grubhub** | Wave 3 lists `order`; Wave 1 keeps `read` + `order` as cart/browse intent only (same as DoorDash). Never claim checkout completed. |

Outcomes use `handoff.DraftOutcome` (never claims booked / ordered / checkout completed).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `airbnb` | Airbnb | `com.airbnb.android` | `travel` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `opentable` | OpenTable | `com.opentable` | `food` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `grubhub` | Grubhub | `com.grubhub.android` | `food` | `read`, `order` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `airbnb_search_prepare_open_smoke`, `opentable_prepare_open_smoke`, `grubhub_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **42** entries (indices 39–41 after youtube at 38).
- Stage2 `ClassMap` travel/food grow via Wave1Specs registration.
- Stage1 coaching: Airbnb travel/read never claim booked; OpenTable food/read never claim reservation booked; Grubhub food read|order never claim checkout completed.
- Android `HandOffActions` maps `airbnb`/`Airbnb`, `opentable`/`OpenTable`, `grubhub`/`Grubhub`.
- `deeplink_proof.go` logs `travel=…+airbnb` and `food_extra=…+opentable+grubhub`.

## Tests (red → green this session)

**One iteration cost:** ~3s Go focused packages + ~8s HandOffActions; shrunk by
running only the packages under change (not full suite / device).

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 39, want 42`
- `unknown adapter: airbnb` / `opentable` / `grubhub`
- panic on `Wave1Specs()[39]`
- stage1 instructions missing `airbnb` / `opentable` / `grubhub`
- HandOffActions missing Airbnb/OpenTable/Grubhub package asserts
- flow Prepare: app not connected for the three named apps

**Green:**

```bash
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# → 5 packages ok; 91 PASS lines

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# → BUILD SUCCESSFUL
```

**Serve:** rebuild `/tmp/codex-launcher-deeplink`; `serve-deeplink-proof` log shows
`adapter_count=42` and travel/food_extra include the three apps.

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was not
exercised this session.

## Overnight status bullet

- **Airbnb + OpenTable + Grubhub prepare-and-open (2026-08-02):** travel/read + food/read + food read|order; Play packages HTTP 200; `Wave1Specs`=42; book demoted to read for Airbnb/OpenTable; never claim booked/checkout completed; stage1 + HandOffActions + proof travel/food_extra logs; go 91 PASS + HandOffActions unit green. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-airbnb-opentable-grubhub-prepare-open.md`.
