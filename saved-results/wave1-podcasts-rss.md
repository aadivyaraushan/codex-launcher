# Wave 1 Podcasts plain RSS (RT-2)

**Date:** 2026-08-02  
**Purpose:** Record overnight Wave-1 Podcasts row: RT-2, verbs `read|play`,
ceiling `completes`, Consent A, plain RSS (no partner API / no OAuth).

**Callers:** companion `adapters/podcasts` + `runtime.NewPodcasts` + stage1
openai coaching; `codex-launcher serve-podcasts-proof` (`podcasts_proof.go`).  
**User ask:** Implement Wave-1 Podcasts from the plan — least code that lists
episodes from a feed URL and plays by resolving an enclosure URL.

## Done (observable)

| Verb | Input | Output |
|---|---|---|
| `read` | configured feed URL (or `Fields["feed_url"]`) | Completes with tab-separated episode list (`guid`, title, enclosure URL) |
| `play` | feed URL + subject (title substring or guid) | Completes with play plan whose `enclosure_url` is the media file; preview shows title + URL |

## How play works

1. Fetch RSS 2.0 feed (`GET`).
2. Parse `<item>` nodes that carry `<enclosure url length type>` (RSS Board
   enclosure; Podcast Index treats enclosure as the playable episode file).
3. Match one episode by exact/partial title or guid (ambiguous → error).
4. Plan/Execute carry `enclosure_url` (and type/length). Ceiling is
   `completes` because resolving the playable media URL finishes the request
   without a vendor session — unlike Spotify/Audible/Apple Music prepare-and-open.

Docs checked: [RSS 2.0 `<enclosure>`](https://www.rssboard.org/rss-specification)
(url/length/type required attrs); Podcast Index itunes reference (enclosure
required for playable episodes; guid fallback to enclosure URL).

## Paths

| Path | Role |
|---|---|
| `companion/internal/capability/adapters/podcasts/client.go` | `Feed`, `HTTPClient`, `ParseFeed`, `Episode` |
| `companion/internal/capability/adapters/podcasts/adapter.go` | RT-2 adapter `read`/`play`, AuthNone |
| `companion/internal/capability/adapters/podcasts/podcasts_test.go` | Fixture RSS + httptest + fake Feed |
| `companion/internal/capability/runtime/podcasts.go` | `NewPodcasts` ClassMap `media` → `podcasts` |
| `companion/internal/capability/runtime/podcasts_test.go` | Prepare/Confirm play through enclosure |
| `companion/internal/capability/routing/stage1/openai/client.go` | Stage1 coaching for plain RSS Podcasts |

Manifest: id `podcasts`, runtime RT-2, ceiling completes, consent A, auth none,
cost free, proves_ceiling `podcasts_rss_enclosure_play_smoke`.

## Tests (red → green this session)

```bash
go test ./companion/internal/capability/adapters/podcasts/ \
        ./companion/internal/capability/runtime/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        -count=1 \
        -run 'Podcasts|ParseFeed|HTTPClient|ManifestIsFree|ReadReturns|PlayResolves|PlayBy|RevokeIs|CoachPodcasts'
```

Green (2026-08-02): **10 passed** across the three packages. Unit tests use an
embedded RSS fixture / `httptest` — no live network.

## Sibling sites checked

Searched `podcasts`, `Podcasts`, `enclosure`, `ParseFeed`, `serve-podcasts`.

| Site | Decision |
|---|---|
| `adapters/podcasts` + `runtime/podcasts` | New here (was empty). |
| Stage1 openai instructions | Added Podcasts plain-RSS coaching (distinct from Spotify/Audible/Apple Music prepare-and-open). |
| `adapters/deeplink` media pack | Left alone — those rows are hands_off open-package; Podcasts completes via enclosure. |
| `HandOffActions` / Android | No package map needed tonight; play path is the enclosure URL in the outcome detail. |
| `serve-*-proof` CLI | Wired: `serve-podcasts-proof` (OPENAI_API_KEY + `PODCASTS_FEED_URL`; oauth=none). |
| Plan row Podcasts | Still RT-2 read/play completes Consent A plain RSS — matches. |

## Blockers

None for overnight RSS list/play. Pixel smoke / opening the enclosure in a
player on-device is still open if product wants a hand-off package later.
No commit made (per ask).

## Judge follow-up (2026-08-02)

Callers: none in code (human/overnight artifact). Peer: `wave1-podcasts-rss-judge.md`. No schema.
User: follow-up on Podcasts judge — add missing regression tests.

[Judge Podcasts RSS](3203b2c2-e57d-4b2c-b28c-9960fe4ce1f8) **Pass-with-warnings** → tests added (green):

1. `Fields["feed_url"]` override used on Resolve
2. Parse skips no-enclosure items; empty guid falls back to enclosure URL
3. Play rejects matched episode with empty enclosure (`ErrNoEnclosure`)

Still open (disclosed): on-device player / Pixel open of enclosure URL.

## Serve proof CLI (2026-08-02)

**Callers:** `main.go` `serve-podcasts-proof` / `liveDependencies.startPodcastsProof`;
`TestPodcastsProofServeUsesTheCapabilityFlow`.  
**User ask:** Implement `serve-podcasts-proof` CLI wiring like Instagram (no OAuth).

| Item | Detail |
|---|---|
| Command | `codex-launcher serve-podcasts-proof` |
| Env | `PODCASTS_FEED_URL` (required, trimmed); `OPENAI_API_KEY` for stage1 |
| Runtime | `capabilityruntime.NewPodcasts` + `podcastsadapter.NewHTTPClient(nil, logger)` |
| Ready log | `adapter_count=1`, `oauth=none`, `feed_url_len` (never the raw URL) |
| Paths | `companion/cmd/codex-launcher/podcasts_proof.go`; `main.go` field + branch + needsConfiguredRuntime |

### Tests (red → green)

```bash
go test ./companion/cmd/codex-launcher/ -count=1 -run 'PodcastsProof|Usage|InstagramProof'
```

- RED: `unknown field startPodcastsProof in struct literal of type liveDependencies`
- GREEN focused: **3 passed** (`PodcastsProof` + InstagramProof suite)
- GREEN verify (cmd + adapters/podcasts + runtime + stage1/openai): **89 passed** in 4 packages

Live serve smoke: **skipped** — main `.env` has `OPENAI_API_KEY` but no `PODCASTS_FEED_URL`. Pixel on Pair — no device smoke.

