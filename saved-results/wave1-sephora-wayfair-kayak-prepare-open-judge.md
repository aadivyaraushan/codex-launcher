# Wave 1 Sephora / Wayfair / Kayak prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-sephora-wayfair-kayak-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Sephora/Wayfair/Kayak bullet already cites this judge); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=51, shopping+Kayak named; Hotels.com/Priceline excluded)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`sephora`/`wayfair`/`kayak`), `adapter_test.go`, `runtime/deeplink/flow.go` + `flow_test.go`, stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Live serve:** terminal `108855.txt` (2026-08-02 09:08) + process `/tmp/codex-launcher-deeplink serve-deeplink-proof`  
**Tests / Play re-run (this judge session):**  
- `go test` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **194 passed in 5 packages**  
- Play Store HTTP: `com.sephora` / `com.wayfair.wayfair` / `com.kayak.android` → **200**; `com.hotels.android` → **404** (unused); `com.priceline.android.hybrid` → **404** (unused; not in Specs)  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines: `adapter_count=51`, `shopping_adapters=5`, `shopping=target+walmart+nike+sephora+wayfair`, travel `…+expedia+kayak`

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-target-walmart-nike-prepare-open-judge.md`. Overnight status bullet references this path by name. No source file imports this markdown.
2. **Existing peer evidence:** A thin stub judge already existed from a parallel agent; this write replaces it with a full first-principles judgment matching peer depth. Task evidence MD exists separately.
3. **Data I/O:** None — static markdown verdict; inspection only of code/logs/terminals.
4. **User instruction (verbatim):** "Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-sephora-wayfair-kayak-prepare-open-judge.md`. Independently inspect code/tests/evidence. Optionally run focused tests. Append one-line judge note to overnight status bullet for this pack in `saved-results/wave1-overnight-batch-and-oauth-prep.md` if missing. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Sephora, Wayfair, and Kayak as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **48 → 51**); Sephora/Wayfair AppClass `shopping` verb **read** only; Kayak AppClass `travel` verb **read** (book demoted); no Hotels.com / Priceline — must have:

1. **Three complete Specs with exact contracts** — IDs `sephora` / `wayfair` / `kayak`; packages `com.sephora` / `com.wayfair.wayfair` / `com.kayak.android`; shopping read-only for the first two; travel read-only for Kayak; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `sephora_prepare_open_smoke` / `wayfair_prepare_open_smoke` / `kayak_search_prepare_open_smoke`. Total Wave1Specs count **51**, locked by test. Appended after Nike (shopping) then Kayak last.
2. **Honest product ceilings** — Browse/open (shopping) and search/prepare-and-open (Kayak) only. Specs must reject order/book/send (and empty draft). Outcomes and stage1 coaching must never claim cart built / ordered / checkout completed (shopping) or booked (Kayak). No UCP cart API claims; no Hotels.com packages; no Priceline Spec.
3. **Full-stack wiring** — Spec → dynamic ClassMap (`shopping` / `travel`) → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof log `shopping=…+sephora+wayfair` and travel `+kayak` and flow ready `shopping_adapters=5` / `adapter_count=51`. Missing any surface is a gap.
4. **Real tests that would go red on regression** — count=51; package/class/verb table including the three; empty-draft + wrong-verb rejects for shopping quintet + Kayak; dedicated flow route tests with completion-phrase bans; ready-log `shopping_adapters=5`; stage1 instruction asserts for Sephora/Wayfair/Kayak; HandOffActions package asserts.
5. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Play 200, Pixel unpaired if blocked; overnight bullet + progress snapshot show Wave1Specs=51. Inventing a device proof is a Fail on honesty.
6. **Scope discipline** — Hotels.com not used (404); Priceline not added. No commit if that was the ask.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**194** passed across five packages), all three Play packages HTTP **200**, HandOffActions unit green, live serve log shows `adapter_count=51` + `shopping_adapters=5` + `shopping=target+walmart+nike+sephora+wayfair` + travel `+kayak`, ceilings appear in Specs/coaching/flow bans, Hotels.com/Priceline stay out of Specs, evidence + overnight status/progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack. Not a Fail. Warnings are open live smoke, thinner shared execute ban list vs dedicated shopping/Kayak flow bans, ProvesCeiling string not locked in the Spec table test, and a soft stage1 shopping assert that only requires one of cart/checkout/ordered.

## Findings (against the bar)

1. **Specs match the task table.** `adapter.go:141-147` — sephora/wayfair/kayak; packages `com.sephora` / `com.wayfair.wayfair` / `com.kayak.android`; AppClass shopping/shopping/travel; verbs read-only; ProvesCeiling `sephora_prepare_open_smoke` / `wayfair_prepare_open_smoke` / `kayak_search_prepare_open_smoke`. Indices 48–49 after nike at 47; kayak at 50. Count locked at `flow_test.go:93-94` (`want 51`) and want-table length in `adapter_test.go`. Session count of Spec IDs: **51**; last six `target…kayak`. Describe() sets HandsOff / ConsentA / AuthNone / RT4.

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Shopping loop rejects empty draft, order, book, and send for target→wayfair (`adapter_test.go:678-706`). Kayak rejects empty/book/send (`adapter_test.go:707-726`). Dedicated shopping flow bans `cart built` / `ordered` / `checkout completed` / `purchased` / `bought` and requires “cannot know” (`flow_test.go:1214-1221`). Kayak flow bans `booked` / `reserved` / `purchased` / `ticketed` (`flow_test.go:1280-1287`). Stage1 coaches never-claim cart/order/checkout for Sephora/Wayfair and never claim booked for Kayak (`client.go:112-114`). Execute uses shared `handoff.DraftOutcome`.

3. **Packages are real; scope held.** Judge curl: Sephora/Wayfair/Kayak **200**. Hotels.com `com.hotels.android` → **404** (unused; comment at `adapter.go:146`). Priceline `com.priceline.android.hybrid` → **404**; no `priceline`/`hotels` Spec ID in Wave1Specs (companion `*.go` grep — only the non-goal comment).

4. **Full-stack wiring present.** Stage1 shopping test requires sephora/wayfair + app_named lines (`client_test.go:833-835`); dedicated Kayak stage1 test (`client_test.go:852-882`); HandOffActions maps Sephora/Wayfair/Kayak (`HandOffActions.kt:65-67`) with unit asserts (`HandOffActionsTest.kt:103-108`); flow ready log `shopping_adapters` (`flow.go:69`) locked at 5 (`flow_test.go:1226-1247`); proof serve log shopping+travel strings (`deeplink_proof.go:41-44`); ClassMap built dynamically from Specs (`flow.go`).

5. **Serve evidence independently confirmed.** Terminal `108855.txt` lines 11–12 (09:08:37): `adapter_count=51`, `shopping_adapters=5`, `shopping=target+walmart+nike+sephora+wayfair`, travel includes `+kayak`. Binary `/tmp/codex-launcher-deeplink` (mtime 09:08) embeds those strings; process was live during this judge session.

6. **Evidence + overnight status match.** Evidence MD records 48→51, ceilings, wiring, red→green (now **194 PASS**), Pixel unpaired, Hotels.com/Priceline non-goals. Overnight bullet at `wave1-overnight-batch-and-oauth-prep.md:209` already includes judge note. Progress snapshot: Wave1Specs=51, Sephora/Wayfair/Kayak named, Hotels.com/Priceline excluded (`wave1-overnight-progress-snapshot.md:18-20`).

7. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Allowed by task (“Pixel unpaired”); evidence is honest. Live package launch still open until re-pair.
2. **Shared execute ban list is thinner than dedicated flow bans** — `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `ordered` / `booked` (among others) but not `cart built` / `checkout completed` / `purchased` / `bought` / `ticketed` (`adapter_test.go:226`). Dedicated shopping + Kayak flows cover those phrases; a DraftOutcome regression that only said “cart built” or “ticketed” would not trip the shared table (same soft gap pattern as prior prepare-and-open judges).
3. **ProvesCeiling string not asserted in Spec table test** — Values are set on Specs and passed through `Describe()`; the want-table test checks id/package/class/verbs/HandsOff but not the ProvesCeiling string itself (`adapter_test.go:84-114`).
4. **Stage1 shopping test OR-gates cart/checkout/ordered** — Coaching text includes all three refuse phrases for Sephora/Wayfair (`client.go:112-113`), but `TestStage1InstructionsCoachShoppingPrepareAndOpen` only requires that *one* of `cart` / `checkout` / `ordered` appears (`client_test.go:847-849`). Kayak stage1 assert is stricter (`never claim` + `booked`). Flow + coaching still carry the full refuse language.

## What would flip this to Fail

- Wrong packages, or shopping Specs allowing order as if checkout completed / Kayak accepting book as if booked.
- Outcomes or coaching claiming cart built / ordered / checkout completed / booked without refuse language.
- Wave1Specs count ≠ 51, or missing HandOffActions / stage1 / flow / proof surface for any of the three.
- Hotels.com or Priceline Specs added contrary to explicit non-goal.
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
# judge session: 194 passed in 5 packages

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

for id in com.sephora com.wayfair.wayfair com.kayak.android; do
  curl -s -o /dev/null -w "$id %{http_code}\n" "https://play.google.com/store/apps/details?id=$id"
done
# 200 / 200 / 200

# Live serve ready (if running): terminals/*/ or process stdout
# adapter_count=51 shopping_adapters=5 shopping=…+sephora+wayfair travel=…+kayak
```
