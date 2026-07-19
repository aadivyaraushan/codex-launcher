# Saved-task opening and dark-title verification

**Date:** 2026-07-19
**Purpose:** Record the physical Pixel proof for the saved-task transcript and
dark-theme title fixes in commit
`a5a41ba33d9c2c9e45401d60e576c0582eea8191`.

## Result

Both reported issues are fixed on physical device `4B230DLAQ001Z5`, a Pixel 9
running Android 16:

- Two different saved tasks opened to their transcripts. Neither final UI dump
  contains `Task unavailable`.
- The companion read both tasks through the post-restart
  `catalog_candidate` path. It returned 5 entries for task
  `019f79cf-33c6-77a2-a37f-29f12c864d2a` and 2 entries for task
  `019f799a-b98f-7592-8ef0-5de1a7ee05a6`.
- The screenshots show the title, back arrow, menu, transcript text, and controls
  in light text on the dark screen. The physical-device pixel test also passed
  with 1 test, 0 failures, and 0 errors.

Local evidence is retained under `.context/task-open-contrast/`:

- `task-1-fixed.png`, SHA-256
  `3e350776afb49020d65e93fc1855d45b8d1d82136b5c175b0bd1e009b8b9941e`
- `task-2-fixed.png`, SHA-256
  `90f34d0f22adaa0a437de808c1b4311bcaa1503ca43dc43a0be19a2b2bc0c5ef`
- `window-task-1.xml` and `window-task-2.xml`, both checked for the absence of
  `Task unavailable`

## Before and after

Before the fix, the same installed companion logged `owner_unavailable` for
task reads at 14:38:43 and 14:50:31. The task screen screenshot showed its title
and primary controls in black on the black background.

After the fix, the companion logged:

```text
15:31:49 transcript ready ... source=catalog_candidate output_count=5 ...
15:32:21 transcript ready ... source=catalog_candidate output_count=2 ...
```

The matching phone UI dumps contained `Task transcript` plus transcript content,
and the audit command printed:

```text
TASK_UNAVAILABLE_ABSENT_IN_BOTH_UI_DUMPS
```

## Builds and checks

- Android APK SHA-256:
  `09a23d5048a0c2910e9bfe8a93b3ec85517e0a0803168d6709c4a5978b1c064c`
- Installed app: version `0.1.0-alpha.1`, version code 1
- Installed Mac companion SHA-256:
  `69feaf8782fab103ed3c8e7c3001d0efcebad88a2474579380e44cffd6d1af2a`
- Companion source commit:
  `a5a41ba33d9c2c9e45401d60e576c0582eea8191`
- `go test -race ./...`: passed
- `go vet ./...`: passed
- `./gradlew testDebugUnitTest lintDebug assembleDebug`: passed
- Physical Pixel test
  `TaskScreenTest#darkTaskScreenUsesReadablePrimaryText`: 1 passed, 0 failed
- Companion `doctor`: 7 checks, 0 failed

The Android test runner cleared the phone's pairing data. The obsolete Mac-side
record for that same Pixel was revoked, and the phone was paired again. Final
status reports one paired device and a running LaunchAgent.

## Reuse

To repeat the two direct checks after installing matching builds:

```sh
./android/gradlew -p android :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.task.transcript.TaskScreenTest#darkTaskScreenUsesReadablePrimaryText

rg 'Task unavailable' \
  .context/task-open-contrast/window-task-1.xml \
  .context/task-open-contrast/window-task-2.xml
```

The second command should return no matches. Open two saved tasks in the normal
app after the instrumented test because the test runner may clear pairing data.
