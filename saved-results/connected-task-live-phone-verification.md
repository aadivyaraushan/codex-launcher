# Connected task live-view phone verification

**Date:** 2026-07-19  
**Purpose:** Record the refresh, reasoning, title, and keyboard checks for the
connected-task fixes on the physical Pixel.

## Result

The updated Android app passed the automated and physical checks on Pixel 9
`4B230DLAQ001Z5`, running Android 16.

The final Conductor-workspace implementation commit is
`d4b248a244366bcaab8f8a58be8b5e894cf6df19`. The APK built from that workspace
has SHA-256
`1719052d066759ec6c83e9afe52a3c758de60e13cdd606f60e0a070a91e4dc75`.

- The paired Mac received transcript reads at `16:20:20`, `16:20:22`,
  `16:20:24`, `16:20:26`, `16:20:28`, and `16:20:30` while one task remained
  open. This is the requested two-second refresh interval; the task was not
  reopened between reads.
- After installing the final workspace APK and pairing it fresh, the same live
  check repeated at `16:47:17`, `16:47:19`, and `16:47:21`, returning 32
  entries from one continuously open task on every read.
- The connected transcript returned 29 entries on every read. Android logged
  `read_mode=refresh` and applied each 29-entry page in memory.
- Collision tests prove a live update arriving during either the initial read
  or an older-page read is coalesced into one immediate follow-up read. A
  refreshed latest page also preserves the oldest loaded cursor, so it cannot
  make `Load earlier` refetch duplicate entries and disconnect the session.
- The physical Compose test rendered a `Reasoning` row and its safe summary,
  `Checking the package`. The real stored task inspected during the hands-on
  run had 179 reasoning records but zero provider-supplied summary parts in
  those records. The app continues to exclude hidden reasoning content and can
  display only safe summaries supplied through the task API.
- The long task title measured `[168,184][912,310]` when collapsed (126 px
  high), exposed the `Expand task title` action, expanded to
  `[168,184][912,751]` (567 px high), and returned to the original bounds after
  `Collapse task title` was tapped.
- With the real keyboard open, the app window was `[0,0][1080,1666]`, the
  follow-up field was `[42,1277][1038,1424]`, and the send-row label ended at
  y=1536. The former roughly 800–1000 px empty band was absent; the keyboard
  begins directly below the composer area in the screenshot.
- The Android test runner removed the installed app and its pairing data. A
  fresh APK was installed and the obsolete exact Pixel pairing record was
  revoked before pairing the phone again. The one-time pairing link and all
  temporary files containing it were deleted after use.

Local screenshots and UI dumps are under
`.context/worktrees/connected-task-live-fix/.context/phone-live-verification/`.
They include `task-live.png`, `keyboard-fixed.png`, `task-live.xml`,
`keyboard-fixed.xml`, `title-state.xml`, and `title-collapsed.xml`.

## Test evidence

```text
./gradlew :app:testDebugUnitTest
BUILD SUCCESSFUL in 6s

./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.task.transcript.TaskScreenTest
Finished 14 tests on Pixel 9 - 16
BUILD SUCCESSFUL in 32s

./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.task.transcript.TaskScreenTest#transcriptRendersChatAndKeepsLargeDetailsBehindExplicitActions
Finished 1 tests on Pixel 9 - 16
BUILD SUCCESSFUL in 13s

./gradlew :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
BUILD SUCCESSFUL in 45s
```

The first unit-test attempt was the required red run. It failed because
`transcriptRefreshWait` and `TRANSCRIPT_REFRESH_MILLIS` did not exist. After the
runtime implementation, the same `LauncherSessionViewModelTest` target passed.

## Same-bug search

The repository was searched for `safeDrawingPadding()` and every
`TaskControls(` call. The only other full safe-drawing padding is the separate
read-only transcript-detail screen, which has no keyboard. `TaskControls` has
one production caller, the task transcript screen changed here. No sibling
keyboard path needed the same fix.

## Reuse

To repeat the connected timing check, note the companion log line count, open a
task on the paired phone, leave it open for at least six seconds, and inspect
new `message_type=task_read` lines in
`~/Library/Logs/CodexLauncher/companion.log`. Consecutive reads should be two
seconds apart. Then focus the follow-up field and use `uiautomator dump` plus a
screenshot to compare the composer bounds with the visible app window.

Final operator checks reported one paired device, a running LaunchAgent, and
`doctor` completed 7 checks with 0 failures. The app was left open on the
connected task with the keyboard dismissed.
