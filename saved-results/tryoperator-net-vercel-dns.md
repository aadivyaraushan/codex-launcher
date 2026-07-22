# tryoperator.net → Operator waitlist

Date: 2026-07-23

## What this is for

Point GoDaddy domain `tryoperator.net` at the Vercel `operator` waitlist site.

## Result

- **Vercel project:** `operator` has domains `tryoperator.net` and `www.tryoperator.net`
- **Apex DNS (GoDaddy):** `A @ → 76.76.21.21` only (WebsiteBuilder Site A removed)
- **www:** still `CNAME www → tryoperator.net.` (locked by `dpsAws`, but fine — follows apex to Vercel)
- **TLS:** Vercel cert issued for `tryoperator.net` + `www.tryoperator.net`
- **Verified (Vercel IP):** HTTPS 200, Operator landing HTML

Public DNS (`8.8.8.8` / local dig) returns only `76.76.21.21`.

## How we got here

1. Installed `gddy`, auth with DNS scopes
2. Added domains on Vercel
3. User unlocked/removed WebsiteBuilder A
4. `www` CNAME still immutable but points at apex → OK
5. `vercel certs issue tryoperator.net www.tryoperator.net`

## URLs

- https://tryoperator.net/
- https://www.tryoperator.net/
- Fallback: https://operator-waitlist.vercel.app/
