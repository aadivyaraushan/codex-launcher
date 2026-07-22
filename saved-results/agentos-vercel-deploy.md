# AgentOS Vercel deployment

Date: 2026-07-22

## What this is for

Public URL for the AgentOS waitlist landing (scroll story + `/api/waitlist`).

## Result

- **Production URL:** https://agentos-beryl.vercel.app/
- **Project:** `aadivyaraushans-projects/agentos`
- **Deployed from:** worktree `codex-launcher-landing-outcome`, branch `landing/outcome-copy`
- Inspect: https://vercel.com/aadivyaraushans-projects/agentos/

## Waitlist persistence (not fully live yet)

API is deployed, but emails will not stick until you set:

1. Google Sheet + Apps Script from `landing/waitlist-apps-script.js` (see `saved-results/waitlist-vercel-sheets-setup.md`)
2. Vercel env (Production + Preview):
   - `WAITLIST_SHEETS_WEBHOOK_URL`
   - `WAITLIST_SHEETS_TOKEN`

```sh
vercel env add WAITLIST_SHEETS_WEBHOOK_URL production --scope aadivyaraushans-projects
vercel env add WAITLIST_SHEETS_TOKEN production --scope aadivyaraushans-projects
vercel deploy --prod --yes --scope aadivyaraushans-projects
```

Until then, signup returns `{"ok":false,"error":"misconfigured"}` (verified 2026-07-22).

## Reproduce

```sh
cd ".../codex-launcher-landing-outcome"
vercel deploy --prod --yes --scope aadivyaraushans-projects
```
