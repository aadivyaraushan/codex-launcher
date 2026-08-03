# Wave 1 Pandora / Asana / Trello prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-pandora-asana-trello-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Pandora/Asana/Trello bullet); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=64; media 11 +pandora; tasks 2 asana+trello; no Microsoft To Do / Todoist deeplink; heartbeat ~31)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`pandora`/`asana`/`trello`), `adapter_test.go`, `runtime/deeplink/flow.go` + `flow_test.go`, stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Live serve (this judge session):** pid **76535** `/tmp/codex-launcher-deeplink serve-deeplink-proof` (STAT Ss, still alive); log `/tmp/wave1-pack64-serve-live.log` (2026-08-02 10:25:38); binary mtime 10:25:38  
**Tests / Play re-run (this judge session):**  
- `go test` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **5 packages ok**; verbose **119 PASS / 0 FAIL** (1 unrelated SKIP: `TestLiveRouteAgainstApprovedAccount`)  
- Play Store HTTP: `com.pandora.android` / `com.asana.app` / `com.trello` → **200**  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines: `adapter_count=64`, `media_adapters=11`, `tasks_adapters=2`, media `…+soundcloud+pandora`, `tasks=asana+trello`

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-chromecast-youtubemusic-soundcloud-prepare-open-judge.md`. Overnight status bullet references this path by name after the companion edit. No source file imports this markdown.
2. **Existing peer evidence:** Glob for `wave1-pandora-asana-trello*` found only the task evidence MD (`wave1-pandora-asana-trello-prepare-open.md`). Grep for `wave1-pandora-asana-trello-prepare-open-judge` returned no matches before this write. No prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; inspection only of code/logs/process. No production data fields; date stamped `2026-08-02`. No secrets copied.
4. **User instruction (verbatim):** "Adversarial LLM-as-judge with FRESH context. Define strong quality bar from first principles FIRST, then grade. … Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-pandora-asana-trello-prepare-open-judge.md`. Cite overnight status with judge file. Independently inspect + confirm serve process live if possible. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Pandora, Asana, and Trello as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **61 → 64**); Pandora AppClass `media` verbs **play|read**; Asana/Trello AppClass **tasks** verb **write**; ready-log `media_adapters=11` + new `tasks_adapters=2`; serve LIVE `adapter_count=64`; no Microsoft To Do / Todoist deeplink Spec — must have:

1. **Three complete Specs with exact contracts** — IDs `pandora` / `asana` / `trello`; packages `com.pandora.android` / `com.asana.app` / `com.trello`; media play|read + tasks write + tasks write; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `pandora_prepare_open_smoke` / `asana_prepare_open_smoke` / `trello_prepare_open_smoke`. Total Wave1Specs count **64**, locked by test. Appended after SoundCloud (index 60), then pandora/asana/trello at 61–63.
2. **Honest product ceilings** — Media prepare-and-open (Pandora; SoundCloud peer). Tasks create/open intent only (Asana/Trello) — **not** Todoist RT-2 completes. Specs must reject empty draft and wrong verbs. Outcomes and stage1 coaching must never claim played / station changed (Pandora) or task created / card moved / assigned / completed (Asana/Trello).
3. **Full-stack wiring** — Spec → dynamic ClassMap (`media` + new `tasks`) → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof log media `+pandora` and `tasks=asana+trello`, and flow ready `media_adapters=11` / `tasks_adapters=2` / `adapter_count=64`. Missing any surface is a gap.
4. **Real tests that would go red on regression** — count=64; package/class/verb/ProvesCeiling table including the three; empty-draft + wrong-verb rejects; dedicated flow route tests with completion-phrase bans; ready-log counts; stage1 instruction asserts for each app; HandOffActions package asserts; shared execute ban covers station/task/card tokens.
5. **Honest evidence + overnight status + live serve** — Evidence MD documents wiring, red→green, Play 200, Pixel blocked if blocked; overnight bullet + progress snapshot show Wave1Specs=64. Serve process actually running with matching ready lines (not log-only archaeology). Inventing a device proof is a Fail on honesty.
6. **Scope discipline** — No Microsoft To Do Spec. No Todoist deeplink Spec (Todoist stays RT-2). No commit if that was the ask.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**119** passed across five packages), all three Play packages HTTP **200**, HandOffActions unit green (`tests=3 failures=0`), **live** serve pid **76535** shows `adapter_count=64` + `media_adapters=11` + `tasks_adapters=2` + proof strings for all three apps, ceilings appear in Specs/coaching/dedicated flow bans/shared execute ban, Todoist/Microsoft To Do stay out of Wave1Specs, evidence + overnight status/progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack. Not a Fail. Warnings are open device smoke, a thin shared-ban gap on bare `completed` / `card created`, and historical red not re-observed this session.

## Findings (against the bar)

1. **Specs match the task table.** `adapter.go:178-182` — pandora/asana/trello; packages `com.pandora.android` / `com.asana.app` / `com.trello`; AppClass media/tasks/tasks; verbs play|read / write / write; ProvesCeiling strings as claimed. Indices 61–63 after soundcloud at 60. Independent Spec ID count: **64**; media AppClass rows **11**; tasks AppClass rows **2**. Count locked at `flow_test.go:95-96` (`want 64`) and want-table length in `adapter_test.go` (also asserts ProvesCeiling per Spec at `:128-129`).

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Pandora rejects empty/write/send (`adapter_test.go:983-1000`). Asana/Trello reject empty/send/compose (`:1002-1035`). Dedicated flows ban played/playing/added to playlist/station changed (Pandora) and task created/card moved/assigned/completed (Asana/Trello) (`flow_test.go:1593-1694`). Stage1 coaches the same refuse language and explicitly says not Todoist completes (`client.go:128-130`; `client_test.go:1165-1239`). Execute uses shared `handoff.DraftOutcome` (“cannot know”). Shared execute ban list also includes `station changed` / `task created` / `card moved` / `assigned` (`adapter_test.go:286-287`).

3. **Packages are real; scope held.** Judge curl: pandora/asana/trello → **200**. Grep `adapters/deeplink` for `todoist` / Microsoft To Do Spec IDs: **none**.

4. **Full-stack wiring present.** Dedicated stage1 tests for Pandora and Asana/Trello; HandOffActions maps all three ids (`HandOffActions.kt:84-86`) with unit asserts for display + lowercase (`HandOffActionsTest.kt:140-145`); flow ready log locks media=11 / tasks=2 (`flow_test.go:1262-1277`); proof serve strings in `deeplink_proof.go:38-42`; runtime logs `tasks_adapters` (`flow.go:70`).

5. **Serve evidence independently confirmed LIVE.** Process **76535** running `serve-deeplink-proof` at judge time. `/tmp/wave1-pack64-serve-live.log` lines 2–3 (10:25:38): `adapter_count=64`, `media_adapters=11`, `tasks_adapters=2`, media includes `+pandora`, `tasks=asana+trello`. Binary mtime matches start.

6. **Evidence + overnight status match.** Evidence MD records 61→64, ceilings, wiring, red→green (119 PASS), Pixel on Pair blocked, no Todoist deeplink. Progress snapshot: Wave1Specs=64, Pandora/Asana/Trello named, media 11 / tasks 2, heartbeat ~31. Overnight bullet updated to cite this judge file.

7. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Allowed by task (“Pixel on Pair — skip device smoke”); evidence is honest. Live package launch still open until re-pair.
2. **Shared execute ban list misses bare `completed` and `card created`** — Shared table bans `task created` / `card moved` / `assigned` but not bare `completed` (only `completed payment`) or `card created`. Dedicated Asana/Trello flow + stage1 cover `completed`; a DraftOutcome regression that only said “completed” or “card created” would not trip the shared table.
3. **TDD red phase not independently re-proven** — Evidence documents red failures before Specs; this judge session only re-ran green. Contracts are locked by tests that would fail without the Specs; historical red is trusted from evidence, not re-observed.
4. **Unrelated SKIP in focused suite** — `TestLiveRouteAgainstApprovedAccount` skipped (owner Approve gate). Does not affect this pack’s contracts.

## What would flip this to Fail

- Wrong packages, or Specs allowing send/compose/play as if task/card completed or track played/station changed.
- Outcomes or coaching claiming played / station changed / task created / card moved / assigned / completed without refuse language.
- Wave1Specs count ≠ 64, or missing HandOffActions / stage1 / flow / proof / `tasks_adapters` surface for any of the three.
- Todoist or Microsoft To Do added as Wave1 deeplink Specs contrary to explicit non-goal.
- Evidence claiming Pixel Auto→Open success while Pair-blocked, or claiming LIVE serve when process is dead and log is stale.
- A commit made contrary to the no-commit ask.

## Reproduce

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe"

go test ./companion/cmd/codex-launcher/ \
        ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/internal/capability/runtime/ -count=1 -v
# judge session: 119 PASS / 0 FAIL in 5 packages

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

for id in com.pandora.android com.asana.app com.trello; do
  curl -s -o /dev/null -w "$id %{http_code}\n" "https://play.google.com/store/apps/details?id=$id"
done
# 200 / 200 / 200

ps -p 76535 -o pid,etime,command
# /tmp/codex-launcher-deeplink serve-deeplink-proof
sed -n '2,3p' /tmp/wave1-pack64-serve-live.log
# adapter_count=64 media_adapters=11 tasks_adapters=2 …+pandora tasks=asana+trello
```
