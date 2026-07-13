# Read-only task transcript checkpoint

**Date:** 2026-07-13

## What this is for

This Task 9 checkpoint makes the four Home rows open real, read-only Codex task
history without storing task contents on the phone or in the companion replay
journal.

## Result

- App-server tasks are read with `thread/read` and `includeTurns=true`.
- Verified Desktop tasks are read from the follower state already loaded and
  authorized for the matching Home row. Unresolved catalog rows fail closed.
- The mapper keeps user and agent messages, plan text, reasoning summaries,
  command text/output, and file paths/diffs in their original order. Hidden
  reasoning content is ignored. An empty reasoning summary becomes the fixed
  label `Reasoning activity`.
- Incomplete Codex plan and file-change items remain valid while they are being
  built: empty plan text becomes `Plan activity`, and a file item with no safe
  changes becomes the content-free label `File activity`.
- Pages contain at most 64 entries, each entry has an 8,192-character content
  budget, and file items contain at most 64 changes. The mapper also keeps the
  encoded page below 192 KiB, leaving room inside the 256 KiB protocol frame.
- `task_read` and `task_page` have no sequence number. The companion sends pages
  directly to the current authenticated phone connection and never applies them
  to the event journal.
- Go and Android both reject `null` transcript arrays. The companion validates
  the complete outgoing `task_page`; an invalid source page becomes an empty,
  retryable internal error before anything is sent.
- The phone correlates request and task IDs, prepends earlier pages, rejects
  overlapping or cross-task results, ignores responses after the task closes,
  and clears transcript state on disconnect or process loss.
- Home rows are tappable. The Task screen shows chat text and collapsed activity.
  Command output and file diffs open only after an explicit tap on their detail
  control. Back, Home, and connection loss return to content-free
  launcher state.
- Task and detail destinations are chosen directly from the current connection
  and in-memory transcript state. A disconnect therefore removes visible
  command output or a diff in the same redraw; it does not wait for later
  cleanup work.

## Test-first evidence

Observed failures before implementation included:

```text
tasktranscript: undefined: MapAppServerPage, PageOptions, and transcript kinds

TestTranscriptTruncationDescribesOnlyTheReturnedPage:
latest page inherited older truncation

TestTranscriptPageCapsFileChangesAndEncodedSize:
bounded file page retained 164 changes

TestTaskReadRejectsCrossTaskSourceResultWithoutLeakingContent:
response contained "private reply from another task"

ProtocolContractTest:
task_read rejected as INVALID_ENVELOPE

LauncherSessionViewModelTest:
openTask, loadEarlierTranscript, and transcript state were undefined

TestIncompleteItemsMapToValidContentFreeActivityLabels:
empty plan text and zero-change file items produced invalid protocol entries

TestContractRejectsTranscriptInternalsAndMalformedPages:
Go accepted entries:null and changes:null while Android rejected them

TestTaskReadConvertsInvalidSourcePageToContentFreeInternalError:
the invalid source page reached the sender instead of a content-free error

TaskScreenTest.disconnectSynchronouslyRemovesVisiblePrivateDetail:
visibleDestination had no connection or transcript inputs
```

Final verified commands:

```bash
go test -race -count=1 ./companion/...
go vet ./companion/...

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home \
./android/gradlew -p android testDebugUnitTest lintDebug

ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home \
./android/gradlew -p android connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=app.codexlauncher.launcher.home.HomeScreenTest,app.codexlauncher.task.transcript.TaskScreenTest
```

The final emulator command passed 12 tests on
`codex_launcher_pixel_9_api_36(AVD) - 16`.

An independent final review returned `READY` after checking the protocol
fallbacks, null-array parity, complete outgoing-page validation, and immediate
disconnect removal of private detail content. The reviewer independently ran
`git diff --check` and `LauncherStartupPolicyTest`; both passed.

## Current API source

The installed local source was checked before implementation:

```text
codex-cli 0.144.1
codex app-server generate-json-schema --experimental
```

The generated schema says `thread/read` includes turns and their items when
`includeTurns` is true. Its item union supplied the user, agent, reasoning,
plan, command, and file-change fields mapped here.

## Remaining work

This is read-only history. Live item deltas still update only the Home summary,
not an already open transcript. Rename, archive, fork, new-task settings,
prompt/steer/interrupt, approval cards, and question cards remain later Task 9
through Task 11 work.
