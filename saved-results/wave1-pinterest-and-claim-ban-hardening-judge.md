# Wave 1 Pinterest + claim-ban hardening — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-pinterest-and-claim-ban-hardening.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Pinterest + claim-ban hardening bullet); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=55; Pinterest named; ProvesCeiling + shared bans noted; no Reddit)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`pinterest`), `adapter_test.go` (want table + ProvesCeiling + shared ban list + empty/send reject), `runtime/deeplink/flow.go` + `flow_test.go` (count 55, messaging=11, Pinterest route), stage1 OpenAI `client.go` + `client_test.go` (Pinterest + shopping AND + LinkedIn AND), `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Live serve:** `/tmp/pinterest-serve.log` (2026-08-02 09:40); process live at judge time (`/tmp/codex-launcher-deeplink serve-deeplink-proof`)  
**Tests / Play re-run (this judge session):**  
- `go test` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **5 packages ok** (session count **205** passed)  
- Play Store HTTP: `com.pinterest` → **200**  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines: `adapter_count=55`, `messaging_adapters=11`, messaging `…+linkedin+pinterest`  
- Historical red artifact `/tmp/pinterest-red.txt` spot-checked (count 54→55, unknown adapter, messaging_adapters=10, stage1 missing pinterest)

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-priceline-linkedin-ebay-prepare-open-judge.md`. Overnight status bullet at `wave1-overnight-batch-and-oauth-prep.md:211` references this path by name after the companion edit. No source file imports this markdown.
2. **Existing peer evidence:** Glob for `wave1-pinterest-and-claim-ban-hardening*` found only the task evidence MD. Grep for `wave1-pinterest-and-claim-ban-hardening-judge` returned no matches before this write. No prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; inspection only of code/logs/terminals. No production data fields; date stamped `2026-08-02`.
4. **User instruction (verbatim):** "Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-pinterest-and-claim-ban-hardening-judge.md`. Independently inspect. Cite overnight status with judge file. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Pinterest as Wave 1 prepare-and-open hand-off (`Wave1Specs` **54 → 55**) **and** close the recurring judge gaps from the prior pack (shared execute ban list, Spec ProvesCeiling asserts, stage1 shopping soft OR) — must have:

1. **One complete Spec with exact contract** — ID `pinterest`; package `com.pinterest`; AppClass `messaging`; verb `compose` only; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `pinterest_prepare_open_smoke`. Total Wave1Specs count **55**, locked by test. Appended after eBay (index 54).
2. **Honest product ceiling** — Messaging/compose prepare-and-open (Facebook/Threads/LinkedIn peer). Spec must reject empty draft and wrong verb (`send`). Outcomes and stage1 coaching must never claim pinned / posted / saved. No OAuth. No Reddit Spec (still out of scope).
3. **Full-stack wiring** — Spec → dynamic ClassMap (`messaging` → 11) → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof log messaging `+pinterest` and flow ready `messaging_adapters=11` / `adapter_count=55`. Missing any surface is a gap.
4. **Hardening that actually locks prior soft gaps** — (a) shared execute ban list in `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` must cover the completion phrases prior judges flagged (`cart built`, `checkout completed`, `purchased`, `bought`, `bid placed`, `posted`, `commented`, `watched`, `ticketed`, `pinned`, plus prior bans); (b) Spec table test must assert `ProvesCeiling` for **every** want row including pinterest; (c) stage1 shopping refuse must require **all** of cart + checkout + ordered (AND, not soft OR). Soft LinkedIn posted/commented OR tightened the same way is in-scope if present.
5. **Real tests that would go red on regression** — count=55; package/class/verb + ProvesCeiling table including pinterest; empty-draft + send reject; dedicated flow route with completion-phrase bans; ready-log messaging=11; stage1 instruction asserts for pinterest + shopping AND; HandOffActions package asserts.
6. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Play 200, hardening; overnight bullet + progress snapshot show Wave1Specs=55 and cite this judge file. Inventing a device proof is a Fail on honesty.
7. **Scope discipline** — No commit if that was the ask. Serve restarted with `adapter_count=55`.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**205** passed across five packages), Play package HTTP **200**, HandOffActions unit green (`tests=3 failures=0`), live serve shows `adapter_count=55` + `messaging_adapters=11` + proof string `…+linkedin+pinterest`, ceilings appear in Spec/coaching/dedicated flow/shared ban expand, prior-pack soft gaps (ProvesCeiling assert, shopping AND, shared ban expand) are closed in tests, evidence + overnight/progress snapshot are present, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open, residual shared-ban tokens still thinner than dedicated flow for `saved`/`published`, shopping AND still not locking `bid` as its own refuse token, and evidence’s “107 PASS lines” undercount vs this session’s 205.

## Findings (against the bar)

1. **Spec matches the task table.** `adapter.go:157-158` — pinterest; package `com.pinterest`; AppClass messaging; verb compose; ProvesCeiling `pinterest_prepare_open_smoke`. Index 54 after ebay at 53. Independent Spec ID count: **55**. ClassMap messaging size matches serve: messaging=11. Count locked at `flow_test.go:95-96` (`want 55`) and want-table length in `adapter_test.go`.

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Pinterest rejects empty/send (`adapter_test.go:811-823`). Dedicated flow bans pinned/posted/saved/published/sent (`flow_test.go:1413-1416`). Stage1 coaches never-claim pinned/posted/saved (`client.go:119`; `client_test.go:994-998`). Execute uses shared `handoff.DraftOutcome` (“cannot know”).

3. **Hardening closes the named prior gaps.** Shared ban list expanded to include `cart built` / `checkout completed` / `purchased` / `bought` / `bid placed` / `posted` / `commented` / `watched` / `ticketed` / `pinned` (`adapter_test.go:244-249`). Spec table asserts `m.ProvesCeiling == want[i].proves` for every row (`adapter_test.go:113-115`). Shopping stage1 requires each of `cart`, `checkout`, `ordered` (`client_test.go:849-855`). LinkedIn refuse tokens also AND (`client_test.go:955-960`).

4. **Package is real; scope held.** Judge curl: `com.pinterest` → **200**. Companion Spec IDs: pinterest present; Reddit Spec ID still absent from Wave1Specs (progress snapshot: “No Reddit in this pack”).

5. **Full-stack wiring present.** Stage1 dedicated Pinterest test (`client_test.go:963-999`); HandOffActions maps `pinterest`/`Pinterest` (`HandOffActions.kt:73`; asserts `HandOffActionsTest.kt:119-120`); flow ready log locks messaging=11 (`flow_test.go:1256-1258`); proof serve string includes `+pinterest` (`deeplink_proof.go:38`).

6. **Serve evidence independently confirmed.** `/tmp/pinterest-serve.log` lines 2–3 (09:40:48): `adapter_count=55`, `messaging_adapters=11`, messaging includes `+linkedin+pinterest`. Process still running at judge time.

7. **Evidence + overnight status match.** Evidence MD records 54→55, ceilings, hardening, red→green, serve 55. Progress snapshot: Wave1Specs=55, Pinterest named, claim-ban/ProvesCeiling hardening noted. Overnight bullet updated to cite this judge file. Historical `/tmp/pinterest-red.txt` shows the claimed red failures (count 54, unknown adapter, messaging_adapters=10, stage1 missing pinterest).

8. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Overnight status still lists Pixel unpaired / Pair screen. Evidence does not invent a device proof. Live package launch still open until re-pair.
2. **Shared execute ban list still misses some dedicated-flow tokens** — Expanded list includes `pinned` but not `saved` or `published` (dedicated Pinterest/LinkedIn flows ban those). A DraftOutcome regression that only said “saved” or “published” would not trip the shared table. DraftOutcome text today is clean (“cannot know”).
3. **Stage1 shopping AND still omits `bid` as a locked refuse token** — Prior judge noted bid refuse lives in eBay coaching but is not separately asserted; this pack ANDed cart/checkout/ordered and left `bid` unlocked. eBay coaching still contains “bid placed” (`client.go:115`).
4. **Evidence PASS-line count understates the suite** — Evidence claims “107 PASS lines from `-v`” (18+4+37+36+12). This judge session re-ran the same five packages and got **205** passed. Packages are green either way; the written count is soft/wrong as a ledger of test cases.
5. **TDD red phase partially independently confirmed** — `/tmp/pinterest-red.txt` matches the claimed reds; this judge session only re-ran green for the live tree. Contracts are locked by tests that would fail without the Spec.

## What would flip this to Fail

- Wrong package, or Spec allowing `send` / empty draft as if completed.
- Outcomes or coaching claiming pinned / posted / saved without refuse language.
- Wave1Specs count ≠ 55, or missing HandOffActions / stage1 / flow / proof surface for pinterest.
- Hardening claimed but not present (ProvesCeiling still unchecked; shopping still soft-OR; shared ban list still missing the named expands).
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
# judge session: 205 passed in 5 packages

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.pinterest"
# 200

# Serve ready (from /tmp/pinterest-serve.log if present):
# adapter_count=55 messaging_adapters=11
# messaging=…+linkedin+pinterest
```
