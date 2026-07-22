# Waitlist on Vercel → Google Sheets

Date: 2026-07-22

## What this is for

Persist public waitlist emails somewhere you can open in a browser (not `/tmp` on one machine), cheap and fast, before shipping the AgentOS landing on Vercel.

## Decision

**Vercel `/api/waitlist` → Google Apps Script web app → Google Sheet.**

- Cost: $0 at waitlist volume (Vercel hobby + free Google Sheet)
- Access: open the Sheet in any browser
- Spam: honeypot field, minimum time-to-submit (~1.5s), light per-IP rate limit, **required** shared token on the Apps Script URL
- Correctness: API parses Apps Script JSON and only returns success when `{ ok: true }` (Apps Script often uses HTTP 200 even on errors)

Docs checked: Context7 `/websites/vercel` (Node serverless handler in `/api`, `vercel.json` rewrites).

## Inputs → Outputs → Algorithm

1. **Inputs:** browser POST JSON `{ email, website, startedAt }` to `/api/waitlist`
2. **Outputs:** `{ ok: true }` (or 4xx/5xx); Sheet row `timestamp | email | ip | userAgent`
3. **Algorithm:**
   1. Reject non-POST / rate-limit IP
   2. Validate email; if honeypot filled or submit too fast → fake 200 (no write)
   3. POST to `WAITLIST_SHEETS_WEBHOOK_URL` with optional `WAITLIST_SHEETS_TOKEN`
   4. Apps Script appends a row on the `signups` tab

## One-time setup (you)

1. Create a Google Sheet (name it e.g. `AgentOS waitlist`).
2. **Extensions → Apps Script**, paste `landing/waitlist-apps-script.js`, save.
3. **Required:** **Project Settings → Script properties** → `WAITLIST_SHEETS_TOKEN` = a long random string (same value as Vercel).
4. **Deploy → New deployment → Web app**
   - Execute as: Me
   - Who has access: Anyone
   - Copy the Web app URL (`…/macros/s/…/exec`)
5. In Vercel project env (Production + Preview):
   - `WAITLIST_SHEETS_WEBHOOK_URL` = that URL
   - `WAITLIST_SHEETS_TOKEN` = same string as the Script property (required — API refuses to store without it)
6. Deploy this branch (`landing/outcome-copy` or merge to main). Root `/` is rewritten to `landing/index.html` via `vercel.json`.

## Local checks

```sh
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher-landing-outcome"
node --test api/waitlist/validate.test.js
# then: vercel dev   (with env vars set)
```

## Reproduce / reuse

- Handler: `api/waitlist/index.js`
- Validation: `api/waitlist/validate.js`
- Sheet bridge: `landing/waitlist-apps-script.js`
- Env template: `.env.example`

Example Sheet row (synthetic):

| timestamp | email | ip | userAgent |
|---|---|---|---|
| 2026-07-22T17:30:00.000Z | you@example.com | 203.0.113.10 | Mozilla/5.0… |
