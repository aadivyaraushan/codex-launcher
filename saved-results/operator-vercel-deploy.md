# Operator Vercel deployment

Date: 2026-07-22

## What this is for

Public URL for the **Operator** waitlist landing (renamed from AgentOS).

## Result

- **Production URL:** https://operator-waitlist.vercel.app/ (public; `operator.vercel.app` was already taken)
- **Project:** `aadivyaraushans-projects/operator` (renamed from `agentos`)
- Old `agentos-beryl.vercel.app` alias removed (404)
- SSO deployment protection disabled so the waitlist URL is public

## Branding

Landing copy, nav, footer, device chrome, and meta tags use **Operator** (e.g. “Introducing Operator.”).

## Waitlist env

Still needs Sheet webhook + token — see `waitlist-vercel-sheets-setup.md`.

## Reproduce

```sh
cd ".../codex-launcher-landing-outcome"
vercel deploy --prod --yes --scope aadivyaraushans-projects
```
