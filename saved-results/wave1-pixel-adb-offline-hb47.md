# Wave 1 Pixel adb offline (heartbeat ~47)

**Date:** 2026-08-02  
**Purpose:** Record that USB/adb lost the Pixel mid-overnight loop; Auto→Open smokes cannot run until reconnect.  
**Callers:** consumer-plan loop hb47; remaining-walls refresh.

## Inputs → Outputs → Algorithm

1. **Inputs:** Expected serial `4B230DLAQ001Z5`; prior paired device `android-3dfb533f-…`; LIVE `serve-deeplink-proof` pid 52653.  
2. **Outputs:** Confirmed empty `adb devices`; keys still missing; walls updated.  
3. **Algorithm:** `adb devices` / `adb -s … get-state` → empty/not found; USB profiler showed no Pixel; do not revoke pair; wait for owner USB/wireless reconnect.

## Result

| Check | Result |
|---|---|
| `adb devices` | **empty** (after `adb kill-server` / `start-server`) |
| `adb -s 4B230DLAQ001Z5 get-state` | **device not found** |
| `system_profiler SPUSBDataType` | no Pixel/Google Android hit in filtered scan |
| Deeplink serve | still running pid **52653** |
| Companion `devices` (hb47) | still listed: `android-3dfb533f-f341-42d8-acc6-cd4218146d62` Pixel 9, `pairedAt=2026-08-02T08:42:44Z` — do not revoke |
| Telegram / Podcasts / Notion keys | still **MISSING** |
| OAuth Approves | still owner wall |
| Prior Auto→Open | still valid evidence (Maps/Spotify/YouTube/WhatsApp) — not invalidated by USB drop |

## Owner action

Replug USB (or re-enable wireless adb). Pairing in companion state should still exist — do **not** revoke unless Home shows Pair again after reconnect.

## How to reuse

```bash
adb devices -l
adb -s 4B230DLAQ001Z5 get-state
```

## Recheck heartbeat ~48

`adb devices` still **empty**. Serve still pid **52653**. Keys still missing.

## Recheck heartbeat ~49

Still empty `adb devices`. Serve 52653. Keys missing.

## Recheck heartbeat ~50

Still empty `adb devices`. Serve 52653.

## Recheck heartbeat ~51

Still empty `adb devices`. Serve 52653.
