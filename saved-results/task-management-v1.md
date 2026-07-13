# Task management V1 checkpoint

Date: 2026-07-13

## What this checkpoint adds

The Android task screen can Rename, Archive, or Fork the current Codex task
when the computer advertises `task_management`. The phone sends only exact,
validated action shapes. Rename trims and validates the new title, Archive
requires confirmation, and Fork uses an extra durable duplicate guard because
forking twice creates two tasks.

The companion rechecks that the sending phone session is still the active
session immediately before the Codex write. It then stores a sequenced action
result and refreshes the task snapshot from Codex. A confirmed phone action is
not acknowledged until that newer snapshot is applied. If snapshot refresh
temporarily fails, the companion retries three times with bounded waits; after
the final failure it closes active phone sessions so reconnect must obtain a
fresh snapshot.

## Crash and uncertain-result behavior

The phone stores metadata-only `PREPARED`, `SENT_UNKNOWN`, and `CONFIRMED`
records using the existing action journal. It does not store task titles or
result bodies.

Fork has a stricter rule:

1. The phone reads durable unresolved records before every Fork.
2. A `SENT_UNKNOWN` Fork for the same task blocks another socket write.
3. An explicit companion `outcome_unknown` result is acknowledged at the
   sequence layer but deliberately leaves the Fork record `SENT_UNKNOWN`.
4. The unresolved task IDs reload when a phone session is rebuilt after a
   disconnect or process recreation.
5. The task screen shows `Previous fork unconfirmed` and keeps Fork disabled.
6. Only the explicit `I checked Codex` action dismisses that durable record and
   allows a later Fork.

This is deliberate manual reconciliation. A companion restart can lose its
current in-memory event journal, but it cannot make the phone blindly repeat an
uncertain Fork. The user must first inspect desktop Codex. Runnable companion
startup still needs the durable production stores already listed in the main
plan; this checkpoint exercises the injected store contracts and session
handler.

## TDD evidence

The new tests were run before implementation and failed on the missing
`NeedsReview`, unresolved-action journal methods, recreated-session state, and
task-screen warning controls. After implementation:

```text
cd companion
go test ./... -count=1
all packages passed

go test -race ./internal/app/mobilesession ./internal/mobileapi/contract ./internal/codex/taskadapter -count=1
all 3 packages passed
```

```text
cd android
./gradlew testDebugUnitTest lintDebug connectedDebugAndroidTest
180 JVM tests, 0 failures, 0 errors
58 tests on codex_launcher_pixel_9_api_36(AVD), 0 failed
lintDebug passed
BUILD SUCCESSFUL
```

The first post-fix full Pixel run had one unrelated `LauncherActivityTest`
failure because Compose temporarily found `All apps` only in the unmerged
semantics tree. That exact test passed immediately when rerun alone, and the
next full 58-test Pixel run passed with zero failures.

The Pixel instrumentation tests interacted with the real Compose dialogs and
buttons. Codex Computer Use could not inspect the emulator because the emulator
had no visible application window in its app list, so the full API 36
instrumentation run was used as the UI fallback.

An independent judge first found the reconnect duplicate-Fork gap and then the
explicit `outcome_unknown` gap. After both fixes and their recreation tests, its
final verdict was `READY`; it found no remaining correctness issue in this
checkpoint.

## Same-bug search

The duplicate-side-effect audit searched every `SENT_UNKNOWN` use and every
action bridge. Project selection and Rename converge on one value; Archive is
safe to repeat against an already archived task. Fork is the only current task
management action that creates a second object when repeated, so it receives
the durable per-task review gate. Future create/send actions must add their own
proven reconciliation or equivalent durable block before they are exposed.

## Re-run

```text
export ANDROID_HOME=/opt/homebrew/share/android-commandlinetools
export ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools

cd companion
go test ./... -count=1
go test -race ./internal/app/mobilesession ./internal/mobileapi/contract ./internal/codex/taskadapter -count=1

cd ../android
./gradlew testDebugUnitTest lintDebug connectedDebugAndroidTest
```
