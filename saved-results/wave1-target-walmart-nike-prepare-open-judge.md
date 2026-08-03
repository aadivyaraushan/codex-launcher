# Wave 1 Target / Walmart / Nike prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-target-walmart-nike-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Target/Walmart/Nike bullet); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=48, shopping pack named)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`target`/`walmart`/`nike`), `adapter_test.go`, `runtime/deeplink/flow.go` + `flow_test.go`, stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Live serve log:** `/tmp/shopping-serve-deeplink.log` (2026-08-02 08:54)  
**Tests / Play re-run (this judge session):**  
- `go test` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **187 passed in 5 packages**  
- Play Store HTTP: `com.target.ui` / `com.walmart.android` / `com.nike.omega` → **200**  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines: `adapter_count=48`, `shopping_adapters=3`, `shopping=target+walmart+nike`

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-threads-tiktok-expedia-prepare-open-judge.md`. No source file imports or reads this markdown.
2. **Existing peer evidence:** Glob for `wave1-target-walmart-nike-prepare-open-judge.md` → **0 matches** before this write. Task evidence MD exists separately; no prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; no structured data files read or written beyond inspection of code/logs.
4. **User instruction (verbatim):** "Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-target-walmart-nike-prepare-open-judge.md` with concrete gaps. Independently inspect code/tests/evidence; optionally run focused tests. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Target, Walmart, and Nike as Wave 1 shopping prepare-and-open hand-offs (`Wave1Specs` **45 → 48**); AppClass `shopping`; verb **read** only; no Sephora/Wayfair — must have:

1. **Three complete Specs with exact contracts** — IDs `target` / `walmart` / `nike`; AppClass `shopping`; verb `read` only; packages `com.target.ui` / `com.walmart.android` / `com.nike.omega`; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `*_prepare_open_smoke`. Total Wave1Specs count **48**, locked by test. Appended after Expedia.
2. **Honest product ceilings** — Browse/open only. Specs must reject order/book/send (and empty draft). Outcomes and stage1 coaching must never claim cart built, ordered, or checkout completed. No UCP cart API claims.
3. **Full-stack wiring** — Spec → dynamic ClassMap `shopping` → stage1 stable class list + per-app coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof log `shopping=…` and flow ready `shopping_adapters`. Missing any surface is a gap.
4. **Real tests that would go red on regression** — count=48; package/class/verb table; empty-draft + wrong-verb rejects; dedicated flow route tests with shopping completion-phrase bans; ready-log `shopping_adapters=3`; stage1 instruction asserts; HandOffActions package asserts.
5. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Play 200, Pixel unpaired if blocked; overnight bullet + progress snapshot show Wave1Specs=48. Inventing a device proof is a Fail on honesty.
6. **Scope discipline** — Sephora/Wayfair not added. No commit if that was the ask.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**187** passed across five packages), all three Play packages HTTP **200**, HandOffActions unit green, live serve log shows `adapter_count=48` + `shopping_adapters=3` + `shopping=target+walmart+nike`, ceilings appear in Specs/coaching/flow bans, evidence + overnight status/progress snapshot are present and honest about Pixel, Sephora/Wayfair absent, and HEAD was not advanced for this pack. Not a Fail. Warnings are open live smoke, thinner shared execute ban list vs dedicated shopping flow bans, ProvesCeiling string not locked in the Spec table test, and a soft stage1 assert that only requires one of cart/checkout/ordered.

## Findings (against the bar)

1. **Specs match the task table.** `adapter.go:136-140` — target/walmart/nike; packages `com.target.ui` / `com.walmart.android` / `com.nike.omega`; AppClass `shopping`; verbs read-only; ProvesCeiling `target_prepare_open_smoke` / `walmart_prepare_open_smoke` / `nike_prepare_open_smoke`. Indices 45–47 after expedia at 44. Count locked at `flow_test.go:93-94` (`want 48`). Describe() sets HandsOff / ConsentA / AuthNone / RT4 (`adapter.go:161-171`).

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Shopping rejects empty draft, order, book, and send (`adapter_test.go:672-700`). Dedicated flow bans `cart built` / `ordered` / `checkout completed` / `purchased` / `bought` and requires “cannot know” (`flow_test.go:1210-1218`). Stage1 coaches never-claim cart built / ordered / checkout completed (`client.go:109-111`). Execute uses shared `handoff.DraftOutcome` (`adapter.go:225-230` → `outcome.go:23-34`).

3. **Packages are real.** Judge curl: all three Play IDs **200**. No Sephora/Wayfair Specs (comment-only non-goal at `adapter.go:137`; overnight docs state the same exclusion).

4. **Full-stack wiring present.** Stage1 stable class list includes `shopping` (`client.go:68`); dedicated stage1 test (`client_test.go:811-849`); HandOffActions maps Target/Walmart/Nike (`HandOffActions.kt:62-64`) with unit asserts (`HandOffActionsTest.kt:97-102`); flow ready log `shopping_adapters` (`flow.go:69`); proof serve log `shopping=target+walmart+nike` (`deeplink_proof.go:44`); ClassMap built dynamically from Specs (`flow.go:46-56`).

5. **Serve evidence independently confirmed.** `/tmp/shopping-serve-deeplink.log` lines 2–3: `adapter_count=48`, `shopping_adapters=3`, `shopping=target+walmart+nike`. Process `/tmp/codex-launcher-deeplink serve-deeplink-proof` was live during this judge session.

6. **Evidence + overnight status match.** Evidence MD records 45→48, ceilings, wiring, red→green, Pixel unpaired. Overnight bullet at `wave1-overnight-batch-and-oauth-prep.md:208`. Progress snapshot: Wave1Specs=48, shopping pack listed (`wave1-overnight-progress-snapshot.md:18-20`).

7. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); shopping-related paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Allowed by task (“Pixel unpaired”); evidence is honest. Live package launch still open until re-pair.
2. **Shared execute ban list is thinner than dedicated shopping flow bans** — `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `ordered` (among others) but not `cart built` / `checkout completed` / `purchased` / `bought` (`adapter_test.go:220`). Dedicated shopping flow covers those phrases; a DraftOutcome regression that only said “cart built” would not trip the shared table (same soft gap pattern as prior prepare-and-open judges).
3. **ProvesCeiling string not asserted in Spec table test** — Values are set on Specs and passed through `Describe()` (`adapter.go:170`); the want-table test checks id/package/class/verbs/HandsOff but not the ProvesCeiling string itself (`adapter_test.go:81-114`).
4. **Stage1 shopping test OR-gates cart/checkout/ordered** — Coaching text includes all three refuse phrases (`client.go:109-111`), but `TestStage1InstructionsCoachShoppingPrepareAndOpen` only requires that *one* of `cart` / `checkout` / `ordered` appears (`client_test.go:847-849`). Flow + coaching still carry the full refuse language.

## What would flip this to Fail

- Wrong packages, or shopping Specs allowing order/compose-cart as if checkout completed.
- Outcomes or coaching claiming cart built / ordered / checkout completed without refuse language.
- Wave1Specs count ≠ 48, or missing HandOffActions / stage1 / flow / proof surface for any of the three.
- Sephora/Wayfair Specs added contrary to explicit non-goal.
- Evidence claiming Pixel Auto→Open success while unpaired.
- A commit made contrary to the no-commit ask.

## Reproduce

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe"

go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# judge session: 187 passed in 5 packages

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

for id in com.target.ui com.walmart.android com.nike.omega; do
  curl -s -o /dev/null -w "$id %{http_code}\n" "https://play.google.com/store/apps/details?id=$id"
done
# 200 / 200 / 200

# Live serve (if running): grep ready lines in /tmp/shopping-serve-deeplink.log
# adapter_count=48 shopping_adapters=3 shopping=target+walmart+nike
```
