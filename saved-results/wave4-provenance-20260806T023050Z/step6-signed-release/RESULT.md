# Wave 4 step 6 — signed internal-release install/smoke

**Date:** 2026-08-06T04:27Z
**Purpose:** First frozen owner internal-release key + install/re-smoke on Pixel
**No passwords in this file.**

## Keystore (public only)

- Path (local, outside git): `$HOME/.config/codex-launcher/operator-internal-release.jks`
- Alias: `operator-internal-release`
- Env helper (local secrets): `$HOME/.config/codex-launcher/store-env.sh` — **copy into 1Password now; do not commit**

```
# Public signing certificate digests only — no passwords/secrets
# Date: 2026-08-06T04:25Z
# Keystore path (local-only, outside git): /Users/aadivyar/.config/codex-launcher/operator-internal-release.jks
# Alias: operator-internal-release
# Algorithm: RSA 2048, validity 10000 days
SHA-256: 35:63:9E:DA:D0:22:41:45:76:5B:0D:0F:2B:21:CD:9C:8C:D9:6B:E6:59:2B:DF:BB:4B:8F:39:D4:A9:F3:F3:E3
SHA-1: D3:2A:DC:34:AB:FF:A7:92:87:1B:2E:FC:33:18:A0:FE:3D:59:B4:99
```

## Build

- Task: `:app:assembleRelease` (JDK 17)
- APK: `android/app/build/outputs/apk/release/app-release.apk`
- Evidence: `assemble-release.log` (BUILD SUCCESSFUL)

## Install / hashes

```
a4327a70865a803cb377bc8686a80428db464a9c4880e93786f0b36c746534ad  /Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe/android/app/build/outputs/apk/release/app-release.apk
a4327a70865a803cb377bc8686a80428db464a9c4880e93786f0b36c746534ad  /Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe/saved-results/wave4-provenance-20260806T023050Z/step6-signed-release/installed.apk
```

- Installed cert SHA-256 matches frozen key (`CERT_MATCH=1`)
- Install: `adb install -r` Success (see `install.txt`)

## Smoke

- Launch: `LauncherActivity` resumed; pid present → **PASS** (`STEP6_SMOKE=PASS`)
- Note: `am start` with LAUNCHER category alone failed resolve; monkey + explicit activity path showed `app.codexlauncher/.LauncherActivity` on top.

## Animation restore (user request)

Prior (test low-motion):
```
prior_window_animation_scale=0
prior_transition_animation_scale=0
prior_animator_duration_scale=0
```
Restored:
```
restored_window_animation_scale=1.0
restored_transition_animation_scale=1.0
restored_animator_duration_scale=1.0
```

## Remaining vs full plan

- Full real-account matrix on this signed APK not re-run in this pass (launch/cert smoke only).
- Clean provenance sibling rebuild at HEAD with this signer still open if claiming full Wave 4 exit.
- User must save `~/.config/codex-launcher/store-env.sh` into a password manager.
