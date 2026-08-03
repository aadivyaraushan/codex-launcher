# Wave 1 Duolingo + Fitbit + Shazam — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-duolingo-fitbit-shazam-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Duolingo/Fitbit/Shazam bullet); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=58; Duolingo/Fitbit services read + Shazam media read named; `services`=4 / `media`=7; no Strava/Amazon/Chromecast; heartbeat ~29)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`duolingo`/`fitbit`/`shazam`), `adapter_test.go` (want table + ProvesCeiling + shared ban expand + empty/wrong-verb reject), `runtime/deeplink/flow.go` + `flow_test.go` (count 58, services=4, media=7, dedicated routes), stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Live serve:** `/tmp/serve-deeplink-58.log` (2026-08-02 09:53); live process at judge time writing `/tmp/pinterest-serve.log` (09:54) still shows `adapter_count=58`  
**Tests / Play re-run (this judge session):**  
- `go test -v` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **5 packages ok**, **111** `--- PASS:` lines, **0** FAIL  
- Play Store HTTP: `com.duolingo` / `com.fitbit.FitbitMobile` / `com.shazam.android` → **200** / **200** / **200**  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines: `adapter_count=58`, `services_adapters=4`, `media_adapters=7`, services `…+duolingo+fitbit`, media `…+youtube+shazam`  
- Independent Spec ID count from `adapter.go`: **58**; last five `ebay, pinterest, duolingo, fitbit, shazam`; no `strava`/`amazon`/`chromecast` Spec IDs

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-pinterest-and-claim-ban-hardening-judge.md`. Overnight status bullet updated to reference this path by name. No source file imports this markdown.
2. **Existing peer evidence:** Glob for `wave1-duolingo-fitbit-shazam*` found only the task evidence MD before this write. Grep for `wave1-duolingo-fitbit-shazam-prepare-open-judge` returned no matches before this write. No prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; inspection only of code/logs/terminals. No production data fields; date stamped `2026-08-02`.
4. **User instruction (verbatim):** "Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-duolingo-fitbit-shazam-prepare-open-judge.md`. Cite overnight status with judge file. Independently inspect. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Duolingo, Fitbit, and Shazam as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **55 → 58**); Duolingo/Fitbit AppClass `services` verb **read** only; Shazam AppClass `media` verb **read** only; no Strava / Amazon / Chromecast — must have:

1. **Three complete Specs with exact contracts** — IDs `duolingo` / `fitbit` / `shazam`; packages `com.duolingo` / `com.fitbit.FitbitMobile` / `com.shazam.android`; services/read, services/read, media/read; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `duolingo_prepare_open_smoke` / `fitbit_prepare_open_smoke` / `shazam_prepare_open_smoke`. Total Wave1Specs count **58**, locked by test. Appended after Pinterest (indices 55–57).
2. **Honest product ceiling** — Browse/open (or identify/search intent for Shazam) only. Specs must reject empty draft and wrong verbs (`compose`/`write`/`play`/`send` as applicable). Outcomes and stage1 coaching must never claim lesson completed / workout logged|synced|saved / identified|played|saved. No OAuth. No Strava/Amazon/Chromecast Specs.
3. **Full-stack wiring** — Spec → dynamic ClassMap (`services` → 4, `media` → 7) → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof logs services `+duolingo+fitbit` and media `+shazam` with ready `services_adapters=4` / `media_adapters=7` / `adapter_count=58`. Missing any surface is a gap.
4. **Claim bans that lock the new completion phrases** — Shared execute ban list and dedicated flow bans must cover the new tokens (`lesson completed`, `workout logged`, `synced`, `identified` plus peer media/save bans). Spec table must still assert `ProvesCeiling` for every want row including the three new ones.
5. **Real tests that would go red on regression** — count=58; package/class/verb + ProvesCeiling table; empty-draft + wrong-verb reject at indices 55–57; dedicated flow routes with completion-phrase bans; ready-log services=4 / media=7; stage1 instruction asserts; HandOffActions package asserts. TDD: tests written to fail before Specs exist, then green.
6. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Play 200, Pixel Pair-blocked if blocked; overnight bullet + progress snapshot show Wave1Specs=58 and cite this judge file. Inventing a device proof is a Fail on honesty.
7. **Scope discipline** — No commit if that was the ask. Serve restarted with `adapter_count=58`. No Strava/Amazon/Chromecast slipped in.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**111** PASS across five packages), Play packages HTTP **200**, HandOffActions unit green (`tests=3 failures=0`), live/historical serve shows `adapter_count=58` + `services_adapters=4` + `media_adapters=7` + proof strings `…+duolingo+fitbit` / `…+shazam`, ceilings appear in Spec/coaching/dedicated flow/shared ban expand, scope excludes Strava/Amazon/Chromecast, evidence + overnight/progress snapshot are present and consistent, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open, TDD red phase not independently artifacted on disk, and overnight bullet lacked a judge citation until this write.

## Findings (against the bar)

1. **Specs match the task table.** `adapter.go:162-166` — duolingo / fitbit / shazam; packages `com.duolingo` / `com.fitbit.FitbitMobile` / `com.shazam.android`; AppClass services/services/media; verbs read-only; ProvesCeiling `*_prepare_open_smoke`. Indices 55–57 after pinterest at 54. Independent Spec ID count: **58**. Serve ClassMap sizes: services=4, media=7. Count locked at `flow_test.go:95-96` (`want 58`) and want-table length in `adapter_test.go`.

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Empty + wrong-verb rejects for all three (`adapter_test.go:838-894`: compose/send on Duolingo; write/send on Fitbit; play/send on Shazam). Dedicated flows ban lesson/workout/synced/saved and identified/played/saved (`flow_test.go:1397-1487`). Stage1 coaches never-claim lines (`client.go:120-122`; tests `client_test.go:1003-1068`). Execute uses shared `handoff.DraftOutcome` (“cannot know”).

3. **Claim-ban expand present.** Shared execute ban list adds `lesson completed` / `workout logged` / `synced` / `identified` (`adapter_test.go:260`). Spec table asserts `m.ProvesCeiling == want[i].proves` for every row including the three new ones (`adapter_test.go:118-119`).

4. **Packages are real; scope held.** Judge curl: all three → **200**. Companion Spec IDs: duolingo/fitbit/shazam present; strava/amazon/chromecast Spec IDs absent from Wave1Specs (strava appears only as ConsentNeverShipped fixture in `consent_test.go`, not a Wave1 adapter). Progress snapshot: “No Strava/Amazon/Chromecast/Reddit in this pack.”

5. **Full-stack wiring present.** Stage1 dedicated Duolingo/Fitbit + Shazam tests; HandOffActions maps all three (`HandOffActions.kt:75-77`; asserts `HandOffActionsTest.kt:123-128`); flow ready log locks services=4 / media=7 (`flow_test.go:1261-1270`); proof serve strings include `+duolingo+fitbit` / `+shazam` (`deeplink_proof.go:38,43`).

6. **Serve evidence independently confirmed.** `/tmp/serve-deeplink-58.log` (09:53:42) and live `/tmp/pinterest-serve.log` (09:54:04): `adapter_count=58`, `services_adapters=4`, `media_adapters=7`, services includes `duolingo+fitbit`, media includes `shazam`. Older `/tmp/codex-launcher-deeplink-serve.log` (09:24) still shows pre-pack `adapter_count=54` / services=2 / media=6 — useful before/after contrast.

7. **Evidence + overnight status match.** Evidence MD records 55→58, ceilings, wiring, red→green (**111** PASS lines — matches this judge session exactly), serve 58, Pixel on Pair. Progress snapshot: Wave1Specs=58, trio named, services=4 / media=7, no Strava/Amazon/Chromecast, heartbeat ~29. Overnight bullet present; judge citation added with this write.

8. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Overnight status still lists Pixel on Pair. Evidence does not invent a device proof. Live package launch still open until re-pair.
2. **TDD red phase not independently artifacted** — Evidence lists expected reds (count 55≠58, unknown adapter, panic at [55], services_adapters=2, media_adapters=6, missing stage1). No `/tmp/*duolingo*red*` (or sibling) file found this session, unlike Pinterest’s `/tmp/pinterest-red.txt`. Contracts are locked by tests that would fail without the Specs; red→green history is claimed, not re-proven from a saved failing run.
3. **Overnight status lacked judge citation until this write** — Bullet named evidence only; updated to cite `wave1-duolingo-fitbit-shazam-prepare-open-judge.md` as part of this judgment (same peer pattern as prior packs).

## What would flip this to Fail

- Wrong package, or Spec allowing empty draft / wrong verb as if completed.
- Outcomes or coaching claiming lesson completed / workout logged|synced / identified|played without refuse language.
- Wave1Specs count ≠ 58, or missing HandOffActions / stage1 / flow / proof surface for any of the three.
- Strava / Amazon / Chromecast slipped into Wave1Specs contrary to scope.
- Evidence claiming Pixel Auto→Open success while Pair-blocked.
- A commit made contrary to the no-commit ask.

## Reproduce

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe"

go test -v ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1
# judge session: 111 PASS lines; 5 packages ok

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.duolingo"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.fitbit.FitbitMobile"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.shazam.android"
# 200 / 200 / 200

# Serve ready (from /tmp/serve-deeplink-58.log if present):
# adapter_count=58 services_adapters=4 media_adapters=7
# services=taskrabbit+thumbtack+duolingo+fitbit
# media=…+youtube+shazam
```
