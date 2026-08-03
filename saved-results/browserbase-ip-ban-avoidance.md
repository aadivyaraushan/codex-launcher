# Browserbase: how they reduce IP / bot blocks

**Date:** 2026-08-02  
**Purpose:** Answer how Browserbase avoids IP bans (for IG / sandbox path).  
**Sources checked:**  
- https://docs.browserbase.com/platform/identity/proxies  
- https://docs.browserbase.com/platform/identity/overview  
- https://docs.browserbase.com/platform/identity/authentication  
- Context7 `/websites/browserbase`

## Bottom line

They do **not** promise “never IP-banned.” They sell a stack that makes sessions look more like normal users:

1. **Residential proxies** (opt-in — default is off)
2. **Verified fingerprints** (Scale plan — real Chromium fingerprints partners recognize)
3. **CAPTCHA solving**
4. **Persisted contexts** (reuse cookies so you’re not re-logging from a new IP every time)
5. Optional **Web Bot Auth** (Cloudflare Signed Agents — site must cooperate)

## What the docs say

### Proxies (main IP lever)

- Default: `proxies` is **false** → traffic exits Browserbase’s own network identity (not residential).
- `proxies: true` → managed **residential** proxies; best-effort US, may fall back to nearby countries (e.g. Canada).
- Or set geo: country / US state / city.
- Or bring your own HTTP(S) proxy.
- Developer plan+ for built-in and custom proxies; usage billed by bandwidth (1 MB minimum per proxied session).
- Third-party proxy providers **restrict high-risk categories** (banking, gov, streaming, ticketing, webmail, gambling, etc.). Support is not a blanket yes/no per site — **test the target**.

### Verified (fingerprint lever, not IP)

- Purpose-built Chromium with **real** fingerprints that bot-protection partners recognize.
- Scale plan (trial via hello@browserbase.com).
- Docs recommend pairing Verified + proxies for protected sites.
- API also still lists `advancedStealth` beside `verified`.

### Auth docs’ explicit “prevent IP-based blocking” recipe

1. Contexts (persist cookies / tokens)  
2. Verified fingerprints  
3. Proxies (rotate residential + match location)

### CAPTCHA

- `solveCaptchas: true` on sessions (we already used this).

## Relevance to our IG spike

Our DM sessions used **context + captcha**, but **`proxies` was left default false**. So Instagram saw Browserbase cloud egress IPs, not residential. That matches the “check Login activity for weird city” test.

`advancedStealth` earlier returned **403 Enterprise-only**; current docs push **Verified** instead (Scale).

## How to test IP risk cleanly (from docs + our earlier advice)

1. Session A: `proxies: false` → note Login activity city / new-login email.  
2. Session B: `proxies: true` (optional geo near the account’s home) → compare.  
3. Optionally try Verified if plan allows.  
4. Keep tests on the throwaway account.
