# Pixel 9 verification requirements — adversarial judge

**Date:** 2026-08-03  
**What for:** Judge whether messaging + consumer plans require Pixel 9 drive-and-verify / self-verification for messaging components and everything else in this push.  
**Callers:** this chat; plan readiness  
**Same-purpose:** supersedes earlier PASS-WITH-FIXES body in this file  
**Data:** plan text only  
**Owner instruction:** verification by driving Pixel 9 for messaging plan components and self-verification for everything else  

## Verdict: **PASS**

After fixing the leftover Spotify Build-cell weasel (`clear “no device”`), all nine prior fix items are present. No remaining weasel that lets COMPLETE claim without on-device Pixel proof for the load-bearing push surfaces.

## Nine-fix checklist

| # | Fix | Status |
|---|---|---|
| 1 | IG Pixel observation | OK |
| 2 | On-device evidence; API id alone insufficient | OK |
| 3 | Lab OK for spike; COMPLETE needs Pixel | OK |
| 4 | Pay/booking → consumer Pixel table | OK |
| 5 | Spotify no-active-device not COMPLETE pass (table + Build + smoke) | OK |
| 6 | Pay/bookings minimum set named | OK |
| 7 | Instagram feed HAND-OFF row | OK |
| 8 | this-push vs Wave sentence | OK |
| 9 | YouTube fail → HAND-OFF / unverified | OK |

## Where it lives

- `planning/operator-complete-messaging-plan.md` — **Pixel 9 drive-and-verify**
- `planning/consumer-app-implementation-plan.md` — **This implementation push — Pixel 9 self-verification** + locked decision row
