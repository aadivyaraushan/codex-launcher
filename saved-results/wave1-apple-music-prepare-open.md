# Wave 1 Apple Music prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A Apple Music prepare-and-open (media peer
of Spotify/Audible; open only; no MusicKit / Apple Music API / tokens), wired
through the shared deeplink adapter pack and `serve-deeplink-proof`.

**Callers:** overnight Group A batch; companion `adapters/deeplink` +
`runtime/deeplink` + `serve-deeplink-proof`; Android `HandOffActions`.  
**User ask:** Wave-1 Apple Music as prepare-and-open hand-off (media class),
peer of Spotify/Audible in the deeplink pack. Confirm Android package via
Play/repo evidence before hardcoding. TDD; no commit; no Apple Music API/OAuth.

## Result

Extended `adapters/deeplink.Wave1Specs()` (least code — same
`handoff.DraftOutcome` path as Spotify/Audible/money/food/messaging):

| ID | App | Package | AppClass | Verbs |
|---|---|---|---|---|
| applemusic | Apple Music | `com.apple.android.music` | media | play, write |

**Package evidence (2026-08-02):** Google Play Store listing
`https://play.google.com/store/apps/details?id=com.apple.android.music`
returned HTTP 200 with `id=com.apple.android.music`. Matches FlyTrap /
Uptodown catalog package names. Verbs follow Spotify media peer
(`play|write`), not the plan’s older MusicKit RT-2 `read|play|write`
completes row — release policy is prepare-and-open only.

Ceiling `hands_off`, consent A, auth none.

`runtime/deeplink` ClassMap still built from each Spec's AppClass:

- `money` → venmo, cashapp, zelle
- `food` → starbucks, chipotle
- `media` → spotify, audible, **applemusic**
- `messaging` → messages, discord

Stage1 OpenAI instructions coach Apple Music as `app_class media`,
`app_named applemusic`, verb `play` (listen/search) or `write`
(playlist/library edit intent), open only — no playback/API/token claims.

Android `HandOffActions` maps display name `Apple Music` →
`com.apple.android.music`.

```bash
go run ./companion/cmd/codex-launcher serve-deeplink-proof
```

Needs `OPENAI_API_KEY` only. No Apple Music OAuth / MusicKit / API tokens.
`adapter_count` is now **10**; media log tag
`spotify+audible+applemusic`.

## How to re-run

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/handoff/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/cmd/codex-launcher/

# Android unit
cd android && ./gradlew :app:testDebugUnitTest \
  --tests 'app.codexlauncher.capability.handoff.HandOffActionsTest'
```

Green this session (2026-08-02):

- Red first: Wave1Specs count 9≠10, unknown adapter `applemusic`, stage1
  missing Apple Music coaching (8 focused failures)
- `go test` on the five packages above → **54 passed**
- Focused red→green covered Apple Music Wave1Specs, media ClassMap routing
  (spotify+audible+applemusic), play+write hands_off outcomes, empty/send
  reject, stage1 coaching, CLI serve media tag
- Android `HandOffActionsTest` → BUILD SUCCESSFUL (Apple Music package assert)

## Sibling sites checked

Searched `applemusic`, `Apple Music`, `com.apple.android.music`,
`Wave1Specs`, `HandOffActions`, `serve-deeplink-proof`, `app_class media`,
`spotify+audible`, `want 9`.

| Site | Decision |
|---|---|
| `adapters/deeplink` + `runtime/deeplink` | Extended here (Apple Music under media ClassMap). |
| `HandOffActions` | Added `apple music` → `com.apple.android.music`. |
| Stage1 openai instructions | Added Apple Music prepare-and-open coaching. |
| `serve-deeplink-proof` / `deeplink_proof.go` | Same entrypoint; adapter_count 10; media=spotify+audible+applemusic. |
| Spotify / Audible prepare-and-open | Sibling media adapters; left intact. |
| `planning/…` Apple Music MusicKit RT-2 row | Planning/research route (completes). Overnight release policy is prepare-and-open hand-off (this work). Left planning alone. |
| Notion OAuth | Explicitly out of scope this task — not touched. |

## Pixel smoke (2026-08-02) — companion path ready; app launch blocked

Device: Pixel 9 `4B230DLAQ001Z5`

```text
adb shell pm path com.apple.android.music   # exit 1, empty (not installed)
adb shell pm list packages | rg -i apple    # no com.apple.android.music
# Installed media peers for contrast: com.spotify.music, com.audible.application
```

Same honesty pattern as Starbucks in `wave1-deeplink-pack.md`: companion
hand-off wiring is green; **Open Apple Music cannot leave Operator** until
`com.apple.android.music` is installed on the device (or the owner installs
from Play). No full Auto→Open smoke run this session for that reason.

## Blockers

- Pixel does not have Apple Music installed → physical Open smoke deferred
  (not a code blocker).
- No Apple Music API/OAuth by design for this wave.
