# Wave 1 Claude / ChatGPT / Grok prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-claude-chatgpt-grok-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Claude + ChatGPT + Grok bullet); `saved-results/wave1-overnight-progress-snapshot.md` (heartbeat ~36; Wave1Specs=76; messaging 14)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`claude`/`chatgpt`/`grok`), `adapter_test.go` want-table + shared execute bans, `runtime/deeplink` ClassMap + flow + ready-log + outcome-ban tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `deeplink_proof.go`  
**Tests / Play / serve re-run (this session):**  
- Same Go package set as evidence (`-count=1 -v`) → **132** `--- PASS:` lines, **0** FAIL, 5 packages ok  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks` → **BUILD SUCCESSFUL**  
- Play Store HTTP: `com.anthropic.claude` / `com.openai.chatgpt` / `ai.x.grok` → **200**; wrong ids `com.anthropic.claude.ai` / `com.openai.chat` / `com.x.grok` → **404**  
- Serve LIVE this session: process `/tmp/codex-launcher-deeplink serve-deeplink-proof` (pid **98639**, matches evidence); ready lines `adapter_count=76`, `messaging_adapters=14`, messaging `…+claude+chatgpt+grok` in `/tmp/wave1-pack76-serve-live.log` (11:39:10). Binary embeds packages + smoke ids + proof messaging string + stage1 coaching lines.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-pocketcasts-goodreads-kindle-prepare-open-judge.md`. Cross-cite from overnight status bullet in `saved-results/wave1-overnight-batch-and-oauth-prep.md` (Claude/ChatGPT/Grok line) and `saved-results/wave1-overnight-progress-snapshot.md` line 20 (Latest pack evidence).
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-claude-chatgpt-grok-prepare-open.md` — **present**, dated 2026-08-02. Glob/Grep/ls: only the evidence `.md` exists; no `wave1-claude-chatgpt-grok-prepare-open-judge.md` before this write (`ls`: No such file).
3. **Data I/O:** None — static markdown verdict; no structured data files; no secrets copied here.
4. **User instruction (verbatim):** Adversarial LLM-as-judge with FRESH context. Define strong quality bar from first principles FIRST, then grade. Overnight pack: Claude, ChatGPT, Grok → Wave1Specs 73→76 in `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`. Claimed: messaging/compose ×3; serve LIVE adapter_count=76 messaging_adapters=14; evidence `saved-results/wave1-claude-chatgpt-grok-prepare-open.md`; no commit. Deliverable: Write Pass / Pass-with-warnings / Fail to `saved-results/wave1-claude-chatgpt-grok-prepare-open-judge.md`. Cite overnight status with judge file. Independently inspect + confirm serve live. No questions, no commit, no secrets.

## First-principles bar (before hunting bugs)

A strong result for **this** task — add Claude, ChatGPT, and Grok as Wave 1 prepare-and-open hand-offs (`Wave1Specs` **73 → 76**) — must have:

1. **Product shape** — Operator prepares prompt/draft text and opens the official consumer app. The user finishes the chat inside that app. Success is open-with-draft, not a completed reply/answer from Operator.
2. **Honest verb + class choice** — All three = AppClass `messaging`, verb **`compose`** (draft prompt / open official app). Outcomes and coaching must **never** claim replied / sent / answered / completed chat.
3. **No partner API for these apps** — This pack must not call Anthropic / OpenAI ChatGPT / xAI APIs. Hand-off packages only. Stage1 may still use OpenAI as the *routing* model (existing stage1 path); that is not a Claude/ChatGPT/Grok completes adapter.
4. **Correct Android packages, live on Play** — Specs and `HandOffActions` must use Play Store ids that return live HTTP 200, not guessed dead ids.
5. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off for all three Specs. No OAuth / completes adapter for this Wave-1 surface.
6. **Routing class that exists** — messaging only. No invented class. Ready log must show `messaging_adapters=14` (+claude+chatgpt+grok).
7. **End-to-end wiring** — Spec → ClassMap → stage1 coaching → Android display-name→package map (including `chat gpt` alias) → `DraftOutcome` → `serve-deeplink-proof` logs (`adapter_count=76`, messaging includes `+claude+chatgpt+grok`).
8. **Real tests** — Tests that go red if count/packages/verbs/empty-draft/wrong-verb reject/flow route/outcome bans/stage1 coaching/HandOffActions/ready-log counts break. ProvesCeiling locked in Spec want table for this pack. Shared execute ban list includes `replied` / `answered`.
9. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.
10. **Scope honesty** — No Anthropic/OpenAI/xAI completes adapters for these three apps. No commit if that was the ask. Overnight status + progress snapshot reflect Wave1Specs=76 and messaging=14 without contradiction.
11. **Serve live, not log-only** — A running `serve-deeplink-proof` process with `adapter_count=76` and `messaging_adapters=14` ready lines, not only a written claim.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session (**132** Go PASS lines on the evidence package set + HandOffActions BUILD SUCCESSFUL), all three Play packages HTTP **200**, serve is LIVE with `adapter_count=76` + `messaging_adapters=14` + proof messaging `+claude+chatgpt+grok` (independently confirmed; pid **98639** matches evidence), no Claude/ChatGPT/Grok API adapter packages were added, evidence + overnight batch bullet + progress snapshot are present and honest about Pixel, and HEAD was not advanced for this pack (`5cb0831`). Not a Fail. Warnings are open Pixel Auto→Open, TDD red chronology not independently time-stamped, and a stale `startDeepLinkProof` header comment.

## Findings (against the bar)

- **Wave1Specs count locked at 76 with three Specs last.** Indices 73–75 after kindle at 72. Specs: `claude` (`com.anthropic.claude`, messaging, `[compose]`, `claude_prepare_open_smoke`), `chatgpt` (`com.openai.chatgpt`, messaging, `[compose]`, `chatgpt_prepare_open_smoke`), `grok` (`ai.x.grok`, messaging, `[compose]`, `grok_prepare_open_smoke`). Manifest path locks RT-4 / HandsOff / AuthNone / ConsentA. Runtime `flow_test.go` locks count 76 + ready log `messaging_adapters=14`. Empty draft + `send` rejected per id.
- **Never claims completion on the hand-off path.** Execute uses `handoff.DraftOutcome` (“cannot know”). Shared execute ban list includes `replied` / `answered` (plus prior `sent` / `completed` tokens that also cover “completed chat”). Dedicated flow test bans pack-specific phrases. Stage1 coaches Claude / ChatGPT / Grok with matching never-claim lines and explicit “Operator does not call … APIs here.”
- **Play packages are the live ids.** Spec + HandOffActions + evidence use the three ids above. This session: all **200**; short/wrong ids **404**. `/tmp/codex-launcher-deeplink` embeds packages, smoke ids, messaging `…+claude+chatgpt+grok`.
- **HandOffActions maps display names + short ids + ChatGPT space alias.** `claude`, `chatgpt`, `chat gpt`, `grok` → matching packages (`HandOffActions.kt` + unit test BUILD SUCCESSFUL).
- **ClassMap / proof wiring.** Runtime builds messaging ClassMap from Spec AppClass (14 messaging Specs including claude+chatgpt+grok). `deeplink_proof.go` logs match. No `claude`/`chatgpt`/`grok` adapter directories under `capability/adapters` (only deeplink Specs).
- **Serve live independently confirmed.** Process running (pid **98639**); ready lines show `adapter_count=76`, `messaging_adapters=14`, proof messaging string. Evidence log `/tmp/wave1-pack76-serve-live.log` matches this session (still alive at judge time).
- **Evidence + overnight + snapshot honesty match this session.** Evidence records 73→76, ceilings, wiring, red→green, Play 200, Pixel on Pair, no commit. Overnight bullet claims 132 PASS + HandOffActions green + Play 200 + serve LIVE `adapter_count=76` pid 98639 — matches this session. Progress snapshot heartbeat ~36 already lists Wave1Specs=76, messaging 14, Claude/ChatGPT/Grok compose hand-offs only (no partner APIs). HEAD remains `5cb0831` (Wave 0); pack files still dirty/untracked — **no commit**, as claimed. TDD red chronology is implementer-reported (not independently time-stamped here); green locks are verified now.
- **Scope kept.** Messaging/compose only. No Anthropic / ChatGPT / xAI completes adapters. Comments + stage1 coaching explicitly say Operator does not call those APIs here.

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence + overnight: Pixel on Pair; companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **TDD red chronology not independently witnessed.** Evidence narrates red failures; this judge verified green locks only.
3. **Stale header comment on `startDeepLinkProof`.** Function body proof strings include Claude/ChatGPT/Grok; the package/function header comment still describes an older Apple Music overnight ask. Ops/human signal lag only — runtime still registers from `Wave1Specs()`.

## Next tip

Re-pair Pixel and run one Auto→Open smoke each for Claude / ChatGPT / Grok (`serve-deeplink-proof`). Keep overnight serve log + pid in sync when restarting. Leave Anthropic/OpenAI/xAI completes adapters out of Wave1Specs unless product scope changes. Refresh the `startDeepLinkProof` header comment on the next pack touch.
