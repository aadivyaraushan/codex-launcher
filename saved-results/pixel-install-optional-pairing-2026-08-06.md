# Pixel install — optional pairing build

**Date:** 2026-08-06  
**Purpose:** Put optional Mac-pairing + finish-consumer tip on physical Pixel after merge to main.

## Device
- Serial: `4B230DLAQ001Z5` (Pixel 9)
- Prior install: lastUpdateTime `2026-08-06 08:50:05` (pre–optional-pairing)
- New install: lastUpdateTime `2026-08-06 10:43:50`
- Signer unchanged (`signatures:[d1e644b9]`) — `adb install -r` of signed release

## What was installed
1. **Operator APK** — `main` @ `fb15075` signed release (`app-release.apk`, SHA-256 `b55042ff…`)
2. **phone-runtime** — rebuilt linux/arm64 from same tip, SHA-256 `3190271f…`, copied into Debian proot `/usr/local/bin/operator-phone-runtime`, `sv restart phone-runtime` (pid 16074)

## Verify
- `adb install -r` Success
- Health still `mode=standalone_phone`, `process=serving` after runtime restart
