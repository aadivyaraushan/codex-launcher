# Wave 1 rides/food hands_off pack — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-rides-food-handoff-pack.md`  
**Code reviewed:** `adapters/deeplink` Wave1Specs (uber / ubereats / resy / doordash), `runtime/deeplink` ClassMap, `handoff.DraftOutcome`, stage1 OpenAI coaching, `HandOffActions` + unit test, `serve-deeplink-proof` / `deeplink_proof.go`  
**Plan rows checked:** `planning/consumer-app-implementation-plan.md` — Uber (estimates) read/hands_off; Uber Eats read/hands_off; Resy read/hands_off; DoorDash (connector) read+order/hands_off; Uber booking BD-gated out of scope  
**Tests re-run (this session):**  
- `go test` adapters/deeplink + runtime/deeplink + stage1/openai + cmd/codex-launcher `-run 'DeepLink|Wave1|Stage1InstructionsCoachUber'` → **39 passed**  
- `./gradlew :app:testDebugUnitTest --tests '...HandOffActionsTest'` → **tests="3" failures="0"**  
**Play re-check (this session):** `com.ubercab`, `com.ubercab.eats`, `com.resy.android.prod`, `com.dd.doordash` live; `com.resy.android` → **404**

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** `saved-results/wave1-rides-food-handoff-pack.md` (delivery evidence). No prior judge file for this pack (Glob/Grep empty for this path).
3. **Data I/O:** None — static markdown verdict, no structured data files.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong Wave-1 Uber/Uber Eats/Resy/DoorDash hands_off prepare-and-open pack must have, then grade the work.

Worktree: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`

Evidence: `saved-results/wave1-rides-food-handoff-pack.md`
Code: deeplink Wave1Specs for uber/ubereats/resy/doordash, HandOffActions, stage1 coaching, ClassMap rides/food.

Bar: hands_off / Consent A, correct Play packages, no partner OAuth/API, verbs match plan ceilings (read vs order), tests real, Pixel honesty.

Return Pass | Pass-with-warnings | Fail with concrete gaps only. Save to `saved-results/wave1-rides-food-handoff-pack-judge.md`."

## First-principles bar (before hunting bugs)

A strong Wave-1 **rides/food hands_off prepare-and-open pack** must:

1. **Product shape** — Prepare text from the user’s request, show it, open the official consumer app, and stop. Operator must never claim a ride was booked, a table reserved, an order placed, or checkout finished.
2. **Ceiling contract** — Every row: `hands_off`, Consent A, Auth none. Device hand-off (RT-4 floor) is the correct ship shape when the overnight ask forbids partner credentials; plan RT-1 “connector” rows stay demoted until a real connector smoke proves higher.
3. **Correct Play packages (consumer apps only)** — Uber `com.ubercab`; Uber Eats `com.ubercab.eats`; Resy `com.resy.android.prod` (not the dead `com.resy.android`); DoorDash `com.dd.doordash` (not the driver package).
4. **Verbs match plan ceilings** — Uber estimates: `read` only (booking stays BD/out of scope). Uber Eats / Resy: `read` only. DoorDash: `read` + `order`, still hands_off (checkout unconfirmed).
5. **No partner OAuth / API path** — No Uber Riders API, no Resy / DoorDash / Uber Eats OAuth or API keys; stage1 must not coach those routes.
6. **End-to-end wiring** — Spec → rides/food ClassMap → stage1 `app_named` coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` loads Wave1Specs.
7. **Real tests** — Fail if ceiling/consent/packages/verbs/completion-claims/stage1 anti-partner coaching break; cover all four apps on the companion path; Android package map covered.
8. **Pixel honesty** — Say which apps were installed and launchable; do not claim full Auto→Open or Open-button success for apps that were missing or untested.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, match the plan verb ceilings, use the correct Play packages (re-verified), and are covered by tests re-run green in this session. Not a Fail. Pixel section is honest.

## Concrete gaps only

1. **Full Auto → preview → Open sheet smoke still open.**  
   Evidence states unit/flow coverage only; live sheet smoke not run. Companion path is green; product UI stop-line is not closed.

2. **Uber Eats and Resy Open not device-verified.**  
   Pixel notes: Eats + Resy **not** installed (`No activity found`). Uber + DoorDash launch resolve only. Claiming Open-button focus leave for Eats/Resy would be a Fail; evidence correctly does not.

3. **Dedicated wrong-verb Resolve rejects missing for Uber Eats `order` and Resy `book`.**  
   Uber `book` and DoorDash `send` are explicitly rejected in `TestDeepLinkRejectsEmptyDraftAndWrongVerb`. Uber Eats / Resy rely on Spec verb lists + shared `allows()` without a peer Resolve failure case. Ceiling is still correct in Specs; the test surface is thinner than Uber’s booking guard.

## Focus checklist

| Focus | Grade |
|---|---|
| hands_off / Consent A | Pass — `Describe()` sets HandsOff / ConsentA / AuthNone; execute uses `DraftOutcome`; adapter + flow tests assert hands_off / Done / cannot-know and ban booked/reserved/ordered/ride requested |
| correct Play packages | Pass — all four packages match live Play; Resy prod id correct (`com.resy.android` 404); DoorDash consumer not driver |
| verbs = plan ceilings | Pass — uber read; ubereats/resy read; doordash read+order; Uber book rejected; booking/API out of scope |
| no partner OAuth/API | Pass — no oauth adapters for these apps; Auth none; stage1 tests reject riders-api / oauth / partner-api coaching |
| ClassMap rides/food | Pass — runtime builds from Spec.AppClass; rides→uber; food includes ubereats/resy/doordash; flow tests route all four by `app_named` |
| HandOffActions | Pass — display names map to same packages; Android unit green |
| tests real | Pass — 39 Go filtered + HandOffActions 3/0 this session; red→green story in evidence matches live count |
| Pixel honesty | Pass (with gaps #1–#2) — installed vs missing stated; Auto→Open not invented |

## Evidence cross-check (not gaps unless they break the bar)

- Plan: Uber estimates RT-1 read hands_off; booking separate BD row. Pack ships RT-4 prepare-and-open under that ceiling — correct for “no partner credentials.”
- DoorDash checkout “unconfirmed” is recorded; `order` still hands_off via DraftOutcome — matches plan.
- `Wave1Specs` count 14 locked by adapter + flow tests.
- Sibling historical docs with adapter_count 9/10 are superseded; live count is 14.
