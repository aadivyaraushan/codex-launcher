# Wave 1 airlines + Citymapper prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-airlines-transit-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Airlines + Citymapper bullet)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`united`…`citymapper`), `runtime/deeplink` ClassMap + travel flow test, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Plan rows checked:** `planning/consumer-app-implementation-plan.md` — Transit/airlines RT-4 deep-link hands_off; later prose: airlines/transit cancel is deep-link manage-booking only (not a completes verb)  
**Tests / Play re-run (this session):**  
- `go test` adapters/deeplink + runtime/deeplink + stage1/openai → **120 passed**  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest` → **BUILD SUCCESSFUL**  
- Play Store HTTP: united/delta/`com.southwestairlines.mobile`/american/citymapper → **200**; wrong Southwest `com.southwestair.mobile` → **404**  

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-netflix-facebook-prepare-open-judge.md`, `wave1-travel-connectors-prepare-open-judge.md`. Overnight status may later link this path the same way it links other `*-judge.md` files; no code imports it.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-airlines-transit-prepare-open.md` — **present**, dated 2026-08-02. Overnight status cites Wave1Specs=38. Glob/Grep found **no** prior `wave1-airlines-transit-prepare-open-judge.md` (only the implementer evidence file).
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. Define strong from first principles, then grade. No implementer checklist.

## Task
Wave 1 airlines + Citymapper prepare-and-open (Wave1Specs→38). read only; never claim booked; Southwest package must be live Play id.

Worktree: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`

Inspect specs, HandOffActions, stage1, `saved-results/wave1-airlines-transit-prepare-open.md`. May run go tests.

Output Standard, Verdict, Findings, Gaps, next tip.
Write `saved-results/wave1-airlines-transit-prepare-open-judge.md` (2026-08-02). No product edits."

## First-principles bar (before hunting bugs)

A strong result for **this** task — add United, Delta, Southwest, American Airlines, and Citymapper as Wave 1 prepare-and-open hand-offs (append after Netflix/Facebook; `Wave1Specs` → **38**) — must have:

1. **Product shape** — Operator prepares intent text and opens the official consumer app. The user finishes flight-status / manage-booking / transit directions inside that app. Success is open-with-draft, not booking or check-in inside Operator.
2. **Read-only honesty** — No partner booking/check-in API in Wave 1. Specs allow **`read` only**. Resolve must **reject `book`** (and other non-read verbs). Outcomes and coaching must **never** claim booked, checked-in, or boarded.
3. **Correct Android packages, live on Play** — Especially Southwest: the package must be a Play Store id that returns live (HTTP 200), not a guessed dead id. All five packages must match the official consumer apps.
4. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for all five Specs.
5. **Routing class that exists** — `travel` (already used by Booking.com / Maps / travel connectors). No invented class.
6. **No fake completion route** — No airline/Citymapper OAuth or partner booking adapter for this Wave-1 surface.
7. **End-to-end wiring** — Spec → ClassMap (`travel`) → stage1 coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` travel log names all five.
8. **Real tests** — Tests that go red if count/packages/verbs/book-reject/empty-draft/flow route/outcome bans/stage1 coaching/HandOffActions break.
9. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**120** Go + HandOffActions BUILD SUCCESSFUL), all five Play packages HTTP **200** with wrong Southwest id **404**, and the evidence file is honest about Pixel. Not a Fail. Warnings are open live smoke, plan-table verb drift (`book` vs implemented `read`), thinner shared ban list vs dedicated flow bans, and American display-name alias gaps.

## Findings (against the bar)

- **Wave1Specs count locked at 38 with five travel/read Specs last.** Want table includes `united` / `delta` / `southwest` (`com.southwestairlines.mobile`) / `american` (`com.aa.android`) / `citymapper` (`com.citymapper.app.release`), all `travel` + `[read]` (`adapter_test.go`). Shared Describe sets RT4 / HandsOff / ConsentA / AuthNone. Every Wave1 Spec asserts `must not allow send`. Empty draft + `book` + `send` rejected per airline id at `Wave1Specs()[33+i]`.
- **Never claims booked / checked-in / boarded on the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know whether you finished”). Flow test `TestDeepLinkFlowRoutesTravelReadToWave1Connectors` bans `booked` / `reserved` / `purchased` / `navigated` / `saved` / `checked-in` / `checked in` / `boarded` for all five. Stage1 coaches travel/read for each and refuses booked/checked-in/boarded (`client.go` + stage1 test asserts for united…citymapper + booked/checked-in/boarded).
- **Southwest package is the live Play id.** Spec + HandOffActions + evidence use `com.southwestairlines.mobile`. This session: that id **200**; suggested-dead `com.southwestair.mobile` **404**. Comment in Spec documents the correction.
- **HandOffActions maps display names.** United/Delta/Southwest/Citymapper Title+lower; American Airlines / `american airlines` → `com.aa.android` (`HandOffActions.kt` + unit test). Wire uses Spec `AppName` (“American Airlines”), so Confirm→open path matches the map.
- **ClassMap / proof wiring.** Runtime builds `travel` ClassMap from Spec AppClass (no new key). `deeplink_proof.go` travel log includes `…+united+delta+southwest+american+citymapper`. No airline/Citymapper OAuth adapter packages added.
- **Verb choice matches the honesty bar better than the plan table.** Implementation plan row lists Transit/airlines verb `book`, but task + later plan prose (manage-booking deep link is floor, not a completes verb) and peer travel hand-offs (Booking.com / Tripadvisor) all use **`read`**. Specs use `read` and explicitly reject `book` — correct for “never claim booked.”
- **Evidence + overnight honesty match this session.** Evidence records red→green, count 38, Southwest correction, Pixel unpaired. Overnight bullet claims go **120/120** + HandOffActions green + Play 200 + Pixel unpaired — matches this session’s **120** Go passes + HandOffActions BUILD SUCCESSFUL + Play checks.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence: Pixel unpaired (Pair screen); companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **Shared execute ban list is thinner than the dedicated travel flow bans.** `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `booked` / `reserved` / `ride requested` but not `checked-in` / `boarded`. Dedicated travel flow covers those phrases today; a shared DraftOutcome wording regression that only said “checked-in” would not trip the shared table.
3. **Implementation-plan table still says verb `book` for Transit/airlines.** Code correctly uses `read` and rejects `book`. Doc drift can mislead the next implementer into “adding book” and weakening honesty. Not a product Fail for this task (task said read-only).
4. **American Airlines HandOffActions aliases are narrow.** Map keys are `"american airlines"` only — not `"american"` or `"aa"`. Happy path uses Spec AppName `American Airlines`; a UI or future caller that hands off the short stage1 id `american` as the display name would miss the package.
5. **ProvesCeiling strings are not asserted in the Wave1Specs want table.** Smoke ids exist on Specs (`united_prepare_open_smoke`, …); a silent rename would not fail the package/count test.
6. **Stage1 still teaches global `book` for “reserving a service, trip…”.** Airline-specific lines force `read`, but a model could still emit `book` for “book a United flight” and then hit Resolve rejection — correct ceiling, weaker coaching lock.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for United / Delta / Southwest / American Airlines / Citymapper (`serve-deeplink-proof`). Then: align the implementation-plan Transit/airlines verb cell to `read` (or document why `book` was rejected), add `checked-in` / `boarded` to the shared execute ban list (or assert them on the airline cases there), and optionally map HandOffActions aliases `american` / `aa` → `com.aa.android` if short names can reach openApp.
