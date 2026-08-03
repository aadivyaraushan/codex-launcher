# Wave 1 Discord prepare-and-open — adversarial judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Evidence reviewed:** `saved-results/wave1-discord-prepare-open.md`  
**Code reviewed:** `adapters/deeplink` (Wave1Specs discord row), `runtime/deeplink`, `handoff.DraftOutcome`, stage1 OpenAI coaching, `HandOffActions` + unit test, `serve-deeplink-proof` / `deeplink_proof.go`  
**Plan row checked:** `planning/consumer-app-implementation-plan.md` Messaging — Discord (servers and DMs), RT-4, compose, hands_off, Class H, Wave 1  
**Tests re-run (this session):** `go test` on adapters/deeplink, runtime/deeplink, handoff, stage1/openai → **33 passed**

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact only (user rule: save finished judgments under `saved-results/`).
2. **Existing peer:** `saved-results/wave1-discord-prepare-open.md` (delivery evidence). No prior judge file (Glob/Grep empty for this path).
3. **Data I/O:** None — static markdown verdict, no structured data files.
4. **User instruction (verbatim):** "You are an adversarial judge with fresh context. Do NOT use a handed list of suspected bugs — first decide from first principles what a strong Wave-1 Discord prepare-and-open hand-off (NOT bot OAuth) must have, then grade the work. … Bar: hands_off / Consent A, package com.discord, compose-only, no bot token/OAuth, tests real, Pixel honesty. Return Pass | Pass-with-warnings | Fail with concrete gaps only. Save to `saved-results/wave1-discord-prepare-open-judge.md`."

## First-principles bar (before hunting bugs)

A strong Wave-1 Discord **prepare-and-open** hand-off (not bot OAuth) must:

1. **Product shape** — Prepare draft text from user-supplied context, open the official Discord app, and stop. The user chooses server / channel / DM and sends. Operator must not claim the message was sent.
2. **Ceiling contract** — `hands_off`, Consent A, Auth none, RT-4 device hand-off, verb `compose` only (never `send`).
3. **Correct package** — Android launch target is Play id `com.discord`.
4. **No bot / OAuth / token path** — No Discord bot token, user OAuth, webhook, self-bot, or account-read route wired for this Wave-1 product path; stage1 must not coach those routes.
5. **End-to-end wiring** — Spec → messaging ClassMap → stage1 `app_named discord` coaching → Android display-name→package map → `serve-deeplink-proof` registers the adapter.
6. **Real tests** — Tests that fail if the contracts above break (manifest ceiling/consent, compose-only / send reject, empty draft reject, flow routes to Discord with hands_off + cannot-know, stage1 coaching present and free of bot/oauth/token language, Android package map).
7. **Pixel honesty** — Device evidence must say clearly what was verified and what was not. Claiming a full Auto→Open proof when only package launch ran is a Fail on honesty; leaving Auto→Open open while documenting monkey open is allowed under this bar.

## Verdict: **Pass-with-warnings**

Core contracts above are met in code and covered by tests re-run green in this session. Not a Fail. Pixel section is honest.

## Concrete gaps only

1. **Full Auto → Open Discord Pixel smoke still open.**  
   Evidence: three adb Auto attempts never reached companion prepare (`[deeplink]` / stage1 logs absent). Only `monkey -p com.discord` launch was verified (`com.discord.main.MainDefault`). Code + unit path is green; live stop-line for the product UI flow is not closed.

## Focus checklist

| Focus | Grade |
|---|---|
| hands_off / Consent A | Pass — `Describe()` sets `Ceiling=HandsOff`, `Consent=ConsentA`, `Auth=AuthNone`; execution uses `handoff.DraftOutcome`; adapter + flow tests assert hands_off / Done / cannot-know |
| package `com.discord` | Pass — Wave1Specs + `HandOffActions` + Android unit assert; Play/device note in evidence |
| compose-only | Pass — verbs `[compose]`; `Allows(send)` forbidden; Resolve rejects `send` for discord |
| no bot token / OAuth | Pass — no `oauth/discord` package; deeplink path Auth none; stage1 test rejects bot/oauth/token/self-bot coaching; serve needs `OPENAI_API_KEY` only |
| tests real | Pass — Wave1Specs row, empty/send reject, flow route `app_named discord` → HandedOffTo Discord, stage1 coaching, HandOffActions package; 33 Go tests green this session |
| Pixel honesty | Pass (with gap #1) — evidence explicitly states Auto→Open not completed; does not invent a full UI proof |

## Evidence cross-check (not gaps unless they break the bar)

- Plan build note (“No bot, webhook, self-bot, account read, or Discord token”) matches shipped shape.
- Messaging ClassMap includes both `messages` and `discord`; stage2 `EqualFold` named-adapter filter is exercised by `TestDeepLinkFlowRoutesMessagingComposeToDiscord`.
- Instagram remains a separate proof ClassMap; overnight Discord bot OAuth prep notes are correctly treated as superseded for this Wave-1 product path.
