# Wave 1 Microsoft To Do / Google Docs / Dropbox prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-mstodo-googledocs-dropbox-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Microsoft To Do + Google Docs + Dropbox bullet); `saved-results/wave1-overnight-progress-snapshot.md` (heartbeat ~32, `Wave1Specs`=67)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`mstodo`/`googledocs`/`dropbox`), `adapter_test.go`, `runtime/deeplink` ClassMap + flow + ready-log tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Tests / Play / serve re-run (this session):**  
- Same Go package set as evidence → **123** `--- PASS:` lines, **0** FAIL, 5 packages ok  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks` → **BUILD SUCCESSFUL**  
- Play Store HTTP: `com.microsoft.todos` / `com.google.android.apps.docs.editors.docs` / `com.dropbox.android` → **200**; wrong ids `com.microsoft.todo` / `com.dropbox` → **404** (`com.google.android.apps.docs` also 200 — Docs family; Spec correctly uses editors package)  
- Serve LIVE this session: process `/tmp/codex-launcher-deeplink serve-deeplink-proof` (parent pid **17092**, child **17222**, worktree cwd); ready lines `adapter_count=67`, `tasks_adapters=3`, `notes_adapters=3`, `tasks=asana+trello+mstodo`, `notes=googlekeep+googledocs+dropbox` (terminal capture 10:40:56 + prior `/tmp/wave1-pack67-serve-live.log` 10:39:29). Binary embeds packages + smoke ids + proof strings.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-pandora-asana-trello-prepare-open-judge.md`.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-mstodo-googledocs-dropbox-prepare-open.md` — **present**, dated 2026-08-02. No prior judge file for this pack before this write.
3. **Data I/O:** None — static markdown verdict; no structured data files; no secrets copied here.
4. **User instruction (verbatim):** adversarial LLM-as-judge with fresh context; define strong from first principles then grade; Pass / Pass-with-warnings / Fail; write this path; cite overnight status with judge file; independently inspect + confirm serve live; no questions; no commit; no secrets.

## First-principles bar (before hunting bugs)

A strong result for **this** task — add Microsoft To Do, Google Docs, and Dropbox as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **64 → 67**) — must have:

1. **Product shape** — Operator prepares intent text and opens the official consumer app. The user finishes create/edit/browse inside that app. Success is open-with-draft (or open-to-browse), not task/doc/file completion inside Operator.
2. **Honest verb + class choice** — Microsoft To Do = AppClass `tasks`, verb **`write`** (Asana/Trello peer). Google Docs = AppClass `notes`, verb **`write`** (Google Keep peer for notes, write not read). Dropbox = AppClass `notes`, verb **`read`** (browse/open file). Outcomes and coaching must **never** claim task created/assigned/completed, doc created/saved/shared, or uploaded/downloaded/shared/synced.
3. **Correct Android packages, live on Play** — Specs and `HandOffActions` must use Play Store ids that return live HTTP 200, not guessed dead ids.
4. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for all three Specs. No OAuth / completes adapter for this Wave-1 surface. **Not** Todoist RT-2 (different package/id; no Todoist deeplink Spec). Google Docs is **not** Google Drive OAuth.
5. **Routing classes that exist** — tasks / notes. No invented class. Ready log must show `tasks_adapters=3` and `notes_adapters=3`.
6. **End-to-end wiring** — Spec → ClassMap → stage1 coaching → Android display-name→package map → `DraftOutcome` → `serve-deeplink-proof` logs (`adapter_count=67`, tasks includes `mstodo`, notes includes `googledocs`+`dropbox`).
7. **Real tests** — Tests that go red if count/packages/verbs/empty-draft/wrong-verb reject/flow route/outcome bans/stage1 coaching/HandOffActions/ready-log counts break. ProvesCeiling locked in Spec want table for this pack.
8. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.
9. **Scope honesty** — No Sheets / Evernote / Telegram / Amazon / Strava in this pack. No commit if that was the ask. Overnight status + progress snapshot reflect Wave1Specs=67.
10. **Serve live, not log-only** — A running `serve-deeplink-proof` process with `adapter_count=67` ready lines, not only a written claim.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**123** Go PASS lines on the evidence package set + HandOffActions BUILD SUCCESSFUL), all three Play packages HTTP **200**, serve is LIVE with `adapter_count=67` + `tasks_adapters=3` + `notes_adapters=3` + proof task/notes strings (independently confirmed), Todoist is not a Wave1 deeplink Spec, evidence + overnight status/progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open, serve pid/log churn vs evidence’s pid **10679**, thinner shared ban wording for doc-created phrases, stale package-file header comment, and Google Docs family package ambiguity on wrong-id control.

## Findings (against the bar)

- **Wave1Specs count locked at 67 with three Specs last.** Want table includes `mstodo` (`com.microsoft.todos`, tasks, `[write]`), `googledocs` (`com.google.android.apps.docs.editors.docs`, notes, `[write]`), `dropbox` (`com.dropbox.android`, notes, `[read]`) with ProvesCeiling smoke ids (`adapter_test.go`). Indices 64–66 after Trello at 63. Runtime `flow_test.go` also locks `len(Wave1Specs()) == 67` and ready log `tasks_adapters=3` / `notes_adapters=3`. Empty draft + wrong verbs rejected (mstodo rejects send/compose; googledocs rejects send/read; dropbox rejects write/send).
- **Never claims completion on the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know”). Shared execute ban list includes `task created` / `assigned` / `completed` plus `uploaded` / `downloaded` / `shared` (and prior `synced`). Dedicated flow tests ban mstodo task phrases and googledocs/dropbox doc/file phrases. Stage1 coaches Microsoft To Do / Google Docs / Dropbox with matching never-claim lines and “Not the Todoist completes adapter” / “Separate from Google Drive OAuth” (`client.go` + stage1 tests).
- **Play packages are the live ids.** Spec + HandOffActions + evidence use the three ids above. This session: all **200**; `com.microsoft.todo` / `com.dropbox` **404**. `/tmp/codex-launcher-deeplink` embeds packages, smoke ids, `asana+trello+mstodo`, `googlekeep+googledocs+dropbox`.
- **HandOffActions maps display names + short ids.** `microsoft to do`/`mstodo`, `google docs`/`googledocs`, `dropbox` → matching packages (`HandOffActions.kt` + unit test). Wire uses Spec `AppName`, so Confirm→open happy path matches.
- **ClassMap / proof wiring.** Runtime builds tasks/notes ClassMap from Spec AppClass. `deeplink_proof.go` logs `adapter_count` via `len(Wave1Specs())`, `tasks=asana+trello+mstodo`, `notes=googlekeep+googledocs+dropbox`. No `mstodo`/`googledocs`/`dropbox` OAuth adapter packages; no `todoist` row in `Wave1Specs`.
- **Serve live independently confirmed.** Process running from worktree binary; ready lines show `adapter_count=67`, `tasks_adapters=3`, `notes_adapters=3`, tasks/notes proof strings. Prior evidence log `/tmp/wave1-pack67-serve-live.log` matches the same fields (pid **10679** instance). Current live restart pid differs (parent **17092** / child **17222**) — still LIVE with same counts.
- **Evidence + overnight honesty match this session.** Evidence records 64→67, ceilings, wiring, red→green, Play 200, Pixel on Pair. Overnight bullet claims 123 PASS + HandOffActions green + Play 200 + serve LIVE `adapter_count=67` — matches this session’s **123** `--- PASS:` count + HandOffActions BUILD SUCCESSFUL + Play checks + live serve. Progress snapshot heartbeat ~32 lists Wave1Specs=67, tasks 3, notes 3, Todoist remains RT-2. HEAD remains `5cb0831` (Wave 0); pack files still dirty/untracked — **no commit**, as claimed. TDD red chronology is implementer-reported (not independently time-stamped here); green locks are verified now.
- **Scope kept.** No Sheets / Evernote / Telegram / Amazon / Strava Specs in adapters/deeplink. No Todoist deeplink Spec.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence + overnight: Pixel on Pair; companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **Serve pid/log churn vs evidence claim.** Evidence cites pid **10679** and `/tmp/wave1-pack67-serve-live.log`. That instance shut down; this judge session found a later LIVE restart (ready lines at 10:40:56) with the same `adapter_count=67` fields. Not a Fail on the claim (serve is live with 67); evidence pid is stale.
3. **Shared execute ban list omits explicit `doc created` / `doc saved` phrases.** It has `uploaded`/`downloaded`/`shared` and `task created`/`completed`. Google Docs dedicated flow bans bare `created`/`saved`/`shared` (broad; works today because DraftOutcome does not use those words). A regression that said “doc created” without hitting shared tokens might only trip the dedicated flow test, not the shared table.
4. **Package file header comment is stale.** `adapter.go` package comment still lists through Citymapper / earlier packs and does not name Microsoft To Do / Google Docs / Dropbox (or many intervening packs), even though Specs are present.
5. **`deeplink_proof.go` function header comment is stale** (still describes early Venmo/…/Apple Music draft-and-open). Runtime behavior and ready log fields are correct.
6. **Wrong-id Play control for Docs is soft.** `com.google.android.apps.docs` also returns HTTP **200** (Docs family). Spec correctly uses `…editors.docs`; alternate wrong ids that 404 are clearer for To Do / Dropbox than for Docs.
7. **TDD red chronology not independently witnessed.** Evidence narrates red failures; this judge verified green locks only.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for Microsoft To Do / Google Docs / Dropbox (`serve-deeplink-proof`). Then: add `doc created` / `doc saved` to the shared execute ban list (or assert those exact phrases on the googledocs shared cases), refresh the `adapter.go` / `deeplink_proof.go` header comments, and keep overnight serve log + pid in sync when restarting so evidence does not cite a dead pid.
