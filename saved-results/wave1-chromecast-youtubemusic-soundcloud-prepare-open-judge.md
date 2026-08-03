# Wave 1 Chromecast + YouTube Music + SoundCloud — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-chromecast-youtubemusic-soundcloud-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Chromecast/YouTube Music/SoundCloud bullet); `saved-results/wave1-overnight-progress-snapshot.md` (`Wave1Specs`=61; media 10; Chromecast/YouTube Music/SoundCloud named; no Pandora/Asana/Trello/Strava/Amazon/Telegram; heartbeat ~30)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`chromecast`/`youtubemusic`/`soundcloud`), `adapter_test.go` (want table + ProvesCeiling + shared ban expand + empty/wrong-verb reject), `runtime/deeplink/flow.go` + `flow_test.go` (count 61, media=10, dedicated routes), stage1 OpenAI `client.go` + `client_test.go`, `HandOffActions.kt` + `HandOffActionsTest.kt`, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Serve evidence:** `/tmp/wave1-pack61-serve.log` (2026-08-02 10:08:53) — no live `serve-deeplink` process at judge time  
**Tests / Play re-run (this judge session):**  
- `go test -v` cmd/codex-launcher + adapters/deeplink + runtime/deeplink + stage1/openai + capability/runtime → **5 packages ok**, **115** `--- PASS:` lines, **0** FAIL  
- Play Store HTTP: `com.google.android.apps.chromecast.app` / `com.google.android.apps.youtube.music` / `com.soundcloud.android` → **200** / **200** / **200**  
- `HandOffActionsTest` gradle unit → **BUILD SUCCESSFUL** (`tests="3" failures="0"`)  
- Serve ready lines (log): `adapter_count=61`, `media_adapters=10`, media `…+shazam+chromecast+youtubemusic+soundcloud`  
- Independent Spec ID count from `adapter.go`: **61**; last three `chromecast, youtubemusic, soundcloud`; no `pandora`/`asana`/`trello`/`amazon`/`strava`/`telegram` Spec IDs

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-duolingo-fitbit-shazam-prepare-open-judge.md`. Overnight status bullet updated to reference this path by name. No source file imports this markdown.
2. **Existing peer evidence:** Glob for `wave1-chromecast*` found only the task evidence MD before this write. Grep for `wave1-chromecast-youtubemusic-soundcloud-prepare-open-judge` returned no matches before this write. No prior judge file for this pack.
3. **Data I/O:** None — static markdown verdict; inspection only of code/logs/terminals. No production data fields; date stamped `2026-08-02`.
4. **User instruction (verbatim):** "Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-chromecast-youtubemusic-soundcloud-prepare-open-judge.md`. Cite overnight status with judge file. Independently inspect. No questions, no commit, no secrets."

## First-principles bar (before looking at the work)

A strong result for **this** task — add Chromecast, YouTube Music, and SoundCloud as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **58 → 61**); Chromecast AppClass `media` verb **play** only; YouTube Music / SoundCloud AppClass `media` verbs **play|read**; no Pandora / Asana / Trello / Amazon / Strava / Telegram — must have:

1. **Three complete Specs with exact contracts** — IDs `chromecast` / `youtubemusic` / `soundcloud`; packages `com.google.android.apps.chromecast.app` / `com.google.android.apps.youtube.music` / `com.soundcloud.android`; media/play, media play|read, media play|read; ceiling `hands_off`, consent A, auth none, RT-4; ProvesCeiling `chromecast_prepare_open_smoke` / `youtubemusic_prepare_open_smoke` / `soundcloud_prepare_open_smoke`. Total Wave1Specs count **61**, locked by test. Appended after Shazam (indices 58–60).
2. **Honest product ceiling** — Cast/open or browse/search intent only. Specs must reject empty draft and wrong verbs. Outcomes and stage1 coaching must never claim cast started / playing / connected (Chromecast) or played / added to playlist / library changed (YouTube Music / SoundCloud). No OAuth. No out-of-scope Specs.
3. **Full-stack wiring** — Spec → dynamic ClassMap (`media` → 10) → stage1 coaching → Android `HandOffActions` display-name→package map → `DraftOutcome` → serve proof log media `+chromecast+youtubemusic+soundcloud` with ready `media_adapters=10` / `adapter_count=61`. Missing any surface is a gap.
4. **Claim bans that lock the new completion phrases** — Shared execute ban list and dedicated flow bans must cover the new tokens (`cast started`, `connected`, `library changed`, plus peer `played`/`playing`/`added to playlist`). Spec table must assert `ProvesCeiling` for every want row including the three new ones.
5. **Real tests that would go red on regression** — count=61; package/class/verb + ProvesCeiling table; empty-draft + wrong-verb reject at indices 58–60; dedicated flow routes with completion-phrase bans; ready-log media=10; stage1 instruction asserts; HandOffActions package asserts. TDD: tests written to fail before Specs exist, then green.
6. **Honest evidence + overnight status** — Evidence MD documents wiring, red→green, Play 200, Pixel Pair-blocked if blocked; overnight bullet + progress snapshot show Wave1Specs=61 and cite this judge file. Inventing a device proof is a Fail on honesty.
7. **Scope discipline** — No commit if that was the ask. Serve restarted with `adapter_count=61`. No Pandora/Asana/Trello/Amazon/Strava/Telegram slipped in.

## Verdict: **Pass-with-warnings**

Core contracts are met in code, locked by focused Go tests re-run green this session (**115** PASS across five packages — matches evidence’s PASS-line count exactly), Play packages HTTP **200**, HandOffActions unit green (`tests=3 failures=0`), historical serve log shows `adapter_count=61` + `media_adapters=10` + proof string `…+shazam+chromecast+youtubemusic+soundcloud`, ceilings appear in Spec/coaching/dedicated flow/shared ban expand, scope excludes Pandora/Asana/Trello/Amazon/Strava/Telegram, evidence + overnight/progress snapshot are present and consistent, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open, TDD red phase not independently artifacted on disk, serve not live at judge time (log-only), and overnight bullet lacked a judge citation until this write.

## Findings (against the bar)

1. **Specs match the task table.** `adapter.go:170-174` — chromecast / youtubemusic / soundcloud; packages `com.google.android.apps.chromecast.app` / `com.google.android.apps.youtube.music` / `com.soundcloud.android`; AppClass media; verbs play / play+read / play+read; ProvesCeiling `*_prepare_open_smoke`. Indices 58–60 after shazam at 57. Independent Spec ID count: **61**. Media ClassMap size from serve ready log: **10** (spotify→soundcloud). Count locked at `flow_test.go:95-96` (`want 61`) and want-table length in `adapter_test.go`.

2. **Ceilings are honest across Spec → Resolve → outcome → coaching.** Empty + wrong-verb rejects for all three (`adapter_test.go:911-963`: read/send on Chromecast; write/send on YouTube Music/SoundCloud). Dedicated Chromecast flow bans cast started/playing/connected/played (`flow_test.go:1491-1527`). YouTube Music/SoundCloud play+read cases ban played/playing/added to playlist/library changed (`flow_test.go:1532-1582`). Stage1 coaches never-claim lines (`client.go:124-126`; tests `client_test.go:1083-1160`). Execute uses shared `handoff.DraftOutcome` (“cannot know”).

3. **Claim-ban expand present.** Shared execute ban list adds `cast started` / `connected` / `library changed` (`adapter_test.go:273-274`). Spec table asserts `m.ProvesCeiling == want[i].proves` for every row including the three new ones (`adapter_test.go:123-124`).

4. **Packages are real; scope held.** Judge curl: all three → **200**. Companion Spec IDs: chromecast/youtubemusic/soundcloud present; pandora/asana/trello/amazon/strava/telegram Spec IDs absent from Wave1Specs. Progress snapshot: “No Pandora/Asana/Trello/Strava/Amazon/Telegram/Reddit in this pack.”

5. **Full-stack wiring present.** Stage1 dedicated Chromecast + YouTube Music/SoundCloud tests; HandOffActions maps chromecast / youtube music / youtubemusic / soundcloud (`HandOffActions.kt:79-82`; asserts `HandOffActionsTest.kt:131-137`); flow ready log locks media=10 (`flow_test.go:1268-1271`); proof serve string includes `+chromecast+youtubemusic+soundcloud` (`deeplink_proof.go:38`).

6. **Serve evidence independently confirmed (log).** `/tmp/wave1-pack61-serve.log` lines 2–3 (10:08:53): `adapter_count=61`, `media_adapters=10`, media includes `+chromecast+youtubemusic+soundcloud`. No live serve process at judge time — gap noted below, not a Fail (log is contemporaneous and matches code).

7. **Evidence + overnight status match.** Evidence MD records 58→61, ceilings, wiring, red→green (**115** PASS lines — matches this judge session exactly), serve 61, Pixel on Pair. Progress snapshot: Wave1Specs=61, trio named, media=10, no out-of-scope apps, heartbeat ~30. Overnight bullet present; judge citation added with this write.

8. **No commit for this pack.** `git log -1` still `5cb0831` (Wave 0); pack paths remain dirty/untracked.

## Gaps vs bar (warnings, not Fail)

1. **Pixel Auto→Open not exercised** — Overnight status still lists Pixel on Pair. Evidence does not invent a device proof. Live package launch still open until re-pair.
2. **TDD red phase not independently artifacted** — Evidence lists expected reds (count 58≠61, unknown adapter, panic at [58], media_adapters=7, missing stage1). No `/tmp/*chromecast*red*` (or sibling) file found this session, unlike Pinterest’s `/tmp/pinterest-red.txt`. Contracts are locked by tests that would fail without the Specs; red→green history is claimed, not re-proven from a saved failing run. Implementer green artifacts `/tmp/wave1-pack61-go*.txt` exist and match.
3. **Serve not live at judge time** — Ready lines come from `/tmp/wave1-pack61-serve.log` (10:08:53), not a still-running process. Log content matches Specs/proof code; restart not re-done by judge.
4. **Overnight status lacked judge citation until this write** — Bullet named evidence only; updated to cite `wave1-chromecast-youtubemusic-soundcloud-prepare-open-judge.md` as part of this judgment (same peer pattern as prior packs).

## What would flip this to Fail

- Wrong package, or Spec allowing empty draft / wrong verb as if completed.
- Outcomes or coaching claiming cast started / playing / connected / played / playlist / library without refuse language.
- Wave1Specs count ≠ 61, or missing HandOffActions / stage1 / flow / proof surface for any of the three.
- Pandora / Asana / Trello / Amazon / Strava / Telegram slipped into Wave1Specs contrary to scope.
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
# judge session: 115 PASS lines; 5 packages ok

./android/gradlew -p android :app:testDebugUnitTest \
  --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks
# BUILD SUCCESSFUL; HandOffActionsTest tests=3 failures=0

curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.chromecast.app"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.google.android.apps.youtube.music"
curl -s -o /dev/null -w "%{http_code}\n" \
  "https://play.google.com/store/apps/details?id=com.soundcloud.android"
# 200 / 200 / 200

# Serve ready (from /tmp/wave1-pack61-serve.log if present):
# adapter_count=61 media_adapters=10
# media=…+shazam+chromecast+youtubemusic+soundcloud
```
