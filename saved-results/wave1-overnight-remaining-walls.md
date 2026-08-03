# Wave 1 overnight — remaining walls

**Date:** 2026-08-02 (heartbeat ~50)  
**Purpose:** Cold-start view of what still blocks Wave 1 exit after overnight agent-only work.  
**Callers:** continuous consumer-plan loop; owner wake-up triage.  
**User ask:** Continue Wave 1 until done; prefer agent-only work; skip owner-only outward asks unless unblocked.

## Inputs → Outputs → Algorithm

1. **Inputs:** Plan Wave 1 exit; overnight status/snapshot; live env (keys present/absent); Pixel Auto→Open smokes; OAuth authorize-start probes.  
2. **Outputs:** Ordered remaining walls; what agent already shipped; what only the owner can clear.  
3. **Algorithm:** Separate agent-done vs owner-blocked; do not invent more low-value hand-offs as "progress" when exit is gated on Approve/keys.

## Verified now (heartbeat ~50)

| Check | How verified | Result |
|---|---|---|
| Pixel `4B230DLAQ001Z5` | `adb devices` hb47 | **adb offline** (USB unplugged or unauthorized). Pair record + prior Auto→Open PASS still stand — see `wave1-pixel-adb-offline-hb47.md` |
| Auto→Open live | warm adb smokes (no force-stop) | **PASS** Maps + Spotify + YouTube + WhatsApp — `wave1-auto-open-smoke-hb41.md` |
| Deeplink serve | pgrep + prior ready log | LIVE pid **52653**, `adapter_count=76` |
| Slack / Google / Microsoft client IDs | presence-only `.env` scan | **set** |
| OAuth authorize *start* | `go test` LiveOAuthStart (hb46) | **PASS** Slack 200 / Google 302 / Microsoft 302 — `/tmp/wave1-oauth-start-hb46-v.log` |
| Telegram keys | same scan | **missing** |
| `PODCASTS_FEED_URL` / Notion token | same scan | **missing** |

## What overnight already shipped (agent-only)

- Deeplink prepare-and-open pack **`Wave1Specs` = 76** (plus separate Instagram).
- Claim-ban / ProvesCeiling hardening; work Teams Graph unit; Podcasts CLI; Apple Reminders RT-6; Home Send gate fix.
- Companion restart + Pixel re-pair (`wave1-companion-restart-pixel-repair.md`).
- Four-class Auto→Open: travel (Maps), media (Spotify/YouTube), messaging (WhatsApp).
- OAuth authorize-start still green (hb39 + hb46).
- Chronological: `wave1-overnight-batch-and-oauth-prep.md`; skim: `wave1-overnight-progress-snapshot.md`.

## Owner walls (Wave 1 exit blockers)

Ordered most-blocking first:

1. ~~Re-pair Pixel~~ **DONE** (pair record). **Replug USB / restore adb** — hb47 offline (`wave1-pixel-adb-offline-hb47.md`)  
2. ~~Auto→Open against 76-Spec serve~~ **DONE** (4 adapters; further Spec smokes optional)  
3. **Browser Approve** for Slack, Google (Cal+Drive), Microsoft Outlook mail, Microsoft work Teams chat (`msteams`), and Notion (Notion may also need a token/connection depending on adapter path) — runbook: `wave1-oauth-approve-runbook.md`  
4. **Telegram keys** `TELEGRAM_API_ID` / `TELEGRAM_API_HASH`  
5. **Optional:** `PODCASTS_FEED_URL` for live Podcasts proof smoke  
6. **Parked:** Gmail (CASA)  
7. **Redirect URI alignment** soft wall — `wave1-oauth-redirect-alignment.md` (Slack HTTPS **9192**, Google **9194**, Outlook **9195**, Teams **9196**)

## Agent-only work left (thin)

Pack growth and more Auto→Open spot-checks are optional coverage, **not** Wave 1 exit. Prefer idle/thin docs until Approves or Telegram keys land. Do **not** build: Amazon shopping (C2), Strava, dating (C3), Reddit commercial.

## How to reuse

1. Approve OAuth apps via `wave1-oauth-approve-runbook.md` (one serve at a time; leave deeplink serve alone).  
2. Add Telegram keys → Telegram adapter (TDD).  
3. Optionally set `PODCASTS_FEED_URL` → `serve-podcasts-proof`.
