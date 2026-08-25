# Operator Vercel deployment

Date: 2026-07-23 (updated)

## What this is for

Public URL for the **Operator** waitlist landing, and which git branch to deploy from.

## Result

- **Custom domain:** https://tryoperator.net/
- **Project:** `aadivyaraushans-projects/operator`
- **Deploy branch:** `landing/operator` (modular landing-only surface; not `main`)
- Latest prod deploy from this branch aliased to tryoperator.net (2026-07-23)
- `.vercelignore` keeps uploads to landing/api/lib (skips android, companion, etc.)

## Why a separate branch

`main` carries the full Android/companion monorepo (local build artifacts can make CLI uploads huge). `landing/operator` is the maintained deploy branch for the waitlist site so production stays small and clear.

Landing files also exist on `main` (merged earlier) for source visibility; **production deploys should come from `landing/operator`.**

## Waitlist env

Still needs Sheet webhook + token — see `waitlist-vercel-sheets-setup.md`.

## Reproduce

```sh
# From the landing worktree or any checkout of landing/operator:
git checkout landing/operator
vercel --prod --yes --scope aadivyaraushans-projects
# Expect: Aliased https://tryoperator.net
```

Worktree path used in this session:
`/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher-landing-outcome`
