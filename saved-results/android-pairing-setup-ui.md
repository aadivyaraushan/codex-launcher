# Android Pairing Setup UI Evidence

**Date:** 2026-07-13

## Purpose

Record the first user-visible pairing setup flow for the Android launcher.

## Implemented result

- A new unpaired install opens `Pair with your computer` instead of pretending
  that an unnamed computer is merely offline.
- Setup supports a CameraX QR view and manual `codex-launcher://pair` entry.
  Camera permission is requested only from the scan path, and camera hardware
  is optional so devices without it can use manual entry.
- ZXing Core decodes only QR frames. The existing strict pairing parser then
  rejects the wrong scheme, public/non-Tailscale addresses, malformed keys,
  wrong protocol, duplicate fields, and invalid secrets before networking.
- Pairing runs off the UI thread, cannot run twice at once, saves the validated
  non-secret record before reporting success, and shows fixed safe retry text.
  If the computer accepts pairing but local storage fails, retry saves only the
  retained non-secret record; it never resends the consumed one-time code.
- Pairing work belongs to an activity-scoped Android ViewModel, so rotation does
  not cancel accepted pairing before the local record is saved. Manually entered
  one-time codes are cleared as soon as the computer accepts them.
- The phone identifier is a random app-private value stored in its own
  DataStore. It does not read Android's hardware/device identifier. Corrupt IDs
  are replaced; unavailable identity storage blocks pairing.
- All apps and Android Settings remain usable before pairing. Android Back and
  later Home intents return an unpaired user to setup rather than a false Home
  state.
- Camera redraws keep one live analyzer/executor and use the latest callback,
  avoiding a stopped camera pipeline after ordinary Compose state changes. A
  closed-screen guard also prevents a delayed camera-provider callback from
  binding after the user switches to manual entry or leaves setup.
- Startup renders a neutral loading surface until paired-computer storage emits,
  so a previously paired user is not briefly shown the false unpaired setup.

## TDD evidence

- The first state/decoder/screen build failed on missing `PairingViewModel`,
  `QrCodeDecoder`, and `PairingScreen` classes.
- The activity test then failed after three seconds because the old launcher
  still showed offline Home. It passed after pairing state was wired to startup.
- The app-private identity test failed on missing `DeviceIdentityStore`, then
  passed after stable generation, repair, and I/O failure behavior were added.
- Save-only retry, ViewModel-owned submission, and camera-close tests first
  failed to compile because those contracts did not exist. The focused emulator
  run then passed all 11 pairing/activity tests after implementation.
- The accepted-secret clearing and loading-route tests first failed on the old
  state behavior/private startup route, then passed after the final fixes. A
  further route test proved that the loaded paired/unpaired root immediately
  overrides a stale opposite root while preserving secondary screens.

## Verification

```text
Android JVM tests: 82, 0 failures, 0 errors, 0 skipped
Android API 36 Pixel 9 AVD tests: 34, 0 failures, 0 errors, 0 skipped
Android lint: PASS, 0 errors
git diff --check: PASS
```

Two intermediate full emulator runs were invalid because the implementation
run and independent judge both installed test APKs on the same AVD at once;
Android killed the instrumentation process with signal 9 during
`package.install`. The named test passed alone, and the full gate above passed
after the second runner was stopped and the AVD was used exclusively.

Visual checks on the running Pixel 9/API 36 emulator covered the permission
card in both light and dark mode plus the granted-camera surface. The final
stable screenshots matched the existing Instrument Sans type, six-pixel corner
language, warm-paper/deep-charcoal surfaces, orange selected action, safe
system insets, and bottom escape links. Computer Use could not address the
Android Emulator as a Mac app, so direct Android screenshots plus Compose UI
tests were used as the documented fallback.

The emulator camera bound successfully and logged:

```text
[pairing-camera] level=INFO message=QR camera ready decision=scan_qr_only input_shape=y_luminance
```

Reproduce:

```bash
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export JAVA_HOME=$(/usr/libexec/java_home -v 17)
export PATH="$ANDROID_HOME/platform-tools:$PATH"
./android/gradlew -p android test lint connectedDebugAndroidTest
git diff --check
```

## Current dependency evidence

Official release pages were checked before implementation. CameraX `1.6.1`
and ZXing `3.5.4` are current stable releases. Lifecycle `2.11.0` is current,
but requires compile SDK 37; this Android 16 project therefore uses the prior
stable `2.10.0` line instead of changing platform scope during a pairing UI
change. Android's current ViewModel documentation was also checked before
moving pairing submission into `viewModelScope`; it confirms that ViewModels
survive configuration changes and that their scope is cleared with the
ViewModel.

## Still pending

- A physical Pixel 9 scan of a QR code generated by the real companion over
  Tailscale. The emulator proves camera binding and the JVM test decodes a real
  generated QR image, but this result does not claim the end-to-end scan.
- Rotation is safe, but process death after computer acceptance and before the
  local save can still lose the in-memory pending record. Companion-side
  recovery or the later explicit unpair/re-pair flow must close that gap.
- Pairing setup has no dedicated maximum-font-scale test, and the analyzer has
  no fake-camera test for padded row/pixel strides. The screen does scroll and
  the decoder has real generated-QR coverage, but those exact cases remain.
- App-private identity/Keystore failures currently use the same safe
  Tailscale-oriented retry text as network failures; later error mapping should
  distinguish local phone storage from computer reachability.
- Visible reduced-protection confirmation/blocking, explicit unpair/re-pair,
  and the atomic local-state wipe.
- The companion must provide a computer display name before Home can replace
  the temporary fixed `Paired computer` label truthfully.
- Project selection and the live session/connection state still need their UI
  wiring.
