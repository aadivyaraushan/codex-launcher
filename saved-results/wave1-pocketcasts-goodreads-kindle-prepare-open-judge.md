# Wave 1 Pocket Casts / Goodreads / Kindle prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-pocketcasts-goodreads-kindle-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Pocket Casts + Goodreads + Kindle bullet); `saved-results/wave1-overnight-progress-snapshot.md` (heartbeat ~35; Wave1Specs=73; media 13; notes 7)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`pocketcasts`/`goodreads`/`kindle`), `adapter_test.go` want-table + shared execute bans, `runtime/deeplink` ClassMap + flow + ready-log + outcome-ban tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`, separate `adapters/podcasts` RT-2  
**Tests / Play / serve re-run (this session):**  
- Same Go package set as evidence (`-count=1 -v`) → **130** `--- PASS:` lines, **0** FAIL, 5 packages ok  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks` → **BUILD SUCCESSFUL**  
- Play Store HTTP: `au.com.shiftyjelly.pocketcasts` / `com.goodreads` / `com.amazon.kindle` → **200**; wrong ids `com.pocketcasts.au` / `com.goodreads.android` / `com.amazon.kindle.reader` → **404**; Amazon shopping `com.amazon.mShop.android.shopping` → **200** but **absent** from Wave1Specs (correct)  
- Serve LIVE this session: process `/tmp/codex-launcher-deeplink serve-deeplink-proof` (pid **82655**, matches evidence); ready lines `adapter_count=73`, `media_adapters=13`, `notes_adapters=7`, media `…+pocketcasts+kindle`, notes `…+goodreads` in `/tmp/wave1-pack73-serve-live.log` (11:26:53). Binary embeds packages + smoke ids + proof media/notes strings + stage1 coaching lines.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-sheets-evernote-slides-prepare-open-judge.md`. Cross-cite from overnight status bullet in `saved-results/wave1-overnight-batch-and-oauth-prep.md` line 217 (Pocket Casts/Goodreads/Kindle) and `saved-results/wave1-overnight-progress-snapshot.md` line 20 (Latest pack evidence).
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-pocketcasts-goodreads-kindle-prepare-open.md` — **present**, dated 2026-08-02. Glob/Grep: only the evidence `.md` exists; no `*-judge.md` for this pack before this write (`ls`: No such file).
3. **Data I/O:** None — static markdown verdict; no structured data files; no secrets copied here.
4. **User instruction (verbatim):** Adversarial LLM-as-judge with FRESH context. Define strong quality bar from first principles FIRST, then grade. Overnight pack: Pocket Casts, Goodreads, Kindle → Wave1Specs 70→73 in worktree. Claimed: media play|read, notes/read, media/read; Kindle is reader prepare-and-open not Amazon shopping; separate from podcasts RT-2; serve LIVE adapter_count=73; evidence `saved-results/wave1-pocketcasts-goodreads-kindle-prepare-open.md`; no commit. Deliverable: Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-pocketcasts-goodreads-kindle-prepare-open-judge.md`. Cite overnight status with judge file. Independently inspect + confirm serve live. No questions, no commit, no secrets.

## First-principles bar (before hunting bugs)

A strong result for **this** task — add Pocket Casts, Goodreads, and Kindle as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **70 → 73**) — must have:

1. **Product shape** — Operator prepares intent text and opens the official consumer app. The user finishes play/subscribe/review/shelf/purchase/read inside that app. Success is open-with-draft, not completion inside Operator.
2. **Honest verb + class choice** — Pocket Casts = AppClass `media`, verbs **`play|read`** (open/search podcast app). Goodreads = AppClass `notes`, verb **`read`** (browse/open book intent). Kindle = AppClass `media`, verb **`read`** (open library/book intent). Outcomes and coaching must **never** claim played / downloaded / subscribed; review posted / shelved / rated; purchased / downloaded / read completed.
3. **Correct Android packages, live on Play** — Specs and `HandOffActions` must use Play Store ids that return live HTTP 200, not guessed dead ids. Kindle package must be the **reader** (`com.amazon.kindle`), not Amazon shopping.
4. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for all three Specs. No OAuth / completes adapter for this Wave-1 surface.
5. **Routing classes that exist** — media + notes only. No invented class. Ready log must show `media_adapters=13` (+pocketcasts+kindle) and `notes_adapters=7` (+goodreads).
6. **Separation from Podcasts RT-2** — Pocket Casts deeplink Spec must not collide with or replace the separate `podcasts` RSS adapter / `serve-podcasts-proof`. Distinct ids and runtimes.
7. **End-to-end wiring** — Spec → ClassMap → stage1 coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` logs (`adapter_count=73`, media includes `+pocketcasts+kindle`, notes includes `+goodreads`).
8. **Real tests** — Tests that go red if count/packages/verbs/empty-draft/wrong-verb reject/flow route/outcome bans/stage1 coaching/HandOffActions/ready-log counts break. ProvesCeiling locked in Spec want table for this pack. Shared execute ban list includes `subscribed` / `shelved` / `rated`.
9. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.
10. **Scope honesty** — No Amazon shopping Spec. Kindle reader only. Separate from Podcasts RT-2. No commit if that was the ask. Overnight status + progress snapshot reflect Wave1Specs=73 and media/notes counts without contradiction.
11. **Serve live, not log-only** — A running `serve-deeplink-proof` process with `adapter_count=73`, `media_adapters=13`, and `notes_adapters=7` ready lines, not only a written claim.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**130** Go PASS lines on the evidence package set + HandOffActions BUILD SUCCESSFUL), all three Play packages HTTP **200**, serve is LIVE with `adapter_count=73` + `media_adapters=13` + `notes_adapters=7` + proof media/notes strings (independently confirmed; pid **82655** matches evidence), Pocket Casts stays separate from Podcasts RT-2, Kindle stays reader-not-shopping (Amazon shopping Play id exists and was correctly not added), evidence + overnight batch bullet + progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open and TDD red chronology not independently time-stamped.

## Findings (against the bar)

- **Wave1Specs count locked at 73 with three Specs last.** Indices 70–72 after googleslides at 69. Specs: `pocketcasts` (`au.com.shiftyjelly.pocketcasts`, media, `[play, read]`, `pocketcasts_prepare_open_smoke`), `goodreads` (`com.goodreads`, notes, `[read]`, `goodreads_prepare_open_smoke`), `kindle` (`com.amazon.kindle`, media, `[read]`, `kindle_prepare_open_smoke`). Manifest path locks RT-4 / HandsOff / AuthNone / ConsentA. Runtime `flow_test.go` locks count 73 + ready log `media_adapters=13` / `notes_adapters=7`. Empty draft + wrong verbs rejected at indices 70–72.
- **Never claims completion on the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know”). Shared execute ban list includes `subscribed` / `shelved` / `rated` (plus prior played/downloaded/purchased/posted/completed tokens that also cover “review posted” / “read completed”). Dedicated flow tests ban pack-specific phrases. Stage1 coaches Pocket Casts / Goodreads / Kindle with matching never-claim lines and Podcasts-RT-2 / Amazon-shopping separation (`client.go` + stage1 tests).
- **Play packages are the live ids.** Spec + HandOffActions + evidence use the three ids above. This session: all **200**; short/wrong ids **404**. Amazon shopping `com.amazon.mShop.android.shopping` is **200** on Play and **absent** from Wave1Specs — scope held. `/tmp/codex-launcher-deeplink` embeds packages, smoke ids, media `…+pocketcasts+kindle`, notes `…+goodreads`.
- **HandOffActions maps display names + short ids.** `pocket casts`/`pocketcasts`, `goodreads`, `kindle` → matching packages (`HandOffActions.kt` + unit test BUILD SUCCESSFUL).
- **ClassMap / proof wiring.** Runtime builds media ClassMap from Spec AppClass (13 media Specs including pocketcasts+kindle) and notes (7 including goodreads). `deeplink_proof.go` logs match. `podcasts` id is **not** in Wave1Specs; RT-2 package `adapters/podcasts` + `serve-podcasts-proof` remain separate.
- **Serve live independently confirmed.** Process running (pid **82655**); ready lines show `adapter_count=73`, `media_adapters=13`, `notes_adapters=7`, proof media/notes strings. Evidence log `/tmp/wave1-pack73-serve-live.log` matches this session (still alive at judge time).
- **Evidence + overnight + snapshot honesty match this session.** Evidence records 70→73, ceilings, wiring, red→green, Play 200, Pixel on Pair, no commit. Overnight bullet claims 130 PASS + HandOffActions green + Play 200 + serve LIVE `adapter_count=73` pid 82655 — matches this session. Progress snapshot heartbeat ~35 already lists Wave1Specs=73, media 13, notes 7, Pocket Casts/Kindle media + Goodreads notes, Podcasts RT-2 separation, no Amazon shopping. HEAD remains `5cb0831` (Wave 0); pack files still dirty/untracked — **no commit**, as claimed. TDD red chronology is implementer-reported (not independently time-stamped here); green locks are verified now.
- **Scope kept.** No Amazon shopping Spec. Kindle comments explicitly mark reader-not-shopping. Pocket Casts comments + stage1 coaching explicitly separate from Podcasts RT-2 RSS.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence + overnight: Pixel on Pair; companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **TDD red chronology not independently witnessed.** Evidence narrates red failures; this judge verified green locks only.
3. **Stale header comments elsewhere remain likely** (package-file / proof function headers historically lag packs). Not re-audited line-by-line beyond this pack’s Spec/proof media+notes keys, which are present and correct.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for Pocket Casts / Goodreads / Kindle (`serve-deeplink-proof`). Keep overnight serve log + pid in sync when restarting. Leave Podcasts RT-2 and Amazon shopping Specs out of Wave1Specs unless product scope changes.
