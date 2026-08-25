# OpenAI metered-spend approval (money-rule record)

**Date:** 2026-08-07
**For:** Wiring the LLM stage-1 router onto the phone (Workstream A of
`planning/openai-beeper-phone-runtime-plan.md`) and adapter language work.

## Approved account

- **OPENAI_ACCOUNT:** `ssdear@gmail.com`
- **Kind:** personal (`OPENAI_ACCOUNT_KIND=personal`)
- **Org:** `org-oC0Cx9jwKVEEvRlRlqdQzTwE`
- **Credit on account:** ~$1,000 OpenAI credit (user-stated).
- **Model:** `gpt-5.6-luna` (per `stage1/openai/client.go`).

## Scope of approval

User explicitly approved metered OpenAI spend on this personal key in
autonomous mode, 2026-08-07, with the instruction "keep spend reasonable."
Applies to: stage-1 routing calls (one small call per utterance) and any
adapter language work that routes through the same key.

## Cost discipline I'll hold

- Routing calls are small (short instructions + JSON schema out). A dogfood
  session of a few dozen asks is low single-digit dollars.
- No bulk/loop calls against OpenAI. The discovery harness runs against the
  **deterministic** router by default (zero spend); the brokered OpenAI
  router is exercised only for targeted on-device acceptance asks, not the
  full corpus.
- Beeper read/reply work (Workstream B) is local Beeper Desktop API, no
  per-call cost.

## Source

Key + account metadata live in repo `.env` (git-ignored), lines flagged
"METERED account: every call here costs money."
