# Task 11 decision-path checkpoint

Date: 2026-07-14

## What this records

This checkpoint covers phone approval and question handling for both tasks owned
by the companion's Codex app-server and active tasks owned by ChatGPT Desktop.

## Result

- The companion publishes only generic attention events until the phone asks for
  the live decision page.
- Every response is matched to its task, turn, item, request ID, and request kind.
- The phone can send only decisions Codex offered. MCP elicitation is limited to
  `decline` and `cancel` on the phone.
- Secret questions cannot be answered from the phone.
- Commands are redacted on the computer. If the remaining command cannot be
  understood, allow buttons are disabled while deny remains available.
- `Not now` dismisses a normal question only on the phone and sends no response.
- App-server and ChatGPT Desktop requests have separate owner-aware phone IDs, so
  reused short request IDs cannot collide across owners or tasks.
- A disconnected owner keeps the request pending. Any response whose delivery
  outcome is unknown is removed so the phone cannot accidentally send it twice.
- Phone responses cross the existing durable action-journal boundary before they
  are sent.

## Verification

Run from the repository root with the listed JDK and Android SDK environment:

```sh
go test -race ./companion/...

export JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools
cd android
./gradlew :app:testDebugUnitTest --no-daemon
./gradlew :app:connectedDebugAndroidTest --no-daemon
```

Observed results:

- Every companion package passed under Go's race detector.
- Android unit tests completed with `BUILD SUCCESSFUL`.
- The Pixel 9 API 36 emulator reported 75/75 instrumentation tests completed,
  zero skipped, and zero failed on Android 16.
- The four focused decision-screen tests passed on the same emulator.
- `jq empty` accepted all three protocol schema files.

The independent judge initially found three gaps: response-order handling,
companion-side rejection of unclear-command approvals, and Desktop tombstone
republication. Each received a failing regression test and a fix. The judge then
returned `READY` with no remaining P1/P2 finding, after the full suites above were
rerun successfully.

Computer Use could not add a manual screenshot check because macOS was locked and
automatic unlock failed. No manual visual claim is made; emulator UI tests are the
fallback evidence.
