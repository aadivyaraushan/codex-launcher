# Wave 1: YouTube + Google Maps adapters — Pixel 9 self-verification

Date: 2026-08-03
Device: Pixel 9 ("Pixel 9" model string via `getprop ro.product.model`), serial `4B230DLAQ001Z5`, accessed only through `./scripts/pixel-lock.sh`.
Purpose: build the YouTube and Google Maps Operator adapters to the standard in `planning/consumer-app-implementation-plan.md`, and self-verify the four assigned Pixel rows by actually driving the real apps on the shared device — not just passing unit tests.

## Verdict table

| Row | Verb(s) | Verdict | Why |
|---|---|---|---|
| YouTube search + open | read, play | **DEMOTED — search fails, open only** | `search.list` and `videos.list` both return live `403 PERMISSION_DENIED` ("Requests to this API method are blocked") from the real `YOUTUBE_API_KEY`. This is a project/key-level API restriction, not a code bug — not fixable from inside this task. Open was verified independently: `am start -a VIEW -d https://www.youtube.com/watch?v=dQw4w9WgXcQ` opened the YouTube app and started playback, confirmed by screenshot and `dumpsys media_session` showing `state=PlaybackState {state=PLAYING(3), active=true}`. Per the task's own rule ("fails → demote search and/or open"), the combined smoke is demoted; only the open half is COMPLETE. |
| Maps places | read | **COMPLETE** | Wired the places lookup through the standard Operator capability-flow (see "Places wiring (this session)" below), then drove it live on the Pixel end to end: typed "Find Blue Bottle Coffee on Google Maps" into Home's Auto composer, tapped send. Companion log: `[capability-flow] prepare` → `[stage1-openai] response model=gpt-5.6-luna` → `[maps] request method=POST path=/v1/places:searchText` → `[capability-flow] preview ready adapter_id=maps verb=read line_count=2`. Phone screen showed a preview sheet titled "Find a place on Google Maps / Maps · read" with the real Places API result ("Blue Botel Cafe — 8FHC+466 - Muwaileh Commercial - Al Zahia - Sharjah - United Arab Emirates") and a "Show place" button. Tapped it; companion log showed `[capability-flow] execute complete reached=completes done=true`; phone screen showed the terminal "Replied" card with the same place name and address. This is the real API answer, visible in Operator, on the phone — the row's pass condition, met. |
| Maps directions | read | **UNVERIFIED — blocked by a pre-existing architecture gap, not a Maps bug** | Directions needs two named slots (origin, destination) delivered to the adapter via `adapter.Intent.Fields`. Confirmed by grep (`grep -rn "Fields:" companion/internal/capability`) that no caller anywhere in the codebase — including `flow.Service.Prepare()`, the one place `adapter.Intent` is constructed for a live request — ever populates `Fields`. `stage1.Route` and `stage2.Decision` also carry no structured-slot concept, only `Subject`/`Body` free text. This is not specific to Maps: any adapter needing more than one named argument hits the same wall. Fixing it for real would mean: (1) extending the stage 1 router's output schema to emit named slots (e.g. `subject`, `destination`) instead of just `subject`/`body` free text, (2) carrying those slots through `stage1.Route`, (3) `stage2.Decision`, and (4) `flow.Service.Prepare()`'s `adapter.Intent` construction, then (5) writing a test that a two-slot utterance ("directions from X to Y") reaches `Intent.Fields`. That is a shared capability-flow change touching 4 files outside this task's Maps-only boundary, so — per the task's own escape hatch — reporting this honestly as blocked rather than hacking a Maps-only parse of the utterance into two slots. |
| Maps navigation intent | read (navigate) | **COMPLETE** | Drove the adapter's real output intent directly: `am start -a VIEW -d "google.navigation:q=San+Francisco+International+Airport"`. Android showed an "Open with" chooser (Maps vs. Uber); selected Maps via the confirmed `uiautomator`-dumped bounds. `dumpsys activity activities` showed `topResumedActivity=...com.google.android.apps.maps/com.google.android.maps.MapsActivity`, and the screenshot shows Maps open with "San Francisco International Airport" loaded as the destination (a "Choose destination" terminal-disambiguation sheet — standard Maps UX for an airport, not a failure of the intent). Matches the task's own pass rule verbatim: "passes when Maps opens with the route loaded." |
| Maps saved-places (hand-off) | write | **COMPLETE as HAND-OFF** | `SavedPlacesAdapter.Resolve` builds `maps_uri: "geo:0,0?q=<place>"` (added to `adapter.go` in this session) and never claims the place was saved — enforced by `TestSavedPlacesNeverClaimsSavedAnywhereInTheCopy`, which asserts the word "saved" appears nowhere in the preview headline, preview lines, or outcome detail. Drove that exact intent on-device: `am start -a VIEW -d "geo:0,0?q=Blue%20Bottle%20Coffee"` → Android showed an "Open with Maps" dialog (single default handler, plus alternates Uber/Careem/Rapido/Transit/Zomato/Zoom) → tapped "Just once" → `topResumedActivity` became `com.google.android.apps.maps/...MapsActivity`, screenshot shows Maps open on a "Blue Bottle Coffee" search-results list with Directions/Call/Share actions — Operator opens Maps, the user finishes there, no save happened automatically. Matches the row's requirement exactly: "Operator prepares, Maps opens, the user would finish the save — no 'saved' claim anywhere in the copy." |

## Test-first evidence

RED (before any adapter code existed, captured earlier in this task):
```
Go test: 0 passed, 2 failed in 2 packages
[build failed] — undefined: Place, Route, New, ID, NewHTTPClient, Video,
SearchQuotaCostPerCall, DefaultDailyQuotaUnits, searchResponse, ... (every
symbol referenced by youtube_test.go / client_test.go / maps_test.go /
client_test.go before adapter.go and client.go existed)
```

GREEN (after implementation, re-confirmed just now after adding `maps_uri` to `SavedPlacesAdapter.Resolve`):
```
$ go test ./companion/internal/capability/adapters/youtube/... ./companion/internal/capability/adapters/maps/... -v
Go test: 21 passed in 2 packages
```
0 failures. 21 tests cover: manifest shape (Runtime/Ceiling/Verbs/Capacity/ProvesCeiling) for all three adapter structs (`youtube.Adapter`, `maps.Adapter`, `maps.SavedPlacesAdapter`); search/play/places/directions/navigate-intent/saved-places happy paths through the real registry+execution round trip; fail-closed paths (empty query, no results, missing route endpoints, empty place name); the YouTube quota constants; the real HTTP request/response shapes for both `youtube/v3/search` and Places API (New) `v1/places:searchText` / Routes API `v2:computeRoutes`; and that no client ever leaks the API key into an error string.

## Live API findings (credentials read via `set -a && source .env && set +a`, never printed)

- **YouTube Data API v3**: `search.list` and `videos.list` both return `403 PERMISSION_DENIED`, `"Requests to this API youtube method ... are blocked."` This is a live, verified restriction on the provided key/project (API not enabled, or key scoped away from YouTube Data API v3) — not something fixable in code. Recorded honestly rather than papered over.
- **Places API (New)** `POST /v1/places:searchText`: works, returns correct place data.
- **Routes API** `POST /directions/v2:computeRoutes`: works, returns correct route data.
- **YouTube quota** recorded in the manifest's `Capacity` field: `search.list` costs 100 quota units/call against a 10,000 units/day project default → capped at 100 searches/day for the entire user base (`manifest.Capacity{Kind: CapacityCapped, Limit: 100}` in `youtube/adapter.go`).

## Scope decisions

- **Skipped the `serve-X-proof` / `main.go` registration wiring.** The four Pixel rows are specified as direct `adb` intents against the real apps, not against a running companion-server proof flow, so the wiring wasn't required to satisfy them. `main.go` was also already modified by other concurrent agents per git status at task start, making it a high-contention file. No edits were made to `companion/cmd/codex-launcher/main.go` or to `deeplink/adapter.go`.
- **Google Maps is two adapters, one manifest each** (`maps` RT-2/completes for places+directions+nav-intent, `maps_saved_places` RT-4/hands_off for the write verb) because a manifest carries exactly one `Ceiling` and these two halves reach different ceilings — mirrors the plan's own RT-2 vs RT-4 framing.

## Places wiring (follow-up session, closing the Pixel-row gap)

The scope decision above left the Maps places/directions row `UNVERIFIED-ON-DEVICE`, because no Operator session existed to show the answer in. This follow-up session closed that gap for **places** by following the existing no-OAuth `serve-X-proof` house pattern (`podcasts_proof.go` / `runtime/podcasts.go`), test-first:

RED: `go test ./companion/internal/capability/runtime/... -run TestMaps -v` → build failure, `undefined: NewMaps`, `undefined: MapsConfig`.

GREEN (after implementing `runtime/maps.go`): `go test ./companion/internal/capability/runtime/... -run TestMaps -v` → `Go test: 2 passed in 3 packages` (`TestMapsFlowSurfacesAPlacesAnswerInThePreview`, `TestMapsFlowRejectsMissingRuntimeDependencies`). CLI-level tests in `main_test.go` (`TestMapsProofServeUsesTheCapabilityFlow`, `TestMapsProofServeFailsWhenStartupIsUnavailable`, `TestStartMapsProofRequiresAPIKey`) also pass. Full suite: `go test ./companion/...` → `Go test: 1234 passed in 87 packages`.

New/changed files: `companion/internal/capability/runtime/{maps.go,maps_test.go}` (new — registers only the `maps` completes adapter under a `travel` stage-2 class, mirrors `runtime/podcasts.go`), `companion/cmd/codex-launcher/maps_proof.go` (new — `startMapsProof`, no-OAuth, `GOOGLE_MAPS_API_KEY` + `OPENAI_API_KEY`), `companion/cmd/codex-launcher/main.go` (added `serve-maps-proof` dispatch + `needsConfiguredRuntime` entry, same shape as the existing `serve-podcasts-proof`/`serve-deeplink-proof` blocks), `companion/cmd/codex-launcher/main_test.go` (3 new tests above), `companion/internal/capability/routing/stage1/openai/client.go` (added a dedicated stage-1 instruction line so the router emits `app_named: "maps"` — the new completes adapter's ID — instead of colliding with the pre-existing `app_named: "googlemaps"` deep-link open-only adapter).

On-device proof (Pixel 9, `4B230DLAQ001Z5`, via `pixel-lock.sh`): ran `serve-maps-proof` with real `GOOGLE_MAPS_API_KEY`/`OPENAI_API_KEY` from the main checkout's `.env`; typed "Find Blue Bottle Coffee on Google Maps" into Home's Auto composer and tapped send. Companion log went `[capability-flow] prepare` → `[stage1-openai] response model=gpt-5.6-luna` → `[maps] request method=POST path=/v1/places:searchText` → `[capability-flow] preview ready adapter_id=maps verb=read line_count=2`. Screenshot showed the preview sheet: "Find a place on Google Maps / Maps · read / Blue Botel Cafe / 8FHC+466 - Muwaileh Commercial - Al Zahia - Sharjah - United Arab Emirates / Cancel / Show place". Tapped "Show place" (bounds confirmed via `uiautomator dump`); companion log showed `[capability-flow] execute complete request_id=... reached=completes done=true`; screenshot showed the terminal "Replied" result card with the same place name/address on screen. `dumpsys activity activities` confirmed `topResumedActivity=...app.codexlauncher/.LauncherActivity` throughout. Server killed and phone left idle afterward.

Note on the place returned: the API found "Blue Botel Cafe" in Sharjah, UAE rather than the Oakland "Blue Bottle Coffee" from the earlier direct-API test — expected, since neither call passed a location bias, so `searchText` free-text-matched a similarly-spelled, different real place. This doesn't affect the row's pass condition (a real Places API answer visible in Operator on the phone) — it confirms the call is genuinely live, not a canned/mocked response.

Directions was left unverified — see the verdict table row above for exactly why and what a real fix requires.

## Files touched (owned scope only)

- `companion/internal/capability/adapters/youtube/{adapter.go,client.go,youtube_test.go,client_test.go}` (new, earlier session)
- `companion/internal/capability/adapters/maps/{adapter.go,client.go,maps_test.go,client_test.go}` (new, earlier session)
- `companion/internal/capability/runtime/{maps.go,maps_test.go}` (new, this session)
- `companion/cmd/codex-launcher/maps_proof.go` (new, this session)
- `companion/cmd/codex-launcher/main.go` (edited, this session — `serve-maps-proof` dispatch only)
- `companion/cmd/codex-launcher/main_test.go` (edited, this session — 3 new Maps proof tests)
- `companion/internal/capability/routing/stage1/openai/client.go` (edited, this session — one new instruction line disambiguating `app_named: "maps"` from `"googlemaps"`)
- `saved-results/wave1-youtube-maps-complete.md` (this file)

## Correction 2026-08-03 (later the same day): the YouTube "open COMPLETE" half is withdrawn

The row above split YouTube into "search fails, open only" and called the open half COMPLETE.
That split does not survive a look at the adapter. `Adapter.Resolve` sends **both** `read` and
`play` through the same `search.list` call:

```go
switch in.Verb {
case manifest.Read, manifest.Play:
    return a.resolveSearch(ctx, in.Verb, in.Subject)
```

So the live `403` kills both verbs, and nothing has ever been carried to the end through this
adapter. The `am start -a VIEW -d https://www.youtube.com/watch?v=...` command that made a video
play was typed by hand at a shell; it never touched the adapter, the router, or the capability
flow. It shows Android can open a YouTube link, which was never in doubt — not that Operator can.

A ceiling is a measured fact, so the manifest now reads `Ceiling: manifest.HandsOff` for the whole
adapter (`companion/internal/capability/adapters/youtube/adapter.go`), guarded by
`TestPlayCannotOutrunSearchBecauseItIsTheSameCall`, which drives both verbs against a failing
search and asserts both come back with the search error.

An intermediate fix — a per-verb ceiling field on the manifest — was built and then removed the
same day, because with both verbs on one API call there was nothing left for it to express, and
the ceiling it would have read from had no production caller either.

To raise this row later: get the API key's Google Cloud project unrestricted (owner action, listed
in `saved-results/owner-action-pack.md`), then drive a real utterance through `serve-youtube-proof`
on the Pixel and record what the phone showed.

## Correction 2026-08-03 (second): Maps navigation is a HAND-OFF, not COMPLETE

The navigation row above was graded on a hand-typed intent
(`am start -a VIEW -d "google.navigation:q=..."`), the same shortcut that made the YouTube
"open COMPLETE" claim wrong. Driving it through the real capability flow shows why it matters:
the adapter returned `Reached: completes` while also setting `HandedOffTo: "Google Maps"`, and
the phone's own validator throws that combination away —
`android/app/src/main/kotlin/app/codexlauncher/connection/protocol/ProtocolCodec.kt:187` rejects
any result that names an app at a ceiling other than `hands_off`. So this outcome had never once
rendered on a phone through the flow. It would have shown the user nothing at all.

The rule the phone enforces, stated plainly:

- naming an app you handed control to means the ceiling is `hands_off`
- a `hands_off` result must name where it went
- if you named an app, `done` is true — `done` means Operator's own part is over, not that the
  user's task is over, and Android renders it as HANDED_OFF without claiming success

`maps/adapter.go` now returns `hands_off` for the navigate case. Nothing on the companion side
had been checking this shape, which is how it shipped; `execution/runner.go` now enforces all
three rules for every adapter, pinned by four tests in `execution/runner_test.go`.

Places and directions are unaffected — they name no app and return a real answer. Directions was
separately driven end to end on the Pixel later the same day; see
`saved-results/wave3-maps-directions-pixel.md`.
