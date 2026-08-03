# Wave 1 rides/food hands_off pack (Uber / Uber Eats / Resy / DoorDash)

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open adapters for plan rows that
need no partner credentials: Uber estimates, Uber Eats, Resy, DoorDash
connector.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave-1 hands_off prepare-and-open for Uber / Uber Eats / Resy /
DoorDash; prefer extending deeplink Wave1Specs; confirm Play packages; TDD;
evidence here; no commit; no partner OAuth.

## Chosen ids / packages / classes / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `uber` | Uber | `com.ubercab` | `rides` | `read` | [Play](https://play.google.com/store/apps/details?id=com.ubercab) — “Uber - Request a ride” |
| `ubereats` | Uber Eats | `com.ubercab.eats` | `food` | `read` | [Play](https://play.google.com/store/apps/details?id=com.ubercab.eats) |
| `resy` | Resy | `com.resy.android.prod` | `food` | `read` | [Play](https://play.google.com/store/apps/details?id=com.resy.android.prod) (`com.resy.android` 404) |
| `doordash` | DoorDash | `com.dd.doordash` | `food` | `read`, `order` | [Play](https://play.google.com/store/apps/details?id=com.dd.doordash) — consumer app (not `com.doordash.driverapp`) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor under the plan’s RT-1
“manifest / hand-off” rows. Outcomes use `handoff.DraftOutcome` (never claims
booked / reserved / ordered / ride requested).

Plan alignment (`planning/consumer-app-implementation-plan.md`):

- Uber (estimates): RT-1 read hands_off → class `rides` (stage1 already lists
  `rides`; booking stays BD-gated / out of scope).
- Uber Eats / Resy: RT-1 read hands_off → class `food`.
- DoorDash connector: RT-1 read, order hands_off → class `food`; checkout
  status still unconfirmed by vendor.

## Wiring

- `Wave1Specs()` now has **14** entries (was 10).
- Stage2 `ClassMap` built from Spec.AppClass:
  - `rides` → `uber`
  - `food` → starbucks, chipotle, **ubereats, resy, doordash**
- Stage1 coaching adds Uber / Uber Eats / Resy / DoorDash prepare-and-open
  lines (no OAuth / API key coaching).
- Android `HandOffActions` maps display names → packages above.

## Tests (red → green this session)

**Red (before specs):**

- `Wave1Specs count = 10, want 14`
- `unknown adapter: uber|ubereats|resy|doordash`
- flow: `I don't know which app to use for "rides"` / named app not connected
- panic on `Wave1Specs()[10]` for Uber empty-draft case

**Green:**

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/cmd/codex-launcher/ -run 'DeepLink|Wave1|Stage1InstructionsCoachUber'
```

- deeplink adapter + runtime + stage1 openai filtered: **39 passed**
- full `./companion/internal/capability/routing/stage1/openai/`: **12 passed**
- Android `HandOffActionsTest`: **tests="3" failures="0"**
  (`TEST-app.codexlauncher.capability.handoff.HandOffActionsTest.xml`)

## Pixel notes (optional)

Device: Pixel 9 `4B230DLAQ001Z5`

| App | Installed? | Launch resolve |
|---|---|---|
| Uber `com.ubercab` | **yes** (`pm path` OK) | `com.ubercab/.presidio.app.core.root.RootActivity` |
| DoorDash `com.dd.doordash` | **yes** | `com.dd.doordash/com.doordash.consumer.ui.login.LauncherActivity` |
| Uber Eats `com.ubercab.eats` | **no** | `No activity found` — same shape as Starbucks disclosure |
| Resy `com.resy.android.prod` | **no** | `No activity found` — companion path unit-tested; package launch needs install |

Full Auto→preview→Open sheet smoke on device was **not** run this pass (unit/
flow coverage only). Companion path for all four is covered by Go tests;
Uber/DoorDash are launchable if a live `serve-deeplink-proof` smoke is run next.

## Sibling sites checked

Searched: `Wave1Specs`, `want 10`, `HandOffActions`, `com.ubercab`,
`ubereats`, `doordash`, `resy`, `rides_adapters`, `serve-deeplink-proof`.

- Extended the same deeplink adapter path (not a parallel OAuth runtime).
- No partner OAuth / API keys added.
- Historical saved-results mentioning adapter_count 9/10 are older docs;
  live count is 14.
- DoorDash driver package `com.doordash.driverapp` deliberately **not** used.

## Blockers / non-blockers

- **No partner credentials required** for this pack (by design).
- Uber booking / Riders API still BD-gated — out of scope.
- DoorDash checkout completion remains **unconfirmed**; hand-off only.
- Pixel: install Uber Eats + Resy before claiming Open-button focus leaves
  Operator for those two (Uber + DoorDash already installed).

## Judge follow-up (2026-08-02)

Callers: none in code (human/overnight artifact). Peer: `wave1-rides-food-handoff-pack-judge.md`. No schema.
User: follow-up on rides/food judge — fix concrete gaps.

[Judge rides/food pack](cb494de3-2de7-4ec4-911e-f6ffcf2fd056) **Pass-with-warnings** → added Resolve rejects for Uber Eats `order` and Resy `book` (sibling of Uber `book` reject). Tests green.

Still open: Auto→Open Pixel smoke (prompt/pairing flaky); Eats/Resy apps not installed.

## How to re-run

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ -count=1

cd android && ./gradlew :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'

# optional live:
# go run ./companion/cmd/codex-launcher serve-deeplink-proof
# adb shell pm path com.ubercab; adb shell pm path com.ubercab.eats
# adb shell pm path com.resy.android.prod; adb shell pm path com.dd.doordash
```
