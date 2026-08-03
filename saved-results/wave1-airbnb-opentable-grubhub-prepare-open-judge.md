# Wave 1 Airbnb / OpenTable / Grubhub prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-airbnb-opentable-grubhub-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Airbnb + OpenTable + Grubhub bullet)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`airbnb`/`opentable`/`grubhub`), `runtime/deeplink` ClassMap + flow tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Plan rows checked:** `planning/consumer-app-implementation-plan.md` — Airbnb / OpenTable / Grubhub listed as Wave 3 RT-4 hands_off with verbs `book` / `book,cancel,modify` / `order`  
**Tests / Play / binary re-run (this session):**  
- Same Go package set as evidence → **91** `--- PASS:` lines, 5 packages ok  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks` → **BUILD SUCCESSFUL**  
- Play Store HTTP: `com.airbnb.android` / `com.opentable` / `com.grubhub.android` → **200**; wrong ids `com.airbnb` / `com.opentable.android` / `com.grubhub` → **404**  
- `/tmp/codex-launcher-deeplink` (mtime 2026-08-02 08:34) embeds `com.airbnb.android`, `com.opentable`, `com.grubhub.android`, travel `…+airbnb`, food_extra `…+opentable+grubhub`, and stage1 never-claim coaching strings  

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-airlines-transit-prepare-open-judge.md`, `wave1-netflix-facebook-prepare-open-judge.md`.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-airbnb-opentable-grubhub-prepare-open.md` — **present**, dated 2026-08-02. No prior judge file for this pack before this write.
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** adversarial LLM-as-judge with fresh context; define strong from first principles then grade; Pass / Pass-with-warnings / Fail; write this path; no questions; no commit; no secrets.

## First-principles bar (before hunting bugs)

A strong result for **this** task — add Airbnb, OpenTable, and Grubhub as Wave 1 prepare-and-open hand-offs (append after YouTube; `Wave1Specs` **39 → 42**) — must have:

1. **Product shape** — Operator prepares intent text and opens the official consumer app. The user finishes search / reservation browse / cart inside that app. Success is open-with-draft, not booked / reserved / checkout completed inside Operator.
2. **Honest verb demotion** — Wave 3 plan rows list Airbnb/OpenTable `book` and Grubhub `order`. Wave 1 overnight may demote Airbnb/OpenTable to **`read`** (peers: Booking.com / Resy) and keep Grubhub **`read` + `order`** as browse/cart intent only (peer: DoorDash). Resolve must reject verbs outside that set (especially `book` / `send` where not allowed). Outcomes and coaching must **never** claim booked, reservation booked, ordered-as-done, or checkout completed.
3. **Correct Android packages, live on Play** — Specs and `HandOffActions` must use Play Store ids that return live HTTP 200, not guessed dead ids.
4. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for all three Specs. No OAuth / partner booking adapter for this Wave-1 surface.
5. **Routing classes that exist** — Airbnb → `travel`; OpenTable/Grubhub → `food`. No invented class.
6. **End-to-end wiring** — Spec → ClassMap → stage1 coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` logs (`adapter_count=42`, travel includes `airbnb`, food_extra includes `opentable`+`grubhub`).
7. **Real tests** — Tests that go red if count/packages/verbs/book-or-send reject/empty-draft/flow route/outcome bans/stage1 coaching/HandOffActions break.
8. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.
9. **No commit** — work stays uncommitted if that was the ask.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**91** Go PASS lines on the evidence package set + HandOffActions BUILD SUCCESSFUL), all three Play packages HTTP **200** with wrong-id **404** controls, serve binary embeds the new ids/log strings, evidence is honest about Pixel, and HEAD was not advanced for this pack. Not a Fail. Warnings are open live smoke, plan-table verb/wave drift, thinner shared ban list vs dedicated flow bans, ProvesCeiling / package-header hygiene, and Grubhub’s thinner explicit `book`-reject assert relative to Airbnb/OpenTable.

## Findings (against the bar)

- **Wave1Specs count locked at 42 with three Specs last.** Want table includes `airbnb` (`com.airbnb.android`, travel, `[read]`), `opentable` (`com.opentable`, food, `[read]`), `grubhub` (`com.grubhub.android`, food, `[read, order]`) (`adapter_test.go`). Shared Describe sets RT4 / HandsOff / ConsentA / AuthNone. Every Wave1 Spec asserts `must not allow send`. Empty draft + `book` + `send` rejected for Airbnb/OpenTable at `Wave1Specs()[39+i]`; Grubhub empty/`send` at `[41]`. Runtime `flow_test.go` also locks `len(Wave1Specs()) == 42`.
- **Never claims booked / reservation booked / checkout completed on the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know whether you finished”). Flow tests ban `booked`/`reserved`/`purchased` (Airbnb), `booked`/`reserved`/`ordered` (OpenTable), `ordered`/`checkout completed`/`purchased` (Grubhub read+order). Stage1 coaches travel/read for Airbnb, food/read for OpenTable, food read|order for Grubhub with matching never-claim lines (`client.go` + stage1 tests).
- **Play packages are the live ids.** Spec + HandOffActions + evidence use the three ids above. This session: all **200**; alternate wrong ids **404**. `/tmp/codex-launcher-deeplink` embeds the live packages and proof travel/food_extra strings.
- **HandOffActions maps display names.** `Airbnb`/`airbnb`, `OpenTable`/`opentable`, `Grubhub`/`grubhub` → matching packages (`HandOffActions.kt` + unit test). Wire uses Spec `AppName`, so Confirm→open happy path matches.
- **ClassMap / proof wiring.** Runtime builds travel/food ClassMap from Spec AppClass (no new key). `deeplink_proof.go` logs `adapter_count` via `len(Wave1Specs())`, travel `…+airbnb`, food_extra `…+opentable+grubhub`. No Airbnb/OpenTable/Grubhub OAuth adapter packages.
- **Verb demotion matches the honesty bar better than the Wave-3 plan cells.** Implementation plan still lists Airbnb/OpenTable `book` and Grubhub `order` at Wave 3. Task + overnight rule demote book→read for Airbnb/OpenTable and treat Grubhub order as cart intent only — consistent with Booking.com / Resy / DoorDash peers. Specs reject out-of-set verbs via `allows()`.
- **Evidence + overnight honesty match this session.** Evidence records red→green narrative, count 42, Play 200, Pixel unpaired. Overnight bullet claims go **91 PASS** + HandOffActions green + Play 200 + Pixel unpaired — matches this session’s **91** `--- PASS:` count + HandOffActions BUILD SUCCESSFUL + Play checks. HEAD remains `5cb0831` (Wave 0); pack files still dirty/untracked — **no commit**, as claimed. TDD red chronology is implementer-reported (not independently time-stamped here); green locks are verified now.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence: Pixel unpaired (Pair screen); companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **Shared execute ban list is thinner than dedicated flow bans.** `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `booked` / `reserved` / `ordered` but not `checkout completed`. Dedicated Grubhub flow covers that phrase today; a shared DraftOutcome wording regression that only said “checkout completed” would not trip the shared table.
3. **Implementation-plan tables still say Wave 3 + `book` (Airbnb/OpenTable) and `order` (Grubhub).** Code correctly uses Wave-1 demoted verbs and rejects `book` on Airbnb/OpenTable. Doc drift can mislead the next implementer into “adding book” and weakening honesty. Not a product Fail for this overnight pack (task asked for demotion).
4. **Grubhub has no dedicated `book`-reject assert** (only empty + `send`). `allows()` still rejects `book` because it is not in Spec verbs; Airbnb/OpenTable get explicit book asserts. Weaker lock if someone later adds `book` to Grubhub without noticing.
5. **ProvesCeiling strings are not asserted in the Wave1Specs want table.** Smoke ids exist on Specs (`airbnb_search_prepare_open_smoke`, `opentable_prepare_open_smoke`, `grubhub_prepare_open_smoke`); a silent rename would not fail the package/count test. Binary strings confirm they are present in the rebuilt proof binary.
6. **Package file header comment is stale.** `adapter.go` package comment still lists through Citymapper / earlier packs and does not name YouTube / Airbnb / OpenTable / Grubhub, even though Specs are present.
7. **Stage1 still teaches global `book` for “reserving a service, trip…”.** App-specific lines force `read` / cart-intent `order`, but a model could still emit `book` for “book an Airbnb” and then hit Resolve rejection — correct ceiling, weaker coaching lock.
8. **Serve live log line not re-captured this session.** Binary + source prove the log fields; a fresh `serve-deeplink-proof` stdout capture was not re-run here (would need API key / process). Independent strength is code+binary, not a new log file.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for Airbnb / OpenTable / Grubhub (`serve-deeplink-proof`). Then: add `checkout completed` to the shared execute ban list (or assert it on the Grubhub shared cases), add an explicit Grubhub `book`-reject assert next to Airbnb/OpenTable, refresh the `adapter.go` package header list, and note Wave-1 demotion (`read` / cart-intent `order`) next to the Wave-3 plan rows so the next implementer does not “restore book.”
