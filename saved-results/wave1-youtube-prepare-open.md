# Wave 1 YouTube prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A prepare-and-open hand-off for YouTube
(media play|read). Append after airlines/Citymapper; `Wave1Specs` count **39**
(was 38).  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** YouTube prepare-and-open (Wave1Specs 38 → 39); TDD; verify Play
HTTP 200; never claim played; evidence here; no commit.

## Why hands_off

| App | Why hand-off (not completes) |
|---|---|
| **YouTube** | No partner playback/search API in Wave 1. `play` = open/search title intent; `read` = browse/search. Never claim played. |

Outcomes use `handoff.DraftOutcome` (never claims played / watched).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `youtube` | YouTube | `com.google.android.youtube` | `media` | `play`, `read` | [Play](https://play.google.com/store/apps/details?id=com.google.android.youtube) — HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `youtube_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **39** entries (was 38 after airlines/Citymapper).
- Stage2 `ClassMap` `media` → … + youtube (via Wave1Specs registration).
- Stage1 coaching: YouTube media play|read; never claim played.
- Android `HandOffActions` maps `youtube` / `YouTube` → `com.google.android.youtube`.
- `deeplink_proof.go` logs `media=…+youtube`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~8s HandOffActions; shrunk by
running only the packages under change (not full suite / device).

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 38, want 39`
- `unknown adapter: youtube`
- panic on `Wave1Specs()[38]`
- stage1 instructions missing `youtube`
- HandOffActions missing YouTube package assert
- flow register count want 38

**Green:**

```bash
go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/proving/microsoft/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# → 161 passed

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# → BUILD SUCCESSFUL
```

## Pixel notes

**Pixel unpaired** assumed from overnight status — Auto→Open device smoke blocked
until re-pair. Companion path covered by Go unit/flow tests; package launch on
device was not exercised this session.

## Overnight status bullet

- **YouTube prepare-and-open (2026-08-02):** media play|read (`com.google.android.youtube` Play HTTP 200); `Wave1Specs`=39; never claim played; stage1 + HandOffActions + proof media log; go 161/161 verify + HandOffActions unit green. Pixel unpaired — Auto→Open blocked. Evidence: `wave1-youtube-prepare-open.md`.
