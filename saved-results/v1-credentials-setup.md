# V1 credentials setup — unblock autonomous implement

**Date:** 2026-08-02  
**What for:** URLs + env keys so Owner can create OAuth/API credentials before messaging + media + Maps + HAND-OFF push.  
**Callers:** this chat; implementer unblock; main `.env`  
**Same-purpose search:** no prior `v1-credentials-setup.md`  
**Data:** env key names only (no secret values). Date ISO in header.  
**User instruction:** get Beeper and other credentials ready; open URLs for Owner to fill credentials  

**Secrets live in:** main checkout `.env` (gitignored). Never paste tokens into chat.

**Lab vs product:** These Beeper/Spotify/Google values unblock **Owner spike + Operator’s shared Spotify/GCP app credentials**. Product UX: Operator **auto-provisions** Beeper/runtime; user only does short per-net link sheets (see operator-complete-messaging-plan.md Invisible setup UX). Do not ship Owner `BEEPER_ACCESS_TOKEN` as the multi-user path.  
**Out of this push:** Netflix, Signal/iMessage COMPLETE, dating, Amazon.

---

## Tabs opened for you

1. [Beeper download](https://www.beeper.com/download) — install Desktop (Mac) for account + later phone Server auth  
2. [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) — create Web API app  
3. [Google Cloud — YouTube Data API](https://console.cloud.google.com/apis/library/youtube.googleapis.com) — enable  
4. [Google Cloud — Places API](https://console.cloud.google.com/apis/library/places-backend.googleapis.com) — enable (or Places API New if prompted)  
5. [Google Cloud — Routes API](https://console.cloud.google.com/apis/library/routes.googleapis.com) — enable  
6. [Google Cloud — Credentials](https://console.cloud.google.com/apis/credentials) — create API key / OAuth client  
7. [Beeper Desktop API auth docs](https://developers.beeper.com/desktop-api/auth) — how tokens work (phone Server later)

---

## What to create / fill

### A. Beeper (no cloud API key — account + later localhost token)

| Step | Action |
|---|---|
| 1 | Install Beeper Desktop from download tab; create/sign in to Beeper account |
| 2 | Note the **email** you used → `BEEPER_ACCOUNT_EMAIL=` in `.env` |
| 3 | In Beeper: link Instagram, Discord, Google Messages (when ready). **Skip** WhatsApp + Messenger / Facebook personal (out of v1) |
| 4 | Optional now: Settings → Developers → enable Desktop API; mint token → `BEEPER_ACCESS_TOKEN=` (desktop lab). Phone P2 Server will mint its own later |

### B. Spotify Web API (COMPLETE search + play)

| Step | Action |
|---|---|
| 1 | Dashboard → **Create app** → APIs: **Web API** |
| 2 | Redirect URI (add exactly): `http://127.0.0.1:8888/callback` *(placeholder; we can change once Operator deep-link is fixed)* |
| 3 | Copy Client ID / Secret → `.env` below |
| 4 | Spotify **Premium** required for Web API playback |
| 5 | **Product:** this Client ID/Secret serve **all users**. Each user still OAuth-connects **their** Spotify in Operator (“Connect Spotify”). Your login is lab smoke only |

### C. Google Cloud (YouTube + Maps)

| Step | Action |
|---|---|
| 1 | Create/select a GCP project (e.g. `operator-v1`) |
| 2 | Enable: YouTube Data API v3, Places API (New or legacy), Routes API |
| 3 | **Credentials** → API key → `GOOGLE_MAPS_API_KEY=` / `YOUTUBE_API_KEY=` (same key OK if API-restricted) |
| 4 | Optional OAuth client for user-context Maps later — not required for basic search |

### D. Device

| Step | Action |
|---|---|
| 1 | Pixel: USB debugging on; `adb devices` shows the phone |
| 2 | Google Messages = default SMS; SIM active (for GM via Beeper) |

### E. HAND-OFF surfaces (no new cloud OAuth)

Uber / DoorDash / Venmo / etc. need apps **installed + logged in on the Pixel** — no partner API keys for HAND-OFF.

---

## `.env` keys to add (main checkout)

```bash
# --- Operator v1 COMPLETE / Beeper ---
BEEPER_ACCOUNT_EMAIL=
BEEPER_ACCESS_TOKEN=

# --- Spotify COMPLETE ---
SPOTIFY_CLIENT_ID=
SPOTIFY_CLIENT_SECRET=
SPOTIFY_REDIRECT_URI=http://127.0.0.1:8888/callback

# --- Google Maps + YouTube COMPLETE ---
GOOGLE_MAPS_API_KEY=
YOUTUBE_API_KEY=
```

After you fill these, reply **creds ready** (don’t paste secrets). I’ll verify key *names* exist and start the P2 implementation push.

---

## How to reproduce

1. Use the Chrome tabs from this session (or reopen URLs above).  
2. Create apps / enable APIs / copy values into main `.env`.  
3. Plug in Pixel; confirm `adb devices`.  
4. Tell agent **creds ready**.
