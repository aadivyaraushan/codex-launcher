# Wave 1 companion service restart + Pixel re-pair

**Date:** 2026-08-02 (heartbeat ~40)  
**Purpose:** Unblock Auto→Open after overnight Pair wall.  
**Callers:** continuous consumer-plan loop; `wave1-overnight-remaining-walls.md`; overnight batch status.

**Gate facts (pre-create):**
1. **Callers:** Overnight status / remaining walls / snapshot cite this evidence. No Go/Kotlin imports.
2. **No duplicate:** No prior `wave1-*re-pair*` / companion-restart evidence for hb40.
3. **Data files:** None in-repo. Pair URI only under `/tmp/codex-launcher-pair-hb40.uri` mode 600 (not copied here). Device id is non-secret.
4. **User instruction (verbatim):** Continue consumer-app-implementation-plan until done … Skip owner-only outward asks unless unblocked … Record evidence in saved-results/.

## Inputs → Outputs → Algorithm

1. **Inputs:** Pixel on Pair UI; doctor `service_running=false` / LaunchAgent unloaded; Mac still listed stale paired device.  
2. **Outputs:** LaunchAgent companion running; fresh Pixel pair; Home UI with Auto.  
3. **Algorithm:** Stop orphan deeplink-proof process → bootstrap LaunchAgent → remove stale Mac device entry → mint pair offer → `LiveFlyPairingInjectTest` with `pairLinkB64` → confirm session + Home.

## Verified results

| Check | Result |
|---|---|
| Doctor before | Companion log ends prior run with `Companion service stopped unexpectedly.` then restart `2026/08/02 12:36:18` local (= `08:36Z`). `health.json` still carries `lastError=service_stopped_unexpectedly` / `updatedAt=2026-08-02T08:37:05Z` (historical detail; doctor `last_error` check can be green while this string remains). |
| LaunchAgent | `gui/501/app.codexlauncher.companion` running (`serve`); doctor **failed_count=0**, `service_running=true` |
| Stale device | No longer listed in `devices`. Prior id `android-28099ed4-0a75-471c-ba12-743cde8e96b9` (Pixel 9; first `device paired` in companion.log `2026/07/22 19:36:53` local). Companion.log has no `revoke` line for the removal. |
| Fresh pair | `android-3dfb533f-f341-42d8-acc6-cd4218146d62` at `2026-08-02T08:42:44.82968Z` (`devices`) |
| Session | companion.log (local UTC+4): `12:42:44` `device paired` → `12:43:18` `session authenticated` → `12:43:19` `hello` → `ack` for that device id |
| Pixel UI | Serial `4B230DLAQ001Z5`: `content-desc=Launcher home`; texts **MacBook Pro**, **Auto**, **What do you want done?**; `content-desc=Send prompt using Auto` / `Auto destination selected` (not Pair) |
| Inject test | `LiveFlyPairingInjectTest` reported compose wait failure after submit, but Mac `devices` + companion.log show pair succeeded (treat as flake on post-pair assertion, not pair failure). Instrumentation stdout not pasted in this file. |

## Secrets hygiene

- Pair URI / `secret=` **not** stored in `saved-results/`.
- Local only: `/tmp/codex-launcher-pair-hb40.uri` (600), `/tmp/codex-launcher-pair-hb40.png`.
- Do not commit those temp files.

## Follow-ups

1. Auto→Open smokes for Wave1Specs still need a companion process that loads the **76-Spec deeplink pack** (`serve-deeplink-proof`). Installed LaunchAgent `serve` keeps the phone paired; swapping to deeplink-proof may replace the mobile runtime — do deliberately before smokes.  
2. OAuth Approves still open (`wave1-oauth-approve-runbook.md`).  
3. Telegram keys still missing.

## How to reuse

```bash
"$HOME/Library/Application Support/codex-launcher/bin/codex-launcher" doctor
# pair offer from `codex-launcher pair`; never log decoded URI
adb shell am instrument -w -e pairLinkB64 "$B64" \
  -e class app.codexlauncher.connection.pairing.LiveFlyPairingInjectTest \
  app.codexlauncher.test/androidx.test.runner.AndroidJUnitRunner
```

Judge: `wave1-companion-restart-pixel-repair-judge.md` (Pass-with-warnings).
