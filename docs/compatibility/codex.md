# Codex and platform compatibility

Codex Launcher is a technical alpha. `experimental` means the code and automated
tests exist, but the required native real-Codex flow has not passed on that
platform.

## Current matrix

| Platform | Service lifecycle | Codex path | Status |
| --- | --- | --- | --- |
| Pixel 9, Android 16 emulator | Launcher, foreground connection, reboot, force-stop, Doze, airplane-mode recovery | Paired mobile protocol | Tested in emulator |
| macOS 15.5 arm64 | Native LaunchAgent install, replace, rollback, uninstall | ChatGPT Desktop follower plus local app-server | Experimental until unlocked live flow passes |
| Linux amd64/arm64 | systemd user service; cross-build and automated backend tests | Public local Codex app-server | Experimental; native lifecycle/live flow open |
| Windows 11 amd64/arm64 | limited current-user scheduled task; cross-build and automated backend tests | Local Codex app-server; Desktop named-pipe path unverified | Experimental; native lifecycle/live flow open |

Android has 239 local JVM tests and 84 Android 16 instrumentation tests in the
latest launcher-service checkpoint. The exact count can change as coverage is
added; CI runs the complete current suites rather than relying on this number.

## Verified Codex and ChatGPT versions

Development probes used:

- ChatGPT Desktop package `26.707.51957` on macOS;
- the ChatGPT-bundled `codex-cli 0.144.0-alpha.4`;
- standalone `codex-cli 0.144.1`.

These are evidence from the recorded probe, not a promise that all later builds
work. The private Desktop adapter checks the installed ChatGPT build, socket
location, process owner, and protocol message versions. If it cannot prove
compatibility, Android shows `Desktop integration needs an update` and write
actions remain disabled.

## Why there are two Codex paths

The public Codex app-server can list, read, resume, and control tasks inside its
own local runtime. It cannot attach to the in-memory runtime of an already-active
ChatGPT Desktop task.

On macOS, Codex Launcher therefore uses ChatGPT Desktop's private same-user
follower bridge for Desktop-owned tasks. The user accepted this maintenance
tradeoff for V1. It is isolated behind a pinned adapter and the companion's
paired mobile contract; raw private frames never reach Android. The companion
does not silently replace an incompatible Desktop task with a different task.

Linux has no ChatGPT Desktop host and uses the public app-server path for tasks
owned by the companion. Windows contains the private named-pipe transport code,
but it remains experimental until native verification passes.

## Compatibility check

After installing or updating Codex, ChatGPT Desktop, the relay box, or the
companion, run:

```bash
./codex-launcher version
./codex-launcher doctor
./codex-launcher status
```

On Windows PowerShell:

```powershell
.\codex-launcher.exe version
.\codex-launcher.exe doctor
.\codex-launcher.exe status
```

`doctor` checks the Codex version, relay reachability, pinned key and registration
secret, service state, phone-door reachability, local schema, pinned computer
identity, and the last fixed service error code without taking the companion's
live relay slot, starting the normal runtime, or repairing state.

No paid model call is required for these checks. A true end-to-end task-start
smoke can use the user's authenticated Codex account and may incur whatever
costs that account normally carries; it must not be run automatically.
