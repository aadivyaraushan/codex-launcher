# Owner internal-release keystore (public only)

**Date:** 2026-08-06
**Purpose:** Record frozen signing identity without secrets
**For:** Wave 4 provenance / App Links / Google Android OAuth fingerprint

## Local paths (do not commit secrets)

- Keystore: `$HOME/.config/codex-launcher/operator-internal-release.jks`
- Env helper: `$HOME/.config/codex-launcher/store-env.sh` — **copy into 1Password now**
- README: `$HOME/.config/codex-launcher/README.txt`

## Public certificate digests

- Alias: `operator-internal-release`
- SHA-256: `35:63:9E:DA:D0:22:41:45:76:5B:0D:0F:2B:21:CD:9C:8C:D9:6B:E6:59:2B:DF:BB:4B:8F:39:D4:A9:F3:F3:E3`
- SHA-1: `D3:2A:DC:34:AB:FF:A7:92:87:1B:2E:FC:33:18:A0:FE:3D:59:B4:99`
- Compact SHA-256: `35639edad0224145765b0d0f2b21cd9c8cd96be6592bdfbb4b8f39d4a9f3f3e3`

## MSAL redirect note

Configured redirect hash in `msal_auth_config.json` is `96ha9R3kgapcHRIRwPGNGwaDxX8=` (prior/debug signer).
Release SHA-1 base64 is `0yrcNKv/p5KHGy78Mxig/j1ZtJk=`. Interactive Outlook consent still succeeded on the release APK in step6 re-smoke; update Azure app registration + config before relying on signature-bound redirect long-term.

## Google OAuth note

New release SHA-1 must be added to the Google Cloud Android OAuth client for package `app.codexlauncher` (project `operator-504223`). Until then, signed-APK Google AuthorizationClient may fail (observed `api_8` after account pick).


## SHA registration (2026-08-06T04:36Z)

Added release SHA-1 + SHA-256 to Firebase Android app on `operator-504223` via Management API. Debug fingerprints kept. Signed-APK Google AuthorizationClient re-smoke **PASS**.
