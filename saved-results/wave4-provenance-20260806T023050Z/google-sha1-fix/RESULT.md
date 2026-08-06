# Google signed-APK re-smoke after SHA-1 registration

**Date:** 2026-08-06T04:36Z
**Project:** `operator-504223` (Firebase Android app `1:914186874774:android:b822c0ac6633aa00c0b7b7`, package `app.codexlauncher`)

## SHA registration (API, no browser login needed)

Firebase Management `projects.androidApps.sha.create`:

| Cert | Hash | Kept prior? |
|---|---|---|
| SHA_1 release | `d32adc34abffa792871b2efc3318a0fe3d59b499` (= `D3:2A:DC:34:…:B4:99`) | added |
| SHA_256 release | `35639edad0224145765b0d0f2b21cd9c8cd96be6592bdfbb4b8f39d4a9f3f3e3` | added |
| SHA_1 debug | `f7a85af51de481aa5c1d1211c0f18d1b0683c57f` | **kept** |
| SHA_256 debug | `c613e6607c404042e6913517278638946b29c18dc1cd81e92c9d3699a580c25d` | **kept** |

## Device re-smoke

- Installed APK signer SHA-256 match: **PASS** (`signer.txt`)
- `GoogleAuthorizeActivity` → account `ssdear@gmail.com` → **Calendar + Drive access granted**: **PASS**
- Animation scales remain **1.0**

Evidence: `result.txt`, UI dumps `s*.xml`, `logcat.txt`
