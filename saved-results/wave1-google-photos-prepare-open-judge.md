# Wave 1 Google Photos prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles before grading. No implementer bug checklist.  
**Product code:** not edited.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-apple-music-prepare-open-judge.md`, `wave1-discord-prepare-open-judge.md`.
2. **Existing peer:** `saved-results/wave1-google-photos-prepare-open.md` is delivery evidence, not a judge note. Glob/Grep found no `wave1-google-photos-prepare-open-judge` file.
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "You are an independent LLM-as-judge with fresh context. First define from first principles what a strong result for THIS task must look like, then grade the work. Do NOT use a checklist of suspected bugs from the implementer. … Write judge note to `saved-results/wave1-google-photos-prepare-open-judge.md` (date 2026-08-02). Do not edit product code."

**Evidence reviewed:** `saved-results/wave1-google-photos-prepare-open.md`, Photos row in `saved-results/wave1-vendor-route-audit.md`  
**Code reviewed:** `adapters/deeplink` (`Wave1Specs` `googlephotos`), `runtime/deeplink`, `oauth/google.ScopesForVerbs`, `HandOffActions` + unit test, stage1 OpenAI coaching, `deeplink_proof.go` media tag, Google runtime (Calendar/Drive only)  
**Vendor docs re-checked (this session):** [Photos API updates](https://developers.google.com/photos/support/updates) — `photoslibrary.readonly` / `sharing` / `photoslibrary` removed effective **2025-03-31**; remaining Library scopes are app-created / append-only; full-library pick → Picker API (user selects).  
**Package re-check (this session):** Play Store `id=com.google.android.apps.photos` HTTP 200.  
**Tests re-run (this session):**  
- `go test ./companion/internal/capability/adapters/deeplink/ ./companion/internal/capability/runtime/deeplink/` → **38 passed**  
- `go test ./companion/internal/capability/oauth/google/` → **4 passed**  
- `./gradlew :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest` → **BUILD SUCCESSFUL**

---

## Standard (first principles — before hunting bugs)

A strong result for **this** task — demote Google Photos after full-library Library API scopes went away, ship prepare-and-open hand-off (not OAuth completes) — must have:

1. **Vendor-grounded product choice** — Wave 1 must not treat Photos as a Calendar/Drive-style OAuth completes path for “find my library / upload and claim done.” Full-library list/search scopes are gone; remaining Library scopes are app-created data (or append-only upload), and full-library selection is Picker (user picks). Prepare-and-open hand-off is the honest Wave 1 product for consumer “find / save in Photos.”
2. **Spec contract** — A `googlephotos` deeplink Spec with `hands_off` ceiling, Auth none, Consent A, RT-4, media class, verbs that express find/save intent without `send`, and Android package `com.google.android.apps.photos`. Execute must hand off via shared draft outcome and never claim found / uploaded / saved / completed.
3. **Phone map** — Android maps display name `Google Photos` (case-insensitive) to that same package so Open can launch the real app.
4. **OAuth demotion held** — `oauth/google.ScopesForVerbs` stays Calendar + Drive only; no Photos Library scopes added; no Photos OAuth completes adapter beside Calendar/Drive.
5. **Tests that would go red if the contracts break** — Spec membership, ceiling/auth, read|write paths, empty draft + wrong verb reject, flow route to HandedOffTo Google Photos with hands_off and no completion words, Android package map.
6. **Evidence + audit honesty** — Saved result and vendor-audit row state the demotion reason, the shipped route, and what was / was not device-proven.

---

## Verdict: **Pass-with-warnings**

Core demotion is real and correctly shipped: Spec, Android map, unit/flow tests, evidence, and Google OAuth ceiling all match the bar. Warnings are about live stage1 coaching and stale proof/logging copy — not about wrongly shipping an OAuth Photos completes path.

---

## Findings (file:line evidence)

### Passes

- **Spec is prepare-and-open, not OAuth completes.** `googlephotos` is the 15th Wave1 Spec: package `com.google.android.apps.photos`, verbs `read|write`, AppClass `media`, proves ceiling `googlephotos_prepare_open_smoke`, with an inline note that Library full-library scopes were removed 2025-03-31 (`companion/internal/capability/adapters/deeplink/adapter.go:60-61`). Shared `Describe()` sets `Ceiling=HandsOff`, `Auth=AuthNone`, `Consent=ConsentA`, `Runtime=RT4` (`adapter.go:82-92`). Execute uses `handoff.DraftOutcome` (`adapter.go:146-151`; `handoff/outcome.go:23-34`).
- **Android package map matches Spec.** `HandOffActions` maps `"google photos"` → `com.google.android.apps.photos` (`HandOffActions.kt:31`); unit test covers `"Google Photos"` and `"google photos"` (`HandOffActionsTest.kt:35-36`). Play Store listing for that id returned HTTP 200 this session.
- **Tests cover the demotion contracts.** Wave1Specs row + hands_off/auth (`adapter_test.go:37`, `:48-53`); read + write execute cases (`adapter_test.go:104-105`); empty draft + `send` reject (`adapter_test.go:270-281`); runtime flow asserts AdapterID `googlephotos`, `HandsOff`, HandedOffTo `Google Photos`, and rejects completion words `found` / `uploaded` / `saved` / `completed` (`runtime/deeplink/flow_test.go:447-478`). Wave1Specs count locked at 15 (`flow_test.go:93-94`). Re-run this session: 38 deeplink tests green; HandOffActions unit test green.
- **Google OAuth was not widened for Photos.** `ScopesForVerbs` returns only `calendar.events` + `drive.file` (`oauth/google/flow.go:106-121`, constants `:27-32`). Ceiling test forbids gmail / broad drive / broad calendar; no Photos scopes present (`flow_test.go:19-37`). Grep of `oauth/` and adapters: no `photoslibrary` / Photos Library OAuth adapter. Google runtime wires Calendar + Drive only (`runtime/google.go:31-33`). Correct demotion.
- **Evidence + vendor audit align with vendor docs.** Evidence records demotion, Spec, package, and test commands (`wave1-google-photos-prepare-open.md`). Audit row states removed scopes, Picker vs full-library, Wave 1 = prepare-and-open, do not ship Calendar-style OAuth completes (`wave1-vendor-route-audit.md:46`). Official updates page confirms the 2025-03-31 scope removals (fetched this session).

### Warnings (do not overturn demotion)

- **Stage1 has no Google Photos coaching line.** Peer prepare-and-open apps (Spotify, Audible, Apple Music, DoorDash, …) get explicit `app_named` / verb / “open only” instructions in `routing/stage1/openai/client.go:70-79`. There is **no** `googlephotos` / “Google Photos” line (Grep of `stage1/` empty). Runtime ClassMap will still include `googlephotos` via Spec registration (`runtime/deeplink/flow.go:50-59`), and the flow test stubs the model (`flow_test.go:449-450`), so unit green does not prove live model routing will pick Photos.
- **Proof/logging copy stale relative to Spec pack.** `deeplink_proof.go` still logs `media` as `spotify+audible+applemusic` (no `googlephotos`). `runtime/deeplink/flow.go` package comment (`:32-40`) lists Wave-1 apps through DoorDash and omits Google Photos even though Spec registration is dynamic. Ops/human signal lag only — adapters still register from `Wave1Specs()`.
- **Shared adapter completion-word list is media-music skewed.** `adapter_test.go:137` bans `played` / `added to playlist` / etc., but not `found` / `uploaded` / `saved`. Photos-specific words are covered in the dedicated flow test (`flow_test.go:473-477`), so this is test-hygiene, not a live claim bug (`DraftOutcome` detail does not use those words).
- **Live Pixel Auto→Open not run.** Evidence discloses device unpaired and stops at companion unit path (`wave1-google-photos-prepare-open.md:36`). Honesty is fine; physical Open remains unproven.

---

## Gaps / risks

1. **Live routing risk (main product gap):** Without stage1 coaching, a real Responses-API stage1 may not reliably emit `app_named=googlephotos` / media `read|write` for Photos utterances — so Spec + phone map can exist while overnight Auto→Open never selects the adapter.
2. **Proof log / comment drift** can mislead the next operator about whether Photos is in the media pack.
3. **Device Open** still needs a paired Pixel with the Photos app installed before claiming end-to-end hand-off.

None of these reverse the demotion: Photos is not an OAuth completes adapter, and Google OAuth scopes were not expanded.

---

## One-line next recommendation

Add stage1 OpenAI coaching (+ a coaching unit assertion) for `app_class media` / `app_named googlephotos` with verbs `read|write` and “open only — never claim found/uploaded/saved,” then refresh the `deeplink_proof` media log string and run Pixel Auto→Open when paired.
