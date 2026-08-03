# Wave 1 Duolingo + Fitbit + Shazam prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Duolingo, Fitbit, and
Shazam. `Wave1Specs` count **58** (was 55). No Strava / Amazon / Chromecast.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 55 → 58; Play HTTP 200; services `+duolingo+fitbit`,
media `+shazam`; services_adapters=4; media_adapters=7; ProvesCeiling in want
table; evidence + overnight heartbeat ~29; no commit; restart serve.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `duolingo` / `fitbit` / `shazam` (packages
   `com.duolingo`, `com.fitbit.FitbitMobile`, `com.shazam.android`); Play HTTP
   200 (2026-08-02); existing services/media prepare-and-open patterns.
2. **Outputs:** `Wave1Specs`=58; stage1 coaches services/read for Duolingo+Fitbit
   and media/read for Shazam without lesson/workout/identified claims;
   HandOffActions maps display names → packages; proof logs
   `services=…+duolingo+fitbit`, `media=…+shazam`; ready log
   `services_adapters=4`, `media_adapters=7`; serve `adapter_count=58`.
3. **Algorithm:** Tests first (count 58, routes, coaching, bans, HandOffActions,
   ready log) → RED → Specs + coaching + packages + proof logs → GREEN →
   restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Duolingo** | Services **read** browse/open only. Never claim lesson completed. |
| **Fitbit** | Services **read** browse/open only. Never claim workout logged / synced / saved. |
| **Shazam** | Media **read** (identify/search intent). Never claim identified / played / saved. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `duolingo` | Duolingo | `com.duolingo` | `services` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `fitbit` | Fitbit | `com.fitbit.FitbitMobile` | `services` | `read` | Play HTTP **200** (verified 2026-08-02) |
| `shazam` | Shazam | `com.shazam.android` | `media` | `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `duolingo_prepare_open_smoke`, `fitbit_prepare_open_smoke`,
`shazam_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **58** entries (indices 55–57 after pinterest at 54).
- Stage2 `ClassMap` services → 4 Specs; media → 7 Specs (dynamic from Specs).
- Stage1 coaching lines for Duolingo/Fitbit/Shazam.
- Android `HandOffActions` maps `duolingo`/`fitbit`/`shazam`.
- `deeplink_proof.go` services `+duolingo+fitbit`, media `+shazam`.
- Shared execute ban list also includes `lesson completed`, `workout logged`,
  `synced`, `identified`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 55, want 58`
- `unknown adapter: duolingo` / `fitbit` / `shazam`
- panic on `Wave1Specs()[55]` (index out of range / length 55)
- flow: I don't have the app you named connected
- ready log `services_adapters=2` (want 4), `media_adapters=6` (want 7)
- stage1 instructions missing `duolingo` / `shazam`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **111** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: `tests=3 failures=0` (BUILD SUCCESSFUL).
- Serve restart: `adapter_count=58`,
  `services=taskrabbit+thumbtack+duolingo+fitbit`,
  `media=…+youtube+shazam`, `services_adapters=4`, `media_adapters=7`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n"   "https://play.google.com/store/apps/details?id=com.duolingo"
curl -s -o /dev/null -w "%{http_code}\n"   "https://play.google.com/store/apps/details?id=com.fitbit.FitbitMobile"
curl -s -o /dev/null -w "%{http_code}\n"   "https://play.google.com/store/apps/details?id=com.shazam.android"

go test ./companion/cmd/codex-launcher/ \
  ./companion/internal/capability/adapters/deeplink/ \
  ./companion/internal/capability/runtime/deeplink/ \
  ./companion/internal/capability/routing/stage1/openai/ \
  ./companion/internal/capability/runtime/ -count=1

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks

pkill -f 'codex-launcher-deeplink serve-deeplink' 2>/dev/null || true
pkill -f '/tmp/codex-launcher-deeplink' 2>/dev/null || true
go build -o /tmp/codex-launcher-deeplink ./companion/cmd/codex-launcher
set -a; source "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.env"; set +a
/tmp/codex-launcher-deeplink serve-deeplink-proof
# expect adapter_count=58 services …+duolingo+fitbit media …+shazam
```

## Sibling search (same-bug check)

Searched: `Wave1Specs`, `want 55`, `services_adapters=2`, `media_adapters=6`,
`HandOffActions`, `taskrabbit+thumbtack`, `netflix+youtube`,
`duolingo`/`fitbit`/`shazam`.

| Candidate | Decision |
|---|---|
| Wave1Specs count / want 55 | Updated to 58 |
| services_adapters=2 | Updated to 4 |
| media_adapters=6 | Updated to 7 |
| HandOffActions | Added duolingo/fitbit/shazam |
| proof services/media strings | Appended +duolingo+fitbit / +shazam |
| Strava / Amazon / Chromecast | Not added (plan/enjoined/out of pack) |
| Historical saved-results with Wave1Specs=55 | Older docs; live count is 58 |

## How to reuse

Re-run the verify commands above; confirm `len(Wave1Specs())==58` and serve
ready log `adapter_count=58` + services/media proof strings include the new apps.
