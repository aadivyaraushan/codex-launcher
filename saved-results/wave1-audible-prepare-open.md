# Wave 1 Audible prepare-and-open

**Date:** 2026-08-02  
**Purpose:** Record overnight Group A #4 Audible prepare-and-open (hand-off
only; no public API / no tokens), wired through the shared deeplink adapter
pack and `serve-deeplink-proof`.

**Callers:** overnight Group A batch; companion `adapters/deeplink` +
`runtime/deeplink` + `serve-deeplink-proof`; Android `HandOffActions`.  
**User ask:** Overnight Group A #4: Audible prepare-and-open (no public API →
hand-off only). Same shape as Spotify: hands_off, DraftOutcome, no API tokens.

## Result

Extended `adapters/deeplink.Wave1Specs()` (least code — same
`handoff.DraftOutcome` path as Spotify/money/food):

| ID | App | Package | AppClass | Verbs |
|---|---|---|---|---|
| audible | Audible | `com.audible.application` | media | read, play |

Package verified from Google Play Store id=`com.audible.application`
(2026-08-02 web check). Ceiling `hands_off`, consent A, auth none.

`runtime/deeplink` ClassMap still built from each Spec's AppClass:

- `money` → venmo, cashapp, zelle
- `food` → starbucks, chipotle
- `media` → spotify, **audible**

Stage1 OpenAI instructions coach Audible as `app_class media`,
`app_named audible`, verb `play` (listen/continue) or `read` (library/title
lookup), open only — no playback/library control claims.

Android `HandOffActions` maps display name `Audible` → `com.audible.application`.

```bash
go run ./companion/cmd/codex-launcher serve-deeplink-proof
```

Needs `OPENAI_API_KEY` only. No Audible OAuth / API tokens.

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

- `go test` on the five packages above → **41 passed**
- Focused red→green covered Audible Wave1Specs, media ClassMap routing
  (spotify+audible), read+play hands_off outcomes, stage1 coaching,
  CLI serve adapter_count 7
- Android `HandOffActionsTest` → BUILD SUCCESSFUL (Audible package assert)

## Sibling sites checked

Searched `audible`, `Audible`, `com.audible`, `Wave1Specs`, `HandOffActions`,
`serve-deeplink-proof`, `app_class media`, `spotify`.

| Site | Decision |
|---|---|
| `adapters/deeplink` + `runtime/deeplink` | Extended here (Audible under media ClassMap). |
| `HandOffActions` | Added `audible` → `com.audible.application`. |
| Stage1 openai instructions | Added Audible prepare-and-open coaching. |
| `serve-deeplink-proof` / `deeplink_proof.go` | Same entrypoint; adapter_count 7; media=spotify+audible. |
| Spotify prepare-and-open | Sibling media adapter; left intact. Updated its “Audible not yet” note. |
| Vendor audit / RT1 audit | Still correctly say no official public Audible API → hand-off only. No code change. |
| `planning/consumer-app-implementation-plan.md` Audible RT-1 completes row | Planning history / older route table. Overnight release policy is prepare-and-open hand-off (this work). Left planning alone. |
| Coverage plan “R1 official connector” | Same — planning/research note, not the release route implemented here. |

## Pixel smoke (2026-08-02) — OPEN

Callers: overnight Group A follow-up after [Audible hand-off](4b8eae5a-82a2-436f-9a55-280f6501ff33).  
User ask: perform follow-up on Audible subagent completion.

Device: Pixel 9 `4B230DLAQ001Z5` · `serve-deeplink-proof` (adapter_count=7)  
Request id: `265a44c7-3c90-4515-aaf8-1531036fa223`

1. Home Auto → “Play my current audiobook on Audible”
2. Preview → **Open Audible**
3. Execute: `reached=hands_off done=true handed_off_to=Audible`
4. Result sheet: Handed off + Copy draft + Open Audible + cannot-know
5. Open Audible → focus `com.audible.application/...FtueExperienceActivity`

## Blockers

None.

