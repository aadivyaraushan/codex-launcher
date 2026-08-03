# Wave 1 messaging extras prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-messaging-extras-prepare-open.md`  
**Overnight status cross-check:** `saved-results/wave1-overnight-batch-and-oauth-prep.md` (messaging extras bullet)  
**Code reviewed:** `adapters/deeplink` Wave1Specs (`whatsapp`, `messenger`, `signal`), `runtime/deeplink` ClassMap + flow tests, stage1 OpenAI coaching + tests, `HandOffActions` + unit test, `handoff.DraftOutcome`, `ReplyCapability` / `WatchList`, `deeplink_proof.go`  
**Plan rows checked:** `planning/consumer-app-implementation-plan.md` — WhatsApp / Signal / Messenger RT-4 compose `hands_off` class H (Wave 3 ceiling); notification-reply row separate and pending probe  
**Tests re-run (this session):**  
- `go test` adapters/deeplink + runtime/deeplink + stage1/openai → **96 passed**  
- `./android/gradlew -p android :app:testDebugUnitTest --tests app.codexlauncher.capability.handoff.HandOffActionsTest --rerun-tasks` → **BUILD SUCCESSFUL**  
- Play Store HTTP: `com.whatsapp` / `com.facebook.orca` / `org.thoughtcrime.securesms` → **200** each  

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`). Peer pattern: `wave1-lyft-keep-prepare-open-judge.md`, `wave1-discord-prepare-open-judge.md`. Overnight status may later link this path the same way it links other `*-judge.md` files; no code imports it.
2. **Existing peer evidence:** Task asked to inspect `saved-results/wave1-messaging-extras-prepare-open.md` — **present**, dated 2026-08-02. Overnight status cites it and Wave1Specs=30. Glob/Grep found **no** prior `wave1-messaging-extras-prepare-open-judge.md` (only the implementer evidence file).
3. **Data I/O:** None — static markdown verdict; no structured data files.
4. **User instruction (verbatim):** "Independent LLM-as-judge. Fresh context. Define strong from first principles, then grade. No implementer checklist.

## Task
Wave 1: WhatsApp, Messenger, Signal prepare-and-open compose hand-offs (Wave1Specs→30). Must reject send; notification reply stays separate.

Worktree: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe`

Inspect specs, HandOffActions, stage1, `saved-results/wave1-messaging-extras-prepare-open.md`. May run go tests.

Output Standard, Verdict, Findings, Gaps, next tip.
Write `saved-results/wave1-messaging-extras-prepare-open-judge.md` (2026-08-02). No product edits."

## First-principles bar (before hunting bugs)

A strong result for **this** task — add WhatsApp, Messenger, and Signal as Wave 1 prepare-and-open **compose** hand-offs (append after Lyft+Keep; `Wave1Specs` → **30**) — must have:

1. **Product shape** — From user-supplied context, prepare draft text, open the official consumer app, and stop. The user chooses the thread and sends. The operator must never claim the message was sent or delivered.
2. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off, verb **`compose` only**. **`send` must be rejected** on Spec/Resolve for all three — this is not a softer “prefer compose” rule.
3. **Notification reply stays a different product path** — Shade `RemoteInput` replies (if any) live under `ReplyCapability` / notification probe. This deeplink pack must not absorb, replace, or claim that path, and must not blur “opened compose” with “replied from shade.”
4. **Correct Android packages** — Launch targets match Play consumer ids: WhatsApp `com.whatsapp`, Messenger `com.facebook.orca`, Signal `org.thoughtcrime.securesms` — same ids on Spec and phone map.
5. **No unauthorized completion route** — No whatsapp-mcp / WhatsApp Web bridge, no signal-cli linked-device control, no Messenger browser-login or account-read path wired for this Wave-1 product surface; stage1 must coach prepare-and-open compose, not those routes.
6. **Live routing coaching** — Stage1 names each app with `messaging` + `compose` + prepare-and-open language, refuses send-completion claims, and keeps notification-reply framing out of this path’s success story.
7. **End-to-end wiring** — Spec → messaging ClassMap → stage1 coaching → Android display-name→package map → `DraftOutcome` (hands_off, no sent/delivered) → `serve-deeplink-proof` loads Wave1Specs and logs all three.
8. **Real tests** — Tests that go red if count/packages/compose-only/send-reject/empty-draft/flow route/outcome bans/stage1 coaching/HandOffActions break.
9. **Honest device evidence** — Pixel Auto→Open either verified or clearly withheld. Inventing a device proof is a Fail on honesty.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code, locked by focused tests re-run green this session, and the evidence file is honest about Pixel. Not a Fail. Warnings are open live smoke and a soft coaching-lock gap on the notification-reply separation phrase.

## Findings (against the bar)

- **Wave1Specs count locked at 30 with the three rows last.** Want table includes `whatsapp` / `messenger` / `signal` with packages `com.whatsapp` / `com.facebook.orca` / `org.thoughtcrime.securesms`, class `messaging`, verbs `[compose]` (`adapter_test.go` want rows + count assert). Shared Describe sets RT4 / HandsOff / ConsentA / AuthNone. Every Wave1 Spec asserts `must not allow send`.
- **Send is rejected on Resolve; empty draft fails.** Indices `[27]`–`[29]` empty-body → `ErrEmptyDraft`; `manifest.Send` → error (`adapter_test.go` messaging-extras block). Execute path uses `handoff.DraftOutcome` — detail tells the user to finish in-app and says Operator **cannot know** whether they finished; flow bans `sent` / `delivered` / `message sent` (`flow_test.go` `TestDeepLinkFlowRoutesMessagingComposeToWave1Extras`).
- **Notification reply remains separate.** `ReplyCapability` / `WatchList` already map the same packages (plus W4B / Messenger Lite aliases) for shade probe — left as its own path. Deeplink Spec comments and stage1 coaching explicitly say notification reply is a separate path; no whatsapp-mcp / signal-cli / Messenger OAuth packages exist under companion capability adapters (repo grep empty).
- **Android Open map matches Specs.** Keys `whatsapp` / `messenger` / `signal` → same packages (`HandOffActions.kt`). Unit asserts Title Case + lowercase for all three (`HandOffActionsTest.kt`). Play Store listings returned HTTP **200** this session.
- **Stage1 coaches compose prepare-and-open and refuses sent claims.** Instructions (`client.go`) for all three; dedicated test requires named app + messaging + compose + prepare-and-open and `never claim` + `sent` (`client_test.go` `TestStage1InstructionsCoachMessagingExtrasPrepareAndOpen`).
- **Proof serve names the trio.** `deeplink_proof.go` logs `messaging=messages+discord+teams+whatsapp+messenger+signal`. Runtime registers from `Wave1Specs()` into ClassMap by AppClass, so messaging includes the three once Specs exist.
- **Evidence + overnight honesty match this session.** Evidence records red→green, count 30, send reject, ReplyCapability untouched, Pixel unpaired. Overnight bullet claims go **96/96** + HandOffActions green + Pixel unpaired — matches this session’s **96** Go passes + HandOffActions **BUILD SUCCESSFUL** (forced `--rerun-tasks`).

## Gaps

1. **Live Auto→Open on Pixel still open.** Evidence: Pixel unpaired (Pair screen); companion path covered by Go unit/flow only; package launch on device not run. Honesty is good; the product UI stop-line is not closed.
2. **Stage1 test does not lock the notification-reply separation phrase.** Coaching text includes “(notification reply is a separate path)” for all three (`client.go`), and the test comment mentions ReplyCapability, but assertions only require `never claim` + `sent`. Someone could drop the separation note and keep tests green. Product separation still holds via untouched `ReplyCapability` + send reject + DraftOutcome bans.

## Next tip

Re-pair Pixel and run one Auto→Open smoke per app (`serve-deeplink-proof`), then tighten `TestStage1InstructionsCoachMessagingExtrasPrepareAndOpen` to require `notification reply` in the coaching string so the “stays separate” rule cannot silently regress in stage1 copy alone.
