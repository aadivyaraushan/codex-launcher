# Wave 1 Threads / TikTok / Expedia prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open hand-offs for Threads
(messaging/compose), TikTok (messaging/compose), and Expedia (travel/read).
Append after Grubhub; `Wave1Specs` count **45** (was 42).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Threads/TikTok/Expedia prepare-and-open (Wave1Specs 42 → 45);
TDD; Play HTTP already verified 200; TikTok uses messaging+compose (Facebook
peer, not media); never claim posted/replied/booked; evidence here; no commit.

## Why hands_off / verb demotion

| App | Why hand-off (not completes) |
|---|---|
| **Threads** | Wave 2 lists post/reply; overnight Wave 1 is compose hand-off only. Never claim posted/replied. |
| **TikTok** | Wave 2 lists post; overnight uses compose (draft/open). AppClass **messaging** (match Facebook; stage1 has no social class; media ClassMap is play/read/write). Never claim posted. Package `com.zhiliaoapp.musically` (not `com.ss.android.ugc.trill` — 404). |
| **Expedia** | Wave 3 lists book; demoted → `read` (same as Booking/Airbnb). Never claim booked. |

Outcomes use `handoff.DraftOutcome` (never claims posted / replied / booked).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `threads` | Threads | `com.instagram.barcelona` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02) |
| `tiktok` | TikTok | `com.zhiliaoapp.musically` | `messaging` | `compose` | Play HTTP **200** (verified 2026-08-02); `com.ss.android.ugc.trill` **404** — unused |
| `expedia` | Expedia | `com.expedia.bookings` | `travel` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `threads_prepare_open_smoke`, `tiktok_prepare_open_smoke`, `expedia_search_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **45** entries (indices 42–44 after grubhub at 41).
- Stage2 `ClassMap` messaging/travel grow via Wave1Specs registration.
- Stage1 coaching: Threads/TikTok messaging/compose never claim posted(/replied); Expedia travel/read never claim booked.
- Android `HandOffActions` maps `threads`/`Threads`, `tiktok`/`TikTok`, `expedia`/`Expedia`.
- `deeplink_proof.go` logs `messaging=…+threads+tiktok` and `travel=…+expedia`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only the packages under change (not full suite / device).

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 42, want 45`
- `unknown adapter: threads` / `tiktok` / `expedia`
- panic on `Wave1Specs()[42]`
- stage1 instructions missing `threads` / `tiktok` / `expedia`
- HandOffActions missing Threads/TikTok/Expedia package asserts
- flow Prepare: app not connected for the three named apps

**Green:**

```bash
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# → 5 packages ok; 96 PASS lines (-v)

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# → BUILD SUCCESSFUL
```

**Serve:** rebuild `/tmp/codex-launcher-deeplink`; `serve-deeplink-proof` log shows
`adapter_count=45`, `messaging_adapters=9`, `travel_adapters=13`, and
messaging/travel strings include threads+tiktok / expedia.

## Sibling search

Searched `threads`, `tiktok`, `expedia`, `barcelona`, `musically`,
`expedia.bookings` under companion + android — only this pack’s Specs /
coaching / HandOffActions / proof logs (plus unrelated registry kill-list
fixture string `tiktok` and promptqueue local var `threads`).

## Pixel notes

**Pixel unpaired** (Pair screen) — Auto→Open device smoke blocked until re-pair.
Companion path covered by Go unit/flow tests; package launch on device was not
exercised this session.

## Overnight status bullet

- **Threads + TikTok + Expedia prepare-and-open (2026-08-02):** messaging/compose + messaging/compose + travel/read; Play packages HTTP 200 (`com.instagram.barcelona`, `com.zhiliaoapp.musically`, `com.expedia.bookings`; trill 404 unused); `Wave1Specs`=45; TikTok messaging to match Facebook; never claim posted/replied/booked; stage1 + HandOffActions + proof messaging/travel logs; go 96 PASS + HandOffActions unit green. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-threads-tiktok-expedia-prepare-open.md`.
