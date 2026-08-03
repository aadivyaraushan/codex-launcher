# Execute more — media, Maps (ABSORBED)

**Date:** 2026-08-02  
**Status:** **Absorbed into** [consumer-app-implementation-plan.md](consumer-app-implementation-plan.md) (Media + Maps rows, 2026-08-02).  
**Do not treat this file as the source of truth.**

## Locked into parent

| Surface | Ceiling |
|---|---|
| Spotify | `unverified` 2026-08-03 — adapter + OAuth flow built, tests green, but no user token exists yet, so playback has never been driven. One owner consent click away |
| YouTube | **DEMOTED to hands_off 2026-08-03, whole adapter** — both `read`/search and `play` go through the same `search.list` call, and the live key returns `403 PERMISSION_DENIED` because its Google Cloud project is restricted, so nothing has ever been carried to the end through this adapter. The earlier "video played on the Pixel" evidence was a hand-typed `am start -a VIEW` intent that never touched the adapter or the capability flow, so it shows Android can open a video, not that Operator can (like/subscribe deferred) |
| Google Maps places / directions | **COMPLETE 2026-08-03, both device-verified** (`saved-results/wave3-maps-directions-pixel.md`) |
| Google Maps nav-intent | **HAND-OFF 2026-08-03** (was COMPLETE) — Corrected 2026-08-03: navigation is a **HAND-OFF**, not COMPLETE. The adapter named Google Maps as the app it passed control to while also reporting `completes`, and the phone's own codec throws that combination away (`ProtocolCodec.kt:187` — only a `hands_off` result may name an app). So this result had never actually rendered through the capability flow; the passing evidence came from a hand-typed `am start` intent. Operator computes the route and opens Maps with it loaded; Maps does the navigating |
| Google Maps saved-places write | HAND-OFF (no API) |
| Netflix | **Dropped from this push** (not necessary now); stay HAND-OFF / unbuilt |
| Bookings / orders / pay | HAND-OFF (sensitive / money) |

Historical draft content lived here before fold-in; see git history if needed.
