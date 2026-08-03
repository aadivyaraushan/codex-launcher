# Wave 1 Threads / TikTok / Expedia prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-threads-tiktok-expedia-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Threads + TikTok + Expedia bullet); `saved-results/wave1-overnight-progress-snapshot.md` (heartbeat ~24, Wave1Specs=45)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`threads`/`tiktok`/`expedia`), `adapter_test.go`, `runtime/deeplink/flow_test.go`, stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Tests / Play re-run (this judge session):**  
- `go test` adapters/deeplink + runtime/deeplink + stage1/openai + cmd/codex-launcher → **166 passed in 4 packages**  
- Play Store HTTP: `com.instagram.barcelona` / `com.zhiliaoapp.musically` / `com.expedia.bookings` → **200**; wrong id `com.ss.android.ugc.trill` → **404**  
- HandOffActions gradle unit test **not** re-run this session (source + unit asserts inspected; evidence claims BUILD SUCCESSFUL)

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-airbnb-opentable-grubhub-prepare-open-judge.md`. No source file imports or reads this markdown.
2. **Existing peer evidence:** Glob/Grep for `*threads*tiktok*expedia*judge*` and path `wave1-threads-tiktok-expedia-prepare-open-judge` → **0 matches**. Task evidence MD exists separately; no prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; no structured data files read or written.
4. **User instruction (verbatim):** "Write judge report to: saved-results/wave1-threads-tiktok-expedia-prepare-open-judge.md" / "Return only the verdict + top findings to parent." Full task: independent LLM-as-judge; fresh context; do NOT trust the implementer; grade Wave1Specs 42→45 (threads/tiktok/expedia); define bar before looking; Pass | Pass-with-warnings | Fail.

## First-principles bar (before looking at the work)

A strong result for **this** task — add Threads, TikTok, and Expedia as Wave 1 prepare-and-open hand-offs (append after Grubhub; `Wave1Specs` **42 → 45**) — must have:

1. **Three complete Specs with exact contracts** — IDs `threads` / `tiktok` / `expedia`; AppClass messaging / messaging / travel; verbs compose / compose / read; packages `com.instagram.barcelona` / `com.zhiliaoapp.musically` / `com.expedia.bookings`; ProvesCeiling `threads_prepare_open_smoke` / `tiktok_prepare_open_smoke` / `expedia_search_prepare_open_smoke`. Total Wave1Specs count **45**, locked by test.
2. **Honest product ceilings** — Threads never claims posted or replied; TikTok is messaging+compose (Facebook peer, not media) and never uses `com.ss.android.ugc.trill`; Expedia is travel/read and never claims booked. Spec verbs, Resolve rejects, outcome wording, and stage1 coaching must all respect those ceilings.
3. **Full-stack wiring mirrored from Airbnb/Facebook packs** — Spec → ClassMap registration → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → `serve-deeplink-proof` messaging/travel logs. Missing any surface is a gap.
4. **Real tests that would go red on regression** — count=45; package/class/verb tables; empty-draft + wrong-verb rejects (send on Threads/TikTok; book/send on Expedia); dedicated flow route tests with completion-phrase bans; stage1 instruction asserts; HandOffActions package asserts.
5. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Pixel unpaired if blocked; overnight bullet + progress snapshot show Wave1Specs=45 / heartbeat ~24. Inventing a device proof is a Fail on honesty.
6. **No commit** — work stays uncommitted if that was the ask.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused Go tests re-run green this session (**166** passed across the four packages), all three Play packages HTTP **200** with trill **404**, TikTok correctly classed as messaging (not media), ceilings appear in Specs/coaching/flow bans, evidence + overnight status/progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack. Not a Fail. Warnings are open live smoke, thinner shared ban list vs dedicated flow bans, Threads stage1 test not locking “replied”, and ProvesCeiling string not asserted in the Spec table test.

## Findings (against the bar)

1. **Specs match the task table exactly.** `adapter.go:127-134` — threads messaging/compose/`com.instagram.barcelona`/`threads_prepare_open_smoke`; tiktok messaging/compose/`com.zhiliaoapp.musically`/`tiktok_prepare_open_smoke`; expedia travel/read/`com.expedia.bookings`/`expedia_search_prepare_open_smoke`. Indices 42–44 after grubhub at 41. Count locked at `flow_test.go:93-94` and adapter Spec table length check.

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Compose-only Threads/TikTok reject send + empty draft (`adapter_test.go:629-644`); Expedia rejects book/send + empty (`adapter_test.go:645-664`). Flow tests ban `posted`/`replied`/`published`/`sent` for Threads/TikTok (`flow_test.go:1115-1121`) and `booked`/`reserved`/`purchased` for Expedia (`flow_test.go:1158-1164`); both require “cannot know”. Stage1 coaches never-claim posted(/replied) and booked (`client.go:84-85`, `108`). Execute uses shared `handoff.DraftOutcome` (`adapter.go:219-224` → `outcome.go:23-34`).

3. **TikTok package and class are correct; trill unused.** Spec + HandOffActions use `com.zhiliaoapp.musically` only. `com.ss.android.ugc.trill` appears only as a documented-404 comment (`adapter.go:130`, `adapter_test.go:76`). Judge curl: musically **200**, trill **404**. AppClass is `messaging`, not media.

4. **Full-stack wiring present.** Stage1 coaching + three dedicated tests (`client_test.go:714-808`); HandOffActions maps all three names (`HandOffActions.kt:59-61`) with unit asserts (`HandOffActionsTest.kt:91-96`); `deeplink_proof.go:38,41` appends `+threads+tiktok` to messaging and `+expedia` to travel; execute cases include all three (`adapter_test.go:179-181`); dedicated flow routes (`flow_test.go:1075-1166`).

5. **Evidence + overnight status match.** Evidence MD records 42→45, ceilings, wiring, red→green, Pixel unpaired. Overnight bullet at `wave1-overnight-batch-and-oauth-prep.md:207`. Progress snapshot: heartbeat ~24, Wave1Specs=45 (`wave1-overnight-progress-snapshot.md:3,18`).

6. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack files remain dirty/untracked in the worktree.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Allowed by task (“Pixel unpaired OK”); evidence is honest. Live smoke still open until re-pair.
2. **Shared execute ban list is thinner than dedicated flow bans** — `TestDeepLinkComposeHandsOffWithoutClaimingCompletion` bans `sent`/`booked`/… but not `posted`/`replied`/`published` (`adapter_test.go:213`). Dedicated Threads/TikTok flow covers those phrases; a DraftOutcome regression that only said “posted” would not trip the shared table (same soft gap as Netflix/Facebook judge).
3. **Threads stage1 test does not lock “replied”** — Coaching text includes “posted or replied” (`client.go:84`), but `TestStage1InstructionsCoachThreadsPrepareAndOpenAsMessaging` only requires `never claim` + `posted` (`client_test.go:741-743`). Grep of `client_test.go` finds no `replied` assert. Flow test still bans `replied`.
4. **ProvesCeiling string not asserted in Spec table test** — Values are set on Specs and passed through `Describe()` (`adapter.go:164`); the want-table test checks id/package/class/verbs/HandsOff but not the ProvesCeiling string itself.

## What would flip this to Fail

- Wrong TikTok package (`trill`) or TikTok classed as media with play/read/write.
- Specs claiming compose/read completes (posted/replied/booked) in outcomes or coaching without refuse language.
- Wave1Specs count ≠ 45, or missing HandOffActions / stage1 / flow / proof surface for any of the three.
- Evidence claiming Pixel Auto→Open success while unpaired.
- A commit made contrary to the no-commit ask.

## Reproduce

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe"
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/routing/stage1/openai/ \
        ./companion/cmd/codex-launcher/ -count=1
# judge session: 166 passed in 4 packages

for id in com.instagram.barcelona com.zhiliaoapp.musically com.expedia.bookings com.ss.android.ugc.trill; do
  curl -s -o /dev/null -w "$id %{http_code}\n" "https://play.google.com/store/apps/details?id=$id"
done
# 200 / 200 / 200 / 404
```
