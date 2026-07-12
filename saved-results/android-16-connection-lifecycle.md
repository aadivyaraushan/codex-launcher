# Android 16 Connection Lifecycle Gate

**Date:** 2026-07-13
**Purpose:** Verify that the Pixel 9 Android 16 launcher can keep a private
computer connection in a foreground service and recover a dropped WebSocket.

## Core gate result: PASS

The core connected-device lifecycle passed on a visible Pixel 9 API 36 ARM64
AVD. This proves the proposed foreground-service type, notification, initial
WebSocket, retry, same-endpoint reconnection, and stop path. It does not yet
prove all release stress cases listed below.

## Official Android requirements used

From the current [foreground-service type documentation](https://developer.android.com/develop/background-work/services/fgs/service-types#connected-device):

- Manifest service type: `connectedDevice`.
- Manifest permission: `FOREGROUND_SERVICE_CONNECTED_DEVICE`.
- Runtime prerequisite used here: declared `CHANGE_NETWORK_STATE`.
- Intended use includes external-device interaction requiring a network
  connection.

From the current [foreground-service launch documentation](https://developer.android.com/develop/background-work/services/fgs/launch)
and [background-start restrictions](https://developer.android.com/develop/background-work/services/fgs/restrictions-bg-start):

- Start with `startForegroundService()` while the launcher is user-visible.
- Promote immediately with a visible notification and the declared type.
- Android 12+ rejects arbitrary background starts with
  `ForegroundServiceStartNotAllowedException`; release behavior must not assume
  an undocumented exemption.

## Test environment

```text
AVD: codex_launcher_pixel_9_api_36
Hardware profile: Pixel 9
OS: Android 16 / API 36
ABI: arm64-v8a
Emulator: 36.6.11
System image: Google APIs ARM64 revision 7
OkHttp / MockWebServer: 5.4.0
```

## Observed green behavior

`ConnectionLifecycleSpikeTest` performed these actions inside the real VM:

1. Granted notification permission for the test app.
2. Started a loopback WebSocket server.
3. Started the launcher service with `startForegroundService()`.
4. Observed state `CONNECTED` and one successful connection.
5. Found foreground notification ID `4101` through Android's active
   notifications API.
6. Verified the installed service declares
   `FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE`.
7. Closed the server WebSocket with `1012 Service Restart` while keeping the
   same host and port.
8. Observed state `RETRYING`.
9. Observed a second successful connection on the same endpoint and at least one
   reconnect attempt.
10. Stopped the service and observed state `STOPPED`.

Final VM result:

```text
Starting 2 tests on codex_launcher_pixel_9_api_36(AVD) - 16
Finished 2 tests on codex_launcher_pixel_9_api_36(AVD) - 16
BUILD SUCCESSFUL
```

Production logs use `[connection]` and record only input/output shapes, branch
reasons, counts, and safe exception shapes. Endpoint values and WebSocket text
are never logged. Cleartext WebSocket traffic is enabled only by the debug
manifest; release traffic remains subject to Android's cleartext block and the
plan requires pinned TLS.

## Remaining release stress cases

These are still unproven and must remain open for Tasks 8 and 12:

- default-launcher cold start and role switching;
- reboot and user unlock;
- screen off and Doze;
- Wi-Fi to cellular transition;
- Tailscale stop, restart, update, and VPN loss;
- notification denial and notification-channel removal;
- explicit background-start rejection handling;
- Android task-manager service stop;
- OS process kill and recovery;
- user force-stop and the required manual reopen behavior.

## Reproduce

With the Pixel 9 API 36 AVD booted:

```bash
export JAVA_HOME=$(/usr/libexec/java_home -v 17)
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
./android/gradlew -p android :app:connectedDebugAndroidTest
```
