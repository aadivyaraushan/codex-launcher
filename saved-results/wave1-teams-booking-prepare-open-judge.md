# Wave 1 Teams + Booking.com prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles before grading. No implementer bug checklist.  
**Product code:** not edited.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-rides-food-handoff-pack-judge.md`, `wave1-google-photos-prepare-open-judge.md`.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-teams-booking-prepare-open.md`. Glob found **no** such file. Overnight status claims it (`wave1-overnight-batch-and-oauth-prep.md:195`) — claim is stale/false. Glob/Grep found **no** prior `wave1-teams-booking-prepare-open-judge.md`.
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. First define from first principles what a strong result for THIS task must look like, then grade. Do not use an implementer bug checklist. … Write `saved-results/wave1-teams-booking-prepare-open-judge.md` (date 2026-08-02). No product code edits."

**Code reviewed:** `adapters/deeplink` Wave1Specs (`teams`, `booking`), `runtime/deeplink`, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `deeplink_proof.go`, plan rows for personal Teams + Booking.com  
**Package re-check (this session):** Play Store `id=com.microsoft.teams` HTTP 200; `id=com.booking` HTTP 200  
**Tests re-run (this session):**  
- `go test ./internal/capability/adapters/deeplink/ ./internal/capability/runtime/deeplink/ ./internal/capability/routing/stage1/openai/` → **57 passed**  
- `./gradlew :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest` → **BUILD SUCCESSFUL**

---

## Standard (first principles — before hunting bugs)

A strong result for **this** task — add personal Microsoft Teams prepare-and-open (compose) and Booking.com search prepare-and-open (read) to the Wave 1 deeplink pack — must have:

1. **Honest product ceiling** — Personal Teams is compose hand-off only (Graph chat send does not support personal Microsoft accounts). Booking.com is search/read hand-off only (vendor-confirmed: reservations finish on Booking.com). Neither path may claim sent / booked / reserved.
2. **Spec contract** — Two Wave1 Specs: `teams` (messaging, `compose`, package `com.microsoft.teams`) and `booking` (travel, `read`, package `com.booking`); shared deeplink shape: RT-4, `hands_off`, Auth none, Consent A; Execute via draft hand-off outcome.
3. **Phone map** — Android maps display names `Teams` and `Booking.com` (case-insensitive) to those same packages so Open can launch the real apps.
4. **Live routing coaching** — Stage1 instructions must name `app_named` / class / verb for both apps and refuse completion claims, so messaging’s multi-adapter class can still pick Teams and travel can pick Booking.
5. **Tests that would go red if contracts break** — Spec membership and package/verb/ceiling; empty draft + wrong verb (`send` / `book`); end-to-end flow to HandedOffTo with `hands_off` and no completion words; stage1 coaching assertions; Android package map.
6. **Evidence honesty** — A saved-results note that records what shipped, what was tested, and what was not device-proven (no invented Auto→Open).

---

## Verdict: **Pass-with-warnings**

Core Specs, packages, stage1 coaching, companion + Android unit wiring, and anti-completion tests meet the product bar. Warnings: required evidence file is missing (overnight invents it), and runtime proof/logging copy still under-reports the new `travel` class.

---

## Findings (file:line evidence)

### Passes

- **Specs match the personal-Teams / search-only ceilings.** `teams`: package `com.microsoft.teams`, verb `compose`, AppClass `messaging`, proves `teams_personal_prepare_open_smoke`, comment cites Graph personal-account limit (`adapter.go:62-63`). `booking`: package `com.booking`, verb `read`, AppClass `travel`, proves `booking_search_prepare_open_smoke`, comment forbids reservation claims (`adapter.go:64-65`). Shared `Describe()` sets `Runtime=RT4`, `Ceiling=HandsOff`, `Auth=AuthNone`, `Consent=ConsentA` (`adapter.go:86-96`). Execute uses `handoff.DraftOutcome` (`adapter.go:150-155`; detail never claims sent/booked — `handoff/outcome.go:23-34`).
- **Plan alignment (ceiling, not runtime letter).** Plan: personal Teams RT-4 compose hands_off (`consumer-app-implementation-plan.md:1464`); Booking.com read hands_off search-only (`:1432`). Pack ships both as deeplink prepare-and-open under that ceiling — correct for this Wave 1 hand-off ask (same pattern as rides/food demotion when credentials/connectors are not in scope).
- **Android package map matches Specs.** `"teams"` → `com.microsoft.teams`; `"booking.com"` → `com.booking` (`HandOffActions.kt:32-33`). Unit tests cover `Teams`/`teams` and `Booking.com`/`booking.com` (`HandOffActionsTest.kt:37-40`). Play Store listings for both ids returned HTTP 200 this session.
- **Stage1 coaches both routes and refuses completion.** Personal Teams: messaging / teams / compose + Graph personal-account note (`client.go:77`). Booking: travel / booking / read + “never claim a hotel was booked” (`client.go:78`). Unit tests assert those strings and reject Graph/bot-send / booking-completion coaching (`client_test.go:165-226`).
- **Tests cover the contracts; re-run green.** Wave1Specs rows + count 17 (`adapter_test.go:38-39`, `flow_test.go:93-94`); compose/read execute cases (`adapter_test.go:108-109`) with shared ban including `sent`/`booked`/`reserved` (`:141-144`); Teams empty draft + `send` reject and Booking empty draft + `book` reject (`adapter_test.go:287-312`); flow routes to HandedOffTo `Teams` / `Booking.com` with hands_off and bans `sent` / `booked|reserved|purchased` (`flow_test.go:481-543`). This session: **57** Go tests passed across adapters/deeplink + runtime/deeplink + stage1/openai; HandOffActions Android unit **BUILD SUCCESSFUL**.
- **Proof serve log names both apps.** `deeplink_proof.go` logs `messaging=messages+discord+teams` and `travel=booking` (`deeplink_proof.go:38-40`). Runtime registers from `Wave1Specs()` into ClassMap by AppClass (`flow.go:50-59`), so travel→booking and messaging→…+teams are live once Specs exist.

### Warnings

- **Required evidence artifact missing.** `saved-results/wave1-teams-booking-prepare-open.md` does not exist (Read/Glob empty). Overnight batch still points at it (`wave1-overnight-batch-and-oauth-prep.md:195`) and claims go 58/58 — this session measured **57** in the three packages above. Delivery/honesty gap against standard item 6; does not reverse the Spec/test contracts.
- **Runtime ready-log omits travel.** `[capability-runtime] deeplink flow ready` logs money/food/media/messaging/rides counts but not `travel_adapters` (`flow.go:61-69`), even though ClassMap includes `travel` from Specs. Ops signal lag only — routing still works (flow test `TestDeepLinkFlowRoutesTravelReadToBooking`).
- **Comment drift.** `runtime/deeplink/flow.go` package/New comments still list Wave-1 apps through DoorDash and omit Teams/Booking (`flow.go:1-40`). Adapters still register dynamically.
- **Device Auto→Open not evidenced.** With no evidence file and overnight noting Pixel unpaired (`wave1-overnight-batch-and-oauth-prep.md:196`), physical Open remains unproven. Not a Fail while no one claims device success.

---

## Gaps / risks

1. **Evidence debt:** Write `wave1-teams-booking-prepare-open.md` (or fix overnight status) so the next operator can reproduce packages, commands, and Pixel honesty without trusting chat.
2. **Ops visibility:** Add `travel_adapters` (and refresh comments) so serve logs match the 17-Spec pack.
3. **Pixel stop-line:** After re-pair, run Auto→preview→Open for Teams and Booking.com with apps installed; do not treat unit green as device proof.

None of these reverse the product bar: personal Teams is compose hand-off, Booking.com is search/read hand-off, packages are real, stage1 coaches both, and companion/Android unit tests are green.

---

## One-line next tip

Write the missing `saved-results/wave1-teams-booking-prepare-open.md` with the green test commands from this session, then add `travel_adapters` to the runtime ready-log before Pixel Auto→Open.
