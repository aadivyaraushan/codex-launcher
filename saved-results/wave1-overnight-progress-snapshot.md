# Wave 1 overnight progress snapshot

**Date:** 2026-08-02 (heartbeat ~47)  
**Purpose:** Cold-start summary of what shipped overnight without Approves / Pixel pairing, and what is still blocked.

**Callers:** continuous consumer-plan loop; overnight status file.  
**API / schema:** none (status artifact).  
**User ask (verbatim):** Wave1Specs 73→76 Claude/ChatGPT/Grok prepare-and-open; overnight status + snapshot heartbeat ~36.

## Inputs → Outputs → Algorithm (session)

1. **Inputs:** Plan Wave 1 rows; Play package verification; existing OAuth keys in main `.env` (Slack/Google/Microsoft present; Telegram absent).
2. **Outputs:** Deeplink pack growth, RT-6 Reminders, OAuth unit+start for Group B, work Teams Graph adapter, Podcasts `serve-podcasts-proof`, evidence under `saved-results/wave1-*`.
3. **Algorithm:** Prefer agent-only work (hand-offs, unit adapters); skip owner browser Approve and unpaired Pixel Auto→Open; record every pack in evidence + overnight status.

## Deeplink prepare-and-open pack

Current serve: **`Wave1Specs` = 76** (plus separate Instagram adapter).

Includes money/food/media/messaging/rides/travel/services/finance/notes/shopping/tasks packs: Venmo→Citymapper airlines, YouTube, Airbnb/OpenTable/Grubhub, Threads/TikTok/Expedia/Pinterest, Target/Walmart/Nike/Sephora/Wayfair/eBay, Kayak/Priceline, LinkedIn, WhatsApp/Messenger/Signal, Netflix/Facebook, Lyft/Keep, Maps, Duolingo/Fitbit (services read), Shazam/Chromecast/YouTube Music/SoundCloud/Pandora/Pocket Casts/Kindle (media), Asana/Trello/Microsoft To Do (tasks write), Google Docs/Sheets/Slides + Evernote + Goodreads (notes) + Dropbox (notes read), etc. All `hands_off`; Play packages HTTP-checked where added. Shopping is browse/open only (no UCP cart API; no bid/checkout claims). Travel read includes Kayak+Priceline (book demoted). LinkedIn/Pinterest are messaging compose (never posted/commented/pinned/saved). Services 4 adapters; media 13; messaging 14 (includes Claude/ChatGPT/Grok compose); tasks 3 (asana+trello+mstodo); notes 7 (googlekeep+googledocs+dropbox+googlesheets+evernote+googleslides+goodreads). No Telegram/Amazon shopping/Strava. Kindle is reader prepare-and-open only. Claude/ChatGPT/Grok are official-app hand-offs only (no partner APIs). Todoist remains RT-2 (not a deeplink Spec). Podcasts RT-2 RSS remains separate from Pocket Casts. Latest pack evidence: `wave1-claude-chatgpt-grok-prepare-open.md`. Judge Pass-with-warnings: `wave1-claude-chatgpt-grok-prepare-open-judge.md`. ProvesCeiling locked per Spec; shared claim bans include lesson/workout/identified/cast/library/station/task/card/doc/sheet/slide/notebook/uploaded/downloaded/shared/subscribed/shelved/rated/replied/answered tokens.

## Other adapters (not Wave1Specs)

| Item | Status |
|---|---|
| Todoist RT-2 | Pixel stop-line previously proven |
| Instagram draft-open | Separate adapter + Pixel OPEN earlier |
| Apple Notes RT-6 | Existing Mac osascript |
| Apple Reminders RT-6 | Shipped overnight (`apple-reminders`, proveadapter) |
| Podcasts RSS | Unit green + `serve-podcasts-proof` CLI (feed URL env; live smoke skipped); serve judge Pass-with-warnings — `wave1-podcasts-serve-proof-judge.md` |
| Slack / Google Cal+Drive / Outlook | Unit + authorize **start**; live Approve owner-gated |
| Personal Teams | Deeplink compose only |
| Work Teams Graph (`msteams`) | Unit green; Chat.ReadWrite via `organizations` tenant; Approve owner-gated |
| Notion MCP | Exists; Approve owner-gated |
| Telegram | Blocked — no `api_id`/`api_hash` |
| Gmail | Parked (restricted/CASA) |

## Android UX fix

Home Send disabled until `selection` and draft `version` present (silent no-op fix). Tests green; debug APK installed. Pixel later showed **Pair** screen — Auto→Open blocked until re-pair.

## Owner walls (unchanged)

1. ~~Re-pair Pixel~~ **DONE**; Auto→Open PASS ×4 (`wave1-auto-open-smoke-hb41.md`). **hb47: Pixel adb offline** — replug USB (`wave1-pixel-adb-offline-hb47.md`)  
2. Browser Approve: Slack, Google, Microsoft (mail + work Teams chat), Notion  
3. Telegram `TELEGRAM_API_ID` / `TELEGRAM_API_HASH`  
4. Redirect URI alignment notes in overnight OAuth evidence files  

## Remaining walls (heartbeat ~47)

Pixel re-paired hb40 (`wave1-companion-restart-pixel-repair.md`). See `wave1-overnight-remaining-walls.md` (judge Pass-with-warnings: `wave1-overnight-remaining-walls-judge.md`) — OAuth Approves + Telegram keys are the Wave 1 exit blockers; further deeplink Spec growth is optional coverage only. Redirect cheat sheet: `wave1-oauth-redirect-alignment.md`. Approve sequence: `wave1-oauth-approve-runbook.md`.

## How to reuse

Read `saved-results/wave1-overnight-batch-and-oauth-prep.md` Status section for the chronological bullet list; this file is the skim summary.
