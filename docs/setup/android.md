# Install and pair the Android launcher

V1 is designed and tested primarily for a Pixel 9 on Android 16. The APK may
work on other devices running Android 12 or later, but those devices are not yet
part of the release test set.

## Before you start

1. Complete the [computer companion setup](companion.md).
2. Confirm the relay box is running and the computer companion passes
   `./codex-launcher doctor` (or `.\codex-launcher.exe doctor` in PowerShell).
3. Download the APK and verify it against `SHA256SUMS.android`.

The APK is signed by the release workflow. Android can still ask whether to
allow installation from the app that opened the download. Grant that permission
only for this install, then turn it off again if you do not normally sideload.

For a development build connected over USB:

```bash
adb install -r codex-launcher-0.1.0-alpha.1.apk
```

## Pair

1. From the extracted companion folder on macOS/Linux, run
   `./codex-launcher pair`. On Windows PowerShell, run
   `.\codex-launcher.exe pair`.
2. Open Codex Launcher on the phone.
3. Choose **Enter link**, paste the complete one-time link, and tap
   **Pair computer**. QR scanning is also available when the same link is shown
   as a QR code by a trusted local tool.
4. Choose one of the project folders approved during companion setup.

The phone stores a hardware-backed pairing key when the device supports it,
the pinned computer identity, and non-secret connection details. Your ChatGPT
sign-in stays on the computer.

## Make it the home screen

Open Android Settings and choose:

`Apps` → `Default apps` → `Home app` → `Codex Launcher`

You can reach Android Settings and **All apps** from the pairing screen or the
launcher menu even when the computer is offline. Appearance offers Follow
system, Light, and Dark; text size, contrast, and motion follow Android settings.

Allow notifications if you want replies, approvals, questions, and failures to
appear while the launcher is not visible. A persistent connection notification
is required while the phone maintains its sealed connection through the relay.

## Daily use

- Recent Codex tasks appear at the top of Home.
- Select the project above the blank prompt before starting new work.
- Attach photos or files with Android's system pickers. Files go only to the
  paired computer.
- When the computer cannot be reached, Home says `Computer offline`. Your
  unfinished draft remains on the phone, and normal app/settings access remains.
- When the relay box itself cannot be reached, Home says
  `Can't reach the relay box` and retries automatically.
- Approval and question cards are tied to the exact pending Codex request. An
  expired or replayed action is rejected.

## Remove the phone pairing

Use **Manage computer** in the launcher and confirm removal. This erases the
pairing, selected project, unfinished draft, and local action records from the
phone. It does not delete Codex tasks on the computer. If the phone is lost or
no longer trusted, also run `./codex-launcher revoke DEVICE_ID` from the
extracted companion folder on macOS/Linux, or
`.\codex-launcher.exe revoke DEVICE_ID` on Windows PowerShell.
