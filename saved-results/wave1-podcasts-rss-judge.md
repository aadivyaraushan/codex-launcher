# Wave 1 Podcasts plain RSS — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-podcasts-rss.md`  
**Code reviewed:** `companion/internal/capability/adapters/podcasts/` (`client.go`, `adapter.go`, `podcasts_test.go`), `runtime/podcasts.go` + `podcasts_test.go`, stage1 OpenAI coaching + `TestStage1InstructionsCoachPodcastsPlainRSSAsMedia`  
**Plan row checked:** `planning/consumer-app-implementation-plan.md` Media — Podcasts, RT-2, read/play, completes, Class A, Wave 1, plain RSS  
**Tests re-run (this session):** `go test` on adapters/podcasts, runtime (Podcasts filters), stage1/openai CoachPodcasts → **10 passed**

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** `saved-results/wave1-podcasts-rss.md` (delivery evidence). No prior judge file for this path.
3. **Data I/O:** None — static markdown verdict.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong Wave-1 Podcasts plain-RSS adapter (read|play, completes, Consent A, no vendor OAuth) must have, then grade the work. … Bar: real RSS enclosure parsing, read lists episodes, play matches one and completes with enclosure URL, tests use fixture/httptest (not flaky live net), secrets N/A, ceiling honesty. Return Pass | Pass-with-warnings | Fail with concrete gaps only. Save to `saved-results/wave1-podcasts-rss-judge.md`."

## First-principles bar (before hunting bugs)

A strong Wave-1 Podcasts **plain-RSS** adapter (not Apple Podcasts partner API, not OAuth) must:

1. **Product shape** — Given a public feed URL, `read` lists episodes; `play` picks one episode and finishes by resolving its playable media URL. No vendor login, no partner token.
2. **Contract** — RT-2, verbs `read|play`, ceiling `completes`, Consent A, Auth none, cost free. Secrets truly N/A (`Revoke` is a no-op).
3. **Real RSS enclosure parsing** — Fetch RSS 2.0; parse `<item>` nodes that carry `<enclosure url … type …>` (length too); skip items with no enclosure URL; stable guid (fallback to enclosure URL when guid empty).
4. **Verb behavior** — `read` Completes with a list that includes enclosure URLs. `play` matches by title/guid (exact then partial); one match required; ambiguous/no-match errors; plan/preview/outcome carry `enclosure_url`.
5. **Ceiling honesty** — `completes` is justified only as “media URL resolved without a vendor session.” Must not claim the audio was played or a device player was opened unless proven. On-device open, if still undone, must be disclosed.
6. **Routing** — Stage1 coaches `app_class media` / `app_named podcasts` with plain-RSS / enclosure language, distinct from Spotify/Audible/Apple Music prepare-and-open. Runtime registers under `media`.
7. **Real tests** — Fixture RSS + `httptest` (no live network). Tests that would fail if parse, read list, play match, Completes outcome, AuthNone/ConsentA, or coaching broke.

## Verdict: **Pass-with-warnings**

Core Wave-1 contracts above are met in code and covered by tests re-run green in this session. Not a Fail. Ceiling language in evidence matches what Execute actually does (URL in outcome detail, not a player launch).

## Concrete gaps only

1. **`Fields["feed_url"]` override is claimed and implemented, but untested.**  
   Evidence Done table lists configured feed URL *or* `Fields["feed_url"]`. `resolveFeedURL` supports the override (`adapter.go`); no test asserts Intent Fields override the configured URL.

2. **Parse edge cases for “real enclosure parsing” are untested.**  
   Code skips empty enclosure URL and falls guid back to enclosure URL (`ParseFeed` in `client.go`). Fixture only has well-formed items with guids. No red/green case for “item without enclosure dropped” or “empty guid → enclosure URL.”

3. **On-device player open / Pixel smoke still open (disclosed).**  
   Evidence Blockers: Pixel smoke / opening the enclosure in a player is still open. Execute Completes with a detail string containing the enclosure URL — correct for this plain-RSS Completes bar, but not a device hand-off proof. Honesty holds; product UI stop-line is not closed.

## Focus checklist

| Focus | Grade |
|---|---|
| Real RSS enclosure parsing | Pass (with gap #2) — XML `<enclosure url/length/type>`; httptest fetch+parse; fixture lists two enclosure URLs |
| `read` lists episodes | Pass — Resolve/Preview/Execute carry tab-separated guid/title/enclosure; Completes + Done |
| `play` matches one + enclosure URL | Pass — ambiguous subject errors; title and guid match; plan/preview/outcome carry enclosure URL; runtime Prepare→Confirm Completes |
| Fixture / httptest tests (no live net) | Pass — embedded `sampleFeed` + `httptest`; fake Feed in adapter/runtime; **10** green this session |
| Secrets N/A / no vendor OAuth | Pass — `AuthNone`; no oauth/podcasts package; Revoke noop + test; stage1 asserts “no partner api” / “no oauth” |
| Ceiling honesty | Pass (with gap #3) — Manifest Completes + Consent A; evidence distinguishes enclosure Completes from Spotify/Audible/Apple Music prepare-and-open; does not claim audio played on device |

## Evidence cross-check (not gaps unless they break the bar)

- Plan row (RT-2, read/play, completes, A, plain RSS) matches `Describe()` and stage1 coaching.
- `NewPodcasts` wires ClassMap `media` → `podcasts`; flow test routes play through enclosure Completes.
- Skipping `serve-*-proof` CLI is consistent with secrets N/A (unlike OAuth siblings); unit+runtime evidence is enough for this overnight bar.
- Deeplink media pack left alone is correct — those rows are hands_off open-package; Podcasts Completes via enclosure.
