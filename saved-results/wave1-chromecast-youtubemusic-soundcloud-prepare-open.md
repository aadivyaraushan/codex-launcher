# Wave 1 Chromecast + YouTube Music + SoundCloud prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Overnight Wave-1 prepare-and-open pack adding Chromecast, YouTube
Music, and SoundCloud. `Wave1Specs` count **61** (was 58). No Pandora / Asana /
Trello / Amazon / Strava / Telegram.  
**Callers:** `adapters/deeplink.Wave1Specs`, `runtime/deeplink`,
`HandOffActions`, stage1 OpenAI coaching, `serve-deeplink-proof`.  
**User ask:** Wave1Specs 58 → 61; Play HTTP 200; media
`+chromecast+youtubemusic+soundcloud`; media_adapters=10 (was 7); ProvesCeiling
in want table; evidence + overnight heartbeat ~30; no commit; restart serve.

## Inputs → Outputs → Algorithm

1. **Inputs:** Spec rows for `chromecast` / `youtubemusic` / `soundcloud`
   (packages `com.google.android.apps.chromecast.app`,
   `com.google.android.apps.youtube.music`, `com.soundcloud.android`); Play HTTP
   200 (2026-08-02); existing media prepare-and-open patterns (YouTube /
   Spotify peers).
2. **Outputs:** `Wave1Specs`=61; stage1 coaches media/play for Chromecast and
   media play|read for YouTube Music/SoundCloud without cast/played/playlist/
   library claims; HandOffActions maps display names → packages; proof logs
   `media=…+chromecast+youtubemusic+soundcloud`; ready log `media_adapters=10`;
   serve `adapter_count=61`.
3. **Algorithm:** Tests first (count 61, routes, coaching, bans, HandOffActions,
   ready log) → RED → Specs + coaching + packages + proof logs → GREEN →
   restart serve → evidence + overnight status.

## Why hands_off / verb choice

| App | Why hand-off (not completes) |
|---|---|
| **Chromecast** | Media **play** (cast/open intent). Never claim cast started / playing / connected. |
| **YouTube Music** | Media **play|read** like YouTube peers. Never claim played / added to playlist / library changed. |
| **SoundCloud** | Media **play|read** like YouTube peers. Never claim played / added to playlist / library changed. |

Outcomes use `handoff.DraftOutcome` (never claims completion).

## Chosen id / package / class / verbs

| ID | App name | Android package | AppClass | Verbs | Play Store evidence |
|---|---|---|---|---|---|
| `chromecast` | Chromecast | `com.google.android.apps.chromecast.app` | `media` | `play` | Play HTTP **200** (verified 2026-08-02) |
| `youtubemusic` | YouTube Music | `com.google.android.apps.youtube.music` | `media` | `play`, `read` | Play HTTP **200** (verified 2026-08-02) |
| `soundcloud` | SoundCloud | `com.soundcloud.android` | `media` | `play`, `read` | Play HTTP **200** (verified 2026-08-02) |

Ceiling `hands_off`, consent A, auth none, RT-4 floor.  
`ProvesCeiling`: `chromecast_prepare_open_smoke`,
`youtubemusic_prepare_open_smoke`, `soundcloud_prepare_open_smoke`.

## Wiring

- `Wave1Specs()` now has **61** entries (indices 58–60 after shazam at 57).
- Stage2 `ClassMap` media → 10 Specs (dynamic from Specs).
- Stage1 coaching lines for Chromecast / YouTube Music / SoundCloud.
- Android `HandOffActions` maps `chromecast` / `youtube music` / `youtubemusic` /
  `soundcloud`.
- `deeplink_proof.go` media `+chromecast+youtubemusic+soundcloud`.
- Shared execute ban list also includes `cast started`, `connected`,
  `library changed`.

## Tests (red → green this session)

**One iteration cost:** ~2–3s Go focused packages + ~7s HandOffActions; shrunk by
running only packages under change (not full suite / device). Pixel on Pair —
device smoke skipped.

**Red (before Spec / coaching / HandOffActions):**

- `Wave1Specs count = 58, want 61`
- `unknown adapter: chromecast` / `youtubemusic` / `soundcloud`
- panic on `Wave1Specs()[58]` (index out of range / length 58)
- flow: I don't have the app you named connected
- ready log `media_adapters=7` (want 10)
- stage1 instructions missing `chromecast` / `youtubemusic`

**Green:**

- Focused Go verify (`-count=1`): 5 packages `ok`
  (`cmd/codex-launcher`, `adapters/deeplink`, `runtime/deeplink`,
  `routing/stage1/openai`, `runtime`). Verbose PASS-line count this session:
  **115** (`^--- PASS:`); FAIL lines **0**.
- HandOffActionsTest: `tests=3 failures=0` (BUILD SUCCESSFUL).
- Serve restart: `adapter_count=61`,
  `media=…+shazam+chromecast+youtubemusic+soundcloud`, `media_adapters=10`.

### Commands to reproduce

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.chromecast.app"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.youtube.music"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.soundcloud.android"

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
# expect adapter_count=61 media …+chromecast+youtubemusic+soundcloud media_adapters=10
```

## Sibling search (same-bug check)

Searched: `Wave1Specs`, `want 58`, `media_adapters=7`, `HandOffActions`,
`…+shazam`, `chromecast`/`youtubemusic`/`soundcloud`, Pandora/Asana/Trello.

| Candidate | Decision |
|---|---|
| Wave1Specs count / want 58 | Updated to 61 |
| media_adapters=7 | Updated to 10 |
| HandOffActions | Added chromecast / youtube music / youtubemusic / soundcloud |
| proof media string | Appended +chromecast+youtubemusic+soundcloud |
| Pandora / Asana / Trello / Amazon / Strava / Telegram | Not added (out of pack) |
| Historical saved-results with Wave1Specs=58 | Older docs; live count is 61 |

## How to reuse

Re-run the verify commands above; confirm `len(Wave1Specs())==61` and serve
ready log `adapter_count=61` + media proof string includes the new apps.

## Post-judge follow-up (2026-08-02)

Judge noted serve was log-only at review time. Restarted live `/tmp/codex-launcher-deeplink serve-deeplink-proof` — confirmed `adapter_count=61` `media_adapters=10` in `/tmp/wave1-pack61-serve-live.log`.
