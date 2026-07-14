# Android 16 final local verification

Date: 2026-07-14

Purpose: record the final local Android and repository checks for the Codex Launcher V1 audit work. This is local verification evidence, not proof of a live Tailscale or model-backed Codex session.

## Environment

- Worktree: `/Users/aadivyar/Documents/Codex/2026-07-12/uf-u-implementation`
- Branch: `codex-launcher-v1`
- Android device: `codex_launcher_pixel_9_api_36` AVD
- Android version: 16
- ADB serial: `emulator-5554`
- Android SDK: `/opt/homebrew/share/android-commandlinetools`
- Audited source commit: `84854bf1ccb05a11a8fc6936676414d38621f6fb`
- Audited debug APK SHA-256: `fb27c0dd9a86d07704fcdaa6e75dd006b119e365d7833937c37ad4c78a922527`
- Audited source state: clean; the runner found no source changes outside the excluded evidence folders.

## Results

- Hands-on launcher audit: 66 passed, 0 failed. It covered 10 real launcher checks, all 42 fixed UI states, and 14 interaction groups. Full screenshots and per-check links are in `saved-results/android-16-hands-on-ui-audit.md`.
- Audit-runner unit tests: 7 passed, including exact APK installation, source/build identity, and replace-before-retry text entry.
- Android unit tests: 248 passed, 0 failed, 0 skipped. Evidence: `android/app/build/test-results/testDebugUnitTest/TEST-*.xml`.
- Android device tests: 108 total, 0 failed, 1 skipped by its external-network guard. Evidence: `android/app/build/outputs/androidTest-results/connected/debug/TEST-codex_launcher_pixel_9_api_36(AVD) - 16-_app-.xml`.
- Android lint and builds: `lintDebug`, `assembleDebug`, and `assembleRelease` passed.
- Release APK isolation: 7 assertions passed. The debug APK contains the audit activity; the release manifest and DEX do not contain the audit activity or scenario classes.
- Release contract checks: every `release/checks/*.test.mjs` file passed. The public-alpha contract alone reported 120 assertions.
- Companion: `go test ./... -race -count=1` and `go vet ./...` passed.
- Protocol schemas: 34 valid frames accepted and 38 invalid frames rejected.
- Shell syntax: `bash -n release/checks/companion-smoke.sh` passed.

## Commands

```sh
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools node release/checks/android-emulator-ui-audit.mjs
go test ./... -race -count=1
go vet ./...
for test in release/checks/*.test.mjs; do node "$test" || exit 1; done
python3 release/checks/protocol/schema_test.py
bash -n release/checks/companion-smoke.sh
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./android/gradlew -p android --no-daemon --stacktrace testDebugUnitTest lintDebug assembleDebug assembleRelease
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools ./android/gradlew -p android --no-daemon --stacktrace connectedDebugAndroidTest
```

## Limits and remaining live checks

- `command -v tailscale` returned no path on this Mac, so no real Tailscale pairing, disconnect, mobile-data handoff, or reconnect was run.
- No model-backed Codex call was made. No paid credential was used or approved in this verification run.
- No physical Pixel 9 was connected; the Android checks used the Pixel 9 API 36 emulator profile.
- Native Windows and Linux VM smoke tests were not run in this verification pass.
- The debug scenario activity uses production Compose screens with local synthetic state. It proves rendering and phone-side interactions, not the live desktop transport. The 10 production checks separately use the real `LauncherActivity`, Android Home role, permission dialog, Settings, persistence, force-stop, and reboot paths.

## Independent judge

The final fresh judge found zero local code/evidence P1 or P2 gaps and marked this incremental source-and-evidence checkpoint safe to commit. It did not mark the whole V1 ready because the live Tailscale, unlocked model-backed Desktop, physical Pixel 9, and native Windows/Linux release gates above remain open.

## Reuse

Run the commands above from the worktree root with the named Android 16 emulator online. Live Tailscale and model-backed tests must remain separate and require the needed environment plus explicit billing-account approval before any paid call.
