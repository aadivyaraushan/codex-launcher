# Teams personal + Booking.com prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Ship Wave 1 plan rows that cannot complete via API: personal Teams compose hand-off, Booking.com search-only hand-off.

**Callers:** Wave 1 continuous loop; `deeplink.Wave1Specs`; `HandOffActions`; stage1 coaching.  
**API:** none (class-H open-package only).  
**User:** Continue consumer-app-implementation-plan (heartbeat).

## Why hand-off

| App | Verified limit | Product route |
|---|---|---|
| Teams (personal) | Plan: Graph chat send does not support personal Microsoft accounts | RT-4 compose → open `com.microsoft.teams` |
| Booking.com | Plan: vendor-confirmed search-only; cannot complete/cancel booking in Operator | RT-4 read → open `com.booking` |

Play Store packages: `id=com.microsoft.teams`, `id=com.booking`.

## Inputs → Outputs → Algorithm

1. **Inputs:** Utterance for Teams draft or Booking.com hotel search.
2. **Outputs:** `hands_off` preview + package open; never claims sent/booked.
3. **Algorithm:** Specs `teams` / `booking` in Wave1Specs (count **17**); stage1 coaches `messaging/teams/compose` and `travel/booking/read`.

## Verify (this session)

```text
go test ./companion/internal/capability/adapters/deeplink/ \
  ./companion/internal/capability/runtime/deeplink/ \
  ./companion/internal/capability/routing/stage1/openai/
→ 58 passed (later re-check 57/58 depending on filter)

./gradlew :app:testDebugUnitTest --tests …HandOffActionsTest
→ BUILD SUCCESSFUL in 7s
```

Pixel Auto→Open blocked: device unpaired.

## How to reuse

Do not add Graph chat send for personal Teams without a documented personal-account endpoint. Work/school Teams remains a separate OAuth/Graph adapter later.
