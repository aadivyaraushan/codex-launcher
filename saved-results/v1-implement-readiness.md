# V1 implement readiness check

**Date:** 2026-08-03  
**What for:** Double-check blockers before messaging COMPLETE (Beeper P2) + Spotify/YouTube/Maps + HAND-OFF push.  
**Callers:** this chat; implement kickoff  
**Same-purpose:** extends `v1-credentials-setup.md`; no prior readiness file  
**Data:** key *presence* only (lengths), Beeper account network names + connection state (no tokens/identities), `adb` device id class, package names.  

**Owner instruction (verbatim):** `i think you should have all the reqs necessary to go and start implementing now. double check and lmk are there any other blockers left`

---

## Verdict

**Ready to start implementation.** No hard Owner blockers left for kickoff. First real gate is still the **P2 Beeper Server-on-phone spike** (fail → demote messaging COMPLETE per plan exit table).

---

## Verified green

| Check | Evidence |
|---|---|
| `.env` keys present | `BEEPER_ACCOUNT_EMAIL`, `BEEPER_ACCESS_TOKEN`, `SPOTIFY_CLIENT_ID/SECRET`, `SPOTIFY_REDIRECT_URI`, `GOOGLE_MAPS_API_KEY`, `YOUTUBE_API_KEY` all SET (non-empty) |
| Pixel on adb | `Pixel_9` / `tokay` listed as `device` |
| Beeper Desktop up | process running; `:23373` responds (401 without auth → API present) |
| Beeper nets for v1 | Desktop API `/v1/accounts` with Owner token: **Instagram**, **Discord**, **Google Messages** all `connected` (+ Beeper self) |
| Out of v1 nets | WhatsApp + Messenger / Facebook personal — not required |
| HAND-OFF apps on phone | Uber, DoorDash, Venmo packages present |
| Media/Maps apps | Spotify, Maps, YouTube packages present |
| Plans locked | messaging fork + consumer plan: IG/Discord/GM COMPLETE-after-smoke; WA/Messenger out; Spotify/YT/Maps COMPLETE; Netflix out |

---

## Soft / not blocking kickoff

| Item | Notes |
|---|---|
| Instagram *Android* app | Not seen in `pm list packages` (Beeper IG link is enough for COMPLETE path; HAND-OFF compose would need the app later if demoted) |
| SMS default app | `sms_default_application` returned `null`; Google Messages package + Beeper GM `connected` anyway — watch during GM smoke |
| Spotify Premium | Not verified; needed for playback smoke, not for coding against Web API |
| Dirty worktree | `worktree-phase0-notification-probe` has large unrelated Wave 1 diffs — use a **fresh worktree** for this push |
| P2 spike unproven | Expected: first implement step writes `saved-results/beeper-server-phone-linux-spike.md` |
| Desktop Beeper token | Lab only; product path still auto-provision + per-net link sheets |

---

## How to reproduce

```bash
# env key presence (lengths only)
# Pixel: adb devices -l
# Beeper accounts: GET http://127.0.0.1:23373/v1/accounts with Bearer token from .env
# packages: adb shell pm list packages | rg -i 'beeper|instagram|discord|messaging|spotify|maps|youtube|uber|doordash|venmo'
```
