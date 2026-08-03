# Wave 1 Spotify prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A Spotify prepare-and-open (open only; no
playback API / no tokens), wired through the shared deeplink adapter pack and
`serve-deeplink-proof`.

**Callers:** overnight Group A batch; companion `adapters/deeplink` +
`runtime/deeplink` + `serve-deeplink-proof`; Android `HandOffActions`.  
**User ask:** Implement overnight Group A item: Spotify prepare-and-open
(policy: open only; no playback API / no tokens).

## Result

Extended `adapters/deeplink.Wave1Specs()` (least code — reuse
`handoff.DraftOutcome`, same hands_off / cannot-know / no-claim pattern):

| ID | App | Package | AppClass | Verbs |
|---|---|---|---|---|
| spotify | Spotify | `com.spotify.music` | media | play, write |

Ceiling `hands_off`, consent A, auth none. Specs now carry `Verbs` +
`AppClass` so money/food/media share one adapter type.

`runtime/deeplink` ClassMap is built from each Spec's AppClass:

- `money` → venmo, cashapp, zelle
- `food` → starbucks, chipotle
- `media` → spotify

Stage1 OpenAI instructions coach Spotify as `app_class media`,
`app_named spotify`, verb `play` (listen/search) or `write` (playlist/library
edit intent), open only — no playback control claims.

Android `HandOffActions` maps display name `Spotify` → `com.spotify.music`.

```bash
go run ./companion/cmd/codex-launcher serve-deeplink-proof
```

Needs `OPENAI_API_KEY` only. No Spotify OAuth / API tokens.

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

- `go test` on the five packages above → **37 passed**
- Focused red→green covered Spotify Wave1Specs, media ClassMap routing,
  play+write hands_off outcomes, stage1 coaching, CLI serve wiring
- Android `HandOffActionsTest` → BUILD SUCCESSFUL (Spotify package assert)

## Sibling sites checked

Searched `spotify`, `Spotify`, `audible`, `Audible`, `com.spotify`,
`Wave1Specs`, `HandOffActions`, `serve-deeplink-proof`, `app_class media`.

| Site | Decision |
|---|---|
| `adapters/deeplink` + `runtime/deeplink` | Extended here (Spotify + media ClassMap). |
| `HandOffActions` | Added `spotify` → `com.spotify.music`. |
| Stage1 openai instructions | Added Spotify prepare-and-open coaching. |
| `serve-deeplink-proof` / `deeplink_proof.go` | Same entrypoint; adapter_count 6. |
| Audible | Implemented in Group A #4 — see `saved-results/wave1-audible-prepare-open.md`. |
| `verification` / `registry` / `execution` tests using fake `spotify` RT2 Completes | Test fixtures for capacity/ceiling math — leave alone; not the release prepare-and-open adapter. |
| Vendor/connector notes (`wave1-vendor-route-audit`, owner decisions) | Still about Web API connector reachability. Release policy remains prepare-and-open; this work does not add Spotify tokens or playback API. |

## Pixel smoke (2026-08-02) — OPEN

Device: Pixel 9 `4B230DLAQ001Z5` · `serve-deeplink-proof` (adapter_count=6, media=spotify)

**Full Copy+Open** request id: `04e29edc-0b62-4ff2-8b75-6cf0ef7d0e95` (after debug APK reinstall with Spotify HandOffActions)

1. Home Auto → Spotify play utterance
2. Preview: `Prepare a Spotify draft` / `Spotify · play` → **Open Spotify**
3. Execute: `reached=hands_off done=true handed_off_to=Spotify`
4. Result sheet: **Handed off**, Copy draft, **Open Spotify**, cannot-know (no “played”)
5. Open Spotify → focus `com.spotify.music/...MainActivity`

Earlier companion-only pass: `0b2cf2b0-…` (Open button missing until APK refresh).

