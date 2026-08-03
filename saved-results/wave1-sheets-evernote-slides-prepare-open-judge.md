# Wave 1 Google Sheets / Evernote / Google Slides prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-sheets-evernote-slides-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Google Sheets + Evernote + Google Slides bullet); `saved-results/wave1-overnight-progress-snapshot.md` (heartbeat ~33 header claims Wave1Specs=70)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`googlesheets`/`evernote`/`googleslides`), `adapter_test.go` want-table + shared execute bans, `runtime/deeplink` ClassMap + flow + ready-log + outcome-ban tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Tests / Play / serve re-run (this session):**  
- Same Go package set as evidence (`-count=1 -v`) → **125** `--- PASS:` lines, **0** FAIL, 5 packages ok  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks` → **BUILD SUCCESSFUL**  
- Play Store HTTP: `com.google.android.apps.docs.editors.sheets` / `com.evernote` / `com.google.android.apps.docs.editors.slides` → **200**; wrong ids `com.google.android.apps.sheets` / `com.google.android.apps.slides` / `com.evernote.android` → **404** (`com.google.android.apps.docs` also **200** — Docs family; Spec correctly uses editors packages)  
- Serve LIVE this session: process `/tmp/codex-launcher-deeplink serve-deeplink-proof` (shell pid **34422**, serve pid **34436**, matches evidence); ready lines `adapter_count=70`, `notes_adapters=6`, `notes=googlekeep+googledocs+dropbox+googlesheets+evernote+googleslides` in `/tmp/wave1-pack70-serve-live.log` (10:53:59). Binary embeds packages + smoke ids + proof notes string + stage1 coaching lines.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-mstodo-googledocs-dropbox-prepare-open-judge.md`. Cross-cite from overnight status bullet in `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Sheets/Evernote/Slides line).
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-sheets-evernote-slides-prepare-open.md` — **present**, dated 2026-08-02. No prior judge file for this pack before this write (`Glob`/`ls`: only the evidence `.md`, no `*-judge.md`).
3. **Data I/O:** None — static markdown verdict; no structured data files; no secrets copied here.
4. **User instruction (verbatim):** Adversarial LLM-as-judge with FRESH context. Define strong quality bar from first principles FIRST, then grade. Overnight pack: Google Sheets, Evernote, Google Slides → Wave1Specs 67→70. Claimed: notes/write ×3; serve LIVE adapter_count=70 notes_adapters=6; evidence `saved-results/wave1-sheets-evernote-slides-prepare-open.md`; no commit. Deliverable: Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-sheets-evernote-slides-prepare-open-judge.md`. Cite overnight status with judge file. Independently inspect + confirm serve live. No questions, no commit, no secrets.

## First-principles bar (before hunting bugs)

A strong result for **this** task — add Google Sheets, Evernote, and Google Slides as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **67 → 70**) — must have:

1. **Product shape** — Operator prepares intent text and opens the official consumer app. The user finishes create/edit inside that app. Success is open-with-draft, not sheet/notebook/slide completion inside Operator.
2. **Honest verb + class choice** — All three = AppClass `notes`, verb **`write`** (Google Keep / Google Docs write peers for draft-open notes-family hand-offs). Outcomes and coaching must **never** claim sheet/notebook/slide created, saved, shared, or synced.
3. **Correct Android packages, live on Play** — Specs and `HandOffActions` must use Play Store ids that return live HTTP 200, not guessed dead ids.
4. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for all three Specs. No OAuth / completes adapter for this Wave-1 surface. Sheets/Slides are **not** Google Drive OAuth and **not** the googledocs Spec.
5. **Routing classes that exist** — notes only. No invented class. Ready log must show `notes_adapters=6` (keep+docs+dropbox+sheets+evernote+slides).
6. **End-to-end wiring** — Spec → ClassMap → stage1 coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` logs (`adapter_count=70`, notes includes `+googlesheets+evernote+googleslides`).
7. **Real tests** — Tests that go red if count/packages/verbs/empty-draft/wrong-verb reject/flow route/outcome bans/stage1 coaching/HandOffActions/ready-log counts break. ProvesCeiling locked in Spec want table for this pack. Shared execute ban list includes `sheet created` / `slide created` / `notebook created`.
8. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.
9. **Scope honesty** — No Telegram / Amazon / Strava in this pack. Separate from Drive OAuth + googledocs Spec. No commit if that was the ask. Overnight status + progress snapshot reflect Wave1Specs=70 and notes_adapters=6 without contradiction.
10. **Serve live, not log-only** — A running `serve-deeplink-proof` process with `adapter_count=70` and `notes_adapters=6` ready lines, not only a written claim.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**125** Go PASS lines on the evidence package set + HandOffActions BUILD SUCCESSFUL), all three Play packages HTTP **200**, serve is LIVE with `adapter_count=70` + `notes_adapters=6` + proof notes string (independently confirmed; pid **34436** matches evidence), Sheets/Slides stay separate from Drive OAuth and googledocs Spec, evidence + overnight batch bullet are present and honest about Pixel, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open, stale contradictory body text in the progress snapshot, Docs-family Play ambiguity for Sheets/Slides packages, and TDD red chronology not independently time-stamped.

## Findings (against the bar)

- **Wave1Specs count locked at 70 with three Specs last.** Indices 67–69 after dropbox at 66. Specs: `googlesheets` (`com.google.android.apps.docs.editors.sheets`, notes, `[write]`, `googlesheets_prepare_open_smoke`), `evernote` (`com.evernote`, notes, `[write]`, `evernote_prepare_open_smoke`), `googleslides` (`com.google.android.apps.docs.editors.slides`, notes, `[write]`, `googleslides_prepare_open_smoke`). Manifest path locks RT-4 / HandsOff / AuthNone / ConsentA. Runtime `flow_test.go` locks count 70 + ready log `notes_adapters=6`. Empty draft + wrong verbs rejected at indices 67–69.
- **Never claims completion on the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know”). Shared execute ban list includes `sheet created` / `slide created` / `notebook created` (plus prior `doc created`/`doc saved`/`saved`/`shared`/`synced`). Dedicated flow tests ban sheet/notebook/slide phrases. Stage1 coaches Google Sheets / Evernote / Google Slides with matching never-claim lines and Drive/googledocs separation (`client.go` + stage1 tests).
- **Play packages are the live ids.** Spec + HandOffActions + evidence use the three ids above. This session: all **200**; short/wrong ids **404**. `/tmp/codex-launcher-deeplink` embeds packages, smoke ids, and `googlekeep+googledocs+dropbox+googlesheets+evernote+googleslides`.
- **HandOffActions maps display names + short ids.** `google sheets`/`googlesheets`, `evernote`, `google slides`/`googleslides` → matching packages (`HandOffActions.kt` + unit test BUILD SUCCESSFUL).
- **ClassMap / proof wiring.** Runtime builds notes ClassMap from Spec AppClass (6 notes Specs). `deeplink_proof.go` logs notes including `+googlesheets+evernote+googleslides`. No Sheets/Slides/Evernote OAuth adapter packages; no Telegram/Amazon/Strava/Todoist rows in `Wave1Specs`.
- **Serve live independently confirmed.** Process running; ready lines show `adapter_count=70`, `notes_adapters=6`, notes proof string. Evidence log `/tmp/wave1-pack70-serve-live.log` and serve pid **34436** match this session (still alive at judge time).
- **Evidence + overnight batch honesty match this session.** Evidence records 67→70, ceilings, wiring, red→green, Play 200, Pixel on Pair, no commit. Overnight bullet claims 125 PASS + HandOffActions green + Play 200 + serve LIVE `adapter_count=70` pid 34436 — matches this session’s **125** `--- PASS:` count + HandOffActions BUILD SUCCESSFUL + Play checks + live serve. HEAD remains `5cb0831` (Wave 0); pack files still dirty/untracked — **no commit**, as claimed. TDD red chronology is implementer-reported (not independently time-stamped here); green locks are verified now.
- **Scope kept.** No Telegram / Amazon / Strava Specs. Sheets/Slides comments explicitly separate from Drive OAuth + googledocs Spec.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence + overnight: Pixel on Pair; companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **Progress snapshot body is stale / contradictory.** Header/user-ask claim Wave1Specs=70 and heartbeat ~33, but body still says notes **3** (`googlekeep+googledocs+dropbox`), “No Sheets/Evernote/…”, and points evidence/judge at the prior mstodo pack. Overnight batch bullet is correct; snapshot skim is not. Honesty gap on the cold-start summary, not on code/serve.
3. **Docs-family Play control is soft for Sheets/Slides.** `com.google.android.apps.docs` also returns HTTP **200**. Spec correctly uses `…editors.sheets` / `…editors.slides`; short wrong ids 404 help, but family-level ambiguity remains (same class of warning as googledocs judge).
4. **TDD red chronology not independently witnessed.** Evidence narrates red failures; this judge verified green locks only.
5. **Stale header comments elsewhere remain likely** (package-file / proof function headers historically lag packs). Not re-audited line-by-line beyond this pack’s Spec/proof notes keys, which are present and correct.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for Google Sheets / Evernote / Google Slides (`serve-deeplink-proof`). Fix `wave1-overnight-progress-snapshot.md` body to notes_adapters=6 + this pack’s evidence/judge paths (remove the “No Sheets/Evernote” line). Keep overnight serve log + pid in sync when restarting.
