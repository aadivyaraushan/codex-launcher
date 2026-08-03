# Wave 1 Priceline / LinkedIn / eBay prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-priceline-linkedin-ebay-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Priceline/LinkedIn/eBay bullet); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=54; Priceline/LinkedIn/eBay named; Reddit/Pinterest excluded)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`priceline`/`linkedin`/`ebay`), `adapter_test.go`, `runtime/deeplink/flow.go` + `flow_test.go`, stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Live serve:** `/tmp/priceline-linkedin-ebay-serve.log` (2026-08-02 09:24); binary `/tmp/codex-launcher-deeplink` mtime 09:24 (process not live at judge time)  
**Tests / Play re-run (this judge session):**  
- `go test` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **5 packages ok** (session count **202** passed)  
- Play Store HTTP: `com.priceline.android.negotiator` / `com.linkedin.android` / `com.ebay.mobile` → **200**; `com.priceline.android.hybrid` → **404** (unused; prior pack note only)  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines: `adapter_count=54`, `shopping_adapters=6`, `travel_adapters=15`, `messaging_adapters=10`, travel `…+kayak+priceline`, messaging `…+tiktok+linkedin`, shopping `…+wayfair+ebay`

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-sephora-wayfair-kayak-prepare-open-judge.md`. Overnight status bullet at `wave1-overnight-batch-and-oauth-prep.md:210` references this path by name after the companion edit. No source file imports this markdown.
2. **Existing peer evidence:** Glob for `wave1-priceline-linkedin-ebay*` found only the task evidence MD (`wave1-priceline-linkedin-ebay-prepare-open.md`). Grep for `wave1-priceline-linkedin-ebay-prepare-open-judge` returned no matches before this write. No prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; inspection only of code/logs/terminals. No production data fields; date stamped `2026-08-02`.
4. **User instruction (verbatim):** "Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-priceline-linkedin-ebay-prepare-open-judge.md`. Independently inspect code/tests/evidence. Optionally run focused tests. Ensure overnight status bullet mentions judge file. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Priceline, LinkedIn, and eBay as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **51 → 54**); Priceline AppClass `travel` verb **read** (book demoted); LinkedIn AppClass `messaging` verb **compose**; eBay AppClass `shopping` verb **read** only; no Reddit / Pinterest — must have:

1. **Three complete Specs with exact contracts** — IDs `priceline` / `linkedin` / `ebay`; packages `com.priceline.android.negotiator` / `com.linkedin.android` / `com.ebay.mobile`; travel/read, messaging/compose, shopping/read; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `priceline_search_prepare_open_smoke` / `linkedin_prepare_open_smoke` / `ebay_browse_prepare_open_smoke`. Total Wave1Specs count **54**, locked by test. Appended after Kayak (travel), then LinkedIn, then eBay.
2. **Honest product ceilings** — Search/prepare-and-open (Priceline), compose-only (LinkedIn; Facebook/Threads peer), browse/open (eBay). Specs must reject empty draft and wrong verbs (book/send for Priceline; send for LinkedIn; order/book/send for eBay). Outcomes and stage1 coaching must never claim booked (Priceline), posted/commented (LinkedIn), or cart built / bid placed / checkout completed (eBay). No Reddit or Pinterest Specs.
3. **Full-stack wiring** — Spec → dynamic ClassMap (`travel` / `messaging` / `shopping`) → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof log travel `+priceline`, messaging `+linkedin`, shopping `+ebay`, and flow ready `travel_adapters=15` / `messaging_adapters=10` / `shopping_adapters=6` / `adapter_count=54`. Missing any surface is a gap.
4. **Real tests that would go red on regression** — count=54; package/class/verb table including the three; empty-draft + wrong-verb rejects; dedicated flow route tests with completion-phrase bans; ready-log counts; stage1 instruction asserts for each app; HandOffActions package asserts.
5. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Play 200, Pixel blocked if blocked; overnight bullet + progress snapshot show Wave1Specs=54. Inventing a device proof is a Fail on honesty.
6. **Scope discipline** — No Reddit / Pinterest Specs. No commit if that was the ask. Prefer the Play-200 Priceline package (`negotiator`), not the prior 404 hybrid id.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**202** passed across five packages), all three Play packages HTTP **200**, HandOffActions unit green, serve log shows `adapter_count=54` + class counts + proof strings for all three apps, ceilings appear in Specs/coaching/dedicated flow bans, Reddit/Pinterest stay out of Specs, evidence + overnight status/progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack. Not a Fail. Warnings are open live smoke, thinner shared execute ban list vs dedicated flow bans, ProvesCeiling string not locked in the Spec table test, and a soft stage1 shopping assert that only requires one of cart/checkout/ordered (bid refuse is in eBay coaching text but not separately locked).

## Findings (against the bar)

1. **Specs match the task table.** `adapter.go:150-154` — priceline/linkedin/ebay; packages `com.priceline.android.negotiator` / `com.linkedin.android` / `com.ebay.mobile`; AppClass travel/messaging/shopping; verbs read/compose/read; ProvesCeiling strings as claimed. Indices 51–53 after kayak at 50. Independent Spec ID count: **54**; last six `sephora…ebay`. ClassMap sizes match serve: travel=15, messaging=10, shopping=6. Count locked at `flow_test.go:95-96` (`want 54`) and want-table length in `adapter_test.go`.

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Priceline rejects empty/book/send (`adapter_test.go:735-753`). LinkedIn rejects empty/send (`adapter_test.go:755-767`). eBay rejects empty/order/book/send (`adapter_test.go:769-793`). Dedicated flows ban booked/reserved/purchased/ticketed (Priceline), posted/commented/published/sent (LinkedIn), and cart built/ordered/checkout completed/purchased/bought/bid placed (shopping including eBay) (`flow_test.go:1173-1380`). Stage1 coaches never-claim booked (Priceline), posted/commented (LinkedIn), cart/bid/checkout (eBay) (`client.go:115-118`). Execute uses shared `handoff.DraftOutcome` (“cannot know”).

3. **Packages are real; scope held.** Judge curl: negotiator/linkedin/ebay **200**. Hybrid `com.priceline.android.hybrid` → **404** and unused. Companion grep for Reddit/Pinterest Spec IDs: **none**.

4. **Full-stack wiring present.** Dedicated stage1 tests for Priceline and LinkedIn (`client_test.go:887-951`); shopping stage1 list includes `ebay` + `app_named ebay` (`client_test.go:835-837`); HandOffActions maps all three ids (`HandOffActions.kt:69-71`) with unit asserts for display + lowercase (`HandOffActionsTest.kt:111-116`); flow ready log locks shopping=6 / travel=15 / messaging=10 (`flow_test.go:1229-1260`); proof serve strings in `deeplink_proof.go:38-45`.

5. **Serve evidence independently confirmed.** `/tmp/priceline-linkedin-ebay-serve.log` lines 2–3 (09:24:11): `adapter_count=54`, `shopping_adapters=6`, `travel_adapters=15`, `messaging_adapters=10`, travel includes `+priceline`, messaging includes `+linkedin`, shopping includes `+ebay`. Binary mtime matches; process was not still running at judge time (log file is the live-serve artifact).

6. **Evidence + overnight status match.** Evidence MD records 51→54, ceilings, wiring, red→green (now **202 PASS**), Pixel on Pair blocked, Reddit/Pinterest non-goals. Progress snapshot: Wave1Specs=54, Priceline/LinkedIn/eBay named, Reddit/Pinterest excluded. Overnight bullet updated to cite this judge file.

7. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Allowed by task (“Pixel on Pair — skip device smoke”); evidence is honest. Live package launch still open until re-pair.
2. **Shared execute ban list is thinner than dedicated flow bans** — `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `ordered` / `booked` / `sent` (among others) but not `cart built` / `checkout completed` / `bid placed` / `purchased` / `bought` / `ticketed` / `posted` / `commented` (`adapter_test.go:234`). Dedicated Priceline/LinkedIn/shopping flows cover those phrases; a DraftOutcome regression that only said “bid placed” or “posted” would not trip the shared table (same soft gap pattern as prior prepare-and-open judges).
3. **ProvesCeiling string not asserted in Spec table test** — Values are set on Specs; the want-table test checks id/package/class/verbs/HandsOff but not the ProvesCeiling string itself.
4. **Stage1 shopping test OR-gates cart/checkout/ordered** — eBay coaching text includes cart / bid placed / checkout (`client.go:115`), but `TestStage1InstructionsCoachShoppingPrepareAndOpen` only requires that *one* of `cart` / `checkout` / `ordered` appears (`client_test.go:849-851`). Priceline and LinkedIn stage1 asserts are stricter (`never claim` + booked / posted|commented). Flow + coaching still carry the full refuse language.
5. **TDD red phase not independently re-proven** — Evidence documents red failures before Specs; this judge session only re-ran green. Contracts are locked by tests that would fail without the Specs; historical red is trusted from evidence, not re-observed.

## What would flip this to Fail

- Wrong packages, or Specs allowing book/send/order as if booked/posted/cart/bid/checkout completed.
- Outcomes or coaching claiming booked / posted / commented / cart built / bid placed / checkout without refuse language.
- Wave1Specs count ≠ 54, or missing HandOffActions / stage1 / flow / proof surface for any of the three.
- Reddit or Pinterest Specs added contrary to explicit non-goal.
- Evidence claiming Pixel Auto→Open success while Pair-blocked.
- A commit made contrary to the no-commit ask.

## Reproduce

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe"

go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# judge session: 202 passed in 5 packages

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

for id in com.priceline.android.negotiator com.linkedin.android com.ebay.mobile; do
  curl -s -o /dev/null -w "$id %{http_code}\n" "https://play.google.com/store/apps/details?id=$id"
done
# 200 / 200 / 200

# Serve ready (from /tmp/priceline-linkedin-ebay-serve.log if present):
# adapter_count=54 shopping_adapters=6 travel_adapters=15 messaging_adapters=10
# travel=…+kayak+priceline messaging=…+linkedin shopping=…+ebay
```
