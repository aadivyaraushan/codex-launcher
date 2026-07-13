# Existing-task queue, redirect, and stop V1

**Date:** 2026-07-14  
**Purpose:** Record the implemented and verified busy-task control behavior for Codex Launcher V1.

## Result

- An idle task sends a follow-up immediately through its verified current owner.
- A busy task stores Queue follow-ups in the companion SQLite database and starts only the oldest prompt after a terminal turn event.
- Restart recovery enumerates every durable existing-task queue, including tasks outside the four Home rows. It starts an idle task and cancels a prepared prompt with cleared text when the task no longer exists.
- If the task becomes busy again at the write boundary, the prompt returns to `PREPARED` at the same queue position with its text intact.
- Redirect reloads the task and uses `turn/steer` only when the current adapter still reports a working turn with redirect support. If support or state changed, the prompt remains queued.
- Stop requires an Android confirmation dialog and uses the same durable `PREPARED -> SENT_UNKNOWN -> CONFIRMED` boundary. Duplicate confirmed or unknown Stop actions never send a second interrupt.
- Queue state is projected as `none`, `queued`, or `outcome_unknown`; unknown state survives Android process recreation, blocks every follow-up/redirect/stop for that task, and tells the user to check desktop Codex.
- `I checked Codex` sends an idempotent metadata-only dismissal to the companion. It cancels the exact unknown action without retrying Codex, clears its stored prompt text, then removes the phone's unknown record.
- Desktop IPC `ErrWriteOutcomeUnknown` and app-server unknown outcomes follow the same path for start, redirect, and stop. Replaying the action ID never writes twice.
- Archiving a task cancels all prepared follow-ups and clears their prompt text before the archive result is reported as safely stored.
- A queued task stores its verified owner source. Restart reads that exact task directly rather than treating absence from the latest 20 catalog rows as deletion; temporary lookup failures retain the queue. A definitive source-specific unavailable result cancels prepared and unknown entries and clears their text.
- The app-server deletion check accepts only its exact missing-thread response. A live local `thread/read` check returned JSON-RPC code `-32600` and message `thread not loaded: 00000000-0000-4000-8000-000000000000`; other RPC errors stay temporary and keep the prompt. Desktop owner connection failures also keep the prompt because they do not prove the task was deleted.
- Dispatch performs a second owner-specific read immediately before the write. If that safety read fails, the entry returns to `PREPARED` without losing its prompt or queue position.
- Archive is blocked while an unknown control still needs explicit review, so the task row and `I checked Codex` action cannot disappear first.
- Existing-task `START_TURN` records never enter or get removed through the new-task review path.
- The phone persists only the existing metadata-only action record. Prompt text remains in the owner-only companion database until dispatch, then is cleared after a confirmed or definite failed send.

## TDD evidence

The new tests failed before implementation because `ActiveTurnID`, `CanRedirect`, `ErrSendDeferred`, existing-task adapter methods, Android control outcomes, and Compose controls did not exist. The restart test also failed with zero dispatch calls before recovery was added. The duplicate Stop test initially called Stop twice before Stop actions were moved into the durable queue state machine. Independent review then found three rounds of failures: Desktop unknown writes returned `failed`; Android forgot or misclassified unknown existing-task actions after recreation; restart ignored queues outside Home; a bounded 20-task lookup could erase an older prompt; Archive could hide an unknown control; and broad owner-read failures could incorrectly cancel a stored prompt. Each regression test failed before its fix and passed afterward.

## Verification

```text
go test ./companion/... -count=1
PASS: all companion packages

go test -race ./companion/internal/promptqueue ./companion/internal/durablestore ./companion/internal/codex/appserver ./companion/internal/codex/taskstate ./companion/internal/codex/taskadapter ./companion/internal/mobileapi/contract ./companion/internal/app/mobilesession -count=1
PASS: all seven changed backend packages

./gradlew :app:testDebugUnitTest :app:lintDebug
BUILD SUCCESSFUL in 1s

./gradlew :app:connectedDebugAndroidTest
Starting 66 tests on codex_launcher_pixel_9_api_36(AVD) - 16
Finished 66 tests on codex_launcher_pixel_9_api_36(AVD) - 16
BUILD SUCCESSFUL in 1m 33s
```

The focused busy-task Compose test typed a follow-up, tapped Queue, opened Stop, chose Keep working, reopened Stop, and confirmed Stop task. A second focused Pixel test displayed the unknown warning and tapped `I checked Codex`. The launcher appearance interaction was also run by itself after adding an explicit wait for encrypted startup state. All focused tests passed before the final full 66-test run.

The exact app-server missing-thread shape was checked against the installed local Codex binary without sending a model request:

```json
{"error":{"code":-32600,"message":"thread not loaded: 00000000-0000-4000-8000-000000000000"},"id":"3"}
```

A fourth independent review graded the final behavior against prompt safety, FIFO restart recovery, uncertain-write handling, verified deletion, archive visibility, and new-task/existing-task separation. It returned `READY` with no remaining high- or medium-severity failure path.

## Reproduce

```bash
cd /Users/aadivyar/Documents/Codex/2026-07-12/uf-u-implementation
go test ./companion/... -count=1

cd android
JAVA_HOME=/Library/Java/JavaVirtualMachines/temurin-17.jdk/Contents/Home \
ANDROID_HOME=/opt/homebrew/share/android-commandlinetools \
ANDROID_SDK_ROOT=/opt/homebrew/share/android-commandlinetools \
./gradlew --no-daemon --max-workers=1 :app:testDebugUnitTest :app:lintDebug :app:connectedDebugAndroidTest
```

## Remaining Task 10 work

- Android speech recognition into editable composer text.
- Android document/photo pickers and companion attachment storage, quota, hashing, cleanup, and restart tests.
